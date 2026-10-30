#!/bin/sh
# Says in one line what machine the tests run on and how fast it is today,
# so a slow run of the Go tests can be told from a slow machine, #592.
# GitHub runs the same job on machines whose tests differ by a fifth on
# average, and by more from run to run, and the log said nothing of which
# machine it was. make unit prints it before the tests when SLOWEST asks
# for the tests' times, which CI does, and once more after them.
#
#   sh scripts/machine.sh          the machine, then how fast it is
#   sh scripts/machine.sh after    how fast it is, after the tests
#
# The chip, the cores, the memory, the ffmpeg that does the work, and how
# long one fixed piece of ffmpeg work took: a test picture of twenty
# seconds drawn and encoded, the same work every run, about a second on
# CI's Mac. Two of them a run show how much the measure itself varies.
# Nothing in it fails: a fact it cannot read is left out.
set -u

if [ "${1:-}" = after ]; then
	line="machine after the tests:"
else
	case $(uname) in
	Darwin)
		chip=$(sysctl -n machdep.cpu.brand_string 2>/dev/null)
		cores=$(sysctl -n hw.ncpu 2>/dev/null)
		bytes=$(sysctl -n hw.memsize 2>/dev/null)
		;;
	*)
		chip=$(sed -n 's/^model name[[:space:]]*: //p' /proc/cpuinfo 2>/dev/null | head -n 1)
		cores=$(getconf _NPROCESSORS_ONLN 2>/dev/null)
		bytes=$(awk '/^MemTotal:/ { print $2 * 1024 }' /proc/meminfo 2>/dev/null)
		;;
	esac
	line="machine:"
	[ -n "$chip" ] && line="$line $chip,"
	[ -n "$cores" ] && line="$line $cores cores,"
	[ -n "$bytes" ] && line="$line $(awk -v b="$bytes" 'BEGIN { printf "%.0f GB", b / 1073741824 }'),"
	# The measure below compares only runs of the same ffmpeg.
	version=$(ffmpeg -hide_banner -version 2>/dev/null | sed -n '1s/^ffmpeg version \([^ ]*\).*/\1/p')
	[ -n "$version" ] && line="$line ffmpeg $version,"
fi

# -benchmark makes ffmpeg say the time it took on its last line,
# "bench: utime=0.965s stime=0.023s rtime=0.732s", on any system.
bench=$(ffmpeg -hide_banner -nostdin -benchmark -f lavfi -i testsrc2=s=1280x720:r=25:d=20 \
	-threads 1 -filter_threads 1 -c:v mpeg4 -q:v 2 -f null - 2>&1 | sed -n 's/^bench: utime=\([0-9.]*\)s.*rtime=\([0-9.]*\)s.*/\1 \2/p')
if [ -n "$bench" ]; then
	set -- $bench
	line="$line fixed ffmpeg work $1 s on the processor, $2 s on the clock"
fi

echo "${line%,}"
