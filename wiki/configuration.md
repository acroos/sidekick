# Configuration

Sidekick is configured via a `sidekick.yaml` file in the repo root plus environment variables for secrets.

## Config File

The YAML file is committed to the repo. It contains **no secrets** — only `${VAR}` references that resolve against environment variables at startup.

```yaml
# sidekick.yaml

github:
  token: ${GITHUB_TOKEN}
  default_repo: "org/repo"
  workflow: "claude-code-action.yml"

connectors:
  linear:
    api_key: ${LINEAR_API_KEY}
    webhook_secret: ${LINEAR_WEBHOOK_SECRET}
  # slack:
  #   bot_token: ${SLACK_BOT_TOKEN}

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
```

An example file is provided at `sidekick.example.yaml`.

## Sections

### `github`

Required. Configures the GitHub Actions integration.

| Field | Required | Description |
|---|---|---|
| `token` | Yes | GitHub token with `actions:write` scope |
| `default_repo` | Yes | Default target repo (`"owner/repo"`) |
| `workflow` | Yes | Workflow filename to dispatch (e.g., `"claude-code-action.yml"`) |

### `connectors`

A map of connector name → connector credentials. Each connector can have:

| Field | Description |
|---|---|
| `api_key` | API key for the tool's API |
| `webhook_secret` | Secret for verifying inbound webhooks |
| `bot_token` | Bot/app token (e.g., Slack) |

Connector names here must match the `connector` references in automations.

### `automations`

A list of automation definitions. See [Automations](automations.md) for the full schema.

## Environment Variables

Secrets live in environment variables, never in the YAML file:

| Where | How |
|---|---|
| **Vercel** | Environment Variables dashboard (per-environment: production/preview/development) |
| **Local dev** | `.env` file (gitignored). Copy `.env.example` to get started. |

### Required Variables

| Variable | Used by |
|---|---|
| `GITHUB_TOKEN` | GitHub Actions workflow dispatch |
| `DATABASE_URL` | Postgres connection string |

### Per-Connector Variables

| Variable | Connector |
|---|---|
| `LINEAR_API_KEY` | Linear API access |
| `LINEAR_WEBHOOK_SECRET` | Linear webhook signature verification |
| `GITHUB_WEBHOOK_SECRET` | GitHub webhook signature verification |

## Validation

### Env Var Interpolation

The config loader (`src/config/loader.ts`) resolves all `${VAR}` references at startup. If any referenced variable is missing, startup fails with a clear error listing the missing variables.

### Schema Validation

After interpolation, the config is validated against a Zod schema (`src/config/schema.ts`). This catches structural issues — missing required fields, wrong types, malformed values.

### Runtime Validation

`src/config/validate.ts` performs additional checks:

- Every connector referenced in an automation exists in the `connectors` map
- Automation names are unique
- Trigger types are valid for the referenced connector
