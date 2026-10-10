# Working with Claude through GitHub

The code lives in a GitHub repository. Claude works on it in cloud sessions
at [claude.ai/code](https://claude.ai/code), in a virtual machine at
Anthropic, and delivers every change as a pull request. The app on your Mac
brings a change in by itself, from its Updates page. Nothing runs Claude
Code locally, and no zip files change hands.

```
you describe a task at claude.ai/code
  → Claude works on a branch in the cloud, tests the change, pushes, runs make changed
  → a pull request opens, CI builds and tests it on Linux and macOS
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
compile, and builds the ffmpeg we ship and the episode's decoder in the
checkout, the way CI does, so the decoder's tests and the walks run in a
session too. It also installs the engineering skills from two plugins, Go skills
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
   screenshots included. Batch numbers from [GUI-PLAN.md](GUI-PLAN.md) work
   as shorthand.
2. **Claude works** on a new branch. It reads `CLAUDE.md` first, which holds
   the project's rules and your preferences. It runs the tests of what it
   changed, pushes, and opens a pull request, so you can try the change at
   once. Then it runs `make changed`, which checks everything the branch
   reaches, and pushes a fix if that finds anything. You can watch and steer the session at
   any time, also from the Claude app on your phone.
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
once that one is merged or closed. Something you report that is not
about its open pull request is logged as a row marked `[ ]` in
[GUI-PLAN.md](GUI-PLAN.md) and taken up in turn, the oldest first unless
you reorder them. See CLAUDE.md.

## CI

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every pull
request and every push to `main`, as six jobs at the same time, so the
answer comes back in the time the slowest takes:

- **interface:** `make interface`, the type check and the interface's own
  tests.
- **build:** the rules that decide what runs, then `make` on Linux, which
  proves the programs still link with the interface built into them.
- **linux:** the Go tests under the race detector, `make unit`, including
  the tests that render.
- **fuzz:** `make fuzz` on Linux.
- **macos:** builds our own ffmpeg, runs `make` and fails if the build prints
  any warning, then runs the Go tests.
- **macos-fuzz:** `make fuzz` on macOS.

On a pull request each job first asks `scripts/ci-needs.sh` whether the
change gives it anything to do, and does nothing when it does not. A push to
`main` narrows nothing.

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
push, and no push to `main` follows to say so. A pull request that still
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
push, and `make changed` after it.
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
