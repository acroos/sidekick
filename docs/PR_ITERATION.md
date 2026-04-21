# PR Iteration — Plan

## Vision

Close the feedback loop on agent-generated PRs. When a reviewer leaves comments on a Sidekick PR, the agent picks them up, makes targeted changes, and pushes — behaving like a teammate who responds to code review, not a bot that fires and forgets.

This is the natural complement to the autonomous intake system. Without iteration, every PR that needs revisions falls back to a human. With it, the agent handles the full lifecycle: task → PR → review → revisions → merge.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                                                                  │
│  GitHub/GitLab PR Events (webhook or poll)                       │
│       │                                                          │
│       ▼                                                          │
│  Review Listener                                                 │
│       │ filter: only sidekick-generated PRs                      │
│       │ filter: only new/unresolved comments since last round    │
│       │                                                          │
│       ▼                                                          │
│  Context Reconstruction                                          │
│       │ Load: original task context (persisted from round 0)     │
│       │ Load: current PR diff                                    │
│       │ Load: review comments (with file/line context)           │
│       │ Load: iteration round number                             │
│       │                                                          │
│       ▼                                                          │
│  Sidekick API (POST /tasks)                                      │
│       │ workflow: pr-iteration                                   │
│       │ variables: original context + review feedback            │
│       │                                                          │
│       ▼                                                          │
│  Agent addresses feedback → pushes commits → replies to comments │
│       │                                                          │
│       ▼                                                          │
│  Round complete                                                  │
│       │                                                          │
│       ├── Reviewer approves → done                               │
│       ├── More feedback → next round                             │
│       └── Max rounds hit → hand back to human                    │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

---

## Trigger & Filtering

### What triggers iteration

- PR review submitted (with comments or changes requested)
- New inline comments on a Sidekick-generated PR
- Reviewer uses GitHub's "Request changes" action

### What does NOT trigger iteration

- PR review that only approves (no action needed)
- Comments on non-Sidekick PRs (ignore entirely)
- Comments the agent has already addressed in a previous round
- The agent's own reply comments (avoid self-triggering loops)

### Identification

Sidekick-generated PRs are identified by:
- A `sidekick-automated` label
- A metadata block in the PR description containing the original task ID

```markdown
<!-- sidekick:task_id=task_abc123 -->
```

This task ID links back to the persisted task context needed for reconstruction.

---

## Context Reconstruction

The original agent and sandbox are gone after round 0. For iteration to work, the new agent needs sufficient context to understand what was done and what the reviewer wants changed.

### Persisted from the original task

When a task completes and opens a PR, Sidekick persists:
- The original workflow ref and variables
- The assembled context (task description, coding standards, etc.)
- The final PR diff (what the agent actually changed)
- The task ID (linked in the PR description)

### Assembled for each iteration round

| Context item | Source | Purpose |
|---|---|---|
| Original task description | Persisted task context | Why this PR exists |
| Coding standards | Static file in repo | Same constraints as the original task |
| Current PR diff | GitHub API | What the agent has done so far (cumulative, all rounds) |
| Review comments | GitHub API | What the reviewer wants changed |
| Iteration round | Sidekick metadata | Awareness of how many rounds have passed |

### Review comment formatting

Comments are structured with file/line context so the agent knows exactly where to look:

```
## Review feedback to address

### Comment 1 (inline: src/user/UserService.ts:44)
Reviewer: @alice
> This null check should use optional chaining instead of an explicit if statement.

### Comment 2 (inline: src/user/UserService.ts:52)
Reviewer: @alice
> Missing test for the case where avatar is undefined (not just null).

### Comment 3 (general)
Reviewer: @bob
> Could you also update the JSDoc for this method to document that avatar can be null?
```

---

## Iteration Workflow

A dedicated workflow, distinct from the original task workflow. The steps are different — no need to clone fresh or discover the problem, just check out the branch, address feedback, and push.

```yaml
name: pr-iteration
timeout: 15m
max_retries: 0

sandbox:
  image: sidekick-sandbox-node:20
  network: restricted
  allow_hosts:
    - registry.npmjs.org
    - github.com

steps:
  - name: checkout
    type: deterministic
    run: |
      git clone $REPO_URL /workspace
      git -C /workspace checkout $BRANCH_NAME
      npm ci --prefix /workspace
    timeout: 5m
    on_failure: abort

  - name: iterate
    type: agent
    context:
      - type: variable
        key: ORIGINAL_TASK
        label: "Original task"
      - type: variable
        key: PR_DIFF
        label: "Current PR diff (all rounds)"
      - type: variable
        key: REVIEW_COMMENTS
        label: "Review feedback to address"
      - type: variable
        key: ITERATION_ROUND
        label: "Iteration round"
      - type: file
        path: .sidekick/coding-standards.md
        label: "Coding standards"
    prompt: |
      You are iterating on an existing PR in /workspace.

      Address each piece of review feedback listed above. For each comment:
      1. Make the requested change
      2. Note what you changed

      Rules:
      - Make targeted changes only — address the feedback, nothing more
      - Do not refactor unrelated code or make unsolicited improvements
      - If a reviewer suggests a specific change, follow it closely
      - If you disagree with feedback or think it would break something,
        note your concern rather than silently ignoring it
      - Run tests to verify your changes don't break anything
    allowed_tools:
      - Edit
      - Read
      - Bash
      - Glob
      - Grep
    timeout: 10m

  - name: test
    type: deterministic
    run: npm test --prefix /workspace
    timeout: 5m
    on_failure: abort

  - name: push
    type: deterministic
    when: steps.test.status == 'succeeded'
    run: |
      git -C /workspace add -A
      git -C /workspace commit -m "address review feedback (round $ITERATION_ROUND)"
      git -C /workspace push origin $BRANCH_NAME
    timeout: 2m
```

---

## Agent Behavior Rules

### Scoping

The iteration agent must be disciplined about scope:
- **Do** address every review comment specifically
- **Do** reply to inline comments explaining what was changed
- **Do** follow suggested changes closely (treat them as near-literal instructions)
- **Don't** refactor surrounding code
- **Don't** "improve" things the reviewer didn't mention
- **Don't** re-solve the original problem from scratch

### Handling different feedback types

| Feedback type | Agent response |
|---|---|
| Specific inline suggestion | Apply the change, reply confirming |
| General style/approach feedback | Make the change across affected code, reply with summary |
| "This approach is wrong" | Escalate — don't attempt a rewrite. Comment explaining and hand back |
| Conflicting feedback from multiple reviewers | Note the conflict, pick the safer option, explain the trade-off |
| Question (not a change request) | Reply with an answer, don't change code unless the answer implies a change |
| Nitpick / optional suggestion | Apply it (low cost), reply confirming |

### Comment replies

After pushing changes, the agent replies to each review comment:

```
@sidekick-bot: Addressed — switched to optional chaining: `user.avatar?.url ?? null`
(commit def456)
```

For general comments:
```
@sidekick-bot: Added JSDoc to `getProfile` documenting the nullable avatar field.
Also added a test case for `undefined` avatar as requested.
(commit def456)
```

---

## Stopping Conditions

| Condition | Action |
|---|---|
| Reviewer approves | Done. Agent takes no further action. |
| Max rounds reached (configurable, default 3) | Agent comments: "I've completed 3 rounds of revisions. Handing this back for a human to continue." Removes self-assignment. |
| Reviewer says "I'll take it from here" | Agent stops. Detects via keyword matching or a signal label (`sidekick-stop`). |
| Feedback is "fundamentally wrong approach" | Agent comments acknowledging the feedback, suggests it may need a fresh approach, and hands back. Does NOT attempt a rewrite. |
| Tests fail after addressing feedback | Agent comments with the failure, asks reviewer for guidance rather than guessing. |

---

## Persistence Requirements

For iteration to work, the following must survive beyond the original task's sandbox lifecycle:

| Data | Stored where | Retention |
|---|---|---|
| Original task context (workflow, variables, assembled context) | Task record in SQLite | Until PR is merged or closed |
| Task ID → PR mapping | Task record | Until PR is merged or closed |
| Iteration round count | Task metadata | Updated each round |
| Previous iteration outcomes | Task metadata | For context in subsequent rounds |

---

## Safety

- **Self-triggering prevention**: Agent ignores its own comments. Review listener filters by author.
- **Rate limiting**: Max 1 iteration round per PR per hour (avoid rapid-fire cycles with an impatient reviewer).
- **Scope boundaries**: Same restricted paths from the autonomy policy apply. If feedback asks the agent to touch restricted code, it escalates.
- **No force push**: Iteration always adds new commits, never rewrites history.
- **Audit trail**: Every iteration round is a separate Sidekick task with full event streaming and logging.

---

## Open Questions

1. **Cross-PR iteration**: If a reviewer says "this should actually be two separate PRs," should the agent handle splitting?
2. **Draft → Ready transition**: Should the agent mark a draft PR as ready-for-review after addressing all feedback, or always leave that to a human?
3. **Iteration on non-Sidekick PRs**: Could this feature be offered for human-authored PRs too? ("Sidekick, address Alice's comments on my PR.") This blurs the line between autonomous and interactive.
4. **Selective addressing**: If there are 10 comments and the agent can confidently address 8, should it push those 8 and flag the remaining 2? Or wait until it can address all of them?
