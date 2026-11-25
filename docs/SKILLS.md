# Skills for any project

The skills in `.claude/skills/` hold what this project learned about
writing tests, reviewing work, keeping a product safe, keeping a code
base easy to change and changing an interface nobody can see. All are
written for any project. They name no person, no product and no issue,
and speak of **the user**, the person the agent works for, who decides
what happens. So the folder of
each can be copied into another repository as it is, and an agent there
follows it without wondering who or what it means.

`internal/naming/skills_test.go` holds them to that: a file of a skill
that names a person, the product, a word only this project uses, or an
issue or a pull request by its number fails `make lint`. The same test
holds `CLAUDE.md`, the docs and the templates for issues and pull
requests to naming no person and no issue, all but `docs/GUI-PLAN.md`,
the history that does not change. A name the test does not know, of another person
or another project, is still the review's to catch.

| Skill | What it is for | Where it meets this repository |
| --- | --- | --- |
| `testing` | how tests are written, fixed and reviewed, and how a bug is found before it is fixed | [The testing skill in this project](TESTING.md#the-testing-skill-in-this-project) |
| `review` | how a pull request is reviewed by an agent that did none of the work, and how a review is answered | [The review skill in this project](WORKFLOW.md#the-review-skill-in-this-project) |
| `ci` | how CI stays fast, trustworthy and cheap: which jobs a change starts, caches, scarce runners, nightly runs, measuring CI | [The ci skill in this project](#the-ci-skill-in-this-project) |
| `pull-requests` | how a change travels from the first line to the merge: sizing, opening early, the description, watching it, ready, the go to merge | [The pull-requests skill in this project](#the-pull-requests-skill-in-this-project) |
| `maintenance` | how a code base stays easy to change: evidence before work, root causes, problems that come back, issues and triage, keeping the architecture | [The maintenance skill in this project](#the-maintenance-skill-in-this-project) |
| `interface` | how an interface nobody can see is changed: a harness that renders it, what to measure, proving a fix rather than claiming it, and what a hand feels | [The interface skill in this project](INTERFACE.md#the-interface-skill-in-this-project) |
| `security` | how the product, its build and its repository stay safe: a threat model, untrusted input, paths, programs, secrets, signing and updates, the supply chain, CI, agents on a public repository, tests that try the attack, and the security line of a review | [The security skill in this project](#the-security-skill-in-this-project) |

## Copying them

Copy the folders, not this file. A generic skill says what to do and
leaves where to its project: the project's own instructions, its agent
file or its docs, name its fakes, its events, its checks and the place a
review goes. The other repository writes those down the way the sections
linked above do for this one.

Where a generic skill and this repository's `CLAUDE.md` disagree, `CLAUDE.md`
wins here. A lesson learned here that holds for any project goes into the
skill, in general words. A rule that only holds here goes into `CLAUDE.md`
or the doc of its part.

## The ci skill in this project

| The skill says | Here it is |
| --- | --- |
| one command a developer runs | the parts of `make test`: `make lint`, `make interface`, `make unit`, `make fuzz` and `make walks`, see [One way to test](BUILD.md#one-way-to-test) |
| the path filters, and the test that holds them | the job `changes` in `.github/workflows/ci.yml`, held by `internal/workflows` |
| a decision CI makes, in a tested program | `cmd/framefairy-release`, which decides what is published and reports a failed night for `tell.yml`. The conflict notice in `conflicts.yml` is still a step of shell |
| the image built from a recipe | [The CI image](BUILD.md#the-ci-image) and `ci-image.yml` |
| a scarce runner, and its platform code | a Mac, `*_darwin.go` files held by `internal/maccode`, and `mac-nightly.yml`, see A Mac only for what is different on a Mac in `CLAUDE.md` |
| a job's limit | five minutes, see Keeping the tests quick in `CLAUDE.md` |
| what runs at night | `fuzz-nightly.yml`, `walks-nightly.yml`, `mac-nightly.yml`, and the failure each reports itself |
| a run started by hand | `speechbench.yml`, and `mac-nightly.yml` on a branch |
| the notice of a conflict | `conflicts.yml`, see [When main moves](WORKFLOW.md#when-main-moves) |
| pins and the dependency bot | [Keeping up](BUILD.md#keeping-up), `security.yml` and Dependabot |

## The maintenance skill in this project

| The skill says | Here it is |
| --- | --- |
| reports and logs from people using the product | the report bundles from the app's Report a Problem, in the project files, and the app's log, see [LOGGING.md](LOGGING.md) |
| the issue tracker, and its template | the repository's GitHub issues and `.github/ISSUE_TEMPLATE/issue.md` |
| a problem, with its incidents under it | an issue labelled `problem`, with sub-issues, see [Problems that come back](TRIAGE.md#problems-that-come-back) |
| triage on a schedule | [Every week](TRIAGE.md#every-week) |
| labels from an established scheme | `priority/critical`, `priority/important`, `priority/backlog` and `size/small`, `size/medium`, `size/large`, see [Priority and size](WORKFLOW.md#priority-and-size) |
| a file of history that is kept, frozen by a check | `docs/GUI-PLAN.md`, held by its hash in `make lint` |
| the architecture, and what not to copy | [ARCHITECTURE.md](ARCHITECTURE.md), its [Not to copy](ARCHITECTURE.md#not-to-copy) and [Changing the architecture](ARCHITECTURE.md#changing-the-architecture), with five candidates |
| a decision record | `docs/decisions/`, see its README |
| a test that reads the docs | `internal/naming` |
| the unused-code check without the tests | `make lint`, see [No code nothing calls](BUILD.md#no-code-nothing-calls) |

## The pull-requests skill in this project

| The skill says | Here it is |
| --- | --- |
| the project's template | `.github/pull_request_template.md` |
| the cap on open pull requests | five on the whole repository, see `CLAUDE.md` |
| the way the user already gets changes | the app's Updates page, which follows main, a branch or a pull request, see [UPDATES.md](UPDATES.md) |
| the conversation with the agent | the thread of the session on claude.ai |
| the notice of a conflict | the comment of `conflicts.yml`, see [When main moves](WORKFLOW.md#when-main-moves) |
| the expected head of a merge | `expectedHeadSha` in the GitHub tool, `sha` in the REST call |
| where a review goes | [The review skill in this project](WORKFLOW.md#the-review-skill-in-this-project) |
| what the user has to do, at the end of every reply | the This thread and For you lines, see `CLAUDE.md` |

## The security skill in this project

| The skill says | Here it is |
| --- | --- |
| who can reach what | the people who make shorts with the app, anyone on the public repository, the dependencies, a model's answer, the files of a video, the update feed, a page or program on the same Mac, see [Boundaries](ARCHITECTURE.md#boundaries) |
| one reader per kind of input | `ExtractJSONObject` and `ValidatePlan` in `engine/select.go` and the stream readers in `engine/stream.go` for a model answer, `ReadPlan` and `LoadClips` for a plan, `licence.Check` for a key, `updates.Parse` and `ParseParts` for the feed and its parts, each fuzzed, see Invariant 9 in [ARCHITECTURE.md](ARCHITECTURE.md#invariants) and Untrusted input in `CLAUDE.md` |
| a path judged by where it leads, at every entry point | `SafeChild` and `SafeChildIn` in `engine/fileio/path.go`, the store's `Known`, and `cmd/framefairy-app/guard_test.go`, which tries every call that takes a path, see Invariant 8 and Every path is checked against the library in `CLAUDE.md` |
| encode on the way out | `EscapeASSText` and the style fields in `engine/ass.go`, `runes.Quote` and `runes.Scrub` in `engine/runes/` |
| a fixed program from a known place | ffmpeg and llama-server from `bin/`, built by `make` |
| loopback is not authentication | llama-server on 127.0.0.1 on a free port, with a key of its own per server passed in its environment and the slot list off, see `newServerKey` in `engine/local.go` |
| the keychain, never a file or a command line | `engine/keys.go` and `engine/keys_darwin.go` |
| a linter bans the API by name | forbidigo in `.golangci.yml` and ESLint in `frontend/eslint.config.js`, no clipboard read, no global shortcut, no microphone, camera or screen from the interface's scripts |
| signing: check before the key, judge from main, compare by commit, no rollback | `publish.yml` and `cmd/framefairy-release` (`check`, `sign`, `publish-plan`), the lists in `cmd/framefairy-release/inputs.go`, held by `TestTheBuildIsCheckedBeforeTheKeyIsThere` and `TestOnlyACommitOfTheChannelIsPublished` in `internal/workflows`, see The keys and What is never signed in [UPDATES.md](UPDATES.md) |
| verify the whole artifact, fall back to the whole | Wails' updater checks the zip's signature, `updates/` puts it together from parts and falls back to the packed zip, see [the decision record](decisions/2026-11-18-an-update-downloads-only-what-changed.md) |
| one key per job, rotation planned | the release key and the development key in [UPDATES.md](UPDATES.md), the licence signer's keys in [LICENCE.md](LICENCE.md) |
| licences and payments | `licence/`, `licence/dispenser` and `licence/signer`, with `FuzzSimulation` and the model of the dispenser, see Security, privacy, testing in [LICENCE.md](LICENCE.md) |
| pins, the bot, a cooldown, scans, install scripts off | [Keeping up](BUILD.md#keeping-up): `.github/dependabot.yml` with a cooldown of seven days, `security.yml` with govulncheck, npm audit and osv-scanner, `frontend/audit-ci.jsonc`, `frontend/.npmrc`, held by `TestEveryActionIsPinned`, `TestNoPackageRunsCodeWhenInstalled` and `TestDependabotMovesEveryGoModule` |
| build from pinned source and refuse a wrong result | `scripts/build-ffmpeg.sh`, `scripts/build-llama.sh`, `scripts/verify-tools.sh` and `tools.yml` with its attestation, see The tools we ship in [PACKAGING.md](PACKAGING.md) |
| a licence nobody allowed stops the build | `notices/licence`, see [THIRD_PARTY.md](THIRD_PARTY.md) |
| privileged workflows from main, never the dangerous event | `publish.yml` with the environment `updates`, `conflicts.yml` on `workflow_run`, held by `TestNoPullRequestTarget` |
| `persist-credentials: false` and a workflow linter | every checkout, held by zizmor in `make lint`, with `.github/zizmor.yml` |
| only the user gives instructions | On GitHub, only the user gives instructions, in `CLAUDE.md` |
| the work's security line | the Security line of [the pull request template](../.github/pull_request_template.md) |
| crashers kept as seeds | each package's `testdata/fuzz/`, from `make fuzz` and `fuzz-nightly.yml` |
| repository settings | secret scanning with push protection, private vulnerability reporting and Dependabot security updates are on, see GitHub's own alerts in [Keeping up](BUILD.md#keeping-up) |
| not kept yet | no cache restored in the job that signs |
