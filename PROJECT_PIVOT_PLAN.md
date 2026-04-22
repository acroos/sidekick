# Sidekick — Project Pivot Plan

## Why the pivot

Sidekick was originally a sandboxed agent runtime — container orchestration, workflow execution, agent management. But that's a constrained reimplementation of what GitHub Actions already does. The [Claude Code GitHub Action](https://github.com/anthropics/claude-code-action) already runs Claude sessions in GitHub's infrastructure with full CI integration.

The real gap isn't execution. It's **connectivity**. There's no good way to trigger an agent run from a Linear ticket, a Slack alert, or any other tool where work actually originates — and no way to route the results back.

## What Sidekick becomes

An integration hub that connects productivity and operational tools to Claude Code GitHub Action runs.

```
┌──────────────┐     ┌──────────────────┐     ┌─────────────────────┐
│   Linear     │◀───▶│                  │────▶│  GitHub Actions     │
│   Slack      │◀───▶│    Sidekick      │     │  (claude-code-action│
│   PagerDuty  │◀───▶│                  │     │   workflow)         │
│   ...        │     │  Receives events │     └─────────┬───────────┘
└──────────────┘     │  Triggers runs   │               │
                     │  Routes results  │◀──────────────┘
                     └────────┬─────────┘     Completion webhooks
                              │
                              ▼
                     ┌────────────────┐
                     │   Postgres     │
                     │   (run state)  │
                     └────────────────┘
```

**Inbound flow:** Receive webhook from tool → parse event → extract context → dispatch GitHub Actions workflow via `workflow_dispatch`

**Outbound flow:** Receive GitHub workflow completion webhook → fetch results (PR links, agent output, status) → format and post back to the originating tool

## Technology

| Choice         | Rationale                                                                                                                                           |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| **TypeScript** | First-class SDKs for Linear, Slack, GitHub (Octokit). JSON-native. Fast iteration on integration/glue code.                                         |
| **Hono**       | Lightweight, fast, portable across runtimes. Runs on Vercel out of the box.                                                                         |
| **Vercel**     | Primary deployment target. Serverless functions, zero infrastructure management, automatic scaling.                                                 |
| **Postgres**   | Durable state tracking for run mappings. Managed options everywhere (Neon, Supabase, Vercel Postgres). Supports richer queries as the system grows. |
| **YAML**       | Configuration format for connector definitions. Familiar, readable, already used in the ecosystem.                                                  |

## Core concepts

### Connectors

A connector is an integration with an external tool (Linear, Slack, GitHub, etc.). Each connector provides:

- **Trigger capability** — Webhook endpoint + event parser. Receives events from the tool, evaluates trigger conditions, extracts context, and produces a normalized trigger event.
- **Notification capability** — Result formatter + API client. Takes run results and posts them back to the tool in the appropriate format (comments, status updates, thread replies).
- **Credentials** — API keys, webhook secrets, etc. Defined once in the `connectors` registry and shared across automations.

A connector can serve as a trigger source, a notification target, or both. The role it plays is determined by how it's referenced in automations.

Connectors are modular. Adding a new tool means implementing its trigger and/or notification handlers and defining its config schema. The core framework handles dispatching, state tracking, and result routing.

### Automations

An automation is the unit of configuration that ties a trigger to its notifications. Each automation defines:

- **One trigger** — Which connector, what conditions, what context to extract
- **Zero or more notifications** — Which connectors to notify, with connector-specific config for each

Automations are independent. A Linear label trigger can notify Linear + Slack, while a Slack reaction trigger can notify only the Slack thread. Different triggers can dispatch to different repos or workflows.

### Trigger conditions

Each connector defines its own trigger condition types. For example:

- **Linear:** Label applied, comment command (`/sidekick investigate`), status change
- **Slack:** Emoji reaction, mention, message keyword
- **PagerDuty:** Alert fired, severity threshold

The trigger condition determines _when_ to act. The context configuration determines _what data_ flows to the GitHub Action.

### Context extraction

When a trigger fires, the connector extracts context from the source event. What gets extracted is configurable per automation:

- **Linear:** Issue title, description, labels, priority, linked PRs, comments, attachments
- **Slack:** Message text, thread context, channel, linked resources
- **PagerDuty:** Alert summary, runbook links, affected services, recent changes

This context is assembled into the prompt/inputs for the Claude Code GitHub Action workflow.

### Result routing

When a GitHub Action run completes, Sidekick iterates the automation's notification list and delivers results to each target:

- **Status updates** — Started, running, completed, failed
- **Rich output** — Summary of what the agent did, decisions it made
- **Artifacts** — Links to PRs created, commits pushed, issues updated
- **Errors** — What went wrong, relevant logs

Each notification tracks its own delivery status (pending/sent/failed) for retry handling. Notifications can be filtered to specific events (e.g., only notify Slack on failure).

## Configuration

### Deployment model

Sidekick is currently designed as a **self-deployed** application. The team deploying Sidekick is the team using it. The deployment consists of:

1. **This repo** — Sidekick source code + `sidekick.yaml` config, deployed to Vercel
2. **Target repo(s)** — The codebases where claude-code-action runs, each with a GitHub Actions workflow file

Future consideration: a managed/multi-tenant model where teams connect through a UI and config is stored per-tenant in the database (encrypted). The YAML-first approach keeps things simple now while the core abstractions (connectors, config schema) remain compatible with a database-backed config store later.

### Config file

A `sidekick.yaml` file in the repo root defines connectors, automations, and their behavior. This file is committed to the repo — it contains **no secrets**, only `${VAR}` references that resolve against environment variables at startup.

Secrets (API keys, webhook signing secrets) live in:

- **Vercel:** Environment Variables dashboard (per-environment: production/preview/development)
- **Local dev:** `.env` file (gitignored)

```yaml
# sidekick.yaml — committed to repo, no secrets

# GitHub configuration for dispatching workflows
github:
  token: ${GITHUB_TOKEN}
  default_repo: "org/repo"
  workflow: "claude-code-action.yml"

# Connector credentials — shared registry
connectors:
  linear:
    api_key: ${LINEAR_API_KEY}
    webhook_secret: ${LINEAR_WEBHOOK_SECRET}
  slack:
    bot_token: ${SLACK_BOT_TOKEN}

# Automations — each pairs one trigger with its notifications
automations:
  - name: "linear-issues"
    trigger:
      connector: linear
      on_label: "sidekick"
      context:
        include:
          - title
          - description
          - comments
          - labels
    notifications:
      - connector: linear
        comment: true
        update_status: true
        status_mapping:
          pr_created: "In Review"
          completed: "Done"
          failed: "Triage"
      - connector: slack
        channel: "#engineering-agents"
        on: [completed, failed]

  - name: "slack-requests"
    trigger:
      connector: slack
      channel: "#agent-requests"
      on_reaction: "robot_face"
    notifications:
      - connector: slack
        thread_reply: true
      - connector: linear
        create_issue: true
        team: "ENG"
        on: [completed]
```

The config loader resolves all `${VAR}` references at startup and fails loudly if any referenced variable is missing.

## API routes

```
POST /api/webhooks/linear       — Receives Linear webhook events
POST /api/webhooks/slack        — Receives Slack webhook events (future)
POST /api/webhooks/github       — Receives GitHub Action completion webhooks
GET  /api/health                — Health check
GET  /api/runs                  — List tracked runs (admin/debugging)
GET  /api/runs/:id              — Get run details
```

## Database schema (Postgres)

Two tables: `runs` tracks the lifecycle of triggered workflow runs, `run_notifications` tracks delivery of results to each notification target.

```
runs
├── id              (uuid, primary key)
├── automation      (text — name of the automation that produced this run)
├── trigger_type    (text — "linear", "slack", etc.)
├── trigger_id      (text — ID in the source system, e.g. Linear issue ID)
├── trigger_url     (text — link back to the source)
├── github_run_id   (bigint — GitHub Actions run ID)
├── repo            (text — "org/repo")
├── status          (text — "triggered", "queued", "running", "completed", "failed")
├── context         (jsonb — extracted context sent to the action)
├── result          (jsonb — results received back from the action)
├── created_at      (timestamptz)
├── updated_at      (timestamptz)
└── completed_at    (timestamptz, nullable)

run_notifications
├── id              (uuid, primary key)
├── run_id          (uuid, FK → runs)
├── connector       (text — "linear", "slack", etc.)
├── target_id       (text — where to post: issue ID, channel ID, etc.)
├── target_url      (text — link to the target)
├── config          (jsonb — connector-specific options from the automation)
├── status          (text — "pending", "sent", "failed")
├── error           (text, nullable — failure reason)
├── notified_at     (timestamptz, nullable)
└── created_at      (timestamptz)
```

## Phases

### Phase 1: Foundation

**Goal:** Deployable Hono app on Vercel with Postgres connectivity and config loading.

- Initialize TypeScript project (Hono, Vercel adapter)
- Postgres connection (Drizzle ORM or similar)
- YAML config loader with `${VAR}` interpolation and validation
- Database migrations (runs + run_notifications tables)
- Health endpoint
- Vercel deployment config
- CI (GitHub Actions): lint, typecheck, test

**Deliverable:** A Hono app deployed to Vercel that connects to Postgres and loads config.

### Phase 2: GitHub Actions integration

**Goal:** Can trigger a GitHub Actions workflow and receive completion webhooks.

- GitHub client (Octokit) — dispatch `workflow_dispatch` events
- GitHub webhook handler — parse `workflow_run` completion events
- Run state machine (triggered → queued → running → completed/failed)
- Result extraction from completed runs (PR links, logs, status)
- Run + notification record creation

**Deliverable:** Can programmatically trigger a claude-code-action workflow and track it to completion.

### Phase 3: Linear connector

**Goal:** End-to-end flow from Linear issue to GitHub Action to Linear update.

- Linear connector: trigger capability
  - Webhook handler (signature verification, event parsing)
  - Trigger condition evaluation (label-based for MVP)
  - Context extraction (configurable fields from Linear issue)
- Linear connector: notification capability
  - Result posting (comments on the Linear issue with run summary)
  - Status updates (move issue through statuses based on run state)
- Automation wiring: trigger creates run → run creates notification records → completion delivers to each target

**Deliverable:** Label a Linear issue with "sidekick" → Claude Code runs in GitHub Actions → results posted as a comment on the issue.

### Phase 4: Polish and robustness

**Goal:** Production-ready for personal/team use.

- Error handling and retry logic for notification delivery
- Webhook signature verification for all inbound endpoints
- Logging and observability (structured logs, Vercel-friendly)
- Run list/detail API for debugging
- Documentation: setup guide, Linear integration walkthrough
- Configuration validation at startup (schema validation, connector references)

**Deliverable:** A reliable system you can set up in under 30 minutes and trust to run unattended.

### Future connectors (post-MVP)

- **Slack** — Trigger from messages/reactions, report back in threads. Useful for incident response ("investigate this alert").
- **PagerDuty** — Trigger from alerts, auto-investigate and post findings.
- **Discord** — Community/open-source use case.
- **Generic webhook** — Catch-all for tools that can send webhooks but don't have a dedicated connector.

## What we're removing

The following from the original Sidekick will be removed from the `develop` branch:

- `internal/sandbox/` — Docker sandbox provider (GitHub Actions handles execution)
- `internal/workflow/` — YAML workflow engine (GitHub Actions handles orchestration)
- `internal/agent/` — Agent runtime (claude-code-action handles this)
- `internal/proxy/` — LLM proxy (not needed when execution is in GitHub)
- `internal/event/` — Event bus (replaced by webhook-driven state tracking)
- `internal/task/` — Task manager (replaced by runs table)
- `internal/api/` — Go API server (replaced by Hono)
- `cmd/sidekick/` — Go CLI (not needed for MVP)
- `images/` — Sandbox Dockerfiles
- `workflows/` — Workflow templates
- All Go source code, `go.mod`, `go.sum`

The `docs/decisions/` directory will be preserved and extended.

## Open questions

1. **Workflow file management** — Should Sidekick help generate/manage the claude-code-action workflow YAML in the target repo, or assume the user sets that up manually?
2. **Multi-repo support** — Different automations dispatching to different repos. The config supports this via per-automation `repo` overrides, but the UX needs thought.
3. **Authentication UX** — OAuth flows vs. manual API key entry for connector setup. Manual is simpler for MVP.
4. **Rate limiting** — GitHub Actions has API rate limits and concurrency limits. How aggressively should Sidekick queue/throttle?
5. **Managed/multi-tenant model** — Config in database (encrypted) instead of YAML, with a UI for onboarding. The automations/connectors abstraction supports this, but the onboarding UX and credential storage need design.
