# Source Contracts + Architecture Decision

## Architecture decision (2026-05-29): synthetic combo, NOT 3-spec merge

The 3 sources have 3 incompatible auth schemes → can't merge into one top-level
`AuthConfig`. Build as a **synthetic combo CLI** (`kind: synthetic`):

- **Generate** foundation from MOCREO's live OpenAPI (primary): gives cobra tree,
  config, MCP server, cliutil helpers, SQLite framework, search/sql.
- **Hand-build** sibling sources under `internal/source/`, each with own env-creds +
  `cliutil.AdaptiveLimiter` + typed `*cliutil.RateLimitError`, normalizing into a
  **unified `readings` table**: `(ts, source, device_id, room, metric, value, unit)`.
  - `internal/source/mocreo/`     (X-API-Key; or reuse generated client)
  - `internal/source/airthings/`  (OAuth2 client-creds)
  - `internal/source/iqair/`      (login-token; Pro + Outdoor)
  - `internal/source/airvisual_smb/` (local SMB read of the Pro)
- **Dashboard** transcendence command aggregates the unified table.

Metrics normalized set: radon, voc, pm01, pm25, pm10, co2, aqi, temp, humidity, pressure.

## MOCREO — cloud (spec: specs/mocreo-openapi.yaml, VERIFIED live)
- Base: `https://api.mocreo.com/v1`  Auth: header `X-API-Key: mok_pfx_secret`
- Mint key from login: `POST /users/login {email,password}` -> bearer ->
  `POST /assets/{assetId}/apikeys` (perms device.read, asset.read).
- `GET /assets` -> assetId(s).  `GET /assets/{assetId}/devices` -> deviceId(s).
- DATA: `GET /assets/{assetId}/devices/{deviceId}/history?from=<ms>&to=<ms>&tz=<IANA>&field=<temperature|humidity|water_leak|water_level|frozen>[&windowDuration=1h&aggregationsType=mean&limit=]`
  - field is single-valued per call (temp OR humidity → 2 calls/device).
  - resp: `{success, result:{data:[{time(ms), value, deviceId, field, unit}]}}`
  - rate: 1000 req/hr/key, 3 concurrent.
- Maps to readings: metric=temp|humidity. (MOCREO = per-room temp+humidity backbone.)

## AirThings — cloud Consumer API (VERIFIED reachable: token 500-on-empty, base 401)
- Auth base: `https://accounts-api.airthings.com`  Data base: `https://consumer-api.airthings.com`
- Token: `POST https://accounts-api.airthings.com/v1/token`
  `{grant_type:client_credentials, client_id, client_secret, scope:"read:device"}` -> JWT bearer.
- `GET /v1/accounts` -> accounts[].id (accountId).
- `GET /v1/accounts/{accountId}/devices` -> devices[] + sensor capabilities.
- DATA (latest only): `GET /v1/accounts/{accountId}/sensors` (paginated max 50/page).
  Build history by polling into the readings table on a timer.
- rate: 120 req/hr.  Hub present → cloud is fresh/real-time.
- Maps to readings: Radon device → radon, temp, humidity. Mini → voc, temp, humidity, pressure.
- **SHAPE TO CONFIRM AT LIVE VALIDATION:** exact /sensors JSON field names + units.

## IQAir / AirVisual — reverse-engineered web API (VERIFIED: signin endpoint = 422 live)
- Covers BOTH the Pro and the Outdoor 1R5N via cloud (no Enterprise plan).
- `POST https://website-api.airvisual.com/v1/auth/signin/by/email {email,password}`
  -> token (header `x-login-token` on subsequent calls). [422 on empty body = live]
- Then (per den.dev reverse-eng): list user devices -> per-device measurements.
  Alt: `https://app-api.airvisual.com/api/v5/devices/{device_id}/measurements`.
- **SHAPES TO CONFIRM AT LIVE VALIDATION:** exact signin response token field, the
  devices-list path, the measurements path + JSON (PM0.1/PM2.5/PM10/CO2/AQI/temp/humidity/VOC).
- Maps to readings: pm01, pm25, pm10, co2, aqi, temp, humidity, voc.

## AirVisual Pro — local SMB (VERIFIED: airvisual.lan 192.168.86.39, share `airvisual`, :445 open; guest denied)
- Auth: SMB user (likely blank/`airvisual`) + password printed on unit (env AIRVISUAL_PRO_SMB_PASSWORD).
- Read JSON files from the share: `latest_measurements.json` + history files (pyairvisual format).
- Go SMB client (e.g. hirochachacha/go-smb2) or shell `mount_smbfs`.
- Pure-LAN read of the Pro; cloud (IQAir login) is the fallback for the same device.

## Live-validation checklist (run when ~/.config/home-health/.env is filled)
1. MOCREO: login → mint key → list assets/devices → 1 history call. Confirm field/value/unit.
2. AirThings: client-creds token → /accounts → /devices → /sensors. Confirm sensor JSON + units.
3. IQAir: signin → token → devices → measurements. Confirm token field + measurement JSON.
4. AirVisual Pro SMB: mount with password → read latest_measurements.json. Confirm fields.
Capture one real response per source into proofs/ (REDACT any account/email/token values).
