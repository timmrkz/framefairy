#!/bin/sh
# Runs one share of the Go tests, so CI can spread them over machines.
#
#   GOSHARD=1/2 sh scripts/go-shard.sh -race ./...   the first half
#
# The arguments are what go test is given, flags and then packages. Every
# test of every package is numbered in the order go test lists them, and
# share i of n takes every nth from the ith on, so the slow tests of one
# file spread over the shares rather than landing in one, and together the
# shares are every test there is. A fuzz target counts as a test, for the
# seeds go test plays.
#
# The list comes from go test -list with the same flags, which builds what
# the run then finds built.
set -eu

shard=${GOSHARD:?say which share, like GOSHARD=1/2}
i=${shard%/*}
n=${shard#*/}
case "$i/$n" in
[1-9]/[1-9]) ;;
*)
	echo "go-shard.sh: GOSHARD is i/n, like 1/2, not $shard" >&2
	exit 1
	;;
esac
if [ "$i" -gt "$n" ]; then
	echo "go-shard.sh: there is no share $i of $n" >&2
	exit 1
fi

GO=${GO:-go}

# go test -list prints a package's tests, one name to a line, and then a
# line for the package, "ok  <package> <time>".
if ! listed=$($GO test -list '.*' "$@" 2>&1); then
	echo "$listed"
	exit 1
fi
names=$(printf '%s\n' "$listed" | grep -E '^(Test|Fuzz|Example)[A-Za-z0-9_]*$' |
	awk -v i="$i" -v n="$n" '(NR - 1) % n == i - 1' | sort -u)

if [ -z "$names" ]; then
	echo "go-shard.sh: share $i of $n has no tests"
	exit 0
fi

# One expression for the names of this share. A name two packages share
# runs in both, which only ever runs a test twice, never not at all.
run="^($(printf '%s\n' "$names" | paste -sd '|' -))\$"
printf 'share %s of %s: %s tests\n' "$i" "$n" "$(printf '%s\n' "$names" | wc -l | tr -d ' ')"
exec $GO test -run "$run" "$@"
