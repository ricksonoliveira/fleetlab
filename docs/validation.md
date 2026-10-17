# Local validation record

Date: 2026-10-17. Environment: macOS 27.0.1, ARM64, Go 1.27.2 (`darwin/arm64`), CGO enabled with the existing Apple Clang toolchain.

The original source-only scaffold was initially blocked by a missing Go toolchain. After installation was approved, the official Go archive was downloaded and its SHA-256 verified. The user-local installation required no sudo, password prompt, shell-profile change, credentials, or security-setting change. See [the exact installation commands](install-macos.md).

## Passed checks

```sh
GOTOOLCHAIN=local make fmt
GOTOOLCHAIN=local make check
```

The check target completed successfully:

- `gofmt -l .` reported no files needing formatting after `make fmt`.
- `go vet ./...` passed.
- `go test ./...` passed, including protocol, pipeline, and real-socket integration tests.
- `go test -race ./...` passed with no reported data races.
- `go build -o bin/server ./cmd/server` and `go build -o bin/simulator ./cmd/simulator` passed.

The default integration suite opens 100 real TCP connections and requires three processed acknowledgments per connection. It also checks bounded framing, malformed input, queue backpressure/cancellation, completion timing, processing errors, serialized replies, connection admission, idle deadlines, and cancellation during shutdown.

## Separate executable smoke test

These commands were run as separate processes on loopback:

```sh
./bin/server -addr 127.0.0.1:19000 -workers 4 -queue 256
./bin/simulator -addr 127.0.0.1:19000 -connections 100 -messages 10 -interval 10ms
```

Observed: 100 connected clients, 1,000 completed messages and matching processed acknowledgments, zero failed clients, and zero server protocol, processing, or I/O errors. SIGINT stopped the server with exit code 0 and zero active connections. Port 19000 was used because the default port was already occupied on the test machine.

This is correctness evidence, not a throughput or latency benchmark. No high-load capacity claim follows from it.

## Still unmeasured or deferred

The opt-in 1,000- and 10,000-connection tests were not run. Throughput capacity, p95/p99 latency, CPU/RSS measurements, profiling, long-duration stability, and UI screenshots remain unmeasured. All benchmark result fields remain TBD. No license has been selected.

Additional static checks found no Go syntax error nodes in the 11 source/test files, and relative Markdown file links resolved. The local Graphify AST graph was updated without an LLM and is ignored by Git, along with built binaries and profiles.
