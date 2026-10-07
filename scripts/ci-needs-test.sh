#!/bin/sh
# Checks the rules in ci-needs.sh, because a mistake in them is silent: CI
# goes green having run less than it should, and nobody finds out until main
# is broken. Every case below is one a pull request really arrives as.
#
#   sh scripts/ci-needs-test.sh
#
# It stands in for git with a script that prints the changed files, so the
# rules can be checked without making a commit to try them on.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
needs=$here/ci-needs.sh
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

check() {
	files=$1
	kind=$2
	want=$3
	cat >"$work/git" <<GIT
#!/bin/sh
printf '%s\n' $files
GIT
	chmod +x "$work/git"
	got=$(PATH="$work:$PATH" GITHUB_EVENT_NAME=pull_request sh "$needs" "$kind" 2>/dev/null)
	if [ "$got" = "run=$want" ]; then
		printf 'ok  \t%-10s %-42s %s\n' "$kind" "$files" "$got"
	else
		printf 'FAIL\t%-10s %-42s %s, wanted run=%s\n' "$kind" "$files" "$got" "$want"
		fail=1
	fi
}

# Docs cannot break a build, so nothing runs.
check "README.md docs/APP.md" go false
check "README.md docs/APP.md" interface false
check "README.md docs/APP.md" build false
check "README.md docs/APP.md" walks false
check "README.md docs/APP.md" outside false

# No Go test builds the interface, so the interface changing on its own
# cannot change what a Go test does. make does build it into the program,
# so the build still has to prove they go together.
check "frontend/src/App.svelte" go false
check "frontend/src/App.svelte" interface true
check "frontend/src/App.svelte" build true
check "frontend/src/App.svelte" walks true
check "frontend/src/App.svelte" outside true

# Except the files of the interface a Go test reads: the bindings test
# reads api.ts and the engine reads the cases it shares with suggest.ts.
# A file a Go test only names in a comment is not one of them.
check "frontend/src/lib/api.ts" go true
check "frontend/src/lib/suggest.cases.json" go true
check "frontend/src/lib/flow.ts" go false

# The other way round.
check "engine/edit.go go.sum" go true
check "engine/edit.go go.sum" interface false
check "engine/edit.go go.sum" build true
check "engine/edit.go" walks true
check "cmd/framefairy-app/licence.go" outside true
check "internal/outside/outside_test.go" outside true
check "Makefile" outside true

# Whatever decides how the project is built runs everything, including the
# rules themselves.
check "Makefile" go true
check "Makefile" interface true
check ".github/workflows/ci.yml" interface true
check "scripts/ci-needs.sh" go true

# A mix runs everything it touches.
check "engine/edit.go frontend/src/App.svelte" go true
check "engine/edit.go frontend/src/App.svelte" interface true

# Anything the rules do not recognise runs the lot, which is the whole
# safety of them: a new kind of file is never quietly skipped.
check ".gitignore" go true
check "some/new/thing.py" interface true
check "Dockerfile" build true

# A push to main narrows nothing, whatever it touched.
got=$(GITHUB_EVENT_NAME=push sh "$needs" go 2>/dev/null)
if [ "$got" = "run=true" ]; then
	printf 'ok  \t%-10s %-42s %s\n' "go" "a push to main" "$got"
else
	printf 'FAIL\t%-10s %-42s %s, wanted run=true\n' "go" "a push to main" "$got"
	fail=1
fi

if [ "$fail" = 0 ]; then
	echo "ok  	the CI rules behave"
fi
exit $fail
