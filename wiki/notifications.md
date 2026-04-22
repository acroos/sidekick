# Notifications

Notifications handle the outbound side of Sidekick — delivering results from completed [runs](runs.md) back to the tools where work originated.

Each [automation](automations.md) can define zero or more notification targets. When a run completes, Sidekick iterates the automation's notification list and delivers results to each.

## What Gets Delivered

| Category | Description |
|---|---|
| Status updates | Move the source item through statuses (e.g., Linear issue → "In Review") |
| Rich output | Summary of what the agent did and decisions it made |
| Artifacts | Links to PRs created, commits pushed, issues updated |
| Errors | What went wrong, relevant context |

## Notification Records

Each notification target for a run gets its own record in the `run_notifications` table. This enables:

- Independent tracking per target (Slack might succeed while Linear fails)
- Per-notification retry handling
- Delivery status visibility via the [API](api.md)

### Fields

| Field | Type | Description |
|---|---|---|
| `id` | UUID | Primary key |
| `runId` | UUID | FK to the parent run |
| `connector` | text | Target connector (`"linear"`, `"slack"`, etc.) |
| `targetId` | text | Where to post (issue ID, channel ID, etc.) |
| `targetUrl` | text | Link to the target |
| `config` | JSONB | Connector-specific options from the automation config |
| `status` | text | `pending → sent` or `pending → failed` |
| `error` | text | Failure reason (if failed) |
| `retryCount` | integer | How many delivery attempts have been made |
| `maxRetries` | integer | Maximum retry attempts (default: 3) |
| `notifiedAt` | timestamptz | When successfully delivered |

## Delivery Flow

1. A run reaches a terminal state (`completed` or `failed`)
2. The GitHub webhook handler calls `NotificationService.deliverNotifications(runId)`
3. For each pending notification record:
   - The connector formats the result for the target tool
   - The connector's API client delivers the notification
   - The notification record is marked `sent` or `failed`

## Retry Handling

When delivery fails:

1. `retryCount` is incremented
2. If `retryCount < maxRetries`, the notification is reset to `pending` for the next delivery attempt
3. If `retryCount >= maxRetries`, the notification stays `failed`

Failed notifications that are still retryable can be found via `RunService.getRetryableNotifications()`.

## Filtering by Outcome

Notifications can be filtered to specific run outcomes using the `on` field:

```yaml
notifications:
  - connector: slack
    channel: "#engineering-agents"
    on: [completed, failed]    # Only notify on these outcomes
  - connector: linear
    comment: true              # No 'on' filter — notify on all outcomes
```

## Implementation

- `src/services/notifications.ts` — `NotificationService`: orchestrates delivery, calls connector-specific formatters and API clients
- `src/services/runs.ts` — Notification record management (create, query, update status, retry logic)
