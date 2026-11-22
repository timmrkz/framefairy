---
name: review
description: How to review a pull request or any finished piece of work, as an agent that did none of it. Beyond the mistakes in the code, whether the work serves the people using the product, whether it is the simplest way there, whether it leaves the architecture better or worse, whether it makes things faster or slower, whether it opens a way in for someone who should not have one, what it costs to run and to keep, and what the history around it says. Short reviews, findings proven at a fair price. Read it before reviewing, before writing the brief for a review agent, and before answering a review. Where another review skill says otherwise, this one wins.
---

# Review

A review looks for mistakes in the code, and that stays part of it. But it
goes further. Its question is whether the product is better for the people
using it once this is merged, and whether the work took the simplest way
there. Code can be correct and the work still wrong: it solves a problem
nobody has, it adds a layer where a simpler design would remove one, it
makes the product slower, it lets someone in who should not get in, or
it stands on a fact nobody checked. A
reviewer who only reads the diff cannot see that, because the diff is the
author's answer, not the question.

Two people are named here, as in the testing skill. **The user** is the
person the agent works for, who owns the product and decides what it
should do. **The people using the product** are the ones it is made for. A
project's own instructions say where its goals are written, where a review
goes and who may give instructions. This skill says what to look for.

## 1. Who reviews, and when

- **An agent that did none of the work.** The author carries its own
  reasoning, and that reasoning has turned guesses into facts. A fresh
  reviewer has only the sources, so it sees what the work rests on.
- **Once, when the work is done** and the tests of the change pass,
  without waiting for CI, whose results it reads as they stand. A review
  after later rounds is the user's to ask for.
- **The brief hands over sources, not conclusions:** the user's own
  words, the issue, the product's goals as the project writes them, the
  pull request and its commits. Not the author's plan or summary of what
  the user wants. A brief that says "check that X is right" gets X
  confirmed.
- **The brief carries every decision the user made since the request,**
  in their words: trade-offs accepted, standing rules, choices between
  options. A reviewer without them raises findings the user already
  settled, and that was the largest group of findings answered as wrong.
- **A rewrite is new work.** A pull request restarted or redesigned gets
  a fresh review of the new version. The old review says nothing about
  it.
- **A later round starts with the earlier findings,** each by its number,
  fixed, partly or not, then the new ones. A fix often moves a problem
  rather than removes it.
- **The description of the work is a claim,** checked like the code.
- **The brief names where the project writes its architecture,** and the
  reviewer reads it itself, section 7. A brief that leaves it out gets a
  review of whether the work matches its issue, not of its design.
- **A change to the design comes with its candidates.** The brief hands
  over every design the author worked through and the one it chose, as
  the project keeps them. They are a claim, like the description, and
  the reviewer works through them itself, section 7.

## 2. The goal first

Before the diff, and before the pull request's own description:

- **Write what the people using the product should get from this work,**
  in one or two sentences, in words they would use, from the user's
  request and the product's goals. If it cannot be written, that is the
  first finding.
- **Then read what the work does, and compare:**
  - The ask, or an easier one next to it? The issue's list is met and
    the person's complaint is not.
  - The cause, or the symptom? A retry, a special case, a workaround on a
    broken design.
  - Toward the product, or away from it? Its people, its platforms, its
    promises.
  - Would the user recognise their ask in what it does?
  - What else changed that nobody asked for?
  - Who meets the problem it fixes? A person using the product, shown by
    a report or a log, only the test's environment, or only someone
    reading the code? Product code changed for a failure only a CI
    runner's virtual hardware showed is a must fix, see the maintenance
    skill.
  - Is the cause it names the cause? Read the trace or the log of the
    failure it fixes. Traces turned inferred causes into seen ones, and
    found second causes and wrong ones.
- **A goal finding outranks every other.** When the goal is wrong, the
  code findings matter little.
- **The reviewer raises the direction, the user decides it.** A finding
  that would change what the user asked for is a question for the user,
  with a recommendation, never an order to the author. Every other
  finding is the author's to act on at once, section 13.

## 3. How far from what matters

Between what the people using the product care about and the line a
change touches lie layers: the feature, its design, the engine under it,
the implementation, the tests and tools around it, and the assumptions
each one rests on. Deeper, further down, further along: these are ways of
speaking, not a measure. What they name is distance. The further the work
is from what people care about, the more likely the review ends up
arguing about assumptions that rest on assumptions, about details of an
engine that serves a feature nobody wants.

- **See the whole chain before judging one link.** Follow the work from
  the line it changes to the person who would notice it: which feature,
  which design, which promise. If the chain breaks, if nobody would
  notice, or if it leads to something nobody asked for, that is the
  finding, and it outranks everything found along the way.
- **Digging deeper does not show the direction.** Following a broken test
  through every branch of its logic can be careful and correct, and still
  miss that the whole pull request goes the wrong way. Before going into
  a detail, ask whether the work it belongs to should exist.
- **Ask the right question, not more questions.** A review does not grow
  more branches by asking more. It looks for the one question, as close
  to what matters as it can be asked, whose answer makes the layers
  under it unnecessary. "Why does this need a workaround at all?" can
  remove what "is the workaround correct?" would only polish.
- **The simplest solution is usually the best one.** Fewer parts are
  usually faster, cheaper to run, easier to test and easier to change.
  When the review finds a simpler way to the same result, that is its
  finding, and it outranks the bugs in the more complex way.
- **Simplicity shows in what a reader must hold:** the concepts, the
  moving parts, the branches, the files. A change that moves complexity
  without removing any is not simpler.

## 4. The signals around the work

The work sits in a history. Read it before judging the diff.

- **The same kind of fix again and again means something underneath is
  broken.** Look at the recent issues and pull requests in the same area.
  If this is the third fix of its kind, the finding is the kind, and the
  review names what it points to.
- **Look at what the issues have in common, not only at each one.** A week
  of merged fixes that ends with more open issues than it began with says
  the work treats symptoms. When most of the new issues are one kind, for
  example one sort of test failing on a busy machine, that kind is where
  the effort belongs.
- **Effort that grows while value does not is a signal.** Work on tools,
  tests or process that keeps growing while the problems people run into in
  the product wait.
- **A small change that came out large has rebuilt something.** One lint
  rule in hundreds of lines usually means a baseline, an installer or a
  set of fixtures written by hand that the tool already has.
- **Work in flight.** Search the open pull requests for the same cause. A
  fix to one test for a cause another open pull request removes at its
  source is work twice.

## 5. Cost and performance

- **Maintenance never ends, so it is kept as small as it can be.** Every
  line, setting, test, workflow and dependency is kept up for as long as
  it lives. Prefer the solution that leaves less to keep up, today and
  after the next change.
- **Every review has a performance section,** whatever the change is
  about, the way it has one on the architecture, section 7, and one on
  security, section 8. It says whether the change makes things faster,
  slower or the same, for the people using the product and for the
  builds and CI, and whether it needs more memory, disk or download.
  Performance is a cost of every change, not only of the ones that set
  out to make something faster.
- **Read the work's own answer as a claim.** The work says itself,
  before anyone asks, whether it is faster or slower, by how much and
  how it knows. Missing, that is a must fix, because the user is left
  to ask. Then follow the change to what runs: what now runs more often
  or less, waits on something new, is read, drawn or started again,
  holds more in memory. A path a person waits on, changed with no
  measurement against the main branch, is a finding.
- **The claim is about the whole wait,** from the gesture to the result
  on screen, in a case the product really produces, measured on the
  commit under review. A fast inner call, one step of a long job, the
  best of several numbers, or a case the product never sends is not the
  claim. "99 % faster" once timed a call that returned before the screen
  changed, and "31 % faster" was one step of a job, under 1 % overall.
- **Recompute the numbers.** Rebuild the tables from the raw runs and run
  the benchmark again on the head commit. Numbers from an earlier commit,
  or runs of two commits pooled into one row, are no evidence.
- **A faster CI is a shorter critical path.** Work skipped beside longer
  work saves no time.
- **Name every cost, per instance and on the rare path:** memory held
  per item times the items kept, a bound per call that should be per
  machine, the size of every fallback and first download, processor
  time taken from other work running at once. Costs hide on the fallback
  path and in copies kept in the background.
- **A change that makes something measurably worse is flagged,** with
  the number against the main branch:
  - slower for the people using the product, where speed matters to them
  - more memory, a bigger download, more disk
  - more money or tokens per run
  - slower builds and slower CI
- **It goes in only with a trade-off worth it,** named in the work: what
  got worse, by how much, and what it buys. A change that is measurably
  worse and says nothing about it is a must fix.
- **Slower needs the user's yes as well.** A slower change that names
  its trade-off, which the user has not accepted yet, is a question for
  the user.
- **What cannot be measured is weighed, not counted.** More steps or more
  to learn for a person using the product, more for the next person to
  work through: these are maintenance, a should fix when a simpler way
  exists, and a question for the user when the gain may be worth it.

## 6. The facts it rests on

- **List the assumptions the work stands on** that the diff does not show:
  what the people using the product have and do, what a tool or library
  does at the version used, what a number is, what the user wants, what a
  rule says.
- **Mark each one** checked, with its source, wrong, or unchecked. An
  unchecked assumption the work stands on is a finding, a wrong one is a
  must fix.
- **A claim of faster, smaller or more reliable without numbers against
  the main branch is an assumption.**
- **A fact about an outside service is checked with one read-only
  call,** what an API accepts, which jobs waited for a runner, and the
  call is named. One such call showed that time blamed on busy runners
  was a queue for scarce ones.

## 7. The architecture

Every review judges the architecture, whatever the change, and says
whether the change makes it better, leaves it as it was, or makes it
worse. Code can work and still put a rule in the wrong place, and that
is how a design wears away, one pull request at a time, each of them
green.

- **Read the architecture as the project writes it,** before the diff:
  its layers and which may use which, where each kind of thing goes, the
  invariants, the boundaries, and the code it names as not to copy. Not
  the code beside the change, which may be the very code not to copy.
- **Ask which side should own the work, before asking where in that
  side it goes.** The issue and the work both start from where the
  trouble showed, and a review that only checks the change against the
  written lines accepts that frame. So for every piece of the work, ask
  first which layer owns it at all, from what the layers are for: a
  layer that only shows things should have nothing to tune. A
  performance problem in a thin layer, a client that has to guess, wait
  or keep order, or a timer, a cache or a read ahead added to hide a
  call, usually means the layer is asking for something it could simply
  hold, or deciding something another layer owns. A fetch policy is
  logic too. Then say whether the change removes that cause or only
  moves it, and a change that tunes the symptom where the design is
  broken is a must fix finding, however well it measures.
- **Place every new thing the change adds** in the layer and the part the
  architecture gives it: a function, a type, a rule, a piece of state, a
  call across a boundary. A rule is placed by what it is about, not by
  where it was needed first. Then check that it is there.
- **Weigh the change as a whole:**
  - **Better:** it moves something to where it belongs, removes a copy,
    or closes a breach the architecture names.
  - **The same:** everything it adds is where the architecture says.
  - **Worse:** it adds to a breach the architecture names, builds on one,
    puts a rule in a layer that should not hold it, copies a rule across
    a boundary without the check that holds the two copies together,
    gives state a second home, or lets a part know one over it.
- **Worse is a must fix,** however good the rest. The fix is the change
  made where it belongs, in the same pull request, never an issue for
  later.
- **A breach that was there before is not this change's,** unless the
  change builds on it or adds to it.
- **A project that writes no architecture** is judged by the layering its
  code shows, and the missing map is a finding of its own.
- **A new kind of thing the architecture has no place for** is a finding
  too: the architecture gets the line it lacks in the same pull request,
  or the change goes where a line already is.
- **A change to the design is judged by its candidates,** as many as the
  project asks for. Reason through each one yourself before reading the
  author's verdict: is it a real alternative from a principle of its own,
  not a straw man or a variation of the winner? Is it worked through on
  the code as it is, with what moves, what crosses each new boundary and
  what it costs? Then: does the choice follow from the comparison, and is
  a better candidate missing? A change to the design without its
  candidates is a finding.

## 8. Security

Every review has a security section, whatever the change is about, the
way it has one on the architecture and one on performance. A change to
a test, a build script or a doc can open a hole as easily as a change
to a login: a workflow that runs code from a stranger's pull request, a
log line that prints a key, a new package nobody looked at.

The question is simple: once this is merged, can someone do something
they could not do before, or see something they should not see? Start
from who that someone could be, then follow the change through six
questions: what comes in from outside, what it runs, what it fetches and
from whom, what it lets out, what it may do, and who may give
instructions. **What to look for under each, and how a gate that checks
a signature or a sum is judged, is in [security.md](security.md),** read
for every review.

- **Read the work's own answer as a claim.** The work says itself,
  before anyone asks, whether it is safer, the same or less safe.
  Missing, that is a must fix. Then check it against what the change
  really lets in and out.

Then weigh the change as a whole:

- **Safer:** it closes a way in, narrows a permission, or adds a check
  that was missing.
- **The same:** it adds no new way in and takes no check away.
- **Less safe:** it opens a way in, widens a permission, removes or
  weakens a check, or lets a secret or a person's data out.

- **Less safe is a must fix,** however good the rest, fixed in the same
  pull request. A trade-off the user accepted in words is the only
  exception, and it is written in the pull request.
- **A hole that was there before is not this change's,** unless the
  change builds on it or makes it worse. It is logged on its own, and
  marked as urgent when people using the product can be hurt by it.
- **A finding says how someone would use it:** who, from where, with
  what input, and what they would get. A hole nobody can reach is not a
  finding. The cheapest proof is usually one input made to slip past
  the check, run against the code or written as a test.
- **A tool beats a reviewer's eye.** A dependency scanner, a secret
  scanner, a fuzz target on what reads outside input: when a kind of
  hole could be caught by a tool and is not, the missing tool is the
  finding.

## 9. The design and the code

- **One thing, one primitive.** A new use goes through what exists. A
  second copy with its own rules is a finding.
- **In the right place,** in the part that owns the idea, section 7.
- **Correct, also when it fails.** Edge cases, error paths, concurrency,
  state kept in two places. No swallowed error, no fallback that hides a
  broken invariant.
- **Safe from misuse,** section 8.
- **The usual shapes of async bugs.** A request built from state read
  before an awaited update it depends on. An undo that can run before the
  action it undoes has answered. A fan-out that lost the loop's stop at
  the first failure or on cancel. A write that wakes the writer's own
  watcher. An effect that writes what it reads. Work sent after its owner
  closed.
- **Nothing left behind,** see the maintenance skill.
- **A cache and a change stamp cover exactly what they serve.** A
  persisted cache's key includes the version of the code that computes
  the value, checked in the build that ships, where build flags may strip
  it. A stamp that tells a reader something changed is made from exactly
  what that reader reads: too broad and every edit recomputes everything,
  too coarse and changes are missed.
- **A persisted format says what an older reader does with it.** A build
  that cannot read a new file may rewrite it and lose what it did not
  understand. The change keeps old readers working, or names the break
  for the user to accept.
- **Docs and comments say what the code does now.** Search for every
  term the change renamed and every behaviour it altered: docs left false
  by the change were among the most common findings.
- **The project's written rules hold,** quoted exactly when cited. A rule
  a tool could check is better held by the tool, and the finding is then
  the missing check.
- **Name the move, not only the problem:** which restructuring would fix
  it.
- **Not a finding:** what a linter holds, a taste the project does not
  state, a problem that was there before. An old problem worth fixing is
  logged on its own.

## 10. The tests

The testing skill says how tests are written and reviewed, and wins here.
Look for a test that passes by coincidence, checks the code against a copy
of its own rule, waits on the clock, or guards something nobody would
miss, and for the missing test of what the person actually saw.

The cheapest and most productive check: for each behaviour the change
adds, remove the one line that makes it, in a scratch copy, and run its
test. A test that stays green guards nothing. Then check that the
fixture can show the case at all, and that a probe or a benchmark the
work relied on is committed, not left in a scratch folder.

## 11. Proof, at a fair price

- **Each finding says how it is known:** run, broken, measured, or read,
  with the file and line.
- **Take the cheapest proof that settles it.** Read before running, run
  the one test before the suite, break one line before building a
  harness. CI's results are read, not run again.
- **Try to disprove each finding once before reporting it.** One that
  does not survive is dropped. Findings answered as wrong failed in four
  ways: a mechanism assumed and not checked, how a platform behaves not
  known, a fix that costs more than it buys, and a clock flagged as a
  wait when it only paces a live input at real speed. Check those first.
- **A finding known only by reading says how sure it is,** sure, likely
  or unsure, and what would settle it, so the author proves the right
  one first.
- **Judge before reading the author's judgement.** For work that is
  itself a judgement, labels, a triage, a severity, the design
  candidates, do your own pass first, then compare, and explain each
  disagreement by the rule that can be read two ways.
- **When proof would be expensive, stop and say so.** If a finding would
  take hours to prove, that is itself a finding: the product is hard to
  check there, and the way it is checked is what needs fixing. Report it
  with how sure the review is and what would settle it, and ask before
  spending the time.
- **The time and tokens of agents and people are spent on purpose.** A
  review that takes hours, or the same costly runs in every review, is
  doing something wrong, and says what.

## 12. Writing the review

Short. The user wants what is going on and where to go next, not an
essay. A review that fits on one screen is read, and a longer one is
skimmed.

The review is a file, in the place the project names, never somewhere it
could read as an instruction from the user. In this order:

1. One line: the commit read, whether it merges, whether CI passed.
2. The goal, in a sentence.
3. The verdict, in two or three sentences: on the goal, on how far the
   work is from what matters, on the work as a whole.
4. The architecture, always, under a heading of its own: which side
   should own the work and whether it does, then better, the same or
   worse, in a sentence, with where each new thing went and where the
   architecture puts it. A worse is also a must fix finding.
5. The performance, always, under a heading of its own: faster,
   slower or the same, for whom, by how much and how it is known,
   measured or read from the code. Whether the work's own answer holds.
6. The security, always, under a heading of its own: safer, the same or
   less safe, in a sentence, with what the change lets in or out and who
   could use it. Whether the work's own answer holds. A less safe is
   also a must fix finding.
7. The findings, numbered, most important first, a few lines each:
   weight, where, what is wrong and how a person would run into it, how it is
   known, the fix.
8. Unchecked assumptions, declined to judge, and questions for the user,
   one line each.

| Weight | Means |
| --- | --- |
| Must fix | Blocks the merge: a wrong goal, a wrong fact, broken behaviour, a cost with no trade-off, lost or exposed data, the architecture made worse, the product made less safe |
| Should fix | Belongs in this pull request |
| For the user | A product choice only the user can make, with a recommendation |
| Optional | Better, and fine to leave |

- **Fewer, stronger findings.** One structural finding and ten small ones
  is a review about the structural one.
- **The evidence is a line, not a transcript.** What was run and what
  came out, never the whole log.
- **No praise section, no "looks good".** What is right is said where it
  decides something, with its reason.

## 13. Answering a review

The author acts on the review at once. The user sees it, but does not
approve it, and nobody waits for them.

- **The review is posted for the user to see,** with the file and the
  findings by number, and the work on it starts in the same turn.
- **Each finding is checked, then acted on.** A reviewer can be wrong
  too, so the author confirms a finding before fixing it, and agrees
  with nothing for its own sake.
- **Each finding is fixed, shown wrong with the reason, or logged when it
  lies outside this work and people using the product can run into it,** and
  reported by its number once the work is done.
- **Only a choice that would change what the user asked for goes to the
  user,** as one question with a recommendation. The rest of the review
  goes ahead meanwhile.
- **No finding is skipped to save time.** One left undone is said, and
  logged.
- **The reviewer's unchecked questions are followed up too,** like
  findings. One about a stream that failed halfway turned out to freeze
  every later stream of that file.

## 14. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| approve when the change improves the health of the code | healthy code can make a worse product | approve when the product is better and the way there is simple |
| flag only what is on the changed lines | the goal, the distance, the history, the cost, the architecture and what an attacker could do are on no line | they are findings, sections 2 to 5, 7 and 8 |
| verify every finding fully before reporting it | proof can cost more than the finding is worth | the cheapest proof that settles it, section 11 |
| leave test coverage out unless the project asks | an untested promise breaks unseen | the testing skill |
| give the reviewer the author's plan or summary | the reviewer inherits the author's assumptions | the user's own words and the sources. Only the candidates of a change to the design are handed over, as a claim to check |
| always say what was done well | praise is noise in a list of work to do | say what is right only where it decides something |
| judge the design by the code around the change | the code nearby may be what the architecture says not to copy | the architecture as the project writes it, section 7 |
| weigh performance only when the change is about speed | every change can make something slower, and nobody asks until someone notices | a performance section in every review, section 5 |
| look at security only when the change touches a login, a key or input | a workflow, a log line or a new package opens a hole as easily | a security section in every review, section 8 |
| review after every task | many reviews of half a design | once when the work is done |
| post the review as comments on the pull request | the project may read comments there as instructions | the place the project names |

## Before saying the review is done

1. The goal is written, and the verdict on it comes first.
2. The review followed the work to what people care about, and says
   whether a simpler way exists.
3. The history around the work was read for repeated fixes.
4. Every cost the work adds is named with its trade-off.
5. The architecture section says which side owns each piece of the
   work and whether the change puts it there, then better, the same or
   worse, and a worse is a must fix.
6. The performance section says faster, slower or the same, and how
   it is known, and a slower the user has not accepted is a finding.
7. The security section says safer, the same or less safe, and what
   the change lets in or out, and a less safe is a must fix.
8. Every assumption is checked, wrong or named as unchecked.
9. Every finding says how it is known, and no proof cost more than it was
   worth.
10. It fits on a screen.
