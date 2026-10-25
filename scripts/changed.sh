#!/bin/sh
# What a change reaches, and only that. This is the one place that
# decides it, for make changed on any machine and for every CI job on a
# pull request, so a change is checked the same way everywhere.
#
#   make changed                  every part the branch reaches against
#                                 origin/main, committed or not
#   make changed PART=unit SHARD=1/3
#                                 one part, or one share of it, which is
#                                 what each CI job runs
#   sh scripts/changed.sh --parts name the parts with work, one line, and
#                                 run nothing. CI's first job asks this,
#                                 so a job with nothing to do never
#                                 starts a machine
#   sh scripts/changed.sh --plan  say what each part would run, and run
#                                 nothing
#
# The parts are the parts of make test, and each runs through its own
# make target, so what runs here is what that target runs:
#
#   lint       make lint, the whole tree read without running it, for a
#              Go file, a script, a workflow, the Makefile, the linters'
#              settings and docs/GUI-PLAN.md
#   interface  make interface, for anything under frontend/
#   unit       make unit, for the packages that changed and every
#              package that imports them, however far down, and the
#              packages whose tests read a file that changed
#   fuzz       make fuzz, for the fuzz targets of those packages
#   walks      make walks, for the interface the app shows and for
#              anything that reaches the app's Go side
#   build      make, for anything that ships. On a Mac the build may
#              print no warning
#   outside    make outside, on a Mac in CI, for anything that ships
#
# A file it knows nothing about reaches the build, so a file nobody
# thought of is checked rather than skipped. The Makefile and ci.yml
# decide how every part runs, so they reach everything, every package
# and every fuzz target.
#
# EVERYTHING=1 takes every part, every package and every target, which is
# what a push to main runs. BASE= names the commit to compare with, which
# on a pull request in CI is the main its merge commit sits on. CHANGED=
# hands it the list of files instead of asking git, which is how
# scripts/changed-test.sh checks the rules. OS=Darwin asks what a Mac
# would run, which is how CI's first job knows whether to start one, and
# UNITSKIP names the packages make unit leaves out there. make changed
# passes both.
set -eu

GO=${GO:-go}
MAKE=${MAKE:-make}
PART=${PART:-}
SHARD=${SHARD:-}
OS=${OS:-$(uname -s 2>/dev/null || echo unknown)}
mode=run
case ${1:-} in
--plan) mode=plan ;;
--parts) mode=parts ;;
"") ;;
*)
	echo "changed.sh: --plan, --parts or nothing, not $1" >&2
	exit 2
	;;
esac

all_parts="lint interface unit fuzz walks build outside"
for p in $PART; do
	case " $all_parts " in *" $p "*) ;; *)
		echo "changed.sh: PART is one or more of $all_parts, not $p" >&2
		exit 2
		;;
	esac
done
if [ -n "$SHARD" ]; then
	case $PART in unit | walks) ;; *)
		echo "changed.sh: SHARD takes one part, unit or walks, not \"$PART\"" >&2
		exit 2
		;;
	esac
fi

# wanted PART: this run is asked for that part.
wanted() {
	[ -z "$PART" ] && return 0
	case " $PART " in *" $1 "*) return 0 ;; esac
	return 1
}

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

say() { echo "$*" >&2; }

# The interface needs no Go, and its job has none, so only a run that
# asks for nothing but the interface goes without it.
need_go=1
[ "$PART" = interface ] && need_go=0

# What changed: the branch's commits since it left main, what is changed
# and not committed, and new files git does not know yet.
base=""
if [ -n "${CHANGED+x}" ]; then
	changed=$CHANGED
elif [ "${EVERYTHING:-}" = 1 ]; then
	changed=""
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

# The modules first, the same check make does before it builds, so a
# go.mod that changed is resolved before anything reads it. Only Go is
# needed to know which packages import which, not the C compiler and the
# libraries the build checks for, which CI's first job does not have.
# What it says goes to stderr, so the plan on stdout stays the plan.
if [ "$need_go" = 1 ] && [ -z "${CHANGED+x}" ]; then
	$MAKE -s --no-print-directory modules >&2
fi

module=""
packages=""
if [ "$need_go" = 1 ]; then
	module=$($GO list -m)
	# Every package of the module, as a directory relative to the top.
	packages=$($GO list -f '{{.Dir}}' ./... | sed "s|^$root/\{0,1\}||; s|^$|.|")
fi

# The package a file belongs to: the nearest directory above it that is
# a package. A file in testdata/, or one a package embeds, belongs to the
# package it sits in.
package_of() {
	[ "$need_go" = 1 ] || return 0
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
	[ "$need_go" = 1 ] || return 0
	for t in $(grep -rlF --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build --exclude-dir=.git -- "../$1\"" . 2>/dev/null); do
		gopkgs="$gopkgs $(package_of "${t#./}")"
	done
}

# runs_the_decoder adds the package of every Go test that runs the
# episode's decoder as a program, through FRAMEFAIRY_FRAMES. They do not
# import it, so the imports never lead to them.
runs_the_decoder() {
	[ "$need_go" = 1 ] || return 0
	for t in $(grep -rlF --include='*_test.go' --exclude-dir=node_modules --exclude-dir=.build --exclude-dir=.git FRAMEFAIRY_FRAMES . 2>/dev/null); do
		gopkgs="$gopkgs $(package_of "${t#./}")"
	done
}

gopkgs=""
alone=""
all_go=0
lint=0
interface=0
walks=0
build=0

# ships: what the app or a program is made of, so the build and, on a
# Mac, the app opened from outside.
ships() { build=1; }

# everything: a file that decides how every part runs.
everything() {
	all_go=1
	lint=1
	interface=1
	walks=1
	build=1
}

[ "${EVERYTHING:-}" = 1 ] && everything

for file in $changed; do
	case $file in
	# The plan is history, and the check that keeps it so is in make
	# lint.
	docs/GUI-PLAN.md) lint=1 ;;
	*.md | docs/* | LICENSE | .claude/* | .vscode/*) ;;
	# The linters that keep tests off the clock, and the test that proves
	# them. make interface runs ESLint, and the packages are ESLint's too.
	.golangci.yml | scripts/lint-test.sh | scripts/testdata/lint/*) lint=1 ;;
	frontend/eslint.config.js | frontend/eslint-suppressions.json)
		interface=1
		lint=1
		;;
	frontend/package.json | frontend/package-lock.json)
		interface=1
		walks=1
		lint=1
		ships
		read_by_tests "$file"
		;;
	# The walk harness: what the walks run, and none of it ships.
	frontend/preview/walks/* | frontend/preview/wails-bridge.ts | frontend/preview/settled.ts | frontend/preview/standing.ts | frontend/preview/bridge.config.ts | frontend/preview/vite.config.ts)
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
		ships
		read_by_tests "$file"
		;;
	go.mod | go.sum)
		all_go=1
		lint=1
		ships
		;;
	Makefile | .github/workflows/ci.yml) everything ;;
	# The machine every Linux job runs in, so every part that runs there.
	# No Mac runs in it, so a Mac has nothing to check.
	.github/ci-image/* | scripts/ci-packages.sh)
		if [ "$OS" != Darwin ]; then
			everything
			interface=0
		fi
		;;
	# make unit and make fuzz run these, so they decide how the Go tests
	# run, like the Makefile.
	scripts/fuzz.sh)
		all_go=1
		lint=1
		;;
	# The rules of the repository, which make lint tests, and this
	# script and its test among them.
	scripts/changed.sh | scripts/changed-test.sh | scripts/needs-build.sh | scripts/needs-build-test.sh | scripts/main-moved.sh | scripts/main-moved-test.sh | scripts/plan-frozen.sh | scripts/plan-frozen-test.sh | scripts/pins.sh) lint=1 ;;
	*.sh)
		lint=1
		ships
		# The notices test reads the build scripts for the versions they
		# pin, and a raised version once passed here and failed in CI.
		read_by_tests "$file"
		# The decoder is built against the ffmpeg this builds.
		[ "$file" = scripts/build-ffmpeg.sh ] && runs_the_decoder
		;;
	# Every workflow is read by internal/workflows, and its pins and the
	# rules for builds and the Main moved notice are in make lint.
	.github/workflows/*)
		lint=1
		[ "$need_go" = 1 ] && gopkgs="$gopkgs $module/internal/workflows"
		;;
	.github/*) ;;
	*)
		pkg=$(package_of "$file")
		if [ -n "$pkg" ]; then
			gopkgs="$gopkgs $pkg"
			case $file in
			# internal/offdisk and internal/ffmpegtest have a test that
			# reads every test file, so a changed test is what reaches
			# them. Their own tests and no more: the engine's tests
			# import ffmpegtest, and a changed test is no change to it.
			*_test.go)
				lint=1
				alone="$alone $module/internal/offdisk $module/internal/ffmpegtest"
				;;
			*/testdata/*) ;;
			*.go)
				lint=1
				ships
				;;
			*) ships ;;
			esac
			case $file in
			cmd/framefairy-frames/*_test.go) ;;
			cmd/framefairy-frames/*) runs_the_decoder ;;
			esac
			# The notices test counts these by a path it builds, which
			# read_by_tests cannot find.
			case $file in engine/hyphenation/*) gopkgs="$gopkgs $module/notices" ;; esac
		elif [ "$need_go" = 1 ]; then
			ships
		fi
		;;
	esac
done

# The packages a change reaches: the ones that changed and every package
# whose code or tests import them, however far down.
affected=""
if [ "$need_go" = 1 ]; then
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
	# What make unit leaves out on this machine, see UNITSKIP in the
	# Makefile, so a change only to those starts no machine for them.
	for s in ${UNITSKIP:-}; do
		affected=$(printf '%s\n' $affected | grep -vxF "$s" || true)
	done
fi

# The walks drive the interface against the app's Go side, so whatever
# reaches the app reaches them, the engine among it.
for p in $affected; do
	[ "$p" = "$module/cmd/framefairy-app" ] && walks=1
done

# The fuzz targets of the packages a change reaches, by name on one line.
# It once ran each target's seeds with coverage and fuzzed only those that
# went through a changed file. For a change to engine/clips.go that took
# 122 s, where fuzzing every target of those packages took 46 s, so a
# package that is tested is fuzzed whole.
targets=""
if [ -n "$affected" ]; then
	list=$(sh scripts/fuzz.sh --list)
	for pkg in $affected; do
		dir=${pkg#"$module"}
		dir=${dir#/}
		[ -z "$dir" ] && dir=.
		targets="$targets $(printf '%s\n' "$list" | awk -v d="./$dir" '$1 == d { print $2 }')"
	done
	targets=$(echo $targets)
fi

# The parts with work, in the order make changed runs them: the quickest
# answer first. Away from a Mac nothing opens the app from outside, and
# on one it runs only in CI, see make outside.
parts=""
[ "$lint" = 1 ] && parts="$parts lint"
[ "$interface" = 1 ] && parts="$parts interface"
[ -n "$affected" ] && parts="$parts unit"
[ -n "$targets" ] && parts="$parts fuzz"
[ "$walks" = 1 ] && parts="$parts walks"
[ "$build" = 1 ] && parts="$parts build"
[ "$build" = 1 ] && [ "$OS" = Darwin ] && [ -n "${CI:-}" ] && parts="$parts outside"
ran=""
for p in $parts; do wanted "$p" && ran="$ran $p"; done
ran=${ran# }

if [ "$mode" = parts ]; then
	echo "$ran"
	exit 0
fi

# What will run, said first, so a run that is less than expected is seen
# before it is trusted.
if [ -z "$ran" ]; then
	if [ -z "$changed" ] && [ "${EVERYTHING:-}" != 1 ]; then
		say "nothing changed against main, so nothing runs"
	else
		say "nothing here for ${PART:-any part}, so nothing runs"
	fi
fi
for p in $ran; do
	case $p in
	unit)
		short=""
		for a in $affected; do
			case $a in "$module") a=. ;; "$module"/*) a=${a#"$module"/} ;; esac
			short="$short $a"
		done
		say "unit:     $short"
		;;
	fuzz) say "fuzz:      $targets" ;;
	*) say "$p" ;;
	esac
done

if [ "$mode" = plan ]; then
	# One word per line for the tests: the part, then what of it.
	for p in $ran; do
		echo "$p"
		case $p in
		unit) for a in $affected; do echo "go $a"; done ;;
		fuzz) for t in $targets; do echo "fuzz $t"; done ;;
		esac
	done
	exit 0
fi

# Each part through its own make target. A push to main tests ./..., the
# same packages by the same name the targets take by default.
pkgs=$affected
[ "${EVERYTHING:-}" = 1 ] && pkgs=./...
[ "${EVERYTHING:-}" = 1 ] && targets=""
for p in $ran; do
	case $p in
	lint) $MAKE -s --no-print-directory lint ;;
	interface) $MAKE -s --no-print-directory interface ;;
	unit) $MAKE -s --no-print-directory unit UNITPKGS="$pkgs" UNITSHARD="$SHARD" ;;
	fuzz) $MAKE -s --no-print-directory fuzz FUZZTARGETS="$targets" ;;
	walks) $MAKE -s --no-print-directory walks WALKSHARD="$SHARD" ;;
	build)
		# Away from a Mac nothing builds the tools we ship, and make
		# stops without them, so there the build proves the programs
		# build and says it goes without. On a Mac it builds them, and
		# a warning from clang or the linker fails it.
		if [ "$OS" = Darwin ]; then
			log=$(mktemp)
			status=0
			$MAKE -s --no-print-directory 2>&1 | tee "$log" || status=1
			# tee hides make's own status, so a failed build is read
			# from what it printed as well.
			if [ "$status" = 0 ] && grep -iE "warning|deprecated" "$log" >/dev/null; then
				grep -iE "warning|deprecated" "$log"
				echo "FAIL	the build printed warnings, see above"
				status=1
			fi
			rm -f "$log"
			[ "$status" = 0 ] || exit 1
		else
			$MAKE -s --no-print-directory NOTOOLS=1
		fi
		;;
	outside) $MAKE -s --no-print-directory outside ;;
	esac
done
exit 0
