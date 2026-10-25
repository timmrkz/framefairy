# The machine every Linux job of CI runs on, the walks and the Go tests
# among them, tools.yml alone left out, see docs/BUILD.md: Ubuntu
# 24.04, the runners' own system, with every system package they use and
# the Chromium the pinned Playwright drives, with its libraries, already
# installed. A job that runs in it downloads nothing from the Ubuntu
# mirror, which on some days gave the runners 18 kB a second and held a
# walks job for twelve minutes, #462.
#
# .github/workflows/ci-image.yml builds it and keeps it in GitHub's
# container registry, tagged by a sum of this file, of
# scripts/ci-packages.sh and of the Playwright version in
# frontend/package-lock.json. So a change to any of them makes a new image
# and an unchanged one is pulled as it is. A file this one comes to read
# must join that sum and the job's sparse checkout too, or a change to it
# reuses the old image. Go and Node are not in it: the jobs take them from
# go.mod and setup-node as every other job does.
#
# Both images below are pinned to a digest, so the same file always means
# the same image. Moving them is a change to this file, which builds anew,
# and Dependabot moves them once a week, see .github/dependabot.yml.
FROM node:22.23.4-bookworm-slim@sha256:de170b39439eb0e5a3444ad7d60b015614a381e10fecae2e30ec9e735122f38d AS node

FROM ubuntu:24.04@sha256:68ee22004a9109b44973313b02ff7b1de250f1de0f5ecb628d73f579aaff33ac

# What a runner has and an Ubuntu image does not: a compiler, git for the
# checkout, ps for scripts/leftovers.sh, which names what the tests left
# running, curl, xz and zstd, the last for actions/cache, which keeps
# what it saves with zstd when it finds it, and finds an entry saved one
# way only that way. Then what scripts/ci-packages.sh installs for the Go
# side, GTK and WebKit, and what the walks and the tests use beside it:
# ffmpeg, which makes the bridge's episode, meson, ninja and nasm, which
# build the ffmpeg we ship when its cache is missed, and patchelf, which
# make looks for before it builds, and python3-yaml, which the job build
# reads every workflow with when one changed, #542. The package lists go
# after, and so does the script's setting for the downloads, which a job
# never makes.
ENV LANG=C.UTF-8
RUN --mount=type=bind,source=scripts/ci-packages.sh,target=/tmp/ci-packages.sh \
	sh /tmp/ci-packages.sh \
		build-essential ca-certificates curl git procps xz-utils zstd \
		ffmpeg meson ninja-build nasm patchelf python3-yaml && \
	rm -rf /etc/apt/apt.conf.d/99ci-retries /var/lib/apt/lists/*

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
LABEL org.opencontainers.image.description="The machine framefairy's Linux CI jobs run on"
