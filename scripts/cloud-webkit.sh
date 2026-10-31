#!/bin/bash
# Installs Playwright's WebKit in a cloud session, in the background, so a
# session can walk a sequence in the engine the app runs in on a Mac, see
# Two browsers in docs/TESTING.md. The environment's own browsers hold
# Chromium alone. It is the version of Playwright frontend/package.json
# pins, with the system libraries it needs, the way CI's image gets it,
# see .github/ci-image/Dockerfile. Started by the SessionStart hook in
# .claude/settings.json, and only in cloud sessions. The log is
# /tmp/framefairy-webkit.log, and it is done when its last line says
# "webkit ready".

[ "$CLAUDE_CODE_REMOTE" = "true" ] || exit 0
repo=$(cd "$(dirname "$0")/.." && pwd) || exit 0
log=/tmp/framefairy-webkit.log
lock=/tmp/framefairy-webkit.lock
exec 9>>"$lock" || exit 0
flock -n 9 || exit 0

version=$(sed -n 's/^ *"playwright": "\([0-9.]*\)",*$/\1/p' "$repo/frontend/package.json")
[ -n "$version" ] || exit 0

# The install holds the lock on 9 until it ends.
nohup setsid bash -c '
	npx --yes "playwright@$1" install --with-deps webkit &&
		echo "webkit ready" || echo "webkit failed"
' _ "$version" >"$log" 2>&1 </dev/null &
echo "Installing Playwright's WebKit in the background, see $log."
