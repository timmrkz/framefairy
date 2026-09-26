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

# The interface on its own is the interface, and no Go at all.
is "frontend/src/App.svelte" "interface"
is "frontend/preview/wails-stub.ts" "interface"

# A package reaches every package that imports it, and only those.
has "updates/updates.go" "go framefairy/updates" "go framefairy/cmd/framefairy-app" \
	"go framefairy/cmd/framefairy-release"
lacks "updates/updates.go" "go framefairy/engine" "build" "interface"

# The engine is under both front ends and the training tool.
has "engine/edit.go" "go framefairy/engine" "go framefairy/cmd/framefairy" \
	"go framefairy/cmd/framefairy-app" "go framefairy/cmd/framefairy-train"

# A package at the top of the tree reaches only itself.
is "cmd/framefairy-release/main.go" "go framefairy/cmd/framefairy-release"

# What a package keeps beside its code belongs to it: a seed in testdata,
# a file it embeds.
has "engine/testdata/fuzz/FuzzValidatePlan/seed" "go framefairy/engine"
is "cmd/framefairy-app/update-key.txt" "go framefairy/cmd/framefairy-app"

# The modules are every package.
has "go.sum" "go framefairy/engine" "go framefairy/updates" "go framefairy/train"

# How the project is built is the build, and not the Go tests.
is "Makefile" "build"
has "scripts/build-ffmpeg.sh" "build" "script scripts/build-ffmpeg.sh"
lacks "scripts/build-ffmpeg.sh" "go framefairy/engine"

# The rules test their own rules.
has "scripts/ci-needs.sh" "rules"
has ".github/workflows/ci.yml" "rules" "workflow .github/workflows/ci.yml"
has "scripts/changed.sh" "changed-rules"
is ".github/workflows/builds.yml" "workflow .github/workflows/builds.yml"

# A file nobody thought of builds, rather than being passed over.
is "build/icon.png" "build"

# A mix runs everything it touches.
has "frontend/src/App.svelte updates/source.go docs/UPDATES.md" "interface" "go framefairy/updates"

exit "$fail"
