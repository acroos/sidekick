import { describe, expect, it, vi } from "vitest";
import { NotificationService } from "../notifications.js";

function makeDeps() {
	const mockRunService = {
		getById: vi.fn(),
		getPendingNotifications: vi.fn().mockResolvedValue([]),
		updateNotificationStatus: vi.fn(),
	};

	const mockLinearClient = {
		postComment: vi.fn(),
		updateIssueState: vi.fn(),
	};

	const service = new NotificationService(
		// biome-ignore lint/suspicious/noExplicitAny: test mocks
		mockRunService as any,
		// biome-ignore lint/suspicious/noExplicitAny: test mocks
		mockLinearClient as any,
	);

	return { service, mockRunService, mockLinearClient };
}

describe("NotificationService.deliverNotifications", () => {
	it("does nothing when run is not found", async () => {
		const { service, mockRunService } = makeDeps();
		mockRunService.getById.mockResolvedValue(null);

		await service.deliverNotifications("run-1");

		expect(mockRunService.getPendingNotifications).not.toHaveBeenCalled();
	});

	it("delivers a Linear comment notification", async () => {
		const { service, mockRunService, mockLinearClient } = makeDeps();

		mockRunService.getById.mockResolvedValue({
			id: "run-1",
			status: "completed",
			result: {
				htmlUrl: "https://github.com/org/repo/actions/runs/123",
				pullRequests: [
					{
						number: 42,
						title: "Fix the bug",
						url: "https://github.com/org/repo/pull/42",
					},
				],
			},
			triggerType: "linear",
			triggerId: "issue-123",
			automation: "linear-issues",
		});

		mockRunService.getPendingNotifications.mockResolvedValue([
			{
				id: "notif-1",
				connector: "linear",
				targetId: "issue-123",
				config: { comment: true },
			},
		]);

		await service.deliverNotifications("run-1");

		expect(mockLinearClient.postComment).toHaveBeenCalledWith(
			"issue-123",
			expect.stringContaining("Sidekick run completed"),
		);
		expect(mockLinearClient.postComment).toHaveBeenCalledWith(
			"issue-123",
			expect.stringContaining("#42 Fix the bug"),
		);
		expect(mockRunService.updateNotificationStatus).toHaveBeenCalledWith(
			"notif-1",
			"sent",
		);
	});

	it("updates Linear issue state on completion", async () => {
		const { service, mockRunService, mockLinearClient } = makeDeps();

		mockRunService.getById.mockResolvedValue({
			id: "run-1",
			status: "completed",
			result: { htmlUrl: "https://example.com", pullRequests: [] },
			triggerType: "linear",
			triggerId: "issue-123",
			automation: "linear-issues",
		});

		mockRunService.getPendingNotifications.mockResolvedValue([
			{
				id: "notif-1",
				connector: "linear",
				targetId: "issue-123",
				config: {
					update_status: true,
					status_mapping: { completed: "Done" },
				},
			},
		]);

		await service.deliverNotifications("run-1");

		expect(mockLinearClient.updateIssueState).toHaveBeenCalledWith(
			"issue-123",
			"Done",
		);
	});

	it("updates Linear issue state to PR-created mapping when PRs exist", async () => {
		const { service, mockRunService, mockLinearClient } = makeDeps();

		mockRunService.getById.mockResolvedValue({
			id: "run-1",
			status: "completed",
			result: {
				htmlUrl: "https://example.com",
				pullRequests: [
					{ number: 1, title: "PR", url: "https://example.com/pr/1" },
				],
			},
			triggerType: "linear",
			triggerId: "issue-123",
			automation: "linear-issues",
		});

		mockRunService.getPendingNotifications.mockResolvedValue([
			{
				id: "notif-1",
				connector: "linear",
				targetId: "issue-123",
				config: {
					update_status: true,
					status_mapping: {
						pr_created: "In Review",
						completed: "Done",
					},
				},
			},
		]);

		await service.deliverNotifications("run-1");

		expect(mockLinearClient.updateIssueState).toHaveBeenCalledWith(
			"issue-123",
			"In Review",
		);
	});

	it("marks notification as failed on error", async () => {
		const { service, mockRunService, mockLinearClient } = makeDeps();

		mockRunService.getById.mockResolvedValue({
			id: "run-1",
			status: "completed",
			result: null,
			triggerType: "linear",
			triggerId: "issue-123",
			automation: "linear-issues",
		});

		mockRunService.getPendingNotifications.mockResolvedValue([
			{
				id: "notif-1",
				connector: "linear",
				targetId: "issue-123",
				config: { comment: true },
			},
		]);

		mockLinearClient.postComment.mockRejectedValue(
			new Error("API rate limited"),
		);

		await service.deliverNotifications("run-1");

		expect(mockRunService.updateNotificationStatus).toHaveBeenCalledWith(
			"notif-1",
			"failed",
			"API rate limited",
		);
	});

	it("marks notification as failed for unsupported connector", async () => {
		const { service, mockRunService } = makeDeps();

		mockRunService.getById.mockResolvedValue({
			id: "run-1",
			status: "completed",
			result: null,
			triggerType: "linear",
			triggerId: "issue-123",
			automation: "linear-issues",
		});

		mockRunService.getPendingNotifications.mockResolvedValue([
			{
				id: "notif-1",
				connector: "slack",
				targetId: "#channel",
				config: {},
			},
		]);

		await service.deliverNotifications("run-1");

		expect(mockRunService.updateNotificationStatus).toHaveBeenCalledWith(
			"notif-1",
			"failed",
			"Unsupported notification connector: slack",
		);
	});
});
