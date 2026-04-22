import { Hono } from "hono";
import {
	type LinearWebhookPayload,
	matchesTrigger,
	parseLabelEvent,
	verifyLinearSignature,
} from "../connectors/linear/webhook.js";
import type { AutomationService } from "../services/automations.js";

interface LinearRoutesDeps {
	automationService: AutomationService;
	webhookSecret: string;
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
		const labelEvent = parseLabelEvent(payload);

		if (!labelEvent) {
			// Not a label event we care about
			return c.json({ ok: true, ignored: true });
		}

		// Find automations that match this label trigger
		const automations = deps.automationService.findLinearLabelAutomations(
			labelEvent.labelName,
		);

		if (automations.length === 0) {
			return c.json({ ok: true, ignored: true });
		}

		// Only process label additions that match a trigger
		const matchingAutomations = automations.filter(() =>
			matchesTrigger(labelEvent, labelEvent.labelName),
		);

		if (matchingAutomations.length === 0) {
			return c.json({ ok: true, ignored: true });
		}

		// Execute each matching automation
		const runIds: string[] = [];
		for (const automation of matchingAutomations) {
			const runId = await deps.automationService.executeLinearTrigger({
				automation,
				issueId: labelEvent.issueId,
				issueUrl: payload.url,
			});
			runIds.push(runId);
		}

		return c.json({ ok: true, runs: runIds });
	});

	return routes;
}
