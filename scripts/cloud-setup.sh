#!/bin/bash
# Setup script for the framefairy cloud environment at claude.ai/code. Paste this
# file's content into the environment's "Setup script" field. It runs as root
# on Ubuntu 24.04 before a session starts, and its result is cached.
#
# It installs, side by side:
# - Go 1.27, downloaded from the Go module proxy, so no Go needs to be there
# - ffmpeg with libass and libx264, for the tests that render
# - GTK 4 and WebKitGTK 6, for compiling the app
# - the engineering skills from the two plugins Tim added on claude.ai, which
#   do not reach cloud sessions by themselves, as skills Claude loads
#
# A failed step never stops the session. Its log stays in /tmp, and running
# this script again inside a session finishes the job.

GO_VERSION=1.27.1

# Each line is a repository on GitHub and the commit its skills are taken
# from. A skill is instructions Claude follows with push rights to this
# repository, so a new version comes in by changing the commit here, where it
# can be read first, and never by itself.
SKILL_SOURCES=(
	"samber/cc-skills-golang 19a0626ae8565d27a7b7bdf59d8d99d94d7e284c"
	"addyosmani/agent-skills 2686b620fc1fed2e8f60c704839c766b8594c6b6"
)

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

# Every skill goes straight into ~/.claude/skills, the one level Claude Code
# looks in. Some skills link to a folder of shared notes at the root of their
# repository, as ../../references, so that folder goes to ~/.claude/references,
# where the same link finds it. What this script put there is kept in a list,
# so a skill that a newer commit no longer has is removed rather than left
# behind, and skills from anywhere else are never touched.
install_skills() {
	local base="${HOME:-/root}/.claude"
	local list="$base/skills/.framefairy-skills"
	mkdir -p "$base/skills" || return 1
	local tmp
	tmp=$(mktemp -d)
	local entry repo commit
	for entry in "${SKILL_SOURCES[@]}"; do
		read -r repo commit <<<"$entry"
		local src="$tmp/${repo//\//-}"
		git init -q "$src" &&
			git -C "$src" fetch -q --depth 1 "https://github.com/$repo.git" "$commit" &&
			git -C "$src" checkout -q FETCH_HEAD || return 1
		echo "$repo at $commit: $(find "$src/skills" -mindepth 2 -maxdepth 2 -name SKILL.md | wc -l) skills"
	done
	if [ -f "$list" ]; then
		local path
		while read -r path; do
			[ -n "$path" ] && rm -rf "${base:?}/$path"
		done <"$list"
	fi
	: >"$list"
	local skill name
	for skill in "$tmp"/*/skills/*/; do
		[ -f "$skill/SKILL.md" ] || continue
		name=$(basename "$skill")
		cp -r "$skill" "$base/skills/$name" || return 1
		echo "skills/$name" >>"$list"
	done
	local notes copied=0
	for notes in "$tmp"/*/references/; do
		[ -d "$notes" ] || continue
		mkdir -p "$base/references" && cp -r "$notes". "$base/references/" || return 1
		copied=1
	done
	if [ "$copied" = 1 ]; then
		echo "references" >>"$list"
	fi
	rm -rf "$tmp"
}

install_go >/tmp/framefairy-setup-go.log 2>&1 &
go_job=$!
install_packages >/tmp/framefairy-setup-packages.log 2>&1 &
packages_job=$!
install_skills >/tmp/framefairy-setup-skills.log 2>&1 &
skills_job=$!

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
if wait "$skills_job"; then
	log "skills ready: $(grep -c "^skills/" "${HOME:-/root}/.claude/skills/.framefairy-skills")"
else
	log "skills failed, see /tmp/framefairy-setup-skills.log:"
	tail -n 20 /tmp/framefairy-setup-skills.log
fi
exit 0
