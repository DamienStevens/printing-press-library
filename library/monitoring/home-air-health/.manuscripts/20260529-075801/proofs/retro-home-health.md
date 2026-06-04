# Printing Press Retro: home-health

## Session Stats
- API: home-health (synthetic combo: AirThings + IQAir/AirVisual + MOCREO)
- Spec source: MOCREO legacy Swagger 2.0 (primary scaffold) + hand-built sibling sources
- Scorecard: 89/100 (Grade A)
- Shipcheck: 6/6 legs PASS; live dogfood 49/49
- Fix loops: ~2
- Manual code edits: substantial (synthetic combo — 3 hand-built source clients + unified readings store + dashboard)
- Features built from scratch: collect, dashboard, rooms, sensor + internal/{source,readings,cred}

## Findings

### 1. Generated `TestUpsertBatch_Sets<Name>ParentID` fails when the dependent sub-resource's typed parent FK column is `NOT NULL` (template gap)
- **What happened:** The CLI generated from MOCREO's `/nodes/{node_id}/samples` sub-resource shipped with a FAILING `go test ./...` — `TestUpsertBatch_SetsSamplesParentID`. The `samples` typed table has both a `nodes_id` column (from the spec's required `node_id` path param → emitted `NOT NULL`) and the dependent-sync-added nullable `parent_id`. The generated test injects only `parent_id` (`{"id":"child-001","parent_id":"parent-A"}`), so the typed `INSERT` violates `nodes_id NOT NULL`, the typed upsert silently falls back to the generic resources table, and the test's `parent_id` assertion fails (0 rows).
- **Scorer correct?** N/A (not a scorecard penalty; it's a shipped failing unit test the generator emits).
- **Root cause (TWO candidates — disambiguate before fixing):**
  - (a) **Generated-test bug** (`internal/generator/templates/store_upsert_batch_test.go.tmpl`): the test injects only `parent_id`, not the typed parent FK (`nodes_id`). Real dependent sync DOES fill the typed FK (`obj[parentFKKey] = parentIDJSON`, generator_test.go:5051), so the test under-specifies vs. the runtime. Fix: have the test also inject the typed FK column.
  - (b) **Upsert-robustness gap** (the store `upsert<Name>Tx` template): when the typed parent FK is `NOT NULL` and a row arrives with only `parent_id`, the insert dies. Fix: coalesce the typed FK from `parent_id` when absent (this is the printed-CLI fix that was applied here and made the test pass).
  - **Disambiguation:** if real sync can ever produce a sub-resource row lacking the typed FK, fix (b) (runtime robustness). If sync always fills it, fix (a) (the test is simply wrong/under-specified). Evidence so far points to "sync always fills it" → (a) is the minimal correct fix, but (b) is defense-in-depth.
- **Why it didn't surface in the generator's own suite:** the generator's `messages`/`channels` fixtures emit a **nullable** parent FK, so their `TestUpsertBatch_SetsMessagesParentID` passes. The failure needs a parent path param that becomes a `NOT NULL` typed column — which Swagger-2.0 path params are (required by default).
- **Frequency:** subclass — dependent sub-resources whose typed parent FK is `NOT NULL`. High incidence in Swagger-2.0-converted specs (path params required by default).
- **Blast radius (Step B, with evidence):** the spec class is named in `internal/openapi/swagger2.go` itself — "Real-world Swagger 2.0 specs (Tripletex, NetSuite REST, Salesforce Tooling)." Any of these with a `/parent/{id}/child` sub-resource generates a `NOT NULL` parent FK and the same failing test. (Honest caveat: not generated end-to-end to confirm per-API; the mechanism is structural and reproduced cleanly on the MOCREO Swagger-2.0 spec.)
- **Counter-check (Step C):** Fixing the generated test to inject the typed FK cannot hurt nullable-FK CLIs (extra column value is harmless). The upsert-coalesce variant is also safe (only fills when absent). No guard needed.
- **Fallback if not fixed:** every Swagger-2.0 nested-sub-resource CLI ships a red `go test ./...`, which blocks the publish gate's cleanliness and reads as "broken" to anyone running tests. Claude catching+fixing this per-CLI is unreliable (it's a generated test most agents won't read).
- **Step G (case against):** "It's narrow — only Swagger-2.0 + required-path-param sub-resources; the runtime sync path is actually fine; the home-health fix was a one-line printed-CLI coalesce." Why the case-against fails: the artifact is a **shipped failing unit test the generator itself emits**, reproducible from a documented spec class (swagger2 targets), not a per-API quirk — and a red `go test` is a publish-quality regression for every CLI in that class.
- **Durable fix:** primary — in `store_upsert_batch_test.go.tmpl`, inject the typed parent FK column alongside `parent_id` (mirror what dependent sync writes). Optional defense-in-depth — in the store upsert template, coalesce the typed FK from `parent_id` when the object omits it. Parameterized by the profiler's `DependentSyncResources` parent-FK name; no API-specific values.
- **Test:** generate a CLI from a spec with a required-path-param sub-resource (or add a NOT-NULL-parent-FK fixture to generator_test.go) and assert `go test ./...` passes (positive); keep the existing nullable-FK `messages` fixture passing (negative/no-regression).
- **Evidence:** `live smoke` showed `samples items: typed-table upsert failed; generic resources rows preserved`; `go test ./...` failed `TestUpsertBatch_SetsSamplesParentID` until a coalesce fix was applied to the printed CLI.
- **Related prior retros:** None.

## Prioritized Improvements

### P2 — Medium priority
| Finding | Title | Component | Frequency | Fallback Reliability | Complexity | Guards |
|---------|-------|-----------|-----------|---------------------|------------|--------|
| 1 | Generated parent-FK upsert test fails on NOT-NULL typed FK (Swagger-2.0 sub-resources) | generator | subclass: NOT-NULL parent FK | low (agents don't read generated tests) | small | none needed |

### Skip
| Finding | Title | Why it didn't make it |
|---------|-------|------------------------|
| S1 | SKILL recipe for synthetic combo with heterogeneous per-source auth (sibling `internal/source/*` + Keychain cred helper + unified readings store) | Step B: couldn't name 3 catalog APIs needing this exact shape with evidence; it's custom orchestration → belongs as a SKILL recipe at most, and the adjacent AWS-SDK synthetic pattern is already memory-documented. Revisit if a 2nd heterogeneous-auth combo appears. |

### Dropped at triage
| Candidate | One-liner | Drop reason |
|-----------|-----------|-------------|
| D1 | Add AirThings/IQAir/MOCREO catalog entries | printed-CLI / fails catalog rubric (niche, reverse-engineered personal IoT) |
| D2 | `auth` command is MOCREO-specific in a multi-source CLI | printed-CLI (hand-relabeled; product-specific) |
| D3 | `novel_features` empty until added late → validate transcendence failed | iteration-noise (artifact of hand-driving a synthetic combo past the absorb gate, not a machine gap) |
| D4 | MOCREO login returns `result.access_token` not `result.token` per its own spec | API-quirk (MOCREO's spec is wrong; not the Printing Press) |

## Work Units

### WU-1: Generated parent-FK upsert test handles NOT-NULL typed parent FK (from F1)
- **Priority:** P2
- **Component:** generator
- **Goal:** A CLI generated from a sub-resource whose typed parent FK is `NOT NULL` ships a passing `go test ./...`.
- **Target:** `internal/generator/templates/store_upsert_batch_test.go.tmpl` (primary); optionally the store upsert template (`upsert<Name>Tx`) for runtime coalesce.
- **Acceptance criteria:**
  - positive: generate from a fixture with a required-path-param sub-resource (NOT-NULL typed parent FK) → `TestUpsertBatch_Sets<Name>ParentID` passes.
  - negative: existing nullable-parent-FK fixture (`messages`/`channels`) still passes — no regression.
- **Scope boundary:** Does not change the dependent-sync runtime (it already fills the typed FK). Only the generated test (and optionally the upsert's coalesce safety net).
- **Dependencies:** none
- **Complexity:** small

## Anti-patterns
- None observed worth machine action.

## What the Printing Press Got Right
- **Swagger 2.0 → OpenAPI 3 conversion** worked transparently for the MOCREO legacy spec — all 8 quality gates passed on first generation.
- **The scaffold is genuinely reusable beyond pure-spec CLIs:** store framework, MCP cobra-tree mirror, `cliutil` (ratelimit/extractnumber/fanout/text), search/sql, config, doctor — all directly usable by hand-built sibling sources, which made the synthetic combo viable.
- **`dogfood` novel-feature sync** (manifest + README + SKILL + root help from `novel_features_built`) made the transcendence-recording fix one step.
- **`kind: synthetic`** existed for exactly this multi-source-aggregator case.
- **shipcheck + live-dogfood caught real bugs** (empty-window array shape, 1-based pagination, NOT-NULL upsert) that unit tests alone missed.
