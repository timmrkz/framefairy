# Skills for any project

The skills in `.claude/skills/` hold what this project learned about
writing tests, reviewing work and keeping a code base easy to change. All
but one are written for any project. They name no person, no product and
no issue, and speak of **the user**, the person the agent works for, who
decides what happens. So the folder of each can be copied into another
repository as it is, and an agent there follows it without wondering who
or what it means.

`internal/naming/skills_test.go` holds them to that: a file of a skill
that names a person, the product, a word only this project uses, or an
issue or a pull request by its number fails `make lint`. The one skill
about this repository alone is `interface`, which says how to change this
app's interface without seeing its window. It may use this project's
words, and it too names no person and no number. The same test holds
`CLAUDE.md`, the docs and the templates for issues and pull requests to
naming no person and no issue, all but `docs/GUI-PLAN.md`, the history
that does not change. A name the test does not know, of another person
or another project, is still the review's to catch.

| Skill | What it is for | Where it meets this repository |
| --- | --- | --- |
| `testing` | how tests are written, fixed and reviewed, and how a bug is found before it is fixed | [The testing skill in this project](TESTING.md#the-testing-skill-in-this-project) |
| `review` | how a pull request is reviewed by an agent that did none of the work, and how a review is answered | [The review skill in this project](WORKFLOW.md#the-review-skill-in-this-project) |
| `ci` | how CI stays fast, trustworthy and cheap: which jobs a change starts, caches, scarce runners, nightly runs, measuring CI | [The ci skill in this project](#the-ci-skill-in-this-project) |
| `pull-requests` | how a change travels from the first line to the merge: sizing, opening early, the description, watching it, ready, the go to merge | [The pull-requests skill in this project](#the-pull-requests-skill-in-this-project) |
| `maintenance` | how a code base stays easy to change: evidence before work, root causes, problems that come back, issues and triage, keeping the architecture | [The maintenance skill in this project](#the-maintenance-skill-in-this-project) |

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
