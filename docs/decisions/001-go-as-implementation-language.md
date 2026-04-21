**status:** "accepted"

---

# Use Go as the implementation language

## Context and Problem Statement

Sidekick is an orchestration platform that manages sandboxed containers, executes DAG-based workflows, streams LLM output, and exposes an HTTP API. It will be distributed as OSS for teams to self-host. Which language best fits this workload and distribution model?

## Decision Drivers

* Must distribute easily to diverse infrastructure (minimal runtime dependencies)
* Core workload is concurrent orchestration (containers, processes, HTTP), not complex domain modeling
* Docker SDK availability and maturity
* Developer velocity for the maintainer team
* Community fit for infrastructure/platform tooling

## Considered Options

* Go
* Ruby/Rails
* Rust

## Decision Outcome

Chosen option: "Go", because it best fits the orchestration workload and single-binary distribution requirement.

### Consequences

* Good, because single-binary distribution makes self-hosting trivial (no runtime, no dependency tree)
* Good, because goroutines and channels are a natural fit for managing concurrent sandbox lifecycles
* Good, because the Docker Engine API has a first-class Go SDK (`github.com/docker/docker/client`)
* Good, because the Go ecosystem is the standard for infrastructure tooling (Docker, K8s, Terraform)
* Bad, because more boilerplate than Ruby for API scaffolding and configuration handling
* Bad, because less expressive type system than Rust (no enums, no pattern matching)

## Pros and Cons of the Options

### Ruby/Rails

* Good, because fastest iteration speed for API development
* Good, because maintainer has strong Ruby preference/experience
* Bad, because long-running concurrent process management is not Ruby's strength
* Bad, because distribution requires Ruby runtime + gems on target hosts
* Bad, because the Rails request-response model fights against the streaming orchestration workload

### Rust

* Good, because strongest safety guarantees and performance
* Good, because single-binary distribution like Go
* Bad, because significantly slower development velocity
* Bad, because the Docker ecosystem tooling is less mature than Go's
* Bad, because the safety guarantees Rust provides are less valuable here (most danger is in the sandbox, not the orchestrator)
