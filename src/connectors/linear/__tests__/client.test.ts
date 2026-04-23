import { describe, expect, it } from "vitest";
import { filterSidekickComments } from "../client.js";

describe("filterSidekickComments", () => {
	it("keeps normal user comments", () => {
		const comments = [
			"This is broken, please fix it",
			"I think the issue is in the tooltip component",
		];
		expect(filterSidekickComments(comments)).toEqual(comments);
	});

	it("filters completed run comments", () => {
		const comments = [
			"Please fix this",
			"✅ **Sidekick run completed**\n\n[View GitHub Actions run](https://github.com/org/repo/actions/runs/123)\n\n**Pull Requests:**\n- [#42 Fix the thing](https://github.com/org/repo/pull/42)",
		];
		expect(filterSidekickComments(comments)).toEqual(["Please fix this"]);
	});

	it("filters failed run comments", () => {
		const comments = [
			"❌ **Sidekick run failed**\n\n**Conclusion:** cancelled",
			"Any updates on this?",
		];
		expect(filterSidekickComments(comments)).toEqual(["Any updates on this?"]);
	});

	it("keeps comments that mention Sidekick but don't match the prefix", () => {
		const comments = [
			"The Sidekick run took too long, can we optimize?",
			"Sidekick run results look good",
		];
		expect(filterSidekickComments(comments)).toEqual(comments);
	});

	it("returns empty array when all comments are Sidekick status updates", () => {
		const comments = [
			"✅ **Sidekick run completed**\n\nSome details",
			"❌ **Sidekick run failed**\n\nSome details",
		];
		expect(filterSidekickComments(comments)).toEqual([]);
	});

	it("returns empty array for empty input", () => {
		expect(filterSidekickComments([])).toEqual([]);
	});
});
