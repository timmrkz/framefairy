# One command does everything:
#
#   make            install what is missing, build all three programs
#   make run        the same, then start the app
#
# That is the whole of it for everyday work. make installs the tools this
# machine lacks and builds the ffmpeg framefairy ships, once, and after
# that it is an ordinary build. The models are not make's business: the app
# fetches the speech model and the language model itself, on first run,
# which is what a customer does.
#
# The rest, for when you want one part of it:
#
#   make motion     the five ways the app shows work in hand, in a browser
#   make ffmpeg     build the ffmpeg we ship again, from scratch
#   make test       all tests: unit, fuzz and interface
#   make unit       the Go tests, under the race detector
#   make fuzz       the fuzz targets, FUZZTIME executions each
#   make interface  the interface type check and its own tests
#   make check      what this machine still needs to run framefairy
#   make tools      install the missing tools and nothing else
#   make models     download the models for the command line, which has no
#                   window to ask in
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

# Whether make may install what this machine is missing. On, because one
# command has to be enough to get from a fresh machine to a running app.
# Off wherever CI is set: a build runner installs its own packages from its
# own workflow, and handing it a Homebrew would be a different build from
# the one it was asked for.
INSTALL ?= $(if $(CI),0,1)

UI_SOURCES := $(shell find frontend/src -type f 2>/dev/null) frontend/index.html \
	frontend/package.json frontend/vite.config.ts frontend/svelte.config.js frontend/tsconfig.json
UI_BUILT := cmd/framefairy-app/dist/app/index.html

PROGRAMS := $(BIN)/framefairy$(EXE) $(BIN)/framefairy-app$(EXE) $(BIN)/framefairy-train$(EXE)

.PHONY: all run motion ffmpeg deps tools-beside test unit fuzz interface check tools models clean help toolchain modules $(PROGRAMS)

all: deps toolchain $(PROGRAMS) tools-beside
	@echo "Ready: $(PROGRAMS)"
	@sh scripts/check.sh --quiet

# What this machine is missing, installed. Before the toolchain check,
# because the toolchain is one of the things it installs, and it costs
# nothing when there is nothing to do: every check inside it is a command -v
# or a file test.
deps:
ifeq ($(INSTALL),1)
	@sh scripts/tools.sh $(STAMPS)/ffmpeg
endif

# Our own ffmpeg goes beside the programs, where they look before the search
# path. Only if there is one: on a machine where the build has not run, or
# did not work, the search path answers instead.
#
# This is what makes the development build use the ffmpeg a customer will
# use, rather than whatever Homebrew happens to have installed.
tools-beside:
	@if [ -x $(STAMPS)/ffmpeg/bin/ffmpeg ]; then 		cp $(STAMPS)/ffmpeg/bin/ffmpeg $(STAMPS)/ffmpeg/bin/ffprobe $(BIN)/ && 		echo "Using our own ffmpeg, from make ffmpeg"; 	fi

help:
	@sed -n '1,28p' Makefile | sed 's/^# \{0,1\}//'

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
# The speech library travels with the program rather than being found in
# the Go module cache, which is where the program would otherwise look and
# which exists on no machine but the one that built it. See
# docs/PACKAGING.md and scripts/carry-libs.sh.
#
# Done on every build rather than only when packaging, so what is run every
# day is what is shipped. A program built without it runs on this machine
# and nowhere else, which is a thing to find out here rather than from a
# customer.
$(BIN)/framefairy$(EXE): modules
	@echo "Building $@"
	@$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/framefairy
	@sh scripts/carry-libs.sh $@ $(BIN)/lib

$(BIN)/framefairy-app$(EXE): modules $(UI_BUILT)
	@echo "Building $@"
	@$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/framefairy-app
	@sh scripts/carry-libs.sh $@ $(BIN)/lib

$(BIN)/framefairy-train$(EXE): modules
	@echo "Building $@"
	@$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/framefairy-train

run: all
	@$(BIN)/framefairy-app$(EXE)

# The ffmpeg we ship, built from source without libx264 so the build is
# LGPL. make builds it once, when it is not there. This builds it again
# whatever is there, for when scripts/build-ffmpeg.sh has changed or the
# last one went wrong.
ffmpeg:
	@rm -rf $(STAMPS)/ffmpeg
	@sh scripts/build-ffmpeg.sh $(STAMPS)/ffmpeg

# Every way the app says work is in hand, on one page, in a browser. It is
# preview material and never goes into the app.
motion: frontend/node_modules/.package-lock.json
	@cd frontend && npx vite --config preview/motion.config.ts

# Everything, in the order that puts the quickest answer first. The three
# stand alone as well, because they do not need each other and CI runs them
# on three machines at once: waiting for the fuzzing to finish before the
# interface is type checked is waiting for nothing.
test: unit fuzz interface

# The tests run with the race detector, because the app is a queue of jobs
# on their own goroutines and a window asking them things from another, and
# a race there is a bug that only shows up on someone else's machine.
#
# This also runs the seed corpus of every fuzz target, so a machine that
# only runs make unit still covers every case anyone has found so far. What
# it does not do is look for new ones.
unit: toolchain modules
	@$(GO) test -race -ldflags '$(LDFLAGS)' ./...

# The fuzzing runs without the race detector: it is the same code, many more
# times over.
#
# Every fuzz target does FUZZTIME executions, one worker each, as many
# targets at a time as the machine has cores. A new crasher is written to
# testdata/fuzz/ next to the code, where it stays as a seed.
fuzz: toolchain modules
	@GO='$(GO)' FUZZTIME='$(FUZZTIME)' sh scripts/fuzz.sh

# The interface needs Node and nothing else, no Go and no system libraries,
# which is why it is worth having on its own: it answers in well under a
# minute while the Go work is still going.
interface:
	@if command -v $(NPM) >/dev/null 2>&1; then \
		$(MAKE) -s --no-print-directory frontend/node_modules/.package-lock.json && \
		out=$$(cd frontend && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }; \
		printf 'ok  \tinterface types\n'; \
		out=$$(cd frontend && $(NPM) run --silent test 2>&1) || { echo "$$out"; exit 1; }; \
		printf 'ok  \tinterface rules\n'; \
	fi

check:
	@sh scripts/check.sh || true

tools:
	@sh scripts/tools.sh $(STAMPS)/ffmpeg

models:
	@sh scripts/models.sh

clean:
	@rm -rf $(BIN) $(STAMPS) frontend/node_modules frontend/preview/dist frontend/preview/dist-motion
	@$(GO) clean -fuzzcache
	@echo "Removed bin/, .build/, frontend/node_modules/, the preview builds and the fuzz corpus"
