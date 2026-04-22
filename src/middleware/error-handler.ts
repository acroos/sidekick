import type { ErrorHandler } from "hono";
import { logger } from "./logger.js";

/**
 * Global error handler that catches unhandled errors and returns
 * structured JSON responses. Logs the error for observability.
 */
export const errorHandler: ErrorHandler = (err, c) => {
	const status =
		"status" in err && typeof err.status === "number" ? err.status : 500;

	logger.error("unhandled error", {
		error: err.message,
		path: c.req.path,
		method: c.req.method,
		stack: process.env.NODE_ENV !== "production" ? err.stack : undefined,
	});

	return c.json(
		{
			error: status >= 500 ? "Internal server error" : err.message,
		},
		status as 500,
	);
};
