import { serve } from "@hono/node-server";
import { createApp } from "./app.js";
import { loadConfig } from "./config/index.js";
import { LinearClient } from "./connectors/linear/client.js";
import { createDb } from "./db/index.js";
import { runMigrations } from "./db/migrate.js";
import { GitHubClient } from "./github/index.js";
import { logger } from "./middleware/logger.js";
import {
	AutomationService,
	NotificationService,
	RunService,
} from "./services/index.js";

async function main() {
	const port = Number(process.env.PORT) || 3000;

	await runMigrations();

	const config = loadConfig();
	const db = createDb();

	const githubClient = new GitHubClient(config.github.token);
	const runService = new RunService(db);

	const linearApiKey = config.connectors.linear?.api_key;
	const linearClient = linearApiKey ? new LinearClient(linearApiKey) : null;

	const automationService = new AutomationService(
		config,
		runService,
		githubClient,
		linearClient,
	);
	const notificationService = new NotificationService(runService, linearClient);

	const linearWebhookSecret = config.connectors.linear?.webhook_secret ?? "";
	const githubWebhookSecret = config.connectors.github?.webhook_secret ?? "";

	const app = createApp({
		runService,
		githubClient,
		githubWebhookSecret,
		automationService,
		notificationService,
		linearWebhookSecret,
		linearClient,
	});

	serve({ fetch: app.fetch, port }, (info) => {
		logger.info(`Sidekick listening on http://localhost:${info.port}`);
	});
}

main().catch((err) => {
	logger.error("Failed to start Sidekick", err);
	process.exit(1);
});
