import type { LinearClient } from "../connectors/linear/client.js";
import type { WorkflowRunResult } from "../github/client.js";
import { logger } from "../middleware/logger.js";
import type { RunService } from "./runs.js";

export class NotificationService {
	constructor(
		private runService: RunService,
		private linearClient: LinearClient | null,
	) {}

	/**
	 * Deliver all pending notifications for a completed/failed run.
	 */
	async deliverNotifications(runId: string): Promise<void> {
		const run = await this.runService.getById(runId);
		if (!run) return;

		const pending = await this.runService.getPendingNotifications(runId);

		for (const notification of pending) {
			try {
				await this.deliverOne(notification, run);
				await this.runService.updateNotificationStatus(notification.id, "sent");
				logger.info("notification delivered", {
					notification_id: notification.id,
					connector: notification.connector,
					run_id: runId,
				});
			} catch (err) {
				const message = err instanceof Error ? err.message : "Unknown error";
				await this.runService.updateNotificationStatus(
					notification.id,
					"failed",
					message,
				);
				logger.warn("notification delivery failed", {
					notification_id: notification.id,
					connector: notification.connector,
					run_id: runId,
					error: message,
				});
			}
		}
	}

	private async deliverOne(
		notification: {
			connector: string;
			targetId: string;
			config: unknown;
		},
		run: {
			status: string;
			result: unknown;
			triggerType: string;
			triggerId: string;
			automation: string;
		},
	): Promise<void> {
		switch (notification.connector) {
			case "linear":
				await this.deliverLinearNotification(notification, run);
				break;
			default:
				throw new Error(
					`Unsupported notification connector: ${notification.connector}`,
				);
		}
	}

	private async deliverLinearNotification(
		notification: { targetId: string; config: unknown },
		run: { status: string; result: unknown },
	): Promise<void> {
		if (!this.linearClient) {
			throw new Error("Linear client not configured");
		}

		const config = notification.config as Record<string, unknown>;
		const result = run.result as WorkflowRunResult | null;

		// Post a comment with the results
		if (config.comment) {
			const body = this.formatLinearComment(run.status, result);
			await this.linearClient.postComment(notification.targetId, body);
		}

		// Update issue status based on the run outcome
		if (config.update_status && config.status_mapping) {
			const mapping = config.status_mapping as Record<string, string>;

			// Check for PR-created mapping
			if (
				mapping.pr_created &&
				result?.pullRequests &&
				result.pullRequests.length > 0
			) {
				await this.linearClient.updateIssueState(
					notification.targetId,
					mapping.pr_created,
				);
			} else if (run.status === "completed" && mapping.completed) {
				await this.linearClient.updateIssueState(
					notification.targetId,
					mapping.completed,
				);
			} else if (run.status === "failed" && mapping.failed) {
				await this.linearClient.updateIssueState(
					notification.targetId,
					mapping.failed,
				);
			}
		}
	}

	private formatLinearComment(
		status: string,
		result: WorkflowRunResult | null,
	): string {
		const icon = status === "completed" ? "✅" : "❌";
		const parts: string[] = [];

		parts.push(`${icon} **Sidekick run ${status}**`);

		if (result?.htmlUrl) {
			parts.push(`\n[View GitHub Actions run](${result.htmlUrl})`);
		}

		if (result?.pullRequests && result.pullRequests.length > 0) {
			parts.push("\n**Pull Requests:**");
			for (const pr of result.pullRequests) {
				parts.push(`- [#${pr.number} ${pr.title}](${pr.url})`);
			}
		}

		if (status === "failed" && result?.conclusion) {
			parts.push(`\n**Conclusion:** ${result.conclusion}`);
		}

		return parts.join("\n");
	}
}
