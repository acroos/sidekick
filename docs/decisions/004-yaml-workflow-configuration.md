**status:** "accepted"

---

# Use YAML for workflow configuration

## Context and Problem Statement

Admins need to define the sequence of steps (deterministic and agentic) that Sidekick executes for a given task type. How should these workflows be defined?

## Decision Drivers

* Must be config-as-code (version controlled, reviewable, diffable)
* Must support both deterministic and agent steps in the same workflow
* Must support conditional execution and failure policies
* Should be familiar to platform/ops teams
* Should be simple enough that non-engineers can read and modify workflows

## Considered Options

* YAML workflow files
* Go plugin system (compiled steps)
* JSON configuration
* Custom DSL

## Decision Outcome

Chosen option: "YAML workflow files", because it's declarative, familiar, and supports the full workflow model without requiring compilation.

### Consequences

* Good, because YAML is the de facto standard for infrastructure configuration (CI/CD, K8s, Docker Compose)
* Good, because workflows can live in the repo alongside the code they operate on
* Good, because declarative format makes validation and visualization straightforward
* Good, because conditional steps (`when:`) and failure policies (`on_failure:`) express naturally in YAML
* Bad, because complex conditional logic can become awkward in YAML (keep expressions simple)
* Bad, because YAML parsing has well-known pitfalls (implicit type coercion, indentation sensitivity)

### Confirmation

The workflow parser must validate schemas strictly and produce clear error messages with line numbers for invalid configurations.

## Schema Highlights

* `type: deterministic | agent` — clear separation of execution modes
* `when:` — conditional execution based on prior step status/output
* `on_failure: abort | continue | retry` — per-step failure handling
* `depends_on:` — explicit DAG dependencies for future parallel execution
* `timeout:` — per-step and workflow-level time bounds
* Variable interpolation (`$VAR`) from task submission inputs
