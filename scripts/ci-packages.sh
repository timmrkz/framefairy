#!/bin/sh
# Installs the system packages the Go side needs to compile: GTK and
# WebKit for the app, and pkg-config to find them. Every Go job compiles
# the app's package, the fuzzing too, because one of its targets is
# there. Whatever else is wanted is named after it:
#
#   sh scripts/ci-packages.sh ffmpeg patchelf
#
# Only the CI image runs it, .github/ci-image/Dockerfile, as root, when
# the image is built. Every Linux job of CI runs in that image, so none
# of them asks the Ubuntu mirror, which crawls on some days: a fuzz job
# once spent fifteen minutes on it, 13.6 MB of ffmpeg's speech synthesis
# library arriving at 100 kB a second.
set -eu

# man-db rebuilds its index after every install that brings a manual page,
# and nobody reads a manual page in CI. It is most of the time apt spends
# after the downloads.
rm -f /var/lib/man-db/auto-update

# A download that stalls is tried again rather than waited on.
printf '%s\n' \
	'Acquire::Retries "3";' \
	'Acquire::http::Timeout "20";' \
	>/etc/apt/apt.conf.d/99ci-retries
apt-get -q update
apt-get -q install -y --no-install-recommends \
	pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev "$@"
