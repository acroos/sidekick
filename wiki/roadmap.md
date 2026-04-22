# Roadmap

## Current Status

The foundation is built and working. The Linear connector is implemented end-to-end: label an issue → Claude Code runs in GitHub Actions → results posted back.

**What's implemented:**
- Hono app deployed to Vercel with Postgres connectivity
- YAML config loading with `${VAR}` interpolation and Zod validation
- Linear webhook receiver with HMAC signature verification
- GitHub webhook receiver for `workflow_run` events
- Linear issue context extraction (title, description, labels, comments)
- GitHub Actions workflow dispatch via Octokit
- Run state tracking with enforced state machine transitions
- Notification delivery to Linear (comments, status updates)
- Run list/detail API endpoints
- Structured JSON logging and error handling middleware

## Phases

### Phase 1: Foundation — Complete

Deployable Hono app on Vercel with Postgres, config loading, and the Linear connector working end-to-end.

### Phase 2: Polish and Robustness

Production-ready for personal/team use.

- Error handling and retry logic for notification delivery
- Logging and observability improvements
- Configuration validation at startup (connector references, duplicate names)
- Setup documentation and Linear integration walkthrough

### Phase 3: Slack Connector

Trigger from Slack messages/reactions, report back in threads.

- Slack webhook handler with signature verification
- Trigger types: emoji reaction (`on_reaction`), mention, message keyword
- Context extraction from messages and threads
- Notification: thread replies with run results
- Useful for incident response and ad-hoc agent requests

### Phase 4: Additional Connectors

- **PagerDuty** — Trigger from alerts, auto-investigate and post findings
- **Discord** — Community/open-source use case
- **Generic webhook** — Catch-all for tools that can send webhooks but don't have a dedicated connector

## Future Vision

### Autonomous Intake

Extend Sidekick from a tool you invoke into a teammate that works alongside you. The system monitors project management tools, identifies tasks it can solve, and autonomously picks them up.

Key ideas:
- **Selection engine** with tiered confidence: deterministic rules (labeled issues), heuristics (complexity scoring), and LLM evaluation (read the issue, assess solvability)
- **Autonomy policy** mapping confidence to action: high confidence → auto-execute, medium → draft PR for review, low → post analysis and hand back
- **Feedback loop** where PR outcomes (merged, revised, rejected) calibrate future confidence

### PR Iteration

Close the feedback loop on agent-generated PRs. When a reviewer leaves comments, the agent picks them up, makes targeted changes, and pushes — behaving like a teammate who responds to code review.

Key ideas:
- **Review listener** filtering to Sidekick-generated PRs only (avoid self-triggering)
- **Context reconstruction** loading the original task context + current diff + review comments
- **Scoped changes** addressing only the review feedback, not unsolicited improvements
- **Stopping conditions** including max rounds, reviewer takeover, and fundamental disagreements

### Incident Triage

When an alert fires, Sidekick automatically investigates — correlating recent deployments, reading error logs, checking metrics, and posting a structured summary to the incident channel.

Key ideas:
- **Alert normalization** across PagerDuty, Datadog, Sentry into a common format
- **Deterministic data fetching** (deploys, logs, metrics) followed by agent investigation
- **Read-only safety** — the agent reasons and reports but never modifies production systems
- **Target: summary within 2 minutes** of the alert firing

## Open Questions

1. **Workflow file management** — Should Sidekick help generate the claude-code-action workflow YAML in the target repo, or assume manual setup?
2. **Multi-repo support** — Different automations dispatching to different repos. Config supports it, UX needs thought.
3. **Authentication UX** — OAuth flows vs. manual API key entry for connector setup.
4. **Rate limiting** — GitHub Actions has API rate limits and concurrency limits. How aggressively should Sidekick queue/throttle?
5. **Managed/multi-tenant model** — Config in database instead of YAML, with a UI for onboarding. The automations/connectors abstraction supports this, but credential storage and onboarding UX need design.
