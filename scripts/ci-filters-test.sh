#!/bin/sh
# Checks .github/ci-filters.yml, which decides which jobs a pull request
# starts, because a mistake in it is silent: CI goes green having run less
# than it should. Every case below is one a pull request really arrives as.
#
#   sh scripts/ci-filters-test.sh
#
# It matches the way dorny/paths-filter v4.0.3 does, with picomatch from
# the interface's packages, so it needs no network. The lines mirrored
# from the action's src/filter.ts: 15 (picomatch with dot), 133 to 148
# (some-with-excludes: a negated pattern that excludes the file ends it,
# else any other pattern that matches takes it) and 199 to 212 (each
# pattern its own matcher, negated when picomatch says so). The filters
# are listed in the order of the file, as the action's changes output is.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
[ -f "$root/frontend/node_modules/picomatch/package.json" ] ||
	make -s --no-print-directory -C "$root" frontend/node_modules/.package-lock.json >&2
fail=0

# check FILES WANT: changing FILES starts the filters in WANT and no other.
check() {
	got=$(node -e '
		const fs = require("fs")
		const pm = require(process.argv[1] + "/frontend/node_modules/picomatch")
		const filters = {}
		let name
		for (const line of fs.readFileSync(process.argv[1] + "/.github/ci-filters.yml", "utf8").split("\n")) {
			let m
			if (/^\s*(#.*)?$/.test(line)) continue
			else if ((m = line.match(/^(\w+):$/))) filters[name = m[1]] = []
			else if ((m = line.match(/^  - \x27([^\x27]+)\x27$/))) filters[name].push(pm(m[1], {dot: true}, true))
			else throw new Error("ci-filters.yml: cannot read " + line)
		}
		const matches = (rules, file) => {
			let included = false
			for (const rule of rules) {
				if (rule.state.negated) { if (!rule(file)) return false }
				else if (rule(file)) included = true
			}
			return included
		}
		const files = process.argv.slice(2)
		console.log(Object.keys(filters).filter(k => files.some(f => matches(filters[k], f))).join(" "))
	' "$root" $1)
	if [ "$got" = "$2" ]; then
		printf 'ok  \t%-52s %s\n' "$1" "${2:-nothing}"
	else
		printf 'FAIL\t%-52s %s, wanted %s\n' "$1" "${got:-nothing}" "${2:-nothing}"
		fail=1
	fi
}

# Docs cannot break a build, so nothing starts. The job build always does,
# for the checks of the rules.
check "README.md docs/APP.md frontend/README.md LICENSE .claude/x.json" ""

# No Go test builds the interface, so the interface on its own does not
# start the Go tests.
check "frontend/src/App.svelte" "app interface"
check "frontend/src/routes/Updates.svelte" "app interface"
# Except the files of it a Go test reads.
check "frontend/src/lib/api.ts" "app go interface"
check "frontend/src/lib/suggest.cases.json" "app go interface"
check "frontend/src/lib/flow.ts" "app interface"

# The other way round.
check "engine/edit.go" "app go"
check "engine/edit_test.go go.sum" "app go"
check "go.mod" "app go"

# Whatever decides how the project is built starts everything.
check "Makefile" "app go interface"
check "scripts/build-ffmpeg.sh" "app go interface"
check ".github/workflows/ci.yml" "app go interface workflows"
check ".github/ci-filters.yml" "app go interface"
check ".golangci.yml" "app go interface"
check "frontend/package.json" "app interface"

# The rules that no job runs but their own check in build start nothing.
# Issue #322: a pull request that changed only them ran every job.
check "scripts/main-moved.sh scripts/main-moved-test.sh docs/BUILD.md" ""
check "scripts/plan-frozen.sh scripts/needs-build-test.sh" ""
check "scripts/changed.sh scripts/changed-test.sh" "changed"
check "scripts/needs-build.sh engine/edit.go" "app go"

# A file nobody thought of starts everything.
check "some/new/thing.py" "app go interface"
check ".gitignore" "app go interface"

# Every file of the interface a Go test reads starts the Go tests, found
# by the path the test reads it by, so a new one needs no case here.
for f in $(grep -rhoE --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build '"(\.\./)+frontend/[^"]*"' "$root" |
	tr -d '"' | sed 's#^\(\.\./\)*##' | sort -u); do
	[ -f "$root/$f" ] && check "$f" "app go interface"
done

[ "$fail" = 0 ] && echo "ok  	the CI filters behave"
exit $fail
