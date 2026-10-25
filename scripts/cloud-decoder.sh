#!/bin/bash
# Builds the ffmpeg we ship and the episode's decoder in a cloud session, in
# the background, so the session starts at once. The setup script cannot do
# it: a setup that runs longer than about five minutes is not cached, and
# new sessions can be stopped while it runs, and ffmpeg from source alone
# takes about six. Started by the SessionStart hook in
# .claude/settings.json, and only in cloud sessions. The log is
# /tmp/framefairy-decoder.log, and the build is done when its last line
# says "decoder ready".
#
#   scripts/cloud-decoder.sh        start the build, unless it is not needed
#   scripts/cloud-decoder.sh wait   wait for a build that is running
#
# make waits before the targets that run the decoder, see the Makefile, so
# a test does not skip only because it came before the build was done.

[ "$CLAUDE_CODE_REMOTE" = "true" ] || exit 0
repo=$(cd "$(dirname "$0")/.." && pwd) || exit 0
log=/tmp/framefairy-decoder.log
# One build at a time, also when a second session starts on the machine.
# The kernel lets go of the lock when the build ends, however it ends.
lock=/tmp/framefairy-decoder.lock
exec 9>>"$lock" || exit 0

if [ "$1" = wait ]; then
	if ! flock -n 9; then
		echo "Waiting for the episode's decoder, which is building in the background, see $log"
		flock 9
	fi
	if [ ! -x "$repo/.build/frames/framefairy-frames" ] &&
		[ "$(tail -n 1 "$log" 2>/dev/null)" = "decoder failed" ]; then
		echo "The episode's decoder did not build in the background, see $log."
		echo "Its tests skip until make ffmpeg frames builds it."
	fi
	exit 0
fi

flock -n 9 || exit 0
# ffmpeg is done when make ffmpeg wrote the sum of the script that built
# it, the rule scripts/tools.sh keeps on the Mac.
recipe=$(cksum "$repo/scripts/build-ffmpeg.sh" | cut -d' ' -f1)
ffmpeg_done=0
[ "$(cat "$repo/.build/ffmpeg/built" 2>/dev/null)" = "$recipe" ] && ffmpeg_done=1
[ "$ffmpeg_done" = 1 ] && [ -x "$repo/.build/frames/framefairy-frames" ] && exit 0

# The build holds the lock on 9 until it ends.
nohup setsid bash -c '
	cd "$1" || exit 1
	export PATH="/usr/local/go/bin:$PATH"
	if [ "$2" != 1 ]; then
		make ffmpeg || { echo "decoder failed"; exit 1; }
	fi
	make frames && echo "decoder ready" || echo "decoder failed"
' _ "$repo" "$ffmpeg_done" >"$log" 2>&1 </dev/null &
echo "Building ffmpeg and the episode's decoder in the background, see $log. make unit, walks, changed and test wait for it."
