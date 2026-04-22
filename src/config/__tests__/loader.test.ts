import { readFileSync } from "node:fs";
import { afterEach, describe, expect, it, vi } from "vitest";
import { loadConfig } from "../loader.js";

vi.mock("node:fs");

const validYaml = `
github:
  token: \${GITHUB_TOKEN}
  default_repo: "org/repo"
  workflow: "claude-code-action.yml"

connectors:
  linear:
    api_key: \${LINEAR_API_KEY}
    webhook_secret: \${LINEAR_WEBHOOK_SECRET}

automations:
  - name: "linear-issues"
    trigger:
      connector: linear
      on_label: "sidekick"
      context:
        include:
          - title
          - description
    notifications:
      - connector: linear
        comment: true
`;

describe("loadConfig", () => {
	afterEach(() => {
		vi.unstubAllEnvs();
	});

	it("loads and interpolates a valid config", () => {
		vi.mocked(readFileSync).mockReturnValue(validYaml);
		vi.stubEnv("GITHUB_TOKEN", "ghp_test123");
		vi.stubEnv("LINEAR_API_KEY", "lin_test456");
		vi.stubEnv("LINEAR_WEBHOOK_SECRET", "whsec_test789");

		const config = loadConfig("sidekick.yaml");

		expect(config.github.token).toBe("ghp_test123");
		expect(config.github.default_repo).toBe("org/repo");
		expect(config.github.workflow).toBe("claude-code-action.yml");
		expect(config.connectors.linear?.api_key).toBe("lin_test456");
		expect(config.connectors.linear?.webhook_secret).toBe("whsec_test789");
		expect(config.automations).toHaveLength(1);
		expect(config.automations[0].name).toBe("linear-issues");
		expect(config.automations[0].trigger.connector).toBe("linear");
		expect(config.automations[0].notifications[0].comment).toBe(true);
	});

	it("throws on missing environment variable", () => {
		vi.mocked(readFileSync).mockReturnValue(validYaml);

		expect(() => loadConfig("sidekick.yaml")).toThrow(
			"Environment variable GITHUB_TOKEN is referenced in config but not set",
		);
	});

	it("throws on invalid YAML schema", () => {
		vi.mocked(readFileSync).mockReturnValue(`
github:
  token: "test"
`);

		expect(() => loadConfig("sidekick.yaml")).toThrow();
	});

	it("defaults automations to empty array when omitted", () => {
		vi.mocked(readFileSync).mockReturnValue(`
github:
  token: "direct-token"
  default_repo: "org/repo"
  workflow: "ci.yml"
`);

		const config = loadConfig("sidekick.yaml");
		expect(config.automations).toEqual([]);
	});

	it("defaults notifications to empty array when omitted", () => {
		vi.mocked(readFileSync).mockReturnValue(`
github:
  token: "direct-token"
  default_repo: "org/repo"
  workflow: "ci.yml"

automations:
  - name: "test"
    trigger:
      connector: linear
`);

		const config = loadConfig("sidekick.yaml");
		expect(config.automations[0].notifications).toEqual([]);
	});
});
