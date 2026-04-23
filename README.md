# Sidekick

An integration hub that connects productivity tools to [Claude Code GitHub Action](https://github.com/anthropics/claude-code-action) runs. Sidekick receives webhooks from tools like Linear and Slack, triggers agent workflows in GitHub Actions, and routes results back to where the work originated.

Label a Linear issue → Claude Code runs in GitHub Actions → results posted back as a comment.

## How It Works

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

**Inbound:** Receive webhook from tool → parse event → extract context → dispatch `workflow_dispatch` to GitHub Actions

**Outbound:** Receive GitHub completion webhook → fetch results → post back to the originating tool (comments, status updates, thread replies)

## Core Concepts

- **Connectors** — Integrations with external tools (Linear, Slack, etc.). Each provides trigger and/or notification capabilities. Credentials defined once, shared across automations.
- **Automations** — The unit of config that pairs one trigger with zero or more notifications. Each automation is independent.
- **Runs** — A triggered workflow execution. Tracked in Postgres with status lifecycle: `triggered → queued → running → completed/failed`.
- **Notifications** — Per-target result delivery. Each notification tracks its own status for retry handling.

## Quick Start

### Prerequisites

- A GitHub token with `actions:write` scope on your target repo
- A target repo with a [claude-code-action](https://github.com/anthropics/claude-code-action) workflow

### Option A: Docker (recommended)

```bash
# Copy the example config and environment
cp sidekick.example.yaml sidekick.yaml
cp .env.example .env
# Edit both files with your credentials

# Start Sidekick + Postgres
docker compose up
```

Docker Compose starts Postgres automatically. Migrations run on startup — no manual steps needed.

### Option B: Local Development

Requires Node.js 20+ and a Postgres database ([Neon](https://neon.tech), [Supabase](https://supabase.com), or local).

```bash
npm install

cp sidekick.example.yaml sidekick.yaml
cp .env.example .env
# Edit both files with your credentials

npm run dev
```

Migrations run automatically on startup.

### Configuration

Sidekick is configured via a `sidekick.yaml` file in the repo root. This file is committed to the repo — it contains no secrets, only `${VAR}` references that resolve against environment variables at startup.

```yaml
github:
  token: ${GITHUB_TOKEN}
  default_repo: "org/repo"
  workflow: "claude-code-action.yml"

connectors:
  linear:
    api_key: ${LINEAR_API_KEY}
    webhook_secret: ${LINEAR_WEBHOOK_SECRET}

automations:
  - name: "linear-issues"
    trigger:
      connector: linear
      on_label: "sidekick"
      context:
        include: [title, description, comments, labels]
    notifications:
      - connector: linear
        comment: true
        update_status: true
        status_mapping:
          pr_created: "In Review"
          completed: "Done"
          failed: "Triage"
```

Secrets live in environment variables — Vercel's dashboard for production, `.env` for local dev.

### Deploy

**Docker:** Run the image anywhere containers run — your own server, Fly.io, Railway, etc. Set environment variables and mount your `sidekick.yaml`.

**Vercel:** The project is also configured for Vercel out of the box. Connect the repo, set your environment variables, and deploy.

## Tech Stack

| Component | Choice |
|---|---|
| Language | TypeScript |
| Web framework | [Hono](https://hono.dev) |
| Deployment | Docker or [Vercel](https://vercel.com) (serverless) |
| Database | Postgres + [Drizzle ORM](https://orm.drizzle.team) |
| Configuration | YAML with `${VAR}` env interpolation |
| Linting | [Biome](https://biomejs.dev) |
| Testing | [Vitest](https://vitest.dev) |

## Development

```bash
npm run build        # TypeScript compilation
npm run typecheck    # Type checking without emit
npm run lint         # Biome linter
npm run lint:fix     # Biome linter with auto-fix
npm test             # Run tests
npm run dev          # Start dev server (hot reload)
```

## Project Wiki

See [`wiki/`](wiki/) for detailed documentation on architecture, connectors, configuration, database schema, API routes, and more.

## License

[Hippocratic License 3.0 (HL3)](LICENSE.md)
