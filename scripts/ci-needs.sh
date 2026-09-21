#!/bin/sh
# Says whether a kind of work is worth doing for the changes in hand, so CI
# does not fuzz two platforms over a typo in a README.
#
#   sh scripts/ci-needs.sh go         # the Go tests and the fuzzing
#   sh scripts/ci-needs.sh interface  # the type check and the interface tests
#   sh scripts/ci-needs.sh build      # building the programs and the interface
#
# It writes "run=true" or "run=false" on standard output, in the shape a
# GitHub Actions step output takes, and says on standard error why.
#
# Every rule here errs towards running. A test that runs when it did not need
# to costs a minute. A test that does not run when it should have costs a
# broken main, and the whole point of the split is to keep the coverage while
# losing the waiting.
set -eu

kind=${1:?say what to decide about: go, interface or build}

say() { echo "$*" >&2; }
answer() {
	say "$2"
	echo "run=$1"
	exit 0
}

# Only a pull request is ever narrowed. A push to main builds and tests
# everything, whatever it touched, so main is always known to be sound and a
# mistake in the rules below can never be what hides a break.
if [ "${GITHUB_EVENT_NAME:-}" != "pull_request" ]; then
	answer true "not a pull request, so everything runs"
fi

# On a pull request the checkout is a merge commit whose first parent is the
# base branch, so this is exactly what the pull request changes. Two commits
# of history are enough for it, which is why the checkout is shallow.
if ! changed=$(git diff --name-only HEAD^1 HEAD 2>/dev/null); then
	answer true "the changed files could not be worked out, so everything runs"
fi
if [ -z "$changed" ]; then
	answer true "nothing looks changed, which is odd, so everything runs"
fi

# Every changed file is one of these. Anything that matches none of them
# counts as other, and other runs the lot: the Makefile, the workflow, the
# scripts and the Go module files all decide how things are built, and none
# of them is safe to call harmless.
docs=0
frontend=0
go=0
other=0

for file in $changed; do
	case $file in
	docs/* | *.md | LICENSE | .vscode/* | .claude/*)
		docs=$((docs + 1))
		;;
	frontend/*)
		frontend=$((frontend + 1))
		;;
	*.go | go.mod | go.sum)
		go=$((go + 1))
		;;
	*)
		other=$((other + 1))
		;;
	esac
done

say "changed: $docs docs, $frontend interface, $go Go, $other other"

if [ "$other" -gt 0 ]; then
	answer true "something changed that decides how the project is built"
fi

case $kind in
go)
	# The interface is built into the app by make, but no Go test builds
	# it, so a change under frontend/ cannot change what a Go test does.
	if [ "$go" -gt 0 ]; then
		answer true "Go code changed"
	fi
	answer false "no Go code changed"
	;;
interface)
	if [ "$frontend" -gt 0 ]; then
		answer true "the interface changed"
	fi
	answer false "the interface did not change"
	;;
build)
	# make builds the interface into the program, so either side changing
	# is a reason to prove they still go together.
	if [ "$go" -gt 0 ] || [ "$frontend" -gt 0 ]; then
		answer true "code changed on one side or the other"
	fi
	answer false "only docs changed"
	;;
*)
	answer true "do not know what $kind is, so it runs"
	;;
esac
