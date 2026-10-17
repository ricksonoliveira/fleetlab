# Benchmark notebook

No performance results have been recorded. Every result remains **TBD** until a reproducible run is captured. Correctness tests and sample commands are not benchmark evidence.

Copy [results-template.md](results-template.md) for each measurement. Record hardware, OS, exact Go version, revision or source snapshot, workload, CPU policy, both process configurations, and raw outputs. The module path is `github.com/ricksonoliveira/fleetlab`.

## Procedure

1. Run formatting, vet, tests, and race tests. Benchmark a normal build without `-race`.
2. Record `go version`, `go env GOOS GOARCH`, `sysctl -n hw.model hw.ncpu hw.memsize` on macOS, and the existing `ulimit -n`. Record power/thermal state, background applications, and `GOMAXPROCS` (unset means runtime default). Do not silently alter limits or system settings.
3. Build once with `make build`. Start with 100 connections, then 1,000 if resources allow. 10,000 is exploratory. A same-Mac run needs roughly two sockets per connection across both processes, plus overhead.
4. Record the exact server and generator command lines. Run a warm-up, then at least three measured repetitions. Current summary includes startup and pacing; clearly label that. A steady-state measurement window is deferred.
5. Require the expected count of processed acknowledgments and report failed connections, protocol/processing/I/O errors, disconnects, and unfinished messages. Preserve failed runs as evidence.
6. Measure server and generator CPU/RSS separately with existing OS tools. Record sampling interval, peak versus average, and units. Same-host contention can bottleneck the generator; compare separate hosts before attributing capacity to the server.
7. End-to-end p95/p99 need per-message send-to-ack durations and a declared collection/quantile method. The bundled summary does not provide them yet. Leave them TBD until instrumentation exists; throughput cannot substitute for latency.
8. Vary one factor at a time: connections, workers, queue capacity, acknowledgment queue capacity, or workload. Keep the validation-only processor limitation visible.

## Example commands (no results implied)

```sh
./bin/server -workers 4 -queue 256 -ack-queue 16 -max-connections 2000
./bin/simulator -connections 100 -messages 1000 -interval 10ms -timeout 30s
./bin/simulator -connections 1000 -messages 1000 -interval 10ms -timeout 30s
```

Do not run the two simulator commands simultaneously unless that combined workload is the intended experiment. `-interval 0` removes deliberate pacing; each socket still waits for its acknowledgment before sending the next message. Application/server admission and OS limits can prevent a 10,000-socket run. Do not present that target as achieved capacity.

Parsing microbenchmarks and profiling commands are in [architecture.md](../docs/architecture.md#verification-and-profiling). They answer a narrower question than the TCP workload.
