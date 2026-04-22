# Database

Sidekick uses Postgres for durable state tracking, accessed through [Drizzle ORM](https://orm.drizzle.team).

## Tables

### `runs`

Tracks the lifecycle of triggered workflow executions. One row per [run](runs.md).

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `automation` | text | No | | Automation name that produced this run |
| `trigger_type` | text | No | | Connector type (`"linear"`, `"slack"`, etc.) |
| `trigger_id` | text | No | | ID in the source system (e.g., Linear issue ID) |
| `trigger_url` | text | Yes | | Link back to the source item |
| `github_run_id` | text | Yes | | GitHub Actions run ID (linked after dispatch) |
| `repo` | text | No | | Target repo (`"owner/repo"`) |
| `status` | text | No | `"triggered"` | Run lifecycle status |
| `context` | JSONB | Yes | | Extracted context sent to the action |
| `result` | JSONB | Yes | | Results received back from the action |
| `created_at` | timestamptz | No | `now()` | Creation time |
| `updated_at` | timestamptz | No | `now()` | Last update time |
| `completed_at` | timestamptz | Yes | | When run reached terminal state |

### `run_notifications`

Tracks delivery of results to each [notification](notifications.md) target. Multiple rows per run (one per notification target in the automation).

| Column | Type | Nullable | Default | Description |
|---|---|---|---|---|
| `id` | UUID | No | `gen_random_uuid()` | Primary key |
| `run_id` | UUID | No | | FK → `runs.id` |
| `connector` | text | No | | Target connector type |
| `target_id` | text | No | | Where to post (issue ID, channel ID, etc.) |
| `target_url` | text | Yes | | Link to the target |
| `config` | JSONB | Yes | | Connector-specific notification options |
| `status` | text | No | `"pending"` | Delivery status (`pending`, `sent`, `failed`) |
| `error` | text | Yes | | Failure reason |
| `retry_count` | integer | No | `0` | Number of delivery attempts |
| `max_retries` | integer | No | `3` | Maximum retry attempts |
| `notified_at` | timestamptz | Yes | | When successfully delivered |
| `created_at` | timestamptz | No | `now()` | Creation time |

## Drizzle ORM

Schema is defined in `src/db/schema.ts` using Drizzle's `pgTable` builder. The database client is configured in `src/db/client.ts`.

### Migrations

Drizzle Kit handles migrations:

```bash
npm run db:generate    # Generate migration from schema changes
npm run db:migrate     # Apply pending migrations
```

Migration files live in the `drizzle/` directory. The config is in `drizzle.config.ts`.

## Managed Postgres Options

Any Postgres provider works. Common choices for Vercel deployments:

- [Neon](https://neon.tech) — Serverless Postgres, generous free tier
- [Supabase](https://supabase.com) — Managed Postgres with extras
- [Vercel Postgres](https://vercel.com/docs/storage/vercel-postgres) — Tight Vercel integration (powered by Neon)
