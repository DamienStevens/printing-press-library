---
name: pp-home-health
description: "Whole-home air-health dashboard aggregating AirThings, IQAir/AirVisual and MOCREO sensors into one local history, focused on mold and allergy monitoring."
author: "Damien Stevens"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - home-health-pp-cli
---

# Home Health — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `home-health-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer:
   ```bash
   npx -y @mvanhorn/printing-press-library install home-health --cli-only
   ```
2. Verify: `home-health-pp-cli --version`
3. Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on `$PATH`.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.

Pulls AirThings (radon/VOC/mold), IQAir AirVisual (PM2.5/CO2/AQI) and MOCREO (room temp/humidity incl. crawlspace) into one local SQLite history, then answers mold and allergy questions across day/week/month/quarter/year/all-time.

## When to Use This CLI

Use to check whole-home air quality, assess mold risk from humidity trends by room, check radon levels, and review allergy-relevant particulates / VOC / CO2 over any time window — aggregated across AirThings, IQAir/AirVisual, and MOCREO sensors.

**Trigger phrases:** "check home air quality", "is there mold risk", "radon level", "home health dashboard", "use home-health", "air quality this week", "which room is most humid", "particulates / PM2.5 at home".

**Not for:** HVAC or thermostat control, smart-home automation, or non-air sensors (water, motion, door, energy). This CLI only reads air-quality and temp/humidity sensor data.

## How sources work

All three sources are **optional**; the CLI works with whatever subset is configured in the Keychain.

- **MOCREO** has true server-side history — `collect --since 168h` backfills a week immediately.
- **AirThings** and **IQAir/AirVisual** return only their latest snapshot, so their history accrues from repeated `collect` runs (a launchd schedule keeps it dense — see README).

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Cross-vendor health intelligence
- **`dashboard`** — One mold/allergy-focused view of your entire home's air, aggregated across every sensor brand, over any time window.

  _Reach for this when an agent needs the whole-home air-health picture or a mold/allergy risk assessment across rooms and vendors at once._

  ```bash
  home-health-pp-cli dashboard --period week --focus mold --agent
  ```
- **`sensor`** — Min/avg/max per metric for a single room over a window, regardless of which vendor's sensor covers it.

  _Use to investigate one room's humidity/temperature/air trend when diagnosing a mold or comfort issue._

  ```bash
  home-health-pp-cli sensor Crawlspace --period month --agent
  ```
- **`rooms`** — Every monitored room with its latest snapshot, merged across all sensor brands.

  _Quick orientation: what's monitored and how each room reads right now._

  ```bash
  home-health-pp-cli rooms --agent
  ```

## Command Reference

- `home-health-pp-cli collect` — Pull the latest readings from every configured source into the local SQLite history. `--since 168h` backfills MOCREO. Idempotent.
- `home-health-pp-cli dashboard` — Whole-home view over a period. `--period day|week|month|quarter|year|all`, `--focus mold|allergy|all`, with `--room`, `--metric`, `--source` filters.
- `home-health-pp-cli rooms` — List every monitored room with its latest readings.
- `home-health-pp-cli sensor <room>` — Detailed readings for one room over a window (`--period`).
- `home-health-pp-cli doctor` — Verify config, cache, and per-source credential status.
- `home-health-pp-cli auth` — Manage the MOCREO login token (other sources read from the Keychain).
- `home-health-pp-cli search` / `export` / `import` — Query, back up, or load the local history.

**Mold** = humidity ≥60% RH exceedance (plus AirThings mold index). **Allergy** = PM2.5 / VOC / CO2. **Radon** is a standalone hazard tile (WHO action level 100 Bq/m³).

### Finding the right command

```bash
home-health-pp-cli which "<capability in your own words>"
```

Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help`.

## Recipes

### Mold risk this week

```bash
home-health-pp-cli dashboard --period week --focus mold
```

Flags rooms with sustained humidity at or above 60% RH plus any AirThings mold index.

### Allergy load over a quarter

```bash
home-health-pp-cli dashboard --period quarter --focus allergy
```

PM2.5, VOC and CO2 trends across the home for allergy sufferers.

### Agent-native mold summary

```bash
home-health-pp-cli dashboard --period month --focus mold --agent --select flags,mold
```

Structured JSON narrowed to the flags and per-room mold exceedance for an agent to act on.

### One room's full trend

```bash
home-health-pp-cli sensor Kitchen --period year
```

Min/avg/max per metric for a single room over the year.

### Backfill MOCREO history

```bash
home-health-pp-cli collect --since 168h
```

MOCREO keeps server-side history; pull the last week in one shot.

## Setup

Credentials live in the macOS Keychain (set up only the sensors you own):

```bash
security add-generic-password -U -s home-health-airthings -a <client_id> -w
security add-generic-password -U -s home-health-iqair    -a <email>     -w
security add-generic-password -U -s home-health-mocreo   -a <email>     -w
```

Run `home-health-pp-cli doctor` to confirm which sources resolved. See the README for the full setup and the launchd auto-collect schedule.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields, critical for keeping context small:

  ```bash
  home-health-pp-cli dashboard --period week --focus mold --agent --select room,fraction,max_value
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — `dashboard`, `rooms`, `sensor`, `search`, `sql` read the local SQLite store
- **Non-interactive** — never prompts, every input is a flag

## Agent Feedback

When something about this CLI surprises you, record it:

```
home-health-pp-cli feedback "dashboard --focus allergy omitted PM10 for an outdoor sensor"
home-health-pp-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.local/share/home-health-pp-cli/feedback.jsonl` and never sent unless explicitly opted in. Write what *surprised* you, one line — that is the part that compounds.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `home-health-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

Install the MCP binary from this CLI's published public-library entry or pre-built release, then register it:

```bash
claude mcp add home-health-pp-mcp -- home-health-pp-mcp
```

The MCP server reads the same macOS Keychain credentials the CLI uses; no env vars required. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which home-health-pp-cli`. If not found, offer to install (see Prerequisites).
2. Match the user query to the best command from the Command Reference and Recipes above.
3. Execute with the `--agent` flag:
   ```bash
   home-health-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `home-health-pp-cli <command> --help`.
