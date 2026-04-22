# Runs

A run represents a single triggered workflow execution — from the moment an [automation](automations.md) fires to the final result. Runs are tracked in Postgres and serve as the central state object connecting triggers, GitHub Actions executions, and [notifications](notifications.md).

## Lifecycle

```
triggered → queued → running → completed
                             → failed
```

| Status | Meaning |
|---|---|
| `triggered` | Automation matched, `workflow_dispatch` sent to GitHub |
| `queued` | GitHub Actions has queued the workflow run |
| `running` | GitHub Actions workflow is executing |
| `completed` | Workflow finished successfully |
| `failed` | Workflow finished with an error |

Transitions are enforced by a state machine in `RunService.updateStatus()`. Invalid transitions (e.g., `completed → running`) are rejected. Terminal states (`completed`, `failed`) cannot transition further.

## State Machine Transitions

```
triggered → queued, running, completed, failed
queued    → running, completed, failed
running   → completed, failed
completed → (none)
failed    → (none)
```

The `triggered → completed` and `triggered → failed` shortcuts exist because GitHub sometimes delivers completion webhooks without prior queued/in_progress events.

## Run Fields

| Field | Type | Description |
|---|---|---|
| `id` | UUID | Primary key, auto-generated |
| `automation` | text | Name of the automation that produced this run |
| `triggerType` | text | Connector type (`"linear"`, `"slack"`, etc.) |
| `triggerId` | text | ID in the source system (e.g., Linear issue ID) |
| `triggerUrl` | text | Link back to the source |
| `githubRunId` | text | GitHub Actions run ID (set after dispatch) |
| `repo` | text | Target repository (`"owner/repo"`) |
| `status` | text | Current lifecycle status |
| `context` | JSONB | Extracted context sent to the action |
| `result` | JSONB | Results received back from the action |
| `createdAt` | timestamptz | When the run was created |
| `updatedAt` | timestamptz | Last status change |
| `completedAt` | timestamptz | When the run reached a terminal state |

See [Database](database.md) for the full schema definition.

## Implementation

`RunService` (`src/services/runs.ts`) provides:

- `createRun()` — Creates a run and its associated notification records in one transaction
- `updateStatus()` — Advances the state machine, sets result/completion data on terminal states
- `setGitHubRunId()` — Links the Sidekick run to the GitHub Actions run after dispatch
- `findByGitHubRunId()` — Looks up a run when processing GitHub completion webhooks
- `getById()` — Fetches a run with its notification records
- `list()` — Queries runs with optional filters (automation, status, pagination)
- `getPendingNotifications()` / `getRetryableNotifications()` — Finds notifications ready for delivery
- `updateNotificationStatus()` — Marks notifications sent/failed, handles retry logic
