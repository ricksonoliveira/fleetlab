GO ?= go
GOFMT ?= gofmt

.PHONY: fmt fmt-check test race vet build check

fmt:
	$(GOFMT) -w .

fmt-check:
	@files="$$($(GOFMT) -l .)" && test -z "$$files" || (printf '%s\n' "$$files"; exit 1)

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

build:
	mkdir -p bin
	$(GO) build -o bin/server ./cmd/server
	$(GO) build -o bin/simulator ./cmd/simulator

check: fmt-check vet test race build
