# Printing Press Retro: AWS Billing Intelligence (aws-billing)

## Session Stats
- API: aws-billing (AWS Cost Explorer + Organizations + EC2/CloudWatch/S3 via AWS SDK for Go v2)
- Spec source: hand-authored internal YAML (synthetic, auth:none, SDK transport)
- Scorecard: 80/100 (Grade A, after polish)
- Verify pass rate: 100%
- Shipcheck: 6/6 legs PASS
- Fix loops: 2 (flag-name mismatch + data-pipeline sync flags; caching coverage bug; no-error-path-probe)
- Manual code edits: heavy (synthetic SDK CLI — all real commands hand-built on internal/awsx)
- Features built from scratch: 7 transcendence + onboarding + report/Slack + full SDK client layer

## Findings

### 1. Cloud/SDK credential-chain APIs have no auth mode → forced `auth: none` → phase5 promote-gate has no valid skip path when creds are unavailable in-session (Assumption mismatch / Skill instruction gap)
- **What happened:** AWS Cost Explorer is consumed via the AWS SDK for Go v2 (SigV4 + the native credential chain: env / shared profile / SSO / assume-role / IMDS). None of the Printing Press auth modes (api_key, bearer_token, cookie, composed, oauth, session_handshake, none) describe "credentials resolved out-of-band by an SDK." The only workable choice is `auth: none`. But `auth: none` tells the rest of the machine "freely testable, no creds needed" — which is false. Concretely it bit the **phase5 promote gate**: `phase5-skip.json` is rejected for an `auth:none` manifest ("auth type mismatch" / "no-auth APIs must produce acceptance"), so a synthetic/SDK CLI that genuinely cannot be live-tested without external credentials the agent can't obtain (no `aws sso login` from an automated session) has **no valid path to promote**. In this run I eventually obtained creds (aws-vault) and wrote a real acceptance — but the general case (creds unavailable in-session) is a dead end.
- **Scorer correct?** Partially. The gate is right that auth:none usually means testable; it's wrong that auth:none can never legitimately need out-of-band creds.
- **Root cause:** Auth-mode taxonomy has no `sdk`/`external-credentials`/`credential-chain` entry; the phase5 skip-marker logic keys validity off the manifest auth type and offers no skip for auth:none-but-needs-external-creds.
- **Cross-API check:** First sighting in this ecosystem. The class is obvious and recurring: AWS (all services via SDK + SigV4), GCP (Cloud Billing / google-cloud SDKs, ADC credential chain), Azure (Cost Management SDK, DefaultAzureCredential). Every cloud-provider CLI built on a vendor SDK lands here identically.
- **Frequency:** subclass:cloud-sdk-auth — every SDK/credential-chain CLI; rare today (1 catalog example), inevitable as cloud CLIs get printed.
- **Fallback if the Printing Press doesn't fix it:** Agent must obtain live creds in-session to write acceptance (often impossible: interactive SSO, MFA, no creds on CI), OR hand-edit the gate, OR leave the CLI stranded unpromoted. All three are bad.
- **Worth a Printing Press fix?** Yes, but P3 — thin current evidence (1 API), high future certainty.
- **Inherent or fixable:** Fixable. (a) Add an auth mode like `external`/`sdk` that signals "creds resolved out-of-band; live-test needs them." (b) Let the phase5 gate accept a skip for `kind: synthetic` (already a signal) or the new auth mode when creds are absent, with reason `auth_required_no_credential`, instead of demanding acceptance.
- **Durable fix:** Smallest increment: allow `phase5-skip.json` with `skip_reason: external_credentials_unavailable` to satisfy the promote gate when the manifest is `kind: synthetic` (or carries the new auth mode), instead of rejecting on auth-type mismatch. Larger: introduce the `external`/`sdk` auth mode end-to-end so doctor/README/auth surfaces describe credential-chain auth honestly.
- **Test:** positive — a `kind: synthetic` CLI with no resolvable creds can write a valid phase5-skip and promote; negative — a normal `auth: none` HTTP CLI still requires acceptance (skip rejected).
- **Evidence:** `lock promote` failed: `phase5 gate failed: phase5 skip marker auth type "oauth2" does not match manifest auth type "none"`. Session reproduced the dead-end before aws-vault creds were found.
- **Related prior retros:** None.

## Prioritized Improvements

### P3 — Low priority
| Finding | Title | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---------|-------|-----------|-----------|---------------------|------------|--------|
| 1 | phase5 skip path for synthetic/SDK CLIs that need out-of-band creds | scorer | subclass:cloud-sdk-auth | Low (agent often can't get creds in-session) | small (gate) / medium (new auth mode) | gate change limited to `kind: synthetic` or new auth mode; normal auth:none unaffected |

### Skip
| Finding | Title | Why it didn't make it |
|---------|-------|------------------------|
| S1 | data_source_strategy:local promoted commands error (exit 5) on empty store → fail verify pre-sync | Step B: can't name 3 catalog CLIs using data_source_strategy:local with evidence; partly my spec choice (I set local strategy). Real but under-evidenced — note for next local-strategy CLI. |
| S2 | `pp:no-error-path-probe` not surfaced in SKILL build checklist for free-text-arg novel commands (ask/explain) | Step G: annotation exists and is discoverable; thin, mostly a one-line doc nicety. Case-against (per-CLI discovery) stronger. |

### Dropped at triage
| Candidate | One-liner | Drop reason |
|-----------|-----------|-------------|
| Period-naive local cache | Wider period reused a narrower cached window | printed-CLI (my read helpers; fixed in-session with cachePeriodCovered) |
| Flag-name mismatch (--profile vs --profile-aws) in narrative | validate-narrative/verify-skill failed | iteration-noise (my research.json examples; fixed) |
| sync missing --full/--resources crashed data-pipeline check | verify "sync crashed" | printed-CLI (my hand-written sync; added the flags) |

## Work Units

### WU-1: phase5 promote-gate skip path for synthetic / external-credential CLIs (from F1)
- **Priority:** P3
- **Component:** scorer
- **Goal:** A `kind: synthetic` (or new `external`/`sdk` auth mode) CLI that cannot be live-tested without out-of-band credentials can write a valid `phase5-skip.json` and promote, instead of being rejected on auth-type mismatch.
- **Target:** the phase5 acceptance/skip gate in the promote + dogfood acceptance tooling; optionally a new auth-mode enum in spec-parser + auth templates.
- **Acceptance criteria:**
  - positive: a `kind: synthetic` CLI with `phase5-skip.json {skip_reason: external_credentials_unavailable}` promotes.
  - negative: a standard `auth: none` HTTP CLI still requires a passing `phase5-acceptance.json` (skip rejected).
- **Scope boundary:** Does NOT require building the full `external`/`sdk` auth mode; the minimal fix is the gate's skip-acceptance for synthetic CLIs. The auth-mode work is a larger follow-on.
- **Dependencies:** none
- **Complexity:** small (gate) / medium (if the new auth mode is included)

## Anti-patterns
- None observed that the machine caused. The synthetic-SDK build was heavy hand-work by design (SDK transport is out of generator scope), which is expected.

## What the Printing Press Got Right
- `kind: synthetic` cleanly relaxed dogfood/scorecard path-validity — exactly the right escape hatch for an SDK-backed CLI.
- The sibling-internal-package reimplementation carve-out + `// pp:client-call` / `// pp:novel-static-reference` markers let a 100%-hand-built command layer pass reimplementation checks without friction.
- Store typed-upsert tables generated from internal-YAML `types:` gave the SQLite cache + search + agent-native output for free.
- The novel-features subagent produced grounded, ruthlessly-cut transcendence features (7 survivors, real personas) that matched the user's vision.
- shipcheck umbrella + verify-mode short-circuit conventions made the offline verification path reliable once the AWS calls were guarded.
