# Connectors

A connector is an integration with an external tool. Each connector can provide:

- **Trigger capability** — Webhook endpoint + event parser. Receives events from the tool, evaluates trigger conditions, and extracts context for the agent run.
- **Notification capability** — Result formatter + API client. Takes run results and posts them back to the tool (comments, status updates, thread replies).
- **Credentials** — API keys, webhook secrets, etc. Defined once in the `connectors` section of [`sidekick.yaml`](configuration.md) and shared across [automations](automations.md).

A connector can serve as a trigger source, a notification target, or both — depending on how it's referenced in automations.

## Linear Connector

The first and currently only fully implemented connector.

### Trigger: Label Events

When a label is added to a Linear issue, the webhook fires. Sidekick checks if the label matches any automation's `on_label` trigger. If so, it extracts context from the issue and dispatches a GitHub Actions workflow.

**Webhook endpoint:** `POST /api/webhooks/linear`

**Signature verification:** HMAC-SHA256 using the `linear-signature` header and the connector's `webhook_secret`.

**Event parsing:** The handler parses the webhook payload, extracts label addition events, and matches against configured automations. Only `on_label` triggers are currently supported.

**Context extraction** is configurable per automation — choose which fields to include:

| Field | Description |
|---|---|
| `title` | Issue title |
| `description` | Issue description/body |
| `comments` | Comment thread on the issue |
| `labels` | All labels on the issue |

The extracted context is assembled into the prompt sent to the Claude Code GitHub Action.

### Notification: Comments and Status Updates

After a run completes, the Linear connector can:

- **Post a comment** on the originating issue with a summary of what the agent did, including links to any PRs created
- **Update issue status** based on the run outcome, using a configurable `status_mapping` (e.g., `pr_created → "In Review"`, `completed → "Done"`, `failed → "Triage"`)

### Implementation

- `src/connectors/linear/client.ts` — `LinearClient` wrapping `@linear/sdk` for fetching issue context, posting comments, and updating issue state
- `src/connectors/linear/webhook.ts` — Webhook payload parsing, signature verification, label event extraction

## Adding a New Connector

Adding a new tool means implementing:

1. **Webhook handler** — Parse the tool's webhook format, verify signatures, extract the relevant event
2. **Trigger matching** — Evaluate whether the event matches a configured automation trigger
3. **Context extractor** — Pull relevant data from the tool's API to assemble agent context
4. **Notification handler** — Format and deliver results back to the tool
5. **Route** — Register the webhook endpoint in `src/routes/`
6. **Config schema** — Add the connector's credential fields to the Zod schema in `src/config/schema.ts`

The core framework handles dispatching to GitHub Actions, state tracking, and orchestrating notification delivery. The connector only needs to handle tool-specific parsing and formatting.

## Planned Connectors

See [Roadmap](roadmap.md) for future connector plans (Slack, PagerDuty, generic webhook).
