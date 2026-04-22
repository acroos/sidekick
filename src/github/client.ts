import { Octokit } from "octokit";

export interface DispatchResult {
	/** GitHub Actions run ID, resolved by polling after dispatch */
	runId: number | null;
}

export interface WorkflowRunResult {
	runId: number;
	status: string;
	conclusion: string | null;
	htmlUrl: string;
	headBranch: string | null;
	pullRequests: Array<{
		number: number;
		url: string;
		title: string;
	}>;
}

function parseRepo(repo: string): { owner: string; repo: string } {
	const [owner, name] = repo.split("/");
	if (!owner || !name) {
		throw new Error(`Invalid repo format "${repo}", expected "owner/repo"`);
	}
	return { owner, repo: name };
}

export class GitHubClient {
	private octokit: Octokit;

	constructor(token: string) {
		this.octokit = new Octokit({ auth: token });
	}

	/**
	 * Dispatch a workflow_dispatch event and attempt to resolve the run ID
	 * by polling recent runs.
	 */
	async dispatchWorkflow(params: {
		repo: string;
		workflow: string;
		ref?: string;
		inputs?: Record<string, string>;
	}): Promise<DispatchResult> {
		const { owner, repo } = parseRepo(params.repo);
		const dispatchedAt = new Date().toISOString();

		await this.octokit.rest.actions.createWorkflowDispatch({
			owner,
			repo,
			workflow_id: params.workflow,
			ref: params.ref ?? "main",
			inputs: params.inputs,
		});

		// Poll briefly to find the created run. GitHub takes a moment to
		// create the run after dispatch.
		const runId = await this.pollForRun({
			owner,
			repo,
			workflow: params.workflow,
			createdAfter: dispatchedAt,
		});

		return { runId };
	}

	/**
	 * Fetch details and pull requests for a workflow run.
	 */
	async getWorkflowRun(params: {
		repo: string;
		runId: number;
	}): Promise<WorkflowRunResult> {
		const { owner, repo } = parseRepo(params.repo);

		const { data: run } = await this.octokit.rest.actions.getWorkflowRun({
			owner,
			repo,
			run_id: params.runId,
		});

		// Fetch PRs associated with the head SHA
		const pullRequests = await this.getPullRequestsForSha({
			owner,
			repo,
			sha: run.head_sha,
		});

		return {
			runId: run.id,
			status: run.status ?? "unknown",
			conclusion: run.conclusion ?? null,
			htmlUrl: run.html_url,
			headBranch: run.head_branch,
			pullRequests,
		};
	}

	/**
	 * Poll the workflow runs API to find a run created after a given timestamp.
	 * Retries a few times with backoff since GitHub takes a moment to create
	 * the run after a dispatch.
	 */
	private async pollForRun(params: {
		owner: string;
		repo: string;
		workflow: string;
		createdAfter: string;
	}): Promise<number | null> {
		const maxAttempts = 5;
		const delayMs = 2000;

		for (let attempt = 0; attempt < maxAttempts; attempt++) {
			if (attempt > 0) {
				await sleep(delayMs);
			}

			const { data } = await this.octokit.rest.actions.listWorkflowRuns({
				owner: params.owner,
				repo: params.repo,
				workflow_id: params.workflow,
				event: "workflow_dispatch",
				created: `>=${params.createdAfter}`,
				per_page: 5,
			});

			if (data.workflow_runs.length > 0) {
				// Return the most recent run
				return data.workflow_runs[0].id;
			}
		}

		return null;
	}

	private async getPullRequestsForSha(params: {
		owner: string;
		repo: string;
		sha: string;
	}): Promise<Array<{ number: number; url: string; title: string }>> {
		try {
			const { data: prs } =
				await this.octokit.rest.repos.listPullRequestsAssociatedWithCommit({
					owner: params.owner,
					repo: params.repo,
					commit_sha: params.sha,
				});

			return prs.map((pr) => ({
				number: pr.number,
				url: pr.html_url,
				title: pr.title,
			}));
		} catch {
			return [];
		}
	}
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
