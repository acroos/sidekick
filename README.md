# Sidekick

An open-source platform for running autonomous coding agents in sandboxed environments. Teams self-host Sidekick on their own infrastructure and interact through an HTTP API that frontends (CLIs, Slack bots, web UIs, GitHub Apps) build on top of.

> **Status: v0** — All core packages are implemented and working end-to-end. See [PROJECT_PLAN.md](PROJECT_PLAN.md) for the roadmap.

## What It Does

You define **workflows** as YAML files containing a DAG of steps:

- **Deterministic steps** run shell commands in a sandbox (clone, install, test, push)
- **Agent steps** run Claude Code sessions that write and edit code autonomously

Sidekick orchestrates the workflow inside a hardened Docker container, streams real-time events via SSE, and exposes results through a REST API.

```bash
# Submit a task and watch it run
sidekick submit --workflow fix-issue --var REPO_URL=... --var ISSUE_NUMBER=42
sidekick logs --follow <task-id>
```

## Design Goals

- **Safety first** — agents run in locked-down Docker containers with no ambient access to production systems
- **Agentic + deterministic** — combine LLM reasoning with scripted steps that always behave correctly
- **Bounded execution** — every task has time and token budgets; agents cannot run forever
- **Simple API, composable frontends** — the core is an HTTP service; UIs are separate concerns
- **Self-hosted** — single binary, minimal dependencies, easy to operate

## Architecture

Sidekick is a layered system. Frontends talk to the API server, which manages tasks and dispatches them to the workflow engine. The workflow engine runs each step inside an isolated sandbox.

```
┌─────────────────────────────────────────────────────────┐
│  Frontends:  CLI · Web UI · Slack Bot · GitHub App      │
└────────────────────────┬────────────────────────────────┘
                         │ REST + SSE
┌────────────────────────▼────────────────────────────────┐
│  API Server  (internal/api)                             │
│  Routes, auth, SSE streaming with replay                │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│  Task Manager  (internal/task)                          │
│  SQLite persistence, worker pool, lifecycle             │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│  Workflow Engine  (internal/workflow)                    │
│  YAML parser, DAG builder, conditional execution        │
└──────────┬──────────────────────────┬───────────────────┘
           │                          │
   deterministic steps           agent steps
           │                          │
           │              ┌───────────▼───────────────┐
           │              │  Agent Runtime             │
           │              │  (internal/agent)          │
           │              │  Runs Claude Code inside   │
           │              │  sandbox, parses output    │
           │              │  into streamed events      │
           │              └───────────┬───────────────┘
           │                          │
┌──────────▼──────────────────────────▼───────────────────┐
│  Sandbox Provider  (internal/sandbox)                   │
│  Docker containers with hardened security:              │
│  dropped caps · no-new-privileges · read-only rootfs    │
│  network isolation · resource limits · non-root user    │
└──────────────────────────┬──────────────────────────────┘
                           │ LLM traffic from agent steps
┌──────────────────────────▼──────────────────────────────┐
│  LLM Proxy  (internal/proxy)                            │
│  Injects API key (never enters sandbox)                 │
│  Enforces per-task token budgets                        │
└─────────────────────────────────────────────────────────┘
```

Events flow in the opposite direction — every layer emits typed events (`task.*`, `step.*`, `agent.*`) that propagate up through an in-memory event bus, get persisted to SQLite, and stream to clients over SSE with `Last-Event-ID` replay support.

See [docs/DESIGN.md](docs/DESIGN.md) for the full design document.

## Getting Started

### Prerequisites

- Go 1.22+
- Docker
- [golangci-lint](https://golangci-lint.run/welcome/install/)

### Build & Run

```bash
go build ./...          # compile everything
go test ./...           # run tests
golangci-lint run ./... # lint
```

## License

[Hippocratic License 3.0 (HL3)](LICENSE.md). See the license file for details.
