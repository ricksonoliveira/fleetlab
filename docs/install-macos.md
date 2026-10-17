# Go installation record: macOS ARM64

Go 1.27.2 was installed from the [official Go distribution](https://go.dev/dl/) on 2026-10-17, after checking `uname -m` (`arm64`) and the existing package manager. Homebrew was available; the official binary archive was chosen for a user-local installation without sudo or an OS password prompt. The archive's SHA-256 was checked against the official release metadata before extraction.

These are the exact installation commands used. They refuse to replace an existing version directory or executable. On the validated Mac, `$HOME/.local/bin` already existed and was on PATH; no shell profile was modified.

```sh
set -eu
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 https://go.dev/dl/go1.27.2.darwin-arm64.tar.gz -o /tmp/fleetlab-go1.27.2.darwin-arm64.tar.gz
printf '%s\n' '76812b213b1b2302c978d28fa52fa92d541704b9e7d9d5db8002c50e4018c4c5  /tmp/fleetlab-go1.27.2.darwin-arm64.tar.gz' | shasum -a 256 -c -
test ! -e "$HOME/.local/share/go/go1.27.2"
test ! -e "$HOME/.local/bin/go"
test ! -e "$HOME/.local/bin/gofmt"
mkdir -p "$HOME/.local/share/go"
mkdir "$HOME/.local/share/go/go1.27.2"
tar -xzf /tmp/fleetlab-go1.27.2.darwin-arm64.tar.gz -C "$HOME/.local/share/go/go1.27.2" --strip-components=1
ln -s "$HOME/.local/share/go/go1.27.2/bin/go" "$HOME/.local/bin/go"
ln -s "$HOME/.local/share/go/go1.27.2/bin/gofmt" "$HOME/.local/bin/gofmt"
go version
go env GOROOT GOOS GOARCH CGO_ENABLED
```

Observed version: `go version go1.27.2 darwin/arm64`. For another operating system or architecture, choose its corresponding current supported release and checksum from the official site instead of using this ARM64 archive. If the directory or links already exist, inspect the existing installation before deciding how to upgrade it.

## Validate and run FleetLab

From the project directory:

```sh
GOTOOLCHAIN=local make fmt
GOTOOLCHAIN=local make check
```

Terminal 1:

```sh
./bin/server -addr 127.0.0.1:19000 -workers 4 -queue 256
```

Terminal 2:

```sh
./bin/simulator -addr 127.0.0.1:19000 -connections 100 -messages 10 -interval 10ms
```

Stop the server with Ctrl-C. `make check` builds the binaries after formatting verification, vet, tests, and race tests. See [validation.md](validation.md) for observed outcomes and the [benchmark notebook](../benchmarks/README.md) for the limits of these correctness runs.
