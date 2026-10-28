#!/bin/sh
# Checks test-failures.sh on what go test prints and on what gotestsum
# prints, because a mistake in it is silent: a failed job then says only
# that make stopped, and finding the test means opening the log.
#
#   sh scripts/test-failures-test.sh
#
# The logs in testdata/test-failures/ are what go test and gotestsum
# v1.13.0 really printed, the way make unit runs them, for a module of
# three packages: one whose tests fail, one printing a blank line before
# its reason, a subtest and a panic, one that runs out of time, and one
# that does not build. Their paths are cut to /work. Each .want is the
# errors read out of its .log, and gotest.want is what main read before
# gotestsum came in.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
data=$here/testdata/test-failures
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

for name in gotest gotestsum; do
	sh "$here/test-failures.sh" "$data/$name.log" >"$work/got"
	if cmp -s "$data/$name.want" "$work/got"; then
		printf 'ok  \t%s\n' "$name"
	else
		printf 'FAIL\t%s\n' "$name"
		diff "$data/$name.want" "$work/got" | sed 's/^/\t/' || true
		fail=1
	fi
done

# The reason a test gave after a blank line is still there, and a
# package that failed as a whole is named after it.
for want in 'title=TestBlank::.*%0A%0A    a_test.go:12: the reason after a blank line' \
	'title=b::=== FAIL: b  (0.00s)%0Apanic: test timed out' \
	'title=build::=== Errors%0Ac/c_test.go:6:2: undefined: undefinedThing'; do
	if grep -q "$want" "$data/gotestsum.want"; then
		printf 'ok  \t%s\n' "$want"
	else
		printf 'FAIL\tgotestsum.want has no %s\n' "$want"
		fail=1
	fi
done

exit $fail
