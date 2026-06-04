# Home Health CLI

**One view of your whole home's air health — radon, mold, particulates, CO2, VOC — aggregated from every sensor on your LAN, with a local history no single vendor app gives you.**

Pulls AirThings (radon/VOC/mold), IQAir AirVisual (PM2.5/CO2/AQI) and MOCREO (room temp/humidity incl. crawlspace) into one local SQLite history, then answers mold and allergy questions across day/week/month/quarter/year/all-time.

## Install

The recommended path installs both the `home-air-health-pp-cli` binary and the `pp-home-air-health` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install home-health
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install home-health --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install home-health --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install home-health --agent claude-code
npx -y @mvanhorn/printing-press-library install home-health --agent claude-code --agent codex
```

### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/home-health-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

> **macOS only.** Credentials live in the macOS login Keychain, and the CLI reads them via `/usr/bin/security`. The sensor adapters and Keychain integration are not portable to Linux or Windows.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-home-air-health --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-home-air-health --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-home-air-health skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-home-air-health. The skill defines how its required CLI can be installed.
```

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/home-health-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`); credentials are read from the Keychain, so the MCP server inherits whatever sources you configured for the CLI.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and register it manually.

Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "home-health": {
      "command": "home-air-health-pp-mcp"
    }
  }
}
```

The MCP server reads the same macOS Keychain credentials the CLI uses; no env vars are required.

</details>

## Setup

Credentials for all three sources live in the **macOS login Keychain**. Each source is optional — only set up the ones whose sensors you own. The `internal/cred` package reads them by service name; the `account` field holds the non-secret identifier and the secret is prompted (never on the command line).

### AirThings (Wave Radon + Mini)

Cloud Consumer API. The account is your AirThings API **client_id**; the secret is the **client_secret**.

```bash
security add-generic-password -U -s home-health-airthings -a <client_id> -w
# (you'll be prompted for the client_secret)
```

### IQAir / AirVisual (Pro + Outdoor)

Cloud web API. The account is the **email** you log in with; the secret is your **password** (or API token).

> **Stability note:** this source uses `website-api.airvisual.com/v1`, the reverse-engineered internal endpoint behind IQAir's own web dashboard — not the published IQAir Developer API at `api.airvisual.com`. It carries no public SLA and could change or disappear without warning; if it does, only this source degrades (AirThings and MOCREO are unaffected) and `doctor` will report the failure.

```bash
security add-generic-password -U -s home-health-iqair -a <email> -w
# (you'll be prompted for the password / token)
```

### MOCREO (room sensors via hub)

Legacy cloud API. The account is your MOCREO **email**; the secret is your **password**.

```bash
security add-generic-password -U -s home-health-mocreo -a <email> -w
# (you'll be prompted for the password)
```

The MOCREO login token can also be managed through `home-air-health-pp-cli auth` once captured. Verify everything with:

```bash
home-air-health-pp-cli doctor
```

`doctor` reports each source as `configured (account=…)` or `not configured (optional)`. Having none configured is a warning, not a failure; at least one configured is healthy.

## Quick Start

```bash
# Pull the latest readings from every configured sensor into local history
home-air-health-pp-cli collect

# Whole-home mold view: rooms over 60% RH this week
home-air-health-pp-cli dashboard --period week --focus mold

# See every monitored room and its latest snapshot
home-air-health-pp-cli rooms

# Drill into one room's trend over a month
home-air-health-pp-cli sensor Crawlspace --period month

```

## How history works

`collect` populates a local SQLite history that `dashboard`, `rooms`, and `sensor` read from.

- **MOCREO** returns true server-side history. Backfill it once with `home-air-health-pp-cli collect --since 168h` (a week) — long windows fill immediately.
- **AirThings** and **IQAir/AirVisual** return only their **latest** snapshot. History for those accrues only as you `collect` repeatedly over time, so longer dashboard periods reflect whatever has been collected so far.

Because AirThings/IQAir are latest-only, run `collect` on a schedule. On macOS, a launchd agent firing every ~10 minutes keeps a dense history without manual runs. Example `~/Library/LaunchAgents/com.user.home-health-collect.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.user.home-health-collect</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/bin/home-air-health-pp-cli</string>
    <string>collect</string>
  </array>
  <key>StartInterval</key>
  <integer>600</integer>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
```

Load it with `launchctl load ~/Library/LaunchAgents/com.user.home-health-collect.plist`. (Adjust the binary path to wherever `home-air-health-pp-cli` is installed.)

## Commands

The product surface:

- **`home-air-health-pp-cli collect`** — Pull the latest readings from every configured source into the local history. `--since 168h` backfills MOCREO history. Idempotent.
- **`home-air-health-pp-cli dashboard`** — Whole-home air-health view over a period. `--period day|week|month|quarter|year|all`, `--focus mold|allergy|all`, plus `--room`, `--metric`, `--source` filters.
- **`home-air-health-pp-cli rooms`** — List every monitored room with its latest readings.
- **`home-air-health-pp-cli sensor <room>`** — Detailed readings for one room over a time window (`--period`).

Plus framework commands: `doctor`, `search`, `export`, `import`, `version`, `agent-context`, `which`. The MOCREO login token is managed via `home-air-health-pp-cli auth`. Raw per-endpoint commands are hidden — the unified surface above supersedes them.

### What "mold" and "allergy" mean

- **Mold focus** — humidity sustained at or above **60% RH** (the level at which mold grows), plus AirThings' native mold-risk index where available. Reported per room as the fraction of samples over the threshold.
- **Allergy focus** — PM2.5, VOC, and CO2 (and PM10 where present). PM2.5 is flagged above the EPA guideline of 12 µg/m³.
- **Radon** is always surfaced as a standalone hazard tile, flagged against the WHO action level of 100 Bq/m³.

### Finding the right command

```bash
home-air-health-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language query to the best matching command. Exit `0` means a match; exit `2` means none — fall back to `--help`.

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** — never prompts, every input is a flag
- **Pipeable** — `--json` output to stdout, errors to stderr
- **Filterable** — `--select room,metric,value` returns only the fields you need
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — `dashboard`, `rooms`, `sensor`, `search`, and `sql` read the local SQLite store
- **Agent-safe by default** — no colors or formatting unless `--human-friendly` is set
- **One-flag agent mode** — `--agent` expands to `--json --compact --no-input --no-color --yes`

```bash
# Mold risk this week, machine-readable, only the fields an agent needs
home-air-health-pp-cli dashboard --period week --focus mold --agent --select room,fraction,max_value
```

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
home-air-health-pp-cli doctor
```

Verifies configuration, the local cache, and — per source — whether AirThings, IQAir/AirVisual, and MOCREO credentials are configured in the Keychain. Add `--json` for a machine-readable report including the per-source `sources` map.

## Cookbook

```bash
# Is there mold risk anywhere right now? (humidity ≥60% RH this week)
home-air-health-pp-cli dashboard --period week --focus mold

# Mold risk in specific rooms only
home-air-health-pp-cli dashboard --period week --focus mold --room Bedroom,Crawlspace

# Allergy picture over a whole quarter (PM2.5 / VOC / CO2)
home-air-health-pp-cli dashboard --period quarter --focus allergy

# One room's full trend over the past month
home-air-health-pp-cli sensor Crawlspace --period month

# What rooms are monitored, and their latest numbers
home-air-health-pp-cli rooms

# Only the AirThings readings in the whole-home view
home-air-health-pp-cli dashboard --period week --source airthings

# Agent JSON: this month's mold signal, trimmed to the essential fields
home-air-health-pp-cli dashboard --period month --focus mold --agent --select room,fraction,max_value

# Backfill a week of MOCREO history, then look at the month
home-air-health-pp-cli collect --since 168h
home-air-health-pp-cli dashboard --period month --focus all
```

## Troubleshooting

**A source shows "not configured" but I own that sensor**
- Re-run the matching `security add-generic-password` command from [Setup](#setup); confirm the service name exactly (`home-health-airthings` / `home-health-iqair` / `home-health-mocreo`).
- `home-air-health-pp-cli doctor --json` shows the `sources` map with each account it resolved.

**Dashboard is empty or thin for long periods**
- AirThings and IQAir are latest-only; history accrues from repeated `collect` runs. Set up the launchd schedule in [How history works](#how-history-works).
- For MOCREO, backfill once with `home-air-health-pp-cli collect --since 168h`.

**`collect` skips a source**
- That source isn't configured (optional). Add its Keychain entry or ignore the skip if you don't own that sensor.

**Authentication errors (exit code 4)**
- Run `home-air-health-pp-cli doctor` to see which credentials resolved.

---

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-vendor health intelligence
- **`dashboard`** — One mold/allergy-focused view of your entire home's air, aggregated across every sensor brand, over any time window.

  _Reach for this when an agent needs the whole-home air-health picture or a mold/allergy risk assessment across rooms and vendors at once._

  ```bash
  home-air-health-pp-cli dashboard --period week --focus mold --agent
  ```
- **`sensor`** — Min/avg/max per metric for a single room over a window, regardless of which vendor's sensor covers it.

  _Use to investigate one room's humidity/temperature/air trend when diagnosing a mold or comfort issue._

  ```bash
  home-air-health-pp-cli sensor Crawlspace --period month --agent
  ```
- **`rooms`** — Every monitored room with its latest snapshot, merged across all sensor brands.

  _Quick orientation: what's monitored and how each room reads right now._

  ```bash
  home-air-health-pp-cli rooms --agent
  ```
