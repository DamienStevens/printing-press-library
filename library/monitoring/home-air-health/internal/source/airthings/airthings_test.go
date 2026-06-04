package airthings

import (
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-air-health/internal/source"
)

func TestParseDevices(t *testing.T) {
	body := `{"devices":[
		{"serialNumber":"1110000001","name":"Bedroom","sensors":["temp","humidity","voc"]},
		{"serialNumber":"1110000002","name":"school room","sensors":["radonShortTermAvg","temp","humidity"]},
		{"serialNumber":"1110000003","name":"Hub","sensors":[]}
	]}`

	got, err := parseDevices([]byte(body))
	if err != nil {
		t.Fatalf("parseDevices returned error: %v", err)
	}

	want := map[string]string{
		"1110000001": "Bedroom",
		"1110000002": "school room",
		"1110000003": "Hub",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d devices, want %d: %+v", len(got), len(want), got)
	}
	for serial, name := range want {
		if got[serial] != name {
			t.Errorf("device[%s]: got %q, want %q", serial, got[serial], name)
		}
	}
}

func TestParseSensors(t *testing.T) {
	nameBySerial := map[string]string{
		"1110000001": "Bedroom",
		"1110000002": "school room",
	}
	body := `{"results":[
		{"serialNumber":"1110000001","recorded":"2026-05-29T12:27:00","sensors":[
			{"sensorType":"humidity","value":65.0,"unit":"pct"},
			{"sensorType":"temp","value":16.8,"unit":"c"},
			{"sensorType":"voc","value":123.0,"unit":"ppb"},
			{"sensorType":"mold","value":1.0,"unit":"riskIndex"}
		]},
		{"serialNumber":"1110000002","recorded":"2026-05-29T12:28:42","sensors":[
			{"sensorType":"radonShortTermAvg","value":40.0,"unit":"bq"},
			{"sensorType":"humidity","value":54.0,"unit":"pct"},
			{"sensorType":"temp","value":21.3,"unit":"c"}
		]}
	]}`

	got, more, err := parseSensors([]byte(body), nameBySerial)
	if err != nil {
		t.Fatalf("parseSensors returned error: %v", err)
	}
	if more {
		t.Errorf("expected no further pages, got hasNext=true")
	}

	bedroomTS := time.Date(2026, 5, 29, 12, 27, 0, 0, time.UTC)
	schoolTS := time.Date(2026, 5, 29, 12, 28, 42, 0, time.UTC)
	want := []source.Reading{
		{TS: bedroomTS, Source: source.SourceAirThings, DeviceID: "1110000001", Room: "Bedroom", Metric: source.MetricHumidity, Value: 65.0, Unit: "pct"},
		{TS: bedroomTS, Source: source.SourceAirThings, DeviceID: "1110000001", Room: "Bedroom", Metric: source.MetricTemp, Value: 16.8, Unit: "c"},
		{TS: bedroomTS, Source: source.SourceAirThings, DeviceID: "1110000001", Room: "Bedroom", Metric: source.MetricVOC, Value: 123.0, Unit: "ppb"},
		{TS: bedroomTS, Source: source.SourceAirThings, DeviceID: "1110000001", Room: "Bedroom", Metric: source.MetricMold, Value: 1.0, Unit: "riskIndex"},
		{TS: schoolTS, Source: source.SourceAirThings, DeviceID: "1110000002", Room: "school room", Metric: source.MetricRadon, Value: 40.0, Unit: "bq"},
		{TS: schoolTS, Source: source.SourceAirThings, DeviceID: "1110000002", Room: "school room", Metric: source.MetricHumidity, Value: 54.0, Unit: "pct"},
		{TS: schoolTS, Source: source.SourceAirThings, DeviceID: "1110000002", Room: "school room", Metric: source.MetricTemp, Value: 21.3, Unit: "c"},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d readings, want %d: %+v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("reading[%d]:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}

// TestParseSensorsSpotChecks asserts the specific values the contract calls out.
func TestParseSensorsSpotChecks(t *testing.T) {
	nameBySerial := map[string]string{"1110000001": "Bedroom", "1110000002": "school room"}
	body := `{"results":[
		{"serialNumber":"1110000001","recorded":"2026-05-29T12:27:00","sensors":[
			{"sensorType":"humidity","value":65.0,"unit":"pct"},
			{"sensorType":"mold","value":1.0,"unit":"riskIndex"}
		]},
		{"serialNumber":"1110000002","recorded":"2026-05-29T12:28:42","sensors":[
			{"sensorType":"radonShortTermAvg","value":40.0,"unit":"bq"}
		]}
	]}`

	got, _, err := parseSensors([]byte(body), nameBySerial)
	if err != nil {
		t.Fatalf("parseSensors returned error: %v", err)
	}

	find := func(room, metric string) (source.Reading, bool) {
		for _, r := range got {
			if r.Room == room && r.Metric == metric {
				return r, true
			}
		}
		return source.Reading{}, false
	}

	if r, ok := find("Bedroom", source.MetricHumidity); !ok || r.Value != 65.0 || r.Unit != "pct" {
		t.Errorf("Bedroom humidity: got %+v ok=%v, want value=65 unit=pct", r, ok)
	}
	if r, ok := find("Bedroom", source.MetricMold); !ok || r.Value != 1.0 {
		t.Errorf("Bedroom mold: got %+v ok=%v, want value=1.0", r, ok)
	}
	if r, ok := find("school room", source.MetricRadon); !ok || r.Value != 40.0 || r.Unit != "bq" {
		t.Errorf("school room radon: got %+v ok=%v, want value=40 unit=bq", r, ok)
	}
}

func TestParseSensorsSkipsHubAndUnknown(t *testing.T) {
	nameBySerial := map[string]string{"1110000001": "Bedroom", "1110000003": "Hub"}
	body := `{"results":[
		{"serialNumber":"1110000003","recorded":"","sensors":[]},
		{"serialNumber":"1110000001","recorded":"2026-05-29T12:27:00","sensors":[
			{"sensorType":"temp","value":16.8,"unit":"c"},
			{"sensorType":"battery","value":99.0,"unit":"pct"}
		]}
	]}`

	got, _, err := parseSensors([]byte(body), nameBySerial)
	if err != nil {
		t.Fatalf("parseSensors returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d readings, want 1 (hub skipped, unknown sensor skipped): %+v", len(got), got)
	}
	if got[0].Metric != source.MetricTemp || got[0].Room != "Bedroom" {
		t.Errorf("unexpected surviving reading: %+v", got[0])
	}
}

func TestParseSensorsRoomFallbackToSerial(t *testing.T) {
	body := `{"results":[
		{"serialNumber":"9999999999","recorded":"2026-05-29T12:27:00","sensors":[
			{"sensorType":"co2","value":800.0,"unit":"ppm"}
		]}
	]}`

	got, _, err := parseSensors([]byte(body), map[string]string{})
	if err != nil {
		t.Fatalf("parseSensors returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d readings, want 1", len(got))
	}
	if got[0].Room != "9999999999" {
		t.Errorf("unmapped device should fall back to serial as room, got %q", got[0].Room)
	}
	if got[0].Metric != source.MetricCO2 {
		t.Errorf("co2 sensor should map to MetricCO2, got %q", got[0].Metric)
	}
}

func TestParseSensorsHasNext(t *testing.T) {
	body := `{"results":[],"hasNext":true}`
	_, more, err := parseSensors([]byte(body), map[string]string{})
	if err != nil {
		t.Fatalf("parseSensors returned error: %v", err)
	}
	if !more {
		t.Errorf("hasNext=true in body should surface as more=true")
	}
}

// TestStaticInterface asserts AirThings satisfies source.Source at compile time.
var _ source.Source = (*AirThings)(nil)
