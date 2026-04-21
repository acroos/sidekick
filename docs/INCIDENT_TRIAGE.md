# Incident Triage — Plan

## Vision

When an alert fires, Sidekick automatically investigates — correlating recent deployments, reading error logs, checking metrics, and posting a structured summary to the incident channel. It does the first 10 minutes of investigative grunt work that an on-call engineer would do manually, delivering actionable context while the human is still getting oriented.

This is the first **non-coding** agent use case, validating Sidekick's extensibility beyond code generation. The agent reads and reasons rather than writes.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                                                                  │
│  Alert Sources                                                   │
│  (PagerDuty, Datadog, Sentry, OpsGenie)                         │
│       │                                                          │
│       │ webhook                                                  │
│       ▼                                                          │
│  Alert Receiver                                                  │
│       │ normalize alert into common format                       │
│       │ identify affected service/component                      │
│       │ deduplicate (don't triage the same incident twice)       │
│       │                                                          │
│       ▼                                                          │
│  Sidekick API (POST /tasks)                                      │
│       │ workflow: incident-triage                                │
│       │ variables: alert details, service, timestamp             │
│       │                                                          │
│       ▼                                                          │
│  Workflow Execution                                              │
│       │                                                          │
│       ├── Deterministic: fetch recent deploys                    │
│       ├── Deterministic: fetch error logs                        │
│       ├── Deterministic: fetch metrics                           │
│       ├── Deterministic: fetch recent commits                    │
│       ├── Agent: correlate, reason, produce summary              │
│       └── Deterministic: post summary to output channel          │
│                                                                  │
│       ▼                                                          │
│  Output Channels                                                 │
│  (Slack, PagerDuty timeline, dashboard)                          │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

---

## Alert Normalization

Alerts come from different systems in different formats. The alert receiver normalizes them into a common structure:

```go
type Alert struct {
    ID          string
    Source      string            // "pagerduty", "datadog", "sentry", "opsgenie"
    Severity    string            // "critical", "high", "medium", "low"
    Service     string            // Affected service name
    Title       string            // Alert title/summary
    Description string            // Full alert details
    Timestamp   time.Time         // When the alert fired
    Tags        map[string]string // Source-specific metadata
    URL         string            // Link back to the alert in the source system
}
```

### Deduplication

Multiple alerts often fire for the same underlying incident. The receiver deduplicates by:
- Service + time window (alerts for the same service within N minutes are grouped)
- Alert grouping IDs from the source system (PagerDuty incident ID, Datadog monitor group)
- If an active triage task already exists for the same service, skip or append to existing investigation

---

## Data Fetching Strategy

The agent needs data from external systems to investigate. Deterministic steps handle all data fetching — the agent never calls external APIs directly. This keeps the architecture simple (no custom tool infrastructure) and ensures all credentials are managed by the workflow, not the agent.

### Required data sources

| Data | Source | Purpose |
|---|---|---|
| Recent deployments | CI/CD API (GitHub Actions, ArgoCD, internal deploy system) | What changed around the alert time |
| Recent commits | Git history | What code changes were deployed |
| Error logs | Log aggregator (Datadog, CloudWatch, ELK) | Error patterns, stack traces, frequency |
| Metrics | Metrics API (Datadog, Prometheus, Grafana) | Latency, error rate, traffic, resource utilization |
| Previous incidents | Alerting system API | Has this happened before? What was the resolution? |

### Credential management

All API credentials are configured at the Sidekick server level and injected into sandbox environment variables at runtime. They are used only by deterministic `curl` / script steps, never exposed to the agent.

```yaml
# sidekick server config
secrets:
  DEPLOY_API_TOKEN: ${DEPLOY_API_TOKEN}
  DATADOG_API_KEY: ${DATADOG_API_KEY}
  DATADOG_APP_KEY: ${DATADOG_APP_KEY}
  GITHUB_TOKEN: ${GITHUB_TOKEN}
```

---

## Triage Workflow

```yaml
name: incident-triage
timeout: 10m

sandbox:
  image: sidekick-sandbox-base
  network: restricted
  allow_hosts:
    - api.datadoghq.com
    - api.github.com
    - deploy.internal.acme.com

steps:
  - name: fetch-deploys
    type: deterministic
    run: |
      curl -sf -H "Authorization: Bearer $DEPLOY_API_TOKEN" \
        "https://deploy.internal.acme.com/api/deploys?service=$SERVICE&since=$ALERT_TIMESTAMP_MINUS_1H&limit=10" \
        > /workspace/recent-deploys.json
    timeout: 30s
    on_failure: continue

  - name: fetch-commits
    type: deterministic
    run: |
      curl -sf -H "Authorization: token $GITHUB_TOKEN" \
        "https://api.github.com/repos/$REPO_OWNER/$REPO_NAME/commits?since=$ALERT_TIMESTAMP_MINUS_1H&per_page=20" \
        > /workspace/recent-commits.json
    timeout: 30s
    on_failure: continue

  - name: fetch-error-logs
    type: deterministic
    run: |
      curl -sf -X POST \
        -H "DD-API-KEY: $DATADOG_API_KEY" \
        -H "DD-APPLICATION-KEY: $DATADOG_APP_KEY" \
        -d '{
          "filter": {
            "query": "service:$SERVICE status:error",
            "from": "$ALERT_TIMESTAMP_MINUS_30M",
            "to": "$ALERT_TIMESTAMP_PLUS_10M"
          },
          "sort": "-timestamp",
          "page": {"limit": 100}
        }' \
        "https://api.datadoghq.com/api/v2/logs/events/search" \
        > /workspace/error-logs.json
    timeout: 30s
    on_failure: continue

  - name: fetch-metrics
    type: deterministic
    run: |
      curl -sf -X POST \
        -H "DD-API-KEY: $DATADOG_API_KEY" \
        -H "DD-APPLICATION-KEY: $DATADOG_APP_KEY" \
        -d '{
          "from": "$ALERT_TIMESTAMP_MINUS_1H_EPOCH",
          "to": "$ALERT_TIMESTAMP_PLUS_10M_EPOCH",
          "queries": [
            {"query": "avg:http.request.duration{service:$SERVICE}", "data_source": "metrics"},
            {"query": "sum:http.request.errors{service:$SERVICE}.as_rate()", "data_source": "metrics"}
          ]
        }' \
        "https://api.datadoghq.com/api/v2/query/timeseries" \
        > /workspace/metrics.json
    timeout: 30s
    on_failure: continue

  - name: investigate
    type: agent
    context:
      - type: variable
        key: ALERT_DETAILS
        label: "Alert"
      - type: step_output
        step: fetch-deploys
        output: stdout
        label: "Recent deployments"
      - type: step_output
        step: fetch-commits
        output: stdout
        label: "Recent commits"
        max_lines: 100
      - type: step_output
        step: fetch-error-logs
        output: stdout
        label: "Error logs"
        max_lines: 200
      - type: step_output
        step: fetch-metrics
        output: stdout
        label: "Metrics (latency and error rate)"
    prompt: |
      An alert has fired for the $SERVICE service. Investigate using the data above.

      Your job:
      1. Build a timeline: when did errors start? What was deployed around that time?
      2. Identify the most likely cause by correlating deploys, commits, logs, and metrics
      3. Assess blast radius: how many requests/users are affected?
      4. Check for patterns: are the errors consistent or intermittent?
      5. Suggest a remediation action (but do NOT execute it)

      Produce a structured incident summary in the following format:

      ## Timeline
      (chronological list of relevant events)

      ## Likely Cause
      (your assessment with evidence)

      ## Blast Radius
      (scope of impact)

      ## Suggested Action
      (what a human should do — revert, config change, scale up, etc.)

      ## Confidence
      (how confident you are in this assessment: high/medium/low, and why)

      Be direct. The on-call engineer reading this is under time pressure.
    timeout: 5m

  - name: post-summary
    type: deterministic
    when: steps.investigate.status == 'succeeded'
    run: |
      curl -sf -X POST \
        -H "Authorization: Bearer $SLACK_BOT_TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
          \"channel\": \"$INCIDENT_CHANNEL\",
          \"text\": \"🔍 *Sidekick Incident Triage* — $ALERT_TITLE\",
          \"blocks\": [{
            \"type\": \"section\",
            \"text\": {
              \"type\": \"mrkdwn\",
              \"text\": $(cat /workspace/investigation-summary.md | jq -Rs .)
            }
          }]
        }" \
        "https://slack.com/api/chat.postMessage"
    timeout: 30s
    on_failure: continue
```

---

## Example Output

What the on-call engineer sees in the Slack incident channel:

```
🔍 Sidekick Incident Triage — payments-service error rate > 5%

## Timeline
  14:28 UTC — Deploy #1847 (commit abc123, PR #891: "tune connection pool settings")
  14:30 UTC — Deployment completes, new pods healthy
  14:32 UTC — Error rate begins climbing (0.1% → 2% → 8%)
  14:35 UTC — Alert fires: payments-service error rate > 5%

## Likely Cause
  Commit abc123 changed the database connection pool max_size from 20 to 5
  in config/database.yml (line 14). Under current load (~180 req/s), this
  causes connection pool exhaustion.

  Supporting evidence:
  - 94% of errors have stack trace: "ConnectionPool::checkout timeout"
  - Error onset correlates precisely with deployment completion
  - No other changes deployed in the last 6 hours
  - Metrics show request latency spiked from 45ms to 12,000ms at 14:32

## Blast Radius
  ~12% of payment requests are failing (based on error rate metric).
  Affected: all payment endpoints. Downstream impact on checkout flow likely.

## Suggested Action
  Revert commit abc123 or deploy a hotfix increasing pool max_size back to
  20 (or higher). The change is a single line in config/database.yml:14.

  Rollback command: `git revert abc123 && deploy payments-service`

## Confidence
  High — strong temporal correlation, consistent error pattern, single
  variable changed. No competing explanations.
```

---

## Hard Safety Boundaries

The incident triage agent operates under strict constraints:

### Must NEVER

- Deploy, rollback, or modify production systems
- Execute commands against production infrastructure
- Modify databases, queues, or any stateful systems
- Send alerts or pages (it responds to them, it doesn't create them)
- Merge, close, or modify PRs or issues
- Access systems not explicitly configured in the workflow

### Enforcement

These boundaries are enforced at multiple levels:
1. **Sandbox network isolation** — only allowlisted API hosts are reachable
2. **Read-only credentials** — API tokens used by deterministic steps should have read-only scopes
3. **No write tools for the agent** — the agent step has no Edit/Bash tools, only reasoning
4. **Deterministic output step** — the agent produces a summary file; a deterministic step posts it. The agent never calls Slack/PagerDuty directly.

---

## Alert Source Adapters

### Adapter interface

```go
type AlertAdapter interface {
    // ParseWebhook extracts an Alert from an incoming webhook payload
    ParseWebhook(r *http.Request) (*Alert, error)
}
```

Each alerting system gets a thin adapter that normalizes its webhook format into the common `Alert` struct.

### Supported sources (planned)

| Source | Webhook format | Notes |
|---|---|---|
| PagerDuty | V3 webhook events | Filter on `incident.triggered` events |
| Datadog | Webhook notification | Includes monitor details, tags, snapshot |
| Sentry | Issue alert webhook | Includes stack trace, breadcrumbs |
| OpsGenie | Alert webhook | Similar to PagerDuty |
| Generic | Configurable JSON mapping | For custom alerting systems |

---

## Configuration

```yaml
# .sidekick/incident-triage.yaml
alert_sources:
  - type: pagerduty
    webhook_path: /webhooks/pagerduty
    filter:
      services: ["payments-service", "auth-service", "api-gateway"]
      severity: ["critical", "high"]

  - type: datadog
    webhook_path: /webhooks/datadog
    filter:
      tags:
        env: production
        team: backend

deduplication:
  window: 15m                    # Alerts for same service within 15min are grouped

output:
  - type: slack
    channel: "#incidents"
    token_secret: SLACK_BOT_TOKEN
  - type: pagerduty_timeline     # Post summary to the PagerDuty incident timeline
    token_secret: PAGERDUTY_API_TOKEN

data_sources:
  deploys:
    endpoint: https://deploy.internal.acme.com/api/deploys
    auth_secret: DEPLOY_API_TOKEN
  logs:
    provider: datadog
    api_key_secret: DATADOG_API_KEY
    app_key_secret: DATADOG_APP_KEY
  metrics:
    provider: datadog
    api_key_secret: DATADOG_API_KEY
    app_key_secret: DATADOG_APP_KEY
  commits:
    provider: github
    token_secret: GITHUB_TOKEN

# Map services to repos for commit correlation
service_repo_map:
  payments-service: acme/payments
  auth-service: acme/auth
  api-gateway: acme/gateway
```

---

## Performance Considerations

Incident triage is time-sensitive. Target: **summary posted within 2 minutes** of alert.

| Step | Target duration |
|---|---|
| Alert reception + normalization | < 1s |
| Data fetching (parallel) | < 15s |
| Agent investigation | < 60s |
| Summary posting | < 5s |
| **Total** | **< 90s** |

To achieve this:
- Data fetching steps should run **in parallel** (this is a strong argument for implementing parallel step execution via `depends_on`)
- Agent step uses a fast model (Sonnet) by default — speed matters more than depth for triage
- Strict timeouts on every step — better to post a partial summary than no summary
- The workflow should have `on_failure: continue` for data fetching — investigate with whatever data was available

---

## Extending Beyond Triage

Once the investigation pipeline is proven, natural extensions include:

**Automated remediation suggestions with approval:**
```
Sidekick found a likely cause. Suggested action: revert commit abc123.
[Approve Revert] [Investigate Further] [Dismiss]
```
A human clicks "Approve Revert" in Slack, which triggers a Sidekick coding task to create and merge the revert PR.

**Runbook execution:**
If the service has a runbook, the agent can follow it step by step — checking each diagnostic, reporting findings, and suggesting the prescribed action.

**Post-incident analysis:**
After resolution, a separate Sidekick task writes a draft incident report by reading the timeline, agent findings, remediation actions, and resolution.

---

## Open Questions

1. **Model selection for speed:** Should triage always use the fastest available model (Sonnet) rather than the default, given the time sensitivity? Should this be configurable per workflow?
2. **Partial results:** If some data fetches fail (e.g., metrics API is down), should the agent investigate with partial data or wait? Current design says investigate with whatever is available.
3. **Ongoing incidents:** If new alerts fire for the same service during investigation, should the agent incorporate them? Or is one triage pass sufficient?
4. **Multi-service correlation:** An incident often involves multiple services. Should the agent investigate all related services, or focus on the one named in the alert?
5. **On-call integration:** Should the summary @mention the current on-call engineer? This requires integration with the on-call schedule.
