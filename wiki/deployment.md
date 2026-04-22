# Deployment

Sidekick is designed for [Vercel](https://vercel.com) serverless deployment.

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

Run migrations before the first deployment:

```bash
DATABASE_URL="your-connection-string" npm run db:migrate
```
