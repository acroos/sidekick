# Sidekick — Project Plan

## Vision

An open-source platform for running autonomous coding agents in sandboxed environments. Teams self-host Sidekick on their own infrastructure and interact with it through a simple API that frontends (Slack bots, web UIs, GitHub Apps, CLIs) can build on top of.

## Principles

1. **Safety first** — agents run in isolated sandboxes with no ambient access to production systems
2. **Agentic + deterministic** — LLM reasoning is powerful but unreliable; combine it with scripted steps that always behave correctly
3. **Bounded execution** — every task has time and token budgets; agents cannot run forever
4. **Simple API, composable frontends** — the core is a service with an HTTP API; UIs are separate concerns
5. **Self-hosted OSS** — distribute as a single binary, minimal dependencies, easy to operate

---

## Phase 0: Project Foundation

**Goal:** Buildable Go project with CI, project structure, and developer tooling.

- Initialize Go module (`github.com/austinroos/sidekick`)
- Establish package structure (see Design doc)
- Set up linting (golangci-lint), testing, and formatting
- CI pipeline (GitHub Actions): lint, test, build
- CLAUDE.md / AGENTS.md for agent-assisted development
- Initial README with project description and goals

**Deliverable:** `go build ./...` and `go test ./...` pass in CI.

---

## Phase 1: Sandbox Provider

**Goal:** Create, execute commands in, and destroy Docker-based sandboxes programmatically.

- Define `SandboxProvider` and `Sandbox` interfaces
- Docker provider implementation using the Docker Engine API
  - Container creation with hardening (dropped capabilities, seccomp, no-new-privileges, non-root user)
  - Read-only rootfs with tmpfs scratch space
  - Network isolation (no egress by default)
  - Resource limits (CPU, memory, PIDs)
  - Timeout enforcement (kill container after deadline)
- `CopyIn` / `CopyOut` for moving files between host and sandbox
- `Exec` for running commands and collecting output
- `ExecStream` for running commands with real-time output streaming (stdout/stderr line by line)
- Build a base sandbox image (`sidekick-sandbox-base`) with common tools (git, curl)
- Integration tests against a real Docker daemon

**Deliverable:** A Go library that can spin up a hardened container, run commands in it (batch or streaming), collect output, and tear it down.

---

## Phase 2: Workflow Engine

**Goal:** Parse YAML workflow definitions and execute DAGs of deterministic steps in a sandbox.

- YAML workflow schema definition and parser
- Workflow validation (cycles, missing refs, schema errors)
- Variable interpolation (`$REPO_URL`, `$TASK_DESCRIPTION`, etc.)
- Step executor for `type: deterministic` steps
  - Runs commands in the sandbox via `Sandbox.ExecStream`
  - Captures exit code, stdout, stderr
  - Emits `step.output` events in real-time
- Step failure policies (`on_failure: abort | continue | retry`)
- Conditional steps (`when:` expressions evaluated against prior step status/output)
- Workflow-level and step-level timeouts
- Workflow execution status tracking (pending → running → succeeded/failed)
- Event bus (in-memory, per-task) with event persistence to SQLite for replay
- Context system: assemble static files, API variables, and prior step output into agent prompts

**Deliverable:** Can load a YAML workflow, run a sequence of shell commands in a sandbox, handle failures, emit real-time events, and report results. No agent steps yet.

---

## Phase 3: Agent Integration

**Goal:** Agent steps in the DAG can invoke Claude Code inside the sandbox to perform coding tasks.

- LLM proxy service
  - Runs alongside the orchestrator, listens on a local port
  - Sandbox containers route LLM traffic through it (no direct API access)
  - Proxy injects auth (API key never enters the sandbox)
  - Enforces per-task token budgets
  - Logs all LLM requests/responses for observability
- Claude Code agent runtime
  - Install Claude Code in sandbox images
  - Execute `claude -p "$PROMPT" --output-format stream-json --allowedTools ...` inside the sandbox
  - Configure Claude Code to use the LLM proxy endpoint
  - Parse Claude Code's structured JSON output into `agent.*` events (thinking, tool calls, tool results, text)
  - Stream all agent events through the event bus in real-time
- Agent step executor for `type: agent` steps
  - Context assembly (static files, API variables, prior step output) prepended to prompt
  - Tool restrictions (configurable per step)
  - Timeout enforcement
- LLM provider abstraction (interface for future providers beyond Claude)

**Deliverable:** A workflow can include an agent step that uses Claude Code to write/edit code in the sandbox, with auth proxied, token-budgeted, and full agent activity (including thinking) streamed as events.

---

## Phase 4: API & Task Management

**Goal:** HTTP API for submitting tasks, checking status, and receiving results.

- Task model: ID, workflow ref, input variables, status, step results, timestamps
- Task persistence (SQLite for single-node MVP, interface for future backends)
- API endpoints:
  - `POST /tasks` — submit a new task (workflow + variables)
  - `GET /tasks/{id}` — get task status and step results
  - `GET /tasks` — list tasks (with filters)
  - `POST /tasks/{id}/cancel` — cancel a running task
  - `GET /tasks/{id}/stream` — SSE endpoint for real-time event streaming
- SSE streaming with `Last-Event-ID` support for reconnection/replay
- Event type filtering via `?types=` query parameter
- API authentication (API key or bearer token for Sidekick itself)
- Webhook notifications on task completion/failure
- Concurrent task execution with configurable parallelism limits

**Deliverable:** A running HTTP server that accepts task submissions, orchestrates them through the workflow engine, streams real-time events via SSE, and reports results via API.

---

## Phase 5: MVP Polish & Demo

**Goal:** Usable end-to-end system with a demo frontend.

- CLI client (`sidekick submit`, `sidekick status`, `sidekick logs`)
- Simple web UI (task submission form, status dashboard, log viewer)
- Built-in workflow templates for common patterns:
  - `fix-issue` — clone, agent solves issue, test, open PR
  - `code-review` — clone, agent reviews diff, posts comments
- Network allowlist configuration (per-sandbox egress rules for package registries)
- Structured logging and basic observability
- End-to-end documentation: setup guide, workflow authoring, API reference
- First tagged release

**Deliverable:** A user can `sidekick submit --workflow fix-issue --repo ... --issue ...` and get a PR opened autonomously.

---

## Future (Post-MVP)

These are explicitly out of scope for the initial build but should be designed for:

- **Additional sandbox runtimes** — gVisor, Firecracker (via the provider interface)
- **Additional LLM providers** — OpenAI, local models (via the provider interface and proxy)
- **Multi-node execution** — distribute tasks across a cluster
- **Non-coding agents** — ops tasks, data pipelines, infrastructure changes
- **GitHub App integration** — trigger workflows from issue comments, PR events
- **Workflow marketplace** — shareable community workflow templates
- **Audit trail** — full record of every action an agent took, for compliance
