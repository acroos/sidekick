import { LinearClient as LinearSDK } from "@linear/sdk";

export interface LinearIssueContext {
	id: string;
	identifier: string;
	title: string;
	description: string | undefined;
	url: string;
	labels: string[];
	comments: string[];
	linkedPullRequests: Array<{ url: string; title: string | undefined }>;
	priority: number;
	priorityLabel: string;
}

export class LinearClient {
	private sdk: LinearSDK;

	constructor(apiKey: string) {
		this.sdk = new LinearSDK({ apiKey });
	}

	/**
	 * Fetch issue details with configurable context extraction.
	 */
	async getIssueContext(
		issueId: string,
		include?: string[],
	): Promise<LinearIssueContext> {
		const issue = await this.sdk.issue(issueId);
		const fields = include ?? ["title", "description", "labels", "comments"];

		const context: LinearIssueContext = {
			id: issue.id,
			identifier: issue.identifier,
			title: issue.title,
			description: fields.includes("description")
				? (issue.description ?? undefined)
				: undefined,
			url: issue.url,
			labels: [],
			comments: [],
			linkedPullRequests: [],
			priority: issue.priority,
			priorityLabel: issue.priorityLabel,
		};

		if (fields.includes("labels")) {
			const labels = await issue.labels();
			context.labels = labels.nodes.map((l) => l.name);
		}

		if (fields.includes("comments")) {
			const comments = await issue.comments();
			context.comments = comments.nodes.map((c) => c.body);
		}

		if (fields.includes("linked_pull_requests")) {
			const attachments = await issue.attachments();
			context.linkedPullRequests = attachments.nodes
				.filter((a) => a.url.includes("pull"))
				.map((a) => ({ url: a.url, title: a.title }));
		}

		return context;
	}

	/**
	 * Post a comment on a Linear issue.
	 */
	async postComment(issueId: string, body: string): Promise<void> {
		await this.sdk.createComment({ issueId, body });
	}

	/**
	 * Update an issue's workflow state by state name.
	 * Looks up the state ID from the issue's team.
	 */
	async updateIssueState(issueId: string, stateName: string): Promise<void> {
		const issue = await this.sdk.issue(issueId);
		const team = await issue.team;

		if (!team) {
			throw new Error(`Issue ${issueId} has no team`);
		}

		const states = await team.states();
		const targetState = states.nodes.find(
			(s) => s.name.toLowerCase() === stateName.toLowerCase(),
		);

		if (!targetState) {
			throw new Error(`State "${stateName}" not found in team ${team.name}`);
		}

		await this.sdk.updateIssue(issueId, { stateId: targetState.id });
	}
}
