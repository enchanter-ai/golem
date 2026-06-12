# Makefile — developer convenience targets for golem.
# Authored by Enchanter Labs. MIT-licensed project.
#
# These targets mirror the exact commands run in CI (.github/workflows/ci.yml and
# wheels.yml) so that `make <target>` locally reproduces the gates without copying
# flags by hand. CI remains the source of truth; this file is a thin local mirror.

# Detect the c-shared library extension per OS (matches wheels.yml: .so / .dylib / .dll).
# Linux -> so, macOS -> dylib, Windows (MSYS/Cygwin/Git-Bash) -> dll.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
	EXT := so
endif
ifeq ($(UNAME_S),Darwin)
	EXT := dylib
endif
ifneq (,$(findstring MINGW,$(UNAME_S)))
	EXT := dll
endif
ifneq (,$(findstring MSYS,$(UNAME_S)))
	EXT := dll
endif
ifneq (,$(findstring CYGWIN,$(UNAME_S)))
	EXT := dll
endif
EXT ?= so

LIB := python/golem/libgolem.$(EXT)

.DEFAULT_GOAL := help

.PHONY: help build test race cover fmt lint vuln cshared wheel pytest clean

help: ## Show this help.
	@echo "golem — make targets:"
	@echo "  build    go build ./..."
	@echo "  test     go test -count=1 ./..."
	@echo "  race     go test -race ./..."
	@echo "  cover    go test -count=1 -cover ./..."
	@echo "  fmt      gofmt -w on all Go sources"
	@echo "  lint     golangci-lint run; staticcheck ./..."
	@echo "  vuln     govulncheck ./..."
	@echo "  cshared  build libgolem.$(EXT) c-shared library"
	@echo "  wheel    build the Python wheel"
	@echo "  pytest   run the Python binding tests"
	@echo "  clean    remove build artifacts"

build: ## Compile all Go packages.
	go build ./...

test: ## Run the Go test suite.
	go test -count=1 ./...

race: ## Run the Go test suite under the race detector.
	go test -race ./...

cover: ## Run the Go test suite with coverage.
	go test -count=1 -cover ./...

fmt: ## Format all Go sources in place.
	gofmt -w .

lint: ## Run golangci-lint and staticcheck.
	golangci-lint run
	staticcheck ./...

vuln: ## Scan for known vulnerabilities.
	govulncheck ./...

cshared: ## Build the c-shared libgolem library for the Python binding.
	go build -tags golemcapi -buildmode=c-shared -o $(LIB) ./python/capi

wheel: ## Build the Python wheel.
	cd python && python -m build --wheel

pytest: ## Run the Python binding test suite.
	cd python && python -m pytest tests -q

clean: ## Remove build artifacts.
	go clean ./...
	rm -f $(LIB) python/golem/libgolem.h
	rm -rf python/build python/dist python/*.egg-info wheelhouse
