import { Hono } from "hono";
import type { GitHubClient } from "../github/client.js";
import {
	parseWorkflowRunEvent,
	verifyWebhookSignature,
} from "../github/webhook.js";
import { logger } from "../middleware/logger.js";
import type { NotificationService } from "../services/notifications.js";
import type { RunService } from "../services/runs.js";

interface GitHubRoutesDeps {
	runService: RunService;
	githubClient: GitHubClient;
	webhookSecret: string;
	notificationService?: NotificationService;
}

export function createGitHubRoutes(deps: GitHubRoutesDeps) {
	const routes = new Hono();

	routes.post("/webhooks/github", async (c) => {
		const body = await c.req.text();
		const signature = c.req.header("x-hub-signature-256") ?? "";
		const eventType = c.req.header("x-github-event") ?? "";

		if (
			!verifyWebhookSignature({
				payload: body,
				signature,
				secret: deps.webhookSecret,
			})
		) {
			return c.json({ error: "Invalid signature" }, 401);
		}

		const payload = JSON.parse(body);
		const event = parseWorkflowRunEvent({ eventType, payload });

		if (!event) {
			// Not a workflow_run event we care about — acknowledge and move on
			return c.json({ ok: true, ignored: true });
		}

		const githubRunId = String(event.workflowRun.id);
		const run = await deps.runService.findByGitHubRunId(githubRunId);

		if (!run) {
			// This workflow run wasn't dispatched by us
			return c.json({ ok: true, ignored: true });
		}

		logger.info("github webhook: processing workflow_run", {
			action: event.action,
			github_run_id: event.workflowRun.id,
			run_id: run.id,
		});

		switch (event.action) {
			case "queued":
				await deps.runService.updateStatus(run.id, "queued");
				break;

			case "in_progress":
				await deps.runService.updateStatus(run.id, "running");
				break;

			case "completed": {
				const conclusion = event.workflowRun.conclusion;
				const newStatus = conclusion === "success" ? "completed" : "failed";

				// Fetch full results from the completed run
				const result = await deps.githubClient.getWorkflowRun({
					repo: event.repo,
					runId: event.workflowRun.id,
				});

				await deps.runService.updateStatus(run.id, newStatus, {
					result,
				});

				// Deliver notifications for the completed run
				if (deps.notificationService) {
					await deps.notificationService.deliverNotifications(run.id);
				}
				break;
			}
		}

		return c.json({ ok: true, runId: run.id, action: event.action });
	});

	return routes;
}
