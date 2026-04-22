import { createHmac, timingSafeEqual } from "node:crypto";

export interface WorkflowRunEvent {
	action: "requested" | "queued" | "in_progress" | "completed";
	workflowRun: {
		id: number;
		name: string;
		status: string;
		conclusion: string | null;
		htmlUrl: string;
		headBranch: string | null;
		headSha: string;
		event: string;
	};
	repo: string;
}

/**
 * Verify that a webhook payload was sent by GitHub using HMAC-SHA256
 * signature verification.
 */
export function verifyWebhookSignature(params: {
	payload: string;
	signature: string;
	secret: string;
}): boolean {
	const { payload, signature, secret } = params;

	if (!signature.startsWith("sha256=")) {
		return false;
	}

	const expected = `sha256=${createHmac("sha256", secret).update(payload).digest("hex")}`;

	if (expected.length !== signature.length) {
		return false;
	}

	return timingSafeEqual(Buffer.from(expected), Buffer.from(signature));
}

/**
 * Parse a GitHub webhook payload into a typed WorkflowRunEvent.
 * Returns null if the event is not a workflow_run event we care about.
 */
export function parseWorkflowRunEvent(params: {
	eventType: string;
	payload: Record<string, unknown>;
}): WorkflowRunEvent | null {
	if (params.eventType !== "workflow_run") {
		return null;
	}

	const { payload } = params;
	const action = payload.action as string;

	if (!["requested", "queued", "in_progress", "completed"].includes(action)) {
		return null;
	}

	const run = payload.workflow_run as Record<string, unknown>;
	const repo = payload.repository as Record<string, unknown>;

	if (!run || !repo) {
		return null;
	}

	return {
		action: action as WorkflowRunEvent["action"],
		workflowRun: {
			id: run.id as number,
			name: run.name as string,
			status: run.status as string,
			conclusion: (run.conclusion as string) ?? null,
			htmlUrl: run.html_url as string,
			headBranch: (run.head_branch as string) ?? null,
			headSha: run.head_sha as string,
			event: run.event as string,
		},
		repo: repo.full_name as string,
	};
}
