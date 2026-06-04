# Absorb Manifest — aws-billing-pp-cli

Source tools surveyed: kamranahmedse/aws-cost-cli (TS, Slack), cduggn/ccExplorer (Go), elC0mpa/aws-doctor, Komiser, jamescarr/aws-cost-report, berkayildi/mcp-aws-cost-explorer, ravikiranvm/aws-finops-mcp-server, aarora79/aws-cost-explorer-mcp-server, aws-samples/sample-cfm-tips-mcp.

## Absorbed (match or beat everything that exists)

| # | Feature | Best Source | Our Implementation | Added Value |
|---|---------|-------------|--------------------|-------------|
| 1 | Total cost (current/last month, last 7d, yesterday presets) | aws-cost-cli | `cost --period this-month\|last-month\|7d\|yesterday` | SQLite-cached → no repeat $0.01/CE call; `--json/--select` |
| 2 | By-service breakdown | aws-cost-cli, ccExplorer | `cost --group-by service` | offline, agent-native, sortable by $ |
| 3 | Slack delivery | aws-cost-cli | `report --post-slack` delegates to `slack-pp-cli` | HTML/PDF attached, smart breakdown, DM target |
| 4 | Profile/region/credential selection | aws-cost-cli | native SDK chain + `--profile`/`--region` | SSO + assume-role + IMDS, no key flags needed |
| 5 | JSON / summary / text output | aws-cost-cli | `--json` / `--compact` / `--csv` (generated flags) | + `--select` dotted-path narrowing |
| 6 | By-account (LINKED_ACCOUNT) breakdown | berkayildi MCP, Komiser | `cost --group-by account` | names resolved via Organizations, org rollup |
| 7 | Forecast next month | berkayildi MCP, CE GetCostForecast | `forecast` | cached; later compares forecast vs actual |
| 8 | Anomaly detection (CE native) | berkayildi MCP, aws-doctor | `anomalies` (CE GetAnomalies) | flagged inline; falls back gracefully if not subscribed |
| 9 | Period compare (what changed) | berkayildi MCP | `compare --from --to` | month-over-month deltas computed from store |
| 10 | Waste audit (idle EC2 / unattached EBS / orphaned snapshots / unassociated EIP) | aws-finops MCP, aws-doctor, Komiser | `waste idle\|ebs\|snapshots\|eip` | resource inventory + CloudWatch util join |
| 11 | Untagged resource / tag-coverage detection | Komiser | `waste untagged` | cost-attribution gap surfacing |
| 12 | Filter by date range / granularity | ccExplorer, CE | `--from`/`--to`/`--granularity DAILY\|MONTHLY` | preset shortcuts too |
| 13 | Filter by tag / cost category | CE GetCostAndUsage | `--tag k=v` / `--cost-category` | composable with group-by |
| 14 | EC2 spend analysis | aarora79 MCP | `cost --service "Amazon Elastic Compute Cloud - Compute"` | pairs with `waste` for spend+idle |
| 15 | Dimension value listing | CE GetDimensionValues | `dimensions <SERVICE\|LINKED_ACCOUNT\|REGION\|USAGE_TYPE>` | discover what exists before filtering |
| 16 | CSV export / report file (HTML+PDF) | ccExplorer, jamescarr | `report --format html\|pdf\|csv` | Chrome-headless PDF, smart sectioned HTML |
| 17 | Cost-optimization tips / line-item decoder | aws-samples cfm-tips MCP | `explain <usage-type\|service>` glossary | plain-English decode of confusing line items |
| 18 | Budget status | aws-finops MCP | `budget` (Budgets API read) `(stub — needs budgets:ViewBudget tier; ships behind doctor gate)` | honest "enable budgets tier" message if unscoped |
| 19 | `sync` populate local store | (Printing Press core) | `sync --from --to --granularity` with per-period cursor | incremental; saves repeat CE calls |
| 20 | `search` / `sql` over synced cost data | (Printing Press core) | FTS + SELECT-only SQL | offline analytics |

Stub note: only **#18 `budget`** may ship as a stub (honest "this needs the budgets read tier — run `iam-setup --tier budgets`" message), because Budgets is a separate IAM scope and not part of the core least-privilege ask. Everything else ships fully.

## Transcendence (only possible with our approach)

| # | Feature | Command | Score | Why Only We Can Do This | Persona |
|---|---------|---------|-------|------------------------|---------|
| 1 | Tiered IAM onboarding | `iam-setup` | 9/10 | Curated least-privilege JSON tiers + validated CloudFormation launch-stack minting `BillingReadOnly` + admin `bootstrap` — no incumbent treats IAM as a feature (`// pp:novel-static-reference`) | Priya / Dana |
| 2 | Permission-aware diagnosis | `doctor` | 8/10 | Maps live SDK AccessDenied codes → the exact missing `ce:`/`ec2:` IAM line + member-vs-management account detection (`// pp:client-call`) | Priya |
| 3 | Per-account + aggregate org rollup | `consolidated` | 8/10 | Local `cost_line × account` join: each linked account's bill (name-resolved) + management rollup + inline per-account MoM delta in one view | Dana |
| 4 | Dollar-ranked waste rollup | `waste rank` | 8/10 | Cross-join `resource × metric_sample × cost_line` → one table sorted by est. monthly $ wasted with a grand-total savings figure | Marcus |
| 5 | gp2→gp3 modernization savings | `waste gp2-gp3` | 7/10 | `ec2:DescribeVolumes` + static gp2/gp3 price table → exact $ saved per volume (the API returns volumes, not the conversion arithmetic) | Marcus |
| 6 | Data-transfer bleed surface | `waste transfer` | 7/10 | Filters synced `cost_line` to `*-DataTransfer-*`/`NatGateway` usage-types, ranks by spend with AZ/region attribution — the canonical confusing AWS cost pattern | Marcus / Dana |
| 7 | Deterministic bill Q&A router | `ask` | 6/10 | Routes known intents (top-N services, biggest MoM mover, per-account total, transfer spend) to SQLite; unmatched path emits the cost slice as JSON for `\| claude` | Dana |

Cross-cutting: **`--snark`** opt-in Corey-Quinn voice flag on human output (static phrase-bank keyed to deterministic signals; `// pp:novel-static-reference`; off by default; suppressed under `--json`).

## Out of scope (explicitly)
- Reserved-Instance / Savings-Plan purchase recommendations / coverage nags — the brief says waste-first, not buy-first.
- Any write/mutation to AWS (delete volume, stop instance). Read-only by design; `waste` *surfaces* candidates and prints the exact `aws`/console step, never executes it.
