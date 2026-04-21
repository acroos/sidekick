**status:** "accepted"

---

# Use Docker as the MVP sandbox runtime

## Context and Problem Statement

Agents executing code autonomously must be sandboxed to prevent harm. What isolation technology should Sidekick use, given that it's an OSS tool meant to be self-hosted by diverse teams?

## Decision Drivers

* Must be safe enough for running untrusted LLM-generated code
* Must be accessible to teams self-hosting (low operational overhead)
* Must not lock us into a single runtime (stronger isolation needed later)
* Stripe's Minions uses VMs (Firecracker) for maximum isolation

## Considered Options

* Docker containers (with hardening)
* Docker + gVisor (runsc)
* Firecracker microVMs
* nsjail / bubblewrap

## Decision Outcome

Chosen option: "Docker containers (with hardening)", because it provides the lowest barrier to entry for self-hosted OSS while the `SandboxProvider` interface enables stronger runtimes later.

The upgrade path is incremental:
1. Docker (MVP) — good enough for trusted internal use
2. Docker + gVisor — one runtime flag change (`--runtime=runsc`), dramatically reduces kernel attack surface
3. Firecracker — full VM isolation, new provider implementation behind the same interface

### Consequences

* Good, because most teams already have Docker; no new infrastructure required
* Good, because gVisor upgrade is nearly zero-effort (runtime flag)
* Good, because the provider interface means swapping to Firecracker later is isolated to one package
* Bad, because Docker's shared-kernel model means a kernel exploit could escape the sandbox
* Bad, because we need to clearly document the hardening requirements (dropped caps, seccomp, network isolation, non-root) so users don't run with defaults

### Confirmation

The `SandboxProvider` interface must be defined before any Docker-specific code is written, ensuring the abstraction is not shaped by the implementation.

## Required Hardening (non-negotiable for MVP)

* `--cap-drop=ALL` — no Linux capabilities
* `--security-opt=no-new-privileges` — prevent privilege escalation
* Read-only rootfs with tmpfs for `/tmp` and `/workspace`
* `--network=none` by default (allowlist mode for restricted)
* `--memory`, `--cpus`, `--pids-limit` resource constraints
* Non-root user inside the container
* Seccomp profile restricting dangerous syscalls
