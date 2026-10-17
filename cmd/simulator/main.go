package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ricksonoliveira/fleetlab/internal/simulator"
)

func main() {
	var config simulator.Config

	flag.StringVar(&config.Address, "addr", "127.0.0.1:9000", "server address")
	flag.IntVar(&config.Connections, "connections", 100, "persistent TCP connections (1..10000)")
	flag.IntVar(&config.Messages, "messages", 10, "messages per connection")
	flag.DurationVar(&config.Interval, "interval", 100*time.Millisecond, "pause between acknowledged messages per connection")
	flag.DurationVar(&config.Timeout, "timeout", 5*time.Second, "dial and per-message round-trip timeout")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	report, err := simulator.Run(ctx, config)

	if encodeErr := json.NewEncoder(os.Stdout).Encode(report); encodeErr != nil {
		log.Fatal(encodeErr)
	}

	if err != nil {
		log.Fatal(err)
	}
}
