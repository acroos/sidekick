# Deployment

Sidekick supports two deployment modes: **Docker** (recommended for self-hosting) and **Vercel** (serverless).

## Docker

The project includes a multi-stage `Dockerfile` and a `docker-compose.yml` that bundles Postgres.

### Docker Compose (Sidekick + Postgres)

```bash
cp sidekick.example.yaml sidekick.yaml    # Edit with your repo/automation config
cp .env.example .env                       # Fill in tokens and secrets

docker compose up -d
```

Docker Compose provisions Postgres automatically. The `DATABASE_URL` in `.env` is overridden by the compose file — no database setup required.

Migrations run automatically on startup.

### Standalone Docker

If you already have a Postgres database:

```bash
docker build -t sidekick .

docker run -p 3000:3000 \
  -e DATABASE_URL="postgresql://user:pass@host:5432/sidekick" \
  -e GITHUB_TOKEN="ghp_..." \
  -e GITHUB_WEBHOOK_SECRET="..." \
  -e LINEAR_API_KEY="lin_api_..." \
  -e LINEAR_WEBHOOK_SECRET="..." \
  -v ./sidekick.yaml:/app/sidekick.yaml:ro \
  sidekick
```

The image exposes port 3000 and includes a health check against `/api/health`.

### Dockerfile details

The `Dockerfile` uses a multi-stage build:

1. **Build stage** — Installs all dependencies, compiles TypeScript to `dist/`
2. **Runtime stage** — Copies compiled output and production dependencies only (no dev deps, no source)

The runtime image is based on `node:24-slim` for a small footprint.

## Vercel Setup

### Entry Point

`api/index.ts` is the Vercel serverless function entry point. It imports the Hono app and wraps it with `hono/vercel`'s `handle()` adapter:

```typescript
import { handle } from "hono/vercel";
import app from "../src/app.js";

export default handle(app);
```

### Vercel Config

`vercel.json` routes all requests to the serverless function:

```json
{
  "rewrites": [{ "source": "/(.*)", "destination": "/api" }]
}
```

### Environment Variables

Set these in the Vercel dashboard (Settings → Environment Variables):

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | Postgres connection string |
| `GITHUB_TOKEN` | Yes | GitHub token with `actions:write` scope |
| `GITHUB_WEBHOOK_SECRET` | Yes | Secret for verifying GitHub webhooks |
| `LINEAR_API_KEY` | Yes (if using Linear) | Linear API key |
| `LINEAR_WEBHOOK_SECRET` | Yes (if using Linear) | Secret for verifying Linear webhooks |

Use per-environment values where needed (production/preview/development).

## Webhook Configuration

After deploying, configure webhooks in each external tool to point at your Vercel deployment:

### Linear

1. Go to Linear → Settings → API → Webhooks
2. Create a webhook pointing to `https://your-app.vercel.app/api/webhooks/linear`
3. Set the webhook secret to match your `LINEAR_WEBHOOK_SECRET` env var
4. Subscribe to relevant events (at minimum: Issue label changes)

### GitHub

1. Go to your target repo → Settings → Webhooks
2. Create a webhook pointing to `https://your-app.vercel.app/api/webhooks/github`
3. Set the secret to match your `GITHUB_WEBHOOK_SECRET` env var
4. Set content type to `application/json`
5. Subscribe to `Workflow runs` events

### Target Repo Workflow

Your target repo needs a [claude-code-action](https://github.com/anthropics/claude-code-action) workflow file that can be triggered via `workflow_dispatch`. The workflow filename must match the `github.workflow` value in your `sidekick.yaml`.

## Database

Sidekick needs a Postgres database. See [Database](database.md) for schema details and managed Postgres options.

Migrations run automatically on startup — no manual step required. For Vercel, you may need to run migrations manually since the serverless function doesn't run startup logic the same way:

```bash
DATABASE_URL="your-connection-string" npm run db:migrate
```
