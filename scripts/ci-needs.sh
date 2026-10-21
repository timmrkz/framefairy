#!/bin/sh
# Says whether a kind of work is worth doing for the changes in hand, so CI
# does not fuzz two platforms over a typo in a README.
#
#   sh scripts/ci-needs.sh go         # the Go tests and the fuzzing
#   sh scripts/ci-needs.sh interface  # the type check and the interface tests
#   sh scripts/ci-needs.sh build      # building the programs and the interface
#   sh scripts/ci-needs.sh walks      # the interface against the Go side
#   sh scripts/ci-needs.sh outside    # the app on a Mac, opened from outside
#   sh scripts/ci-needs.sh workflows  # the check of the workflows themselves
#   sh scripts/ci-needs.sh changed    # the check of what make changed runs
#
# It writes "run=true" or "run=false" on standard output, in the shape a
# GitHub Actions step output takes, and says on standard error why.
#
# Every rule here errs towards running. A test that runs when it did not need
# to costs a minute. A test that does not run when it should have costs a
# broken main, and the whole point of the split is to keep the coverage while
# losing the waiting.
set -eu

kind=${1:?say what to decide about: go, interface, build, walks, outside, workflows or changed}

say() { echo "$*" >&2; }

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

# read_by_go FILE: whether a Go test reads this file of the interface, the
# way the bindings test reads api.ts. It is found by the path the test
# reads it by, "../frontend/..." in quotes, so a file a test only names in
# a comment does not count, and a new test that reads one needs no rule of
# its own here.
read_by_go() {
	grep -rqF --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build --exclude-dir=.git -- "../$1\"" "$root" 2>/dev/null
}
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
#
# Except the scripts that check a rule and the rules they check, when no
# job runs them but their own check: the build job checks those whatever
# changed, so they give no other job anything to do. A pull request that
# changed only them ran every job, issue #322. The one Go test that reads
# every script, for the endings of a video, then runs on main only, as it
# does for the docs. The rules for make changed are checked in the build
# job whenever it sets up Go, and a change to them alone sets it up, see
# changed below.
docs=0
frontend=0
go=0
other=0
workflows=0
rules=0
changed_rules=0

for file in $changed; do
	case $file in
	docs/* | *.md | LICENSE | .vscode/* | .claude/*)
		docs=$((docs + 1))
		;;
	frontend/*)
		frontend=$((frontend + 1))
		if read_by_go "$file"; then
			go=$((go + 1))
		fi
		;;
	*.go | go.mod | go.sum)
		go=$((go + 1))
		;;
	.github/workflows/*)
		workflows=$((workflows + 1))
		other=$((other + 1))
		;;
	scripts/main-moved.sh | scripts/main-moved-test.sh | scripts/needs-build.sh | scripts/needs-build-test.sh | \
		scripts/plan-frozen.sh | scripts/plan-frozen-test.sh)
		rules=$((rules + 1))
		;;
	scripts/changed.sh | scripts/changed-test.sh)
		changed_rules=$((changed_rules + 1))
		;;
	*)
		other=$((other + 1))
		;;
	esac
done

say "changed: $docs docs, $frontend interface, $go Go, $((rules + changed_rules)) rules, $other other"

# The check of the workflows reads the workflows and nothing else, so it is
# decided before the rule that anything unknown runs everything: a Makefile
# or Go code changing gives it nothing new to look at.
if [ "$kind" = workflows ]; then
	if [ "$workflows" -gt 0 ]; then
		answer true "a workflow changed"
	fi
	answer false "no workflow changed"
fi

# The check of what make changed runs, which reads the packages with Go.
# The build job runs it whenever it builds, and this says when it runs it
# without building: when its rules changed, or anything unknown.
if [ "$kind" = changed ]; then
	if [ "$changed_rules" -gt 0 ]; then
		answer true "the rules of make changed changed"
	fi
	if [ "$other" -gt 0 ]; then
		answer true "something changed that decides how the project is built"
	fi
	answer false "the rules of make changed did not change"
fi

if [ "$other" -gt 0 ]; then
	answer true "something changed that decides how the project is built"
fi

case $kind in
go)
	# The interface is built into the app by make, but no Go test builds
	# it, so a change under frontend/ cannot change what a Go test does,
	# except in a file a Go test reads, which counts as Go above.
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
	answer false "only docs and rules changed"
	;;
walks)
	# The interface driven against the Go side, see docs/TESTING.md, so
	# either side changing moves them.
	if [ "$go" -gt 0 ] || [ "$frontend" -gt 0 ]; then
		answer true "the interface or the Go side changed"
	fi
	answer false "only docs and rules changed"
	;;
outside)
	# The app as a whole, opened by macOS, see From outside the app in
	# docs/TESTING.md: what it does when a link arrives is on both sides.
	if [ "$go" -gt 0 ] || [ "$frontend" -gt 0 ]; then
		answer true "the interface or the Go side changed"
	fi
	answer false "only docs and rules changed"
	;;
*)
	answer true "do not know what $kind is, so it runs"
	;;
esac
