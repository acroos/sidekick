# Autonomous Task Intake — Plan

## Vision

Extend Sidekick from a tool you invoke into a teammate that works alongside you. The system monitors project management tools (Linear, Jira, GitHub Issues), identifies tasks it can solve, and autonomously picks them up — operating as a 24/7 engineer that pulls work off the board.

The existing Sidekick system (API, sandboxing, workflows, streaming) stays unchanged. This is a new layer that sits *upstream*, feeding tasks into the existing API.

---

## Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│                        Source Integrations                         │
│                                                                    │
│   Linear ──┐                                                       │
│   Jira ────┼──→ Source Adapter (polling / webhooks)                │
│   GitHub ──┘         │                                             │
│                      ▼                                             │
│               Intake Queue                                         │
│               (candidate issues)                                   │
│                      │                                             │
│                      ▼                                             │
│            ┌─────────────────┐                                     │
│            │ Selection Engine │                                     │
│            │                 │                                      │
│            │  Tier 1: Rules  │  ← deterministic, label/filter      │
│            │  Tier 2: Heuristics │ ← complexity, coverage, history │
│            │  Tier 3: LLM eval   │ ← read issue, assess solvability│
│            │                 │                                      │
│            │  → confidence score + recommended action              │
│            └────────┬────────┘                                     │
│                     │                                              │
│                     ▼                                              │
│            ┌─────────────────┐                                     │
│            │ Autonomy Policy │                                     │
│            │                 │                                      │
│            │  confidence → action mapping                          │
│            │  org/team overrides                                   │
│            │  scope boundaries                                     │
│            │  rate limits                                          │
│            └────────┬────────┘                                     │
│                     │                                              │
│          ┌──────────┼──────────┐                                   │
│          ▼          ▼          ▼                                    │
│       Execute    Plan+Ask    Skip/Triage                           │
│       (auto)     (approval)  (comment)                             │
│          │          │                                              │
│          ▼          ▼                                              │
│     Sidekick API (POST /tasks)                                     │
│          │                                                         │
│          ▼                                                         │
│     Source Writeback                                                │
│     (assign self, update status, post PR link, leave notes)        │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

---

## Selection Engine

### Tier 1 — Deterministic Rules

Explicit, human-defined filters. Highest trust, lowest risk.

```yaml
# .sidekick/intake.yaml
sources:
  - type: linear
    project: "BACKEND"
    filters:
      label: "sidekick-eligible"
      status: "Todo"
      priority: ["Urgent", "High", "Medium"]
    workflow: fix-issue
    autonomy: execute    # auto-execute, no approval needed
```

Teams tag issues as agent-eligible. No ambiguity about what enters the system.

**Examples:**
- Any issue labeled `sidekick-eligible`
- Dependency update PRs from Renovate/Dependabot
- Linting violations auto-created by CI
- Issues in a specific "agent-friendly" board/column

### Tier 2 — Heuristics

Pattern-based scoring without LLM involvement. Medium trust.

**Signals that increase confidence:**
- Issue has a linked test failure or reproduction steps
- Issue references a small number of files (< 5)
- Affected code area has high test coverage
- Issue type matches historically successful patterns
- Issue was created by a bot (structured, well-defined)
- Small estimated scope (based on labels, keywords)

**Signals that decrease confidence:**
- Issue mentions multiple services or cross-cutting concerns
- No test coverage in the affected area
- Issue description is vague or conversational
- Touches security-sensitive code paths
- Issue has unresolved discussion/debate in comments

**Output:** A numeric confidence score + contributing factors.

### Tier 3 — LLM Evaluation

Full reasoning about the issue. Richest understanding, lowest inherent trust.

The system feeds the issue (title, description, comments, linked PRs) to an LLM and asks for a structured assessment:

```json
{
  "issue_id": "BACKEND-1234",
  "assessment": {
    "solvable": true,
    "confidence": 0.82,
    "estimated_complexity": "low",
    "estimated_files": ["src/user/UserService.ts"],
    "risk_factors": ["touches payment-adjacent code"],
    "missing_context": [],
    "approach_summary": "Add null check for user.avatar before accessing .url property",
    "recommended_action": "execute_with_review"
  }
}
```

The LLM evaluation itself runs through the Sidekick sandbox (dogfooding the isolation model).

---

## Confidence → Autonomy Mapping

Confidence level determines how much human involvement is required.

| Confidence | Action | What happens | Human involvement |
|---|---|---|---|
| **High** | `execute` | Run workflow → open PR | Code review on the PR |
| **Medium** | `execute_with_review` | Run workflow → draft PR → flag for review | Reviewer marks PR ready when satisfied |
| **Low** | `plan_and_ask` | Draft an approach → post plan to issue → wait for approval → execute if approved | Human approves approach before code is written |
| **Very low** | `triage` | Post a comment with analysis: "I looked at this but I'm not confident I can solve it. Here's what I think is needed: ..." | Human takes over, agent's analysis is a head start |
| **Below threshold** | `skip` | Silently skip, log internally | None |

### Configurable policies

```yaml
# .sidekick/intake.yaml
policy:
  # Global defaults
  default_autonomy: plan_and_ask
  min_confidence: 0.3                # Below this, skip entirely

  # Tier overrides
  tier1_autonomy: execute            # Tagged issues → auto-execute
  tier2_autonomy: execute_with_review
  tier3_autonomy: plan_and_ask

  # Scope boundaries — never auto-execute in these areas
  restricted_paths:
    - "src/billing/**"
    - "src/auth/**"
    - "infrastructure/**"
  restricted_labels:
    - "security"
    - "data-migration"

  # Rate limits
  max_concurrent_tasks: 3
  max_tasks_per_hour: 10
  max_tasks_per_day: 50

  # Team overrides
  overrides:
    - team: "platform"
      tier2_autonomy: execute        # Platform team trusts tier 2
      max_concurrent_tasks: 5
```

---

## Source Integration Design

### Adapter interface

```go
type SourceAdapter interface {
    // Poll returns new/updated issues since the last check
    Poll(ctx context.Context, cfg SourceConfig) ([]Issue, error)

    // Assign marks the issue as being worked on by Sidekick
    Assign(ctx context.Context, issueID string) error

    // UpdateStatus moves the issue to a new status
    UpdateStatus(ctx context.Context, issueID string, status string) error

    // Comment posts a comment on the issue
    Comment(ctx context.Context, issueID string, body string) error

    // LinkPR associates a pull request with the issue
    LinkPR(ctx context.Context, issueID string, prURL string) error
}

type Issue struct {
    ID          string
    Source      string            // "linear", "jira", "github"
    Title       string
    Description string
    Labels      []string
    Priority    string
    Status      string
    Assignee    string            // Empty if unassigned
    Comments    []Comment
    URL         string
    Metadata    map[string]string // Source-specific fields
}
```

### Discovery methods

| Method | Latency | Complexity | Best for |
|---|---|---|---|
| **Polling** | Seconds to minutes | Low | MVP, simple setup |
| **Webhooks** | Real-time | Medium | Production, faster response |

Start with polling (configurable interval, default 5 minutes). Add webhook support later for lower latency.

---

## Feedback Loop

PR outcomes feed back into the selection engine to improve accuracy over time.

### Signals

| Outcome | Signal | Impact |
|---|---|---|
| PR merged without changes | Strong success | Boost confidence for similar issues |
| PR merged after minor revisions | Success | Moderate boost |
| PR merged after major revisions | Weak success | Neutral |
| PR closed (rejected) | Failure | Decrease confidence, log reason |
| Task failed before PR | Failure | Classify root cause |
| Task timed out | Failure | Likely too complex |

### What gets tracked

```go
type TaskOutcome struct {
    TaskID        string
    IssueID       string
    Source        string
    SelectionTier int               // 1, 2, or 3
    Confidence    float64           // At selection time
    Labels        []string
    RepoArea      string            // Generalized path pattern
    Outcome       string            // merged, merged_with_revisions, rejected, failed, timeout
    RevisionCount int               // Number of review rounds before merge
    Duration      time.Duration     // End-to-end time
    TokensUsed    int
    FailureReason string            // If failed
}
```

### How it improves selection

- **Heuristic tuning:** Confidence weights for tier 2 signals are adjusted based on outcome data. "Issues with test failures have a 91% success rate" → weight that signal higher.
- **LLM context enrichment:** Tier 3 evaluations include historical performance. "In this repo, I succeed 88% on null-check bugs but only 25% on concurrency issues."
- **Automatic scope restriction:** If the system consistently fails in a code area, that area gets added to `restricted_paths` suggestions (human approves).
- **Confidence calibration:** Compare predicted confidence vs actual outcomes. Recalibrate so that "0.8 confidence" actually means ~80% success.

---

## Social Dynamics & Etiquette

The agent should be a good teammate, not a noisy bot.

### Rules

1. **Never pick up assigned issues** — if someone is already working on it, leave it alone
2. **Assign itself immediately** — when the agent starts work, it assigns the issue to itself so the team sees it
3. **Update status** — move the issue through the board (Todo → In Progress → In Review)
4. **Clear PR labeling** — PRs are labeled `sidekick-automated` and the description states it's agent-generated
5. **Comment, don't spam** — one status comment when starting, one when PR is opened or when it gives up. No progress dumps in the issue tracker.
6. **Graceful failure** — if the agent can't solve it, it comments with what it learned and unassigns itself, leaving the issue in a better state than it found it
7. **Respect working hours** — configurable quiet hours for PR creation (optional, some teams won't want this)
8. **Back off on rejection** — if a team consistently closes the agent's PRs for a certain issue type, reduce confidence for that pattern

---

## Safety Rails

### Hard limits

- **Kill switch** — admin can pause all autonomous intake instantly (API + CLI)
- **Scope boundaries** — explicit path/label restrictions that cannot be overridden by confidence
- **Rate limits** — per-team, per-repo, global caps on concurrent tasks and throughput
- **No self-modification** — the agent cannot modify its own config, intake rules, or workflow definitions
- **No destructive operations** — agent workflows cannot delete branches, force push, close issues, or merge its own PRs

### Monitoring

- Dashboard: tasks selected, attempted, succeeded, failed, rejected — sliced by source, team, repo, tier
- Alerts: success rate drops below threshold, task failure spike, rate limit hit
- Audit log: every issue the system evaluated, what it decided, and why

---

## Rollout Strategy

### Phase 1 — Deterministic intake (v1.1)

- Source adapter for Linear (first integration)
- Polling-based discovery
- Tier 1 only (label-based rules)
- Agent assigns itself, updates status, links PRs
- No feedback loop yet — just execution + reporting
- Teams opt in by tagging issues

This alone is valuable. "Tag an issue, get a PR" is a meaningful workflow improvement.

### Phase 2 — Smart selection (v1.2)

- Tier 2 heuristic scoring
- Tier 3 LLM evaluation
- Confidence → autonomy mapping with configurable policies
- Source adapters for Jira and GitHub Issues
- Feedback loop from PR outcomes
- Selection accuracy dashboard

### Phase 3 — Learning system (v2.0)

- Heuristic weight tuning from outcome data
- Confidence calibration
- Automatic scope restriction suggestions
- Cross-repo learning (patterns from one repo inform another)
- Webhook-based intake for real-time response

---

## Open Questions

1. **Multi-repo issues** — some issues span multiple repos. Should the agent handle these or skip them?
2. **Issue decomposition** — if an issue is too large, should the agent break it into sub-tasks and tackle them individually?
3. **Human-in-the-loop iteration** — if a reviewer requests changes on the PR, should the agent respond and iterate? (This is essentially multi-turn agentic work.)
4. **Priority scheduling** — when multiple issues are eligible, how do we prioritize? Source priority? Estimated complexity (easy first)? Age?
5. **Cost attribution** — how do we report LLM costs per team/repo/issue for chargeback?
