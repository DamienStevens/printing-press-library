// Package source defines the common contract every home-health data source
// implements. Each vendor (AirThings cloud, IQAir web API, MOCREO legacy
// cloud, AirVisual Pro local SMB) is a sibling Source that fetches readings
// and normalizes them into the unified Reading shape the readings store and
// dashboard consume. This is the seam that lets a synthetic combo CLI treat
// three incompatible auth schemes as one aggregated history.
package source

import (
	"context"
	"time"
)

// Canonical metric names. Every source maps its vendor-specific field names to
// one of these so the dashboard can aggregate across vendors. Keep this list as
// the single source of truth; do not invent per-source metric strings.
const (
	MetricRadon    = "radon"    // bq (Bq/m³)
	MetricVOC      = "voc"      // ppb
	MetricPM01     = "pm01"     // µg/m³
	MetricPM25     = "pm25"     // µg/m³
	MetricPM10     = "pm10"     // µg/m³
	MetricCO2      = "co2"      // ppm
	MetricAQI      = "aqi"      // index
	MetricTemp     = "temp"     // °C
	MetricHumidity = "humidity" // % RH
	MetricPressure = "pressure" // hPa
	MetricMold     = "mold"     // risk index (vendor-provided where available)
)

// Source name constants — used as the `source` column value in readings.
const (
	SourceAirThings    = "airthings"
	SourceIQAir        = "iqair"
	SourceMOCREO       = "mocreo"
	SourceAirVisualSMB = "airvisual-smb"
)

// Reading is one normalized sensor measurement.
type Reading struct {
	TS       time.Time `json:"ts"`
	Source   string    `json:"source"`    // one of the Source* constants
	DeviceID string    `json:"device_id"` // vendor device/serial/node id
	Room     string    `json:"room"`      // human label (device/room name)
	Metric   string    `json:"metric"`    // one of the Metric* constants
	Value    float64   `json:"value"`
	Unit     string    `json:"unit"`
}

// Source fetches readings from one vendor/transport and normalizes them.
type Source interface {
	// Name returns the Source* constant identifying this source.
	Name() string

	// Available reports whether this source has the credentials/reachability
	// it needs to run. Sources missing config return false with a human reason
	// so `doctor` and `sync` can report per-source status instead of failing
	// the whole run.
	Available(ctx context.Context) (bool, string)

	// Fetch returns readings recorded at or after `since`. Sources that only
	// expose "latest" values (e.g. AirThings, IQAir) return a single snapshot
	// per device; sources with history endpoints (MOCREO /samples) return the
	// window. Implementations MUST use cliutil.AdaptiveLimiter for outbound
	// HTTP and surface *cliutil.RateLimitError when throttled, so an empty
	// result is never confused with "rate-limited".
	Fetch(ctx context.Context, since time.Time) ([]Reading, error)
}
