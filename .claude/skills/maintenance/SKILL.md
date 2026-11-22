---
name: maintenance
description: How to keep a code base easy to change and stop chasing the same problems in any project. Evidence before work, root causes, problems that come back, issues and triage, fixes that leave fewer issues, keeping the architecture. Read it before opening, picking, triaging or closing an issue, before a second fix of one symptom, and before adding a part, a copy or a new home for state.
---

# Maintenance

Churn comes from habits: fixing what nobody meets, fixing the symptom
where it showed, logging every thought, a special case per fix. This
skill is the opposite habits.

**The user** is the person the agent works for, who decides what happens.
The project's instructions say where its issues, architecture and
decision records are.

## 1. Evidence first

- **Every piece of work names its evidence** (a failing check, a user's
  log, a report, code drifted from its doc) **and what gets better** for
  the people using the product. Without both, do not start.
- **Judge every problem by who it happens to,** users' reports and logs
  first:
  - **Seen in use:** a person met it. It is the product's.
  - **Seen only where tests run** (virtual hardware, a loaded machine, a
    stand-in program, a test-made file): rewritten as an issue about the
    test.
  - **Seen only by reading code,** or in a scenario nobody meets: closed,
    unless a test can force that order of events, and then kept with it.
- **Made-up data is not evidence for a product change.** Fixes for
  synthetic noise, unused file kinds or imaginary machines brought only
  risk.
- **Check the premise first** with the test that would fail as the issue
  describes. If none can, it is a cleanup at most. Check its proposed fix
  against real data and the project's rules too.
- **Remeasure an old number before designing around it:** a load time
  remembered as 24 s was 9 s. Before closing something as free, search
  users' logs for slow calls there, which a benchmark can miss.
- **Fresh evidence goes first.** A test just failed on main, or an issue
  with its trace or log, beats an older one picked by priority and age.
  Say which was picked and why.

## 2. Root causes only

- **A workaround closes nothing.** A give-up, longer wait, skipped case
  or retry ends one occurrence, and the cause returns unrecognised.
- **Nor does a change that only helps find the cause,** and it says so.
- **Close only on a proven cause,** shown failing before and passing
  after. A change to the likeliest gap says it works on the issue, leaves
  it open and says what the next failure will show. A closing keyword
  never makes a choice the user has not made.
- **When a few fixes have not found the cause, stop fixing.** The next
  change only captures it (stacks of every program involved, a log line,
  a kept trace) and changes no behaviour. One captured stack ended five
  guessed fixes.
- **No product workaround for a failure only test machines have.** A
  guard for a CI-only hang turned a hardware path off for every user.
  Remove shipped guards no real log has seen fire.
- **A diagnostic that only cries wolf comes out,** since it buries real
  signals.

## 3. Problems that come back

- **One occurrence is an incident. A second with the same cause makes a
  problem,** as in established incident management: one issue over both,
  stating the cause as known, every fix tried and why the cause stayed.
  It takes its incidents' highest priority, closes after its last, and
  reopens when the cause returns, since the named cause was incomplete.
- **Search closed issues too,** by test name, the function the stack
  stopped in and the error's words. One hang got five issues and five
  fixes.
- **One issue per flaky test,** each sighting a comment with run,
  commit, test and output.
- **Check the work in flight first** for the same cause, fix it once at
  the cause, and tell the others.

## 4. When fixes stack up

- **The second fix of one symptom names the problem. The third moves the
  decision.** A special case per state breaks on the next state. Replace
  them with one rule every state goes through, owned by the side holding
  the state, asked after every event. A layout bug returned four times
  until one shared reference replaced one per state.
- **Count what the fixes added:** flags, effects, counters, tickets,
  polls. One per fix means no rule owns the decision.
- **Two places decide one thing: delete one.** Making the second agree
  keeps the race.
- **After two or three rounds of tuning nobody can feel, stop** and reuse
  a part that solved the same shape of problem. When each fix makes new
  bugs, test the assumption that forced the design.
- **The same test fix twice means one helper,** see the testing skill.
- **Fix the instruction that misled** agents into a repeated mistake, in
  place. A new rule beside it is a second hole, and the opposite extreme
  is wrong too.

## 5. Issues

- **Open work is in the issue tracker, never in rows of a file,** where
  changes conflict, numbers collide and status drifts. A kept history
  file is frozen by a check.
- **An issue follows the template,** read by hand when using an API,
  which skips the form:
  - a title a person would say,
  - 30 words or fewer: what goes wrong, who saw it and where, what gets
    better,
  - Seen: the report, the log lines or the run,
  - Cause: a few words traced to file and name, or unknown and what
    would find it, the trace first,
  - What to build, in one place, as a plan, never as a question.
- **Read the whole rule an issue cites,** exceptions included, and search
  for a duplicate first.
- **Log only what someone can meet,** saying who and what breaks. "Can",
  "may" and "likely" are not evidence.
- **Labels from an established scheme,** priority and size, each with a
  few words why, for the user to correct. A failing test or red main is
  critical, a flaky or hanging one at least important, since an untrusted
  test checks nothing.
- **An issue shows its progress and what is known:** a checklist on top
  when it spans several pull requests, ticked as each merges. Whoever
  learns a cause, a trace or a failed reproduction (with its count) edits
  the body at once.
- **Take only free work:** check, and say, that no pull request or other
  agent holds the issue.

## 6. Triage

On a schedule, in a fixed order, most current first:

1. **Every red run on main since the last triage,** scheduled ones
   included, is matched to an issue.
2. **Missing labels** are set.
3. **Every problem is judged by who it happens to,** section 1.
4. **What can close, closes,** with one line why and the matching
   reason:
   - done on main,
   - its reason is gone,
   - a duplicate, its additions copied to the one that stays,
   - nothing to act on until it fails again, no new trace since: not
     planned, saying what the next failure will show and that it reopens
     then,
   - fixed on the way, with the count of runs passed since.
5. **What changed is rewritten:** a mostly done issue to what is left,
   changed facts to what is measured now.
6. **Incidents of the period are grouped under problems,** section 3.

Triage changes no code and none of the user's labels or words. It ends
with a linked list of every change for the user to check.

## 7. A fix leaves fewer issues than it found

- **A fix closes what it picked,** and fixes its review's findings on the
  same code path instead of logging them.
- **A small thing the user asks for on the way joins the open change**
  as its own commit, named in the description.
- **Clean up now, not later,** since later is mostly never: fields read
  by nothing, code only tests call, settings of removed jobs, debug
  commits, fallbacks that cannot run.
- **Fix every claim the change made false:** search docs, comments,
  decision records and templates for each renamed term and changed
  behaviour.
- **Log only what the change did not cause and cannot do now,** linked
  from the change.

## 8. Keeping the architecture

- **The architecture is written down** in one document read before every
  change: layers and their allowed uses, where each kind of thing goes,
  invariants, boundaries. A new kind of thing adds its line.
- **It lists what not to copy,** code breaking its rules today, each with
  its fixing issue, because agents copy what is nearby. An item leaves
  when its fix lands.
- **A change to the architecture is worked through as several candidate
  designs first,** each from its own principle: who knows whom, what
  changes together, where state lives, what runs together, what a test
  must build. Count on the real code what moves and which names cross
  each new boundary. Variations count as one. The smallest, with no new
  state, is always a candidate.
- **The choice is written as a decision record,** dated, not numbered,
  so open changes never collide, ending with its cost, what it buys and
  how it would be shown wrong. A later record replaces it, never a
  rewrite, and the old one says so.
- **The drawings change with the text,** since a lying map is worse than
  none. A test fails when a path, name or drawing in the docs is gone.
- **One rule, one home.** A rule in two layers moves to one, and the
  other asks for or receives the answer. A shared table of cases between
  two copies lowers drift but keeps the copy.
- **The bottom changes least.** A type every feature edits puts the most
  changed thing under everything, so each feature owns its data behind a
  small interface. Count commits per part, replay recent ones against a
  proposed layout to see how many still touch the core, and hold layers
  with an import linter in allow-list mode.
- **Hot files are split,** as parallel changes conflict there.
- **Code that only tests call is dead.** Run the unused-code check again
  without the tests. A concurrency test of code the product never calls
  proves nothing. One moved to the real path found a race.
- **Note what you rely on inside a dependency** where its updates are
  reviewed.
- **Keep what failed and what the user settled,** with numbers, where the
  next person looks, so neither returns. An accepted exception goes into
  the rule it breaks, with the reason.
- **Now and then, review the architecture against a checkout from weeks
  before.** Mark each finding new, grew, shrank or gone, check the map,
  and open only the few issues that help most.

## 9. Rules that hold

- **A rule lives where the gate reads it:** the checklist that decides
  ready, the work's template, or best, a check. One kept in a memory, a
  brief or an unchecked doc is skipped.
- **A written rule that is still broken becomes a check,** not louder
  words.
- **Numbers hold better than qualities.** Agents keep a 30-word limit and
  drift on "short".

## 10. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| log every risk a review finds | it buries what people meet | log what someone can meet, section 5 |
| fix the bug where it showed | the cause returns on other paths | the cause, on every path, section 2 |
| close the issue when the change lands | the likely gap may not be the cause | close only on a proven cause, section 2 |
| a defensive guard can only help | a test-hardware guard reached every user | no product workaround for a test-only failure, section 2 |
| priority by a home-made questionnaire | reworded every round | an established scheme, section 5 |
| track tasks in a file in the repository | every change conflicts on it | the issue tracker, section 5 |
| refactor later, ship now | later is mostly never | clean up in the same change, section 7 |

## Before saying done

1. The work named its evidence and what gets better.
2. It removed a cause, or says it works on the issue and leaves it open.
3. A recurring cause is one problem with its incidents under it.
4. Nothing it added is a second home for a rule or for state.
5. It leaves fewer open issues than it found, and nothing behind.
6. The docs, drawings and architecture still say what the code does.
