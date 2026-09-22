#!/bin/bash
# Setup script for the framefairy cloud environment at claude.ai/code. Paste this
# file's content into the environment's "Setup script" field. It runs as root
# on Ubuntu 24.04 before a session starts, and its result is cached.
#
# It installs, side by side:
# - Go 1.27, downloaded from the Go module proxy, so no Go needs to be there
# - ffmpeg with libass and libx264, for the tests that render
# - GTK 4 and WebKitGTK 6, for compiling the app
#
# A failed step never stops the session. Its log stays in /tmp, and running
# this script again inside a session finishes the job.

GO_VERSION=1.27.1

log() { echo "[framefairy setup] $*"; }

install_go() {
	if /usr/local/go/bin/go version 2>/dev/null | grep -q "go$GO_VERSION "; then
		echo "Go $GO_VERSION is already there"
		return 0
	fi
	local name="v0.0.1-go$GO_VERSION.linux-amd64"
	local tmp
	tmp=$(mktemp -d)
	curl -fsSL --retry 3 -o "$tmp/go.zip" \
		"https://proxy.golang.org/golang.org/toolchain/@v/$name.zip" || return 1
	python3 -c 'import sys, zipfile; zipfile.ZipFile(sys.argv[1]).extractall(sys.argv[2])' \
		"$tmp/go.zip" "$tmp" || return 1
	local src="$tmp/golang.org/toolchain@$name"
	chmod +x "$src"/bin/* "$src"/pkg/tool/*/* || return 1
	rm -rf /usr/local/go
	mv "$src" /usr/local/go || return 1
	ln -sf /usr/local/go/bin/go /usr/local/bin/go
	ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
	rm -rf "$tmp"
	/usr/local/go/bin/go version
}

install_packages() {
	export DEBIAN_FRONTEND=noninteractive
	local apt=(apt-get -o DPkg::Lock::Timeout=180 -o Acquire::Retries=3 -q)
	# Extra package sources that come with the machine can be outside the
	# allowed network. apt then reports an error, while Ubuntu's own lists
	# still update, so only the install decides.
	"${apt[@]}" update || echo "Some package sources could not be reached, continuing"
	"${apt[@]}" install -y --no-install-recommends \
		ffmpeg patchelf pkg-config libgtk-4-dev libwebkitgtk-6.0-dev || return 1
}

install_go >/tmp/framefairy-setup-go.log 2>&1 &
go_job=$!
install_packages >/tmp/framefairy-setup-packages.log 2>&1 &
packages_job=$!

if wait "$go_job"; then
	log "Go ready: $(/usr/local/go/bin/go version)"
else
	log "Go failed, see /tmp/framefairy-setup-go.log:"
	tail -n 20 /tmp/framefairy-setup-go.log
fi
if wait "$packages_job"; then
	log "system packages ready"
else
	log "system packages failed, see /tmp/framefairy-setup-packages.log:"
	tail -n 20 /tmp/framefairy-setup-packages.log
fi
exit 0
