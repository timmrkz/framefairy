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
# and one per panic, with the tests that were running. At most ten, which
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
	if (name != "" && shown < 10) {
		text = body
		for (i = 1; i <= lines; i++) {
			if (lines > 80 && i == 21) text = text "%0A        …"
			if (lines <= 80 || i <= 20 || i > lines - 60) text = text "%0A" line[i]
		}
		printf "::error title=%s::%s\n", title(name), text
		shown++
	}
	name = ""; body = ""; lines = 0
}
# "--- FAIL: TestName (1.23s)", the subtests indented under their test.
/^[ \t]*--- FAIL: / {
	flush()
	name = $3; body = esc($0)
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
