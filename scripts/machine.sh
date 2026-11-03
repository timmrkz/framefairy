#!/bin/sh
# Says in one line what machine the tests run on and how fast it is today,
# so a slow run of the Go tests can be told from a slow machine, #592.
# GitHub gives the macos job an M1 with 3 cores or an M2 Pro with 5, and
# the M1 is not equally fast from one run to the next, so the same tests
# take from about 60 s to 150 s. make unit prints the line before the
# tests when SLOWEST asks for the tests' times, which CI does, and once
# more after them.
#
#   sh scripts/machine.sh          the machine, then how fast it is
#   sh scripts/machine.sh after    how fast it is, after the tests
#   sh scripts/machine.sh after BEFORE JSON
#                                  and each package's time in fixed works,
#                                  from the line the machine printed before
#                                  and the go test -json events of the tests
#   sh scripts/machine.sh packages WORK JSON
#                                  only the packages, for a work of WORK s
#
# The chip, the cores, the memory, the ffmpeg that does the work, and how
# long one fixed piece of ffmpeg work took: a test picture of twenty
# seconds drawn and encoded, the same work every run, about a second on
# CI's Mac. Two of them a run show how much the measure itself varies.
# Nothing in it fails: a fact it cannot read is left out.
#
# A package's time over the mean of the two pieces of fixed work, its
# fixed works, compares runs on the same chip, which the line names.
# Over 149 runs on the M1, the engine's seconds varied by 17 % and its
# fixed works by 11 %. It does not make the two chips alike: the M2 Pro's
# two cores more still show, about 60 fixed works against 68, #592.
set -u

# Every package that took over a second, slowest first, from the line
# go test prints as a package ends, "ok  \tframefairy/engine\t89.845s",
# the time the log shows above. The Elapsed of the event after it can be
# seconds longer. A package that failed says so, since a hang ends at the
# timeout and its time says nothing about how fast it was.
packages() {
	awk '/"Action":"output"/ && !/"Test":/ && match($0, /"Output":"(ok  |FAIL)\\t[^"]*\\t[0-9.]+s/) {
		n = split(substr($0, RSTART + 10, RLENGTH - 11), f, /\\t/)
		if (f[n] + 0 > 1) print f[n] "\t" f[2] (f[1] == "FAIL" ? " (failed)" : "")
	}' "$2" | sort -t "$(printf '\t')" -k1,1nr | awk -F '\t' -v work="$1" '
		{ name[NR] = $2; secs[NR] = $1; if (length($2) > w) w = length($2) }
		END {
			if (!NR) exit
			print "Each package that took over a second, in seconds and in pieces of the fixed work, " work " s, which compare runs on the same chip:"
			if (w < 7) w = 7
			bar = ""; for (i = 0; i < w; i++) bar = bar "-"
			row = "| %-" w "s | %8s | %12s |\n"
			printf row, "package", "time, s", "fixed works"
			printf "| %s | -------: | -----------: |\n", bar
			row = "| %-" w "s | %8.2f | %12.1f |\n"
			for (i = 1; i <= NR; i++) printf row, name[i], secs[i], secs[i] / work
		}'
}

if [ "${1:-}" = packages ]; then
	[ -n "${2:-}" ] && [ -r "${3:-}" ] && packages "$2" "$3"
	exit 0
fi

if [ "${1:-}" = after ]; then
	line="machine after the tests:"
	before=$(printf '%s\n' "${2:-}" | sed -n 's/.*fixed ffmpeg work \([0-9.]*\) s on the processor.*/\1/p')
	json=${3:-}
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

if [ -n "${before:-}" ] && [ -n "$bench" ] && [ -r "${json:-}" ]; then
	packages "$(awk -v a="$before" -v b="$1" 'BEGIN { printf "%.3f", (a + b) / 2 }')" "$json"
fi
