package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ricksonoliveira/fleetlab/internal/pipeline"
	"github.com/ricksonoliveira/fleetlab/internal/processing"
	"github.com/ricksonoliveira/fleetlab/internal/protocol"
	"github.com/ricksonoliveira/fleetlab/internal/server"
	"github.com/ricksonoliveira/fleetlab/internal/simulator"
)

func startServer(
	t *testing.T,
	config server.Config,
	process pipeline.Processor,
) (*server.Server, string, context.CancelFunc, <-chan error) {
	t.Helper()
	s, err := server.New(config, process)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- s.Serve(ctx, listener)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()

		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("server did not shut down")
		}
	})

	return s, listener.Addr().String(), cancel, done
}

func dial(t *testing.T, address string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = conn.Close()
	})
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}

	return conn
}

func expectClosed(t *testing.T, conn net.Conn) {
	t.Helper()
	if n, err := conn.Read(make([]byte, 1)); n != 0 || err == nil {
		t.Fatal("expected the connection to close without a reply")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatalf("connection timed out instead of closing: %v", err)
	}
}

func message(id string) protocol.Telemetry {
	return protocol.Telemetry{
		ID:        id,
		VehicleID: "vehicle-1",
		SentAt:    time.Now().UTC(),
		SpeedKPH:  40,
	}
}

func TestSimulatorEndToEnd100Connections(t *testing.T) {
	runConnections(t, 100)
}

// Opt in explicitly; these use real sockets and can exceed local resource limits.
func TestConnectionScale(t *testing.T) {
	value := os.Getenv("FLEETLAB_CONNECTIONS")
	if value == "" {
		t.Skip("set FLEETLAB_CONNECTIONS=1000 or 10000 on suitable hardware")
	}

	if value != "1000" && value != "10000" {
		t.Fatal("FLEETLAB_CONNECTIONS must be 1000 or 10000")
	}

	count, _ := strconv.Atoi(value)
	runConnections(t, count)
}

func runConnections(t *testing.T, count int) {
	t.Helper()
	config := server.DefaultConfig()
	config.MaxConnections = count + 16
	config.ReadTimeout = 90 * time.Second
	s, address, _, _ := startServer(t, config, processing.Process)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	report, err := simulator.Run(ctx, simulator.Config{
		Address:     address,
		Connections: count,
		Messages:    3,
		Timeout:     60 * time.Second,
	})
	if err != nil {
		t.Fatalf("simulator: %v; report=%+v", err, report)
	}

	if report.Connected != int64(count) ||
		report.Completed != int64(count*3) ||
		report.Errors != 0 {
		t.Fatalf("unexpected report: %+v", report)
	}

	if got := s.Snapshot().Processed; got != int64(count*3) {
		t.Fatalf("processed %d messages, want %d", got, count*3)
	}
}

func TestRepliesAreCompleteJSONAfterProcessing(t *testing.T) {
	config := server.DefaultConfig()
	config.Workers = 4
	_, address, _, _ := startServer(t, config, processing.Process)
	conn := dial(t, address)

	const total = 40

	for i := 0; i < total; i++ {
		if err := protocol.Write(conn, message(fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}

	reader := protocol.NewReader(conn, protocol.DefaultMaxLineBytes)
	seen := map[string]bool{}

	for i := 0; i < total; i++ {
		line, err := reader.ReadLine()
		if err != nil {
			t.Fatal(err)
		}

		var ack protocol.Ack
		if err := json.Unmarshal(line, &ack); err != nil {
			t.Fatalf("interleaved or malformed reply: %s: %v", line, err)
		}

		if seen[ack.ID] || ack.Status != "processed" || ack.ProcessedAt.IsZero() {
			t.Fatalf("invalid acknowledgment: %+v", ack)
		}

		seen[ack.ID] = true
	}

	for i := 0; i < total; i++ {
		if !seen[fmt.Sprint(i)] {
			t.Fatalf("missing acknowledgment %d", i)
		}
	}
}

func TestAcknowledgmentWaitsForProcessing(t *testing.T) {
	release := make(chan struct{})
	_, address, _, _ := startServer(t, server.DefaultConfig(), func(ctx context.Context, _ protocol.Telemetry) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	conn := dial(t, address)
	if err := protocol.Write(conn, message("blocked")); err != nil {
		t.Fatal(err)
	}

	if err := conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}

	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("received data before processing completed")
	} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("expected timeout before processing completion: %v", err)
	}

	close(release)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	line, err := protocol.NewReader(conn, protocol.DefaultMaxLineBytes).ReadLine()
	if err != nil || !strings.Contains(string(line), `"status":"processed"`) {
		t.Fatalf("missing completion: %s, %v", line, err)
	}
}

func TestRejectMalformedAndOversizedFrames(t *testing.T) {
	for _, payload := range []string{"{bad}\n", strings.Repeat("x", 4097) + "\n"} {
		t.Run(fmt.Sprint(len(payload)), func(t *testing.T) {
			s, address, _, _ := startServer(t, server.DefaultConfig(), processing.Process)
			conn := dial(t, address)
			if _, err := conn.Write([]byte(payload)); err != nil {
				t.Fatal(err)
			}

			expectClosed(t, conn)
			if got := s.Snapshot().ProtocolErrors; got != 1 {
				t.Fatalf("protocol errors=%d, want 1", got)
			}
		})
	}
}

func TestIdleDeadlineAndShutdown(t *testing.T) {
	config := server.DefaultConfig()
	config.ReadTimeout = 30 * time.Millisecond
	_, address, cancel, done := startServer(t, config, processing.Process)
	conn := dial(t, address)
	expectClosed(t, conn)
	active := dial(t, address)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown did not finish")
	}

	expectClosed(t, active)
}

func TestConnectionLimit(t *testing.T) {
	config := server.DefaultConfig()
	config.MaxConnections = 1
	s, address, _, _ := startServer(t, config, processing.Process)
	first := dial(t, address)
	if err := protocol.Write(first, message("admitted")); err != nil {
		t.Fatal(err)
	}

	if _, err := protocol.NewReader(first, protocol.DefaultMaxLineBytes).ReadLine(); err != nil {
		t.Fatal(err)
	}

	excess := dial(t, address)
	expectClosed(t, excess)
	if got := s.Snapshot(); got.Rejected != 1 || got.Active != 1 {
		t.Fatalf("unexpected admission counters: %+v", got)
	}
}

func TestShutdownCancelsInFlightProcessing(t *testing.T) {
	started := make(chan struct{})
	s, address, cancel, done := startServer(t, server.DefaultConfig(), func(ctx context.Context, _ protocol.Telemetry) error {
		close(started)
		<-ctx.Done()

		return ctx.Err()
	})
	conn := dial(t, address)
	if err := protocol.Write(conn, message("in-flight")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("processing did not start")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight processing blocked shutdown")
	}

	if got := s.Snapshot().Active; got != 0 {
		t.Fatalf("%d active connections after shutdown", got)
	}
}
