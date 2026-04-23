import { createHmac } from "node:crypto";
import { describe, expect, it } from "vitest";
import {
	type LinearWebhookPayload,
	matchesTrigger,
	parseLabelAppliedEvent,
	parseLabelEvent,
	verifyLinearSignature,
} from "../webhook.js";

function sign(payload: string, secret: string): string {
	return createHmac("sha256", secret).update(payload).digest("hex");
}

describe("verifyLinearSignature", () => {
	const secret = "linear-test-secret";
	const payload = '{"action":"create"}';

	it("returns true for valid signature", () => {
		const signature = sign(payload, secret);
		expect(verifyLinearSignature({ payload, signature, secret })).toBe(true);
	});

	it("returns false for invalid signature", () => {
		expect(
			verifyLinearSignature({
				payload,
				signature: "invalid-hex",
				secret,
			}),
		).toBe(false);
	});

	it("returns false for empty signature", () => {
		expect(verifyLinearSignature({ payload, signature: "", secret })).toBe(
			false,
		);
	});
});

describe("parseLabelEvent", () => {
	it("parses an IssueLabel create event", () => {
		const payload: LinearWebhookPayload = {
			action: "create",
			type: "IssueLabel",
			data: {
				issueId: "issue-123",
				labelId: "label-456",
				label: { name: "sidekick" },
			},
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		};

		const event = parseLabelEvent(payload);

		expect(event).not.toBeNull();
		expect(event?.action).toBe("create");
		expect(event?.issueId).toBe("issue-123");
		expect(event?.labelName).toBe("sidekick");
	});

	it("parses an IssueLabel remove event", () => {
		const payload: LinearWebhookPayload = {
			action: "remove",
			type: "IssueLabel",
			data: {
				issueId: "issue-123",
				labelId: "label-456",
				label: { name: "sidekick" },
			},
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		};

		const event = parseLabelEvent(payload);
		expect(event?.action).toBe("remove");
	});

	it("returns null for non-IssueLabel events", () => {
		const payload: LinearWebhookPayload = {
			action: "create",
			type: "Issue",
			data: { id: "issue-123" },
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		};

		expect(parseLabelEvent(payload)).toBeNull();
	});

	it("returns null for update action on IssueLabel", () => {
		const payload: LinearWebhookPayload = {
			action: "update",
			type: "IssueLabel",
			data: {
				issueId: "issue-123",
				labelId: "label-456",
				label: { name: "sidekick" },
			},
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		};

		expect(parseLabelEvent(payload)).toBeNull();
	});

	it("returns null when issueId is missing", () => {
		const payload: LinearWebhookPayload = {
			action: "create",
			type: "IssueLabel",
			data: {
				labelId: "label-456",
				label: { name: "sidekick" },
			},
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		};

		expect(parseLabelEvent(payload)).toBeNull();
	});
});

describe("parseLabelAppliedEvent", () => {
	it("parses an IssueLabel update event with lastAppliedAt change", () => {
		const payload: LinearWebhookPayload = {
			action: "update",
			type: "IssueLabel",
			data: {
				id: "label-456",
				name: "sidekick",
				color: "#blue",
				isGroup: false,
				lastAppliedAt: "2026-04-22T01:00:00Z",
			},
			url: "https://linear.app/team/settings/labels",
			createdAt: "2026-04-22T01:00:00Z",
			updatedFrom: {
				lastAppliedAt: "2026-04-21T00:00:00Z",
			},
		};

		const event = parseLabelAppliedEvent(payload);
		expect(event).not.toBeNull();
		expect(event?.labelName).toBe("sidekick");
		expect(event?.labelId).toBe("label-456");
	});

	it("returns null for non-IssueLabel types", () => {
		const payload: LinearWebhookPayload = {
			action: "update",
			type: "Issue",
			data: { id: "issue-123", name: "some issue" },
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		};

		expect(parseLabelAppliedEvent(payload)).toBeNull();
	});

	it("returns null for create action", () => {
		const payload: LinearWebhookPayload = {
			action: "create",
			type: "IssueLabel",
			data: {
				id: "label-456",
				name: "sidekick",
				issueId: "issue-123",
				labelId: "label-456",
			},
			url: "https://linear.app/team/issue/ENG-123",
			createdAt: "2026-04-22T00:00:00Z",
		};

		expect(parseLabelAppliedEvent(payload)).toBeNull();
	});

	it("returns null when label name is missing", () => {
		const payload: LinearWebhookPayload = {
			action: "update",
			type: "IssueLabel",
			data: { id: "label-456", color: "#blue" },
			url: "https://linear.app/team/settings/labels",
			createdAt: "2026-04-22T00:00:00Z",
			updatedFrom: { lastAppliedAt: "2026-04-21T00:00:00Z" },
		};

		expect(parseLabelAppliedEvent(payload)).toBeNull();
	});

	it("returns null when updatedFrom has no lastAppliedAt (e.g. color change)", () => {
		const payload: LinearWebhookPayload = {
			action: "update",
			type: "IssueLabel",
			data: {
				id: "label-456",
				name: "sidekick",
				color: "#red",
			},
			url: "https://linear.app/team/settings/labels",
			createdAt: "2026-04-22T00:00:00Z",
			updatedFrom: { color: "#blue" },
		};

		expect(parseLabelAppliedEvent(payload)).toBeNull();
	});

	it("parses event when updatedFrom is absent", () => {
		const payload: LinearWebhookPayload = {
			action: "update",
			type: "IssueLabel",
			data: {
				id: "label-456",
				name: "sidekick",
				lastAppliedAt: "2026-04-22T01:00:00Z",
			},
			url: "https://linear.app/team/settings/labels",
			createdAt: "2026-04-22T01:00:00Z",
		};

		const event = parseLabelAppliedEvent(payload);
		expect(event).not.toBeNull();
		expect(event?.labelName).toBe("sidekick");
	});
});

describe("matchesTrigger", () => {
	it("matches label creation with correct name", () => {
		expect(
			matchesTrigger(
				{
					action: "create",
					issueId: "issue-123",
					labelName: "sidekick",
					labelId: "label-456",
				},
				"sidekick",
			),
		).toBe(true);
	});

	it("matches case-insensitively", () => {
		expect(
			matchesTrigger(
				{
					action: "create",
					issueId: "issue-123",
					labelName: "Sidekick",
					labelId: "label-456",
				},
				"sidekick",
			),
		).toBe(true);
	});

	it("does not match label removal", () => {
		expect(
			matchesTrigger(
				{
					action: "remove",
					issueId: "issue-123",
					labelName: "sidekick",
					labelId: "label-456",
				},
				"sidekick",
			),
		).toBe(false);
	});

	it("does not match different label name", () => {
		expect(
			matchesTrigger(
				{
					action: "create",
					issueId: "issue-123",
					labelName: "bug",
					labelId: "label-456",
				},
				"sidekick",
			),
		).toBe(false);
	});
});
