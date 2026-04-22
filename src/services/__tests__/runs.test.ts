import { describe, expect, it } from "vitest";
import { type RunStatus, isValidTransition } from "../runs.js";

describe("isValidTransition", () => {
	it("allows triggered → queued", () => {
		expect(isValidTransition("triggered", "queued")).toBe(true);
	});

	it("allows triggered → running", () => {
		expect(isValidTransition("triggered", "running")).toBe(true);
	});

	it("allows triggered → completed", () => {
		expect(isValidTransition("triggered", "completed")).toBe(true);
	});

	it("allows triggered → failed", () => {
		expect(isValidTransition("triggered", "failed")).toBe(true);
	});

	it("allows queued → running", () => {
		expect(isValidTransition("queued", "running")).toBe(true);
	});

	it("allows running → completed", () => {
		expect(isValidTransition("running", "completed")).toBe(true);
	});

	it("allows running → failed", () => {
		expect(isValidTransition("running", "failed")).toBe(true);
	});

	it("rejects completed → running (terminal state)", () => {
		expect(isValidTransition("completed", "running")).toBe(false);
	});

	it("rejects failed → running (terminal state)", () => {
		expect(isValidTransition("failed", "running")).toBe(false);
	});

	it("rejects running → queued (backward transition)", () => {
		expect(isValidTransition("running", "queued")).toBe(false);
	});

	it("rejects same-state transition", () => {
		expect(isValidTransition("running", "running")).toBe(false);
	});

	const allStatuses: RunStatus[] = [
		"triggered",
		"queued",
		"running",
		"completed",
		"failed",
	];

	it("allows no transitions from completed", () => {
		for (const to of allStatuses) {
			expect(isValidTransition("completed", to)).toBe(false);
		}
	});

	it("allows no transitions from failed", () => {
		for (const to of allStatuses) {
			expect(isValidTransition("failed", to)).toBe(false);
		}
	});
});
