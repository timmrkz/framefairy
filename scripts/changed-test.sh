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

# Docs change nothing that runs.
is "README.md docs/APP.md CLAUDE.md .claude/skills/interface/SKILL.md" ""

# The interface on its own is the interface and the walks, and no Go at
# all. The preview's stand-in is not under the walks, which run against
# the bridge, but the bridge is.
is "frontend/src/App.svelte" "interface walks"
is "frontend/preview/wails-stub.ts" "interface"
is "frontend/preview/walks/words.mjs" "interface walks"
is "frontend/preview/wails-bridge.ts" "interface walks"

# Except the files of the interface a Go test reads, which are that test's
# package too. A file a Go test only names in a comment is not one of them.
has "frontend/src/lib/api.ts" "interface" "go framefairy/cmd/framefairy-app" "walks"
has "frontend/src/lib/suggest.cases.json" "interface" "go framefairy/engine"
is "frontend/src/lib/flow.ts" "interface walks"

# A package reaches every package that imports it, and only those.
has "updates/updates.go" "go framefairy/updates" "go framefairy/cmd/framefairy-app" \
	"go framefairy/cmd/framefairy-release"
lacks "updates/updates.go" "go framefairy/engine" "build" "interface"
# A package only the release tool reaches is not under the walks.
lacks "cmd/framefairy-release/main.go" "walks"

# The engine is under both front ends and the training tool, and so
# under the walks, which drive the app.
has "engine/edit.go" "go framefairy/engine" "go framefairy/cmd/framefairy" \
	"go framefairy/cmd/framefairy-app" "go framefairy/cmd/framefairy-train" "walks"

# A package at the top of the tree reaches only itself.
is "cmd/framefairy-release/main.go" "go framefairy/cmd/framefairy-release"

# What a package keeps beside its code belongs to it: a seed in testdata,
# a file it embeds.
has "engine/testdata/fuzz/FuzzValidatePlan/seed" "go framefairy/engine"
is "cmd/framefairy-app/update-key.txt" "go framefairy/cmd/framefairy-app walks"

# A changed test reaches the check that tests stay off the disk, which
# reads every test file and imports none of them. Code alone does not.
has "train/export_test.go" "go framefairy/train" "go framefairy/internal/offdisk"
lacks "train/export.go" "go framefairy/internal/offdisk"

# A changed test is linted for waits on the clock, and code is not. The
# linters' settings and their proof run the proof.
has "train/export_test.go" "lint"
lacks "train/export.go" "lint"
is ".golangci.yml" "lint lint-rules"
is "scripts/testdata/lint/walks/waits.mjs" "lint lint-rules"
is "frontend/eslint-suppressions.json" "interface lint-rules"

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

# The scripts the Go jobs run decide how they run, like the Makefile.
has "scripts/fuzz.sh" "build" "go framefairy/engine" "go framefairy/updates"
has "scripts/ci-packages.sh" "build" "go framefairy/engine" "go framefairy/updates"

# The modules are every package.
has "go.sum" "go framefairy/engine" "go framefairy/updates" "go framefairy/train"

# How the project is built is the build. The Makefile and the CI
# workflow also decide how the Go tests run, so they are every package.
has "Makefile" "build" "go framefairy/engine" "go framefairy/updates"
has ".github/workflows/ci.yml" "go framefairy/engine" "go framefairy/updates"
lacks ".github/workflows/builds.yml" "go framefairy/engine"
has "scripts/build-ffmpeg.sh" "build" "script scripts/build-ffmpeg.sh"
# Except for the Go test that reads it: the notices test checks the
# versions the build scripts pin.
has "scripts/build-ffmpeg.sh" "go framefairy/notices"
has "scripts/build-llama.sh" "go framefairy/notices"

# The rules test their own rules.
has "scripts/ci-filters-test.sh" "rules"
is ".github/ci-filters.yml" "rules"
has ".github/workflows/ci.yml" "rules" "workflow .github/workflows/ci.yml"
has "scripts/changed.sh" "changed-rules"
is ".github/workflows/builds.yml" "build-rules workflow .github/workflows/builds.yml"
is ".github/workflows/publish.yml" "build-rules workflow .github/workflows/publish.yml"
is "scripts/needs-build.sh" "build-rules script scripts/needs-build.sh"
is "scripts/main-moved.sh" "moved-rules script scripts/main-moved.sh"
is ".github/workflows/main-moved.yml" "moved-rules workflow .github/workflows/main-moved.yml"
is "scripts/plan-frozen.sh" "plan-rules script scripts/plan-frozen.sh"

# The plan is history, so a change to it runs the check that says so,
# while every other doc runs nothing.
is "docs/GUI-PLAN.md" "plan-rules"

# A file nobody thought of builds, rather than being passed over.
is "build/icon.png" "build"

# A mix runs everything it touches.
has "frontend/src/App.svelte updates/source.go docs/UPDATES.md" "interface" "go framefairy/updates"

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
	# MAKE=true: changed.sh asks make for the toolchain first, which the
	# job build runs this before it installs, and a plan needs none of it.
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
