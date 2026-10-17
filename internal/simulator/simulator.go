// Package simulator drives real TCP sockets with one outstanding message each.
package simulator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ricksonoliveira/fleetlab/internal/protocol"
)

type Config struct {
	Address     string
	Connections int
	Messages    int
	Interval    time.Duration
	Timeout     time.Duration
}

type Report struct {
	Connected  int64   `json:"connected"`
	Completed  int64   `json:"completed"`
	Errors     int64   `json:"failed_connections"`
	Seconds    float64 `json:"elapsed_seconds"`
	Throughput float64 `json:"acknowledged_messages_per_second"`
}

func Run(ctx context.Context, config Config) (Report, error) {
	if config.Address == "" || config.Connections < 1 || config.Connections > 10000 || config.Messages < 1 || config.Interval < 0 || config.Timeout <= 0 {
		return Report{}, errors.New("address, 1..10000 connections, positive messages/timeout, and nonnegative interval are required")
	}
	start := time.Now()
	var connected, completed, failures atomic.Int64
	var clients, ready sync.WaitGroup
	begin := make(chan struct{})
	firstError := make(chan error, 1)
	for id := 0; id < config.Connections; id++ {
		clients.Add(1)
		ready.Add(1)
		go func(id int) {
			defer clients.Done()
			if err := runClient(ctx, config, id, begin, &ready, &connected, &completed); err != nil {
				failures.Add(1)
				select {
				case firstError <- err:
				default:
				}
			}
		}(id)
	}
	ready.Wait()
	close(begin)
	clients.Wait()
	elapsed := time.Since(start).Seconds()
	report := Report{
		Connected: connected.Load(), Completed: completed.Load(), Errors: failures.Load(),
		Seconds: elapsed, Throughput: float64(completed.Load()) / elapsed,
	}
	select {
	case err := <-firstError:
		return report, err
	default:
		return report, ctx.Err()
	}
}

func runClient(ctx context.Context, config Config, id int, begin <-chan struct{}, ready *sync.WaitGroup, connected, completed *atomic.Int64) error {
	dialer := net.Dialer{Timeout: config.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", config.Address)
	if err != nil {
		ready.Done()
		return fmt.Errorf("client %d dial: %w", id, err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	connected.Add(1)
	ready.Done()
	select {
	case <-begin:
	case <-ctx.Done():
		return ctx.Err()
	}
	reader := protocol.NewReader(conn, protocol.DefaultMaxLineBytes)
	for sequence := 0; sequence < config.Messages; sequence++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		message := protocol.Telemetry{
			ID: fmt.Sprintf("%d-%d", id, sequence), VehicleID: fmt.Sprintf("vehicle-%d", id),
			SentAt: time.Now().UTC(), SpeedKPH: 40, Latitude: 51.5, Longitude: -0.12,
		}
		if err := conn.SetDeadline(time.Now().Add(config.Timeout)); err != nil {
			return err
		}
		if err := protocol.Write(conn, message); err != nil {
			return fmt.Errorf("client %d send: %w", id, err)
		}
		line, err := reader.ReadLine()
		if err != nil {
			return fmt.Errorf("client %d acknowledgment: %w", id, err)
		}
		var ack protocol.Ack
		if err := json.Unmarshal(line, &ack); err != nil {
			return err
		}
		if ack.ID != message.ID || ack.Status != "processed" || ack.ProcessedAt.IsZero() || ack.Error != "" {
			return fmt.Errorf("client %d received an unsuccessful or mismatched acknowledgment", id)
		}
		completed.Add(1)
		if config.Interval > 0 && sequence+1 < config.Messages {
			timer := time.NewTimer(config.Interval)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}
	}
	return nil
}
