# Sidekick — Setup & Usage Guide

## Prerequisites

- **Go 1.22+** — [install](https://go.dev/dl/)
- **Docker** — running and accessible to your user (`docker ps` should work)
- **Anthropic API key** — from [console.anthropic.com](https://console.anthropic.com/)

## Build

```bash
# From the repo root
go build -o sidekick ./cmd/sidekick/
```

This produces a single `sidekick` binary.

## Build the sandbox image

Sidekick runs all task work inside Docker containers. You need to build the base image first:

```bash
docker build -t sidekick-sandbox-base:latest images/base/
```

The base image is Ubuntu 24.04 with git, curl, and jq. It runs as a non-root user with a read-only root filesystem. You can build your own images on top of it for language-specific tooling.

## Configuration

Sidekick is configured entirely through environment variables. The server requires two keys; everything else has sensible defaults.

### Required

| Variable | Description |
|---|---|
| `SIDEKICK_API_KEY` | API key for authenticating with the Sidekick server. Pick any secret string. Used by both the server and CLI. |
| `ANTHROPIC_API_KEY` | Your Anthropic API key. Used by the LLM proxy to authenticate requests from Claude Code inside the sandbox. |

### Optional (server)

| Variable | Default | Description |
|---|---|---|
| `SIDEKICK_LISTEN_ADDR` | `:8080` | HTTP server listen address |
| `SIDEKICK_PROXY_ADDR` | `:8089` | LLM proxy listen address |
| `SIDEKICK_WORKFLOW_DIR` | `workflows/templates` | Directory containing workflow YAML files |
| `SIDEKICK_DB_PATH` | `sidekick.db` | SQLite database file path |
| `SIDEKICK_MAX_CONCURRENT_TASKS` | `4` | Maximum tasks running in parallel |
| `SIDEKICK_DEFAULT_TOKEN_BUDGET` | `0` (unlimited) | Default per-task token limit |
| `SIDEKICK_LOG_FORMAT` | `text` | Log format: `text` or `json` |
| `SIDEKICK_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `SIDEKICK_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown deadline |

### Optional (CLI client)

| Variable | Default | Description |
|---|---|---|
| `SIDEKICK_SERVER_URL` | `http://localhost:8080` | Sidekick server URL (for remote servers) |
| `SIDEKICK_API_KEY` | (required) | Same key the server is configured with |

## Start the server

```bash
export SIDEKICK_API_KEY="sk-my-secret-key"
export ANTHROPIC_API_KEY="sk-ant-..."

./sidekick server
```

You should see:

```
time=... level=INFO msg="LLM proxy started" addr=:8089
time=... level=INFO msg="Sidekick API server started" addr=:8080 version=dev
```

The server is now ready. It persists tasks and events to `sidekick.db` (SQLite) in the working directory.

## Using the CLI

The CLI reads `SIDEKICK_API_KEY` and `SIDEKICK_SERVER_URL` from the environment. If you're running the server locally, only the API key is needed.

```bash
export SIDEKICK_API_KEY="sk-my-secret-key"
```

### Submit a task

```bash
./sidekick submit \
  --workflow fix-issue \
  --var REPO_URL=https://github.com/yourorg/yourrepo.git \
  --var BRANCH_NAME=sidekick/fix-123 \
  --var TASK_DESCRIPTION="Fix the null pointer in UserService.getProfile when avatar is null" \
  --var COMMIT_MESSAGE="fix: handle null avatar in UserService.getProfile"
```

Output:

```
Task created: task_a1b2c3d4e5f6
Status: pending
```

Add `--follow` to stream events in real time after submission:

```bash
./sidekick submit --workflow fix-issue --var ... --follow
```

### Check task status

```bash
# Single task
./sidekick status task_a1b2c3d4e5f6

# List all tasks
./sidekick status
```

### Stream task logs

```bash
./sidekick logs task_a1b2c3d4e5f6
```

This connects to the server's SSE endpoint and prints color-coded events as they happen: step starts, stdout/stderr output, agent thinking, tool calls, and task completion.

Filter to specific event types:

```bash
./sidekick logs task_a1b2c3d4e5f6 --types step.started,step.completed,task.completed
```

## Using the web UI

Open `http://localhost:8080` in a browser. Enter your API key in the top-right input field (it's stored in your browser's session storage).

The UI has three views:

- **Tasks** — list of all tasks with status, workflow, and creation time. Click a task ID to see details.
- **Task detail** — task metadata, step results table, and a live event log that streams via SSE.
- **Submit** — form to submit a new task with workflow name, key-value variables, and optional webhook URL.

## Using the API directly

All API endpoints require the `X-Sidekick-Key` header (or `Authorization: Bearer <key>`).

### Submit a task

```bash
curl -X POST http://localhost:8080/tasks \
  -H "X-Sidekick-Key: sk-my-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "workflow": "fix-issue",
    "variables": {
      "REPO_URL": "https://github.com/yourorg/yourrepo.git",
      "BRANCH_NAME": "sidekick/fix-123",
      "TASK_DESCRIPTION": "Fix the bug",
      "COMMIT_MESSAGE": "fix: the bug"
    },
    "webhook_url": "https://example.com/hooks/sidekick"
  }'
```

### Get task status

```bash
curl http://localhost:8080/tasks/task_a1b2c3d4e5f6 \
  -H "X-Sidekick-Key: sk-my-secret-key"
```

### List tasks

```bash
# All tasks
curl http://localhost:8080/tasks -H "X-Sidekick-Key: sk-my-secret-key"

# With filters
curl "http://localhost:8080/tasks?status=running&workflow=fix-issue&limit=10" \
  -H "X-Sidekick-Key: sk-my-secret-key"
```

### Cancel a task

```bash
curl -X POST http://localhost:8080/tasks/task_a1b2c3d4e5f6/cancel \
  -H "X-Sidekick-Key: sk-my-secret-key"
```

### Stream events (SSE)

```bash
curl -N http://localhost:8080/tasks/task_a1b2c3d4e5f6/stream \
  -H "X-Sidekick-Key: sk-my-secret-key" \
  -H "Accept: text/event-stream"
```

Events are Server-Sent Events formatted as:

```
event: step.started
id: 1
data: {"step":"clone"}

event: step.output
id: 2
data: {"step":"clone","stream":"stdout","line":"Cloning into '/workspace/repo'..."}
```

Reconnect with `Last-Event-ID` to replay missed events. Filter with `?types=step.started,task.completed`.

## Built-in workflows

### fix-issue

Clones a repo, runs Claude Code to solve a task, tests the fix, pushes, and opens a PR.

**Required variables:**

| Variable | Description |
|---|---|
| `REPO_URL` | Git clone URL |
| `BRANCH_NAME` | Branch to create for the fix |
| `TASK_DESCRIPTION` | What the agent should do |
| `COMMIT_MESSAGE` | Commit and PR title |

**Steps:** clone, solve (agent), test, push, open-pr

The push and open-pr steps are conditional — they only run if tests pass.

### code-review

Clones a repo, generates a diff, and has Claude Code review the changes.

**Required variables:**

| Variable | Description |
|---|---|
| `REPO_URL` | Git clone URL |
| `PR_BRANCH` | Branch being reviewed |
| `BASE_BRANCH` | Branch to diff against (e.g., `main`) |
| `REVIEW_INSTRUCTIONS` | What to focus on in the review |

**Steps:** clone, diff, review (agent)

This is a read-only workflow — it produces review feedback but doesn't modify the repo.

## Writing custom workflows

Workflows are YAML files placed in the workflow directory (default: `workflows/templates/`). The filename without `.yaml` is the workflow name used in the API.

### Minimal example

```yaml
name: hello
timeout: 5m

sandbox:
  image: sidekick-sandbox-base:latest

steps:
  - name: greet
    type: deterministic
    run: echo "Hello from Sidekick"
```

### Step types

**Deterministic** — runs a shell command:

```yaml
- name: install
  type: deterministic
  run: npm ci --prefix /workspace
  timeout: 5m
  on_failure: abort    # abort | continue | retry
```

**Agent** — runs Claude Code with a prompt:

```yaml
- name: solve
  type: agent
  context:
    - type: variable
      key: TASK_DESCRIPTION
      label: "Task"
    - type: step_output
      step: install
      output: stderr
      label: "Install warnings"
      max_lines: 30
  prompt: |
    You are working in /workspace.
    Solve the task described above.
  allowed_tools:
    - Edit
    - Read
    - Bash
    - Glob
    - Grep
  timeout: 20m
```

### Variables

Use `$VARIABLE` or `${VARIABLE}` syntax in `run` and `prompt` fields. Variables are passed in the task submission payload.

### Dependencies and conditionals

```yaml
- name: push
  depends_on: [test]
  when: steps.test.status == 'succeeded'
  run: git push origin my-branch
```

### Agent context sources

| Type | Fields | Description |
|---|---|---|
| `variable` | `key`, `label` | Value from the task submission variables |
| `file` | `path`, `label` | File from the workspace (read after clone) |
| `step_output` | `step`, `output` (stdout/stderr), `label`, `max_lines` | Output from a previous step |

Context items are assembled in order under markdown headings and prepended to the prompt.

### Sandbox configuration

```yaml
sandbox:
  image: sidekick-sandbox-base:latest   # Docker image
  network: restricted                    # none (default) | restricted | open
  allow_hosts:                           # Egress allowlist (restricted mode)
    - github.com
    - registry.npmjs.org
```

All containers run with dropped capabilities, no-new-privileges, read-only rootfs, non-root user, and resource limits.

### Failure policies

| Policy | Behavior |
|---|---|
| `abort` (default) | Stop the workflow on failure |
| `continue` | Log the failure and move to the next step |
| `retry` | Retry the step up to `max_retries` times |

## Event types

| Event | Description |
|---|---|
| `task.started` | Task execution begins |
| `task.completed` | Task finished (status + token total) |
| `step.started` | A step begins |
| `step.output` | A line of stdout/stderr from a step |
| `step.completed` | A step finished (status + duration) |
| `step.skipped` | A step was skipped (when condition false) |
| `agent.thinking` | Agent's reasoning text |
| `agent.action` | Agent invoked a tool |
| `agent.action_result` | Output from agent tool call |
| `agent.output` | Agent's text output |

## Webhooks

If you include `webhook_url` in the task submission, Sidekick will POST a JSON payload when the task completes:

```json
{
  "event": "task.completed",
  "task": {
    "id": "task_a1b2c3d4e5f6",
    "status": "succeeded",
    "workflow": "fix-issue",
    "steps": [...],
    "completed_at": "2026-04-20T12:05:32Z",
    "total_tokens_used": 48320
  }
}
```
