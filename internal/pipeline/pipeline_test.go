package pipeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ricksonoliveira/fleetlab/internal/metrics"
	"github.com/ricksonoliveira/fleetlab/internal/protocol"
)

func TestBoundedQueueAndCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stats := &metrics.Counters{}
	started, release := make(chan struct{}), make(chan struct{})
	p := New(ctx, 1, 1, func(ctx context.Context, _ protocol.Telemetry) error {
		select {
		case started <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, stats)
	defer func() { cancel(); p.Wait() }()
	results := make(chan protocol.Ack, 3)
	job := Job{Context: ctx, Message: protocol.Telemetry{ID: "1"}, Results: results}
	if err := p.Submit(job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("processor did not start")
	}
	select {
	case <-results:
		t.Fatal("acknowledged before processing completed")
	default:
	}
	if err := p.Submit(job); err != nil {
		t.Fatal(err)
	}
	blocked, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	defer stop()
	job.Context = blocked
	if err := p.Submit(job); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("full queue did not block until cancellation: %v", err)
	}
	close(release)
	select {
	case ack := <-results:
		if ack.Status != "processed" || ack.ProcessedAt.IsZero() || stats.Processed.Load() < 1 {
			t.Fatalf("invalid completion: %+v", ack)
		}
	case <-time.After(time.Second):
		t.Fatal("no completion after processing")
	}
}

func TestProcessingFailureAcknowledgment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stats := &metrics.Counters{}
	p := New(ctx, 1, 1, func(context.Context, protocol.Telemetry) error {
		return errors.New("private implementation detail")
	}, stats)
	defer func() { cancel(); p.Wait() }()
	results := make(chan protocol.Ack, 1)
	if err := p.Submit(Job{Context: ctx, Message: protocol.Telemetry{ID: "failed"}, Results: results}); err != nil {
		t.Fatal(err)
	}
	select {
	case ack := <-results:
		if ack.Status != "error" || ack.Error != "processing failed" || stats.ProcessingErrors.Load() != 1 || stats.Processed.Load() != 0 {
			t.Fatalf("unexpected failure acknowledgment: %+v", ack)
		}
	case <-time.After(time.Second):
		t.Fatal("no failure acknowledgment")
	}
}
