package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/ricksonoliveira/fleetlab/internal/processing"
	"github.com/ricksonoliveira/fleetlab/internal/server"
)

func main() {
	config := server.DefaultConfig()
	address := flag.String("addr", "127.0.0.1:9000", "TCP listen address")

	flag.IntVar(&config.Workers, "workers", config.Workers, "processing workers")
	flag.IntVar(&config.QueueCapacity, "queue", config.QueueCapacity, "global queued messages")
	flag.IntVar(&config.AckCapacity, "ack-queue", config.AckCapacity, "queued replies per connection")
	flag.IntVar(&config.MaxConnections, "max-connections", config.MaxConnections, "concurrent connection limit")
	flag.IntVar(&config.MaxLineBytes, "max-line-bytes", config.MaxLineBytes, "maximum frame bytes including newline")
	flag.DurationVar(&config.ReadTimeout, "read-timeout", config.ReadTimeout, "time allowed to read a frame")
	flag.DurationVar(&config.WriteTimeout, "write-timeout", config.WriteTimeout, "time allowed to write a reply")
	flag.Parse()

	s, err := server.New(config, processing.Process)

	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.Listen("tcp", *address)

	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	log.Printf(
		"FleetLab listening on %s; workers=%d queue=%d ack_queue=%d max_connections=%d",
		listener.Addr(),
		config.Workers,
		config.QueueCapacity,
		config.AckCapacity,
		config.MaxConnections,
	)

	err = s.Serve(ctx, listener)

	if encodeErr := json.NewEncoder(os.Stdout).Encode(s.Snapshot()); encodeErr != nil {
		log.Print(encodeErr)
	}

	if err != nil {
		log.Fatal(err)
	}
}
