#!/bin/sh
# Checks the rules in changed.sh, because a mistake in them is silent: the
# check before a push goes green having run less than the change needed,
# and CI finds it a round trip later. Every case below is one a change
# really arrives as.
#
#   sh scripts/changed-test.sh
#
# It hands changed.sh the files with CHANGED= and asks for the plan, so
# nothing is run and no commit is needed to try a rule.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fail=0

# make hands the variables it was called with to what it runs, so make
# lint PART=lint in CI would plan every case below for that part alone.
# Each case says what it needs itself.
unset PART SHARD EVERYTHING BASE UNITSKIP

# plan FILES: what changed.sh would run for these files, one line each,
# on one line.
plan() {
	CHANGED=$1 sh "$here/changed.sh" --plan 2>/dev/null | tr '\n' ' ' | sed 's/ $//'
}

# has FILES WANT...: the plan holds every line wanted.
has() {
	files=$1
	shift
	got=$(plan "$files")
	for want in "$@"; do
		case " $got " in
		*" $want "*) ;;
		*)
			printf 'FAIL\t%-44s wanted %s, plan is: %s\n' "$files" "$want" "$got"
			fail=1
			return
			;;
		esac
	done
	printf 'ok  \t%-44s %s\n' "$files" "$*"
}

# lacks FILES NOT...: the plan holds none of these.
lacks() {
	files=$1
	shift
	got=$(plan "$files")
	for not in "$@"; do
		case " $got " in
		*" $not "* | *" $not")
			printf 'FAIL\t%-44s did not want %s, plan is: %s\n' "$files" "$not" "$got"
			fail=1
			return
			;;
		esac
	done
	printf 'ok  \t%-44s not %s\n' "$files" "$*"
}

# is FILES WANT: the plan is exactly this.
is() {
	got=$(plan "$1")
	if [ "$got" = "$2" ]; then
		printf 'ok  \t%-44s = %s\n' "$1" "${2:-nothing}"
	else
		printf 'FAIL\t%-44s wanted "%s", plan is "%s"\n' "$1" "$2" "$got"
		fail=1
	fi
}

# mac FILES WANT: the parts a Mac in CI runs for these files are exactly
# these.
mac() {
	got=$(CHANGED=$1 OS=Darwin CI=1 UNITSKIP="framefairy/licence/dispenser framefairy/licence/paddle framefairy/cmd/framefairy-dispenser framefairy/internal/workflows" sh "$here/changed.sh" --parts 2>/dev/null)
	if [ "$got" = "$2" ]; then
		printf 'ok  \t%-44s Mac = %s\n' "$1" "${2:-nothing}"
	else
		printf 'FAIL\t%-44s wanted on a Mac "%s", got "%s"\n' "$1" "$2" "$got"
		fail=1
	fi
}

# Docs change nothing that runs.
is "README.md docs/APP.md CLAUDE.md .claude/skills/interface/SKILL.md" ""

# The interface on its own is the interface, the walks and the build, and
# no Go at all. The preview's stand-in is not under the walks, which run
# against the bridge, but the bridge is. Neither ships, so neither builds,
# and a walk alone starts no Mac (#521). Whatever reaches the walks walks
# every run, and the times they took reach none.
is "frontend/src/App.svelte" "interface walks build"
is "frontend/preview/wails-stub.ts" "interface"
is "frontend/preview/walks/words.mjs" "interface walks"
is "frontend/preview/walks/walk-times.tsv" "interface"
is "frontend/preview/wails-bridge.ts" "interface walks"
is "frontend/preview/settled.ts" "interface walks"
is "frontend/preview/standing.ts" "interface walks"
mac "frontend/preview/walks/words.mjs" "interface walks"
mac "frontend/src/App.svelte" "interface walks build outside"

# Except the files of the interface a Go test reads, which are that test's
# package too. A file a Go test only names in a comment is not one of them.
has "frontend/src/lib/api.ts" "interface" "unit" "go framefairy/cmd/framefairy-app" "walks"
has "frontend/src/lib/suggest.cases.json" "interface" "go framefairy/engine"
is "frontend/src/lib/flow.ts" "interface walks build"

# A package reaches every package that imports it, and only those.
has "updates/updates.go" "go framefairy/updates" "go framefairy/cmd/framefairy-app" \
	"go framefairy/cmd/framefairy-release" "build"
lacks "updates/updates.go" "go framefairy/engine" "interface"
# A package only the release tool reaches is not under the walks.
lacks "cmd/framefairy-release/main.go" "walks"

# The engine is under both front ends and the training tool, and so
# under the walks, which drive the app.
has "engine/edit.go" "go framefairy/engine" "go framefairy/cmd/framefairy" \
	"go framefairy/cmd/framefairy-app" "go framefairy/cmd/framefairy-train" "walks"

# A package at the top of the tree reaches only itself.
is "cmd/framefairy-release/main.go" "lint unit go framefairy/cmd/framefairy-release build"

# What a package keeps beside its code belongs to it: a seed in testdata,
# a file it embeds. A seed is no part of a program, so it builds nothing.
is "engine/testdata/fuzz/FuzzValidatePlan/seed" "$(CHANGED=engine/edit.go sh "$here/changed.sh" --plan 2>/dev/null | grep -vx 'lint' | grep -vx build | tr '\n' ' ' | sed 's/ $//')"
has "cmd/framefairy-app/update-key.txt" "unit" "go framefairy/cmd/framefairy-app" "walks" "build"
lacks "cmd/framefairy-app/update-key.txt" "lint"

# A changed test reaches the check that tests stay off the disk, which
# reads every test file and imports none of them. Code alone does not.
has "train/export_test.go" "go framefairy/train" "go framefairy/internal/offdisk"
lacks "train/export.go" "go framefairy/internal/offdisk"
# A test is no part of a program, so it builds nothing and opens no app.
lacks "train/export_test.go" "build" "outside"

# Go code and tests are linted, and the linters' settings and their proof
# run the lint.
has "train/export_test.go" "lint"
has "train/export.go" "lint"
is ".golangci.yml" "lint"
is "scripts/testdata/lint/walks/waits.mjs" "lint"
is "frontend/eslint-suppressions.json" "lint interface"

# Tests that run the decoder as a program do not import it, so what
# builds it reaches them by FRAMEFAIRY_FRAMES.
has "cmd/framefairy-frames/main.go" "go framefairy/cmd/framefairy-frames" "go framefairy/engine" "go framefairy/cmd/framefairy-app"
has "scripts/build-ffmpeg.sh" "go framefairy/engine" "go framefairy/cmd/framefairy-app"
lacks "cmd/framefairy-frames/ink_test.go" "go framefairy/engine"
# A changed test runs ffmpegtest's own test, not its importers.
lacks "updates/cache_test.go" "go framefairy/engine"

# internal/ffmpegtest reads every test for skips, as offdisk does.
has "engine/rotate_test.go" "go framefairy/internal/ffmpegtest"
lacks "engine/rotate.go" "go framefairy/internal/ffmpegtest"

# The notices test counts the hyphenation files by a path it builds.
has "engine/hyphenation/hyph-de.head.txt" "go framefairy/engine" "go framefairy/notices"

# The fuzzing script decides how the Go tests run, like the Makefile.
has "scripts/fuzz.sh" "lint" "go framefairy/engine" "go framefairy/updates" "fuzz FuzzParse"
# So does the CI image, which every Linux job runs in, the walks among
# them. No Mac runs in it.
has ".github/ci-image/Dockerfile" "build" "go framefairy/engine" "go framefairy/updates" "walks"
has "scripts/ci-packages.sh" "build" "go framefairy/engine" "walks"
mac ".github/ci-image/Dockerfile" ""

# The fuzz targets are those of the packages a change reaches, all of them.
fuzz=$(plan updates/source.go | tr ' ' '\n' | grep '^Fuzz' | tr '\n' ' ' | sed 's/ $//')
if [ "$fuzz" = "FuzzKeyFromLink FuzzKnownStaysInTheLibrary FuzzParse" ]; then
	printf 'ok  \t%-44s fuzz %s\n' "updates/source.go" "$fuzz"
else
	printf 'FAIL\t%-44s wanted the targets of updates and the app, got: %s\n' "updates/source.go" "$fuzz"
	fail=1
fi
# A package with no fuzz target fuzzes nothing, rather than every target.
lacks "cmd/framefairy-release/main.go" "fuzz"

# The modules are every package.
has "go.sum" "go framefairy/engine" "go framefairy/updates" "go framefairy/train"

# The Makefile and the CI workflow decide how every part runs, so they
# are every part and every package.
has "Makefile" "lint" "interface" "unit" "fuzz" "walks" "build" "go framefairy/engine" "go framefairy/updates"
has ".github/workflows/ci.yml" "lint" "interface" "walks" "build" "go framefairy/engine" "go framefairy/updates"
# Another workflow is read by the workflows' own test and the lint, which
# holds the pins and the rules for builds and the Main moved notice, and
# no Mac reads it.
is ".github/workflows/builds.yml" "lint unit go framefairy/internal/workflows"
is ".github/workflows/main-moved.yml" "lint unit go framefairy/internal/workflows"
mac ".github/workflows/builds.yml" "lint"
has "scripts/build-ffmpeg.sh" "build" "lint"
# Except for the Go test that reads it: the notices test checks the
# versions the build scripts pin.
has "scripts/build-ffmpeg.sh" "go framefairy/notices"
has "scripts/build-llama.sh" "go framefairy/notices"

# The rules are tested in make lint.
is "scripts/changed.sh" "lint"
is "scripts/needs-build.sh" "lint"
is "scripts/main-moved.sh" "lint"
is "scripts/plan-frozen.sh" "lint"

# The plan is history, so a change to it runs the check that says so,
# while every other doc runs nothing.
is "docs/GUI-PLAN.md" "lint"

# A file nobody thought of builds, rather than being passed over.
is "build/icon.png" "build"
mac "build/icon.png" "build outside"

# A mix runs everything it touches.
has "frontend/src/App.svelte updates/source.go docs/UPDATES.md" "interface" "go framefairy/updates"

# A part asked for alone is that part and nothing else, and the
# interface alone needs no Go.
got=$(CHANGED="frontend/src/App.svelte updates/source.go" PART=interface GO=false sh "$here/changed.sh" --parts 2>/dev/null) || got="failed"
if [ "$got" = interface ]; then
	printf 'ok  \t%-44s %s\n' "PART=interface, with no Go" "interface"
else
	printf 'FAIL\t%-44s wanted interface, got: %s\n' "PART=interface, with no Go" "$got"
	fail=1
fi
# On a push to main every part runs whole.
got=$(EVERYTHING=1 sh "$here/changed.sh" --parts 2>/dev/null)
if [ "$got" = "lint interface unit fuzz walks build" ]; then
	printf 'ok  \t%-44s %s\n' "EVERYTHING=1" "$got"
else
	printf 'FAIL\t%-44s wanted every part, got: %s\n' "EVERYTHING=1" "$got"
	fail=1
fi

# Two cases that need a real checkout, made in a worktree of its own
# with this changed.sh in it. A file moved out of a package is seen at
# the place it left too, since git reports a move by its new place only.
# And a package reached through its external tests, package x_test, is
# named x, the package go test knows.
root=$(CDPATH= cd -- "$here/.." && pwd)
wt=$(mktemp -d)
if git -C "$root" worktree add -q --detach "$wt" HEAD 2>/dev/null; then
	cp "$here/changed.sh" "$wt/scripts/changed.sh"
	git -C "$wt" update-index --skip-worktree scripts/changed.sh
	mkdir "$wt/updates/sub"
	git -C "$wt" mv updates/cache_test.go updates/sub/cache_test.go
	printf 'package train_test\n\nimport _ "framefairy/licence"\n' >"$wt/train/outside_test.go"
	echo "// changed" >>"$wt/licence/licence.go"
	# MAKE=true: changed.sh asks make for the modules first, and a plan
	# needs none of it.
	got=$(cd "$wt" && MAKE=true BASE=HEAD sh scripts/changed.sh --plan 2>/dev/null | tr '\n' ' ')
	git -C "$root" worktree remove --force "$wt"
	case " $got " in
	*" go framefairy/train_test "*)
		printf 'FAIL\t%-44s did not want go framefairy/train_test, plan is: %s\n' "an external test package" "$got"
		fail=1
		;;
	*" go framefairy/train "*) printf 'ok  \t%-44s %s\n' "an external test package" "go framefairy/train" ;;
	*)
		printf 'FAIL\t%-44s wanted go framefairy/train, plan is: %s\n' "an external test package" "$got"
		fail=1
		;;
	esac
	case " $got " in
	*" go framefairy/updates "*) printf 'ok  \t%-44s %s\n' "a move out of updates/" "go framefairy/updates" ;;
	*)
		printf 'FAIL\t%-44s wanted go framefairy/updates, plan is: %s\n' "a move out of updates/" "$got"
		fail=1
		;;
	esac
else
	printf 'FAIL\tno worktree for the cases that need one\n'
	fail=1
fi

exit "$fail"
