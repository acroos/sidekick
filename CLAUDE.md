# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Sidekick is an open-source platform for running autonomous coding agents in sandboxed environments. Teams self-host it on their own infrastructure and interact through an HTTP API that frontends (CLI, Slack bots, web UIs, GitHub Apps) build on top of.

**Current status:** Pre-implementation. The design docs and ADRs are complete; no Go code exists yet. See PROJECT_PLAN.md for the phased delivery roadmap.

## Build Commands

```bash
go build ./...
go test ./...
go test ./internal/sandbox/... -run TestExecStream   # single test/package
golangci-lint run ./...
```

## Architecture

The system is a layered orchestration platform:

```
Frontends (CLI, Web UI, Slack, GitHub App)
  → API Server (internal/api) — REST + SSE streaming
    → Task Manager (internal/task) — SQLite persistence, worker pool
      → Workflow Engine (internal/workflow) — YAML DAG parser/executor
        ├── Deterministic steps → Sandbox.Exec (shell commands)
        └── Agent steps → Agent Runtime (internal/agent) → Claude Code in sandbox
      → Sandbox Provider (internal/sandbox) — Docker (MVP), interface for gVisor/Firecracker
      → LLM Proxy (internal/proxy) — auth injection, token budgeting (API key never enters sandbox)
```

Key package layout:
- `cmd/sidekick/` — CLI entrypoint (server + client modes)
- `internal/api/` — HTTP handlers, middleware, SSE endpoints
- `internal/task/` — Task model, persistence, lifecycle
- `internal/workflow/` — YAML parser, DAG builder, execution engine
- `internal/sandbox/` — Sandbox provider interface + Docker implementation
- `internal/agent/` — Claude Code execution inside sandbox, stream-json output parsing
- `internal/event/` — Event bus, types, SQLite event store
- `internal/proxy/` — LLM reverse proxy
- `pkg/config/` — Server configuration
- `images/` — Sandbox Dockerfiles (base, language-specific)
- `workflows/templates/` — Built-in workflow templates

## Design Patterns

- **Provider interfaces** for extensibility (sandbox runtimes, LLM providers). Docker is the MVP sandbox; the interface enables future runtimes.
- **DAG execution** with conditional steps (`when:`), failure policies (`abort`/`continue`/`retry`), and step dependencies (`depends_on`).
- **Event-driven architecture** — all execution produces typed events (`task.*`, `step.*`, `agent.*`) streamed via an in-memory bus, persisted to SQLite, exposed over SSE with `Last-Event-ID` replay.
- **Three-source context system** for agent prompts: static files from the repo, API variables from task submission, and output from prior steps. Validated at different lifecycle stages.
- **Agent steps** run Claude Code via `claude -p "$PROMPT" --output-format stream-json --allowedTools ...` inside the sandbox. The agent runtime parses stream-json into `agent.*` events.

## Sandbox Security (Non-Negotiable)

All sandbox containers must be created with:
- `--cap-drop=ALL`
- `--security-opt=no-new-privileges`
- Read-only rootfs with tmpfs `/tmp` and `/workspace`
- `--network=none` by default (allowlist mode for `restricted`)
- Resource limits (CPU, memory, PIDs)
- Non-root user
- API key injected by LLM proxy, never present inside the container

## Decisions

Architecture Decision Records live in `docs/decisions/`. Reference these when working in related areas:

| ADR | When relevant |
|-----|--------------|
| 001 — Go as implementation language | Language or dependency choices |
| 002 — Docker as MVP sandbox runtime | Sandbox provider implementation, hardening |
| 003 — Claude Code as agent runtime | Agent step execution, prompt rendering |
| 004 — YAML workflow configuration | Workflow schema, parser, validation |
| 005 — Structured agent context system | Context assembly, variable interpolation |
| 006 — SSE event streaming | Event bus, SSE endpoint, replay |

New decisions must follow the template in `.claude/guides/DECISION_TEMPLATE.md`, be prefixed with a DateTime stamp, committed individually, and referenced in this file.

## Workflow YAML Schema

Workflows define a DAG of steps executed in a sandbox. Full schema and examples are in `docs/DESIGN.md`. Key types:
- `type: deterministic` — runs a shell command via `Sandbox.Exec`
- `type: agent` — runs Claude Code with assembled context and tool restrictions

Variables use `$VARIABLE` syntax and are interpolated from the task submission payload.
