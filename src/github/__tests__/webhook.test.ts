import { createHmac } from "node:crypto";
import { describe, expect, it } from "vitest";
import { parseWorkflowRunEvent, verifyWebhookSignature } from "../webhook.js";

function sign(payload: string, secret: string): string {
	return `sha256=${createHmac("sha256", secret).update(payload).digest("hex")}`;
}

describe("verifyWebhookSignature", () => {
	const secret = "test-secret";
	const payload = '{"action":"completed"}';

	it("returns true for valid signature", () => {
		const signature = sign(payload, secret);
		expect(verifyWebhookSignature({ payload, signature, secret })).toBe(true);
	});

	it("returns false for invalid signature", () => {
		expect(
			verifyWebhookSignature({
				payload,
				signature: "sha256=invalid",
				secret,
			}),
		).toBe(false);
	});

	it("returns false for wrong secret", () => {
		const signature = sign(payload, "wrong-secret");
		expect(verifyWebhookSignature({ payload, signature, secret })).toBe(false);
	});

	it("returns false for missing sha256 prefix", () => {
		expect(
			verifyWebhookSignature({
				payload,
				signature: "not-prefixed",
				secret,
			}),
		).toBe(false);
	});
});

describe("parseWorkflowRunEvent", () => {
	const basePayload = {
		action: "completed",
		workflow_run: {
			id: 12345,
			name: "claude-code-action",
			status: "completed",
			conclusion: "success",
			html_url: "https://github.com/org/repo/actions/runs/12345",
			head_branch: "main",
			head_sha: "abc123",
			event: "workflow_dispatch",
		},
		repository: {
			full_name: "org/repo",
		},
	};

	it("parses a valid completed workflow_run event", () => {
		const event = parseWorkflowRunEvent({
			eventType: "workflow_run",
			payload: basePayload,
		});

		expect(event).not.toBeNull();
		expect(event?.action).toBe("completed");
		expect(event?.workflowRun.id).toBe(12345);
		expect(event?.workflowRun.conclusion).toBe("success");
		expect(event?.repo).toBe("org/repo");
	});

	it("parses in_progress event", () => {
		const event = parseWorkflowRunEvent({
			eventType: "workflow_run",
			payload: { ...basePayload, action: "in_progress" },
		});

		expect(event?.action).toBe("in_progress");
	});

	it("returns null for non-workflow_run events", () => {
		const event = parseWorkflowRunEvent({
			eventType: "push",
			payload: basePayload,
		});

		expect(event).toBeNull();
	});

	it("returns null for unrecognized actions", () => {
		const event = parseWorkflowRunEvent({
			eventType: "workflow_run",
			payload: { ...basePayload, action: "deleted" },
		});

		expect(event).toBeNull();
	});

	it("handles null conclusion for in-progress runs", () => {
		const event = parseWorkflowRunEvent({
			eventType: "workflow_run",
			payload: {
				...basePayload,
				action: "in_progress",
				workflow_run: {
					...basePayload.workflow_run,
					conclusion: null,
				},
			},
		});

		expect(event?.workflowRun.conclusion).toBeNull();
	});
});
