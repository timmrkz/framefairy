#!/bin/sh
# Fuzzes every target, as many targets at a time as the machine has cores,
# or only the targets FUZZTARGETS names, by name and separated by spaces.
# FUZZSHARD=2/3 fuzzes the second third of them, so a deep run can be
# spread over machines, and --list says which targets there are.
#
# The work is a number of executions, not a number of seconds, so a run here
# and a run in CI do the same thing on a fast machine and a slow one. Each
# target gets one worker, so the cores go to different targets rather than to
# the same one, which is what makes the fan-out worth anything.
set -eu

cd "$(dirname -- "$0")/.."

GO=${GO:-go}
FUZZTIME=${FUZZTIME:-10000x}
SHRINKTIME=${SHRINKTIME:-100x}
JOBS=${FUZZJOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)}
case $JOBS in
[1-9] | [1-9][0-9]*) ;;
*) JOBS=4 ;;
esac

# Called once per target by xargs below.
if [ "${1:-}" = "--one" ]; then
	pkg=$2
	target=$3
	work=$FUZZTIME
	# One execution of the licence dispenser's simulation is a whole
	# history of 400 steps, a fifth of a second, where every other target
	# reads one input in well under a millisecond. Ten thousand of them
	# would be half an hour, so it plays 300 new histories a run, 36
	# seconds on the cloud machine.
	#
	# Its input is only the seed of a random generator, so coverage cannot
	# steer it: one bit more is a history that has nothing to do with the
	# last. It is fuzzed for new histories at night, see fuzz-nightly.yml,
	# and CI's fuzz job passes SIMTIME=0 to leave it to that. Its known
	# seeds still play in every go test.
	if [ "$target" = FuzzSimulation ]; then
		work=${SIMTIME:-300x}
		case $work in 0 | 0x | 0s)
			printf 'skip\t%s, SIMTIME is %s\n' "$target" "$work"
			exit 0
			;;
		esac
	fi
	# An input that reaches new code is shrunk before it is kept, and Go
	# shrinks for up to a minute by the clock, in runs FUZZTIME does not
	# count. Its search is a run for every pair of places in the input, so
	# a target whose run takes milliseconds rather than microseconds shrank
	# for the whole minute every time and the fuzzing seemed to hang. A
	# count bounds that work the way FUZZTIME bounds the rest.
	if out=$($GO test -run '^$' -fuzz "^$target\$" -fuzztime "$work" -fuzzminimizetime "$SHRINKTIME" -parallel 1 "$pkg" 2>&1); then
		# A target go test did not find, behind a build tag for one, is
		# fuzzed by nobody and would read as a pass.
		case $out in *"no tests to run"* | *"matches no fuzz tests"*)
			printf 'FAIL\t%s was not found in %s\n%s\n' "$target" "$pkg" "$out"
			exit 1
			;;
		esac
		printf 'ok  \t%s %s\n' "$target" "$work"
		exit 0
	fi
	printf 'FAIL\t%s\n%s\n' "$target" "$out"
	exit 1
fi

# Every target of the module, a package directory and a name to a line,
# found in the source. Asking go test for them instead compiles the tests
# of every package, which was two minutes of CI's fuzz job before anything
# was fuzzed.
all_targets() {
	grep -rlE --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build --exclude-dir=.git --exclude-dir=testdata '^func Fuzz[A-Za-z0-9_]*\(f \*testing\.F\)' . |
		sort | while read -r file; do
		dir=$(dirname -- "$file")
		sed -nE 's/^func (Fuzz[A-Za-z0-9_]*)\(f \*testing\.F\).*/\1/p' "$file" | while read -r name; do
			echo "$dir $name"
		done
	done
}

if [ "${1:-}" = "--list" ]; then
	all_targets
	exit 0
fi

ONLY=${FUZZTARGETS:-}
targets=$(all_targets)
if [ -n "$ONLY" ]; then
	named=""
	for name in $ONLY; do
		line=$(printf '%s\n' "$targets" | awk -v n="$name" '$2 == n { print; exit }')
		# A name that is no target would fuzz nothing and read as a pass.
		if [ -z "$line" ]; then
			echo "fuzz.sh: there is no fuzz target called $name" >&2
			exit 1
		fi
		named="$named$line
"
	done
	targets=$(printf '%s' "$named")
fi

# One share of the targets, every nth from the ith on.
SHARD=${FUZZSHARD:-}
if [ -n "$SHARD" ]; then
	i=${SHARD%/*}
	n=${SHARD#*/}
	case "$i/$n" in
	[1-9]/[1-9] | [1-9]/[1-9][0-9] | [1-9][0-9]/[1-9][0-9]) ;;
	*)
		echo "fuzz.sh: FUZZSHARD is i/n, like 2/3, not $SHARD" >&2
		exit 1
		;;
	esac
	targets=$(printf '%s\n' "$targets" | awk -v i="$i" -v n="$n" '(NR - 1) % n == i - 1')
fi

[ -z "$targets" ] && exit 0

printf '%s\n' "$targets" | xargs -P "$JOBS" -n 2 sh "$0" --one
