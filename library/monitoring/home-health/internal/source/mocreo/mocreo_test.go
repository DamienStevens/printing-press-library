package mocreo

import (
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/monitoring/home-health/internal/source"
)

func TestParseSamples(t *testing.T) {
	const nodeID = "node-0001"
	const room = "Kitchen"

	tests := []struct {
		name string
		body string
		want []source.Reading
	}{
		{
			name: "temp and humidity from one record",
			body: `{"code":200,"data":{"records":[{"time":1780058161,"data":{"tm":2069,"hu":5701}}]}}`,
			want: []source.Reading{
				{
					TS:       time.Unix(1780058161, 0).UTC(),
					Source:   source.SourceMOCREO,
					DeviceID: nodeID,
					Room:     room,
					Metric:   source.MetricTemp,
					Value:    20.69,
					Unit:     "c",
				},
				{
					TS:       time.Unix(1780058161, 0).UTC(),
					Source:   source.SourceMOCREO,
					DeviceID: nodeID,
					Room:     room,
					Metric:   source.MetricHumidity,
					Value:    57.01,
					Unit:     "pct",
				},
			},
		},
		{
			name: "hm alias is accepted for humidity",
			body: `{"code":200,"data":{"records":[{"time":1780058161,"data":{"tm":2069,"hm":5701}}]}}`,
			want: []source.Reading{
				{
					TS:       time.Unix(1780058161, 0).UTC(),
					Source:   source.SourceMOCREO,
					DeviceID: nodeID,
					Room:     room,
					Metric:   source.MetricTemp,
					Value:    20.69,
					Unit:     "c",
				},
				{
					TS:       time.Unix(1780058161, 0).UTC(),
					Source:   source.SourceMOCREO,
					DeviceID: nodeID,
					Room:     room,
					Metric:   source.MetricHumidity,
					Value:    57.01,
					Unit:     "pct",
				},
			},
		},
		{
			name: "occupancy and leak keys are ignored",
			body: `{"code":200,"data":{"records":[{"time":1780058161,"data":{"tm":2069,"hu":5701,"o":true,"s":false}}]}}`,
			want: []source.Reading{
				{
					TS:       time.Unix(1780058161, 0).UTC(),
					Source:   source.SourceMOCREO,
					DeviceID: nodeID,
					Room:     room,
					Metric:   source.MetricTemp,
					Value:    20.69,
					Unit:     "c",
				},
				{
					TS:       time.Unix(1780058161, 0).UTC(),
					Source:   source.SourceMOCREO,
					DeviceID: nodeID,
					Room:     room,
					Metric:   source.MetricHumidity,
					Value:    57.01,
					Unit:     "pct",
				},
			},
		},
		{
			name: "temp only when humidity absent",
			body: `{"code":200,"data":{"records":[{"time":1780058161,"data":{"tm":1850}}]}}`,
			want: []source.Reading{
				{
					TS:       time.Unix(1780058161, 0).UTC(),
					Source:   source.SourceMOCREO,
					DeviceID: nodeID,
					Room:     room,
					Metric:   source.MetricTemp,
					Value:    18.5,
					Unit:     "c",
				},
			},
		},
		{
			name: "empty records yields no readings",
			body: `{"code":200,"data":{"records":[]}}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSamples(nodeID, room, []byte(tt.body))
			if err != nil {
				t.Fatalf("parseSamples returned error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d readings, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("reading[%d]:\n got  %+v\n want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseSamplesConversion(t *testing.T) {
	body := `{"code":200,"data":{"records":[{"time":1780058161,"data":{"tm":2069,"hu":5701}}]}}`
	got, err := parseSamples("node", "room", []byte(body))
	if err != nil {
		t.Fatalf("parseSamples returned error: %v", err)
	}
	if got[0].Value != 20.69 {
		t.Errorf("tm=2069 should convert to 20.69 C, got %v", got[0].Value)
	}
	if got[1].Value != 57.01 {
		t.Errorf("hu=5701 should convert to 57.01 %%RH, got %v", got[1].Value)
	}
}

func TestParseNodes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []node
	}{
		{
			name: "data is a plain array",
			body: `{"code":200,"data":[{"nodeId":"abc","type":"SENSOR","model":"ST6","name":"Kitchen","onlined":true},{"nodeId":"def","name":"Bedroom"}]}`,
			want: []node{{NodeID: "abc", Name: "Kitchen"}, {NodeID: "def", Name: "Bedroom"}},
		},
		{
			name: "data wraps nodes array",
			body: `{"code":200,"data":{"nodes":[{"nodeId":"abc","name":"Kitchen"}]}}`,
			want: []node{{NodeID: "abc", Name: "Kitchen"}},
		},
		{
			name: "data wraps list array",
			body: `{"code":200,"data":{"list":[{"nodeId":"xyz","name":"Garage"}]}}`,
			want: []node{{NodeID: "xyz", Name: "Garage"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseNodes([]byte(tt.body))
			if err != nil {
				t.Fatalf("parseNodes returned error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d nodes, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("node[%d]: got %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestStaticInterface asserts MOCREO satisfies source.Source at compile time.
var _ source.Source = (*MOCREO)(nil)
