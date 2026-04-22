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
});
