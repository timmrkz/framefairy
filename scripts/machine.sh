#!/bin/sh
# Says in one line what machine the tests run on and how fast it is today,
# so a slow run of the Go tests can be told from a slow machine, #592.
# GitHub runs the same job on machines that differ by a fifth or more, and
# the log said nothing of which one it was. make unit prints it before the
# tests when SLOWEST asks for the tests' times, which CI does.
#
#   sh scripts/machine.sh
#
# The chip, the cores, the memory, the runner image when there is one, and
# how long one fixed piece of ffmpeg work took, about a second on one core:
# a test picture of ten seconds drawn and encoded, the same work every run.
# Nothing in it fails: a fact it cannot read is left out.
set -u

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
# GitHub's runners name their image in these.
[ -n "${ImageOS:-}" ] && line="$line image $ImageOS ${ImageVersion:-},"

# -benchmark makes ffmpeg say the time it took on its last line,
# "bench: utime=0.965s stime=0.023s rtime=0.732s", on any system.
bench=$(ffmpeg -hide_banner -nostdin -benchmark -f lavfi -i testsrc2=s=1280x720:r=25:d=10 \
	-threads 1 -filter_threads 1 -c:v mpeg4 -q:v 2 -f null - 2>&1 | sed -n 's/^bench: utime=\([0-9.]*\)s.*rtime=\([0-9.]*\)s.*/\1 \2/p')
if [ -n "$bench" ]; then
	set -- $bench
	line="$line fixed ffmpeg work $1 s on the processor, $2 s on the clock"
fi

echo "${line%,}"
