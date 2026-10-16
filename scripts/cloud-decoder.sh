#!/bin/bash
# Builds the ffmpeg we ship and the episode's decoder in a cloud session, in
# the background, so the session starts at once. The setup script cannot do
# it: ffmpeg from source takes longer than the five minutes a setup script
# may run. Started by the SessionStart hook in .claude/settings.json, and
# only in cloud sessions. Until it is done, the decoder's tests skip, as
# without it. The log is /tmp/framefairy-decoder.log, and the build is done
# when its last line says "decoder ready".

[ "$CLAUDE_CODE_REMOTE" = "true" ] || exit 0
repo=$(cd "$(dirname "$0")/.." && pwd) || exit 0
[ -x "$repo/.build/frames/framefairy-frames" ] && exit 0
lock=/tmp/framefairy-decoder.lock
# One build at a time, also when a second session starts on the machine.
if ! mkdir "$lock" 2>/dev/null; then
	exit 0
fi
log=/tmp/framefairy-decoder.log
nohup setsid bash -c '
	trap "rmdir \"$2\"" EXIT
	cd "$1" || exit 1
	export PATH="/usr/local/go/bin:$PATH"
	if [ ! -f .build/ffmpeg/lib/pkgconfig/libavcodec.pc ]; then
		make ffmpeg || { echo "decoder failed"; exit 1; }
	fi
	make frames && echo "decoder ready" || echo "decoder failed"
' _ "$repo" "$lock" >"$log" 2>&1 </dev/null &
echo "Building ffmpeg and the episode's decoder in the background, see $log"
