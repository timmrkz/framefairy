---
name: maintenance
description: How to keep a code base easy to change and stop chasing the same problems in any project. Evidence before work, root causes over workarounds, problems that come back, issues and triage, fixes that leave fewer issues than they found, and keeping the architecture from wearing away. Read it before opening, picking, triaging or closing an issue, before a second fix of the same symptom, and before a change that adds a part, a copy or a new home for state.
---

# Maintenance

A project can spend all its time on itself. Every fix brings two issues,
every issue is fixed three times in three places, and the work people
asked for waits behind it. That is churn, and it is not bad luck. It comes
from a few habits: fixing what nobody meets, fixing the symptom where it
showed, logging every thought as an issue, and letting each fix add one
more special case. This skill is the opposite habits.

**The user** is the person the agent works for, who decides what happens.
**The people using the product** are the ones it is made for. The project's
own instructions say where its issues, its architecture and its decision
records are.

## 1. Evidence first

- **Every piece of work names its evidence and what gets better.** A
  failing check, a log from someone using the product, a report, code
  that drifted from its doc. And in a sentence, what gets better for the
  people using the product once it is done. Work with neither is not
  started.
- **Judge every problem by who it happens to.** Read it against reports
  and logs from people using the product first. Then sort it:
  - **Seen in use:** a person met it. It is the product's.
  - **Seen only where tests run:** a CI runner's virtual hardware, a
    machine loaded on purpose, a stand-in program, a file made for the
    test. It is about the test or its environment, and is rewritten as
    one.
  - **Seen only by reading code,** in a review or in a scenario nobody
    meets, like running an older build or a file kind nobody has. It is
    closed, unless the code itself can be made to take that order of
    events by a test, which stays and gets the test.
- **Made-up data is not evidence for a product change.** Fixes built for
  a noise found only on synthetic input, for file kinds no user had, for
  machines that do not exist, brought risk and no gain. Check them
  against real logs first.
- **Check the premise before the work.** Write the test that would fail
  as the issue describes. If none can fail, it is a cleanup at most. An
  issue's proposed fix is a claim too: check it against real data and
  against the rules the project writes. One proposed deny list would have
  refused every build.
- **Measure an old number again before designing around it.** A load
  time remembered as 24 s was 9 s. Before closing something as costing
  nothing, look in the logs of people using the product for slow calls
  in that area, since a benchmark of the suspected function can miss
  what they wait on.
- **Fresh evidence goes first.** A test that just failed on the main
  branch, or an issue that comes with its trace or its log, goes before
  an older issue picked by priority and age. Labels lag behind what is
  happening, and evidence in hand gets a fix done sooner. Say which was
  picked and why.

## 2. Root causes only

- **A workaround closes nothing.** A place that gives up, a longer wait,
  a skipped case, a retry: it ends this occurrence and leaves the cause
  to come back under the next issue, where nobody sees it is the same
  thing.
- **A change that only helps find the cause closes nothing either.** A
  log line or a kept trace says so, and the issue stays open.
- **Fixes only for a proven cause.** A change closes an issue only when
  it removes the cause the issue saw and that is shown, failing before and
  passing after. A change to the most likely gap says it works on the
  issue, leaves it open, and says what the next failure will show. A
  closing keyword never makes a choice the user has not made.
- **When the cause is still unknown after a few fixes, stop fixing.** The
  next change only captures it: a stack dump of every program involved, a
  log line, a kept trace. It changes no behaviour. One captured stack
  ended a run of five guessed fixes.
- **No product workaround for a failure only the test machines have.**
  Five guards for a hang seen only on CI's virtual machine went into
  shipped code. One of them turned a hardware path off for every user for
  three weeks, and logs from people using the product never showed the
  hang. Search the shipped code for such guards now and then, and remove
  each that no real log has ever seen fire.
- **A diagnostic that only cries wolf comes out.** A health check whose
  warnings were mostly false, and never used to find a bug, costs time
  and buries the real signals.

## 3. Problems that come back

- **One occurrence is an incident. A second one with the same cause makes
  a problem.** Open one problem issue with both incidents under it. It
  states the cause as far as it is known, every fix tried and why the
  cause stayed. It takes the highest priority of its incidents, closes
  only after its last incident, and is reopened when the cause comes back,
  which means the cause it named was not the whole cause. This is how
  established incident management works. Use it, do not invent one.
- **Search closed issues as well as open ones,** by the failing test's
  name, the function the stack stopped in and the words of the error. A
  hang in five places was five issues and five fixes before anyone put
  them side by side.
- **One issue per flaky test, not one per sighting.** Each sighting is a
  comment with the run, the commit, the test and what it printed.
- **Check the work in flight before fixing.** Search the open pull
  requests and issues for the same cause, fix it once, at the cause, and
  tell the others it affects.

## 4. When fixes stack up

- **The second fix of one symptom names the problem. The third moves the
  decision.** When each fix gives one more state its own special case,
  the next state breaks it again. Replace the cases with one rule that
  every state goes through, owned by the side that holds the state and
  asked again after every event. One layout bug came back four times,
  each fix giving one more state its own reference to lay out from,
  until one reference shared by every state ended it.
- **Count what the fixes added.** Flags, effects, counters, tickets,
  polls. When every fix adds one, no single rule owns the decision.
- **Two places that decide one thing: delete one.** Making the second
  agree, with a better key or a later timestamp, leaves the race.
- **After rounds of tuning, look for what already solves it.** When two or
  three rounds of tuning change nothing a person can feel, stop. Look for
  a part of the product that already solved the same shape of problem, and
  use it. When each fix makes new bugs, find the assumption that forced
  the design, and test it.
- **The same test fix twice means one helper** every test goes through,
  see the testing skill.
- **Fix the instruction that misled.** When agents keep making the same
  mistake, find the line in their instructions that pointed them there
  and correct it in place. A new rule beside a broken one is a second
  hole, and the opposite extreme of a wrong rule is wrong too.

## 5. Issues

- **Open work is in the issue tracker, never in rows of a file.** Rows
  every change appends to one file make any two changes conflict, numbers
  collide, and the status drifts from the code. An issue number is never
  taken twice and lives in no file. A file of history that is kept is
  frozen by a check.
- **An issue is written from a template,** and an agent that opens one
  through an API reads the template itself, since the API skips the form:
  - a title a person would say,
  - one short paragraph: what goes wrong, who saw it and where, what gets
    better,
  - Seen: the report, the log lines or the run,
  - Cause: a few words traced to the file and the name, or not known yet
    and what would find it, the trace first,
  - What to build, in one place, as a plan, never as a question.
- **Read the whole rule an issue cites,** its exceptions included, and
  search for a duplicate first. An issue once said a nightly job broke a
  job limit the rule exempts nightly runs from.
- **Log only what someone can meet.** "Can", "may" and "likely" are not
  evidence. Before logging, say who runs into it and what breaks for them.
  An issue nobody can meet is work nobody asked for.
- **Labels from an established scheme,** priority and size, one line
  each, judged with a few words why, and the user corrects. A home-made
  list of questions was reworded every round and never settled. A failing
  test or a red main branch is critical. Every flaky or hanging test is at
  least important, because a test that cannot be trusted no longer checks
  anything.
- **An issue shows its progress and what is known.** An issue that takes
  more than one pull request has a checklist at the top, one line each,
  ticked when each merges. Whoever learns something about it, a cause, a
  trace, a failed attempt to reproduce with its count, edits the body in
  the same turn.
- **Take only free work.** Before picking an issue, check that no open
  pull request or other agent holds it, and say so.

## 6. Triage

On a schedule, in a fixed order, most current first:

1. **Every red run on the main branch since the last triage,** scheduled
   runs included, is matched to an issue.
2. **Missing labels** are set.
3. **Every problem is judged by who it happens to,** section 1.
4. **What can close, closes,** each with one line saying why and the
   matching reason:
   - done on the main branch,
   - its reason is gone,
   - a duplicate, with what it added copied to the one that stays,
   - nothing to act on until it fails again, with no new trace since its
     last failure: closed as not planned, saying what the next failure
     will show and that it reopens then,
   - fixed on the way: closed with the count of runs that passed since.
5. **What changed is rewritten.** An issue mostly done is rewritten to
   what is left. One whose facts changed is rewritten to what is measured
   now.
6. **Incidents of the period are grouped under problems,** section 3.

Triage changes no code, never changes the user's own labels or words, and
ends with a list of every change, each linked, for the user to check.

## 7. A fix leaves fewer issues than it found

- **A fix closes what it picked,** and the findings of its review about
  the same code path are fixed in it, not logged. A pull request that
  fixed one bug and logged four new ones from its review leaves the
  project worse off than before.
- **A small thing the user asks for on the way goes into the open
  change,** as a commit of its own, named in its description.
- **Clean up now, not later.** What the change left behind goes in the
  same change: fields still written and read by nothing, code only tests
  call, settings of removed jobs, temporary debug commits, fallbacks that
  cannot run. A cleanup put off is mostly never done.
- **Find every claim the change made false.** Search the docs, comments,
  decision records and templates for every renamed term and every
  behaviour the change alters, and fix each.
- **Only what the change did not cause and cannot do now is logged,** as
  an issue of its own, linked from the change.

## 8. Keeping the architecture

- **The architecture is written down,** in one document read before every
  change: the layers and which may use which, where each kind of thing
  goes, the invariants, the boundaries. A change that brings a new kind of
  thing adds its line in the same change.
- **It keeps a list of what not to copy:** code that breaks its rules
  today, each with the issue that fixes it. Agents copy what they find
  nearby. An item leaves the list when its fix lands.
- **A change to the architecture is worked through as several candidate
  designs first,** each from a principle of its own: who may know about
  whom, what changes together, where state lives, what runs together,
  what a test has to build. On the code as it is, with what moves and the
  names that cross each new boundary counted, not guessed. Variations of
  one idea count as one, and the smallest candidate, the one with no new
  state, is always among them.
- **The choice is written as a decision record,** dated, not numbered,
  so two open changes never pick the same number. It ends with what it
  costs, what it buys, and how it would be shown wrong. A record is never
  rewritten. A later one replaces it, and the old one says so.
- **The drawings change with the text.** A map that lies is worse than
  none. A test reads the docs and fails when a path, a name or a drawing
  they mention is gone.
- **One rule, one home.** A rule written in two layers moves to one, and
  the other asks for the answer or receives it as data. A shared table of
  cases between two copies lowers the drift and keeps the copy.
- **The bottom changes least.** A type every feature edits puts the most
  changed thing where everything depends on it. Each feature owns its
  data behind a small interface. Count from the history how many commits
  touch each part, and replay recent commits against a proposed layout to
  see how much would still touch the core. Hold the layers with an import
  linter in allow-list mode.
- **Hot files are split.** The files most commits touch are where parallel
  changes conflict.
- **Code that only tests call is dead.** Run the unused-code check a
  second time with the tests left out. A concurrency test that drives
  functions the product never calls proves nothing about the product, and
  once one was written for the real path it found a real race.
- **Write down what you rely on inside a dependency,** where its updates
  are reviewed, so the next bump checks it.
- **Keep what was tried and failed, and what the user settled,** with the
  numbers, where the next person looks, so neither is tried or proposed
  again. An exception the user accepted is written into the rule it
  breaks, as the one place allowed, with the reason.
- **Review the architecture now and then against itself.** Compare the
  code with a checkout from a few weeks before, give each finding a
  trend, new, grew, shrank or gone, check that the map is still true,
  and open only a handful of issues that help most.

## 9. Rules that hold

- **A rule lives where the gate reads it.** A rule kept only in a memory,
  a brief or a doc no check reads is skipped. Put it in the checklist
  that decides ready, in the template the work is written in, or best, in
  a check.
- **A rule broken despite being written becomes a check,** not the same
  words louder.
- **Numbers hold better than qualities.** Agents keep a limit of 30 words,
  and drift on "short and to the point".

## 10. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| log every risk a review or a reading of the code finds | an issue nobody meets is work nobody asked for, and it buries the ones people meet | log what someone can meet, with the evidence, section 5 |
| fix the bug where it showed | the cause is usually shared by other paths and comes back there | the cause, every path that shares it, section 2 |
| close the issue when the change lands | a change to the likely gap may not be the cause | close only on a proven cause, section 2 |
| a defensive guard can only help | a guard for the test machine's hardware shipped to every user | no product workaround for a test-only failure, section 2 |
| priority by a questionnaire of our own | reworded every round | an established scheme, section 5 |
| track tasks in a file in the repository | every change conflicts on it | the issue tracker, section 5 |
| refactor later, ship now | a cleanup put off is mostly never done | clean up in the same change, section 7 |

## Before saying done

1. The work named its evidence and what gets better.
2. It removed a cause, or says it works on the issue and leaves it open.
3. A recurring cause is one problem with its incidents under it.
4. Nothing it added is a second home for a rule or for state.
5. It leaves fewer open issues than it found, and nothing behind.
6. The docs, the drawings and the architecture still say what the code
   does.
