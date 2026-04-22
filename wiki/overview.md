# Overview

## What Sidekick Is

Sidekick is an integration hub that connects productivity and operational tools to [Claude Code GitHub Action](https://github.com/anthropics/claude-code-action) runs. It receives webhooks from tools like Linear and Slack, triggers agent workflows in GitHub Actions, and routes results back to the originating tool.

The core loop: **something happens in a tool you use** (a Linear issue gets labeled, a Slack message gets a reaction) **→ Sidekick dispatches a Claude Code run** in GitHub Actions **→ results flow back** as comments, status updates, or thread replies.

## Why It Exists

The [Claude Code GitHub Action](https://github.com/anthropics/claude-code-action) already runs Claude sessions in GitHub's infrastructure with full CI integration. The execution problem is solved. What's missing is **connectivity** — there's no good way to trigger an agent run from a Linear ticket, a Slack alert, or any other tool where work actually originates, and no way to route the results back.

Sidekick fills that gap. It doesn't run agents itself. It connects the tools where work lives to the infrastructure where agents run.

## Background

Sidekick was originally designed as a sandboxed agent runtime — container orchestration, workflow execution, agent management, all in Go. That turned out to be a constrained reimplementation of what GitHub Actions already does. The project pivoted in April 2026 to focus on the integration layer, rewriting in TypeScript with Hono for Vercel deployment.

## Design Principles

- **Delegate execution** — GitHub Actions runs the agents. Sidekick handles everything around that: receiving events, extracting context, dispatching workflows, tracking state, and routing results.
- **Connectors are modular** — Adding a new tool means implementing its webhook handler and notification formatter. The core framework handles dispatching, state tracking, and result routing.
- **Config as code** — Automations are defined in a committed YAML file with no secrets. Environment variables handle credentials.
- **Self-deployed** — The team deploying Sidekick is the team using it. Deploy to Vercel, configure your connectors, and go.
