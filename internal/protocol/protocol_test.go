package protocol

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"
	"time"
)

const validJSON = `{"id":"1","vehicle_id":"vehicle-1","sent_at":"2026-01-01T00:00:00Z","speed_kph":40,"latitude":51.5,"longitude":-0.12}`

func TestReaderLimitsAndFraming(t *testing.T) {
	for _, test := range []struct {
		name, input string
		limit       int
		want        error
	}{
		{"exact limit", "{}\n", 3, nil},
		{"too large", "{}\n", 2, ErrMessageTooLarge},
		{"unterminated", "{}", 3, io.ErrUnexpectedEOF},
		{"empty stream", "", 3, io.EOF},
		{"large unterminated", strings.Repeat("x", 100), 16, ErrMessageTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewReader(strings.NewReader(test.input), test.limit).ReadLine()
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}

	r := NewReader(io.MultiReader(strings.NewReader("{\"a\":"), strings.NewReader("1}\n{}\n")), 32)

	for _, want := range []string{"{\"a\":1}\n", "{}\n"} {
		line, err := r.ReadLine()
		if err != nil || string(line) != want {
			t.Fatalf("got %q, %v; want %q", line, err, want)
		}
	}
}

func TestDecodeTelemetry(t *testing.T) {
	if _, err := DecodeTelemetry([]byte(validJSON)); err != nil {
		t.Fatal(err)
	}

	for _, input := range []string{
		"not-json", "null", "{}", validJSON + " {}",
		strings.Replace(validJSON, `"speed_kph":40`, `"speed_kph":-1`, 1),
		strings.Replace(validJSON, `"latitude":51.5`, `"latitude":91`, 1),
		strings.Replace(validJSON, `"longitude":-0.12`, `"longitude":181`, 1),
		strings.Replace(validJSON, `"id":"1"`, `"id":""`, 1),
		strings.Replace(validJSON, `"id":"1"`, `"unknown":"x","id":"1"`, 1),
	} {
		if _, err := DecodeTelemetry([]byte(input)); err == nil {
			t.Errorf("accepted invalid telemetry: %s", input)
		}
	}

	m := Telemetry{
		ID:        "1",
		VehicleID: "v",
		SentAt:    time.Now(),
		SpeedKPH:  math.NaN(),
	}
	if m.Validate() == nil {
		t.Fatal("accepted non-finite speed")
	}
}

func BenchmarkDecodeTelemetry(b *testing.B) {
	line := []byte(validJSON)
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		if _, err := DecodeTelemetry(line); err != nil {
			b.Fatal(err)
		}
	}
}
