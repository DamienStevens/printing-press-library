// Package iqair implements the IQAir / AirVisual reverse-engineered web API
// Source, covering both the AirVisual Pro and the AirVisual Outdoor (1R5N) via
// the cloud. The website API takes an email/password sign-in that returns a
// loginToken; every subsequent call carries that token in an x-login-token
// header. The /users/{id}/devices listing embeds each device's latest snapshot
// under `current`, which this package normalizes into shared source.Reading
// values. An offline device's `current` carries only a timestamp and no
// measurement keys, so it contributes no readings rather than an error.
package iqair

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/cred"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source"
)

const (
	baseURL = "https://website-api.airvisual.com/v1"

	// initialRate seeds the adaptive limiter. The web API tolerates a couple of
	// requests per second; the limiter ramps up from here on sustained success.
	initialRate = 2.0

	// maxRetries bounds 429 retries before surfacing a *cliutil.RateLimitError.
	maxRetries = 3
)

// IQAir is a source.Source backed by the AirVisual website API.
type IQAir struct {
	client  *http.Client
	limiter *cliutil.AdaptiveLimiter

	// token caches the loginToken for the duration of a single Fetch so the
	// devices request reuses the one authentication.
	token string
}

// New returns an IQAir source using the default HTTP client.
func New() *IQAir {
	return &IQAir{
		client:  &http.Client{Timeout: 30 * time.Second},
		limiter: cliutil.NewAdaptiveLimiter(initialRate),
	}
}

func (q *IQAir) Name() string { return source.SourceIQAir }

// Available reports whether an IQAir credential exists; it does not dial the API.
func (q *IQAir) Available(ctx context.Context) (bool, string) {
	_, found, err := cred.Get(ctx, cred.ServiceIQAir)
	if err != nil {
		return false, fmt.Sprintf("IQAir credential lookup failed: %v", err)
	}
	if !found {
		return false, "no IQAir credential in Keychain (home-health-iqair)"
	}
	return true, ""
}

// Fetch signs in, lists the account's devices, and normalizes each device's
// latest snapshot. The website API exposes only the latest `current` value per
// device, so since acts purely as a lower-bound filter on those snapshots.
func (q *IQAir) Fetch(ctx context.Context, since time.Time) ([]source.Reading, error) {
	c, found, err := cred.Get(ctx, cred.ServiceIQAir)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("iqair: no credential configured (home-health-iqair)")
	}

	userID, err := q.signin(ctx, c.Account, c.Secret)
	if err != nil {
		return nil, err
	}

	body, err := q.get(ctx, fmt.Sprintf("%s/users/%s/devices", baseURL, userID))
	if err != nil {
		return nil, err
	}
	readings, err := parseDevices(body)
	if err != nil {
		return nil, err
	}

	if since.IsZero() {
		return readings, nil
	}
	filtered := readings[:0]
	for _, r := range readings {
		if !r.TS.Before(since) {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

// signin performs the email/password sign-in and caches the loginToken,
// returning the account's user id for the subsequent devices call.
func (q *IQAir) signin(ctx context.Context, email, password string) (string, error) {
	payload, err := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	if err != nil {
		return "", err
	}
	body, err := q.do(ctx, http.MethodPost, baseURL+"/auth/signin/by/email", payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		ID         string `json:"id"`
		LoginToken string `json:"loginToken"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("iqair: decode signin response: %w", err)
	}
	if resp.LoginToken == "" || resp.ID == "" {
		return "", fmt.Errorf("iqair: signin returned no loginToken/id")
	}
	q.token = resp.LoginToken
	return resp.ID, nil
}

// get is a token-authenticated GET returning the response body.
func (q *IQAir) get(ctx context.Context, url string) ([]byte, error) {
	return q.do(ctx, http.MethodGet, url, nil)
}

// do performs one rate-limited request with 429 retries, returning the body on
// 2xx and a *cliutil.RateLimitError when throttling persists. The cached
// loginToken, when present, rides on x-login-token rather than Authorization —
// the API authenticates only that header.
func (q *IQAir) do(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	var lastResp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		q.limiter.Wait()

		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if q.token != "" {
			req.Header.Set("x-login-token", q.token)
		}

		resp, err := q.client.Do(req)
		if err != nil {
			return nil, err
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			q.limiter.OnRateLimit()
			lastResp = resp
			if attempt < maxRetries {
				if err := sleepCtx(ctx, cliutil.RetryAfter(resp)); err != nil {
					return nil, err
				}
				continue
			}
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			q.limiter.OnSuccess()
			return respBody, nil
		default:
			return nil, fmt.Errorf("iqair: %s %s: HTTP %d: %s", method, url, resp.StatusCode, string(respBody))
		}
	}
	return nil, &cliutil.RateLimitError{
		URL:        url,
		RetryAfter: cliutil.RetryAfter(lastResp),
	}
}

// sleepCtx waits for d while honoring context cancellation.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// measurement is one `{ "value": <number> }` object under a device's `current`.
// A pointer Value distinguishes "key absent" from a real zero so offline
// devices (whose current carries only ts) produce no readings.
type measurement struct {
	Value *float64 `json:"value"`
}

// device is one entry from the /devices array. Only the fields the normalizer
// needs are decoded; current is kept raw so unknown/nested keys (e.g. outdoor)
// are ignored rather than mapped.
type device struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Current json.RawMessage `json:"current"`
}

// metricMap pairs each known `current` measurement key with its canonical
// metric name and unit. Keys absent from this map (outdoor, nested snapshots,
// future fields) are ignored by the normalizer.
var metricMap = []struct {
	key, metric, unit string
}{
	{"aqi", source.MetricAQI, ""},
	{"pm25", source.MetricPM25, "µg/m³"},
	{"pm10", source.MetricPM10, "µg/m³"},
	{"pm01", source.MetricPM01, "µg/m³"},
	{"co2", source.MetricCO2, "ppm"},
	{"humidity", source.MetricHumidity, "pct"},
	{"temperature", source.MetricTemp, "c"},
}

// parseDevices normalizes the /devices array into readings. Each device emits
// one reading per known measurement key present under `current` with a numeric
// value; a device whose current has no measurement keys (offline) yields none.
// Factored out of Fetch so it is table-testable without network access.
func parseDevices(body []byte) ([]source.Reading, error) {
	var devices []device
	if err := json.Unmarshal(body, &devices); err != nil {
		return nil, fmt.Errorf("iqair: decode devices: %w", err)
	}

	var readings []source.Reading
	for _, d := range devices {
		if len(d.Current) == 0 {
			continue
		}
		// Decode current as a key->raw map: ts gives the timestamp and every other
		// entry is a candidate measurement. Unknown/nested keys (e.g. outdoor) drop
		// out because only keys in metricMap are inspected.
		var cur map[string]json.RawMessage
		if err := json.Unmarshal(d.Current, &cur); err != nil {
			return nil, fmt.Errorf("iqair: decode current for device %q: %w", d.ID, err)
		}

		var ts time.Time
		if raw, ok := cur["ts"]; ok {
			if err := json.Unmarshal(raw, &ts); err != nil {
				return nil, fmt.Errorf("iqair: decode current.ts for device %q: %w", d.ID, err)
			}
		}

		for _, m := range metricMap {
			value, ok := measurementValue(cur[m.key])
			if !ok {
				continue
			}
			readings = append(readings, source.Reading{
				TS:       ts,
				Source:   source.SourceIQAir,
				DeviceID: d.ID,
				Room:     d.Name,
				Metric:   m.metric,
				Value:    value,
				Unit:     m.unit,
			})
		}
	}
	return readings, nil
}

// measurementValue pulls a numeric `value` out of a `{ "value": <number> }`
// object. It reports false for an absent key, a non-object, or a value that
// isn't a number, so non-measurement keys never produce a reading.
func measurementValue(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var m measurement
	if err := json.Unmarshal(raw, &m); err != nil || m.Value == nil {
		return 0, false
	}
	return *m.Value, true
}
