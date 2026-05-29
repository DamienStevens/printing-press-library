# Live Validation Results (2026-05-29) — REDACTED

Creds from macOS Keychain (services `home-health-*`). Secrets never printed.

## AirThings — ✅ FULLY WORKING (cloud Consumer API)
- Token: `POST accounts-api.airthings.com/v1/token`, JSON body, **OMIT scope** (the documented `read:device` scope → `invalid_scope`; Business-only). `{grant_type:client_credentials, client_id, client_secret}` → `access_token` (len ~1215, ~expires).
- `GET consumer-api.airthings.com/v1/accounts` → `{accounts:[{id}]}` (1 account).
- `GET /v1/accounts/{id}/devices` → `{devices:[{serialNumber, name, sensors:[...]}]}`.
- `GET /v1/accounts/{id}/sensors` → `{results:[{serialNumber, recorded, sensors:[{sensorType,value,unit}]}]}`.
- **Live data (3 devices):**
  - "school room" (Wave Radon): radonShortTermAvg=40 bq, humidity=54 pct, temp=21.3 c
  - "Bedroom" (Wave Mini): humidity=65 pct, temp=16.8 c, voc=123 ppb, **mold=1.0 riskIndex**
  - "Hub": no sensors
- sensorType set: radonShortTermAvg, temp, humidity, voc, mold. units: bq, c, pct, ppb, riskIndex.
- Device name == room name. Native `mold` riskIndex is a gift for the dashboard.

## IQAir / AirVisual — ✅ WORKING (reverse-engineered web API)
- `POST website-api.airvisual.com/v1/auth/signin/by/email` {email,password} → body `{id, email, name, loginToken, ...}`. Token = **`loginToken`** (body, len ~44).
- Auth header on subsequent calls: **`x-login-token: <loginToken>`**.
- `GET website-api.airvisual.com/v1/users/{id}/devices` → array of devices. (`/v1/users/{id}` and `/v1/devices` → 404; `/stations` → []).
- Device fields: `{id, serialNumber, name, model, isConnected, lastSeenAt, current:{...}, sensorsDefinition:[{pollutant,unit,name,hasSensor}]}`.
- `current` block (model avp): `{ts, aqi:{value}, pm25:{value,aqi}, co2:{value}, humidity:{value}, temperature:{value}, outdoor:{ts}}`.
- **Live data:**
  - "Home" (model **avp** = AirVisual Pro): aqi=0, pm25=0 µg/m³, **co2=587 ppm**, humidity=51, temp=21.9 c. FRESH (today). Covers the Pro via cloud → SMB optional.
  - "AirVisual Outdoor - 1R5N" (model avo): **isConnected=false, lastSeen 2026-05-18** → DEVICE OFFLINE. API path is correct; needs reconnect to report.

## MOCREO — ✅ WORKING via LEGACY "Sensor System" API (api.sync-sign.com/v2)
- His hub is LEGACY (Sync-Sign backend), NOT the v1 Smart System (v1 /assets = []; platforms have separate data stores, no migration). Use the legacy API. Swagger saved: specs/mocreo-legacy-swagger.json (Swagger 2.0, "Mocreo Sensor API", 12 endpoints).
- Auth: `POST https://api.sync-sign.com/v2/oauth/token` {username, password, provider:"mocreo"} → envelope `{code,data:{accessToken,refreshToken,accessTokenExpiresAt}}`. accessToken valid 24h. Header on calls: `Authorization: Bearer <accessToken>`.
- `GET /v2/devices` → hubs (5): `{sn,status,region}`. `GET /v2/nodes` → sensors (7): `{nodeId,type,model,name,onlined,batteryLevel,signalLevel}`.
- DATA: `GET /v2/nodes/{node_id}/samples?limit=N[&offset&beginTime&endTime]` (limit REQUIRED) → envelope `{data:{records:[{time(epoch s), data:{tm,hu}}]}}`. **tm/100=°C, hu/100=%RH.** (CRITICAL: use curl/Go with `Authorization: Bearer`; Python urllib gave spurious 403 in testing.)
- **Live nodes (room names):** Kitchen (ST6, 20.7°C/57%), Crawlspace (ST6 — mold-critical), Indoor freezer, Outdoor freezer (ST5), Networking Gear, Sony Camera, Safe (ST1). All onlined.
- Maps to readings: metric=temp|humidity per node. Crawlspace + Kitchen humidity = mold signal.

## VERDICT: 3/3 VENDORS LIVE. AirThings ✅ + IQAir(Pro) ✅ + MOCREO(legacy) ✅.
## Non-blocking gaps: IQAir Outdoor 1R5N device OFFLINE (reconnect); AirVisual Pro SMB optional (wrong pw; cloud covers Pro).

## AirVisual Pro SMB (local) — ✗ guessed password wrong
- `airvisual.lan` .39, share `airvisual` :445. Guessed 4-char pw rejected (users tried: airvisual/admin/AirVisual). Optional — cloud covers the Pro. Need real password from the unit for pure-LAN read.

## Scorecard: 2 of 3 vendors live (AirThings + IQAir-Pro). Open: MOCREO account/API, Outdoor offline, real SMB pw.
