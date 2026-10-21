#!/bin/sh
# Checks .github/ci-filters.yml, which decides which jobs a pull request
# starts, because a mistake in it is silent: CI goes green having run less
# than it should. Every case below is one a pull request really arrives as.
#
#   sh scripts/ci-filters-test.sh
#
# It runs dorny/paths-filter itself, the commit ci.yml pins, with the
# quantifier ci.yml gives it, on the changes staged in a repository of its
# own. The action is fetched into .build/ the first time, so that run
# needs the network.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
ci=$root/.github/workflows/ci.yml
pin=$(sed -n 's/.*dorny\/paths-filter@\([0-9a-f]*\).*/\1/p' "$ci" | head -n 1)
quantifier=$(sed -n 's/.*predicate-quantifier: *//p' "$ci" | head -n 1)
action=$root/.build/paths-filter/$pin
if [ ! -f "$action/dist/index.js" ]; then
	rm -rf "$action" && mkdir -p "$action"
	git -C "$action" init -q
	git -C "$action" fetch -q --depth 1 https://github.com/dorny/paths-filter "$pin"
	git -C "$action" checkout -q FETCH_HEAD
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
work=$tmp/repo
git init -q "$work"
git -C "$work" -c user.name=t -c user.email=t@t commit -q --allow-empty -m base
fail=0

# check FILES WANT: changing FILES starts the filters in WANT and no other.
check() {
	git -C "$work" reset -q --hard
	for f in $1; do
		mkdir -p "$work/$(dirname "$f")"
		echo changed >"$work/$f"
	done
	git -C "$work" add -A
	: >"$tmp/out"
	env GITHUB_OUTPUT="$tmp/out" INPUT_FILTERS="$root/.github/ci-filters.yml" \
		INPUT_BASE=HEAD INPUT_TOKEN= INPUT_WORKING-DIRECTORY="$work" \
		"INPUT_PREDICATE-QUANTIFIER=$quantifier" INPUT_LIST-FILES=none \
		node "$action/dist/index.js" >"$tmp/log" || true
	got=$(sed -n '/^changes<</{n;p;}' "$tmp/out" | tr -d '[]"' | tr ',' ' ')
	if [ "$got" = "$2" ]; then
		printf 'ok  \t%-52s %s\n' "$1" "${2:-nothing}"
	else
		printf 'FAIL\t%-52s %s, wanted %s\n' "$1" "${got:-nothing}" "${2:-nothing}"
		cat "$tmp/log"
		fail=1
	fi
}

# Docs cannot break a build, so nothing starts. The job build always does,
# for the checks of the rules.
check "README.md docs/APP.md frontend/README.md LICENSE .claude/x.json" ""

# No Go test builds the interface, so the interface on its own does not
# start the Go tests.
check "frontend/src/App.svelte" "app interface"
check "frontend/src/routes/Updates.svelte" "app interface"
# Except the files of it a Go test reads.
check "frontend/src/lib/api.ts" "app go interface"
check "frontend/src/lib/suggest.cases.json" "app go interface"
check "frontend/src/lib/flow.ts" "app interface"

# The other way round.
check "engine/edit.go" "app go"
check "engine/edit_test.go go.sum" "app go"
check "go.mod" "app go"

# Whatever decides how the project is built starts everything.
check "Makefile" "app go interface"
check "scripts/build-ffmpeg.sh" "app go interface"
check ".github/workflows/ci.yml" "app go interface workflows"
check ".github/ci-filters.yml" "app go interface"
check ".golangci.yml" "app go interface"
check "frontend/package.json" "app interface"

# The rules that no job runs but their own check in build start nothing.
# Issue #322: a pull request that changed only them ran every job.
check "scripts/main-moved.sh scripts/main-moved-test.sh docs/BUILD.md" ""
check "scripts/plan-frozen.sh scripts/needs-build-test.sh" ""
check "scripts/changed.sh scripts/changed-test.sh" "changed"
check "scripts/needs-build.sh engine/edit.go" "app go"

# A file nobody thought of starts everything.
check "some/new/thing.py" "app go interface"
check ".gitignore" "app go interface"

# Every file of the interface a Go test reads starts the Go tests, found
# by the path the test reads it by, so a new one needs no case here.
for f in $(grep -rhoE --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build '"(\.\./)+frontend/[^"]*"' "$root" |
	tr -d '"' | sed 's#^\(\.\./\)*##' | sort -u); do
	[ -f "$root/$f" ] && check "$f" "app go interface"
done

[ "$fail" = 0 ] && echo "ok  	the CI filters behave"
exit $fail
