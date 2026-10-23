# The machine the walks run on in CI: Ubuntu 24.04, the runners' own
# system, with every system package they use and the Chromium the pinned
# Playwright drives, with its libraries, already installed. A job that runs
# in it downloads nothing from the Ubuntu mirror, which on some days gave
# the runners 18 kB a second and held a walks job for twelve minutes, #462.
#
# The job image in .github/workflows/ci.yml builds it and keeps it in
# GitHub's container registry, tagged by a sum of this file, of
# scripts/ci-packages.sh and of the Playwright version in
# frontend/package-lock.json. So a change to any of them makes a new image
# and an unchanged one is pulled as it is. Go and Node are not in it: the
# jobs take them from go.mod and setup-node as every other job does.
#
# Both images below are pinned to a digest, so the same file always means
# the same image. Moving them is a change to this file, which builds anew.
FROM node:22.23.4-bookworm-slim@sha256:de170b39439eb0e5a3444ad7d60b015614a381e10fecae2e30ec9e735122f38d AS node

FROM ubuntu:24.04@sha256:68ee22004a9109b44973313b02ff7b1de250f1de0f5ecb628d73f579aaff33ac

# What a runner has and an Ubuntu image does not: a compiler, git for the
# checkout, curl, xz and zstd, the last for actions/cache, which keeps
# what it saves with zstd when it finds it, and finds an entry saved one
# way only that way. Then what scripts/ci-packages.sh installs for the Go
# side, GTK and WebKit, and what the walks and the tests use beside it:
# ffmpeg, which makes the bridge's episode, meson, ninja and nasm, which
# build the ffmpeg we ship when its cache is missed, and patchelf, which
# make looks for before it builds. The script's cache of downloads is
# for a runner, so it goes, and so do the package lists.
ENV LANG=C.UTF-8
RUN --mount=type=bind,source=scripts/ci-packages.sh,target=/tmp/ci-packages.sh \
	CACHE=/tmp/apt sh /tmp/ci-packages.sh \
		build-essential ca-certificates curl git xz-utils zstd \
		ffmpeg meson ninja-build nasm patchelf && \
	rm -rf /tmp/apt /etc/apt/apt.conf.d/99ci-cache /var/lib/apt/lists/*

# Chromium for the Playwright the interface pins, with the libraries and
# fonts it needs, in the place Playwright's own images keep it. Only its
# headless shell, which is the one a headless launch runs, as the walks'
# is: the full browser beside it would add some 170 MB to every pull and
# never start. Node is lent from the image above for this step only.
ARG PLAYWRIGHT
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN --mount=type=bind,from=node,source=/usr/local,target=/opt/node \
	test -n "$PLAYWRIGHT" && \
	PATH="/opt/node/bin:$PATH" npx --yes "playwright@$PLAYWRIGHT" install --with-deps --only-shell chromium && \
	rm -rf /root/.npm /var/lib/apt/lists/*

LABEL org.opencontainers.image.source=https://github.com/timmrkz/framefairy
LABEL org.opencontainers.image.description="The machine framefairy's walks run on in CI"
