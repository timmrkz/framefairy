#!/bin/sh
# Fuzzes every target, as many targets at a time as the machine has cores.
#
# The work is a number of executions, not a number of seconds, so a run here
# and a run in CI do the same thing on a fast machine and a slow one. Each
# target gets one worker, so the cores go to different targets rather than to
# the same one, which is what makes the fan-out worth anything.
set -eu

GO=${GO:-go}
FUZZTIME=${FUZZTIME:-10000x}
JOBS=${FUZZJOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)}
case $JOBS in
[1-9] | [1-9][0-9]*) ;;
*) JOBS=4 ;;
esac

# Called once per target by xargs below.
if [ "${1:-}" = "--one" ]; then
	pkg=$2
	target=$3
	if out=$($GO test -run '^$' -fuzz "^$target\$" -fuzztime "$FUZZTIME" -parallel 1 "$pkg" 2>&1); then
		printf 'ok  \t%s %s\n' "$target" "$FUZZTIME"
		exit 0
	fi
	printf 'FAIL\t%s\n%s\n' "$target" "$out"
	exit 1
fi

for pkg in $($GO list ./...); do
	for target in $($GO test -list 'Fuzz.*' "$pkg" 2>/dev/null | grep '^Fuzz' || true); do
		echo "$pkg $target"
	done
done | xargs -P "$JOBS" -n 2 sh "$0" --one
