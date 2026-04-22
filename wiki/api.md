# API

All routes are mounted under `/api`. The app is a [Hono](https://hono.dev) server deployed as a Vercel serverless function.

## Routes

### Health

```
GET /api/health
```

Returns server status. No authentication required.

```json
{ "status": "ok", "version": "0.1.0" }
```

### Linear Webhook

```
POST /api/webhooks/linear
```

Receives webhook events from Linear. Verifies the `linear-signature` header using HMAC-SHA256.

**Flow:**
1. Verify signature against the connector's `webhook_secret`
2. Parse the payload for label addition events
3. Match against configured [automations](automations.md)
4. For each match, execute the trigger (extract context, dispatch to GitHub, create [run](runs.md))

**Responses:**
- `200 { ok: true, runs: ["run-id-1"] }` — trigger matched, runs created
- `200 { ok: true, ignored: true }` — valid webhook but no trigger matched
- `401 { error: "Invalid signature" }` — signature verification failed

### GitHub Webhook

```
POST /api/webhooks/github
```

Receives `workflow_run` events from GitHub Actions. Verifies the `x-hub-signature-256` header.

**Flow:**
1. Verify signature against `GITHUB_WEBHOOK_SECRET`
2. Parse the `workflow_run` event (action: `queued`, `in_progress`, `completed`)
3. Look up the Sidekick run by GitHub run ID
4. Update run status through the state machine
5. On completion, fetch results and deliver [notifications](notifications.md)

**Responses:**
- `200 { ok: true, runId: "...", action: "completed" }` — processed
- `200 { ok: true, ignored: true }` — not a workflow_run event or not a Sidekick-dispatched run
- `401 { error: "Invalid signature" }` — signature verification failed

### List Runs

```
GET /api/runs
```

Query [runs](runs.md) with optional filters, ordered by creation time (newest first).

**Query parameters:**
- `automation` — filter by automation name
- `status` — filter by run status (`triggered`, `queued`, `running`, `completed`, `failed`)
- `limit` — max results (default: 50)
- `offset` — pagination offset (default: 0)

**Response:**
```json
{
  "runs": [
    {
      "id": "uuid",
      "automation": "linear-issues",
      "triggerType": "linear",
      "triggerId": "ISSUE-123",
      "status": "completed",
      "repo": "org/repo",
      "createdAt": "2026-04-20T12:00:00Z",
      "completedAt": "2026-04-20T12:05:00Z"
    }
  ]
}
```

### Get Run

```
GET /api/runs/:id
```

Fetch a single run by ID, including its notification records.

**Response:**
```json
{
  "run": {
    "id": "uuid",
    "automation": "linear-issues",
    "triggerType": "linear",
    "triggerId": "ISSUE-123",
    "status": "completed",
    "context": { ... },
    "result": { ... },
    "notifications": [
      {
        "id": "uuid",
        "connector": "linear",
        "targetId": "ISSUE-123",
        "status": "sent",
        "notifiedAt": "2026-04-20T12:05:30Z"
      }
    ]
  }
}
```

**Errors:**
- `404 { error: "Run not found" }`

## Middleware

All routes pass through:

- **Request logger** (`src/middleware/logger.ts`) — Structured JSON logging to stdout, logs method, path, status, and duration for every request
- **Error handler** (`src/middleware/error-handler.ts`) — Catches unhandled errors, returns JSON with appropriate status codes

## Implementation

Route handlers are defined as Hono router factories that accept dependencies:

- `src/routes/health.ts` — `healthRoutes` (static, no deps)
- `src/routes/linear.ts` — `createLinearRoutes(deps)` — needs `AutomationService`, webhook secret
- `src/routes/github.ts` — `createGitHubRoutes(deps)` — needs `RunService`, `GitHubClient`, `NotificationService`, webhook secret
- `src/routes/runs.ts` — `createRunsRoutes(deps)` — needs `RunService`
