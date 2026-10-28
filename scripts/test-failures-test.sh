#!/bin/sh
# Checks test-failures.sh on what go test prints and on what gotestsum
# prints, because a mistake in it is silent: a failed job then says only
# that make stopped, and finding the test means opening the log.
#
#   sh scripts/test-failures-test.sh
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
fail=0

# expect NAME LOG WANT: the errors read out of LOG are WANT, line for line.
expect() {
	name=$1
	printf '%s' "$3" >"$work/want"
	sh "$here/test-failures.sh" "$2" >"$work/got"
	if cmp -s "$work/want" "$work/got"; then
		printf 'ok  \t%s\n' "$name"
	else
		printf 'FAIL\t%s\n\twanted:\n' "$name"
		sed 's/^/\t\t/' "$work/want"
		printf '\tgot:\n'
		sed 's/^/\t\t/' "$work/got"
		fail=1
	fi
}

# go test as it prints by itself, a subtest and a panic among them.
cat >"$work/gotest.log" <<'EOF'
--- FAIL: TestFails (0.00s)
    fail_test.go:6: a line the test printed
    fail_test.go:7: broken on purpose
--- FAIL: TestSub (0.00s)
    --- FAIL: TestSub/inner (0.00s)
        fail_test.go:11: inner broke
FAIL
panic: test timed out after 4m0s
	running tests:
		TestHangs (4m0s)

goroutine 1 [running]:
FAIL	framefairy/train	240.055s
EOF
expect "go test" "$work/gotest.log" '::error title=TestFails::--- FAIL: TestFails (0.00s)%0A    fail_test.go:6: a line the test printed%0A    fail_test.go:7: broken on purpose
::error title=TestSub::--- FAIL: TestSub (0.00s)
::error title=TestSub/inner::    --- FAIL: TestSub/inner (0.00s)%0A        fail_test.go:11: inner broke
::error title=panic::panic: test timed out after 4m0s%0A	running tests:%0A		TestHangs (4m0s)
'

# gotestsum, as make unit runs it when SLOWEST says: the failures again at
# the end, each with what it printed, a panic included.
cat >"$work/gotestsum.log" <<'EOF'
FAIL	framefairy/train	0.055s

=== Failed
=== FAIL: train TestFails (0.00s)
    fail_test.go:6: a line the test printed
    fail_test.go:7: broken on purpose

=== FAIL: train TestPanics (0.00s)
panic: assignment to entry in nil map [recovered, repanicked]

goroutine 55 [running]:
testing.tRunner.func1()
	/usr/local/go/src/testing/testing.go:2126 +0x65f

DONE 13 tests, 2 failures in 0.056s
The tests that took over 5s, slowest first:
framefairy/train TestSlow 6.2s
EOF
expect "gotestsum" "$work/gotestsum.log" '::error title=TestFails::=== FAIL: train TestFails (0.00s)%0A    fail_test.go:6: a line the test printed%0A    fail_test.go:7: broken on purpose
::error title=TestPanics::=== FAIL: train TestPanics (0.00s)%0Apanic: assignment to entry in nil map [recovered, repanicked]
'

exit $fail
