# One command does everything:
#
#   make            install what is missing, build all three programs
#   make run        the same, then start the app
#
# That is the whole of it for everyday work. make installs the tools this
# machine lacks and builds the two programs framefairy ships beside
# itself, ffmpeg and llama-server, once, and after that it is an ordinary
# build. The models are not make's business: the app fetches the speech
# model and the language model itself, on first run, which is what a
# customer does.
#
# The rest, for when you want one part of it:
#
#   make motion     the five ways the app shows work in hand, in a browser
#   make app        the macOS bundle, Frame Fairy.app, which make run starts
#   make install    the same, into /Applications, where it updates itself
#   make update-key make the key updates are signed with, once, see
#                   docs/UPDATES.md
#   make dispenser  the licence dispenser on this machine, with a pretend
#                   Paddle, at http://127.0.0.1:8090/dev
#   make icon       the .icns, from build/icon.png. make app does it for you
#   make ffmpeg     build the ffmpeg we ship again, from scratch
#   make llama      build the llama-server we ship again, from scratch
#   make frames     build the episode's decoder, on the ffmpeg make ffmpeg
#                   builds, where it or its code changed
#   make tools-archive
#                   pack both of them, with a manifest, for a release
#   make notices    write the licence notices again, from what the programs
#                   are built from
#   make hyphenation
#                   write the caption hyphenation patterns again, from
#                   hyph-utf8 and the German word list. Then make notices
#   make changed    the tests of what this branch changed against main,
#                   and only those. PART=unit runs one part of it, and
#                   SHARD=1/3 a share of that part, which is what every
#                   CI job on a pull request runs
#   make test       all tests: lint, interface, unit, fuzz and walks. CI
#                   runs the same on main, each part on a machine of its own
#   make lint       gofmt, go vet, that no Go test waits on the clock, the
#                   scripts read by their shell, the action pins and the
#                   tests of the repository's own rules
#   make unit       the Go tests, under the race detector except on a Mac.
#                   UNITSHARD=1/3 runs a third of them, as CI does
#   make fuzz       the fuzz targets, FUZZTIME executions each
#   make interface  the interface type check, its own tests, and that no
#                   walk or interface test waits on the clock
#   make walks      the interface against the real Go side, in Chromium:
#                   the sequences, and every walk WALKS times, each of
#                   its own length unless STEPS says, WALKERS at once,
#                   stopped after WALKTIMEOUT, 30m unless set, with
#                   a trace of each that failed in WALKTRACES, with
#                   a film of the screen for those walks/filmed.mjs
#                   names, or TRACE=film for all, TRACE=none for none
#   make check      what this machine still needs to run framefairy
#   make tools      install the missing tools and nothing else
#   make models     download the models for the command line, which has no
#                   window to ask in
#   make speechbench AUDIO=episode.mp4
#                   how fast the speech model hears on this machine
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
# Only these fuzz targets, by name and separated by spaces, or all of them.
FUZZTARGETS ?=
# One share of the targets, like 2/3, so a deep run can be spread over
# machines. And SIMTIME, the histories the licence simulation plays, 0 to
# leave it out, see scripts/fuzz.sh.
FUZZSHARD ?=
SIMTIME ?=
BIN := bin
STAMPS := .build
# The tests take ffmpeg and llama-server from the PATH, and on a Mac make
# puts the ones it built first, the ones the programs ship with and take no
# other. Without it a Mac tested Homebrew's ffmpeg, which is not the
# version the app ships and not built the same way. Only on a Mac, because
# that is the one system ours has an H.264 encoder on: on Linux it has
# none yet, and every test that renders failed with ours first. There the
# PATH keeps the system's. The walks keep it everywhere: they make their
# episode in VP9, which Chromium plays and ours cannot write.
TOOLS_FIRST := $(PATH)
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
TOOLS_FIRST := $(CURDIR)/$(BIN):$(PATH)
export MACOSX_DEPLOYMENT_TARGET := $(MACOS_MIN)
export CGO_CFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN)
export CGO_CXXFLAGS := -O2 -g -mmacosx-version-min=$(MACOS_MIN)
LDFLAGS := -extldflags=-mmacosx-version-min=$(MACOS_MIN)
# The episode's decoder links ffmpeg's static libraries, and pkg-config
# names some of them more than once, each library listing what it needs.
# Apple's linker uses each once and warns about the rest, and the build
# must print no warning.
FRAMES_LDFLAGS := -extldflags '-mmacosx-version-min=$(MACOS_MIN) -Wl,-no_warn_duplicate_libraries'
endif
FRAMES_LDFLAGS ?= $(LDFLAGS)

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
LICENCES_BUILT := cmd/framefairy-app/dist/licences.json

# What the build workflow says about a build of the app: its version, the
# channel it came from and the commit. A build made here leaves them empty,
# and follows no channel until one is picked in the app. See
# docs/UPDATES.md.
BUILD_VERSION ?=
BUILD_CHANNEL ?=
BUILD_COMMIT ?=
APP_LDFLAGS := $(LDFLAGS) -X main.buildVersion=$(BUILD_VERSION) -X main.buildChannel=$(BUILD_CHANNEL) -X main.buildCommit=$(BUILD_COMMIT)

PROGRAMS := $(BIN)/framefairy$(EXE) $(BIN)/framefairy-app$(EXE) $(BIN)/framefairy-train$(EXE)

.PHONY: frames all run app install update-key dispenser icon motion ffmpeg llama tools-archive notices hyphenation deps tools-beside changed test unit fuzz interface walks lint golangci-lint outside outside-allowed check tools models speechbench clean help toolchain modules $(PROGRAMS)

all: deps toolchain frames $(PROGRAMS) tools-beside
	@echo "Ready: $(PROGRAMS)"
	@sh scripts/check.sh --quiet

# What this machine is missing, installed. Before the toolchain check,
# because the toolchain is one of the things it installs, and it costs
# nothing when there is nothing to do: every check inside it is a command -v
# or a file test.
deps:
ifeq ($(INSTALL),1)
	@sh scripts/tools.sh $(STAMPS)/ffmpeg $(STAMPS)/llama
endif

# Our own ffmpeg and llama-server go beside the programs, which is the only
# place the programs take them from, and only once they are checked to be
# the files whose sums were built into them, see scripts/tool-sums.sh.
#
# A tool that is not there stops make. It used to be left to the search
# path, so a build that had failed looked like one that worked, with
# Homebrew's ffmpeg in it. NOTOOLS=1 builds the programs without them, for
# a runner that only proves the programs link, and they then run no tool.
#
# Their licence texts come with them, because scripts/bundle-macos.sh
# carries whatever LICENSE-* it finds here into the app.
NOTOOLS ?=
tools-beside:
ifneq ($(NOTOOLS),1)
	@for tool in $(STAMPS)/ffmpeg/bin/ffmpeg $(STAMPS)/ffmpeg/bin/ffprobe $(STAMPS)/llama/bin/llama-server $(FRAMES); do \
		if [ ! -x $$tool ]; then \
			case $$tool in \
			*llama-server) how="make llama" ;; \
			*framefairy-frames*) how="make frames, on the ffmpeg make ffmpeg builds" ;; \
			*) how="make ffmpeg" ;; \
			esac; \
			echo "$$tool is missing, and the programs run no other. Build it with $$how."; \
			exit 1; \
		fi; \
	done
	@cp $(STAMPS)/ffmpeg/bin/ffmpeg $(STAMPS)/ffmpeg/bin/ffprobe $(STAMPS)/llama/bin/llama-server $(FRAMES) $(BIN)/
	@echo "Using our own ffmpeg, llama-server and episode's decoder"
endif
	@for licence in $(STAMPS)/ffmpeg/bin/LICENSE-ffmpeg.txt $(STAMPS)/llama/bin/LICENSE-llama.cpp; do \
		if [ -f $$licence ]; then cp $$licence $(BIN)/; fi; \
	done

# The comment at the top of this file, however long it grows.
help:
	@awk '!/^#/ { exit } { sub(/^# ?/, ""); print }' Makefile

# Go and a C compiler, checked before anything is built.
toolchain:
	@sh scripts/check.sh --toolchain

# The project's own Go packages. Checked by content on every run, because
# copied files keep old timestamps and would fool a time-based check.
# go.sum is rewritten to match go.mod, and the downloads are only shown when
# something fails. A build runner changes no file of the checkout, so in
# CI an untidy go.mod or go.sum fails instead, where a machine of one's
# own would tidy it quietly and it would be committed that way.
modules:
	@mkdir -p $(STAMPS)
ifeq ($(TIDY),1)
	@sum=$$(cat go.mod go.sum 2>/dev/null | cksum); \
	if [ "$$sum" != "$$(cat $(STAMPS)/modules 2>/dev/null)" ]; then \
		if [ -n '$(CI)' ]; then \
			out=$$($(GO) mod tidy -diff 2>&1) || { echo "go.mod and go.sum are not tidy, run go mod tidy:"; echo "$$out"; exit 1; }; \
		else \
			echo "Resolving Go modules"; \
			out=$$($(GO) mod tidy 2>&1) || { echo "$$out"; exit 1; }; \
		fi; \
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
# The SHA-256 of each tool tools-beside puts beside the programs is built
# into every program that runs one, see scripts/tool-sums.sh. It is worked
# out as the program is built, not when this file is read, because make
# builds the tools after reading it.
$(BIN)/framefairy$(EXE): modules
	@echo "Building $@"
	@sums=$$(sh scripts/tool-sums.sh $(STAMPS)) && \
		$(GO) build -trimpath -ldflags "$(LDFLAGS) -X framefairy/engine.toolSums=$$sums" -o $@ ./cmd/framefairy
	@sh scripts/carry-libs.sh $@ $(BIN)/lib

# The licence of every Go module the programs compile on this system and
# of every package in the interface bundle, written by the build from what
# it really has and built into the app beside notices/. It is never in the
# repository, so a new version needs nothing written by hand. It stops the
# build on a licence that is not on the list in notices/licence.
$(LICENCES_BUILT): go.mod go.sum $(UI_BUILT)
	@echo "Writing the licences of the Go modules and the interface's packages"
	@$(GO) run ./notices/gen -built $@

$(BIN)/framefairy-app$(EXE): modules $(UI_BUILT) $(LICENCES_BUILT) frames
	@echo "Building $@"
	@sums=$$(sh scripts/tool-sums.sh $(STAMPS)) && \
		$(GO) build -trimpath -tags production -ldflags "$(APP_LDFLAGS) -X framefairy/engine.toolSums=$$sums" -o $@ ./cmd/framefairy-app
	@sh scripts/carry-libs.sh $@ $(BIN)/lib

$(BIN)/framefairy-train$(EXE): modules
	@echo "Building $@"
	@$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $@ ./cmd/framefairy-train

# The bundle a customer gets, made out of what is already in bin/. It is
# not only for shipping: make run starts it, so the app run every day has
# the same Info.plist, the same privacy prompts and the same tools inside
# the same folder as the one a customer opens. See docs/PACKAGING.md.
#
# Only on macOS. A .app on another system would be a folder nothing reads.
app: all
ifeq ($(UNAME),Darwin)
	@sh scripts/bundle-macos.sh $(BIN) $(BIN)
endif

# The app where apps live, which is where it updates itself: from then on
# it is started like any other app and follows the channel picked in its
# settings. Built here, it carries no quarantine mark, so macOS opens it
# without asking. See docs/UPDATES.md.
install: app
ifeq ($(UNAME),Darwin)
	@rm -rf "/Applications/Frame Fairy.app"
	@ditto "$(BIN)/Frame Fairy.app" "/Applications/Frame Fairy.app"
	@echo "Installed /Applications/Frame Fairy.app"
else
	@echo "make install puts the Mac app in /Applications, and this is not a Mac."
endif

# The update key, made once, on Tim's Mac. The public half goes into
# cmd/framefairy-app/update-key.txt and the private half to the clipboard,
# never to a file. See docs/UPDATES.md.
update-key: toolchain modules
	@$(GO) run ./cmd/framefairy-release key

# The licence dispenser on this machine, with a pretend Paddle, mail
# service and signer around it, to try every sale and every failure by
# hand. Its dev page is at http://127.0.0.1:8090/dev. It keeps nothing, so
# every start is an empty shop. See docs/LICENCE.md.
dispenser: toolchain modules
	@$(GO) run ./cmd/framefairy-dispenser dev

# The icon on its own, for when build/icon.png changed and you want to see
# it without building everything. make app does this by itself.
icon:
	@sh scripts/make-icon.sh

# Started from inside the bundle rather than with open, so the log stays in
# this terminal. macOS reads the Info.plist either way, because it finds the
# bundle by walking up from the program, so the prompts are the real ones.
run: app
ifeq ($(UNAME),Darwin)
	@"$(BIN)/Frame Fairy.app/Contents/MacOS/framefairy-app"
else
	@$(BIN)/framefairy-app$(EXE)
endif

# The episode's decoder, see cmd/framefairy-frames and docs/VIDEO-PREVIEW.md:
# a program of its own on the libraries of the ffmpeg below, built static
# against them like ffmpeg itself, with the tag that builds it for real.
# It is a tool beside the app like ffmpeg, summed and checked the same way.
# Built when it is not there, or its code, the modules or the ffmpeg under
# it changed since, which make decides by file times alone. Without the
# ffmpeg make builds it cannot be built, and says so.
FRAMES := $(STAMPS)/frames/framefairy-frames$(EXE)
FRAMES_SRC := $(wildcard cmd/framefairy-frames/*.go internal/framewire/*.go) go.mod go.sum
frames:
ifneq ($(NOTOOLS),1)
frames: $(FRAMES)
endif
$(FRAMES): $(FRAMES_SRC) $(wildcard $(STAMPS)/ffmpeg/built) | modules
	@if [ ! -f $(STAMPS)/ffmpeg/lib/pkgconfig/libavcodec.pc ]; then \
		echo "framefairy-frames is built on the libraries of the ffmpeg make builds, and it is not there. Build it with make ffmpeg."; \
		exit 1; \
	fi
	@echo "Building the episode's decoder, framefairy-frames"
	@mkdir -p $(dir $(FRAMES))
	@PKG_CONFIG="$(CURDIR)/scripts/pkg-config-static.sh" \
		PKG_CONFIG_PATH="$(CURDIR)/$(STAMPS)/ffmpeg/lib/pkgconfig:$(CURDIR)/$(STAMPS)/ffmpeg/deps/lib/pkgconfig" \
		CGO_ENABLED=1 CGO_CFLAGS="$${CGO_CFLAGS:--O2} -Wno-unused-result" $(GO) build -trimpath -tags ffmpeglibs -ldflags "$(FRAMES_LDFLAGS)" -o $(FRAMES) ./cmd/framefairy-frames

# In a cloud session the decoder is built in the background as the session
# starts, see scripts/cloud-decoder.sh, and the targets that run it wait
# for that build here, before they look for it. Without the wait, a test
# run in the first minutes skips the decoder's tests and still says ok.
ifeq ($(CLAUDE_CODE_REMOTE),true)
ifneq ($(filter changed unit walks test,$(MAKECMDGOALS)),)
$(shell bash scripts/cloud-decoder.sh wait >&2)
endif
endif

# The tests run the decoder that is built, so one older than its code is
# built again before them, or new tests meet an old decoder and fail for
# no reason in the change. Only one that is there and can be built again:
# without it the tests that need it skip, as they did.
FRESH_FRAMES := $(if $(and $(wildcard $(FRAMES)),$(wildcard $(STAMPS)/ffmpeg/lib/pkgconfig/libavcodec.pc)),frames)

# The ffmpeg we ship, built from source without libx264 so the build is
# LGPL. make builds it when it is not there or scripts/build-ffmpeg.sh has
# changed since, see scripts/tools.sh. This builds it again whatever is
# there, for when the last one went wrong.
ffmpeg:
	@rm -rf $(STAMPS)/ffmpeg
	@sh scripts/build-ffmpeg.sh $(STAMPS)/ffmpeg
	@cksum scripts/build-ffmpeg.sh | cut -d' ' -f1 >$(STAMPS)/ffmpeg/built

# The llama-server we ship, so choosing a local model is not an instruction
# to go and install something. Same rules as ffmpeg above: make builds it
# when it is not there or its script changed, this builds it again
# whatever is there.
llama:
	@rm -rf $(STAMPS)/llama
	@sh scripts/build-llama.sh $(STAMPS)/llama
	@cksum scripts/build-llama.sh | cut -d' ' -f1 >$(STAMPS)/llama/built

# Both of them in one archive, with a manifest that says what each one is
# and the sha256 to check a copy against. This is what a release picks up
# rather than building them itself, and the workflow that makes one by hand
# is .github/workflows/tools.yml.
tools-archive:
	@sh scripts/pack-tools.sh $(STAMPS)/ffmpeg $(STAMPS)/llama $(STAMPS)/dist

# The licence of what the programs bring with them whose version is pinned
# by hand, and the text each asks to travel with every copy, into notices/,
# which the app shows under Licences: Go, ffmpeg and llama-server with
# what is in them, the speech runtime, the fonts, the hyphenation patterns
# and the models. It reads the source trees ffmpeg and llama-server are
# built from, so it builds everything first. The tests say when it is due:
# a pinned version that changed without it. The Go modules and the
# interface's packages are not in it, see $(LICENCES_BUILT).
notices: all
	@$(GO) run ./notices/gen
	@printf 'ok  \tnotices\n'

# The patterns that break a word too long for a caption line, in
# engine/hyphenation, from the commits scripts/hyphenation.sh pins. It
# needs patgen, and is only run when a pin moves. The notices follow it.
hyphenation:
	@sh scripts/hyphenation.sh

# Every way the app says work is in hand, on one page, in a browser. It is
# preview material and never goes into the app.
motion: frontend/node_modules/.package-lock.json
	@cd frontend && npx vite --config preview/motion.config.ts

# Everything, the quickest answer first. Each part stands alone as well,
# because none needs another, and CI runs each on machines of its own on
# a push to main.
test: lint interface unit fuzz walks

# The same parts, for what this branch changed and only that, worked out
# file by file by scripts/changed.sh, which says what each kind of file
# reaches. Every CI job on a pull request runs this for its own part, so
# what CI runs for a change is what this runs here. PART names the parts,
# like PART=unit or PART="lint build", and SHARD one share of unit or
# walks. EVERYTHING=1 takes every part whole, which is a push to main.
#
# MODE=parts names the parts with work and runs nothing, which is how CI's
# first job knows which jobs to start, and UNAME=Darwin asks the same for
# a Mac. MODE=plan says what each part would run.
PART ?=
SHARD ?=
MODE ?=
changed:
	@PART='$(PART)' SHARD='$(SHARD)' OS='$(UNAME)' UNITSKIP='$(UNITSKIP)' GO='$(GO)' MAKE='$(MAKE)' sh scripts/changed.sh $(if $(MODE),--$(MODE))

# The tests run with the race detector, because the app is a queue of jobs
# on their own goroutines and a window asking them things from another, and
# a race there is a bug that only shows up on someone else's machine.
#
# This also runs the seed corpus of every fuzz target, so a machine that
# only runs make unit still covers every case anyone has found so far. What
# it does not do is look for new ones.
#
# On a Mac without the race detector: the Go code is the same as on
# Linux, where it runs with it, and it makes the tests 1.7 times slower.
# And without the licence server's packages, which run on a Linux server
# and never on a customer's Mac, nor the read of the workflows and of how
# the video endings are named, which are the same on every machine. RACE= and UNITSKIP= say otherwise.
#
# UNITSHARD=1/3 runs a third of the tests, by the first letters of their
# names, which is how CI spreads them over three machines, and 1/1 all of
# them, for a change that needs one machine. The patterns
# never change, because Go keys its test cache on them, and -skip takes
# exactly what the two -run leave. The engine's tests whose names start
# with A are a third of all the time, so they are split by their second
# letter. UNITFLAGS is one more flag for go test, passed as it is.
#
# UNITPKGS names the packages to test, every one unless it says.
#
# UNITTIMEOUT is how long one package's tests may take before go test
# ends them and prints every goroutine. A test waits on signals and never
# on the clock, so one whose signal never comes hangs until then. Go's own
# ten minutes here, and CI sets 4m, under its five-minute jobs.
ifeq ($(UNAME),Darwin)
RACE ?=
UNITSKIP ?= framefairy/licence/dispenser framefairy/licence/paddle framefairy/cmd/framefairy-dispenser framefairy/internal/workflows framefairy/internal/naming
else
RACE ?= -race
UNITSKIP ?=
endif
UNITSHARD ?=
ifeq ($(UNITSHARD),1/1)
UNITFLAGS ?=
else ifeq ($(UNITSHARD),1/3)
UNITFLAGS ?= -run=^Test(A[^S-Z]|[B-O])
else ifeq ($(UNITSHARD),2/3)
UNITFLAGS ?= -run=^Test(A[S-Z]|[P-S])
else ifeq ($(UNITSHARD),3/3)
UNITFLAGS ?= -skip=^Test(A[^S-Z]|[B-O])|^Test(A[S-Z]|[P-S])
else ifneq ($(UNITSHARD),)
$(error UNITSHARD is 1/1, 1/3, 2/3 or 3/3, not $(UNITSHARD))
endif
UNITFLAGS ?=
UNITPKGS ?= ./...
# COVERAGE names a folder, and make unit, make interface and make walks
# then count the lines their tests run and write them there, for the
# nightly snapshot in Codecov, see Coverage in docs/TESTING.md. Every Go
# package is counted for every test, -coverpkg, so a line of the engine a
# walk runs counts as the engine's. Empty, nothing is counted, which is
# every run but the nightly one. A share of the tests writes its own file,
# named by its share.
COVERAGE ?=
COVERDIR = $(abspath $(COVERAGE))
COVERPKG = -coverpkg=framefairy/...
UNITTIMEOUT ?= 10m
unit: toolchain modules $(FRESH_FRAMES)
	@pkgs='$(strip $(UNITPKGS))'; \
	if [ -n "$$pkgs" ] && [ -n '$(UNITSKIP)' ]; then \
		pkgs=$$($(GO) list $$pkgs) || exit 1; \
		pkgs=$$(printf '%s\n' "$$pkgs" | grep -vxF $(foreach p,$(UNITSKIP),-e $(p))); \
	fi; \
	if [ -z "$$pkgs" ]; then echo "no Go package to test"; exit 0; fi; \
	$(if $(COVERAGE),mkdir -p '$(COVERDIR)' && ) \
	PATH="$(TOOLS_FIRST)" FRAMEFAIRY_FRAMES="$(if $(wildcard $(FRAMES)),$(CURDIR)/$(FRAMES))" \
		$(GO) test $(RACE) -timeout $(UNITTIMEOUT) $(if $(UNITFLAGS),'$(UNITFLAGS)') \
		$(if $(COVERAGE),$(COVERPKG) -coverprofile='$(COVERDIR)/go-unit$(if $(UNITSHARD),-$(subst /,-of-,$(UNITSHARD))).txt') \
		-ldflags '$(LDFLAGS)' $$pkgs

# The fuzzing runs without the race detector: it is the same code, many more
# times over.
#
# Every fuzz target does FUZZTIME executions, one worker each, as many
# targets at a time as the machine has cores. A new crasher is written to
# testdata/fuzz/ next to the code, where it stays as a seed.
fuzz: toolchain modules
	@PATH="$(TOOLS_FIRST)" GO='$(GO)' FUZZTIME='$(FUZZTIME)' FUZZTARGETS='$(FUZZTARGETS)' FUZZSHARD='$(FUZZSHARD)' SIMTIME='$(SIMTIME)' sh scripts/fuzz.sh

# The interface in Chromium against the real Go side, see docs/TESTING.md:
# every sequence and every walk for seeds 1 to WALKS, half as many again at
# once as the machine has cores unless WALKERS says. It needs Node, ffmpeg and
# the Chromium Playwright drives, which CI installs and a cloud session
# already has. Built for the bridge first, which takes seconds.
WALKS ?= 3
STEPS ?=
WALKERS ?=
# One share of the runs, like 1/3, which is how CI spreads them over three
# machines, each share as long as the others by frontend/preview/walks/
# walk-times.tsv. WALKTIMES=1 writes what the runs took into that file.
WALKSHARD ?=
WALKTIMES ?=
# STILL=none walks every sequence with the app's motion, see moving in
# frontend/preview/walks/common/steps.mjs.
STILL ?=
# How long the runs may take before go test stops them. TestWalks stops
# what still runs a minute before and names it with what it printed. CI
# sets a few minutes over what a share takes, so a run that hangs is
# named in minutes and does not hold its machine for half an hour.
WALKTIMEOUT ?= 30m
# The videos the walks add, kept by what makes them, so each is made once
# and not on every add, issue #527. CI keeps the folder between runs.
WALKVIDEOS ?= $(CURDIR)/.build/walk-videos
# A Playwright trace of a walk or sequence that failed, threw or was
# stopped, one file for each page it had open, see Finding why a walk
# failed in docs/TESTING.md. CI keeps the folder with a run that failed.
# Only the walks frontend/preview/walks/filmed.mjs names have a film of
# the screen in it, since the film makes a walk slower. TRACE=film films
# every walk and sequence, and TRACE=none records no trace.
WALKTRACES ?= $(CURDIR)/.build/walk-traces
TRACE ?= light
# With COVERAGE, the bridge counts the Go lines the walks run and every
# page the interface lines, see COVERAGE above make unit. A run where
# either counted nothing fails, since every walk reaches both: no line of
# the job queue, jobs.go, which only a bridge runs, and no line of any
# component.
WALKCOVER = $(COVERDIR)/walks$(if $(WALKSHARD),-$(subst /,-of-,$(WALKSHARD)))
walks: toolchain modules frontend/node_modules/.package-lock.json $(FRESH_FRAMES)
	@cd frontend && $(if $(COVERAGE),WALKCOVERAGE='$(WALKCOVER)-pages') $(NPM) exec -- vite build --config preview/bridge.config.ts --logLevel error
	@$(if $(COVERAGE),rm -rf '$(WALKCOVER)-pages' && mkdir -p '$(COVERDIR)' && )FRAMEFAIRY_FRAMES="$(if $(wildcard $(FRAMES)),$(CURDIR)/$(FRAMES))" FRAMEFAIRY_WALKS=1 FRAMEFAIRY_WALK_VIDEOS='$(WALKVIDEOS)' WALKS='$(WALKS)' STEPS='$(STEPS)' WALKERS='$(WALKERS)' WALKSHARD='$(WALKSHARD)' WALKTIMES='$(WALKTIMES)' STILL='$(STILL)' WALKTRACES='$(WALKTRACES)' TRACE='$(TRACE)' $(if $(COVERAGE),WALKCOVERAGE='$(WALKCOVER)-pages') $(GO) test -count=1 -ldflags '$(LDFLAGS)' -timeout $(WALKTIMEOUT) $(if $(COVERAGE),$(COVERPKG) -coverprofile='$(WALKCOVER)-go.txt') -run '^TestWalks$$' ./cmd/framefairy-app
ifneq ($(COVERAGE),)
	@awk -F'[ ]' '/^framefairy\/cmd\/framefairy-app\/jobs\.go:/ && $$NF > 0 { found = 1 } END { exit !found }' '$(WALKCOVER)-go.txt' || { echo "$(WALKCOVER)-go.txt counts no line of the job queue, so the bridge did not count what the walks ran"; exit 1; }
	@printf 'ok  \tthe walks ran lines of the job queue\n'
	@cd frontend && node preview/coverage.mjs '$(WALKCOVER)-pages' '$(WALKCOVER)-interface.lcov'
endif

# The checks from outside the app: a link that opens it, closed or open,
# handed to macOS the way a browser does, with the local dispenser running
# beside it. Only on a Mac, and only in CI, because it replaces the
# licence in the keychain. The app is built again with the probe the test
# drives it through, into a folder of its own, so bin/ never has it. See
# From outside the app in docs/TESTING.md. Each check waits on what the
# app says, with no limit of its own, so a check whose signal never comes
# hangs until go test ends it after OUTSIDETIMEOUT, 2m in CI, and prints
# every goroutine.
OUTSIDE := $(STAMPS)/outside
OUTSIDETIMEOUT ?= 10m
outside: outside-allowed all
	@rm -rf $(OUTSIDE) && mkdir -p $(OUTSIDE)
	@cp -R $(BIN)/. $(OUTSIDE)/ && rm -rf "$(OUTSIDE)/Frame Fairy.app"
	@echo "Building $(OUTSIDE)/framefairy-app with the probe"
	@sums=$$(sh scripts/tool-sums.sh $(STAMPS)) && \
		$(GO) build -trimpath -tags 'production outside' -ldflags "$(APP_LDFLAGS) -X framefairy/engine.toolSums=$$sums" -o $(OUTSIDE)/framefairy-app ./cmd/framefairy-app
	@sh scripts/carry-libs.sh $(OUTSIDE)/framefairy-app $(OUTSIDE)/lib
	@sh scripts/bundle-macos.sh $(OUTSIDE) $(OUTSIDE)
	@$(GO) build -o $(OUTSIDE)/framefairy-dispenser ./cmd/framefairy-dispenser
	@FRAMEFAIRY_OUTSIDE_APP="$(CURDIR)/$(OUTSIDE)/Frame Fairy.app" FRAMEFAIRY_OUTSIDE_DISPENSER="$(CURDIR)/$(OUTSIDE)/framefairy-dispenser" \
		$(GO) test -tags outside -count=1 -v -timeout $(OUTSIDETIMEOUT) ./internal/outside

# Said before anything is built, so the refusal is the first thing read.
outside-allowed:
ifneq ($(UNAME),Darwin)
	@echo "make outside opens the app the way macOS does, and this is not a Mac."; exit 1
endif
ifeq ($(CI),)
	@echo "make outside runs in CI only: on a Mac of your own it would replace the licence in the keychain."; exit 1
endif

# The interface needs Node and nothing else, no Go and no system libraries,
# which is why it is worth having on its own: it answers in well under a
# minute while the Go work is still going.
interface:
	@if command -v $(NPM) >/dev/null 2>&1; then \
		$(MAKE) -s --no-print-directory frontend/node_modules/.package-lock.json && \
		out=$$(cd frontend && $(NPM) run --silent check 2>&1) || { echo "$$out"; exit 1; }; \
		printf 'ok  \tinterface types\n'; \
		out=$$(cd frontend && $(NPM) run --silent test $(if $(COVERAGE),-- --coverage.enabled --coverage.reportsDirectory='$(COVERDIR)/interface-unit') 2>&1) || { echo "$$out"; exit 1; }; \
		printf 'ok  \tinterface rules\n'; \
		out=$$(cd frontend && $(NPM) run --silent lint 2>&1) || { echo "$$out"; exit 1; }; \
		printf 'ok  \tinterface waits on no clock\n'; \
	fi

# Everything that reads the code without running it, over the whole tree,
# the same here and in CI. gofmt and go vet. That ./... is our packages
# and nothing the interface's packages carry, #471. That no Go test waits
# on the clock, see .golangci.yml, with the proof that the linters catch
# it first, so settings that let all through fail. The checks from
# outside the app build only on a Mac with the outside tag, so they are
# read a second time the way they build. Every script read by the shell
# its first line names. The action pins. And the tests of the
# repository's own rules: what a change reaches, the rule for a build,
# the Main moved notice, and that the plan stays history.
GOLANGCI_LINT_VERSION := 2.14.0
GOLANGCI_LINT := $(STAMPS)/golangci-lint-$(GOLANGCI_LINT_VERSION)/golangci-lint
lint: toolchain modules golangci-lint frontend/node_modules/.package-lock.json
	@out=$$(gofmt -l $$(git ls-files '*.go')); [ -z "$$out" ] || { echo "gofmt -w these:"; echo "$$out"; exit 1; }
	@printf 'ok  \tgofmt\n'
	@$(GO) vet ./...
	@printf 'ok  \tgo vet\n'
	@! $(GO) list ./... | grep /node_modules/ || { echo "go list ./... takes in the packages above, see the ignore in go.mod"; exit 1; }
	@sh scripts/lint-test.sh
	@out=$$($(GOLANGCI_LINT) run ./... 2>&1) || { echo "$$out"; exit 1; }
	@out=$$(GOOS=darwin $(GOLANGCI_LINT) run --build-tags outside ./internal/outside/ 2>&1) || { echo "$$out"; exit 1; }
	@printf 'ok  \tGo tests wait on no clock\n'
	@for s in $$(git ls-files '*.sh'); do \
		case $$(head -n 1 "$$s") in *bash*) bash -n "$$s" ;; *) sh -n "$$s" ;; esac || exit 1; \
	done
	@printf 'ok  \tscripts read\n'
	@sh scripts/pins.sh
	@sh scripts/changed-test.sh
	@sh scripts/needs-build-test.sh
	@sh scripts/main-moved-test.sh
	@sh scripts/plan-frozen-test.sh
	@sh scripts/plan-frozen.sh

# The release golangci-lint builds and checks against its own checksums,
# as its install.sh does. Not install.sh itself, which asks github.com's
# release page for the tag, a page a cloud session cannot reach.
golangci-lint: $(GOLANGCI_LINT)
$(GOLANGCI_LINT):
ifneq ($(INSTALL),1)
	@echo "golangci-lint $(GOLANGCI_LINT_VERSION) is missing, and make installs nothing here: make golangci-lint INSTALL=1"; exit 1
endif
	@echo "Fetching golangci-lint $(GOLANGCI_LINT_VERSION)"
	@name=golangci-lint-$(GOLANGCI_LINT_VERSION)-$$(uname -s | tr A-Z a-z)-$$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/'); \
		url=https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION); \
		tmp=$$(mktemp -d); \
		(cd $$tmp && curl -sSfLO $$url/$$name.tar.gz && curl -sSfLO $$url/golangci-lint-$(GOLANGCI_LINT_VERSION)-checksums.txt && \
			grep " $$name.tar.gz$$" golangci-lint-$(GOLANGCI_LINT_VERSION)-checksums.txt | shasum -a 256 -c - >/dev/null && \
			tar -xzf $$name.tar.gz) && mkdir -p $(@D) && mv $$tmp/$$name/golangci-lint $@; \
		status=$$?; rm -rf $$tmp; exit $$status

check:
	@sh scripts/check.sh || true

tools:
	@sh scripts/tools.sh $(STAMPS)/ffmpeg $(STAMPS)/llama

models: toolchain modules
	@$(GO) run ./scripts/models

# How fast the speech model hears on this machine, every way it can run,
# over three minutes of AUDIO, which can be an episode. It prints a table.
# ARGS passes more to scripts/speechbench, for example ARGS="-seconds 60".
speechbench: toolchain modules
	@if [ -z "$(AUDIO)" ]; then \
		echo "Say what to hear: make speechbench AUDIO=path/to/episode.mp4"; exit 1; \
	fi
	@$(GO) run -ldflags '$(LDFLAGS)' ./scripts/speechbench -audio "$(AUDIO)" $(ARGS)

clean:
	@rm -rf $(BIN) $(STAMPS) frontend/node_modules frontend/preview/dist frontend/preview/dist-motion frontend/preview/dist-bridge
	@$(GO) clean -fuzzcache
	@echo "Removed bin/, .build/, frontend/node_modules/, the preview builds and the fuzz corpus"
