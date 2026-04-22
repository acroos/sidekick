# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Sidekick is an integration hub that connects productivity tools (Linear, Slack, etc.) to Claude Code GitHub Action runs. It receives webhooks from external tools, triggers claude-code-action workflows via GitHub Actions, and routes results back to the originating tools.

**Current status:** Phase 1 (Foundation) complete. See `wiki/roadmap.md` for the full roadmap.

## Build Commands

```bash
npm run build                # TypeScript compilation
npm run typecheck            # Type checking without emit
npm run lint                 # Biome linter
npm run lint:fix             # Biome linter with auto-fix
npm test                     # Run tests (vitest)
npm run test:watch           # Run tests in watch mode
npm run dev                  # Start dev server (tsx watch)
npm run db:generate          # Generate Drizzle migrations
npm run db:migrate           # Run Drizzle migrations
```

## Pre-push checks

Run all of these before pushing. CI will fail if any of them fail.

```bash
npm run lint
npm run typecheck
npm test
```

## Architecture

```
External tools (Linear, Slack, etc.)
  → Webhook endpoints (src/routes/)
    → Connector trigger handlers — parse events, extract context
      → GitHub Actions dispatch (workflow_dispatch via Octokit)
        → claude-code-action runs in GitHub infrastructure
      → Run state tracking (Postgres via Drizzle)
    → GitHub completion webhooks
      → Notification delivery — route results back to originating tools
```

Key source layout:
- `src/app.ts` — Hono app setup and route mounting
- `src/index.ts` — Node.js server entry point (local dev)
- `api/index.ts` — Vercel serverless entry point
- `src/config/` — YAML config loader, schema validation (zod)
- `src/db/` — Drizzle ORM schema (runs, run_notifications), database client
- `src/routes/` — HTTP route handlers

## Technology

- **TypeScript** + **Hono** (web framework)
- **Vercel** (deployment target)
- **Postgres** + **Drizzle ORM** (persistence)
- **YAML** config with `${VAR}` env var interpolation
- **Biome** (linting/formatting)
- **Vitest** (testing)

## Core Concepts

- **Connectors** — Integrations with external tools (Linear, Slack, etc.). Each provides trigger and/or notification capabilities. Credentials defined once in the `connectors` registry.
- **Automations** — The unit of config that pairs one trigger with zero or more notifications. Each automation is independent.
- **Runs** — A triggered workflow execution. Tracked in Postgres with status lifecycle: triggered → queued → running → completed/failed.
- **Notifications** — Per-target result delivery. Each notification tracks its own status (pending/sent/failed) for retry handling.

## Wiki

Detailed documentation lives in `wiki/`. See `wiki/index.md` for a catalog of all pages. When building new features or changing architecture, update the relevant wiki pages to reflect the current state.
