# Jane App CLI

**Book, view, and manage your Jane (janeapp.com) appointments across every clinic from one terminal — with a unified agenda, next-opening finder, and availability watch the patient portal can't do.**

Jane is the booking platform behind thousands of physio, massage, chiro, and wellness clinics, but each clinic is a separate subdomain with its own login and no public API. This CLI holds a profile per clinic, imports the session from a browser where you're already logged in (Jane gates password login behind reCAPTCHA), and unifies booking, viewing, and managing appointments across all of them. `agenda` merges every booking into one view; `next-opening` pages past Jane's 7-day availability cap to find the soonest slot.

Learn more at [Jane App](https://jane.app).

Created by [@omarshahine](https://github.com/omarshahine) (Omar Shahine).

## Install

The recommended path installs both the `janeapp-pp-cli` binary and the `pp-janeapp` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install janeapp
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install janeapp --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install janeapp --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install janeapp --agent claude-code
npx -y @mvanhorn/printing-press-library install janeapp --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.4 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/health/janeapp/cmd/janeapp-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/janeapp-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install janeapp --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-janeapp --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-janeapp --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install janeapp --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local browser session — set it up first if you haven't:

```bash
janeapp-pp-cli auth login --chrome
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/janeapp-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/health/janeapp/cmd/janeapp-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "janeapp": {
      "command": "janeapp-pp-mcp"
    }
  }
}
```

</details>

## Authentication

Each Jane clinic is its own subdomain with a separate patient account. **Jane gates
username/password login behind reCAPTCHA**, which a CLI can't solve, so this tool
authenticates by importing the session from a browser where you're already logged in.

Register a clinic, log in to it once in your browser, then import the session:

```bash
clinic add <name> --url=https://<clinic>.janeapp.com
auth login --clinic <name> --chrome            # read the session cookie from Chrome
# or, if you don't want a cookie tool touching the keychain:
auth login --clinic <name> --cookies-file <file>   # import an exported cookie
```

`--chrome` needs a cookie reader (`pycookiecheat`, `cookies`, or `cookie-scoop-cli`).
For `--cookies-file`, copy the `_front_desk_session` cookie for the clinic from Chrome
DevTools (Application → Cookies) into a file as `_front_desk_session=<value>`. Repeat
per clinic; read commands accept `--all-clinics`. When a session expires, re-import it.

## Quick Start

```bash
# Register a clinic; each keeps its own subdomain and session.
janeapp-pp-cli clinic add embophysio --url=https://embophysio.janeapp.com

# Log in to embophysio in Chrome, then import that session (reCAPTCHA already solved).
janeapp-pp-cli auth login --clinic embophysio --chrome

# View your upcoming appointments at that clinic.
janeapp-pp-cli appointments upcoming --clinic embophysio

# List practitioners so you have IDs for availability and booking.
janeapp-pp-cli staff --clinic embophysio

# Find the soonest bookable slot, paging past the 7-day window.
janeapp-pp-cli next-opening --clinic embophysio --treatment 1 --staff 1

# Every appointment across every logged-in clinic in one view.
janeapp-pp-cli agenda

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Cross-clinic
- **`agenda`** — See every appointment across every Jane clinic you use in one chronological view.

  _One call answers 'what do I have coming up anywhere' instead of logging into each clinic portal separately._

  ```bash
  janeapp-pp-cli agenda --agent
  ```
- **`conflict-check`** — Before booking, warn if a candidate slot collides with an existing appointment at another clinic.

  _Prevents double-booking yourself across different clinics._

  ```bash
  janeapp-pp-cli conflict-check --at 2026-07-15T09:00:00 --duration 60
  ```

### Availability intelligence
- **`next-opening`** — Find the soonest available slot for a practitioner + treatment, paging past Jane's 7-day availability cap.

  _Answers 'when is the earliest I can get in' without clicking week-by-week through the portal._

  ```bash
  janeapp-pp-cli next-opening --clinic embophysio --treatment 1 --staff 1
  ```
- **`watch`** — Poll availability and alert when an earlier slot than a target opens up.

  _Catches cancellations that free up an earlier appointment._

  ```bash
  janeapp-pp-cli watch --clinic embophysio --treatment 1 --staff 1 --before 2026-08-01
  ```

## Recipes

### Unified agenda across clinics

```bash
janeapp-pp-cli agenda --agent --select clinic,date,start_at,practitioner,treatment
```

Every upcoming appointment from every logged-in clinic, narrowed to the fields an agent needs.

### Earliest slot with a specific PT

```bash
janeapp-pp-cli next-opening --clinic embophysio --treatment 1 --staff 1
```

Stitches 7-day windows until it finds the first available opening.

### Watch for an earlier slot

```bash
janeapp-pp-cli watch --clinic embophysio --treatment 1 --staff 1 --before 2026-08-01
```

Polls availability and reports when something opens up before your target date.

### Dry-run a booking

```bash
janeapp-pp-cli book --clinic embophysio --treatment 1 --staff 1 --location 1 --at 2026-07-15T09:00:00
```

Shows exactly what would be booked without writing anything to Jane (dry-run by default; add --confirm to submit).

## Usage

Run `janeapp-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data: `credentials.toml`, `data.db`, cookies, browser-session proof files, and other auth sidecars |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `JANEAPP_CONFIG_DIR`, `JANEAPP_DATA_DIR`, `JANEAPP_STATE_DIR`, or `JANEAPP_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `JANEAPP_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export JANEAPP_HOME=/srv/janeapp
janeapp-pp-cli doctor
```

Under `JANEAPP_HOME=/srv/janeapp`, the four dirs resolve to `/srv/janeapp/config`, `/srv/janeapp/data`, `/srv/janeapp/state`, and `/srv/janeapp/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "janeapp": {
      "command": "janeapp-pp-mcp",
      "env": {
        "JANEAPP_HOME": "/srv/janeapp"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `JANEAPP_DATA_DIR` overrides an explicit `--home` for that kind. Use `JANEAPP_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `JANEAPP_HOME` does not move files back to platform defaults, and `doctor` cannot find credentials left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. On the first auth write, stored secrets leave `config.toml` and are consolidated into `credentials.toml` under the data directory. Run `janeapp-pp-cli doctor --fail-on warn` to check path and credential-location warnings in automation.

## Commands

### appointments

Your own appointments at the clinic (requires a logged-in session).

- **`janeapp-pp-cli appointments`** - List your upcoming and past appointments for the active profile.

### disciplines

Disciplines (categories of care) offered by the clinic, e.g. Physical Therapy.

- **`janeapp-pp-cli disciplines`** - List disciplines (service categories) with descriptions.

### locations

Clinic locations (address, phone, booking URL) for the active profile's Jane instance.

- **`janeapp-pp-cli locations`** - List clinic locations with address, contact info, and booking URL.

### openings

Live availability (openings) for a practitioner + treatment at a location.

- **`janeapp-pp-cli openings`** - List available appointment openings for a staff member + treatment at a location over a date window. Jane caps num_days at 1..7; the CLI's next-opening/watch commands page across multiple windows automatically.

### staff

Practitioners (staff members) and the treatments they offer.

- **`janeapp-pp-cli staff`** - List practitioners, their bookable treatment IDs, and online-booking availability.

### treatments

Bookable treatments/services with price, duration, and online-booking eligibility.

- **`janeapp-pp-cli treatments`** - List treatments (services) with price, duration, discipline, and whether they can be booked online.


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
janeapp-pp-cli appointments

# JSON for scripting and agents
janeapp-pp-cli appointments --json

# Filter to specific fields
janeapp-pp-cli appointments --json --select id,name,status

# Dry run — show the request without sending
janeapp-pp-cli appointments --dry-run

# Agent mode — JSON + compact + no prompts in one flag
janeapp-pp-cli appointments --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Freshness

This CLI owns bounded freshness for registered store-backed read command paths. In `--data-source auto` mode, covered commands check the local SQLite store before serving results; stale or missing resources trigger a bounded refresh, and refresh failures fall back to the existing local data with a warning. `--data-source local` never refreshes, and `--data-source live` reads the API without mutating the local store.

Set `JANEAPP_NO_AUTO_REFRESH=1` to disable the pre-read freshness hook while preserving the selected data source.

Covered command paths:
- `janeapp-pp-cli appointments`
- `janeapp-pp-cli appointments get`
- `janeapp-pp-cli appointments list`
- `janeapp-pp-cli appointments search`
- `janeapp-pp-cli disciplines`
- `janeapp-pp-cli disciplines get`
- `janeapp-pp-cli disciplines list`
- `janeapp-pp-cli disciplines search`
- `janeapp-pp-cli locations`
- `janeapp-pp-cli locations get`
- `janeapp-pp-cli locations list`
- `janeapp-pp-cli locations search`
- `janeapp-pp-cli openings`
- `janeapp-pp-cli openings get`
- `janeapp-pp-cli openings list`
- `janeapp-pp-cli openings search`
- `janeapp-pp-cli staff`
- `janeapp-pp-cli staff get`
- `janeapp-pp-cli staff list`
- `janeapp-pp-cli staff search`
- `janeapp-pp-cli treatments`
- `janeapp-pp-cli treatments get`
- `janeapp-pp-cli treatments list`
- `janeapp-pp-cli treatments search`

JSON outputs that use the generated provenance envelope include freshness metadata at `meta.freshness`. This metadata describes the freshness decision for the covered command path; it does not claim full historical backfill or API-specific enrichment.

## Health Check

```bash
janeapp-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Run `janeapp-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/janeapp-pp-cli/config.toml`; `--home`, `JANEAPP_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `janeapp-pp-cli doctor` to check credentials
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific
- **auth login says "reCAPTCHA required" / lands on /auth/failure** — Jane blocks command-line password login. Log in to the clinic in your browser, then import the session with `auth login --clinic <name> --chrome` (or `--cookies-file`).
- **appointments return 401** — Your imported session expired — log in again in the browser and re-run `auth login --clinic <name> --chrome`.
- **openings returns 'Number of days must be an integer between 1 and 7'** — Use num-days 1..7; for longer horizons use next-opening/watch which page automatically.
- **book says a treatment can't be booked online** — Check 'treatments' — only entries with book_online=true are online-bookable; others are call-to-book.

## Known Gaps

Jane exposes no public patient API, so this CLI is built against its live web
endpoints. Coverage is verified to different depths:

- **Verified live (public, unauthenticated):** `locations`, `disciplines`,
  `treatments`, `staff`, `openings`, and `next-opening` were exercised against
  real Jane clinics.
- **Auth is browser-session import, not password.** Jane gates username/password
  login behind reCAPTCHA, so `auth login` imports the `_front_desk_session` cookie
  from a browser where you're already logged in (`--chrome` / `--cookies-file`).
  A `--username`/`--password` path exists but Jane rejects it with a reCAPTCHA
  error; the CLI detects this and points you to the cookie path.
- **Verified live (authenticated):** `auth login --chrome`, `appointments`
  (upcoming/past), `agenda`, `openings`/`next-opening` (future dates), `book`, and
  `calendar` were all confirmed against a real logged-in account.
- **`book`, `cancel`, and `reschedule` are verified live** against a real account:
  - `book` runs Jane's reserve → confirm flow (`POST /api/v2/reservations` then
    `POST /api/v2/appointments/{id}/book`).
  - `cancel` is `DELETE /api/v2/appointments/{id}` (the patient endpoint; the
    `/cancel` suffix is the staff API).
  - `reschedule` books the new slot first, then cancels the old with a fresh CSRF
    token (Jane rotates it after each mutation) — so a failed new booking never
    loses your original appointment.
  All three are dry-run by default; add `--confirm` to submit.
- **Cancelled appointments are excluded from `upcoming`/`agenda`** but shown in
  `past` and the ICS feed (with `STATUS:CANCELLED`).
- **Availability uses `date`, not `start_date`.** Jane's `/api/v2/openings` keys the
  window on `date`; passing `start_date` is silently ignored (it returns only the
  current week). The CLI sends `date` — a fix over the initial spec guess.
- **`calendar`** generates a `.ics` from your appointments (`--out`) and prints
  Jane's native live subscribe URL (`--url`) for auto-syncing in a calendar app.
