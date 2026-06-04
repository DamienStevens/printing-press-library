# Novel Features Brainstorm — aws-billing-pp-cli

(Subagent audit trail. Customer model + killed candidates persisted per Phase 1.5c.5 contract.)

## Customer model

**Dana — AWS account owner / fractional ops lead (the sharer)**
Owns the payer account for a small org (dev and prod member accounts under a management account). Not a FinOps specialist; wants to understand and trim the bill, hates RI upsell.
- Today: AWS Console Cost Explorer, stacked bar chart, CSV export, fights date pickers, screenshots into Slack.
- Weekly ritual: Monday "is the bill creeping?" check — this-month-so-far vs last month per service; forwards a screenshot.
- Frustration: CE charges $0.01/call, console is read-only-to-her-eyes; can't ask "what changed and why." Sharing = screenshots, not a runnable tool.

**Marcus — FinOps-curious backend engineer (the waste hunter)**
Admin on member accounts (a member account today), `aws sso login` in terminal. Suspects idle instances + orphaned volumes bleeding money.
- Today: ad-hoc `aws ec2 describe-instances`, manual CloudWatch CPU cross-ref, mental EBS math, gives up halfway.
- Weekly ritual: pre-bill "clean up the dev account" sweep — kill stopped-but-paid instances, delete unattached volumes, prune snapshots.
- Frustration: no single command says "here's idle/orphaned/over-provisioned ranked by $ wasted." Incumbents need their own infra/dashboards; he wants a terminal answer.

**Priya — non-expert colleague Dana shares the binary with (the onboarding test case)**
Handed the binary + "just run this." Doesn't know IAM. Either running in 5 minutes or the tool is dead to the team.
- Today: bounces off any "create an IAM policy with 9 actions" instruction; doesn't know what a management account is.
- Weekly ritual: none yet — she's the adoption gate.
- Frustration: IAM. The stated #1 adoption killer.

## Survivors (transcendence rows)

| # | Feature | Command | Score | How It Works | Evidence |
|---|---------|---------|-------|-------------|----------|
| 1 | Tiered IAM onboarding | `iam-setup` | 9/10 | Least-privilege JSON tiers + validated CloudFormation launch-stack URL minting `BillingReadOnly` + admin `bootstrap`, via `// pp:novel-static-reference` | Brief names IAM as #1 adoption killer; sole differentiator vs incumbents |
| 2 | Permission-aware diagnosis | `doctor` | 8/10 | Real SDK calls (`// pp:client-call`) map AccessDenied → exact missing `ce:`/`ec2:` line + member-vs-management check | Build Priority 1; persona Priya |
| 3 | Per-account + aggregate org rollup | `consolidated` | 8/10 | Joins local `cost_line × account` for names, inlines per-account MoM delta + management rollup | Top Workflow #1 + verbatim "by Organization and aggregate"; Dana |
| 4 | Dollar-ranked waste rollup | `waste rank` | 8/10 | Cross-joins `resource × metric_sample × cost_line`, sorted by est. monthly $ wasted | Top Workflow #2; cross-join "makes transcendence possible"; Marcus |
| 5 | gp2→gp3 modernization savings | `waste gp2-gp3` | 7/10 | `ec2:DescribeVolumes` + static gp2/gp3 price table → exact $ saved per volume | Build Priority 3; testable against a member account now |
| 6 | Data-transfer bleed surface | `waste transfer` | 7/10 | Filters synced `cost_line` to `*-DataTransfer-*`/`NatGateway` usage-types, ranks with AZ/region attribution | Build Priority 3 NAT/transfer bleed; Top Workflow #4 |
| 7 | Deterministic bill Q&A router | `ask` | 6/10 | Routes known intents (top-N, biggest MoM mover, per-account total) to SQLite; unmatched emits cost slice as JSON for `\| claude` | Top Workflow #5 + verbatim "ask questions about our bill"; Dana |

Plus `--snark` (Corey-Quinn voice) as a cross-cutting opt-in flag on human output — static phrase-bank keyed to deterministic signals (`// pp:novel-static-reference`), off by default, suppressed under `--json`. Not a standalone command.

## Killed candidates

| Feature | Kill reason | Closest-surviving-sibling |
|---|---|---|
| `compare` standalone | Duplicate of manifest #9 | `consolidated` (embeds delta column; defers full diff to manifest `compare`) |
| `waste` family standalone | Duplicate of manifest #10 | `waste rank` (aggregates hunters) |
| `explain` standalone | Duplicate of manifest #17 | `ask` (routes glossary intents) |
| `report`/Slack/HTML-PDF standalone | Duplicate of manifest #3/#16 | `waste rank` (table feeding the report) |
| `anomalies baseline` | Marginal over "biggest MoM mover"; needs ≥3mo synced history (data-floor) | `consolidated` delta / `compare` |
| `freetier` | CE lacks clean allowance data; account-age-dependent; reimplementation risk | `waste gp2-gp3` |
| `coverage` (RI/SP) | Explicitly out of scope per brief (anti-upsell) | `waste transfer` (waste-first) |
| `sync --cursor` standalone | Infrastructure of `sync`, not a surface | folded into `sync` |
| `--snark` as command | Belongs as opt-in flag | `--snark` flag |
