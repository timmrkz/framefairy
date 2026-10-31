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

# Every package comes from Ubuntu's archive as it stood at this moment,
# Ubuntu's snapshot service, so the same file builds the same image
# whatever day it is built, #591. The base image's digest in the
# Dockerfile pins what Ubuntu installed, this pins what is added to it,
# and scripts/upstream.sh says when it is a month old, so the security
# fixes since come in by moving it.
SNAPSHOT=20261030T000000Z

# man-db rebuilds its index after every install that brings a manual page,
# and nobody reads a manual page in CI. It is most of the time apt spends
# after the downloads.
rm -f /var/lib/man-db/auto-update

# A download that stalls is tried again rather than waited on.
printf '%s\n' \
	'Acquire::Retries "3";' \
	'Acquire::http::Timeout "20";' \
	>/etc/apt/apt.conf.d/99ci-retries
# The snapshot is reached over https, and the base image has no
# certificates to check it with, so they alone come from the day's
# archive first.
apt-get -q update
apt-get -q install -y --no-install-recommends ca-certificates
# Kept in the image, so Playwright's install of its libraries after this
# takes them from the same snapshot.
printf 'APT::Snapshot "%s";\n' "$SNAPSHOT" >/etc/apt/apt.conf.d/99ci-snapshot
apt-get -q update
apt-get -q install -y --no-install-recommends \
	pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev "$@"
