import { createHmac } from "node:crypto";
import { Hono } from "hono";
import { describe, expect, it, vi } from "vitest";
import { createGitHubRoutes } from "../github.js";

function sign(payload: string, secret: string): string {
	return `sha256=${createHmac("sha256", secret).update(payload).digest("hex")}`;
}

const webhookSecret = "test-webhook-secret";

function makeApp(overrides?: {
	findByGitHubRunId?: ReturnType<typeof vi.fn>;
	updateStatus?: ReturnType<typeof vi.fn>;
}) {
	const mockRunService = {
		findByGitHubRunId: overrides?.findByGitHubRunId ?? vi.fn(),
		updateStatus: overrides?.updateStatus ?? vi.fn(),
	};

	const mockGitHubClient = {
		getWorkflowRun: vi.fn().mockResolvedValue({
			runId: 12345,
			status: "completed",
			conclusion: "success",
			htmlUrl: "https://github.com/org/repo/actions/runs/12345",
			headBranch: "main",
			pullRequests: [],
		}),
	};

	const app = new Hono();
	app.route(
		"/api",
		createGitHubRoutes({
			// biome-ignore lint/suspicious/noExplicitAny: test mocks
			runService: mockRunService as any,
			// biome-ignore lint/suspicious/noExplicitAny: test mocks
			githubClient: mockGitHubClient as any,
			webhookSecret,
		}),
	);

	return { app, mockRunService, mockGitHubClient };
}

function workflowRunPayload(action: string, conclusion: string | null = null) {
	return {
		action,
		workflow_run: {
			id: 12345,
			name: "claude-code-action",
			status: action === "completed" ? "completed" : "in_progress",
			conclusion,
			html_url: "https://github.com/org/repo/actions/runs/12345",
			head_branch: "main",
			head_sha: "abc123",
			event: "workflow_dispatch",
		},
		repository: {
			full_name: "org/repo",
		},
	};
}

describe("POST /api/webhooks/github", () => {
	it("rejects requests with invalid signature", async () => {
		const { app } = makeApp();
		const body = JSON.stringify(workflowRunPayload("completed"));

		const res = await app.request("/api/webhooks/github", {
			method: "POST",
			body,
			headers: {
				"x-hub-signature-256": "sha256=invalid",
				"x-github-event": "workflow_run",
			},
		});

		expect(res.status).toBe(401);
	});

	it("ignores non-workflow_run events", async () => {
		const { app } = makeApp();
		const body = JSON.stringify({ action: "opened" });

		const res = await app.request("/api/webhooks/github", {
			method: "POST",
			body,
			headers: {
				"x-hub-signature-256": sign(body, webhookSecret),
				"x-github-event": "push",
			},
		});

		expect(res.status).toBe(200);
		const json = (await res.json()) as { ignored: boolean };
		expect(json.ignored).toBe(true);
	});

	it("ignores workflow runs not tracked by sidekick", async () => {
		const findByGitHubRunId = vi.fn().mockResolvedValue(null);
		const { app } = makeApp({ findByGitHubRunId });
		const body = JSON.stringify(workflowRunPayload("completed", "success"));

		const res = await app.request("/api/webhooks/github", {
			method: "POST",
			body,
			headers: {
				"x-hub-signature-256": sign(body, webhookSecret),
				"x-github-event": "workflow_run",
			},
		});

		expect(res.status).toBe(200);
		const json = (await res.json()) as { ignored: boolean };
		expect(json.ignored).toBe(true);
	});

	it("updates run to completed on success", async () => {
		const updateStatus = vi.fn().mockResolvedValue({});
		const findByGitHubRunId = vi
			.fn()
			.mockResolvedValue({ id: "run-1", status: "running" });

		const { app, mockGitHubClient } = makeApp({
			findByGitHubRunId,
			updateStatus,
		});
		const body = JSON.stringify(workflowRunPayload("completed", "success"));

		const res = await app.request("/api/webhooks/github", {
			method: "POST",
			body,
			headers: {
				"x-hub-signature-256": sign(body, webhookSecret),
				"x-github-event": "workflow_run",
			},
		});

		expect(res.status).toBe(200);
		expect(updateStatus).toHaveBeenCalledWith(
			"run-1",
			"completed",
			expect.objectContaining({ result: expect.any(Object) }),
		);
		expect(mockGitHubClient.getWorkflowRun).toHaveBeenCalledWith({
			repo: "org/repo",
			runId: 12345,
		});
	});

	it("updates run to failed on failure conclusion", async () => {
		const updateStatus = vi.fn().mockResolvedValue({});
		const findByGitHubRunId = vi
			.fn()
			.mockResolvedValue({ id: "run-1", status: "running" });

		const { app } = makeApp({ findByGitHubRunId, updateStatus });
		const body = JSON.stringify(workflowRunPayload("completed", "failure"));

		const res = await app.request("/api/webhooks/github", {
			method: "POST",
			body,
			headers: {
				"x-hub-signature-256": sign(body, webhookSecret),
				"x-github-event": "workflow_run",
			},
		});

		expect(res.status).toBe(200);
		expect(updateStatus).toHaveBeenCalledWith(
			"run-1",
			"failed",
			expect.any(Object),
		);
	});

	it("updates run to running on in_progress", async () => {
		const updateStatus = vi.fn().mockResolvedValue({});
		const findByGitHubRunId = vi
			.fn()
			.mockResolvedValue({ id: "run-1", status: "queued" });

		const { app } = makeApp({ findByGitHubRunId, updateStatus });
		const body = JSON.stringify(workflowRunPayload("in_progress"));

		const res = await app.request("/api/webhooks/github", {
			method: "POST",
			body,
			headers: {
				"x-hub-signature-256": sign(body, webhookSecret),
				"x-github-event": "workflow_run",
			},
		});

		expect(res.status).toBe(200);
		expect(updateStatus).toHaveBeenCalledWith("run-1", "running");
	});
});
