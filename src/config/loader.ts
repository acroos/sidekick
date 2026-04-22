import { readFileSync } from "node:fs";
import yaml from "js-yaml";
import { type Config, configSchema } from "./schema.js";

/**
 * Interpolate ${VAR} references in a string with values from process.env.
 * Throws if a referenced variable is not set.
 */
function interpolateEnvVars(value: string): string {
	return value.replace(/\$\{(\w+)\}/g, (match, varName: string) => {
		const envValue = process.env[varName];
		if (envValue === undefined) {
			throw new Error(
				`Environment variable ${varName} is referenced in config but not set`,
			);
		}
		return envValue;
	});
}

/**
 * Recursively walk a parsed YAML structure and interpolate env vars in all
 * string values.
 */
function interpolateDeep(obj: unknown): unknown {
	if (typeof obj === "string") {
		return interpolateEnvVars(obj);
	}
	if (Array.isArray(obj)) {
		return obj.map(interpolateDeep);
	}
	if (obj !== null && typeof obj === "object") {
		const result: Record<string, unknown> = {};
		for (const [key, value] of Object.entries(obj)) {
			result[key] = interpolateDeep(value);
		}
		return result;
	}
	return obj;
}

/**
 * Load and validate the sidekick.yaml configuration file.
 *
 * 1. Read the YAML file from disk
 * 2. Interpolate ${VAR} references from environment variables
 * 3. Validate against the config schema
 *
 * Throws on missing env vars, invalid YAML, or schema validation failures.
 */
export function loadConfig(path = "sidekick.yaml"): Config {
	const raw = readFileSync(path, "utf-8");
	const parsed = yaml.load(raw);
	const interpolated = interpolateDeep(parsed);
	return configSchema.parse(interpolated);
}
