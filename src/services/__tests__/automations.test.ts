import { describe, expect, it, vi } from "vitest";
import type { Config } from "../../config/schema.js";
import { AutomationService } from "../automations.js";

const baseConfig: Config = {
	github: {
		token: "ghp_test",
		default_repo: "org/repo",
		workflow: "claude-code-action.yml",
	},
	connectors: {},
	automations: [
		{
			name: "linear-issues",
			trigger: {
				connector: "linear",
				on_label: "sidekick",
				context: { include: ["title", "description"] },
			},
			notifications: [{ connector: "linear", comment: true }],
		},
		{
			name: "linear-bugs",
			trigger: {
				connector: "linear",
				on_label: "auto-fix",
			},
			notifications: [],
		},
	],
};

function makeDeps() {
	const mockRunService = {
		createRun: vi.fn().mockResolvedValue({ id: "run-1" }),
		setGitHubRunId: vi.fn(),
	};

	const mockGithubClient = {
		dispatchWorkflow: vi.fn().mockResolvedValue({ runId: 99999 }),
	};

	const mockLinearClient = {
		getIssueContext: vi.fn().mockResolvedValue({
			id: "issue-123",
			identifier: "ENG-123",
			title: "Fix the bug",
			description: "It's broken",
			url: "https://linear.app/team/issue/ENG-123",
			labels: ["sidekick"],
			comments: [],
			linkedPullRequests: [],
			priority: 1,
			priorityLabel: "Urgent",
		}),
	};

	const service = new AutomationService(
		baseConfig,
		// biome-ignore lint/suspicious/noExplicitAny: test mocks
		mockRunService as any,
		// biome-ignore lint/suspicious/noExplicitAny: test mocks
		mockGithubClient as any,
		// biome-ignore lint/suspicious/noExplicitAny: test mocks
		mockLinearClient as any,
	);

	return { service, mockRunService, mockGithubClient, mockLinearClient };
}

describe("AutomationService.findLinearLabelAutomations", () => {
	it("finds automations matching the label", () => {
		const { service } = makeDeps();
		const results = service.findLinearLabelAutomations("sidekick");

		expect(results).toHaveLength(1);
		expect(results[0].name).toBe("linear-issues");
	});

	it("matches case-insensitively", () => {
		const { service } = makeDeps();
		const results = service.findLinearLabelAutomations("Sidekick");

		expect(results).toHaveLength(1);
	});

	it("returns empty array for non-matching labels", () => {
		const { service } = makeDeps();
		const results = service.findLinearLabelAutomations("unrelated");

		expect(results).toHaveLength(0);
	});

	it("finds auto-fix automation", () => {
		const { service } = makeDeps();
		const results = service.findLinearLabelAutomations("auto-fix");

		expect(results).toHaveLength(1);
		expect(results[0].name).toBe("linear-bugs");
	});
});

describe("AutomationService.executeLinearTrigger", () => {
	it("creates a run, dispatches workflow, and links run ID", async () => {
		const { service, mockRunService, mockGithubClient, mockLinearClient } =
			makeDeps();

		const automation = baseConfig.automations[0];
		const runId = await service.executeLinearTrigger({
			automation,
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});

		expect(runId).toBe("run-1");

		// Should fetch issue context with configured includes
		expect(mockLinearClient.getIssueContext).toHaveBeenCalledWith("issue-123", [
			"title",
			"description",
		]);

		// Should create a run with notification records
		expect(mockRunService.createRun).toHaveBeenCalledWith(
			expect.objectContaining({
				automation: "linear-issues",
				triggerType: "linear",
				triggerId: "issue-123",
				repo: "org/repo",
			}),
		);

		// Should dispatch the workflow
		expect(mockGithubClient.dispatchWorkflow).toHaveBeenCalledWith(
			expect.objectContaining({
				repo: "org/repo",
				workflow: "claude-code-action.yml",
			}),
		);

		// Should link the GitHub run ID
		expect(mockRunService.setGitHubRunId).toHaveBeenCalledWith(
			"run-1",
			"99999",
		);
	});

	it("uses automation-level repo override", async () => {
		const { service, mockGithubClient, mockRunService } = makeDeps();

		const automation = {
			...baseConfig.automations[0],
			repo: "org/other-repo",
		};

		await service.executeLinearTrigger({
			automation,
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});

		expect(mockGithubClient.dispatchWorkflow).toHaveBeenCalledWith(
			expect.objectContaining({ repo: "org/other-repo" }),
		);
		expect(mockRunService.createRun).toHaveBeenCalledWith(
			expect.objectContaining({ repo: "org/other-repo" }),
		);
	});

	it("includes automation prompt in dispatched workflow", async () => {
		const { service, mockGithubClient } = makeDeps();

		const automation = {
			...baseConfig.automations[0],
			prompt: "Fix this issue and create a PR.",
		};

		await service.executeLinearTrigger({
			automation,
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});

		const dispatchCall = mockGithubClient.dispatchWorkflow.mock.calls[0][0];
		expect(dispatchCall.inputs.prompt).toContain("ENG-123: Fix the bug");
		expect(dispatchCall.inputs.prompt).toContain(
			"Fix this issue and create a PR.",
		);
	});

	it("dispatches without custom prompt section when prompt is not configured", async () => {
		const { service, mockGithubClient } = makeDeps();

		const automation = baseConfig.automations[1]; // linear-bugs — no prompt

		await service.executeLinearTrigger({
			automation,
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});

		const dispatchCall = mockGithubClient.dispatchWorkflow.mock.calls[0][0];
		const prompt: string = dispatchCall.inputs.prompt;
		expect(prompt).toContain("ENG-123: Fix the bug");
		// No custom prompt separator — the only --- should be from output instructions
		const outputStart = prompt.indexOf(".sidekick-output.json");
		expect(outputStart).toBeGreaterThan(0);
		const beforeOutput = prompt.slice(0, outputStart);
		// The context section has no --- separators (no comments, no custom prompt)
		expect(beforeOutput).not.toContain("Fix this issue");
	});

	it("always includes structured output instructions", async () => {
		const { service, mockGithubClient } = makeDeps();

		await service.executeLinearTrigger({
			automation: baseConfig.automations[0],
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});

		const prompt =
			mockGithubClient.dispatchWorkflow.mock.calls[0][0].inputs.prompt;
		expect(prompt).toContain(".sidekick-output.json");
		expect(prompt).toContain("files_changed");
		expect(prompt).toContain("Do NOT create branches");
	});

	it("places output instructions after custom prompt", async () => {
		const { service, mockGithubClient } = makeDeps();

		const automation = {
			...baseConfig.automations[0],
			prompt: "Fix the tooltip issue.",
		};

		await service.executeLinearTrigger({
			automation,
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});

		const prompt =
			mockGithubClient.dispatchWorkflow.mock.calls[0][0].inputs.prompt;
		const customPromptIdx = prompt.indexOf("Fix the tooltip issue.");
		const outputIdx = prompt.indexOf(".sidekick-output.json");
		expect(customPromptIdx).toBeGreaterThan(0);
		expect(outputIdx).toBeGreaterThan(customPromptIdx);
	});

	it("handles null GitHub run ID gracefully", async () => {
		const { service, mockGithubClient, mockRunService } = makeDeps();
		mockGithubClient.dispatchWorkflow.mockResolvedValue({ runId: null });

		await service.executeLinearTrigger({
			automation: baseConfig.automations[0],
			issueId: "issue-123",
			issueUrl: "https://linear.app/team/issue/ENG-123",
		});

		expect(mockRunService.setGitHubRunId).not.toHaveBeenCalled();
	});
});
