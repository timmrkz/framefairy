#!/bin/sh
# Says which Go tests failed, as errors on the CI check, read out of the
# output of go test in LOG. Without it a failed job says only that make
# stopped with code 2, and finding the test means opening the job's log,
# which the pull request does not show and a session in the cloud cannot
# read.
#
#   sh scripts/test-failures.sh unit.log
#
# One error per failed test, with the lines the test printed under it,
# and one per panic, with the tests that were running. It reads what go
# test prints and what gotestsum prints, see make unit. At most ten, which
# is as many as GitHub shows for a step.
set -eu

log=${1:?say which log of go test to read}

awk '
function esc(s) { gsub(/%/, "%25", s); gsub(/\r/, "%0D", s); return s }
function title(s) { s = esc(s); gsub(/:/, "%3A", s); gsub(/,/, "%2C", s); return s }
# The first twenty lines a test printed and the last sixty, with an
# ellipsis between when there were more: a walk that broke a rule prints
# its steps first and the rule it broke last.
function flush(   i, text) {
	while (lines > 0 && line[lines] == "") lines--
	if (name != "" && shown < 10) {
		text = body
		for (i = 1; i <= lines; i++) {
			if (lines > 80 && i == 21) text = text "%0A        …"
			if (lines <= 80 || i <= 20 || i > lines - 60) text = text "%0A" line[i]
		}
		printf "::error title=%s::%s\n", title(name), text
		shown++
	}
	name = ""; body = ""; lines = 0; block = 0
}
# "--- FAIL: TestName (1.23s)", the subtests indented under their test.
/^[ \t]*--- FAIL: / {
	flush()
	name = $3; body = esc($0)
	next
}
# Under gotestsum, which make unit runs when SLOWEST says, a failed test
# prints no "--- FAIL" line where it failed. gotestsum names it again at
# the end, "=== FAIL: package TestName (1.23s)", with everything it
# printed under it, blank lines and a panic included, up to the next
# "=== " or "DONE". A package that failed as a whole, by a timeout or a
# build that broke, has no test name there, "=== FAIL: package  (0.00s)",
# and is named after the package. What the compiler said comes last,
# under "=== Errors".
/^=== FAIL: / {
	flush()
	name = $4 ~ /^\(/ ? $3 : $4; body = esc($0); block = 1
	next
}
/^=== Errors$/ {
	flush()
	name = "build"; body = esc($0); block = 1
	next
}
/^=== / || /^DONE / { flush(); next }
block {
	line[++lines] = esc($0)
	next
}
# A panic, a test that ran out of time among them, ends the binary: the
# tests running then are listed under it.
/^panic: / {
	flush()
	name = "panic"; body = esc($0)
	next
}
# What the failed test printed, indented under it.
name != "" && /^[ \t]/ {
	line[++lines] = esc($0)
	next
}
{ flush() }
END { flush() }
' "$log"
