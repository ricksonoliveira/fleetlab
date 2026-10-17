// Package processing contains the deliberately small initial processing step.
package processing

import (
	"context"

	"github.com/ricksonoliveira/fleetlab/internal/protocol"
)

// Process validates telemetry in memory. A successful return is not durable storage.
// shortcut: no enrichment or persistence, add only after choosing a concrete use case.
func Process(ctx context.Context, message protocol.Telemetry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return message.Validate()
}
