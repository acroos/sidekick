import { createHmac, timingSafeEqual } from "node:crypto";

export interface LinearWebhookPayload {
	action: "create" | "update" | "remove";
	type: string;
	data: Record<string, unknown>;
	url: string;
	createdAt: string;
	updatedFrom?: Record<string, unknown>;
}

export interface LinearLabelEvent {
	action: "create" | "remove";
	issueId: string;
	labelName: string;
	labelId: string;
}

/**
 * Event fired when a label is applied to an issue.
 * Linear sends this as an IssueLabel "update" with the label definition data
 * (no issueId — requires a follow-up API call to find the issue).
 */
export interface LinearLabelAppliedEvent {
	labelName: string;
	labelId: string;
}

/**
 * Verify a Linear webhook signature using HMAC-SHA256.
 * Linear sends the signature in the `Linear-Signature` header as a hex digest.
 */
export function verifyLinearSignature(params: {
	payload: string;
	signature: string;
	secret: string;
}): boolean {
	const { payload, signature, secret } = params;

	if (!signature) {
		return false;
	}

	const expected = createHmac("sha256", secret).update(payload).digest("hex");

	if (expected.length !== signature.length) {
		return false;
	}

	return timingSafeEqual(Buffer.from(expected), Buffer.from(signature));
}

/**
 * Parse a Linear webhook payload to extract a label event.
 * Returns null if the event is not a label change we care about.
 */
export function parseLabelEvent(
	payload: LinearWebhookPayload,
): LinearLabelEvent | null {
	// Label events come as type "IssueLabel" with create/remove actions
	if (payload.type !== "IssueLabel") {
		return null;
	}

	if (payload.action !== "create" && payload.action !== "remove") {
		return null;
	}

	const data = payload.data;
	const issueId = data.issueId as string | undefined;
	const labelId = data.labelId as string | undefined;
	const label = data.label as Record<string, unknown> | undefined;
	const labelName = (label?.name as string) ?? "";

	if (!issueId || !labelId) {
		return null;
	}

	return {
		action: payload.action,
		issueId,
		labelName,
		labelId,
	};
}

/**
 * Parse an IssueLabel "update" event, which fires when a label is applied to an issue.
 * Linear sends the label definition data (name, color, etc.) but no issueId.
 * Returns null if the event is not a label application.
 */
export function parseLabelAppliedEvent(
	payload: LinearWebhookPayload,
): LinearLabelAppliedEvent | null {
	if (payload.type !== "IssueLabel" || payload.action !== "update") {
		return null;
	}

	const data = payload.data;
	const labelName = data.name as string | undefined;
	const labelId = data.id as string | undefined;

	if (!labelName || !labelId) {
		return null;
	}

	// Only trigger when lastAppliedAt changed (label was applied, not just renamed/recolored)
	if (payload.updatedFrom && !("lastAppliedAt" in payload.updatedFrom)) {
		return null;
	}

	return { labelName, labelId };
}

/**
 * Check if a label event matches a trigger condition.
 */
export function matchesTrigger(
	event: LinearLabelEvent,
	triggerLabel: string,
): boolean {
	return (
		event.action === "create" &&
		event.labelName.toLowerCase() === triggerLabel.toLowerCase()
	);
}
