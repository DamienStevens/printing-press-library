# Shipcheck — aws-billing-pp-cli

Run: 20260529-075312 · spec: internal YAML (synthetic, auth:none, SDK transport)

## Verdict: hold (live-credential gate only — no code defect)

Structurally verified; live AWS path unverified because the agent cannot perform
the interactive `aws sso login` the AWS credential chain requires.

## Shipcheck umbrella: PASS (6/6 legs)
| leg | result |
|---|---|
| verify | PASS (100%, 0 critical) |
| validate-narrative | PASS (11/11 commands + full examples) |
| dogfood | PASS |
| workflow-verify | PASS |
| verify-skill | PASS |
| scorecard | PASS — 80/100 Grade A (after polish 75→80) |

## What was built
- **Transport:** native AWS SDK for Go v2 (`internal/awsx`) — SigV4 + full credential
  chain (env / profile / SSO / assume-role / IMDS). No `aws` CLI dependency. Marked `// pp:client-call`.
- **Onboarding (headline):** `iam-setup` (tiered least-privilege policy / CloudFormation / bootstrap),
  AWS-aware `doctor` (credential chain + member-vs-management detection + AccessDenied→exact-permission mapping).
- **Bill + store:** `sync` (Cost Explorer + Organizations + EC2/CloudWatch/S3 → SQLite),
  `bill` (group-by service/account/region/usage-type), `consolidated` (per-account + aggregate + MoM delta),
  `compare`, `forecast`, `dimensions`.
- **Waste-first:** `waste rank` (dollar-ranked rollup), `waste gp2-gp3`, `waste transfer` (NAT/data-transfer bleed),
  `waste idle|ebs|snapshots|eip`. Read-only — surfaces the remediation step, never executes it. RI/SP upsell out of scope.
- **Report + deliver:** `report` (HTML + Chrome-headless PDF) + `--post-slack` (delegates to working `slack-pp-cli`).
- **Ideal:** `explain` (usage-type glossary), `ask` (deterministic intent router over the cache).
- **Bonus:** `--snark` opt-in Corey-Quinn voice (static, suppressed under --json).

## Verified offline (agent)
- All 26 commands: `--help`, `--dry-run`, `--json`, verify-mode short-circuit (no AWS dial), exit codes.
- `explain`, `iam-setup` (policy/CFN/bootstrap) produce correct static output.
- `doctor` cleanly reports "no AWS credentials resolved — run aws sso login" (permission-aware path works).
- `report --post-slack` fast-fails with a clear error when no channel is set.
- go vet clean, go test green (incl. new awsx unit tests).

## NOT verified (needs live AWS session — the hold)
- Real Cost Explorer pull / breakdown / per-account rollup (needs management-account access).
- Waste hunters against a real account (dev is reachable for EC2/EBS/etc. once authenticated).
- Real Slack post of an HTML/PDF report.

## Shareability fix applied
`report --post-slack` no longer hardcodes a personal Slack DM. Target resolves
`--slack-channel` → `$AWS_BILLING_SLACK_CHANNEL` → else a clear error.

## Working copy
`~/printing-press/.runstate/cli-printing-press-70e28f12/runs/20260529-075312/working/aws-billing-pp-cli`
(not promoted to library — promote gate requires live acceptance for an auth:none manifest).
