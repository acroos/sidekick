**status:** "accepted"

---

# Use Server-Sent Events for real-time task streaming

## Context and Problem Statement

Tasks can run for 20+ minutes. UIs, Slack bots, and other frontends need real-time visibility into what a task is doing — which step is running, what the agent is thinking, what commands are executing. How should we deliver this output?

## Decision Drivers

* Must support real-time, low-latency streaming
* Must be consumable by browsers (web UI) and server-side clients (Slack bot) alike
* Must support reconnection and replay of missed events
* Traffic is unidirectional (server → client)
* Should be simple to implement and operate

## Considered Options

* Polling (GET /tasks/{id} on interval)
* Server-Sent Events (SSE)
* WebSockets
* Webhooks only

## Decision Outcome

Chosen option: "Server-Sent Events (SSE)", because it's the simplest streaming mechanism that meets all requirements for a unidirectional event stream.

### Consequences

* Good, because SSE works over standard HTTP — no protocol upgrade, no special infrastructure
* Good, because native browser support via `EventSource` API (no libraries needed for the web UI)
* Good, because `Last-Event-ID` is part of the SSE spec, giving us replay/reconnection for free
* Good, because event type filtering (`?types=`) keeps lightweight integrations simple
* Good, because events are persisted to SQLite, enabling replay of completed tasks for debugging
* Bad, because SSE is limited to ~6 concurrent connections per domain in some browsers (solvable with HTTP/2)
* Neutral, because polling endpoint (`GET /tasks/{id}`) is retained for simple clients that don't need streaming

## Event Taxonomy

Three levels of granularity, all on the same stream:

1. **Workflow progress** — `task.started`, `task.completed`, `step.started`, `step.completed`, `step.skipped`
2. **Step output** — `step.output` (stdout/stderr lines from deterministic steps)
3. **Agent activity** — `agent.thinking` (reasoning), `agent.action` (tool calls), `agent.action_result` (tool output), `agent.output` (text)

Including agent thinking/reasoning in the stream is a deliberate choice — it builds trust by making the agent's decision-making visible and is valuable for debugging.
