# Sidekick

An open-source platform for running autonomous coding agents in sandboxed environments. Teams self-host Sidekick on their own infrastructure and interact with it through an HTTP API that frontends (CLIs, Slack bots, web UIs, GitHub Apps) build on top of.

## Goals

- **Safety first** -- agents run in isolated Docker containers with no ambient access to production systems
- **Agentic + deterministic** -- combine LLM reasoning with scripted steps that always behave correctly
- **Bounded execution** -- every task has time and token budgets; agents cannot run forever
- **Simple API, composable frontends** -- the core is an HTTP service; UIs are separate concerns
- **Self-hosted OSS** -- single binary, minimal dependencies, easy to operate

## How It Works

You define **workflows** as YAML files containing a DAG of steps. Steps can be:

- **Deterministic** -- shell commands executed in a sandbox (clone, install, test, push)
- **Agent** -- Claude Code sessions that write and edit code autonomously

Sidekick orchestrates the workflow inside a hardened Docker container, streams real-time events via SSE, and exposes results through a REST API.

## Project Status

Pre-implementation. See [PROJECT_PLAN.md](PROJECT_PLAN.md) for the phased delivery roadmap and [docs/DESIGN.md](docs/DESIGN.md) for the system architecture.

## Development

### Prerequisites

- Go 1.22+
- Docker (for sandbox integration tests in later phases)
- [golangci-lint](https://golangci-lint.run/welcome/install/)

### Build

```bash
go build ./...
```

### Test

```bash
go test ./...
```

### Lint

```bash
golangci-lint run ./...
```

## Architecture

```
Frontends (CLI, Web UI, Slack, GitHub App)
  -> API Server (internal/api)
    -> Task Manager (internal/task)
      -> Workflow Engine (internal/workflow)
        -> Sandbox Provider (internal/sandbox) -- Docker containers
        -> Agent Runtime (internal/agent) -- Claude Code in sandbox
      -> LLM Proxy (internal/proxy) -- auth injection, token budgeting
```

See [docs/DESIGN.md](docs/DESIGN.md) for full details.

## License

TBD
