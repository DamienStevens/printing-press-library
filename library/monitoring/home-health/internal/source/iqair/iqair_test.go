package iqair

import (
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source"
)

// connectedHomeJSON is the live-validated shape of a connected AirVisual Pro
// ("Home"): current carries ts plus several measurement objects, including the
// non-measurement `outdoor` key the normalizer must ignore.
const connectedHomeJSON = `[
  {
    "id": "avp_abc123",
    "serialNumber": "SN-1",
    "name": "Home",
    "model": "avp",
    "isConnected": true,
    "lastSeenAt": "2026-05-29T12:28:07.725Z",
    "current": {
      "ts": "2026-05-29T12:28:00.793Z",
      "aqi": {"value": 0},
      "pm25": {"value": 0},
      "co2": {"value": 587},
      "humidity": {"value": 51},
      "temperature": {"value": 21.9},
      "outdoor": {"ts": "2026-05-29T12:00:00.000Z"}
    },
    "sensorsDefinition": [
      {"pollutant": "pm25", "unit": "µg/m³", "name": "PM2.5", "hasSensor": true}
    ]
  }
]`

// offlineOutdoorJSON is the live-validated shape of an offline AirVisual Outdoor
// (1R5N): isConnected false and current carries only ts, so it yields no
// readings rather than an error.
const offlineOutdoorJSON = `[
  {
    "id": "avo_1R5N",
    "name": "AirVisual Outdoor - 1R5N",
    "model": "avo",
    "isConnected": false,
    "current": {"ts": "2026-05-18T19:52:09.000Z"}
  }
]`

func TestParseDevices(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCount int
		// want maps metric -> value for spot-checking the emitted readings.
		want map[string]float64
		// wantTS is the expected reading timestamp when wantCount > 0.
		wantTS string
		wantID string
	}{
		{
			name:      "connected Home avp",
			body:      connectedHomeJSON,
			wantCount: 5,
			want: map[string]float64{
				source.MetricCO2:      587,
				source.MetricHumidity: 51,
				source.MetricTemp:     21.9,
				source.MetricPM25:     0,
				source.MetricAQI:      0,
			},
			wantTS: "2026-05-29T12:28:00.793Z",
			wantID: "avp_abc123",
		},
		{
			name:      "offline Outdoor avo",
			body:      offlineOutdoorJSON,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readings, err := parseDevices([]byte(tt.body))
			if err != nil {
				t.Fatalf("parseDevices: %v", err)
			}
			if len(readings) != tt.wantCount {
				t.Fatalf("got %d readings, want %d: %+v", len(readings), tt.wantCount, readings)
			}
			if tt.wantCount == 0 {
				return
			}

			wantTS, err := time.Parse(time.RFC3339Nano, tt.wantTS)
			if err != nil {
				t.Fatalf("bad wantTS fixture: %v", err)
			}

			got := make(map[string]float64, len(readings))
			for _, r := range readings {
				got[r.Metric] = r.Value
				if r.Source != source.SourceIQAir {
					t.Errorf("metric %s: Source=%q, want %q", r.Metric, r.Source, source.SourceIQAir)
				}
				if r.DeviceID != tt.wantID {
					t.Errorf("metric %s: DeviceID=%q, want %q", r.Metric, r.DeviceID, tt.wantID)
				}
				if !r.TS.Equal(wantTS) {
					t.Errorf("metric %s: TS=%s, want %s", r.Metric, r.TS, wantTS)
				}
			}
			for metric, want := range tt.want {
				if got[metric] != want {
					t.Errorf("metric %s = %v, want %v", metric, got[metric], want)
				}
			}
		})
	}
}

// TestParseDevicesUnits pins the canonical unit per metric so a wiring mistake
// in metricMap surfaces as a test failure rather than a silent dashboard bug.
func TestParseDevicesUnits(t *testing.T) {
	readings, err := parseDevices([]byte(connectedHomeJSON))
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}
	wantUnit := map[string]string{
		source.MetricAQI:      "",
		source.MetricPM25:     "µg/m³",
		source.MetricCO2:      "ppm",
		source.MetricHumidity: "pct",
		source.MetricTemp:     "c",
	}
	for _, r := range readings {
		if want, ok := wantUnit[r.Metric]; ok && r.Unit != want {
			t.Errorf("metric %s: Unit=%q, want %q", r.Metric, r.Unit, want)
		}
	}
}

// TestParseDevicesIgnoresOutdoor guards the rule that the nested `outdoor` key
// (an object without a numeric `value`) never becomes a reading.
func TestParseDevicesIgnoresOutdoor(t *testing.T) {
	readings, err := parseDevices([]byte(connectedHomeJSON))
	if err != nil {
		t.Fatalf("parseDevices: %v", err)
	}
	for _, r := range readings {
		if r.Metric == "outdoor" {
			t.Errorf("outdoor key leaked into readings: %+v", r)
		}
	}
}
