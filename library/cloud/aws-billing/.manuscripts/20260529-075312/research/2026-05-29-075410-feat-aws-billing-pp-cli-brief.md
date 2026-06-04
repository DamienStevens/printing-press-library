# AWS Billing Intelligence CLI Brief

## API Identity
- **Domain:** AWS Cost Management — primarily Cost Explorer (`ce`), with AWS Organizations (account-name mapping) and per-account resource inventory (EC2 / EBS / CloudWatch / S3) for waste detection.
- **Users:** AWS account owners / finance / FinOps-curious engineers who want to *understand and trim* a bill, not get upsold Reserved Instances. Explicitly meant to be **shared with non-experts** — onboarding friction is the make-or-break.
- **Data profile:** Time-series cost amounts grouped by dimension (LINKED_ACCOUNT, SERVICE, REGION, USAGE_TYPE, tag). Forecasts. Resource inventory snapshots (instances, volumes, snapshots, addresses) joined against CloudWatch utilization. All read-only.

## Transport & Auth (decided)
- **Native AWS SDK for Go v2** (`config`, `costexplorer`, `organizations`, `ec2`, `cloudwatch`, `s3`). Public modules. Does SigV4 + the full credential chain (env → shared profile → SSO → assume-role → IMDS) for free. **No `aws` CLI dependency** → shareable single binary.
- The generated HTTP client is unused for AWS calls; every AWS-touching command is hand-built and marked `// pp:client-call` (real external SDK call).
- Internal YAML spec scaffolds with `auth.type: none`; `doctor` and `auth` are hand-customized to validate the AWS credential chain + billing reachability.

## IAM Onboarding (HEADLINE feature — the #1 adoption lever)
IAM friction is the stated #1 reason people won't adopt. Ship a tiered, copy-paste-or-one-click path:
- **Core billing policy** (tiny): `ce:GetCostAndUsage`, `ce:GetCostForecast`, `ce:GetDimensionValues`, `ce:GetCostAndUsageWithResources`, `ce:GetTags`, `ce:GetCostCategories`, `ce:GetCostCategories`, `organizations:ListAccounts`, `organizations:DescribeOrganization`. Org-wide bill ⇒ must live in the **management/payer account**.
- **Optional waste-scan policy**: adds `ec2:Describe{Instances,Volumes,Snapshots,Addresses}`, `cloudwatch:GetMetricStatistics`, `s3:ListAllMyBuckets`, `s3:GetBucketLocation`. Works in **any** account (the dev/prod member accounts today).
- Delivery: `iam-setup` command emits (a) ready-to-paste JSON policy, (b) a one-click **CloudFormation** template (launch-stack URL + raw template) that mints a `BillingReadOnly` role/user, and (c) an admin `bootstrap` path for someone who already has Administrator. Plus `doctor` tells you exactly which permission is missing and how to add it.

## Reachability Risk
- **Low for the product, account-dependent for org-wide CE.** the dev/prod member accounts are member accounts; org-wide `ce:` requires management-account read access. `ec2`/`cloudwatch`/`s3` describes are reachable in member accounts now. Live testing: refresh `aws sso login`, test waste-hunters against a member account immediately; CE path validates when management access lands. No bot-protection — it's signed AWS API.

## Top Workflows
1. **Pull this month's bill, broken down per-account (org) AND per-service, with deltas vs last month.**
2. **Hunt waste** — idle/stopped-but-paid EC2, unattached EBS volumes, orphaned snapshots, unassociated Elastic IPs, gp2→gp3 candidates, S3 storage-class drift, NAT-gateway/data-transfer bleed.
3. **Auto-report**: render HTML + PDF, post a smart digest to Slack DM (delegates to working `slack-pp-cli`).
4. **Explain** a confusing line item ("what the hell is `EUC1-DataTransfer-Out-Bytes`?").
5. **Ask** a natural-language question about the synced bill.

## Table Stakes (must-match, from incumbents)
- Total cost + breakdown by service (aws-cost-cli, ccExplorer)
- Slack delivery (aws-cost-cli)
- Scheduled/daily report (aws-cost-cli GH Action) → our `report` + cron guidance
- Filters: by service / account / region / tag / date range / granularity (DAILY/MONTHLY)
- CSV/JSON output (ccExplorer)
- Idle/orphaned resource detection + anomaly flags (aws-doctor, Komiser)
- Multi-account view (Komiser)
- Forecast (CE GetCostForecast)

## Data Layer (SQLite — this is what makes transcendence possible)
- **Primary entities:** `cost_line` (period, account_id, service, region, usage_type, amount, unit, granularity), `account` (id, name, status, joined), `resource` (type, id, account, region, state, attrs json, monthly_cost_est), `metric_sample` (resource_id, metric, avg, max, period), `forecast`.
- **Sync cursor:** per (granularity, dimension, period) so re-sync only fetches new months — **saves $0.01/CE call** by querying the cache, not the API.
- **FTS/search:** over service names, usage-types, resource ids/tags, and an `explain` glossary table for AWS line-item decoding.

## Why install this over the incumbents
- **Onboarding that doesn't make you quit** — the only one treating least-privilege IAM as a first-class, one-click feature.
- **Offline & cheap** — sync once into SQLite, then compare/ask/report without paying per CE call.
- **Waste-first, not upsell-first** — finds money you're *wasting*, not commitments to *buy*.
- **Agent-native** — `--json`/`--select`/MCP surface so an agent can answer "why did my bill jump?" end-to-end.
- **Self-contained binary**, no `aws` CLI needed; same UX whether you share it with a colleague or a client.

## User Vision (verbatim drivers)
- "least privilege IAM (or other as IAM is a pain)" → tiered policy + CFN + bootstrap, native SDK so no key-minting where avoidable.
- "break down what it means for each charge (by Organization and aggregate)" → per-LINKED_ACCOUNT + rollup.
- "compare/contrast with other months" → SQLite month-over-month deltas.
- "spot waste in storage, long-running instances… more than being recommended to buy the 1 yr EC2 instance" → waste-hunters, RI/SP nags explicitly out of scope.
- "Minimum: auto analyze bill and post analysis to shared slack channel with HTML/PDF with smart breakdowns" → `report --post-slack` (a Slack DM channel) + HTML/PDF.
- "Ideal: reply to ask questions about our bill (or confusing AWS services)" → `ask` + `explain`.
- "Bonus: Corey Quinn swagger" → opt-in `--snark` voice layer.

## Product Thesis
- **Name:** AWS Billing Intelligence — slug `aws-billing`, binary `aws-billing-pp-cli`.
- **Why it should exist:** Every AWS cost tool assumes you already cleared the IAM hurdle and already know what `DataTransfer-Out-Bytes` means. This one gets a non-expert from zero to a Slack-delivered, plain-English, waste-flagged bill breakdown — and is cheap to run because it caches.

## Build Priorities
1. **Onboarding + transport spine:** native SDK credential chain, `doctor` (permission-aware), `iam-setup` (policy + CFN + bootstrap). Without this nothing else is reachable for a shared user.
2. **Bill pull + store:** `sync`, `bill`/`cost` (by account + service + region + usage-type, date range, granularity), `compare` (month-over-month), `forecast`. SQLite cache.
3. **Waste-hunters:** `waste` (idle EC2, unattached EBS, orphaned snapshots, unassociated EIPs, gp2→gp3, S3 class drift, NAT/data-transfer bleed) — testable against a member account now.
4. **Report + deliver:** `report` (HTML+PDF, smart breakdowns) → `--post-slack` via slack-pp-cli.
5. **Explain + Ask:** glossary decoder + NL Q&A over the synced store.
6. **Snark:** `--snark` Corey-Quinn voice flag across human output.
