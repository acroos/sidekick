# Development

## Prerequisites

- Node.js 20+ **or** Docker
- A Postgres database (local, managed, or via Docker Compose — see [Database](database.md))

## Setup

### Option A: Docker Compose

The simplest way to get running — no Node.js or Postgres installation required:

```bash
cp sidekick.example.yaml sidekick.yaml
cp .env.example .env
# Edit both files with your credentials

docker compose up
```

Docker Compose starts Postgres and Sidekick together. Migrations run automatically on startup.

### Option B: Local Node.js

```bash
npm install
cp sidekick.example.yaml sidekick.yaml
cp .env.example .env
# Edit both files with your credentials

npm run dev
```

The dev server runs on `http://localhost:3000` with hot reload via `tsx watch`. Migrations run automatically on startup.

## Commands

| Command | Description |
|---|---|
| `npm run dev` | Start dev server with hot reload |
| `npm run build` | TypeScript compilation to `dist/` |
| `npm run typecheck` | Type checking without emit |
| `npm run lint` | Biome linter |
| `npm run lint:fix` | Biome linter with auto-fix |
| `npm test` | Run tests (Vitest) |
| `npm run test:watch` | Run tests in watch mode |
| `npm run db:generate` | Generate Drizzle migration from schema changes |
| `npm run db:migrate` | Apply pending database migrations |

## Pre-Push Checks

Run before pushing — CI will fail if any of these fail:

```bash
npm run lint
npm run typecheck
npm test
```

## Project Structure

```
├── api/index.ts               Vercel serverless entry point
├── src/
│   ├── app.ts                 Hono app factory (dependency injection)
│   ├── index.ts               Node.js entry point (auto-migrates on startup)
│   ├── config/                YAML config loader + Zod validation
│   ├── connectors/linear/     Linear client, webhook parsing, signature verification
│   ├── github/                Octokit client, workflow dispatch, webhook parsing
│   ├── db/                    Drizzle schema, database client, migration runner
│   ├── routes/                HTTP route handlers
│   ├── services/              Business logic (runs, automations, notifications)
│   └── middleware/            Request logging, error handling
├── Dockerfile                 Multi-stage Docker build
├── docker-compose.yml         Sidekick + Postgres for local/self-hosted use
├── sidekick.example.yaml      Example config (copy to sidekick.yaml)
├── .env.example               Example env vars (copy to .env)
├── drizzle.config.ts          Drizzle Kit config
├── biome.json                 Biome linter/formatter config
├── tsconfig.json              TypeScript config
├── vercel.json                Vercel routing config
└── vitest.config.ts           Vitest test config
```

## Testing

Tests use [Vitest](https://vitest.dev) and live alongside source files in `__tests__/` directories.

```bash
npm test              # Single run
npm run test:watch    # Watch mode
```

## Tooling

- **Biome** — Linting and formatting. Config in `biome.json`.
- **TypeScript** — Strict mode. Config in `tsconfig.json`.
- **Drizzle Kit** — Database migration generation and application. Config in `drizzle.config.ts`.
