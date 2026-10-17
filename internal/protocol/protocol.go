// Package protocol defines the newline-delimited JSON wire format.
package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"time"
)

const DefaultMaxLineBytes = 4096 // Includes the terminating newline.

var ErrMessageTooLarge = errors.New("message exceeds line limit")

type Telemetry struct {
	ID        string    `json:"id"`
	VehicleID string    `json:"vehicle_id"`
	SentAt    time.Time `json:"sent_at"`
	SpeedKPH  float64   `json:"speed_kph"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
}

type Ack struct {
	ID          string    `json:"id"`
	Status      string    `json:"status"`
	ProcessedAt time.Time `json:"processed_at,omitempty"`
	Error       string    `json:"error,omitempty"`
}

func (m Telemetry) Validate() error {
	if m.ID == "" || len(m.ID) > 128 || m.VehicleID == "" || len(m.VehicleID) > 128 {
		return errors.New("id and vehicle_id must contain 1 to 128 bytes")
	}

	if m.SentAt.IsZero() {
		return errors.New("sent_at is required")
	}

	for _, field := range []struct {
		name       string
		value, min float64
		max        float64
	}{
		{"speed_kph", m.SpeedKPH, 0, 300},
		{"latitude", m.Latitude, -90, 90},
		{"longitude", m.Longitude, -180, 180},
	} {
		if math.IsNaN(field.value) ||
			math.IsInf(field.value, 0) ||
			field.value < field.min ||
			field.value > field.max {
			return fmt.Errorf("%s is outside its allowed range", field.name)
		}
	}

	return nil
}

func DecodeTelemetry(line []byte) (Telemetry, error) {
	var m Telemetry
	d := json.NewDecoder(bytes.NewReader(line))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}

	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return m, errors.New("expected one JSON object per line")
	}

	return m, m.Validate()
}

// Reader bounds each frame and requires a newline, even before EOF.
type Reader struct {
	r   *bufio.Reader
	max int
}

func NewReader(r io.Reader, max int) *Reader {
	return &Reader{
		r:   bufio.NewReaderSize(r, max+1),
		max: max,
	}
}

func (r *Reader) ReadLine() ([]byte, error) {
	line, err := r.r.ReadSlice('\n')
	if len(line) > r.max || errors.Is(err, bufio.ErrBufferFull) {
		return nil, ErrMessageTooLarge
	}

	if errors.Is(err, io.EOF) && len(line) > 0 {
		return nil, io.ErrUnexpectedEOF
	}

	return line, err
}

func Write(w io.Writer, value any) error {
	return json.NewEncoder(w).Encode(value)
}
