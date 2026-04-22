# Automations

An automation is the unit of configuration that ties a trigger to its notifications. Each automation defines:

- **One trigger** — Which [connector](connectors.md), what conditions, what context to extract
- **Zero or more notifications** — Which connectors to notify on completion, with connector-specific config for each

Automations are independent. A Linear label trigger can notify both Linear and Slack, while a Slack reaction trigger can notify only the Slack thread. Different automations can dispatch to different repos or workflows.

## Configuration

Automations are defined in [`sidekick.yaml`](configuration.md):

```yaml
automations:
  - name: "linear-issues"
    trigger:
      connector: linear
      on_label: "sidekick"
      context:
        include: [title, description, comments, labels]
    notifications:
      - connector: linear
        comment: true
        update_status: true
        status_mapping:
          pr_created: "In Review"
          completed: "Done"
          failed: "Triage"
      - connector: slack
        channel: "#engineering-agents"
        on: [completed, failed]
```

### Fields

| Field | Required | Description |
|---|---|---|
| `name` | Yes | Unique identifier for the automation |
| `trigger.connector` | Yes | Which connector provides the trigger |
| `trigger.on_label` | Depends | Label name that triggers the automation (Linear) |
| `trigger.on_reaction` | Depends | Emoji reaction that triggers (Slack, future) |
| `trigger.channel` | No | Channel filter (Slack, future) |
| `trigger.context.include` | No | Which fields to extract from the source |
| `notifications[].connector` | Yes | Which connector delivers the notification |
| `notifications[].on` | No | Filter to specific run outcomes (e.g., `[completed, failed]`) |
| `repo` | No | Override the default GitHub repo for this automation |

Notification fields beyond `connector` and `on` are connector-specific. See [Connectors](connectors.md) for each connector's notification options.

## Trigger Conditions

Each connector defines its own trigger types:

| Connector | Trigger type | Condition |
|---|---|---|
| Linear | `on_label` | A specific label is added to an issue |
| Slack (future) | `on_reaction` | An emoji reaction is added to a message |
| Slack (future) | `channel` + keyword | A message in a specific channel matches |

## Context Extraction

When a trigger fires, the connector extracts context from the source event. The `context.include` array controls which fields are pulled. This context is assembled into the prompt/inputs for the Claude Code GitHub Action workflow.

What gets extracted depends on the connector:

- **Linear:** Issue title, description, labels, priority, comments, linked PRs
- **Slack (future):** Message text, thread context, channel info, linked resources

## How Automation Execution Works

1. A webhook arrives at a connector's route handler
2. The `AutomationService` finds automations whose trigger matches the event
3. For each match, the connector extracts context from the source
4. A [run](runs.md) is created in the database
5. [Notification](notifications.md) records are created for each target in the automation
6. A `workflow_dispatch` event is sent to GitHub Actions
7. When the workflow completes, notifications are delivered to each target

## Implementation

- `src/services/automations.ts` — `AutomationService`: finds matching automations, executes triggers, builds prompts, dispatches to GitHub Actions
- `src/config/schema.ts` — Zod validation schemas for automation config
