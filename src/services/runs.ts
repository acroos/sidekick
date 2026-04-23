import { and, desc, eq, gt } from "drizzle-orm";
import type { Notification } from "../config/schema.js";
import type { Database } from "../db/client.js";
import { runNotifications, runs } from "../db/schema.js";

export type RunStatus =
	| "triggered"
	| "queued"
	| "running"
	| "completed"
	| "failed";

/** Valid status transitions for the run state machine */
const validTransitions: Record<RunStatus, RunStatus[]> = {
	triggered: ["queued", "running", "completed", "failed"],
	queued: ["running", "completed", "failed"],
	running: ["completed", "failed"],
	completed: [],
	failed: [],
};

export function isValidTransition(from: RunStatus, to: RunStatus): boolean {
	return validTransitions[from]?.includes(to) ?? false;
}

export class RunService {
	constructor(private db: Database) {}

	/**
	 * Check if a run was already created for the same automation + trigger
	 * within a recent time window. Prevents duplicates from webhook retries
	 * or multiple event shapes for the same user action.
	 */
	async findRecentRun(
		automation: string,
		triggerId: string,
		windowMs = 5 * 60 * 1000,
	) {
		const cutoff = new Date(Date.now() - windowMs);
		return (
			(await this.db.query.runs.findFirst({
				where: and(
					eq(runs.automation, automation),
					eq(runs.triggerId, triggerId),
					gt(runs.createdAt, cutoff),
				),
			})) ?? null
		);
	}

	/**
	 * Create a new run and its associated notification records.
	 */
	async createRun(params: {
		automation: string;
		triggerType: string;
		triggerId: string;
		triggerUrl?: string;
		repo: string;
		context?: unknown;
		notifications?: Array<{
			connector: string;
			targetId: string;
			targetUrl?: string;
			config: Notification;
		}>;
	}) {
		const [run] = await this.db
			.insert(runs)
			.values({
				automation: params.automation,
				triggerType: params.triggerType,
				triggerId: params.triggerId,
				triggerUrl: params.triggerUrl,
				repo: params.repo,
				context: params.context,
				status: "triggered",
			})
			.returning();

		if (params.notifications?.length) {
			await this.db.insert(runNotifications).values(
				params.notifications.map((n) => ({
					runId: run.id,
					connector: n.connector,
					targetId: n.targetId,
					targetUrl: n.targetUrl,
					config: n.config,
				})),
			);
		}

		return run;
	}

	/**
	 * Update a run's status, enforcing the state machine transitions.
	 * Returns the updated run, or null if the transition is invalid.
	 */
	async updateStatus(
		runId: string,
		newStatus: RunStatus,
		updates?: {
			githubRunId?: string;
			result?: unknown;
		},
	) {
		const existing = await this.db.query.runs.findFirst({
			where: eq(runs.id, runId),
		});

		if (!existing) {
			return null;
		}

		const currentStatus = existing.status as RunStatus;
		if (!isValidTransition(currentStatus, newStatus)) {
			return null;
		}

		const isTerminal = newStatus === "completed" || newStatus === "failed";

		const [updated] = await this.db
			.update(runs)
			.set({
				status: newStatus,
				githubRunId: updates?.githubRunId ?? existing.githubRunId,
				result: updates?.result ?? existing.result,
				updatedAt: new Date(),
				completedAt: isTerminal ? new Date() : existing.completedAt,
			})
			.where(eq(runs.id, runId))
			.returning();

		return updated;
	}

	/**
	 * Link a GitHub Actions run ID to a sidekick run.
	 */
	async setGitHubRunId(runId: string, githubRunId: string) {
		const [updated] = await this.db
			.update(runs)
			.set({
				githubRunId,
				updatedAt: new Date(),
			})
			.where(eq(runs.id, runId))
			.returning();

		return updated ?? null;
	}

	/**
	 * Find a run by its GitHub Actions run ID.
	 */
	async findByGitHubRunId(githubRunId: string) {
		return (
			this.db.query.runs.findFirst({
				where: eq(runs.githubRunId, githubRunId),
			}) ?? null
		);
	}

	/**
	 * Get a run by ID, including its notification records.
	 */
	async getById(runId: string) {
		const run = await this.db.query.runs.findFirst({
			where: eq(runs.id, runId),
		});

		if (!run) {
			return null;
		}

		const notifications = await this.db.query.runNotifications.findMany({
			where: eq(runNotifications.runId, runId),
		});

		return { ...run, notifications };
	}

	/**
	 * List runs with optional filters, ordered by creation time descending.
	 */
	async list(params?: {
		automation?: string;
		status?: RunStatus;
		limit?: number;
		offset?: number;
	}) {
		const conditions = [];

		if (params?.automation) {
			conditions.push(eq(runs.automation, params.automation));
		}
		if (params?.status) {
			conditions.push(eq(runs.status, params.status));
		}

		const where = conditions.length > 0 ? and(...conditions) : undefined;

		return this.db.query.runs.findMany({
			where,
			orderBy: desc(runs.createdAt),
			limit: params?.limit ?? 50,
			offset: params?.offset ?? 0,
		});
	}

	/**
	 * Get pending notifications for a run.
	 */
	async getPendingNotifications(runId: string) {
		return this.db.query.runNotifications.findMany({
			where: and(
				eq(runNotifications.runId, runId),
				eq(runNotifications.status, "pending"),
			),
		});
	}

	/**
	 * Get failed notifications that are eligible for retry.
	 */
	async getRetryableNotifications() {
		return this.db.query.runNotifications.findMany({
			where: eq(runNotifications.status, "failed"),
		});
	}

	/**
	 * Mark a notification as sent or failed. On failure, increment retry count
	 * and reset to pending if retries remain.
	 */
	async updateNotificationStatus(
		notificationId: string,
		status: "sent" | "failed",
		error?: string,
	) {
		if (status === "failed") {
			// Fetch current state to check retry eligibility
			const current = await this.db.query.runNotifications.findFirst({
				where: eq(runNotifications.id, notificationId),
			});

			if (current) {
				const newRetryCount = current.retryCount + 1;
				const hasRetriesLeft = newRetryCount < current.maxRetries;

				const [updated] = await this.db
					.update(runNotifications)
					.set({
						status: hasRetriesLeft ? "pending" : "failed",
						error: error ?? null,
						retryCount: newRetryCount,
					})
					.where(eq(runNotifications.id, notificationId))
					.returning();

				return updated ?? null;
			}
		}

		const [updated] = await this.db
			.update(runNotifications)
			.set({
				status,
				error: error ?? null,
				notifiedAt: status === "sent" ? new Date() : null,
			})
			.where(eq(runNotifications.id, notificationId))
			.returning();

		return updated ?? null;
	}
}
