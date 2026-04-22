import type { MiddlewareHandler } from "hono";

export interface LogEntry {
	timestamp: string;
	level: "info" | "warn" | "error";
	message: string;
	[key: string]: unknown;
}

function formatLog(entry: LogEntry): string {
	return JSON.stringify(entry);
}

/**
 * Structured JSON logger. Writes to stdout for Vercel compatibility.
 */
export const logger = {
	info(message: string, data?: Record<string, unknown>) {
		console.log(
			formatLog({
				timestamp: new Date().toISOString(),
				level: "info",
				message,
				...data,
			}),
		);
	},

	warn(message: string, data?: Record<string, unknown>) {
		console.warn(
			formatLog({
				timestamp: new Date().toISOString(),
				level: "warn",
				message,
				...data,
			}),
		);
	},

	error(message: string, data?: Record<string, unknown>) {
		console.error(
			formatLog({
				timestamp: new Date().toISOString(),
				level: "error",
				message,
				...data,
			}),
		);
	},
};

/**
 * Hono middleware that logs each request with method, path, status, and duration.
 */
export function requestLogger(): MiddlewareHandler {
	return async (c, next) => {
		const start = Date.now();
		await next();
		const duration = Date.now() - start;

		logger.info("request", {
			method: c.req.method,
			path: c.req.path,
			status: c.res.status,
			duration_ms: duration,
		});
	};
}
