# One command builds everything:
#
#   make            project packages, the interface and all three programs
#   make run        build, then start the app
#   make motion     the five ways the app shows work in hand, in a browser
#   make test       all tests
#   make check      what this machine still needs to run framefairy
#   make tools      install the system tools that are missing (macOS)
#   make models     download the speech model and the language model
#   make clean      remove what make built and downloaded into the project
#
# The programs land in bin/. Details are in docs/BUILD.md.

SHELL := /bin/sh

GO ?= go
NPM ?= npm
# How much work each fuzz target does in make test, as a number of
# executions. A count and not a clock, so a run here and a run in CI do the
# same thing on a fast machine and a slow one. A duration like 30s also
# works, for a deep run by hand.
FUZZTIME ?= 10000x
BIN := bin
STAMPS := .build
EXE :=
ifeq ($(OS),Windows_NT)
EXE := .exe
endif
UNAME := $(shell uname -s 2>/dev/null)

# Go 1.27 builds its own code for macOS 13, so everything else is built for
# the same version. Without this, clang builds C code for the running macOS
# and some libraries ask for 10.13, and the linker warns about the mismatch.
# The linker flag comes last on the command line, so it wins over the
# libraries' own.
MACOS_MIN := 13.0
LDFLAGS :=
ifeq ($(UNAME),Darwin)
export MACOSX_DEPLOYMENT_TARGET := $(MACOS_MIN)
export CGO_CFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN)
export CGO_CXXFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN)
LDFLAGS := -extldflags=-mmacosx-version-min=$(MACOS_MIN)
endif

# The speech library is native code, so Go's C support must be on. Go never
# downloads another toolchain behind your back, unless GOTOOLCHAIN is set
# on purpose, as the cloud environment does.
export CGO_ENABLED := 1
GOTOOLCHAIN ?= local
export GOTOOLCHAIN

# TIDY=0 skips resolving the Go modules, for machines without network.
TIDY ?= 1

UI_SOURCES := $(shell find frontend/src -type f 2>/dev/null) frontend/index.html \
	frontend/package.json frontend/vite.config.ts frontend/svelte.config.js frontend/tsconfig.json
UI_BUILT := cmd/framefairy-app/dist/app/index.html

PROGRAMS := $(BIN)/framefairy$(EXE) $(BIN)/framefairy-app$(EXE) $(BIN)/framefairy-train$(EXE)

.PHONY: all run motion test fuzz check tools models clean help toolchain modules $(PROGRAMS)

all: toolchain $(PROGRAMS)
	@echo "Ready: $(PROGRAMS)"
	@sh scripts/check.sh --quiet

help:
	@sed -n '1,11p' Makefile | sed 's/^# \{0,1\}//'

# Go and a C compiler, checked before anything is built.
toolchain:
	@sh scripts/check.sh --toolchain

# The project's own Go packages. Checked by content on every run, because
# copied files keep old timestamps and would fool a time-based check.
# go.sum is rewritten to match go.mod, and the downloads are only shown when
# something fails.
modules:
	@mkdir -p $(STAMPS)
ifeq ($(TIDY),1)
	@sum=$$(cat go.mod go.sum 2>/dev/null | cksum); \
	if [ "$$sum" != "$$(cat $(STAMPS)/modules 2>/dev/null)" ]; then \
		echo "Resolving Go modules"; \
		out=$$($(GO) mod tidy 2>&1) || { echo "$$out"; exit 1; }; \
		cat go.mod go.sum | cksum > $(STAMPS)/modules; \
	fi
endif

# The interface's packages, exactly as locked.
frontend/node_modules/.package-lock.json: frontend/package-lock.json
	@echo "Installing the interface packages"
	@out=$$(cd frontend && $(NPM) ci --no-audit --no-fund --loglevel=error 2>&1) || { echo "$$out"; exit 1; }

# The interface, built from frontend/ into cmd/framefairy-app/dist/app/, which
# is not in the repository. It needs Node.js.
$(UI_BUILT): $(UI_SOURCES)
	@command -v $(NPM) >/dev/null 2>&1 || { \
		echo "Node.js is needed to build the app's interface. Install it with: brew install node"; \
		echo "See docs/INSTALL.md for other systems."; \
		exit 1; }
	@$(MAKE) -s --no-print-directory frontend/node_modules/.package-lock.json
	@echo "Building the interface"
	@cd frontend && $(NPM) run --silent build -- --logLevel warn

# The programs are always handed to go build, which rebuilds only what
# changed and takes a moment otherwise.
$(BIN)/framefairy$(EXE): modules
	@echo "Building $@"
	@$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/framefairy
ifeq ($(OS),Windows_NT)
	@sh scripts/check.sh --copy-dlls $(BIN)
endif

$(BIN)/framefairy-app$(EXE): modules $(UI_BUILT)
	@echo "Building $@"
	@$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/framefairy-app

$(BIN)/framefairy-train$(EXE): modules
	@echo "Building $@"
	@$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/framefairy-train

run: all
	@$(BIN)/framefairy-app$(EXE)

# Every way the app says work is in hand, on one page, in a browser. It is
# preview material and never goes into the app.
motion: frontend/node_modules/.package-lock.json
	@cd frontend && npx vite --config preview/motion.config.ts

test: toolchain modules fuzz
	@if command -v $(NPM) >/dev/null 2>&1; then \
		$(MAKE) -s --no-print-directory frontend/node_modules/.package-lock.json && \
		out=$$(cd frontend && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }; \
		printf 'ok  \tinterface types\n'; \
		out=$$(cd frontend && $(NPM) run --silent test 2>&1) || { echo "$$out"; exit 1; }; \
		printf 'ok  \tinterface rules\n'; \
	fi

# The tests run with the race detector, because the app is a queue of jobs
# on their own goroutines and a window asking them things from another, and
# a race there is a bug that only shows up on someone else's machine. The
# fuzzing runs without it: it is the same code, many more times over.
#
# Every fuzz target does FUZZTIME executions, one worker each, as many
# targets at a time as the machine has cores. A new crasher is written to
# testdata/fuzz/ next to the code, where it stays as a seed.
fuzz: toolchain modules
	@$(GO) test -race -ldflags '$(LDFLAGS)' ./...
	@GO='$(GO)' FUZZTIME='$(FUZZTIME)' sh scripts/fuzz.sh

check:
	@sh scripts/check.sh || true

tools:
	@sh scripts/tools.sh

models:
	@sh scripts/models.sh

clean:
	@rm -rf $(BIN) $(STAMPS) frontend/node_modules frontend/preview/dist frontend/preview/dist-motion
	@$(GO) clean -fuzzcache
	@echo "Removed bin/, .build/, frontend/node_modules/, the preview builds and the fuzz corpus"
