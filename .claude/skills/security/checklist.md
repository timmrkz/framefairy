# Reviewing security: the questions

Section 6 of the security skill says how a verdict is stated. These are
the questions to ask of a diff, and what to look for under each. Start
from who could use the change, then go down the list. Most changes answer
most questions with "nothing new", and that is the answer to write.

## What comes in from outside

- A new file format, field, endpoint, link scheme, webhook, service or
  language model response, or config key. Is it read by the one reader made for it, strict on
  shape and version, with every size and count capped?
- A path from anywhere. Is it resolved through links and checked against
  the root, at this entry point, by the shared function? Does the test
  that tries every entry point have a line for it?
- An ID or key from the interface. Is it only compared, never joined
  onto a path or a query?
- Text written into another format: HTML, a header, a log line, a CSV. Is it encoded for that format where it is written?
- A new parser of outside bytes with no fuzz target is a finding.

## What it runs

- `exec` with arguments built from outside text, `sh -c`, `eval`, a
  template rendered from input, a program looked up on `PATH`.
- Code loaded from a place someone else can write: a plugin folder, a
  cache, a download not yet checked.
- In CI, a step that runs code from a pull request or a freshly bumped
  dependency while it holds a secret or a token that writes.

## What it fetches, and from whom

- A new dependency, action, image, tool or model. Pinned exactly, by
  commit or digest, in the lock file, watched by the bot, scanned?
- A download at run time. Checked by checksum or signature before use,
  bounded in size and time, over TLS with verification on?
- A new single-maintainer dependency, or a bump of one. Was the source
  diff read?

## What it lets out

- A key, token, password or a person's text in a log, an error, a crash
  report, a URL, a test fixture, a commit or a message to a service.
- Something sent off the machine that the product says stays on it.
- A new local server or port. Who else on the machine can reach it?

## What it may do

- A new permission the platform grants, such as an Apple entitlement or
  usage description, or an Android manifest permission, or a newly
  linked system framework. A privileged API reached through another name: a
  clipboard read through a second library, a lookup at run time by
  string. Does the linter or the signing step ban every name it goes by?
- A workflow's `permissions`, a new secret, a new environment, a job-wide
  token that a step reading untrusted bytes can see.
- A new trigger. In which branch's copy does it run, and can a fork or a
  branch start it with a token that writes?

## Who may give instructions

- Does the change, a workflow, a bot or an agent rule act on text a
  stranger can write: a comment, a title, a branch name, release notes,
  a model's answer?
- Is a setting or permission change in the work suggested by anyone but
  the user? Then it is a finding, however reasonable it sounds.

## A gate guards what really runs

What reviews of signing and update work actually found:

- **It covers the file that is installed or run,** not a sibling, a
  slice, or a copy made before the last step changed it. A binary for
  several processors was judged with offsets from the wrong slice.
- **It is tied to the step it protects by an explicit condition,** not
  by the order of steps, which the next edit can change.
- **It names things that cannot be shadowed:** a commit, not a branch
  name a tag can take.
- **It checks the hash or signature before it decodes untrusted bytes,**
  and caps the declared size before reading.
- **Its list cannot be changed by the code it judges,** because it runs
  from the protected branch.
- **Its test fails with the bug in place.** A test that checks a string
  is present, globs only `*.yml`, or reads one spelling of a trigger
  passes while the hole is open. Try the bad input against it.
- **Look-alikes stay allowed for a reason,** written beside them: a call
  that only drops rights is not one that raises them.

## Writing the section

One verdict, safer, the same or less safe, then the evidence in a line
or a short list: what is new at the door, which guard it goes through,
and whether the work's own security line holds. Name what was not
checked.
