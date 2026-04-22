import { Hono } from "hono";
import type { RunService, RunStatus } from "../services/runs.js";

interface RunsRoutesDeps {
	runService: RunService;
}

export function createRunsRoutes(deps: RunsRoutesDeps) {
	const routes = new Hono();

	routes.get("/runs", async (c) => {
		const automation = c.req.query("automation");
		const status = c.req.query("status") as RunStatus | undefined;
		const limit = c.req.query("limit");
		const offset = c.req.query("offset");

		const runs = await deps.runService.list({
			automation: automation ?? undefined,
			status: status ?? undefined,
			limit: limit ? Number(limit) : undefined,
			offset: offset ? Number(offset) : undefined,
		});

		return c.json({ runs });
	});

	routes.get("/runs/:id", async (c) => {
		const id = c.req.param("id");
		const run = await deps.runService.getById(id);

		if (!run) {
			return c.json({ error: "Run not found" }, 404);
		}

		return c.json({ run });
	});

	return routes;
}
