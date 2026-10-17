// Package metrics provides process-local counters without an external service.
package metrics

import "sync/atomic"

type Counters struct {
	Accepted         atomic.Int64
	Rejected         atomic.Int64
	Active           atomic.Int64
	Received         atomic.Int64
	Processed        atomic.Int64
	ProcessingErrors atomic.Int64
	ProtocolErrors   atomic.Int64
	IOErrors         atomic.Int64
	Acknowledged     atomic.Int64
}

type Snapshot struct {
	Accepted         int64 `json:"accepted_connections"`
	Rejected         int64 `json:"rejected_connections"`
	Active           int64 `json:"active_connections"`
	Received         int64 `json:"received_messages"`
	Processed        int64 `json:"processed_messages"`
	ProcessingErrors int64 `json:"processing_errors"`
	ProtocolErrors   int64 `json:"protocol_errors"`
	IOErrors         int64 `json:"io_errors"`
	Acknowledged     int64 `json:"written_acknowledgments"`
}

// Snapshot reads each counter atomically; the group is not a transaction.
func (c *Counters) Snapshot() Snapshot {
	return Snapshot{
		Accepted:         c.Accepted.Load(),
		Rejected:         c.Rejected.Load(),
		Active:           c.Active.Load(),
		Received:         c.Received.Load(),
		Processed:        c.Processed.Load(),
		ProcessingErrors: c.ProcessingErrors.Load(),
		ProtocolErrors:   c.ProtocolErrors.Load(),
		IOErrors:         c.IOErrors.Load(),
		Acknowledged:     c.Acknowledged.Load(),
	}
}
