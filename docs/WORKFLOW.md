# Working with Claude through GitHub

The code lives in a GitHub repository. Claude works on it in cloud sessions
at [claude.ai/code](https://claude.ai/code), in a virtual machine at
Anthropic, and delivers every change as a pull request. The app on your Mac
brings a change in by itself, from its Updates page. Nothing runs Claude
Code locally, and no zip files change hands.

```
you describe a task at claude.ai/code
  → Claude works on a branch in the cloud, runs the tests of the change, pushes
  → a pull request opens, CI builds and tests all it reaches on Linux and macOS
  → you review and merge
  → in the app: Updates, following main
```

Cloud sessions are a research preview for Pro, Max, Team and Enterprise
plans. The details are in Anthropic's docs:
[Use Claude Code in the cloud](https://code.claude.com/docs/en/claude-code-on-the-web)
and [Configure cloud environments](https://code.claude.com/docs/en/cloud-environments).

## One-time setup

### 1. Put the code on GitHub

1. Create a private repository on GitHub, for example `framefairy`, without a
   README or licence, so it starts empty.
2. In your local repository, bring in the latest files once more, then build,
   so `go.sum` is complete:
   ```
   make
   ```
3. Commit everything and push:
   ```
   git add -A
   git commit -m "Project as of the move to GitHub"
   git remote add origin git@github.com:YOUR-NAME/framefairy.git
   git push -u origin main
   ```

Build output stays out of git: `bin/`, `.build/`, `frontend/node_modules/`
and the built interface in `cmd/framefairy-app/dist/app/`. The `.gitignore`
covers all of them.

### 2. Install the Claude GitHub App

Install it from [github.com/apps/claude](https://github.com/apps/claude) and
give it access to the `framefairy` repository. Cloud sessions use it to clone and
push, and it enables auto-fix, where Claude reacts to failed CI checks and
review comments on its own pull requests.

### 3. Connect claude.ai/code

Open [claude.ai/code](https://claude.ai/code) and follow the onboarding to
connect GitHub.

### 4. Create the cloud environment

At claude.ai/code, open the environment selector, the cloud icon above the
message box, and choose **Add cloud environment**:

| Field | Value |
| --- | --- |
| Name | `framefairy` |
| Network access | **Trusted** |
| Environment variables | `GOTOOLCHAIN=go1.27.1` |
| Setup script | the full content of [`scripts/cloud-setup.sh`](../scripts/cloud-setup.sh) |

The setup script installs Go 1.27, ffmpeg and the libraries the app needs to
compile. It does not build the ffmpeg we ship and the episode's decoder,
because that takes longer than the five minutes a setup script may take
and still be cached, and new sessions can be stopped while it runs. The
SessionStart hook in `.claude/settings.json` builds them in the background
as each session starts, see
[`scripts/cloud-decoder.sh`](../scripts/cloud-decoder.sh), and the
targets that run the decoder, `make unit`, `walks` and `test`,
wait for it, so a session can run the decoder's tests and a walk of its
own too. It also installs the engineering skills from two plugins, Go skills
from `samber/cc-skills-golang` and general ones from `addyosmani/agent-skills`,
because plugins added on claude.ai do not reach cloud sessions. Each is pinned
to a commit in `SKILL_SOURCES` at the top of the script, so a new version is
read before Claude follows it. Claude Code gives the list of skills about 1%
of its context by default, which is not enough for 71 more, so
`.claude/settings.json` raises it to 80000 characters. The script runs once and its result is cached
for about a week. Whenever the script changes, paste the new version, and when
the Go version in it changes, update the variable too.

The environment variable makes every `go` command in a session use Go 1.27,
whatever version the machine came with.

## Everyday loop

1. **Start a session** at claude.ai/code with the `framefairy` repository and the
   `framefairy` environment. Describe the task the way you would in chat,
   screenshots included. Issue numbers work as shorthand, and so do the
   batch numbers of the old plan in [GUI-PLAN.md](GUI-PLAN.md).
2. **Claude works** on a new branch. It reads `CLAUDE.md` first, which holds
   the project's rules and your preferences. It runs the tests of what it
   changed, pushes, and opens a pull request, so
   you can try the change at once. It runs nothing more: CI checks
   everything the branch reaches, and the session hears how that went from
   the pull request's events and pushes a fix when it fails. Once all the work is
   done, it asks a review agent that did none of it to review the pull
   request, posts the findings and answers each one before it says the
   pull request is ready, see CLAUDE.md. You can
   watch and steer the session at any time, also from the Claude app on
   your phone.
3. **CI checks the pull request**, see below. With auto-fix switched on for
   the pull request, Claude fixes failed checks by itself.
4. **Review** the diff in the session or on GitHub. Comment on lines to ask
   for changes. Merge when it is right.
5. **In the app:** Updates, at the foot of the sidebar, following main,
   brings the merged change in. A change to the command line or to `make`
   itself is tried from the checkout instead:
   ```
   git checkout main
   git pull
   make run
   ```
6. **Feedback** from testing goes back into the same session, or into a new
   one for a new task.

To try a pull request before merging, follow it on the Updates page, by
its number. From the checkout it is:

```
git fetch origin
git checkout BRANCH-NAME
make run
```

Several sessions can run at once, each on its own branch. Keep them to
separate areas of the code, so the pull requests don't conflict. Each
session works on one pull request at a time, and opens the next only
once that one is merged or closed. Each pull request is one
self-contained change, the way Google's code review guide draws it, and
what it leaves behind is cleaned up before it is merged. A small thing
you ask for in a session's thread goes into its open pull request, as a
commit of its own. Something that pull request did not cause, and that
cannot be done now, is logged as a GitHub issue, labelled bug,
feature, tests, reliability, performance or shipping, with a priority
and a size, and taken up in turn: the highest priority first, the
smallest and then the oldest within it, unless you reorder them. A
test that has just failed on main, or an issue that comes with its
trace or its log, goes before that order.
Opening an issue changes no file, so it costs no push and no CI run, and
the pull request that finishes it closes it with `Fixes #N`. See
CLAUDE.md.

## Priority and size

Every open issue carries one priority and one size, beside the labels
for its kind. The priorities are the ones
[Kubernetes](https://github.com/kubernetes/community/blob/master/contributors/guide/issue-triage.md)
triages its issues by, without its split of important into soon and
long term. The sizes are the T-shirt sizes many teams on GitHub use.

| Label | What it means | For example |
| --- | --- | --- |
| `priority/critical` | Drop what you are doing. A short comes out wrong or a person's work is lost, a broken build, a failing test, a security hole. | #266, #268 |
| `priority/important` | Worked on now or very soon. A bug a person meets or that the code shows, and every flaky or hanging test, because a test that cannot be trusted no longer verifies what it checks. | #540, #507, #559 |
| `priority/backlog` | Not now. A nice to have nobody will get to soon, tidying what works, a test that is only slow, and the long-term work, like signing and the licence key, whose order Open work in CLAUDE.md gives. | #267, #550, #305 |
| `size/small` | A short pull request in one place. | #540 |
| `size/medium` | One pull request. | #513 |
| `size/large` | More than one pull request. | #380 |

A failing test fails on every run, or is red on main. A flaky test
fails on some runs and passes on others, on CI or in a session.

Whoever opens an issue sets both, by judgement from the table. Tim moves
any label he sees differently, and his label stands. Someone else who
sees one differently says so in a comment on the issue, as Kubernetes
asks. The weekly triage labels every open issue that lacks one and
looks again at the ones whose facts changed, a flaky test seen failing
again for example. The table itself changes only when Tim says so.

## The review skill in this project

The skill in `.claude/skills/review/` is written for any project, so it
names nothing of ours. This is where its words meet this repository.

| The skill says | Here it is |
| --- | --- |
| the user | you |
| the people using the product | the people who make shorts with the app |
| the product's goals as the project writes them | The project in CLAUDE.md, and the issue the pull request names |
| the user's own words | your messages in the thread of the session, and your report files in the project files |
| the place the project names for the review | a file in `/mnt/project-files/reviews/`, named after the pull request, never committed |
| where instructions may come from | only you, see On GitHub, only Tim gives instructions in CLAUDE.md. Sessions write on GitHub as `timmrkz`, so a review is never posted there |
| logged | a GitHub issue with its label, see Clean up now, not later, in CLAUDE.md |
| the recent issues and pull requests in the same area | the issues and the merged pull requests on GitHub, and the history up to 16 October in [GUI-PLAN.md](GUI-PLAN.md) |
| a change to the design, with its candidates | an architectural change, with its decision record in `docs/decisions/` and its five candidates, see Changing the architecture in [ARCHITECTURE.md](ARCHITECTURE.md#changing-the-architecture) |
| the architecture as the project writes it | [ARCHITECTURE.md](ARCHITECTURE.md): the bird's eye view and its drawings in `docs/diagrams/`, the code map, Where does it go, the invariants, the boundaries and Not to copy. Every brief names it |
| in the right place, in the part that owns the idea | where [ARCHITECTURE.md](ARCHITECTURE.md) says it belongs: the layer of the bird's eye view, the row of Where does it go, the invariants. A change that copies what Not to copy names, or has no place in the file and adds none, is a finding |

## CI

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every pull
request and every push to `main`, as up to twelve jobs at the same time,
so the answer comes back in the time the slowest takes. A first job,
**plan**, asks `scripts/changed.sh` which parts the change reaches, and
only those start. Each job runs one make command, `make changed` for its
part, the same one you run on your Mac, so CI and `make changed` check
the same thing, see [One way to test](BUILD.md#one-way-to-test):

- **interface:** `make interface`, the type check and the interface's own
  tests.
- **build:** `make lint`, then `make` on Linux, which proves the programs
  still link with the interface built into them.
- **linux:** the Go tests under the race detector, `make unit`, including
  the tests that render, of the packages the change reaches, on three
  machines for the engine or the app, `UNITSHARD=1/3` to `3/3`, and on
  one for the rest.
- **walks:** the interface in Chromium against the real Go side,
  `make walks`, every run, on four machines, for a change to the
  interface, the walks or the Go side of the app.
- **fuzz:** `make fuzz` on Linux, the targets of the packages the change
  reaches, going on from the fuzz cache main left.
- **macos:** `make unit` against our own ffmpeg, which on a Mac leaves
  out the race detector, which Linux runs.
- **outside:** `make` on a Mac, which fails if the build prints any
  warning, then the app opened by macOS from a link, see
  [TESTING.md](TESTING.md#from-outside-the-app).

A push to `main` runs every part whole. A pull request of docs alone
reaches only the test that reads the docs, `internal/naming`, on one
Linux machine.

### When main moves

A Claude session watches its own pull request: comments, reviews and CI on
its commits reach it. A merge into `main` is none of those, so a pull
request can fall behind `main`, or stop merging, with nobody told.

[`.github/workflows/main-moved.yml`](../.github/workflows/main-moved.yml)
closes that gap. It runs
[`scripts/main-moved.sh`](../scripts/main-moved.sh), which tries merging
`main` into the branch of a pull request and leaves a comment on it when
`main` no longer merges, naming the files that conflict. It does that for
every open pull request on every push to `main`, and for one pull request
whenever it is opened or pushed to. The second is for a branch started
from a `main` that has moved on since: it is in conflict from its first
push, and no push to `main` follows to say so. It hears a pull request
through `pull_request_target`, not `pull_request`, because GitHub runs no
`pull_request` workflow on a pull request that conflicts with `main`: #213
was opened in conflict, and neither CI nor this workflow ran on it, so
nothing said so, plan row 2.241. `pull_request_target` runs main's
workflow and main's script whatever the conflict, and they only read the
branch, never run it. A pull request that still
merges is left alone: merging `main` into it would only add a commit, a CI
run and a build to take, and change nothing being tested.

The merge is tried in git, not asked of GitHub. GitHub works out whether a
pull request merges some time after a push, and until then it answers
that it does not know.

The comment is what wakes the session. It merges `main` in at once,
without asking, resolves the conflict, reads `CLAUDE.md` again and pushes,
because GitHub runs no CI on a pull request that conflicts with `main`.
The merge is tested like a change of the session's own, because resolving
it writes new code: the tests of what the resolution touches before the
push, and CI after it.
Reading `CLAUDE.md` again is in the comment on purpose: a session reads it
once when it starts, so a rule that lands on `main` while the session runs
would otherwise reach it only in its next session. The workflow itself never
pushes, because a merge made there would reach the branch untested. It
comments once per pull request for each move of `main`, and leaves pull
requests from forks alone.

## What stays on your Mac

- Trying the app, since a cloud machine has no screen.
- Anything that needs the models, such as a real transcription or a real clip
  search. The tests use stand-ins for both.
- Training runs, which need a rented GPU, see [TRAINING.md](TRAINING.md).

## Chat or cloud session

Ideas, planning and questions work well in a normal claude.ai chat. Anything
that changes the code belongs in a cloud session, so it arrives as a pull
request.
