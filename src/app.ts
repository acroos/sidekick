import { Hono } from "hono";
import type { GitHubClient } from "./github/client.js";
import { createGitHubRoutes } from "./routes/github.js";
import { healthRoutes } from "./routes/health.js";
import { createRunsRoutes } from "./routes/runs.js";
import type { RunService } from "./services/runs.js";

export interface AppDeps {
	runService: RunService;
	githubClient: GitHubClient;
	githubWebhookSecret: string;
}

/**
 * Create the Hono app with full dependency injection.
 * Used when database and services are available.
 */
export function createApp(deps: AppDeps) {
	const app = new Hono();

	app.route("/api", healthRoutes);
	app.route(
		"/api",
		createGitHubRoutes({
			runService: deps.runService,
			githubClient: deps.githubClient,
			webhookSecret: deps.githubWebhookSecret,
		}),
	);
	app.route(
		"/api",
		createRunsRoutes({
			runService: deps.runService,
		}),
	);

	return app;
}

/**
 * Minimal app with just the health endpoint.
 * Used for testing and when services aren't needed.
 */
const app = new Hono();
app.route("/api", healthRoutes);
export default app;
