import { handle } from "hono/vercel";
import { loadConfig } from "../src/config/index.js";
import { LinearClient } from "../src/connectors/linear/client.js";
import { createDb } from "../src/db/index.js";
import { GitHubClient } from "../src/github/index.js";
import { AutomationService, NotificationService, RunService } from "../src/services/index.js";
import { createApp } from "../src/app.js";

const config = loadConfig();
const db = createDb();

const githubClient = new GitHubClient(config.github.token);
const runService = new RunService(db);

const linearApiKey = config.connectors.linear?.api_key;
const linearClient = linearApiKey ? new LinearClient(linearApiKey) : null;

const automationService = new AutomationService(config, runService, githubClient, linearClient);
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

export default handle(app);
