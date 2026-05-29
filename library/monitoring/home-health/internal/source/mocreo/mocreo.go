// Package mocreo implements the MOCREO legacy "Sync-Sign" cloud Source. MOCREO
// sensors report through the Sync-Sign v2 API: an OAuth password grant yields a
// short-lived bearer token, /nodes lists the sensors, and /nodes/{id}/samples
// returns the per-sensor history. Temperature and humidity arrive as integer
// hundredths (tm=2069 means 20.69 °C), which this package normalizes into the
// shared source.Reading shape.
package mocreo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/cliutil"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/cred"
	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source"
)

const (
	baseURL = "https://api.sync-sign.com/v2"

	// initialRate seeds the adaptive limiter; the Sync-Sign API tolerates a few
	// requests per second and the limiter ramps from here.
	initialRate = 3.0

	// maxRetries bounds 429 retries before surfacing a *cliutil.RateLimitError.
	maxRetries = 3

	// historyLimit is the per-node sample cap when fetching a `since` window;
	// latestLimit covers the "latest snapshot" path when since is zero.
	historyLimit = 2000
	latestLimit  = 200
)

// MOCREO is a source.Source backed by the Sync-Sign v2 cloud API.
type MOCREO struct {
	client  *http.Client
	limiter *cliutil.AdaptiveLimiter

	// token caches the bearer token for the duration of a single Fetch call so
	// the N per-node sample requests reuse one authentication.
	token string
}

// New returns a MOCREO source using the default HTTP client.
func New() *MOCREO {
	return &MOCREO{
		client:  &http.Client{Timeout: 30 * time.Second},
		limiter: cliutil.NewAdaptiveLimiter(initialRate),
	}
}

func (m *MOCREO) Name() string { return source.SourceMOCREO }

// Available reports whether a MOCREO credential exists; it does not dial the API.
func (m *MOCREO) Available(ctx context.Context) (bool, string) {
	_, found, err := cred.Get(ctx, cred.ServiceMOCREO)
	if err != nil {
		return false, fmt.Sprintf("MOCREO credential lookup failed: %v", err)
	}
	if !found {
		return false, "no MOCREO credential in Keychain (home-health-mocreo)"
	}
	return true, ""
}

// Fetch authenticates, lists nodes, and pulls each node's samples. When since is
// non-zero only readings at or after it are returned; otherwise the latest
// snapshot per node is returned.
func (m *MOCREO) Fetch(ctx context.Context, since time.Time) ([]source.Reading, error) {
	c, found, err := cred.Get(ctx, cred.ServiceMOCREO)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("mocreo: no credential configured (home-health-mocreo)")
	}

	if err := m.authenticate(ctx, c.Account, c.Secret); err != nil {
		return nil, err
	}

	nodesBody, err := m.get(ctx, baseURL+"/nodes")
	if err != nil {
		return nil, err
	}
	nodes, err := parseNodes(nodesBody)
	if err != nil {
		return nil, err
	}

	limit := latestLimit
	if !since.IsZero() {
		limit = historyLimit
	}

	var readings []source.Reading
	for _, n := range nodes {
		url := fmt.Sprintf("%s/nodes/%s/samples?limit=%d", baseURL, n.NodeID, limit)
		if !since.IsZero() {
			url += "&beginTime=" + strconv.FormatInt(since.Unix(), 10)
		}
		body, err := m.get(ctx, url)
		if err != nil {
			return nil, err
		}
		parsed, err := parseSamples(n.NodeID, n.Name, body)
		if err != nil {
			return nil, err
		}
		for _, r := range parsed {
			if since.IsZero() || !r.TS.Before(since) {
				readings = append(readings, r)
			}
		}
	}
	return readings, nil
}

// authenticate performs the OAuth password grant and caches the bearer token.
func (m *MOCREO) authenticate(ctx context.Context, email, password string) error {
	payload, err := json.Marshal(map[string]string{
		"username": email,
		"password": password,
		"provider": "mocreo",
	})
	if err != nil {
		return err
	}
	body, err := m.do(ctx, http.MethodPost, baseURL+"/oauth/token", payload, "")
	if err != nil {
		return err
	}
	var env struct {
		Code int `json:"code"`
		Data struct {
			AccessToken string `json:"accessToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("mocreo: decode token response: %w", err)
	}
	if env.Data.AccessToken == "" {
		return fmt.Errorf("mocreo: auth returned no accessToken (code %d)", env.Code)
	}
	m.token = env.Data.AccessToken
	return nil
}

// get is a token-authenticated GET returning the response body.
func (m *MOCREO) get(ctx context.Context, url string) ([]byte, error) {
	return m.do(ctx, http.MethodGet, url, nil, m.token)
}

// do performs one rate-limited request with 429 retries, returning the body on
// 2xx and a *cliutil.RateLimitError when throttling persists. The token, when
// non-empty, is sent as a Bearer credential — the raw token is rejected by the
// API, so the "Bearer " prefix is mandatory.
func (m *MOCREO) do(ctx context.Context, method, url string, body []byte, token string) ([]byte, error) {
	var lastResp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		m.limiter.Wait()

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

		resp, err := m.client.Do(req)
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
			m.limiter.OnRateLimit()
			lastResp = resp
			if attempt < maxRetries {
				if err := sleepCtx(ctx, cliutil.RetryAfter(resp)); err != nil {
					return nil, err
				}
				continue
			}
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			m.limiter.OnSuccess()
			return respBody, nil
		default:
			return nil, fmt.Errorf("mocreo: %s %s: HTTP %d: %s", method, url, resp.StatusCode, string(respBody))
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

// node is one sensor from the /nodes listing.
type node struct {
	NodeID string `json:"nodeId"`
	Name   string `json:"name"`
}

// parseNodes extracts the sensor list from the /nodes envelope. The `data`
// field is normally an array, but some responses wrap it as an object with a
// "nodes" or "list" array — both shapes are accepted.
func parseNodes(body []byte) ([]node, error) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("mocreo: decode nodes envelope: %w", err)
	}

	var nodes []node
	if err := json.Unmarshal(env.Data, &nodes); err == nil {
		return nodes, nil
	}

	var wrapped struct {
		Nodes []node `json:"nodes"`
		List  []node `json:"list"`
	}
	if err := json.Unmarshal(env.Data, &wrapped); err != nil {
		return nil, fmt.Errorf("mocreo: decode nodes data: %w", err)
	}
	if wrapped.Nodes != nil {
		return wrapped.Nodes, nil
	}
	return wrapped.List, nil
}

// parseSamples normalizes one node's /samples envelope into readings. Each
// record yields up to two readings (temp + humidity); occupancy/leak booleans
// and any other keys are ignored. Temperature and humidity are integer
// hundredths in the wire format.
// sampleRecord is one /samples row: a timestamp plus a sparse measurement map.
type sampleRecord struct {
	Time int64 `json:"time"`
	Data struct {
		TM *int `json:"tm"`
		HU *int `json:"hu"`
		HM *int `json:"hm"` // humidity alias on some firmware
	} `json:"data"`
}

func parseSamples(nodeID, room string, body []byte) ([]source.Reading, error) {
	// The envelope's `data` is `{"records":[...]}` when samples exist, but a
	// node with no samples in the requested window returns a bare `[]` at
	// `data`. Decode `data` raw, then try the object shape and fall back to the
	// array shape so a quiet node never errors the whole sync.
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("mocreo: decode samples envelope: %w", err)
	}
	var records []sampleRecord
	if len(env.Data) > 0 {
		var wrapped struct {
			Records []sampleRecord `json:"records"`
		}
		if err := json.Unmarshal(env.Data, &wrapped); err == nil {
			records = wrapped.Records
		} else if err := json.Unmarshal(env.Data, &records); err != nil {
			return nil, fmt.Errorf("mocreo: decode samples data: %w", err)
		}
	}

	var readings []source.Reading
	for _, rec := range records {
		ts := time.Unix(rec.Time, 0).UTC()
		if rec.Data.TM != nil {
			readings = append(readings, source.Reading{
				TS:       ts,
				Source:   source.SourceMOCREO,
				DeviceID: nodeID,
				Room:     room,
				Metric:   source.MetricTemp,
				Value:    float64(*rec.Data.TM) / 100.0,
				Unit:     "c",
			})
		}
		if hu := rec.Data.HU; hu != nil {
			readings = append(readings, humidityReading(ts, nodeID, room, *hu))
		} else if hm := rec.Data.HM; hm != nil {
			readings = append(readings, humidityReading(ts, nodeID, room, *hm))
		}
	}
	return readings, nil
}

func humidityReading(ts time.Time, nodeID, room string, raw int) source.Reading {
	return source.Reading{
		TS:       ts,
		Source:   source.SourceMOCREO,
		DeviceID: nodeID,
		Room:     room,
		Metric:   source.MetricHumidity,
		Value:    float64(raw) / 100.0,
		Unit:     "pct",
	}
}
