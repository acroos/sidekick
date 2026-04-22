import type { Automation, Config } from "../config/schema.js";
import type {
	LinearClient,
	LinearIssueContext,
} from "../connectors/linear/client.js";
import type { GitHubClient } from "../github/client.js";
import type { RunService } from "./runs.js";

export class AutomationService {
	constructor(
		private config: Config,
		private runService: RunService,
		private githubClient: GitHubClient,
		private linearClient: LinearClient | null,
	) {}

	/**
	 * Find automations that match a Linear label trigger.
	 */
	findLinearLabelAutomations(labelName: string): Automation[] {
		return this.config.automations.filter(
			(a) =>
				a.trigger.connector === "linear" &&
				a.trigger.on_label?.toLowerCase() === labelName.toLowerCase(),
		);
	}

	/**
	 * Execute an automation triggered by a Linear issue.
	 * Creates a run, dispatches the GitHub Action, and links the run ID.
	 */
	async executeLinearTrigger(params: {
		automation: Automation;
		issueId: string;
		issueUrl: string;
	}): Promise<string> {
		const { automation, issueId, issueUrl } = params;

		// Extract context from the Linear issue
		let context: LinearIssueContext | null = null;
		if (this.linearClient) {
			context = await this.linearClient.getIssueContext(
				issueId,
				automation.trigger.context?.include,
			);
		}

		// Build notification records from the automation config
		const notifications = automation.notifications.map((n) => ({
			connector: n.connector,
			targetId: n.connector === "linear" ? issueId : (n.channel ?? "unknown"),
			targetUrl: n.connector === "linear" ? issueUrl : undefined,
			config: n,
		}));

		// Determine target repo and workflow
		const repo = automation.repo ?? this.config.github.default_repo;
		const workflow = this.config.github.workflow;

		// Create the run record
		const run = await this.runService.createRun({
			automation: automation.name,
			triggerType: "linear",
			triggerId: issueId,
			triggerUrl: issueUrl,
			repo,
			context,
			notifications,
		});

		// Dispatch the GitHub Actions workflow
		const prompt = this.buildPrompt(context);
		const dispatchResult = await this.githubClient.dispatchWorkflow({
			repo,
			workflow,
			inputs: {
				prompt,
				sidekick_run_id: run.id,
			},
		});

		// Link the GitHub run ID if we found one
		if (dispatchResult.runId) {
			await this.runService.setGitHubRunId(
				run.id,
				String(dispatchResult.runId),
			);
		}

		return run.id;
	}

	/**
	 * Build a prompt from the extracted context to pass to claude-code-action.
	 */
	private buildPrompt(context: LinearIssueContext | null): string {
		if (!context) {
			return "No context available.";
		}

		const parts: string[] = [];
		parts.push(`# ${context.identifier}: ${context.title}`);

		if (context.description) {
			parts.push(`\n## Description\n${context.description}`);
		}

		if (context.labels.length > 0) {
			parts.push(`\n**Labels:** ${context.labels.join(", ")}`);
		}

		if (context.comments.length > 0) {
			parts.push("\n## Comments");
			for (const comment of context.comments) {
				parts.push(`\n---\n${comment}`);
			}
		}

		if (context.linkedPullRequests.length > 0) {
			parts.push("\n## Linked Pull Requests");
			for (const pr of context.linkedPullRequests) {
				parts.push(`- ${pr.title ?? pr.url}`);
			}
		}

		return parts.join("\n");
	}
}
