#!/bin/sh
# Checks what this branch changed, and only that, before a push.
#
#   make changed              what the branch changed against origin/main,
#                             committed or not
#   sh scripts/changed.sh --plan
#                             say what would run, and run nothing
#   sh scripts/changed.sh --fuzz-targets
#                             name the fuzz targets a change reaches, and
#                             run nothing
#
# make test runs everything, which is ten minutes, most of it the engine's
# tests under the race detector and ten thousand executions of every fuzz
# target. Nearly every change reaches a small part of that. This works out
# which part, the way .github/ci-filters.yml does for CI, and runs it:
#
#   Go code         gofmt on the files, then go vet and the tests under the
#                   race detector for the packages that changed and every
#                   package that imports them, and their fuzz targets,
#                   see fuzz_targets. go.mod or go.sum is every package
#                   and every target. A changed test also runs the
#                   tests of internal/offdisk and internal/ffmpegtest,
#                   which read every test file, and make lint, which
#                   reads them for waits on the clock. The decoder and
#                   scripts/build-ffmpeg.sh reach the tests that run the
#                   decoder, and the hyphenation files reach notices.
#   frontend/       make interface
#   the linters' settings and scripts/lint-test.sh
#                   that test, which proves them
#   Makefile, a script, a workflow
#                   what that file is for: make for the build, the rules
#                   tests for the rules, a read by its own shell for a
#                   script, a read of the YAML for a workflow. The
#                   Makefile and ci.yml decide how the Go tests run, so
#                   they are every package and every target too
#   docs            nothing, but docs/GUI-PLAN.md runs the check that it
#                   is history and left alone, scripts/plan-frozen.sh
#
# Anything it does not know runs the build, so a file nobody thought of is
# checked rather than skipped. On a pull request CI runs the Go packages
# --plan names and fuzzes those --fuzz-targets names, comparing with main,
# see ci.yml. A push to main still runs everything.
#
# BASE= names another commit to compare with. CHANGED= hands it the list of
# files instead of asking git, which is how scripts/changed-test.sh checks
# the rules.
set -eu

GO=${GO:-go}
MAKE=${MAKE:-make}
FUZZTIME=${FUZZTIME:-10000x}
plan_only=0
[ "${1:-}" = "--plan" ] && plan_only=1

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

say() { echo "$*" >&2; }

# What changed: the branch's commits since it left main, what is changed
# and not committed, and new files git does not know yet.
if [ -n "${CHANGED+x}" ]; then
	changed=$CHANGED
else
	base=${BASE:-}
	if [ -z "$base" ]; then
		base=$(git merge-base HEAD origin/main 2>/dev/null || git merge-base HEAD main 2>/dev/null || echo HEAD)
	fi
	changed=$({
		# Without renames, so a file moved out of a package is seen at
		# the place it left as well.
		git diff --name-only --no-renames "$base" 2>/dev/null
		git ls-files --others --exclude-standard 2>/dev/null
	} | sort -u)
fi

if [ -z "$changed" ]; then
	say "nothing changed against main, so nothing runs"
	exit 0
fi

# Go and the modules first, the same check make does before it builds,
# so a go.mod that changed is resolved before anything reads it. What
# they say goes to stderr, so a tool that is missing is named, and the
# plan on stdout stays the plan.
[ -z "${CHANGED+x}" ] && $MAKE -s --no-print-directory toolchain modules >&2

# Every package of the module, as a directory relative to the top.
module=$($GO list -m)
packages=$($GO list -f '{{.Dir}}' ./... | sed "s|^$root/\{0,1\}||; s|^$|.|")

# The package a file belongs to: the nearest directory above it that is
# a package. A file in testdata/, or one a package embeds, belongs to the
# package it sits in.
package_of() {
	dir=$(dirname -- "$1")
	while :; do
		if printf '%s\n' "$packages" | grep -qxF "$dir"; then
			if [ "$dir" = . ]; then echo "$module"; else echo "$module/$dir"; fi
			return
		fi
		[ "$dir" = . ] && return
		dir=$(dirname -- "$dir")
	done
}

# read_by_tests adds the package of every Go test that reads a file that
# is not Go, the way the bindings test reads api.ts and the notices test
# reads the build scripts. A test is found by the path it reads the file
# by, "../frontend/..." in quotes, so a file a test only names in a
# comment does not count.
read_by_tests() {
	for t in $(grep -rlF --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build --exclude-dir=.git -- "../$1\"" . 2>/dev/null); do
		gopkgs="$gopkgs $(package_of "${t#./}")"
	done
}

# runs_the_decoder adds the package of every Go test that runs the
# episode's decoder as a program, through FRAMEFAIRY_FRAMES. They do not
# import it, so the imports never lead to them.
runs_the_decoder() {
	for t in $(grep -rlF --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build --exclude-dir=.git FRAMEFAIRY_FRAMES . 2>/dev/null); do
		gopkgs="$gopkgs $(package_of "${t#./}")"
	done
}

gopkgs=""
alone=""
gofiles=""
all_go=0
interface=0
walks=0
build=0
rules=0
changed_rules=0
build_rules=0
moved_rules=0
plan_rules=0
lint=0
lint_rules=0
workflows=""
shells=""

for file in $changed; do
	case $file in
	# The plan is history, and the check that keeps it so runs on it.
	docs/GUI-PLAN.md) plan_rules=1 ;;
	*.md | docs/* | LICENSE | .claude/* | .vscode/*) ;;
	# The linters that keep tests off the clock, and the test that proves
	# them. make interface runs ESLint, and the packages are ESLint's too.
	.golangci.yml | scripts/lint-test.sh | scripts/testdata/lint/*)
		lint=1
		lint_rules=1
		;;
	frontend/eslint.config.js | frontend/eslint-suppressions.json)
		interface=1
		lint_rules=1
		;;
	frontend/package.json | frontend/package-lock.json)
		interface=1
		walks=1
		lint_rules=1
		read_by_tests "$file"
		;;
	frontend/preview/walks/* | frontend/preview/wails-bridge.ts | frontend/preview/bridge.config.ts | frontend/preview/vite.config.ts)
		interface=1
		walks=1
		read_by_tests "$file"
		;;
	frontend/preview/*)
		interface=1
		read_by_tests "$file"
		;;
	frontend/*)
		interface=1
		walks=1
		read_by_tests "$file"
		;;
	go.mod | go.sum) all_go=1 ;;
	# make unit and the job linux decide how the Go tests run, the
	# flags, the PATH and the shards, so a change to either runs them all.
	Makefile)
		build=1
		all_go=1
		;;
	.github/ci-filters.yml) rules=1 ;;
	scripts/ci-filters-test.sh)
		rules=1
		shells="$shells $file"
		;;
	scripts/changed.sh | scripts/changed-test.sh)
		changed_rules=1
		shells="$shells $file"
		;;
	scripts/needs-build.sh | scripts/needs-build-test.sh)
		build_rules=1
		shells="$shells $file"
		;;
	scripts/main-moved.sh | scripts/main-moved-test.sh)
		moved_rules=1
		shells="$shells $file"
		;;
	scripts/plan-frozen.sh | scripts/plan-frozen-test.sh)
		plan_rules=1
		shells="$shells $file"
		;;
	# The Go jobs run these, so they decide how the tests run and what
	# is fuzzed, like the Makefile.
	scripts/fuzz.sh | scripts/ci-packages.sh)
		shells="$shells $file"
		build=1
		all_go=1
		;;
	*.sh)
		shells="$shells $file"
		build=1
		# The notices test reads the build scripts for the versions they
		# pin, and a raised version passed here and failed in CI.
		read_by_tests "$file"
		# The decoder is built against the ffmpeg this builds.
		[ "$file" = scripts/build-ffmpeg.sh ] && runs_the_decoder
		;;
	.github/workflows/*)
		workflows="$workflows $file"
		# The CI workflow runs the rules, so a change to it runs them too,
		# and the builds and publish workflows are what the rule for a
		# build is for.
		case $file in
		*/ci.yml)
			rules=1
			all_go=1
			;;
		*/builds.yml | */publish.yml) build_rules=1 ;;
		*/main-moved.yml) moved_rules=1 ;;
		esac
		;;
	# Every Linux job of CI runs in the CI image, so a change to it is a
	# change to the machine every test runs on.
	.github/ci-image/*)
		build=1
		all_go=1
		;;
	.github/*) ;;
	*)
		pkg=$(package_of "$file")
		if [ -n "$pkg" ]; then
			gopkgs="$gopkgs $pkg"
			case $file in *.go) [ -f "$file" ] && gofiles="$gofiles $file" ;; esac
			case $file in
			# internal/offdisk and internal/ffmpegtest have a test that
			# reads every test file, so a changed test is what reaches
			# them, and make lint too. Their own tests and no more: the
			# engine's tests import ffmpegtest, and a changed test is no
			# change to it.
			*_test.go)
				alone="$alone $module/internal/offdisk $module/internal/ffmpegtest"
				lint=1
				;;
			cmd/framefairy-frames/*) runs_the_decoder ;;
			# The notices test counts these by a path it builds, which
			# read_by_tests cannot find.
			engine/hyphenation/*) gopkgs="$gopkgs $module/notices" ;;
			esac
		else
			build=1
		fi
		;;
	esac
done

# The packages a change reaches: the ones that changed and every package
# whose code or tests import them, however far down.
affected=""
if [ "$all_go" = 1 ]; then
	affected=$($GO list ./...)
elif [ -n "$gopkgs" ]; then
	gopkgs=$(printf '%s\n' $gopkgs | sort -u)
	affected=$($GO list -test -f '{{.ImportPath}} {{join .Deps " "}}' ./... |
		while read -r pkg deps; do
			# A test variant reads "p [p.test]" and p.test is its binary.
			# Each pattern opens with "(" because the bash 3.2 of macOS
			# reads a lone ")" inside $( ) as its end.
			case $pkg in (*.test) continue ;; esac
			pkg=${pkg%% *}
			# An external test package, x_test, is tested as x.
			pkg=${pkg%_test}
			for c in $gopkgs; do
				case " $pkg $deps " in (*" $c "*)
					echo "$pkg"
					break
					;;
				esac
			done
		done | sort -u)
fi
[ -n "$alone" ] && affected=$(printf '%s\n' $affected $alone | sort -u)

# What will run, said first, so a run that is less than expected is seen
# before it is trusted.
# The walks drive the interface against the app's Go side, so whatever
# reaches the app reaches them, the engine among it.
for p in $affected; do
	[ "$p" = "$module/cmd/framefairy-app" ] && walks=1
done

[ -n "$gofiles" ] && say "gofmt:     $(echo $gofiles)"
if [ -n "$affected" ]; then
	short=""
	for p in $affected; do
		case $p in "$module") p=. ;; "$module"/*) p=${p#"$module"/} ;; esac
		short="$short $p"
	done
	say "go test:  $short"
fi
[ "$interface" = 1 ] && say "interface: make interface"
[ "$walks" = 1 ] && say "walks:     make walks"
[ "$build" = 1 ] && say "build:     make"
[ "$rules" = 1 ] && say "rules:     scripts/ci-filters-test.sh"
[ "$changed_rules" = 1 ] && say "rules:     scripts/changed-test.sh"
[ "$build_rules" = 1 ] && say "rules:     scripts/needs-build-test.sh"
[ "$moved_rules" = 1 ] && say "rules:     scripts/main-moved-test.sh"
[ "$plan_rules" = 1 ] && say "rules:     scripts/plan-frozen-test.sh, scripts/plan-frozen.sh"
[ "$lint" = 1 ] && say "lint:      make lint"
[ "$lint_rules" = 1 ] && say "rules:     scripts/lint-test.sh"
[ -n "$shells" ] && say "scripts:   read by the shell each names$shells"
[ -n "$workflows" ] && say "workflows:$workflows"
if [ -z "$gofiles$affected$shells$workflows" ] && [ "$interface$walks$build$rules$changed_rules$build_rules$moved_rules$plan_rules$lint$lint_rules" = 0000000000 ]; then
	say "only docs changed, so nothing runs"
fi

if [ "$plan_only" = 1 ]; then
	# One word per line for the tests: what kind, then what.
	for p in $affected; do echo "go $p"; done
	[ "$interface" = 1 ] && echo interface
	[ "$walks" = 1 ] && echo walks
	[ "$build" = 1 ] && echo build
	[ "$rules" = 1 ] && echo rules
	[ "$changed_rules" = 1 ] && echo changed-rules
	[ "$build_rules" = 1 ] && echo build-rules
	[ "$moved_rules" = 1 ] && echo moved-rules
	[ "$plan_rules" = 1 ] && echo plan-rules
	[ "$lint" = 1 ] && echo lint
	[ "$lint_rules" = 1 ] && echo lint-rules
	for s in $shells; do echo "script $s"; done
	for w in $workflows; do echo "workflow $w"; done
	exit 0
fi

# The fuzz targets of the packages a change reaches, by name on one line.
# It once ran each target's seeds with coverage and fuzzed only those that
# went through a changed file. For a change to engine/clips.go that took
# 122 s, where fuzzing every target of those packages took 46 s, so a
# package that is tested is fuzzed whole.
fuzz_targets() {
	all=$(sh scripts/fuzz.sh --list)
	targets=""
	for pkg in $affected; do
		dir=${pkg#"$module"}
		dir=${dir#/}
		[ -z "$dir" ] && dir=.
		targets="$targets $(printf '%s\n' "$all" | awk -v d="./$dir" '$1 == d { print $2 }')"
	done
	echo $targets
}

# --fuzz-targets says them and stops, which is how CI hands them to make
# fuzz on a pull request.
if [ "${1:-}" = "--fuzz-targets" ]; then
	targets=$(fuzz_targets)
	echo "$targets"
	exit 0
fi

if [ -n "$gofiles" ]; then
	unformatted=$(gofmt -l $gofiles)
	if [ -n "$unformatted" ]; then
		echo "FAIL	gofmt, run gofmt -w on:"
		echo "$unformatted"
		exit 1
	fi
	printf 'ok  \tgofmt\n'
fi

if [ -n "$affected" ]; then
	$GO vet $affected
	printf 'ok  \tgo vet\n'
	# The same flags and the same PATH make unit passes, so a test here is
	# the test there. The walks below keep the PATH they came with, see
	# the Makefile.
	path=$PATH
	PATH=${TOOLS_FIRST:-$PATH}
	$GO test -race -ldflags "${LDFLAGS:-}" $affected
	targets=$(fuzz_targets)
	if [ -n "$targets" ]; then
		GO=$GO FUZZTIME=$FUZZTIME FUZZTARGETS="$targets" sh scripts/fuzz.sh
	fi
	PATH=$path
fi

[ "$interface" = 1 ] && $MAKE -s --no-print-directory interface
[ "$lint" = 1 ] && $MAKE -s --no-print-directory lint
[ "$walks" = 1 ] && $MAKE -s --no-print-directory walks

# A script is read by the shell its first line names, so a bash script is not
# failed for being bash. One the branch removed has nothing to read.
for s in $shells; do
	[ -f "$s" ] || continue
	case $(head -n 1 "$s") in
	*bash*) bash -n "$s" ;;
	*) sh -n "$s" ;;
	esac
done
[ -n "$shells" ] && printf 'ok  \tscripts read\n'

for w in $workflows; do
	if [ -f "$w" ] && command -v python3 >/dev/null 2>&1 &&
		python3 -c 'import yaml' 2>/dev/null; then
		python3 -c 'import sys, yaml; yaml.safe_load(open(sys.argv[1]))' "$w"
		printf 'ok  \t%s reads\n' "$w"
	fi
done

[ -n "$workflows" ] && sh scripts/pins.sh
[ "$rules" = 1 ] && sh scripts/ci-filters-test.sh
[ "$changed_rules" = 1 ] && sh scripts/changed-test.sh
[ "$build_rules" = 1 ] && sh scripts/needs-build-test.sh
[ "$moved_rules" = 1 ] && sh scripts/main-moved-test.sh
if [ "$lint_rules" = 1 ]; then
	$MAKE -s --no-print-directory golangci-lint frontend/node_modules/.package-lock.json
	sh scripts/lint-test.sh
fi
if [ "$plan_rules" = 1 ]; then
	sh scripts/plan-frozen-test.sh
	sh scripts/plan-frozen.sh ${base:-}
fi
# Away from a Mac nothing builds the tools we ship, and make stops without
# them, so there the build proves the programs build and says it goes
# without, the way CI's Linux job does. On a Mac make builds them.
if [ "$build" = 1 ]; then
	if [ "$(uname -s 2>/dev/null)" = Darwin ]; then
		$MAKE -s --no-print-directory
	else
		$MAKE -s --no-print-directory NOTOOLS=1
	fi
fi

exit 0
