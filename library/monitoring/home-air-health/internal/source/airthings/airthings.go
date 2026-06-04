// Package airthings implements the AirThings Consumer API cloud Source.
// AirThings devices report through two hosts: accounts-api issues a short-lived
// OAuth2 bearer token (client_credentials grant), and consumer-api exposes the
// account, its devices, and their latest sensor snapshot. This package fetches
// that snapshot and normalizes each sensor into the shared source.Reading shape.
package airthings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-air-health/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-air-health/internal/cred"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-air-health/internal/source"
)

const (
	tokenURL    = "https://accounts-api.airthings.com/v1/token"
	consumerAPI = "https://consumer-api.airthings.com/v1"

	// initialRate seeds the adaptive limiter. The Consumer API caps at 120
	// req/hr, but a sync makes only a handful of calls, so starting near 1
	// req/sec keeps a single run fast and lets OnRateLimit back off if the
	// hourly budget is already spent.
	initialRate = 1.0

	// maxRetries bounds 429 retries before surfacing a *cliutil.RateLimitError.
	maxRetries = 3

	// recordedLayout is the timestamp format /sensors emits — no zone suffix,
	// so it is parsed as UTC.
	recordedLayout = "2006-01-02T15:04:05"
)

// AirThings is a source.Source backed by the AirThings Consumer API.
type AirThings struct {
	client  *http.Client
	limiter *cliutil.AdaptiveLimiter

	// token caches the bearer token for the duration of a single Fetch so the
	// account/devices/sensors calls reuse one authentication.
	token string
}

// New returns an AirThings source using the default HTTP client.
func New() *AirThings {
	return &AirThings{
		client:  &http.Client{Timeout: 30 * time.Second},
		limiter: cliutil.NewAdaptiveLimiter(initialRate),
	}
}

func (a *AirThings) Name() string { return source.SourceAirThings }

// Available reports whether an AirThings credential exists; it does not dial.
func (a *AirThings) Available(ctx context.Context) (bool, string) {
	_, found, err := cred.Get(ctx, cred.ServiceAirThings)
	if err != nil {
		return false, fmt.Sprintf("AirThings credential lookup failed: %v", err)
	}
	if !found {
		return false, "no AirThings credential in Keychain (home-health-airthings)"
	}
	return true, ""
}

// Fetch authenticates, resolves the account, lists devices, and pulls the
// latest sensor snapshot. The Consumer API is latest-only — one snapshot per
// device per call — so `since` only filters out stale snapshots.
func (a *AirThings) Fetch(ctx context.Context, since time.Time) ([]source.Reading, error) {
	c, found, err := cred.Get(ctx, cred.ServiceAirThings)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("airthings: no credential configured (home-health-airthings)")
	}

	if err := a.authenticate(ctx, c.Account, c.Secret); err != nil {
		return nil, err
	}

	accountID, err := a.accountID(ctx)
	if err != nil {
		return nil, err
	}

	devicesBody, err := a.get(ctx, fmt.Sprintf("%s/accounts/%s/devices", consumerAPI, accountID))
	if err != nil {
		return nil, err
	}
	nameBySerial, err := parseDevices(devicesBody)
	if err != nil {
		return nil, err
	}

	var readings []source.Reading
	// AirThings pageNumber is 1-based; page 0 returns HTTP 400.
	for page := 1; ; page++ {
		url := fmt.Sprintf("%s/accounts/%s/sensors?pageNumber=%d", consumerAPI, accountID, page)
		body, err := a.get(ctx, url)
		if err != nil {
			return nil, err
		}
		parsed, more, err := parseSensors(body, nameBySerial)
		if err != nil {
			return nil, err
		}
		for _, r := range parsed {
			if since.IsZero() || !r.TS.Before(since) {
				readings = append(readings, r)
			}
		}
		if !more {
			break
		}
	}
	return readings, nil
}

// authenticate performs the client_credentials grant and caches the token.
// The documented `read:device` scope returns invalid_scope, so no scope field
// is sent — omitting it yields a token with the access the snapshot calls need.
func (a *AirThings) authenticate(ctx context.Context, clientID, clientSecret string) error {
	payload, err := json.Marshal(map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     clientID,
		"client_secret": clientSecret,
	})
	if err != nil {
		return err
	}
	body, err := a.do(ctx, http.MethodPost, tokenURL, payload, "")
	if err != nil {
		return err
	}
	var env struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("airthings: decode token response: %w", err)
	}
	if env.AccessToken == "" {
		return fmt.Errorf("airthings: auth returned no access_token")
	}
	a.token = env.AccessToken
	return nil
}

// accountID resolves the first account on the credential; the Consumer API
// keys every device and sensor call by account.
func (a *AirThings) accountID(ctx context.Context) (string, error) {
	body, err := a.get(ctx, consumerAPI+"/accounts")
	if err != nil {
		return "", err
	}
	var env struct {
		Accounts []struct {
			ID string `json:"id"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("airthings: decode accounts response: %w", err)
	}
	if len(env.Accounts) == 0 || env.Accounts[0].ID == "" {
		return "", fmt.Errorf("airthings: no accounts on credential")
	}
	return env.Accounts[0].ID, nil
}

// get is a token-authenticated GET returning the response body.
func (a *AirThings) get(ctx context.Context, url string) ([]byte, error) {
	return a.do(ctx, http.MethodGet, url, nil, a.token)
}

// do performs one rate-limited request with 429 retries, returning the body on
// 2xx and a *cliutil.RateLimitError when throttling persists. A non-empty token
// is sent as a Bearer credential.
func (a *AirThings) do(ctx context.Context, method, url string, body []byte, token string) ([]byte, error) {
	var lastResp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		a.limiter.Wait()

		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := a.client.Do(req)
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
			a.limiter.OnRateLimit()
			lastResp = resp
			if attempt < maxRetries {
				if err := sleepCtx(ctx, cliutil.RetryAfter(resp)); err != nil {
					return nil, err
				}
				continue
			}
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			a.limiter.OnSuccess()
			return respBody, nil
		default:
			return nil, fmt.Errorf("airthings: %s %s: HTTP %d: %s", method, url, resp.StatusCode, string(respBody))
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

// parseDevices builds a serialNumber→name map from the /devices response.
// Devices with an empty name fall back to their serial at read time, so they
// are stored as-is here.
func parseDevices(body []byte) (map[string]string, error) {
	var env struct {
		Devices []struct {
			SerialNumber string `json:"serialNumber"`
			Name         string `json:"name"`
		} `json:"devices"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("airthings: decode devices response: %w", err)
	}
	nameBySerial := make(map[string]string, len(env.Devices))
	for _, d := range env.Devices {
		nameBySerial[d.SerialNumber] = d.Name
	}
	return nameBySerial, nil
}

// sensorMetric maps an AirThings sensorType to a canonical source metric.
// The second return is false for sensor types this CLI does not track, so the
// caller can skip them without erroring.
func sensorMetric(sensorType string) (string, bool) {
	switch sensorType {
	case "radonShortTermAvg":
		return source.MetricRadon, true
	case "temp":
		return source.MetricTemp, true
	case "humidity":
		return source.MetricHumidity, true
	case "voc":
		return source.MetricVOC, true
	case "mold":
		return source.MetricMold, true
	case "co2":
		return source.MetricCO2, true
	case "pressure":
		return source.MetricPressure, true
	case "pm25":
		return source.MetricPM25, true
	default:
		return "", false
	}
}

// parseSensors normalizes the /sensors snapshot into readings, resolving each
// device's room from nameBySerial (falling back to the serial). Results whose
// `recorded` is empty (e.g. a hub with no measurements) are skipped. The second
// return reports whether another page exists.
func parseSensors(body []byte, nameBySerial map[string]string) ([]source.Reading, bool, error) {
	var env struct {
		Results []struct {
			SerialNumber string `json:"serialNumber"`
			Recorded     string `json:"recorded"`
			Sensors      []struct {
				SensorType string  `json:"sensorType"`
				Value      float64 `json:"value"`
				Unit       string  `json:"unit"`
			} `json:"sensors"`
		} `json:"results"`
		HasNext bool `json:"hasNext"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, false, fmt.Errorf("airthings: decode sensors response: %w", err)
	}

	var readings []source.Reading
	for _, res := range env.Results {
		if res.Recorded == "" {
			continue
		}
		ts, err := time.ParseInLocation(recordedLayout, res.Recorded, time.UTC)
		if err != nil {
			return nil, false, fmt.Errorf("airthings: parse recorded %q: %w", res.Recorded, err)
		}
		room := nameBySerial[res.SerialNumber]
		if room == "" {
			room = res.SerialNumber
		}
		for _, s := range res.Sensors {
			metric, ok := sensorMetric(s.SensorType)
			if !ok {
				continue
			}
			readings = append(readings, source.Reading{
				TS:       ts,
				Source:   source.SourceAirThings,
				DeviceID: res.SerialNumber,
				Room:     room,
				Metric:   metric,
				Value:    s.Value,
				Unit:     s.Unit,
			})
		}
	}
	return readings, env.HasNext, nil
}
