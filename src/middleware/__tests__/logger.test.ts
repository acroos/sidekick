import { Hono } from "hono";
import { afterEach, describe, expect, it, vi } from "vitest";
import { logger, requestLogger } from "../logger.js";

describe("logger", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("outputs structured JSON to console.log for info", () => {
		const spy = vi.spyOn(console, "log").mockImplementation(() => {});
		logger.info("test message", { key: "value" });

		expect(spy).toHaveBeenCalledOnce();
		const output = JSON.parse(spy.mock.calls[0][0] as string);
		expect(output.level).toBe("info");
		expect(output.message).toBe("test message");
		expect(output.key).toBe("value");
		expect(output.timestamp).toBeDefined();
	});

	it("outputs to console.error for error level", () => {
		const spy = vi.spyOn(console, "error").mockImplementation(() => {});
		logger.error("bad thing");

		expect(spy).toHaveBeenCalledOnce();
		const output = JSON.parse(spy.mock.calls[0][0] as string);
		expect(output.level).toBe("error");
	});

	it("outputs to console.warn for warn level", () => {
		const spy = vi.spyOn(console, "warn").mockImplementation(() => {});
		logger.warn("watch out");

		expect(spy).toHaveBeenCalledOnce();
		const output = JSON.parse(spy.mock.calls[0][0] as string);
		expect(output.level).toBe("warn");
	});
});

describe("requestLogger middleware", () => {
	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("logs request method, path, status, and duration", async () => {
		const spy = vi.spyOn(console, "log").mockImplementation(() => {});

		const app = new Hono();
		app.use("*", requestLogger());
		app.get("/test", (c) => c.json({ ok: true }));

		await app.request("/test");

		expect(spy).toHaveBeenCalledOnce();
		const output = JSON.parse(spy.mock.calls[0][0] as string);
		expect(output.method).toBe("GET");
		expect(output.path).toBe("/test");
		expect(output.status).toBe(200);
		expect(output.duration_ms).toBeGreaterThanOrEqual(0);
	});
});
