# Sidekick Wiki

A structured knowledge base for the Sidekick project — an integration hub connecting productivity tools to Claude Code GitHub Action runs.

This wiki is maintained by LLM agents (primarily Claude Code) and humans collaboratively. When new features are built, concepts change, or architecture evolves, the relevant pages should be updated to reflect the current state. The [index](#pages) below is the entry point — read it to find the right page, then drill into details.

## Pages

### Core Concepts

- [Overview](overview.md) — What Sidekick is, why it exists, and the problem it solves
- [Architecture](architecture.md) — System design, data flow, and how the pieces fit together
- [Connectors](connectors.md) — External tool integrations (Linear, Slack, etc.) and how to build new ones
- [Automations](automations.md) — Configuration that pairs triggers with notifications
- [Runs](runs.md) — Workflow execution lifecycle and state machine
- [Notifications](notifications.md) — Result routing, delivery, and retry handling

### Technical Reference

- [Configuration](configuration.md) — `sidekick.yaml` format, environment variables, and validation
- [Database](database.md) — Postgres schema, tables, and Drizzle ORM usage
- [API](api.md) — HTTP routes, request/response formats, and webhook handling
- [Deployment](deployment.md) — Vercel setup, serverless entry point, and environment config

### Contributing

- [Development](development.md) — Local setup, testing, tooling, and project structure

### Direction

- [Roadmap](roadmap.md) — Current status, planned phases, and future vision

## Conventions

- Pages describe the **current** state of the system. Future plans belong in [Roadmap](roadmap.md) and are clearly marked.
- Cross-reference between pages using relative markdown links.
- When adding a new page, add it to this index under the appropriate section.
- Keep pages focused — one concept per page. If a section grows beyond what's needed, split it.
