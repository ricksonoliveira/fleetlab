// Package pipeline owns a bounded work queue and a fixed worker pool.
package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/ricksonoliveira/fleetlab/internal/metrics"
	"github.com/ricksonoliveira/fleetlab/internal/protocol"
)

type Processor func(context.Context, protocol.Telemetry) error

type Job struct {
	Context context.Context
	Message protocol.Telemetry
	Results chan<- protocol.Ack
}

type Pool struct {
	ctx     context.Context
	jobs    chan Job
	workers sync.WaitGroup
}

// New starts workers. The caller validates sizes, cancels ctx, then calls Wait.
func New(
	ctx context.Context,
	workers, capacity int,
	process Processor,
	stats *metrics.Counters,
) *Pool {
	p := &Pool{
		ctx:  ctx,
		jobs: make(chan Job, capacity),
	}

	for i := 0; i < workers; i++ {
		p.workers.Add(1)

		go func() {
			defer p.workers.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case job := <-p.jobs:
					if ctx.Err() != nil || job.Context.Err() != nil {
						continue
					}

					ack := protocol.Ack{
						ID:     job.Message.ID,
						Status: "processed",
					}

					if err := process(job.Context, job.Message); err != nil {
						stats.ProcessingErrors.Add(1)
						ack.Status, ack.Error = "error", "processing failed"
					} else {
						stats.Processed.Add(1)
						ack.ProcessedAt = time.Now().UTC()
					}

					select {
					case job.Results <- ack:
					case <-job.Context.Done():
					case <-ctx.Done():
					}
				}
			}
		}()
	}

	return p
}

// Submit blocks when the queue is full, propagating pressure to the TCP reader.
func (p *Pool) Submit(job Job) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}

	if err := job.Context.Err(); err != nil {
		return err
	}

	select {
	case p.jobs <- job:
		return nil
	case <-p.ctx.Done():
		return p.ctx.Err()
	case <-job.Context.Done():
		return job.Context.Err()
	}
}

func (p *Pool) Wait() {
	p.workers.Wait()
}
