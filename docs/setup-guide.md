# Setup Guide

This guide walks you through getting Sidekick running from scratch. By the end, you'll have a deployed instance that receives Linear webhooks, triggers Claude Code GitHub Action workflows, and posts results back to Linear.

## What you're building

Sidekick sits between your productivity tools (like Linear) and GitHub Actions. When you label a Linear issue with a specific label, Sidekick dispatches a [claude-code-action](https://github.com/anthropics/claude-code-action) workflow in your repo. When the workflow finishes, Sidekick posts the results back to the Linear issue as a comment.

```
Linear issue labeled "sidekick"
  → Sidekick receives webhook
    → Dispatches workflow_dispatch to GitHub Actions
      → claude-code-action runs
    → GitHub sends completion webhook back to Sidekick
  → Sidekick posts results as a Linear comment
```

## Prerequisites

You will need:

- **Node.js 24** (specified in `.node-version`; 20+ will work)
- **A GitHub account** with a repo you want Claude Code to work on
- **A Linear account** (the only connector currently supported)
- **One of:** a [Vercel](https://vercel.com) account or a [Railway](https://railway.com) account for deployment

## Step 1: Clone the repo

```bash
git clone https://github.com/your-org/sidekick.git
cd sidekick
npm install
```

## Step 2: Set up Postgres

Sidekick stores run state in Postgres. You need a database before anything else.

### Option A: Railway Postgres (if deploying to Railway)

Railway can provision a Postgres database as part of your project. Skip this step for now — you'll create it during Railway deployment in [Step 7B](#step-7b-deploy-to-railway).

### Option B: Neon (recommended for Vercel)

1. Create a free account at [neon.tech](https://neon.tech)
2. Create a new project
3. Copy the connection string from the dashboard — it looks like:
   ```
   postgresql://user:password@ep-something.us-east-2.aws.neon.tech/neondb?sslmode=require
   ```

### Option C: Supabase

1. Create a free account at [supabase.com](https://supabase.com)
2. Create a new project
3. Go to Settings > Database > Connection string > URI
4. Copy the connection string

### Option D: Local Postgres (for development only)

If you have Postgres installed locally:

```bash
createdb sidekick
# Connection string: postgresql://localhost:5432/sidekick
```

Save your connection string — you'll need it in the next step.

## Step 3: Configure environment variables

Copy the example environment file:

```bash
cp .env.example .env
```

Edit `.env` and fill in each value:

```bash
# Database
DATABASE_URL=postgresql://user:password@host:5432/sidekick

# GitHub
GITHUB_TOKEN=ghp_...
GITHUB_WEBHOOK_SECRET=whsec_...

# Linear
LINEAR_API_KEY=lin_api_...
LINEAR_WEBHOOK_SECRET=...
```

How to get each value:

### `DATABASE_URL`

The Postgres connection string from Step 2.

### `GITHUB_TOKEN`

A GitHub personal access token with permission to trigger workflows on your target repo.

1. Go to [github.com/settings/tokens](https://github.com/settings/tokens) > **Fine-grained tokens** > **Generate new token**
2. Set **Repository access** to your target repo
3. Under **Permissions > Repository permissions**, set **Actions** to **Read and write**
4. Generate the token and copy it

### `GITHUB_WEBHOOK_SECRET`

A secret string used to verify that incoming GitHub webhooks are authentic. Generate one:

```bash
openssl rand -hex 32
```

Save this value — you'll use it again when configuring the GitHub webhook in Step 6.

### `LINEAR_API_KEY`

1. Go to [linear.app/your-workspace/settings/account/security](https://linear.app/austin-roos/settings/account/security)
2. Under **Personal API keys**, create a new key
3. Copy the key (starts with `lin_api_`)

### `LINEAR_WEBHOOK_SECRET`

A secret string used to verify Linear webhooks. Generate one:

```bash
openssl rand -hex 32
```

Save this value — you'll use it again when configuring the Linear webhook in Step 6.

## Step 4: Configure Sidekick

Copy the example config:

```bash
cp sidekick.example.yaml sidekick.yaml
```

Edit `sidekick.yaml`:

```yaml
github:
  token: ${GITHUB_TOKEN}
  default_repo: "your-org/your-repo"       # <-- change this
  workflow: "claude-code-action.yml"        # <-- must match your workflow filename

connectors:
  linear:
    api_key: ${LINEAR_API_KEY}
    webhook_secret: ${LINEAR_WEBHOOK_SECRET}

automations:
  - name: "linear-issues"
    trigger:
      connector: linear
      on_label: "sidekick"                  # <-- the label that triggers a run
      context:
        include:
          - title
          - description
          - comments
          - labels
    notifications:
      - connector: linear
        comment: true                       # post results as a comment
        update_status: true                 # move the issue through workflow states
        status_mapping:
          pr_created: "In Review"
          completed: "Done"
          failed: "Triage"
```

The `${VAR}` references resolve from your environment variables at startup — the YAML file itself contains no secrets.

**Important:** Update `default_repo` to point to the repo where your claude-code-action workflow lives.

## Step 5: Set up the target repo workflow

Your target GitHub repo needs a workflow file that claude-code-action can use. Sidekick triggers it via `workflow_dispatch`.

Create `.github/workflows/claude-code-action.yml` in your target repo:

```yaml
name: Claude Code Action

on:
  workflow_dispatch:
    inputs:
      prompt:
        description: "The prompt for Claude"
        required: true
      sidekick_run_id:
        description: "Sidekick run tracking ID"
        required: false

jobs:
  claude:
    runs-on: ubuntu-latest
    permissions:
      contents: write
      pull-requests: write
      issues: write
      id-token: write
    steps:
      - uses: actions/checkout@v6

      - uses: anthropics/claude-code-action@v1
        with:
          prompt: ${{ inputs.prompt }}
          anthropic_api_key: ${{ secrets.ANTHROPIC_API_KEY }}
          claude_args: '--allowedTools "Bash(git *),Bash(gh *),Bash(npm *),Read,Write,Edit,Glob,Grep"'
```

Key settings:

- **`id-token: write`** — Required for OIDC token exchange. The action uses this to obtain a GitHub app token for authentication.
- **`claude_args` with `--allowedTools`** — Required for agent mode (`workflow_dispatch`). Without this, Claude runs successfully but gets permission denials when trying to create branches, push commits, or open PRs. The `Bash(git *)` and `Bash(gh *)` entries are what allow PR creation. Note: `--allowedTools` is a Claude Code CLI flag passed via `claude_args`, not a top-level action input.

You'll also need to add an `ANTHROPIC_API_KEY` secret to that repo (Settings > Secrets and variables > Actions > New repository secret).

The workflow filename must match the `github.workflow` value in your `sidekick.yaml` (default: `claude-code-action.yml`).

## Step 6: Run database migrations

Before starting Sidekick for the first time, create the database tables:

```bash
npm run db:generate
npm run db:migrate
```

If you're using a remote database, make sure `DATABASE_URL` is set in your environment (or in your `.env` file) before running this.

## Step 7: Deploy

Choose one of the following deployment options.

---

### Step 7A: Deploy to Vercel

Vercel is the primary deployment target. Sidekick uses a serverless function entry point at `api/index.ts`.

#### 1. Connect your repo

1. Go to [vercel.com/new](https://vercel.com/new)
2. Import your Sidekick repository
3. Vercel auto-detects the configuration from `vercel.json` — no framework selection needed

#### 2. Set environment variables

Before deploying, add your environment variables in the Vercel dashboard:

**Project Settings > Environment Variables**

| Variable | Value |
|---|---|
| `DATABASE_URL` | Your Postgres connection string |
| `GITHUB_TOKEN` | Your GitHub token |
| `GITHUB_WEBHOOK_SECRET` | The secret you generated |
| `LINEAR_API_KEY` | Your Linear API key |
| `LINEAR_WEBHOOK_SECRET` | The secret you generated |

Add each variable for the **Production** environment (and Preview/Development if you want those too).

#### 3. Deploy

Click **Deploy**. Vercel runs `npm run build` (TypeScript compilation) and deploys the serverless function.

After deployment, your app is live at `https://your-project.vercel.app`. Verify by visiting:

```
https://your-project.vercel.app/api/health
```

You should see a JSON response with `"status": "ok"`.

#### 4. Upload the config file

The `sidekick.yaml` config file needs to be available at the project root at runtime. Since it's in `.gitignore` (it's generated from the example), you have two options:

- **Commit it to the repo** — the file contains no secrets (just `${VAR}` references), so this is safe and recommended.
- **Add it to your build step** — generate it during the Vercel build.

The simplest approach: commit `sidekick.yaml` to your repo.

---

### Step 7B: Deploy to Railway

Railway runs Sidekick as a long-lived Node.js process (not serverless). It uses the `src/index.ts` entry point.

#### 1. Create a Railway project

1. Go to [railway.com/new](https://railway.com/new)
2. Click **Deploy from GitHub repo** and select your Sidekick repository

#### 2. Add a Postgres database

1. In your Railway project, click **+ New** > **Database** > **Add PostgreSQL**
2. Railway provisions a Postgres instance and makes a `DATABASE_URL` variable available
3. In the database service, go to **Settings > Networking** and note the **Internal** connection string — Railway services can connect to each other over the private network

#### 3. Link the database to your service

1. Click on your Sidekick service
2. Go to **Variables**
3. Add a reference to the Postgres database:
   - Click **Add a Variable** > **Add Reference** > select your Postgres service > choose `DATABASE_URL`
   - This injects the internal connection string automatically

#### 4. Set environment variables

In your Sidekick service's **Variables** tab, add:

| Variable | Value |
|---|---|
| `GITHUB_TOKEN` | Your GitHub token |
| `GITHUB_WEBHOOK_SECRET` | The secret you generated |
| `LINEAR_API_KEY` | Your Linear API key |
| `LINEAR_WEBHOOK_SECRET` | The secret you generated |
| `PORT` | `3000` (Railway sets this automatically, but it's good to be explicit) |

`DATABASE_URL` should already be set from the reference in the previous step.

#### 5. Configure build and start commands

In your Sidekick service, go to **Settings**:

- **Build Command:** `npm install && npm run build`
- **Start Command:** `npm run start`

Alternatively, Railway auto-detects these from `package.json` in most cases.

#### 6. Run database migrations

Railway doesn't run migrations automatically. You can either:

**Option A: Add a deploy command** — In Settings, set a **Deploy Command** (runs after build, before start):

```bash
npm run db:migrate
```

**Option B: Run manually** — Use the Railway CLI:

```bash
railway run npm run db:migrate
```

#### 7. Get your public URL

1. In your Sidekick service, go to **Settings > Networking**
2. Click **Generate Domain** to get a public URL (e.g., `sidekick-production.up.railway.app`)

Verify the deployment:

```
https://your-app.up.railway.app/api/health
```

---

## Step 8: Configure webhooks

Now that Sidekick is deployed and reachable, configure the external tools to send webhooks to it.

Replace `YOUR_SIDEKICK_URL` below with your deployment URL (e.g., `https://your-project.vercel.app` or `https://your-app.up.railway.app`).

### Linear webhook

1. Go to **Linear** > **Settings** > **API** > **Webhooks**
2. Click **Create webhook**
3. Set the URL to: `YOUR_SIDEKICK_URL/api/webhooks/linear`
4. Set the secret to the same value as your `LINEAR_WEBHOOK_SECRET` environment variable
5. Subscribe to: **Issues** (specifically label change events)
6. Save the webhook

### GitHub webhook

1. Go to your **target repo** on GitHub > **Settings** > **Webhooks**
2. Click **Add webhook**
3. Set the Payload URL to: `YOUR_SIDEKICK_URL/api/webhooks/github`
4. Set Content type to: `application/json`
5. Set the Secret to the same value as your `GITHUB_WEBHOOK_SECRET` environment variable
6. Under **Which events would you like to trigger this webhook?**, select **Let me select individual events** and check **Workflow runs**
7. Click **Add webhook**

## Step 9: Create the trigger label in Linear

In Linear, create the label that will trigger Sidekick:

1. Go to **Settings** > **Labels** (workspace or team level)
2. Create a label named `sidekick` (or whatever you set `on_label` to in your `sidekick.yaml`)

## Step 10: Test it

1. Create a new issue in Linear (or open an existing one)
2. Add the `sidekick` label to the issue
3. Watch what happens:
   - Sidekick receives the webhook from Linear
   - It extracts context from the issue (title, description, comments, labels)
   - It dispatches a `workflow_dispatch` event to your GitHub repo
   - The claude-code-action workflow runs in GitHub Actions
   - When the workflow completes, GitHub sends a webhook back to Sidekick
   - Sidekick posts the results as a comment on the Linear issue

Check the health endpoint for basic connectivity:

```
YOUR_SIDEKICK_URL/api/health
```

Check the runs endpoint to see tracked runs:

```
YOUR_SIDEKICK_URL/api/runs
```

## Troubleshooting

### Webhook not received

- Verify the webhook URL is correct and publicly accessible
- Check that the webhook secret matches between the tool's settings and your environment variable
- For Linear: ensure you subscribed to Issue events
- For GitHub: ensure you subscribed to Workflow run events and content type is `application/json`

### Workflow completes but no PR is created

- Check the action's result output for `permission_denials_count` — if this is greater than 0, Claude was blocked from performing actions. Add `--allowedTools` via `claude_args` in your workflow (see Step 5).
- Enable `show_full_output: true` in the action's `with` block to see exactly which tools were denied.
- Ensure the `id-token: write` permission is set — without it, OIDC token exchange fails and git push won't authenticate.

### Workflow not dispatching

- Verify `GITHUB_TOKEN` has `actions:write` permission on the target repo
- Verify `default_repo` in `sidekick.yaml` matches the repo with the workflow file
- Verify the workflow filename in `sidekick.yaml` matches the actual file in `.github/workflows/`

### Database connection failing

- Check that `DATABASE_URL` is set and the connection string is valid
- For Neon/Supabase: ensure the connection string includes `?sslmode=require`
- For Railway: ensure you're using the internal connection URL (not the public one) if both services are in the same project

### Config validation errors at startup

- Ensure all `${VAR}` references in `sidekick.yaml` have matching environment variables set
- Check that connector names in automations match keys in the `connectors` section
- Run locally first with `npm run dev` to see detailed validation errors

## Local development

For day-to-day development without deploying:

```bash
npm run dev          # Start dev server on http://localhost:3000
npm run lint         # Run the Biome linter
npm run typecheck    # Type-check without emitting
npm test             # Run the test suite
```

To test webhooks locally, use a tunneling tool like [ngrok](https://ngrok.com) to expose your local server:

```bash
ngrok http 3000
# Use the ngrok URL as YOUR_SIDEKICK_URL when configuring webhooks
```

## Reference

- [Configuration](../wiki/configuration.md) — Full `sidekick.yaml` schema and environment variable reference
- [Architecture](../wiki/architecture.md) — How the system fits together
- [Database](../wiki/database.md) — Table schemas and Drizzle ORM usage
- [API](../wiki/api.md) — HTTP endpoints and webhook handling
- [Connectors](../wiki/connectors.md) — Connector architecture and how to add new ones
