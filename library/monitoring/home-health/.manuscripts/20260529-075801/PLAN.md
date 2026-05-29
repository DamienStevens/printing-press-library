# Home Health CLI — Build Plan (resumable)

**Goal:** One CLI (`home-health-pp-cli`) aggregating ALL home air-quality sensors into a unified local SQLite store with a `dashboard` supporting day/week/month/quarter/year/all-time + mold/allergy focus. User mandate: **3/3 — make every source work**, any method (cloud creds, app reverse-engineering, port scans).

Run dir: `~/printing-press/.runstate/cli-printing-press-70e28f12/runs/20260529-075801`

## Verified reachability (probed 2026-05-29)

LAN: `192.168.86.0/24`, this Mac = `.83`.

| Device | Address | Path (VERIFIED) | Credential needed |
|---|---|---|---|
| MOCREO sensors (via Hub) | Hub=`mocreo-90bc`/`espressif.lan` .53 (portal :80 login-only) | **Cloud** `api.mocreo.com/v1`. Spec live (HTTP 200, OpenAPI 3.0, 1451 lines). `X-API-Key` (`mok_pfx_secret`). Mint via `POST /users/login` -> bearer -> `POST /assets/{id}/apikeys`. Data: `/assets/{id}/devices/{id}/history`. | MOCREO login OR existing key |
| AirThings Wave Radon + Mini (via Hub) | Hub=`airthings-hub.lan` .70 (ALL local ports closed) | **Cloud** Consumer API. `accounts-api.airthings.com/v1/token` (OAuth2 client-creds, scope `read:device`) -> `consumer-api.airthings.com/v1/accounts`,`/devices`,`/sensors`. Latest-only; build history by polling. 120 req/hr. Hub keeps cloud fresh. | client_id + client_secret (create app) |
| AirVisual Pro | `airvisual.lan` .39 | **Local SMB** :445, share `airvisual` (guest denied). JSON: latest_measurements + history. Fallback: IQAir cloud login (below). | SMB password (on unit) and/or IQAir login |
| AirVisual Outdoor 1R5N | WiFi (IP TBD) | **Cloud** reverse-engineered `website-api.airvisual.com/v1/auth/signin/by/email` -> token -> devices -> measurements. Endpoint LIVE (POST empty body = 422). Free (no Enterprise). | IQAir login (email/pass) |

Sensors available: radon (AirThings Radon), VOC (AirThings Mini + AirVisual Pro), PM0.1/PM2.5/PM10 + CO2 + AQI (AirVisual Pro), temp + humidity (all).

## Architecture: synthetic combo (REVISED — see SOURCE-CONTRACTS.md)

3 sources = 3 incompatible auth schemes → NOT a 3-spec merge. Build as `kind: synthetic`:
- **Generate** foundation from MOCREO live spec (primary): cobra tree, config, MCP, cliutil, SQLite framework, search/sql.
- **Hand-build** `internal/source/{mocreo,airthings,iqair,airvisual_smb}/` siblings, each own env-creds + `cliutil.AdaptiveLimiter` + typed `*cliutil.RateLimitError`, normalizing into unified `readings(ts,source,device_id,room,metric,value,unit)`.
- **Transcendence** = `dashboard --period <day|week|month|quarter|year|all> --focus <mold|allergy|all>`. Mold = sustained humidity >60% RH; allergy = PM2.5/VOC/CO2; radon standalone hazard. Per-room/per-sensor filters, trends, alerts.

## Credential plan (env-var, never in code/artifacts)

Drop file (gitignored): `~/.config/home-health/.env`
```
MOCREO_EMAIL= / MOCREO_PASSWORD=   (or MOCREO_API_KEY=mok_..._...)
AIRTHINGS_CLIENT_ID= / AIRTHINGS_CLIENT_SECRET=   (dashboard.airthings.com/integrations/api-integration -> Create Application)
IQAIR_EMAIL= / IQAIR_PASSWORD=
AIRVISUAL_PRO_SMB_PASSWORD=   (optional; on the unit)
AIRVISUAL_OUTDOOR_IP=   (optional)
```

## Status
- [x] Reachability probed + verified, run state initialized
- [ ] T1 author specs / T2 creds / T3 manifest / T4 generate / T5 SMB adapter / T6 dashboard / T7 shipcheck+dogfood / T8 polish
