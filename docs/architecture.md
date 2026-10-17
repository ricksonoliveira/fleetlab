# Architecture notes

## End-to-end path

1. The server accepts TCP sockets up to `-max-connections` (default 2,000). Excess connections are closed and counted. One reader and one writer goroutine serve each admitted connection.
2. The reader permits at most 4,096 bytes per frame, including the newline. It refreshes a 30-second read deadline before each frame and rejects unknown fields, invalid IDs/timestamps, non-finite numbers, speed outside 0..300 km/h, latitude outside -90..90, and longitude outside -180..180. IDs are 1..128 bytes; timestamps must be nonzero, but clock skew is not checked. These bounds are demo policy, not a claim about every real vehicle.
3. The reader submits a job to a shared bounded channel (default capacity 256). A full channel blocks submission, which stops further application reads. Kernel TCP buffers are finite too; the sender eventually waits.
4. Four workers by default perform a second validation step. `processing.Process` is the deliberate extension point for a later real workload. Workers share no mutable vehicle state and do not store telemetry.
5. After the processor returns, the worker submits a result to that connection's bounded reply channel (default capacity 16). Exactly one writer encodes acknowledgments, with a 5-second write deadline per reply. Completion order may differ from input order. IDs identify messages.

The server owns the listener and all sockets. The caller cancels its context to stop it. Cancellation closes sockets, unblocks reads/writes and queue submissions, cancels processor contexts, and waits for handlers and workers. Queued work is not drained. Any future processor must honor cancellation so shutdown can finish. A Server instance is for a single Serve call.

An acknowledgment confirms completion of the in-memory step, not persistence. A disconnect can lose an acknowledgment after work has completed; the client cannot infer whether work ran. There is no exactly-once guarantee, deduplication, retransmission, or durable recovery. A successful socket write is counted by the server; the simulator counts only acknowledgments it actually reads and validates.

## Bounds and fairness

Memory scales with admitted connections, fixed-size frame buffers, per-connection reply channels, the global queue, active workers, and OS socket buffers. Each reader may hold one additional decoded message while waiting to enqueue. JSON IDs and frame sizes are bounded. The server does not retain a growing history or latency sample list.

A slow reader can fill its reply queue and block a worker while its writer waits for the network. Other workers may still progress, but enough slow readers can occupy the entire pool. Write deadlines eventually release them when writes block. This is an intentional first-slice limit; fair per-client scheduling and separate result dispatch would be follow-up work if measurements justify them. The read deadline applies to reading a frame, not total queue residence time. There is no per-job processing timeout or queue-time SLA yet.

The simulator dials one real socket per client, waits for all dial attempts, and then starts sending. Each connection has one request in flight and can send repeatedly. This exercises persistent connections and worker contention but is a closed-loop workload: slowing acknowledgments slows the offered rate. It is not an open-loop arrival-rate generator or a coordinated-omission-corrected latency tool. Connection establishment is included in reported elapsed time. Successful TCP handshakes alone do not prove application admission; require all expected processed acknowledgments.

## Concurrency versus CPU parallelism

Go goroutines multiplex many I/O waits onto operating-system threads. A thousand connections are a concurrency experiment; they do not require a thousand cores. Simultaneous CPU work is constrained by cores, `GOMAXPROCS`, processor cost, and worker count. Increasing workers can increase contention and allocation without improving throughput. Record `GOMAXPROCS` and vary workers independently from connections and queue capacity. A larger queue absorbs bursts but can increase latency and memory without increasing processing capacity.

## Verification and profiling

With Go already installed:

```sh
GOTOOLCHAIN=local make fmt
GOTOOLCHAIN=local make check
go test ./tests/integration -count=1
go test -race ./... -count=1
```

Tests exercise bounded framing, schema errors, the queue's blocking/cancellation behavior, processing failure acknowledgments, acknowledgments after processing, serialized pipelined replies, admission limits, idle deadlines, cancellation of in-flight processing during shutdown, and 100 real socket connections. Execution results are recorded in [validation.md](validation.md). The 1,000/10,000 scale test is opt-in. Full slow-reader fairness, reconnects, write-timeout fault injection, long-duration soak tests, and production operating conditions remain deferred.

The following profiles isolate parsing, not server throughput:

```sh
mkdir -p profiles
go test ./internal/protocol -run '^$' -bench BenchmarkDecodeTelemetry -benchmem -benchtime=10s -cpuprofile profiles/protocol-cpu.out -memprofile profiles/protocol-memory.out -o profiles/protocol.test
go tool pprof profiles/protocol.test profiles/protocol-cpu.out
go tool pprof -alloc_space profiles/protocol.test profiles/protocol-memory.out
```

For an end-to-end correctness workload, collect profiles from the integration process:

```sh
go test ./tests/integration -run '^TestSimulatorEndToEnd100Connections$' -count=1 -cpuprofile profiles/integration-cpu.out -memprofile profiles/integration-memory.out -trace profiles/integration.trace -o profiles/integration.test
go tool pprof profiles/integration.test profiles/integration-cpu.out
go tool trace profiles/integration.trace
```

These short integration profiles combine the server and generator and may have few samples. For useful isolated profiles, a future step is opt-in process profiling in each executable plus a longer representative workload. No pprof HTTP listener is exposed. Use Activity Monitor or existing OS tools to capture separate server/generator CPU and resident memory during a longer run. Do not treat Go heap allocation profiles as total process RSS. Keep profiles out of Git; they can contain captured program data.

## Deferred deliberately

- Durable state, database, Kafka, Kubernetes, deployment automation.
- TLS, authentication, abuse protection beyond basic bounds, public network hosting.
- Representative enrichment/aggregation and per-vehicle ordering.
- Histograms for p95/p99, queue occupancy/high-water marks, periodic metrics export, UI.
- Retries, deduplication, automatic reconnects, graceful draining, fairness across clients.

The next step should follow a measured question, not add infrastructure by default.
