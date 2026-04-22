import { Hono } from "hono";

export const healthRoutes = new Hono();

healthRoutes.get("/health", (c) => {
	return c.json({
		status: "ok",
		version: process.env.npm_package_version ?? "unknown",
	});
});
