import { Hono } from "hono";
import { describe, expect, it } from "vitest";
import { errorHandler } from "../error-handler.js";

describe("errorHandler", () => {
	it("returns 500 with generic message for unhandled errors", async () => {
		const app = new Hono();
		app.onError(errorHandler);
		app.get("/fail", () => {
			throw new Error("something broke");
		});

		const res = await app.request("/fail");
		expect(res.status).toBe(500);

		const body = (await res.json()) as { error: string };
		expect(body.error).toBe("Internal server error");
	});

	it("preserves status code from HTTPException-like errors", async () => {
		const app = new Hono();
		app.onError(errorHandler);
		app.get("/bad", () => {
			const err = new Error("bad request") as Error & { status: number };
			err.status = 400;
			throw err;
		});

		const res = await app.request("/bad");
		expect(res.status).toBe(400);

		const body = (await res.json()) as { error: string };
		expect(body.error).toBe("bad request");
	});
});
