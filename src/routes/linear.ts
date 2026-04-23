import { Hono } from "hono";
import type { LinearClient } from "../connectors/linear/client.js";
import {
	type LinearWebhookPayload,
	matchesTrigger,
	parseLabelAppliedEvent,
	parseLabelEvent,
	verifyLinearSignature,
} from "../connectors/linear/webhook.js";
import { logger } from "../middleware/logger.js";
import type { AutomationService } from "../services/automations.js";

interface LinearRoutesDeps {
	automationService: AutomationService;
	webhookSecret: string;
	linearClient: LinearClient | null;
}

export function createLinearRoutes(deps: LinearRoutesDeps) {
	const routes = new Hono();

	routes.post("/webhooks/linear", async (c) => {
		const body = await c.req.text();
		const signature = c.req.header("linear-signature") ?? "";

		if (
			!verifyLinearSignature({
				payload: body,
				signature,
				secret: deps.webhookSecret,
			})
		) {
			return c.json({ error: "Invalid signature" }, 401);
		}

		const payload = JSON.parse(body) as LinearWebhookPayload;

		logger.info("linear webhook: received", {
			type: payload.type,
			action: payload.action,
		});

		// Resolve the label name and issue from the webhook payload.
		// Two event shapes exist:
		//   1. IssueLabel create/remove — contains issueId directly
		//   2. IssueLabel update — label definition only, requires API lookup for the issue
		let labelName: string;
		let issueId: string;
		let issueUrl: string;

		const labelEvent = parseLabelEvent(payload);
		if (labelEvent) {
			if (!matchesTrigger(labelEvent, labelEvent.labelName)) {
				logger.info("linear webhook: ignored (label removed, not added)", {
					action: labelEvent.action,
					label_name: labelEvent.labelName,
				});
				return c.json({ ok: true, ignored: true });
			}
			labelName = labelEvent.labelName;
			issueId = labelEvent.issueId;
			issueUrl = payload.url;
		} else {
			const appliedEvent = parseLabelAppliedEvent(payload);
			if (!appliedEvent) {
				logger.info("linear webhook: ignored", {
					type: payload.type,
					action: payload.action,
				});
				return c.json({ ok: true, ignored: true });
			}

			// Check for matching automations before making API calls
			const earlyAutomations =
				deps.automationService.findLinearLabelAutomations(
					appliedEvent.labelName,
				);
			if (earlyAutomations.length === 0) {
				logger.info("linear webhook: ignored (no matching automations)", {
					label_name: appliedEvent.labelName,
				});
				return c.json({ ok: true, ignored: true });
			}

			if (!deps.linearClient) {
				logger.error(
					"linear webhook: no Linear client configured to resolve issue",
				);
				return c.json({ error: "Linear client not configured" }, 500);
			}

			const issue = await deps.linearClient.findRecentlyLabeledIssue(
				appliedEvent.labelId,
			);
			if (!issue) {
				logger.info("linear webhook: no issue found with label", {
					label_name: appliedEvent.labelName,
					label_id: appliedEvent.labelId,
				});
				return c.json({ ok: true, ignored: true });
			}

			labelName = appliedEvent.labelName;
			issueId = issue.id;
			issueUrl = issue.url;
		}

		// Find automations that match this label trigger
		const automations =
			deps.automationService.findLinearLabelAutomations(labelName);

		if (automations.length === 0) {
			logger.info("linear webhook: ignored (no matching automations)", {
				label_name: labelName,
			});
			return c.json({ ok: true, ignored: true });
		}

		logger.info("linear webhook: trigger matched", {
			label: labelName,
			issue_id: issueId,
			automations: automations.map((a) => a.name),
		});

		// Execute each matching automation
		const runIds: string[] = [];
		for (const automation of automations) {
			const runId = await deps.automationService.executeLinearTrigger({
				automation,
				issueId,
				issueUrl,
			});
			runIds.push(runId);
		}

		return c.json({ ok: true, runs: runIds });
	});

	return routes;
}
