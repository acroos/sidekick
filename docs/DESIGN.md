# Sidekick — High-Level Design

## System Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        Frontends                              │
│           (CLI, Web UI, Slack Bot, GitHub App)                │
└──────────────────────┬───────────────────────────────────────┘
                       │ HTTP (REST)
┌──────────────────────▼───────────────────────────────────────┐
│                      API Server                               │
│                                                               │
│  POST /tasks          — submit task                           │
│  GET  /tasks/{id}     — status + results                      │
│  GET  /tasks          — list tasks                            │
│  POST /tasks/{id}/cancel — cancel running task                │
│                                                               │
│  Auth: API key (X-Sidekick-Key header)                        │
└──────────────────────┬───────────────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────────────┐
│                    Task Manager                               │
│                                                               │
│  - Persists task state (SQLite)                               │
│  - Manages concurrency (bounded worker pool)                  │
│  - Dispatches tasks to the Workflow Engine                    │
│  - Fires webhooks on completion/failure                       │
└──────────────────────┬───────────────────────────────────────┘
                       │
┌──────────────────────▼───────────────────────────────────────┐
│                   Workflow Engine                              │
│                                                               │
│  - Parses YAML workflow definitions                           │
│  - Builds and validates DAG                                   │
│  - Executes steps in order, respecting dependencies           │
│  - Evaluates conditionals (when: expressions)                 │
│  - Handles failure policies (abort / continue / retry)        │
│  - Interpolates variables into step configs                   │
└────────┬─────────────────────────────────┬───────────────────┘
         │                                 │
         │ deterministic steps             │ agent steps
         │                                 │
┌────────▼────────┐              ┌─────────▼──────────────────┐
│  Sandbox.Exec   │              │    Agent Runtime            │
│  (run command)  │              │                             │
│                 │              │  - Renders prompt template   │
│                 │              │  - Runs Claude Code in       │
│                 │              │    sandbox (claude -p ...)   │
│                 │              │  - Streams output            │
│                 │              │  - Enforces timeout          │
└────────┬────────┘              └─────────┬──────────────────┘
         │                                 │
         │        ┌────────────────────────┘
         │        │
┌────────▼────────▼────────────────────────────────────────────┐
│                   Sandbox Provider                            │
│                                                               │
│  Interface:                                                   │
│    Create(cfg) → Sandbox                                      │
│    Destroy(id)                                                │
│                                                               │
│  Sandbox interface:                                           │
│    Exec(cmd) → ExecResult                                     │
│    CopyIn(src, dst)                                           │
│    CopyOut(src) → Reader                                      │
│    Status() → SandboxStatus                                   │
│                                                               │
│  Implementations: Docker (MVP), gVisor, Firecracker (future)  │
└──────────────────────────────────────────────────────────────┘
         │
         │ LLM traffic (from Claude Code inside sandbox)
         │
┌────────▼─────────────────────────────────────────────────────┐
│                    LLM Proxy                                  │
│                                                               │
│  - Listens on localhost, exposed to sandbox via Docker network │
│  - Injects API key (key never enters sandbox)                 │
│  - Enforces per-task token budget                             │
│  - Logs requests/responses                                    │
│  - Routes to Anthropic API (pluggable for future providers)   │
└──────────────────────────────────────────────────────────────┘
```

---

## Package Structure

```
sidekick/
├── cmd/
│   └── sidekick/           # CLI entrypoint (both server and client)
│       └── main.go
├── internal/
│   ├── api/                # HTTP API server, handlers, middleware (incl. SSE)
│   ├── task/               # Task model, persistence, lifecycle
│   ├── workflow/           # YAML parser, DAG builder, engine
│   ├── sandbox/            # Sandbox interfaces + Docker provider
│   ├── agent/              # Agent step runtime (Claude Code execution + event parsing)
│   ├── event/              # Event types, bus, and persistent store
│   └── proxy/              # LLM proxy (auth injection, budgeting)
├── pkg/
│   └── config/             # Sidekick server configuration
├── images/
│   ├── base/               # Base sandbox Dockerfile
│   └── node/               # Node.js sandbox Dockerfile (example)
├── workflows/
│   └── templates/          # Built-in workflow templates
├── docs/
│   ├── DESIGN.md           # This file
│   └── decisions/          # Architecture Decision Records
├── PROJECT_PLAN.md
├── IDEA.md
├── go.mod
└── go.sum
```

---

## Core Interfaces

### Sandbox Provider

```go
package sandbox

type Config struct {
    Image        string            // Container image to use
    Network      NetworkPolicy     // none, restricted, open
    AllowHosts   []string          // Egress allowlist (when restricted)
    CPULimit     float64           // CPU cores
    MemoryLimit  int64             // Bytes
    Timeout      time.Duration     // Hard kill deadline
    Env          map[string]string // Environment variables
    Mounts       []Mount           // Volume mounts (e.g., repo checkout)
}

type NetworkPolicy string

const (
    NetworkNone       NetworkPolicy = "none"
    NetworkRestricted NetworkPolicy = "restricted"
    NetworkOpen       NetworkPolicy = "open"
)

type Provider interface {
    Create(ctx context.Context, cfg Config) (Sandbox, error)
    Destroy(ctx context.Context, id string) error
}

type Sandbox interface {
    ID() string
    Exec(ctx context.Context, cmd Command) (*ExecResult, error)
    ExecStream(ctx context.Context, cmd Command) (*ExecStream, error)
    CopyIn(ctx context.Context, src, dst string) error
    CopyOut(ctx context.Context, src string) (io.Reader, error)
    Status() Status
}

// ExecStream provides real-time output from a running command.
// Used by the workflow engine to feed the event bus.
type ExecStream struct {
    Output <-chan OutputLine  // Multiplexed stdout/stderr, line by line
    Done   <-chan ExecResult  // Final result when process exits
}

type OutputLine struct {
    Stream string    // "stdout" or "stderr"
    Line   string
    Time   time.Time
}

type Command struct {
    Args    []string // e.g., ["npm", "test"]
    WorkDir string   // Working directory inside sandbox
    Env     map[string]string
    Timeout time.Duration
}

type ExecResult struct {
    ExitCode int
    Stdout   string
    Stderr   string
    Duration time.Duration
}

type Status string

const (
    StatusCreating Status = "creating"
    StatusReady    Status = "ready"
    StatusRunning  Status = "running"
    StatusStopped  Status = "stopped"
    StatusFailed   Status = "failed"
)
```

### Workflow

```go
package workflow

type Workflow struct {
    Name       string        `yaml:"name"`
    Timeout    time.Duration `yaml:"timeout"`
    MaxRetries int           `yaml:"max_retries"`
    Sandbox    SandboxConfig `yaml:"sandbox"`
    Steps      []Step        `yaml:"steps"`
}

type SandboxConfig struct {
    Image      string   `yaml:"image"`
    Network    string   `yaml:"network"`
    AllowHosts []string `yaml:"allow_hosts"`
}

type Step struct {
    Name         string        `yaml:"name"`
    Type         StepType      `yaml:"type"`          // deterministic | agent
    Run          string        `yaml:"run"`            // Shell command (deterministic)
    Prompt       string        `yaml:"prompt"`         // LLM prompt (agent)
    Context      []ContextItem `yaml:"context"`        // Assembled context for agent steps
    AllowedTools []string      `yaml:"allowed_tools"`  // Agent tool restrictions
    Timeout      time.Duration `yaml:"timeout"`
    OnFailure    FailPolicy    `yaml:"on_failure"`     // abort | continue | retry
    When         string        `yaml:"when"`           // Conditional expression
    DependsOn    []string      `yaml:"depends_on"`     // DAG dependencies
}

// ContextItem represents a single piece of context assembled into the agent prompt.
// Items are rendered in order and prepended to the prompt.
type ContextItem struct {
    Type     ContextType `yaml:"type"`       // file | variable | step_output
    Path     string      `yaml:"path"`       // File path relative to workspace (type: file)
    Key      string      `yaml:"key"`        // Variable name (type: variable)
    Step     string      `yaml:"step"`       // Step name (type: step_output)
    Output   string      `yaml:"output"`     // stdout | stderr (type: step_output)
    Label    string      `yaml:"label"`      // Section heading in assembled context
    MaxLines int         `yaml:"max_lines"`  // Truncate output to last N lines
}

type ContextType string

const (
    ContextFile       ContextType = "file"
    ContextVariable   ContextType = "variable"
    ContextStepOutput ContextType = "step_output"
)

type StepType string

const (
    StepDeterministic StepType = "deterministic"
    StepAgent         StepType = "agent"
)

type FailPolicy string

const (
    FailAbort    FailPolicy = "abort"
    FailContinue FailPolicy = "continue"
    FailRetry    FailPolicy = "retry"
)
```

### Task

```go
package task

type Task struct {
    ID          string
    WorkflowRef string                // Name or path of workflow definition
    Variables   map[string]string     // Interpolated into workflow steps
    Status      Status
    Steps       []StepResult
    CreatedAt   time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    Error       string                // Set if failed
}

type Status string

const (
    StatusPending   Status = "pending"
    StatusRunning   Status = "running"
    StatusSucceeded Status = "succeeded"
    StatusFailed    Status = "failed"
    StatusCancelled Status = "cancelled"
)

type StepResult struct {
    Name      string
    Status    Status
    ExitCode  int
    Stdout    string
    Stderr    string
    StartedAt time.Time
    Duration  time.Duration
    TokensUsed int           // For agent steps
}
```

### LLM Proxy

```go
package proxy

type Config struct {
    ListenAddr    string        // e.g., ":8089"
    APIKey        string        // Anthropic API key
    BaseURL       string        // Anthropic API base URL
    MaxTokens     int           // Per-task token budget
    RequestTimeout time.Duration
}

// The proxy is a reverse proxy that:
// 1. Listens for requests from Claude Code inside the sandbox
// 2. Injects the Authorization header (API key)
// 3. Tracks token usage per task
// 4. Rejects requests that exceed the budget
// 5. Logs all requests for observability
```

---

## API Design

### Submit a task

```
POST /tasks
Content-Type: application/json
X-Sidekick-Key: sk-sidekick-...

{
  "workflow": "fix-issue",
  "variables": {
    "REPO_URL": "https://github.com/acme/app.git",
    "BRANCH_NAME": "sidekick/fix-123",
    "TASK_DESCRIPTION": "Fix the null pointer exception in UserService.getProfile when the user has no avatar set. See issue #123.",
    "COMMIT_MESSAGE": "fix: handle null avatar in UserService.getProfile"
  },
  "webhook_url": "https://acme.com/hooks/sidekick"
}

→ 201 Created
{
  "id": "task_abc123",
  "status": "pending",
  "workflow": "fix-issue",
  "created_at": "2026-04-20T12:00:00Z"
}
```

### Check task status

```
GET /tasks/task_abc123
X-Sidekick-Key: sk-sidekick-...

→ 200 OK
{
  "id": "task_abc123",
  "status": "running",
  "workflow": "fix-issue",
  "created_at": "2026-04-20T12:00:00Z",
  "started_at": "2026-04-20T12:00:01Z",
  "steps": [
    {
      "name": "clone",
      "status": "succeeded",
      "duration_ms": 3200
    },
    {
      "name": "install",
      "status": "succeeded",
      "duration_ms": 15400
    },
    {
      "name": "solve",
      "status": "running",
      "tokens_used": 12840
    }
  ]
}
```

### Cancel a task

```
POST /tasks/task_abc123/cancel
X-Sidekick-Key: sk-sidekick-...

→ 200 OK
{
  "id": "task_abc123",
  "status": "cancelled"
}
```

### Webhook payload (on completion)

```json
{
  "event": "task.completed",
  "task": {
    "id": "task_abc123",
    "status": "succeeded",
    "workflow": "fix-issue",
    "steps": [ ... ],
    "completed_at": "2026-04-20T12:05:32Z",
    "total_tokens_used": 48320
  }
}
```

---

## Workflow YAML Schema

Full example workflow for the MVP use case:

```yaml
name: fix-issue
timeout: 30m
max_retries: 1

sandbox:
  image: sidekick-sandbox-node:20
  network: restricted
  allow_hosts:
    - registry.npmjs.org
    - github.com

steps:
  - name: clone
    type: deterministic
    run: |
      git clone $REPO_URL /workspace
      git -C /workspace checkout -b $BRANCH_NAME
    timeout: 2m
    on_failure: abort

  - name: install
    type: deterministic
    run: npm ci --prefix /workspace
    timeout: 5m
    on_failure: abort

  - name: solve
    type: agent
    context:
      # Static: repo-specific instructions checked into the codebase
      - type: file
        path: .sidekick/coding-standards.md
        label: "Coding standards"
      # API: passed in at task submission time
      - type: variable
        key: TASK_DESCRIPTION
        label: "Task"
      # Runtime: output from a prior deterministic step
      - type: step_output
        step: install
        output: stderr
        label: "Install warnings"
        max_lines: 30
    prompt: |
      You are working in /workspace.
      Solve the task described in the context above.
      Make the minimal change needed. Do not refactor unrelated code.
      Run tests to verify your fix before finishing.
    allowed_tools:
      - Edit
      - Read
      - Bash
      - Glob
      - Grep
    timeout: 20m

  - name: lint
    type: deterministic
    run: npm run lint --prefix /workspace -- --fix
    on_failure: continue

  - name: test
    type: deterministic
    run: npm test --prefix /workspace
    timeout: 5m
    on_failure: abort

  - name: push
    type: deterministic
    when: steps.test.status == 'succeeded'
    run: |
      git -C /workspace add -A
      git -C /workspace commit -m "$COMMIT_MESSAGE"
      git -C /workspace push origin $BRANCH_NAME
    timeout: 2m

  - name: open-pr
    type: deterministic
    when: steps.push.status == 'succeeded'
    depends_on: [push]
    run: |
      gh pr create \
        --repo $REPO_NAME \
        --head $BRANCH_NAME \
        --title "$COMMIT_MESSAGE" \
        --body "Automated fix by Sidekick. See original issue for context."
    timeout: 1m
```

---

## Streaming & Event System

### Overview

All task execution produces a stream of events that can be consumed in real-time via SSE or replayed from storage. This enables live UIs, Slack integrations, and debugging of completed tasks.

### Event Types

| Event | Emitted by | Description |
|---|---|---|
| `task.started` | Workflow engine | Task execution begins |
| `task.completed` | Workflow engine | Task finished (succeeded, failed, or cancelled) |
| `step.started` | Workflow engine | A step begins execution |
| `step.output` | Sandbox (ExecStream) | A line of stdout/stderr from a deterministic step |
| `step.completed` | Workflow engine | A step finished |
| `step.skipped` | Workflow engine | A step was skipped (conditional `when:` evaluated false) |
| `agent.thinking` | Agent runtime | Agent's reasoning/thinking text |
| `agent.action` | Agent runtime | Agent invoked a tool (Read, Edit, Bash, etc.) |
| `agent.action_result` | Agent runtime | Output from an agent tool invocation |
| `agent.output` | Agent runtime | Agent's text output (non-tool, non-thinking) |

### Event Schema

Every event shares a common envelope:

```go
package event

type Event struct {
    ID        int64           `json:"id"`          // Monotonic, per-task sequence number
    TaskID    string          `json:"task_id"`
    Type      string          `json:"type"`        // e.g., "step.output", "agent.action"
    Timestamp time.Time       `json:"timestamp"`
    Data      json.RawMessage `json:"data"`        // Type-specific payload
}

// Step events
type StepStarted struct {
    Step string `json:"step"`
}

type StepOutput struct {
    Step   string `json:"step"`
    Stream string `json:"stream"`   // "stdout" or "stderr"
    Line   string `json:"line"`
}

type StepCompleted struct {
    Step       string `json:"step"`
    Status     string `json:"status"`
    DurationMs int64  `json:"duration_ms"`
    TokensUsed int    `json:"tokens_used,omitempty"`
}

type StepSkipped struct {
    Step   string `json:"step"`
    Reason string `json:"reason"`   // e.g., "when condition evaluated false"
}

// Agent events
type AgentThinking struct {
    Step string `json:"step"`
    Text string `json:"text"`
}

type AgentAction struct {
    Step   string `json:"step"`
    Tool   string `json:"tool"`     // "Read", "Edit", "Bash", etc.
    Detail string `json:"detail"`   // e.g., file path, command
}

type AgentActionResult struct {
    Step   string `json:"step"`
    Tool   string `json:"tool"`
    Output string `json:"output"`
    MaxLines int  `json:"-"`        // Truncate in the event if too large
}

type AgentOutput struct {
    Step string `json:"step"`
    Text string `json:"text"`
}
```

### SSE Endpoint

```
GET /tasks/{id}/stream
Accept: text/event-stream
X-Sidekick-Key: sk-sidekick-...
Last-Event-ID: 42              (optional — replay from this point)

→ 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive

event: task.started
id: 1
data: {"timestamp":"2026-04-20T12:00:01Z"}

event: step.started
id: 2
data: {"step":"clone"}

event: step.output
id: 3
data: {"step":"clone","stream":"stdout","line":"Cloning into '/workspace'..."}

event: step.completed
id: 4
data: {"step":"clone","status":"succeeded","duration_ms":3200}

event: step.started
id: 5
data: {"step":"solve"}

event: agent.thinking
id: 6
data: {"step":"solve","text":"Let me look at the UserService to understand the null pointer issue..."}

event: agent.action
id: 7
data: {"step":"solve","tool":"Read","detail":"src/user/UserService.ts"}

event: agent.action_result
id: 8
data: {"step":"solve","tool":"Read","output":"export class UserService {\n  async getProfile(id: string) {\n    const user = await this.repo.find(id);\n    return {\n      name: user.name,\n      avatar: user.avatar.url  // NPE when avatar is null\n    };\n  }\n}"}

event: agent.thinking
id: 9
data: {"step":"solve","text":"The bug is on the line accessing user.avatar.url — avatar can be null. I need to add a null check."}

event: agent.action
id: 10
data: {"step":"solve","tool":"Edit","detail":"src/user/UserService.ts:6"}

event: agent.action
id: 11
data: {"step":"solve","tool":"Bash","detail":"npm test"}

event: agent.action_result
id: 12
data: {"step":"solve","tool":"Bash","output":"Tests: 42 passed, 0 failed"}

event: step.completed
id: 13
data: {"step":"solve","status":"succeeded","duration_ms":45000,"tokens_used":12840}

event: task.completed
id: 14
data: {"status":"succeeded","total_tokens_used":13200}
```

### Architecture

```
Sandbox (ExecStream)
    │ OutputLine per line of stdout/stderr
    ▼
Workflow Engine
    │ Tags with step name, parses agent JSON output
    ▼
Event Bus (in-memory, per-task)
    │
    ├──→ SSE endpoint (long-lived HTTP connection)
    ├──��� Webhook dispatcher (batches events, POSTs on completion)
    └──→ Event store (persists to SQLite for replay)
```

**Replay support:** Events are persisted with their monotonic ID. When an SSE client connects with `Last-Event-ID`, the endpoint replays all subsequent events from storage, then continues with live events. This means a client can reconnect mid-task without missing anything.

**Agent event parsing:** Claude Code supports `--output-format stream-json`, which emits structured JSON events for tool calls, thinking, and text output. The agent runtime parses these into `agent.*` events. If the output format is unavailable or unparseable, we fall back to streaming raw output as `agent.output` events.

### Event Filtering

Clients may not want every event type. The SSE endpoint supports an optional `types` query parameter:

```
GET /tasks/{id}/stream?types=step.started,step.completed,task.completed
```

This is useful for lightweight integrations (e.g., a Slack bot) that only care about high-level progress, not every line of stdout or agent reasoning.

---

## Agent Context System

Agent steps receive context from three sources, assembled in order and prepended to the prompt.

### Sources

| Type | Source | Validated | When |
|------|--------|-----------|------|
| `file` | Static file in the repo (e.g., `.sidekick/coding-standards.md`) | Must exist | Workflow load time (after clone step) |
| `variable` | Key from the `variables` map submitted via API | Must be present in API submission | Task submission time |
| `step_output` | stdout/stderr from a previously executed step | Step must exist in workflow; runtime check that it actually ran | Workflow load time + runtime |

### Assembly

Context items are rendered in order, each under its `label` as a markdown heading, then the prompt is appended at the end:

```
## Coding standards
<contents of .sidekick/coding-standards.md>

## Task
Fix the null pointer exception in UserService.getProfile when the user
has no avatar set. See issue #123.

## Install warnings
npm warn deprecated inflight@1.0.6: This module is not supported...

---

You are working in /workspace.
Solve the task described in the context above.
Make the minimal change needed. Do not refactor unrelated code.
Run tests to verify your fix before finishing.
```

### Validation Rules

1. **`file` references** — validated after the clone step executes. If the file does not exist in the repo, the workflow fails with a clear error before the agent step runs.
2. **`variable` references** — validated at task submission time. The API rejects submissions that are missing required variables referenced by any context item or step.
3. **`step_output` references** — the referenced step must exist in the workflow (validated at load time). At runtime, if the referenced step was skipped (due to a `when:` condition), the context item is omitted and a warning is logged.
4. **`max_lines`** — when set, output is truncated to the last N lines. Useful for large test or build output where only the tail (failures, summary) matters.

### Future Considerations

- **Token-aware truncation** — truncate based on estimated token count rather than line count
- **Semantic context** — automatically include files the agent is likely to need (e.g., files mentioned in an issue)
- **Context from external sources** — fetch context from URLs, APIs, or databases as a new context type

---

## Sandbox Lifecycle

```
1. Task submitted via API
       │
2. Workflow engine loads + validates YAML
       │
3. SandboxProvider.Create(config)
       │  → pulls/builds image if needed
       │  → creates container with hardening:
       │       --cap-drop=ALL
       │       --security-opt=no-new-privileges
       │       --read-only (+ tmpfs /tmp, /workspace)
       │       --network=none (or restricted)
       │       --memory, --cpus, --pids-limit
       │       non-root user
       │
4. For each step in DAG:
       │
       ├── deterministic: Sandbox.Exec(command)
       │       → captures exit code, stdout, stderr
       │       → applies on_failure policy
       │
       ├── agent: Agent runtime
       │       → renders prompt template
       │       → runs `claude -p "..." --allowedTools ...` via Sandbox.Exec
       │       → Claude Code talks to LLM Proxy (not directly to Anthropic)
       │       → proxy injects auth, tracks tokens
       │       → enforces timeout
       │
       └── evaluate when: / depends_on before each step
       │
5. Sandbox.CopyOut (if results needed on host)
       │
6. SandboxProvider.Destroy(id)
       │  → container removed
       │  → volumes cleaned up
       │  → API key was never in the container
       │
7. Task status updated, webhook fired
```

---

## Key Design Decisions

Recorded as ADRs in `docs/decisions/`:

| # | Decision | Rationale |
|---|----------|-----------|
| 001 | Go as implementation language | Single binary distribution, strong concurrency, Docker SDK, right fit for orchestration workload |
| 002 | Docker as MVP sandbox runtime | Lowest barrier for self-hosted OSS; provider interface enables gVisor/Firecracker later |
| 003 | Claude Code as agent runtime | Avoids rebuilding agent loop; proven coding agent; supports API key auth |
| 004 | YAML workflow configuration | Config-as-code; declarative DAGs with deterministic + agent steps; familiar to ops/platform teams |
| 005 | Structured agent context system | Three sources (static files, API variables, step output) assembled in order; validated at appropriate lifecycle stages |
| 006 | SSE for real-time event streaming | Unidirectional, standard HTTP, native browser support, replay via Last-Event-ID; includes agent thinking/reasoning |
