import { describe, expect, it } from "vitest";
import app from "../../app.js";

describe("GET /api/health", () => {
	it("returns status ok", async () => {
		const res = await app.request("/api/health");
		expect(res.status).toBe(200);

		const body = (await res.json()) as { status: string };
		expect(body.status).toBe("ok");
	});

	it("includes version field", async () => {
		const res = await app.request("/api/health");
		const body = (await res.json()) as Record<string, unknown>;
		expect(body).toHaveProperty("version");
	});
});
