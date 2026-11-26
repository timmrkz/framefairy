# The machine every Linux job of CI runs on, the walks and the Go tests
# among them, tools.yml alone left out, see docs/BUILD.md: Ubuntu
# 24.04, the runners' own system, with every system package they use and
# the Chromium and WebKit the pinned Playwright drives, with their
# libraries, already installed, in three images of this one file. A job
# that runs in one downloads nothing from the Ubuntu mirror, which on some
# days gave the runners 18 kB a second and held a walks job for twelve
# minutes, #462.
#
# .github/workflows/ci-image.yml builds it and keeps it in GitHub's
# container registry, tagged by a sum of this file and of the Playwright
# version in frontend/package-lock.json. So a change to either makes a new
# image and an unchanged one is pulled as it is. A file this one comes to
# read must join that sum and the job's sparse checkout too, or a change
# to it reuses the old image. Go is in it, the version go.mod names, so a
# job in it downloads none, which took setup-go about 6 s of every job.
# .github/actions/prepare takes this Go when it is the one go.mod names,
# and sets up the one it names otherwise. Node is not in it: setup-node
# finds it in the runner's tool cache at once.
#
# The images below are pinned to a digest, so the same file always means
# the same image. Moving them is a change to this file, which builds anew,
# and Dependabot moves them once a week, see .github/dependabot.yml.
FROM node:22.23.5-bookworm-slim@sha256:e9f565d32859f0cbfb52125eafe8be39390f24dac302dc3cdf566e8ba3ab21ee AS node

FROM golang:1.27.4-bookworm@sha256:4fa7c968ba93f1635670522dbb01a2c78150443e1385746924566079f36570ea AS go

FROM ubuntu:24.04@sha256:68ee22004a9109b44973313b02ff7b1de250f1de0f5ecb628d73f579aaff33ac AS base

# Every package comes from Ubuntu's archive of the day the image is
# built. The image is built once for each sum of this file and kept in
# the registry, so what the jobs run stays the same until this file
# changes, and the base images' digests pin the rest. Dependabot moves
# them once a week, so the image is built again then with the security
# fixes of that day. Ubuntu's snapshot service, which pinned the packages
# to a day before, #591, answered 500 to 503 for most of an hour on 11
# November and held every change to this file.

# What a runner has and an Ubuntu image does not: a compiler, git for the
# checkout, ps, curl and the certificates it checks https against, xz and
# zstd, the last for actions/cache, which keeps what it saves with zstd
# when it finds it. GTK and WebKit, which every Go
# job compiles the app's package against, the fuzzing too, since one of
# its targets is there, with pkg-config to find them. And what the walks
# and the tests use beside it: ffmpeg, which makes the bridge's episode,
# meson, ninja and nasm, which build the ffmpeg we ship when its cache is
# missed, and patchelf, which make looks for before it builds. man-db
# would index every manual page installed, and nobody reads one here. A
# download that stalls is tried again rather than waited on, and a list
# the archive did not hand over stops the build there: apt only warns of
# it, and then finds none of the packages, which hid the 503 behind them.
ENV LANG=C.UTF-8
RUN rm -f /var/lib/man-db/auto-update && \
	printf '%s\n' 'Acquire::Retries "3";' 'Acquire::http::Timeout "20";' >/etc/apt/apt.conf.d/99ci-retries && \
	apt-get -q update --error-on=any && \
	apt-get -q install -y --no-install-recommends \
		pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev \
		build-essential ca-certificates curl git procps xz-utils zstd \
		ffmpeg meson ninja-build nasm patchelf && \
	rm -rf /etc/apt/apt.conf.d/99ci-retries /var/lib/apt/lists/*

# Go, from the official image above, see the head of this file.
COPY --from=go /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH FRAMEFAIRY_CI_GO=/usr/local/go/bin/go

LABEL org.opencontainers.image.source=https://github.com/timmrkz/framefairy
LABEL org.opencontainers.image.description="The machine framefairy's Linux CI jobs run on"

# The walks' machine is the one above with Chromium for the Playwright
# the interface pins, with the libraries and fonts it needs, in the place
# Playwright's own images keep them. Of Chromium only its headless shell,
# which is the one a headless launch runs, as the walks' is: the full
# browser beside it would add some 170 MB to every pull and never start.
# A push walks in Chromium alone, so WebKit, the engine the app runs in on
# a Mac, is in an image of its own below, for the nightly walks, see Two
# browsers in docs/TESTING.md. With WebKit in one image for every job,
# pulling it took 73 to 80 s where 24 to 30 s had done, so the jobs that
# open no browser take the one above, #380. Node is lent from the image
# above for these steps only.
FROM base AS walks
ARG PLAYWRIGHT
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright
RUN --mount=type=bind,from=node,source=/usr/local,target=/opt/node \
	test -n "$PLAYWRIGHT" && \
	PATH="/opt/node/bin:$PATH" npx --yes "playwright@$PLAYWRIGHT" install --with-deps --only-shell chromium && \
	rm -rf /root/.npm /var/lib/apt/lists/*

FROM walks AS webkit
ARG PLAYWRIGHT
RUN --mount=type=bind,from=node,source=/usr/local,target=/opt/node \
	test -n "$PLAYWRIGHT" && \
	PATH="/opt/node/bin:$PATH" npx --yes "playwright@$PLAYWRIGHT" install --with-deps webkit && \
	rm -rf /root/.npm /var/lib/apt/lists/*
