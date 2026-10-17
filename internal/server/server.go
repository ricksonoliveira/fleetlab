// Package server accepts persistent TCP connections and serializes their replies.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ricksonoliveira/fleetlab/internal/metrics"
	"github.com/ricksonoliveira/fleetlab/internal/pipeline"
	"github.com/ricksonoliveira/fleetlab/internal/protocol"
)

type Config struct {
	Workers        int
	QueueCapacity  int
	AckCapacity    int
	MaxConnections int
	MaxLineBytes   int
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

func DefaultConfig() Config {
	return Config{
		Workers:        4,
		QueueCapacity:  256,
		AckCapacity:    16,
		MaxConnections: 2000,
		MaxLineBytes:   protocol.DefaultMaxLineBytes,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   5 * time.Second,
	}
}

type Server struct {
	config  Config
	process pipeline.Processor
	stats   metrics.Counters
}

func New(config Config, process pipeline.Processor) (*Server, error) {
	if config.Workers < 1 ||
		config.QueueCapacity < 1 ||
		config.AckCapacity < 1 ||
		config.MaxConnections < 1 {
		return nil, errors.New("workers, queue capacities, and connection limit must be positive")
	}

	if config.MaxLineBytes < 1 ||
		config.MaxLineBytes > 1<<20 ||
		config.ReadTimeout <= 0 ||
		config.WriteTimeout <= 0 {
		return nil, errors.New("line limit must be 1..1048576 bytes and timeouts must be positive")
	}

	if process == nil {
		return nil, errors.New("processor is required")
	}

	return &Server{
		config:  config,
		process: process,
	}, nil
}

func (s *Server) Snapshot() metrics.Snapshot {
	return s.stats.Snapshot()
}

// Serve owns listener. Cancellation closes sockets and cancels queued work;
// it waits for handlers and workers rather than promising to drain the queue.
// A Server is intended for one Serve call. Processors must honor their context.
func (s *Server) Serve(parent context.Context, listener net.Listener) error {
	ctx, cancel := context.WithCancel(parent)
	pool := pipeline.New(ctx, s.config.Workers, s.config.QueueCapacity, s.process, &s.stats)

	var handlers sync.WaitGroup
	stopListener := context.AfterFunc(ctx, func() {
		_ = listener.Close()
	})
	defer func() {
		cancel()
		_ = listener.Close()
		stopListener()
		handlers.Wait()
		pool.Wait()
	}()

	slots := make(chan struct{}, s.config.MaxConnections)

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("accept: %w", err)
		}

		select {
		case slots <- struct{}{}:
		default:
			s.stats.Rejected.Add(1)
			_ = conn.Close()
			continue
		}

		s.stats.Accepted.Add(1)
		s.stats.Active.Add(1)
		handlers.Add(1)

		go func() {
			defer handlers.Done()
			defer func() {
				<-slots
				s.stats.Active.Add(-1)
			}()

			s.handle(ctx, conn, pool)
		}()
	}
}

func (s *Server) handle(parent context.Context, conn net.Conn, pool *pipeline.Pool) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	acks := make(chan protocol.Ack, s.config.AckCapacity)
	writerDone := make(chan struct{})

	go func() {
		defer close(writerDone)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				return
			case ack := <-acks:
				if err := conn.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout)); err != nil {
					s.stats.IOErrors.Add(1)

					return
				}
				if err := protocol.Write(conn, ack); err != nil {
					if ctx.Err() == nil {
						s.stats.IOErrors.Add(1)
					}

					return
				}
				s.stats.Acknowledged.Add(1)
			}
		}
	}()
	defer func() {
		cancel()
		_ = conn.Close()
		stop()
		<-writerDone
	}()

	reader := protocol.NewReader(conn, s.config.MaxLineBytes)

	for {
		if err := conn.SetReadDeadline(time.Now().Add(s.config.ReadTimeout)); err != nil {
			return
		}

		line, err := reader.ReadLine()
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, io.EOF) {
				if errors.Is(err, protocol.ErrMessageTooLarge) || errors.Is(err, io.ErrUnexpectedEOF) {
					s.stats.ProtocolErrors.Add(1)
				} else {
					s.stats.IOErrors.Add(1)
				}
			}

			return
		}

		message, err := protocol.DecodeTelemetry(line)
		if err != nil {
			s.stats.ProtocolErrors.Add(1)

			return
		}

		s.stats.Received.Add(1)
		if err := pool.Submit(pipeline.Job{
			Context: ctx,
			Message: message,
			Results: acks,
		}); err != nil {
			return
		}
	}
}
