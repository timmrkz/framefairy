#!/bin/sh
# Names and stops what the Go tests left running: ffmpeg and ffprobe, the
# episode's decoder, llama-server, the test binaries and go itself. CI runs
# it after the Go tests whichever way they ended, passed, failed, out of
# time or cancelled. A program a test started and never stopped holds a job
# open until somebody cancels it by hand, which is how the macOS job once
# waited half an hour on two ffmpeg programs, and the log never said whose
# they were.
#
#   sh scripts/leftovers.sh
#
# It prints nothing when nothing is left. Otherwise it prints a table of
# what is left, with what started it and how long it has run, names it on
# the check, stops it, and fails: a test that leaves a program running is
# a test with a bug, even when it passed.
set -u

me=$$

# One line per process: pid, parent, how long it has run, the command. The
# same keywords on macOS and on Linux.
left=$(ps -A -o pid= -o ppid= -o etime= -o args= | awk -v me="$me" '
{
	prog = $4
	sub(/.*\//, "", prog)
	if ($1 == me) next
	if (prog ~ /^(ffmpeg|ffprobe|framefairy-frames|llama-server|go)$/ || prog ~ /\.test$/) print
}')

if [ -z "$left" ]; then
	exit 0
fi

# Padded to the widest cell of each column, numbers to the right, so the
# table reads straight down in the log.
table=$(printf '%s\n' "$left" | awk '
{
	pid[NR] = $1; parent[NR] = $2; ran[NR] = $3
	cmd = $0
	sub(/^[ \t]*[^ \t]+[ \t]+[^ \t]+[ \t]+[^ \t]+[ \t]+/, "", cmd)
	if (length(cmd) > 160) cmd = substr(cmd, 1, 159) "…"
	what[NR] = cmd
	if (length($1) > wp) wp = length($1)
	if (length($2) > wa) wa = length($2)
	if (length($3) > wr) wr = length($3)
}
END {
	if (wp < 3) wp = 3
	if (wa < 6) wa = 6
	if (wr < 3) wr = 3
	printf "%" wp "s  %" wa "s  %" wr "s  %s\n", "pid", "parent", "ran", "command"
	for (i = 1; i <= NR; i++)
		printf "%" wp "s  %" wa "s  %" wr "s  %s\n", pid[i], parent[i], ran[i], what[i]
}')

echo "The tests left these running:"
printf '%s\n' "$table"

# One error on the check, so the pull request says it without the log.
n=$(printf '%s\n' "$left" | wc -l | tr -d ' ')
first=$(printf '%s\n' "$table" | sed -n 2p | sed 's/%/%25/g')
echo "::error title=Left running::$n programs were still running after the Go tests, the first: $first"

printf '%s\n' "$left" | awk '{ print $1 }' | xargs kill -KILL 2>/dev/null
exit 1
