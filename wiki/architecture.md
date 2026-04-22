# Architecture

## System Diagram

```
┌──────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│   Linear     │◀───▶│                  │────▶│  GitHub Actions     │
│   Slack      │◀───▶│    Sidekick      │     │  (claude-code-action│
│   PagerDuty  │◀───▶│                  │     │   workflow)         │
│   ...        │     │  Receives events │     └─────────┬───────────┘
└──────────────┘     │  Triggers runs   │               │
                     │  Routes results  │◀──────────────┘
                     └────────┬─────────┘   Completion webhooks
                              │
                              ▼
                     ┌────────────────┐
                     │   Postgres     │
                     │   (run state)  │
                     └────────────────┘
```

## Data Flow

### Inbound (trigger)

1. External tool sends a webhook to Sidekick (e.g., `POST /api/webhooks/linear`)
2. Route handler verifies the webhook signature (HMAC)
3. Event is parsed and matched against configured [automations](automations.md)
4. If a trigger matches, the [connector](connectors.md) extracts context from the source (issue details, thread content, etc.)
5. Sidekick dispatches a `workflow_dispatch` event to GitHub Actions via Octokit
6. A [run](runs.md) record is created in Postgres with status `triggered`
7. [Notification](notifications.md) records are created for each target in the automation

### Outbound (completion)

1. GitHub sends a `workflow_run` webhook to `POST /api/webhooks/github`
2. Sidekick verifies the signature and maps the GitHub run ID to a Sidekick run
3. Run status is updated through the state machine (`queued → running → completed/failed`)
4. On completion, results are fetched from the GitHub API (PR links, agent output, status)
5. Notifications are delivered to each target (Linear comments, Slack threads, status updates)

## Application Structure

```
src/
├── app.ts                 — Hono app factory with dependency injection
├── index.ts               — Node.js entry point (local dev)
├── config/                — YAML config loader + Zod schema validation
├── connectors/
│   └── linear/            — Linear webhook parsing, signature verification, API client
├── github/                — Octokit client, workflow dispatch, webhook parsing
├── db/                    — Drizzle ORM schema and database client
├── routes/                — HTTP route handlers (health, webhooks, runs)
├── services/              — Business logic (runs, automations, notifications)
└── middleware/             — Request logging, error handling

api/
└── index.ts               — Vercel serverless entry point
```

## Dependency Injection

The Hono app uses constructor-based dependency injection. `createApp()` in `src/app.ts` accepts a `AppDeps` object containing all services and clients. This makes testing straightforward — inject mocks for any dependency.

A minimal app (just the health endpoint) is also exported as the default for contexts where full services aren't needed.

## Technology Choices

| Choice | Rationale |
|---|---|
| **TypeScript** | First-class SDKs for Linear, Slack, GitHub (Octokit). JSON-native. Fast iteration on integration/glue code. |
| **Hono** | Lightweight, fast, portable across runtimes. Runs on Vercel out of the box. |
| **Vercel** | Serverless functions, zero infrastructure management, automatic scaling. |
| **Postgres** | Durable state tracking for run mappings. Managed options everywhere (Neon, Supabase, Vercel Postgres). |
| **Drizzle ORM** | Type-safe queries, lightweight, good migration tooling. |
| **YAML config** | Familiar, readable, supports `${VAR}` env interpolation. No secrets in the repo. |
