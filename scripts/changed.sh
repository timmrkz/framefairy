#!/bin/sh
# Checks what this branch changed, and only that, before a push.
#
#   make changed              what the branch changed against origin/main,
#                             committed or not
#   sh scripts/changed.sh --plan
#                             say what would run, and run nothing
#
# make test runs everything, which is ten minutes, most of it the engine's
# tests under the race detector and ten thousand executions of every fuzz
# target. Nearly every change reaches a small part of that. This works out
# which part, the way scripts/ci-needs.sh does for CI, and runs it:
#
#   Go code         gofmt on the files, then go vet and the tests under the
#                   race detector for the packages that changed and every
#                   package that imports them, and of their fuzz targets
#                   those that go through a changed file, see
#                   fuzz_what_changed. go.mod or go.sum is every package
#                   and every target.
#   frontend/       make interface
#   Makefile, a script, a workflow
#                   what that file is for: make for the build, the rules
#                   tests for the rules, sh -n for a script, a read of the
#                   YAML for a workflow
#   docs            nothing
#
# Anything it does not know runs the build, so a file nobody thought of is
# checked rather than skipped. CI still runs everything on every pull
# request. This is for getting there with fewer round trips, not instead.
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
		git diff --name-only "$base" 2>/dev/null
		git ls-files --others --exclude-standard 2>/dev/null
	} | sort -u)
fi

if [ -z "$changed" ]; then
	say "nothing changed against main, so nothing runs"
	exit 0
fi

# Go and the modules first, the same check make does before it builds,
# so a go.mod that changed is resolved before anything reads it.
[ -z "${CHANGED+x}" ] && $MAKE -s --no-print-directory toolchain modules >/dev/null

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

gopkgs=""
gofiles=""
all_go=0
interface=0
build=0
rules=0
changed_rules=0
workflows=""
shells=""

for file in $changed; do
	case $file in
	*.md | docs/* | LICENSE | .claude/* | .vscode/*) ;;
	frontend/*) interface=1 ;;
	go.mod | go.sum) all_go=1 ;;
	Makefile) build=1 ;;
	scripts/ci-needs.sh | scripts/ci-needs-test.sh)
		rules=1
		shells="$shells $file"
		;;
	scripts/changed.sh | scripts/changed-test.sh)
		changed_rules=1
		shells="$shells $file"
		;;
	*.sh)
		shells="$shells $file"
		build=1
		;;
	.github/workflows/*)
		workflows="$workflows $file"
		# The CI workflow runs the rules, so a change to it runs them too.
		case $file in */ci.yml) rules=1 ;; esac
		;;
	.github/*) ;;
	*)
		pkg=$(package_of "$file")
		if [ -n "$pkg" ]; then
			gopkgs="$gopkgs $pkg"
			case $file in *.go) [ -f "$file" ] && gofiles="$gofiles $file" ;; esac
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
			case $pkg in *.test) continue ;; esac
			pkg=${pkg%% *}
			for c in $gopkgs; do
				case " $pkg $deps " in *" $c "*)
					echo "$pkg"
					break
					;;
				esac
			done
		done | sort -u)
fi

# What will run, said first, so a run that is less than expected is seen
# before it is trusted.
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
[ "$build" = 1 ] && say "build:     make"
[ "$rules" = 1 ] && say "rules:     scripts/ci-needs-test.sh"
[ "$changed_rules" = 1 ] && say "rules:     scripts/changed-test.sh"
[ -n "$shells" ] && say "scripts:   sh -n$shells"
[ -n "$workflows" ] && say "workflows:$workflows"
if [ -z "$gofiles$affected$shells$workflows" ] && [ "$interface$build$rules$changed_rules" = 0000 ]; then
	say "only docs changed, so nothing runs"
fi

if [ "$plan_only" = 1 ]; then
	# One word per line for the tests: what kind, then what.
	for p in $affected; do echo "go $p"; done
	[ "$interface" = 1 ] && echo interface
	[ "$build" = 1 ] && echo build
	[ "$rules" = 1 ] && echo rules
	[ "$changed_rules" = 1 ] && echo changed-rules
	for s in $shells; do echo "script $s"; done
	for w in $workflows; do echo "workflow $w"; done
	exit 0
fi

# The fuzz targets a change reaches, fuzzed, and no others. Fuzzing is the
# slow part, ten thousand executions a target, and most targets read one
# kind of input a change never goes near. A target is fuzzed when:
#
#   one of its own seeds changed, in testdata/fuzz/<target>/
#   a test file of its package changed, which may be the target itself
#   go.mod or go.sum changed, which can change anything
#   running its seeds once, with coverage, goes through a file that changed
#
# The last is the one that decides most of the time. Running the seeds is
# what go test does anyway, and takes a second. What it goes through is
# what the target is about, so a change to the renderer does not fuzz the
# plan reader, and a change to the plan reader does.
fuzz_what_changed() {
	gosources=""
	gotests=""
	for f in $changed; do
		case $f in
		*_test.go) gotests="$gotests $f" ;;
		*.go) gosources="$gosources $f" ;;
		esac
	done
	targets=""
	skipped=""
	for pkg in $affected; do
		dir=${pkg#"$module"}
		dir=${dir#/}
		[ -z "$dir" ] && dir=.
		for target in $($GO test -list 'Fuzz.*' "$pkg" 2>/dev/null | grep '^Fuzz' || true); do
			if fuzz_reaches "$pkg" "$dir" "$target"; then
				targets="$targets$pkg $target
"
			else
				skipped="$skipped $target"
			fi
		done
	done
	[ -n "$skipped" ] && printf 'skip	fuzzing%s, which no change reaches\n' "$skipped"
	if [ -n "$targets" ]; then
		printf '%s' "$targets" | GO=$GO FUZZTIME=$FUZZTIME \
			xargs -P "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)" -n 2 sh scripts/fuzz.sh --one
	fi
}

fuzz_reaches() {
	pkg=$1
	dir=$2
	target=$3
	[ "$all_go" = 1 ] && return 0
	for f in $changed; do
		case $f in "$dir"/testdata/fuzz/"$target"/*) return 0 ;; esac
	done
	for f in $gotests; do
		[ "$(dirname -- "$f")" = "$dir" ] && return 0
	done
	[ -z "$gosources" ] && return 1
	profile=$(mktemp)
	if ! $GO test -run "^$target\$" -coverpkg="$module/..." -coverprofile="$profile" "$pkg" >/dev/null 2>&1; then
		# Its seeds fail, so fuzzing it says why.
		rm -f "$profile"
		return 0
	fi
	for f in $gosources; do
		if grep "^$module/$f:" "$profile" | awk '$NF > 0 { hit = 1 } END { exit !hit }'; then
			rm -f "$profile"
			return 0
		fi
	done
	rm -f "$profile"
	return 1
}

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
	# The same flags make unit passes, so a test here is the test there.
	$GO test -race -ldflags "${LDFLAGS:-}" $affected
	fuzz_what_changed
fi

[ "$interface" = 1 ] && $MAKE -s --no-print-directory interface

for s in $shells; do
	sh -n "$s"
done
[ -n "$shells" ] && printf 'ok  \tscripts read\n'

for w in $workflows; do
	if [ -f "$w" ] && command -v python3 >/dev/null 2>&1 &&
		python3 -c 'import yaml' 2>/dev/null; then
		python3 -c 'import sys, yaml; yaml.safe_load(open(sys.argv[1]))' "$w"
		printf 'ok  \t%s reads\n' "$w"
	fi
done

[ "$rules" = 1 ] && sh scripts/ci-needs-test.sh
[ "$changed_rules" = 1 ] && sh scripts/changed-test.sh
[ "$build" = 1 ] && $MAKE -s --no-print-directory

exit 0
