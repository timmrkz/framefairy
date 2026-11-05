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
# that does not build. Their paths are cut to /work, and they end in .txt
# because the repository ignores every .log. Each .want is the
# errors read out of its .txt, and gotest.want is what main read before
# gotestsum came in.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
data=$here/testdata/test-failures
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

for name in gotest gotestsum; do
	sh "$here/test-failures.sh" "$data/$name.txt" >"$work/got"
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

# A log with a hang dump in it names the hang first, and then no more
# than nine failures, so the ten GitHub shows still hold all of them. The
# dump's line is put in the log of the package that ran out of time.
sed '/^panic: test timed out/i\
== hangdump: the programs process 4242 started, 1 of them, before go test'"'"'s -timeout' \
	"$data/gotestsum.txt" >"$work/hung.txt"
sh "$here/test-failures.sh" "$work/hung.txt" >"$work/hung"
if head -n 1 "$work/hung" | grep -q '^::error title=A program a test started hung::.*== hangdump' &&
	[ "$(grep -c '^::error' "$work/hung")" -le 10 ] &&
	[ "$(tail -n +2 "$work/hung" | sed 's/::.*//')" = "$(head -n 9 "$data/gotestsum.want" | sed 's/::.*//')" ]; then
	printf 'ok  \t%s\n' "a hang dump is named first"
else
	printf 'FAIL\t%s\n' "a hang dump is named first"
	sed 's/^/\t/' "$work/hung"
	fail=1
fi
# A hang dump whose stack is inside the virtual Mac's call to its host
# says it is the virtual Mac's, and one without it does not.
sed '/^panic: test timed out/i\
== hangdump: the programs process 4242 started, 1 of them, before go test'"'"'s -timeout\
    VTParavirtualizationGuestSendMessageWithIOSurfacesToHostAndCopyReplySync' \
	"$data/gotestsum.txt" >"$work/host.txt"
sh "$here/test-failures.sh" "$work/host.txt" >"$work/host"
if head -n 1 "$work/host" | grep -q "^::error title=A program a test started hung in the virtual Mac's media engine::" &&
	! head -n 1 "$work/hung" | grep -q 'virtual Mac'; then
	printf 'ok  \t%s\n' "a hang in the virtual Mac's media engine is named as its own"
else
	printf 'FAIL\t%s\n' "a hang in the virtual Mac's media engine is named as its own"
	sed 's/^/\t/' "$work/host"
	fail=1
fi
grep -q '== hangdump' "$data/gotestsum.txt" "$data/gotest.txt" && {
	printf 'FAIL\t%s\n' "a log with no hang dump names no hang"
	fail=1
}

exit $fail
