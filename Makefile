.PHONY: help build test test-race lint fmt vet tidy clean

GO ?= go
PKGS := ./...

help:
	@printf "Targets:\n"
	@printf "  build      Build all packages\n"
	@printf "  test       Run unit/integration tests\n"
	@printf "  test-race  Run tests with the race detector\n"
	@printf "  lint       Run formatting + static analysis checks (gofmt + go vet)\n"
	@printf "  fmt        Auto-format Go files (gofmt -w)\n"
	@printf "  tidy       go mod tidy\n"
	@printf "  clean      Remove build artifacts\n"

build:
	$(GO) build $(PKGS)

test:
	$(GO) test $(PKGS) -timeout 60s

test-race:
	$(GO) test $(PKGS) -race -timeout 120s

lint: vet
	@# Ensure files are gofmt'd (fails if formatting differs)
	@test -z "$$(gofmt -l . | tr -d '\n')" || (echo "gofmt needed on these files:" && gofmt -l . && exit 1)

vet:
	$(GO) vet $(PKGS)

fmt:
	gofmt -w .

tidy:
	$(GO) mod tidy

clean:
	$(GO) clean -cache -testcache


