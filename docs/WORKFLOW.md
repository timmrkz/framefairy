# Working with Claude through GitHub

The code lives in a GitHub repository. Claude works on it in cloud sessions
at [claude.ai/code](https://claude.ai/code), in a virtual machine at
Anthropic, and delivers every change as a pull request. Your Mac only pulls
and builds. Nothing runs Claude Code locally, and no zip files change hands.

```
you describe a task at claude.ai/code
  → Claude works on a branch in the cloud, runs make changed, pushes
  → a pull request opens, CI builds and tests it on Linux and macOS
  → you review and merge
  → on your Mac: git pull && make run
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
compile. It runs once and its result is cached for about a week. When the Go
version in the script changes, paste the new version and update the variable.

The environment variable makes every `go` command in a session use Go 1.27,
whatever version the machine came with.

## Everyday loop

1. **Start a session** at claude.ai/code with the `framefairy` repository and the
   `framefairy` environment. Describe the task the way you would in chat,
   screenshots included. Batch numbers from [GUI-PLAN.md](GUI-PLAN.md) work
   as shorthand.
2. **Claude works** on a new branch. It reads `CLAUDE.md` first, which holds
   the project's rules and your preferences. It runs `make changed`, which
   checks what the branch changed and only that, pushes, and opens a pull request. You can watch and steer the session at
   any time, also from the Claude app on your phone.
3. **CI checks the pull request**, see below. With auto-fix switched on for
   the pull request, Claude fixes failed checks by itself.
4. **Review** the diff in the session or on GitHub. Comment on lines to ask
   for changes. Merge when it is right.
5. **On your Mac:**
   ```
   git checkout main
   git pull
   make run
   ```
6. **Feedback** from testing goes back into the same session, or into a new
   one for a new task.

To try a pull request before merging:

```
git fetch origin
git checkout BRANCH-NAME
make run
```

Several sessions can run at once, each on its own branch. Keep them to
separate areas of the code, so the pull requests don't conflict.

## CI

[`.github/workflows/ci.yml`](../.github/workflows/ci.yml) runs on every pull
request and every push to `main`:

- **Linux:** installs the system packages, runs `make` and `make test`,
  including the tests that render.
- **macOS:** runs `make` and fails if the build prints any warning, then runs
  `make test`.

### When main moves

A Claude session watches its own pull request: comments, reviews and CI on
its commits reach it. A merge into `main` is none of those, so a pull
request can fall behind `main`, or stop merging, with nobody told.

[`.github/workflows/main-moved.yml`](../.github/workflows/main-moved.yml)
closes that gap. On every push to `main` it runs
[`scripts/main-moved.sh`](../scripts/main-moved.sh), which tries merging
`main` into the branch of every open pull request and leaves one comment on
each one `main` is not already in:

- **conflicts:** which files, and that `main` has to be merged in and
  resolved
- **merges cleanly:** that `main` should be merged in, so the pull request is
  tested against what it will land on

The comment is what wakes the session. It merges `main` in, resolves what
conflicts, runs `make changed`, and pushes. The workflow itself never
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
