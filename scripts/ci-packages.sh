#!/bin/sh
# Installs the system packages a Linux CI job needs to compile the Go side:
# GTK and WebKit for the app, and pkg-config to find them. Every Go job
# compiles the app's package, the fuzzing too, because one of its targets
# is there. Whatever else a job uses is named after it, and only there:
#
#   sh scripts/ci-packages.sh            # the fuzzing, govulncheck
#   sh scripts/ci-packages.sh patchelf   # the build, which carries the speech library
#   sh scripts/ci-packages.sh ffmpeg     # the tests, some of which render
#
# The packages are downloaded into CACHE, which the workflow keeps from one
# run to the next, so a run downloads only what changed since. The mirror
# the runners use is fast most days and crawls on some: a fuzz job once
# spent fifteen minutes on it, 13.6 MB of ffmpeg's speech synthesis library
# arriving at 100 kB a second, ffmpeg being a package fuzzing never runs.
set -eu

CACHE=${CACHE:-$HOME/.cache/apt}
mkdir -p "$CACHE/partial"

# man-db rebuilds its index after every install that brings a manual page,
# and nobody reads a manual page on a runner. It is most of the time apt
# spends after the downloads.
sudo rm -f /var/lib/man-db/auto-update

# A download that stalls is tried again rather than waited on. The cache
# is in the runner's home, which apt's own user cannot reach, so apt
# downloads as root, as it would anyway, without saying so each time.
apt="sudo apt-get -q -o Acquire::Retries=3 -o Acquire::http::Timeout=20 -o Dir::Cache::Archives=$CACHE -o APT::Keep-Downloaded-Packages=true -o APT::Sandbox::User=root"
$apt update
$apt install -y --no-install-recommends \
	pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev "$@"

# The cache is saved by the runner's user, and apt left it root's.
sudo rm -rf "$CACHE/partial" "$CACHE/lock"
sudo chown -R "$(id -u):$(id -g)" "$CACHE"
