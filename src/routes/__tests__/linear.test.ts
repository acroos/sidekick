import { createHmac } from "node:crypto";
import { Hono } from "hono";
import { describe, expect, it, vi } from "vitest";
import { createLinearRoutes } from "../linear.js";

function sign(payload: string, secret: string): string {
	return createHmac("sha256", secret).update(payload).digest("hex");
}

const webhookSecret = "linear-test-secret";

function makeApp(overrides?: {
	findLinearLabelAutomations?: ReturnType<typeof vi.fn>;
	executeLinearTrigger?: ReturnType<typeof vi.fn>;
	linearClient?: {
		findRecentlyLabeledIssue: ReturnType<typeof vi.fn>;
	} | null;
}) {
	const mockAutomationService = {
		findLinearLabelAutomations:
			overrides?.findLinearLabelAutomations ?? vi.fn().mockReturnValue([]),
		executeLinearTrigger:
			overrides?.executeLinearTrigger ?? vi.fn().mockResolvedValue("run-1"),
	};

	const app = new Hono();
	app.route(
		"/api",
		createLinearRoutes({
			// biome-ignore lint/suspicious/noExplicitAny: test mocks
			automationService: mockAutomationService as any,
			webhookSecret,
			// biome-ignore lint/suspicious/noExplicitAny: test mocks
			linearClient: (overrides?.linearClient ?? null) as any,
		}),
	);

	return { app, mockAutomationService };
}

function labelPayload(action: "create" | "remove", labelName: string) {
	return {
		action,
		type: "IssueLabel",
		data: {
			issueId: "issue-123",
			labelId: "label-456",
			label: { name: labelName },
		},
		url: "https://linear.app/team/issue/ENG-123",
		createdAt: "2026-04-22T00:00:00Z",
	};
}

function labelAppliedPayload(labelName: string) {
	return {
		action: "update" as const,
		type: "IssueLabel",
		data: {
			id: "label-456",
			name: labelName,
			color: "#blue",
			isGroup: false,
			lastAppliedAt: "2026-04-22T01:00:00Z",
			createdAt: "2026-01-01T00:00:00Z",
			updatedAt: "2026-04-22T01:00:00Z",
			organizationId: "org-1",
			creatorId: "user-1",
		},
		url: "https://linear.app/team/settings/labels",
		createdAt: "2026-04-22T01:00:00Z",
		updatedFrom: {
			lastAppliedAt: "2026-04-21T00:00:00Z",
			updatedAt: "2026-04-21T00:00:00Z",
		},
	};
}

describe("POST /api/webhooks/linear", () => {
	it("rejects requests with invalid signature", async () => {
		const { app } = makeApp();
		const body = JSON.stringify(labelPayload("create", "sidekick"));

		const res = await app.request("/api/webhooks/linear", {
			method: "POST",
			body,
			headers: { "linear-signature": "invalid" },
		});

		expect(res.status).toBe(401);
	});

	it("ignores non-label events", async () => {
		const { app } = makeApp();
		const body = JSON.stringify({
			action: "create",
			type: "Issue",
			data: { id: "issue-123" },
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		});

		const res = await app.request("/api/webhooks/linear", {
			method: "POST",
			body,
			headers: { "linear-signature": sign(body, webhookSecret) },
		});

		expect(res.status).toBe(200);
		const json = (await res.json()) as { ignored: boolean };
		expect(json.ignored).toBe(true);
	});

	it("ignores label events that don't match any automation", async () => {
		const { app } = makeApp();
		const body = JSON.stringify(labelPayload("create", "bug"));

		const res = await app.request("/api/webhooks/linear", {
			method: "POST",
			body,
			headers: { "linear-signature": sign(body, webhookSecret) },
		});

		expect(res.status).toBe(200);
		const json = (await res.json()) as { ignored: boolean };
		expect(json.ignored).toBe(true);
	});

	it("executes matching automation on label create", async () => {
		const automation = {
			name: "linear-issues",
			trigger: { connector: "linear", on_label: "sidekick" },
			notifications: [],
		};
		const findLinearLabelAutomations = vi.fn().mockReturnValue([automation]);
		const executeLinearTrigger = vi.fn().mockResolvedValue("run-1");

		const { app } = makeApp({
			findLinearLabelAutomations,
			executeLinearTrigger,
		});
		const body = JSON.stringify(labelPayload("create", "sidekick"));

		const res = await app.request("/api/webhooks/linear", {
			method: "POST",
			body,
			headers: { "linear-signature": sign(body, webhookSecret) },
		});

		expect(res.status).toBe(200);
		const json = (await res.json()) as { ok: boolean; runs: string[] };
		expect(json.runs).toEqual(["run-1"]);
		expect(executeLinearTrigger).toHaveBeenCalledWith({
			automation,
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});
	});

	it("does not execute on label remove", async () => {
		const automation = {
			name: "linear-issues",
			trigger: { connector: "linear", on_label: "sidekick" },
			notifications: [],
		};
		const findLinearLabelAutomations = vi.fn().mockReturnValue([automation]);
		const executeLinearTrigger = vi.fn();

		const { app } = makeApp({
			findLinearLabelAutomations,
			executeLinearTrigger,
		});
		const body = JSON.stringify(labelPayload("remove", "sidekick"));

		const res = await app.request("/api/webhooks/linear", {
			method: "POST",
			body,
			headers: { "linear-signature": sign(body, webhookSecret) },
		});

		expect(res.status).toBe(200);
		const json = (await res.json()) as { ignored: boolean };
		expect(json.ignored).toBe(true);
		expect(executeLinearTrigger).not.toHaveBeenCalled();
	});

	describe("IssueLabel update events (label applied)", () => {
		it("triggers automation via API lookup when label is applied", async () => {
			const automation = {
				name: "linear-issues",
				trigger: { connector: "linear", on_label: "sidekick" },
				notifications: [],
			};
			const findLinearLabelAutomations = vi.fn().mockReturnValue([automation]);
			const executeLinearTrigger = vi.fn().mockResolvedValue("run-1");
			const linearClient = {
				findRecentlyLabeledIssue: vi.fn().mockResolvedValue({
					id: "issue-789",
					url: "https://linear.app/team/issue/ENG-789",
				}),
			};

			const { app } = makeApp({
				findLinearLabelAutomations,
				executeLinearTrigger,
				linearClient,
			});
			const body = JSON.stringify(labelAppliedPayload("sidekick"));

			const res = await app.request("/api/webhooks/linear", {
				method: "POST",
				body,
				headers: { "linear-signature": sign(body, webhookSecret) },
			});

			expect(res.status).toBe(200);
			const json = (await res.json()) as { ok: boolean; runs: string[] };
			expect(json.runs).toEqual(["run-1"]);
			expect(linearClient.findRecentlyLabeledIssue).toHaveBeenCalledWith(
				"label-456",
			);
			expect(executeLinearTrigger).toHaveBeenCalledWith({
				automation,
				issueId: "issue-789",
				issueUrl: "https://linear.app/team/issue/ENG-789",
			});
		});

		it("ignores label applied event when no automations match", async () => {
			const { app } = makeApp();
			const body = JSON.stringify(labelAppliedPayload("unrelated"));

			const res = await app.request("/api/webhooks/linear", {
				method: "POST",
				body,
				headers: { "linear-signature": sign(body, webhookSecret) },
			});

			expect(res.status).toBe(200);
			const json = (await res.json()) as { ignored: boolean };
			expect(json.ignored).toBe(true);
		});

		it("ignores when no issue found for label", async () => {
			const automation = {
				name: "linear-issues",
				trigger: { connector: "linear", on_label: "sidekick" },
				notifications: [],
			};
			const findLinearLabelAutomations = vi.fn().mockReturnValue([automation]);
			const linearClient = {
				findRecentlyLabeledIssue: vi.fn().mockResolvedValue(null),
			};

			const { app } = makeApp({
				findLinearLabelAutomations,
				linearClient,
			});
			const body = JSON.stringify(labelAppliedPayload("sidekick"));

			const res = await app.request("/api/webhooks/linear", {
				method: "POST",
				body,
				headers: { "linear-signature": sign(body, webhookSecret) },
			});

			expect(res.status).toBe(200);
			const json = (await res.json()) as { ignored: boolean };
			expect(json.ignored).toBe(true);
		});

		it("returns 500 when linear client is not configured", async () => {
			const automation = {
				name: "linear-issues",
				trigger: { connector: "linear", on_label: "sidekick" },
				notifications: [],
			};
			const findLinearLabelAutomations = vi.fn().mockReturnValue([automation]);

			const { app } = makeApp({
				findLinearLabelAutomations,
				linearClient: null,
			});
			const body = JSON.stringify(labelAppliedPayload("sidekick"));

			const res = await app.request("/api/webhooks/linear", {
				method: "POST",
				body,
				headers: { "linear-signature": sign(body, webhookSecret) },
			});

			expect(res.status).toBe(500);
		});

		it("ignores label definition edits (no lastAppliedAt change)", async () => {
			const { app } = makeApp();
			const payload = {
				action: "update" as const,
				type: "IssueLabel",
				data: {
					id: "label-456",
					name: "sidekick",
					color: "#red",
					isGroup: false,
					lastAppliedAt: "2026-04-22T01:00:00Z",
					createdAt: "2026-01-01T00:00:00Z",
					updatedAt: "2026-04-22T01:00:00Z",
					organizationId: "org-1",
					creatorId: "user-1",
				},
				url: "https://linear.app/team/settings/labels",
				createdAt: "2026-04-22T01:00:00Z",
				// updatedFrom only has color, not lastAppliedAt
				updatedFrom: {
					color: "#blue",
					updatedAt: "2026-04-21T00:00:00Z",
				},
			};
			const body = JSON.stringify(payload);

			const res = await app.request("/api/webhooks/linear", {
				method: "POST",
				body,
				headers: { "linear-signature": sign(body, webhookSecret) },
			});

			expect(res.status).toBe(200);
			const json = (await res.json()) as { ignored: boolean };
			expect(json.ignored).toBe(true);
		});
	});
});
