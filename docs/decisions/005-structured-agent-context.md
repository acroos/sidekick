**status:** "accepted"

---

# Structured context system for agent steps

## Context and Problem Statement

Agent step quality depends heavily on the context provided to the LLM. Context can come from multiple sources at different lifecycle stages: static files in the repo, variables from the API call, and output from previously executed steps. How should we model and assemble this context?

## Decision Drivers

* Context quality is the biggest lever on agent effectiveness
* Sources are known at different times (authoring, submission, runtime)
* Must be validatable — config errors should fail fast, not produce silent garbage
* Must be truncatable — large step output (test suites, build logs) can eat the token budget

## Considered Options

* Template expressions in prompt (e.g., `{{ steps.test.stderr }}`, `{{ file "standards.md" }}`)
* Structured `context` block on agent steps with typed items
* Variables-only (flatten everything into string interpolation)

## Decision Outcome

Chosen option: "Structured context block", because it makes sources explicit, enables per-source validation at the right lifecycle stage, and supports per-item truncation.

### Consequences

* Good, because each context source has a clear type with validation rules appropriate to its lifecycle
* Good, because `max_lines` per item prevents large outputs from consuming the token budget
* Good, because the assembled context is deterministic and inspectable (useful for debugging agent behavior)
* Good, because adding new context types later (URLs, database queries) is a natural extension
* Bad, because it's more verbose than inline template expressions for simple cases
* Bad, because prompt authors can't control exactly where context appears within the prompt (it's always prepended)

## Validation Strategy

| Context type | Validated at | Failure mode |
|---|---|---|
| `file` | After clone step (file must exist in repo) | Workflow error, task fails |
| `variable` | Task submission (key must be in variables map) | API rejects submission (400) |
| `step_output` | Load time: step must exist. Runtime: step must have run | Load: workflow error. Runtime: omit item + warn |
