**status:** "accepted"

---

# Use Claude Code as the agent runtime inside sandboxes

## Context and Problem Statement

Sidekick's agent steps need an LLM-powered coding agent that can edit files, run commands, and reason about code inside the sandbox. Should we build our own agent loop against the Claude API, or use Claude Code (Anthropic's existing coding agent) as the runtime?

## Decision Drivers

* Building a good agent loop (context management, tool execution, error recovery) is a significant engineering investment
* Sidekick's core value is orchestration and sandboxing, not the agent loop itself
* Must support API key authentication for headless server use
* Should not prevent supporting other LLM providers in the future

## Considered Options

* Build custom agent loop (raw Claude API + tool use)
* Use Claude Code as agent runtime (`claude -p` headless mode)
* Use Anthropic Agent SDK

## Decision Outcome

Chosen option: "Claude Code as agent runtime", because it avoids rebuilding a proven coding agent and lets us focus on orchestration.

### Consequences

* Good, because Claude Code already handles file editing, command execution, context management, and error recovery
* Good, because `claude -p` with `--allowedTools` gives us headless execution with tool restrictions
* Good, because dramatically less code to write and maintain for the agent step
* Bad, because creates a dependency on Claude Code being installed in sandbox images
* Bad, because less fine-grained control over the agent's behavior than a custom loop
* Bad, because supporting non-Claude LLM providers for agent steps will require building Option A (custom agent loop) as an alternative runtime later

### Confirmation

The `type: agent` step configuration must include a `runtime` field (defaulting to `claude-code`) so that alternative agent runtimes can be added later without changing the workflow schema.

## Auth Architecture

The API key must NOT be injected into the sandbox. Instead:
1. Sidekick runs an LLM proxy on a local port
2. The sandbox is configured to route Claude Code's API traffic through the proxy
3. The proxy injects the API key and enforces token budgets
4. If the sandbox is compromised, the attacker has no API key and no network egress
