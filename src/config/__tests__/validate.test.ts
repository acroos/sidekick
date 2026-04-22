import { describe, expect, it } from "vitest";
import type { Config } from "../schema.js";
import { assertConfigValid, validateConfig } from "../validate.js";

const validConfig: Config = {
	github: {
		token: "ghp_test",
		default_repo: "org/repo",
		workflow: "ci.yml",
	},
	connectors: {
		linear: { api_key: "lin_test" },
	},
	automations: [
		{
			name: "test-automation",
			trigger: { connector: "linear", on_label: "sidekick" },
			notifications: [{ connector: "linear", comment: true }],
		},
	],
};

describe("validateConfig", () => {
	it("returns no errors for a valid config", () => {
		expect(validateConfig(validConfig)).toEqual([]);
	});

	it("detects trigger referencing undefined connector", () => {
		const config: Config = {
			...validConfig,
			connectors: {},
			automations: [
				{
					name: "test",
					trigger: { connector: "linear" },
					notifications: [],
				},
			],
		};

		const errors = validateConfig(config);
		expect(errors).toContainEqual(
			expect.objectContaining({
				field: "trigger.connector",
				message: expect.stringContaining("not defined in connectors"),
			}),
		);
	});

	it("detects notification referencing undefined connector", () => {
		const config: Config = {
			...validConfig,
			automations: [
				{
					name: "test",
					trigger: { connector: "linear" },
					notifications: [{ connector: "slack" }],
				},
			],
		};

		const errors = validateConfig(config);
		expect(errors).toContainEqual(
			expect.objectContaining({
				field: "notifications.connector",
				message: expect.stringContaining("not defined in connectors"),
			}),
		);
	});

	it("detects unsupported trigger connector type", () => {
		const config: Config = {
			...validConfig,
			connectors: { pagerduty: {} },
			automations: [
				{
					name: "test",
					trigger: { connector: "pagerduty" },
					notifications: [],
				},
			],
		};

		const errors = validateConfig(config);
		expect(errors).toContainEqual(
			expect.objectContaining({
				field: "trigger.connector",
				message: expect.stringContaining("not supported"),
			}),
		);
	});

	it("detects duplicate automation names", () => {
		const config: Config = {
			...validConfig,
			automations: [
				{
					name: "dupe",
					trigger: { connector: "linear" },
					notifications: [],
				},
				{
					name: "dupe",
					trigger: { connector: "linear" },
					notifications: [],
				},
			],
		};

		const errors = validateConfig(config);
		expect(errors).toContainEqual(
			expect.objectContaining({
				message: expect.stringContaining("Duplicate automation name"),
			}),
		);
	});

	it("returns empty array for config with no automations", () => {
		const config: Config = {
			...validConfig,
			automations: [],
		};

		expect(validateConfig(config)).toEqual([]);
	});
});

describe("assertConfigValid", () => {
	it("does not throw for valid config", () => {
		expect(() => assertConfigValid(validConfig)).not.toThrow();
	});

	it("throws with formatted error message for invalid config", () => {
		const config: Config = {
			...validConfig,
			connectors: {},
			automations: [
				{
					name: "bad",
					trigger: { connector: "linear" },
					notifications: [],
				},
			],
		};

		expect(() => assertConfigValid(config)).toThrow(
			"Configuration validation failed",
		);
	});
});
