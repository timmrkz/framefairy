---
name: review
description: How to review a pull request or other finished work, as an agent that did none of it. Beyond code mistakes, whether the work serves the people using the product, is the simplest way there, leaves the architecture better or worse, makes things faster or slower, opens a way in for someone who should not have one, what it costs to run and keep, and what its history says. Short reviews, findings proven at a fair price. Read it before reviewing, before briefing a review agent, and before answering a review. Where another review skill says otherwise, this one wins.
---

# Review

Beyond mistakes in the code, a review asks: is the product better for the
people using it once this is merged, and did the work take the simplest
way there? Correct code can be wrong work: it solves a problem nobody has,
adds a layer a simpler design would remove, makes the product slower,
lets someone in who should not get in, or stands on an unchecked fact. The
diff is the author's answer, not the question.

**The user** is the person the agent works for, who owns the product and
decides what it does, as in the testing skill. **The people using the
product** are who it is made for. The project's instructions say where its
goals are written, where a review goes and who may give instructions.

## 1. Who reviews, and when

- **An agent that did none of the work.** The author's reasoning has turned
  guesses into facts. A fresh reviewer has only the sources.
- **Once, when the work is done** and the change's tests pass. Read CI as
  it stands, do not wait. Later rounds only when the user asks.
- **The brief hands over sources, not conclusions:** the user's own words,
  the issue, the product's goals as the project writes them, the pull
  request and its commits, and where the architecture is written, which
  the reviewer reads itself, section 7. Never the author's plan or
  summary. "Check that X is right" gets X confirmed.
- **The brief carries every decision the user has made since,** in their
  words: accepted trade-offs, standing rules, choices. Without them the
  reviewer raises settled points, the largest group of findings answered
  as wrong.
- **A design change comes with its candidates,** every design worked
  through and the one chosen, a claim the reviewer works through itself,
  section 7. So is the work's description, checked like the code.
- **A rewrite gets a fresh review.** The old one says nothing about it.
- **A later round starts with the earlier findings** by number, fixed,
  partly or not. A fix often moves a problem instead of removing it.

## 2. The goal first

Before the diff and its description:

- **Write what the people using the product should get from this work,**
  in one or two sentences in their words, from the user's request and the
  product's goals. If you cannot, that is the first finding.
- **Then compare with what the work does:**
  - The ask, or an easier one beside it? The issue's list can be met while
    the person's complaint is not.
  - The cause, or the symptom: a retry, a special case, a workaround?
  - Toward the product's people, platforms and promises, or away?
  - Would the user recognise their ask? What changed that nobody asked for?
  - Who meets the problem: a person using the product, shown by a report
    or log, only the test environment, or only a code reader? Product code
    changed for a failure only a CI runner's virtual hardware showed is a
    must fix, see the maintenance skill.
  - Is the named cause the cause? Read the failure's trace or log. Traces
    have turned guessed causes into seen ones and exposed wrong ones.
- **A goal finding outranks every other.**
- **The reviewer raises the direction, the user decides it.** A finding
  that would change the ask is a question to the user with a
  recommendation, never an order to the author. Every other finding is the
  author's to act on at once, section 13.

## 3. How far from what matters

Between what people care about and the changed line lie layers: feature,
design, engine, implementation, tests and tools, and the assumptions under
each. The further down the work is, the likelier the review polishes an
engine serving a feature nobody wants.

- **See the whole chain before judging one link.** Follow the changed line
  to the person who would notice it. If the chain breaks, nobody would
  notice, or nobody asked for where it leads, that is the finding, above
  everything below it.
- **Digging deeper does not show the direction.** A test traced correctly
  through every branch can hide that the pull request goes the wrong way.
  Before a detail, ask whether its work should exist.
- **Ask the one question closest to what matters.** "Why does this need a
  workaround at all?" removes what "is the workaround correct?" would only
  polish.
- **The simplest solution is usually the best:** fewer parts are faster,
  cheaper, easier to test and change. A simpler way to the same result is
  the finding, above the bugs in the complex one. Simplicity is what a
  reader must hold, concepts, moving parts, branches, files, and moved
  complexity is not removed.

## 4. The signals around the work

Read the history before the diff.

- **The same kind of fix again and again means something underneath is
  broken.** Check recent issues and pull requests in the area. At the third
  fix of a kind, the kind is the finding, with what it points to.
- **Look at what the issues share.** A week of fixes that ends with more
  open issues than it began treats symptoms. When most new issues are one
  kind, say one test failing on a busy machine, the effort belongs there.
- **Effort growing while value does not:** tools, tests or process grow
  while problems people hit in the product wait.
- **A small change that came out large has rebuilt something,** like one
  lint rule in hundreds of lines with a hand-made baseline, installer or
  fixtures the tool already has.
- **Work in flight.** Search open pull requests for the same cause. Fixing
  a test for a cause another one removes at its source is work done twice.

## 5. Cost and performance

- **Keep maintenance small.** Every line, setting, test, workflow and
  dependency is kept up as long as it lives. Prefer what leaves less.
- **Every review has a performance section,** like the ones on
  architecture, section 7, and security, section 8: faster, slower or the
  same, for the people using the product and for builds and CI, and more
  memory, disk or download or not.
- **The work's own answer is a claim.** It says unasked whether it is
  faster or slower, by how much and how it knows. Missing is a must fix.
  Then follow the change to what runs more or less often, waits on
  something new, is read, drawn or started again, or holds more memory. A
  path a person waits on, changed with no measurement against main, is a
  finding.
- **The claim is about the whole wait,** from gesture to result on screen,
  in a case the product really produces, on the commit under review. Not
  an inner call, one step of a job, the best of several runs, or a case the
  product never sends. "99 % faster" once timed a call that returned
  before the screen changed, and "31 % faster" was one step, under 1 %
  overall.
- **Recompute the numbers** from the raw runs and rerun the benchmark on
  the head commit. An earlier commit's numbers, or two commits pooled in
  one row, are no evidence.
- **A faster CI is a shorter critical path.** Work skipped beside longer
  work saves nothing.
- **Name every cost, per instance and on the rare path:** memory per item
  times items kept, a per-call bound that should be per machine, every
  fallback and first download, processor time taken from work running at
  once. Costs hide in fallbacks and background copies.
- **Flag what gets measurably worse,** with the number against main: slower
  where speed matters to people, more memory, download or disk, more money
  or tokens per run, slower builds and CI. It goes in only with a trade-off
  worth it, named in the work: what got worse, by how much, what it buys.
  Worse and silent is a must fix. Slower the user has not accepted is a
  question for the user.
- **What cannot be measured is weighed:** more steps or more to learn for
  a person using the product or the next developer. A should fix when a
  simpler way exists, a question for the user when the gain may be worth
  it.

## 6. The facts it rests on

- **List the assumptions the diff does not show:** what people using the
  product have and do, what a tool does at the version used, what a number
  is, what the user wants, what a rule says.
- **Mark each** checked with its source, wrong, or unchecked. Unchecked is
  a finding, wrong a must fix. Faster, smaller or more reliable without
  numbers against main is an assumption.
- **Check a fact about an outside service with one read-only call,** and
  name it. One showed that time blamed on busy runners was a queue for
  scarce ones.

## 7. The architecture

Every review says whether the architecture gets better, stays the same or
gets worse. Working code can put a rule in the wrong place, and a design
wears away one green pull request at a time.

- **Read the architecture as the project writes it,** before the diff:
  layers and which may use which, where each kind of thing goes,
  invariants, boundaries, the code not to copy. Not the code beside the
  change, which may be exactly that code.
- **Ask which side should own the work, before asking where in that
  side it goes.** The issue and the work start where the trouble showed,
  and checking only against the written lines accepts that frame. Decide
  the owner from what each layer is for: a layer that only shows things
  has nothing to tune. A performance problem in a thin layer, a client
  that guesses, waits or keeps order, or a timer, cache or read ahead
  hiding a call, usually means the layer asks for what it could simply
  hold, or decides what another layer owns. A fetch policy is logic too.
  Tuning a symptom of a broken design, instead of removing the cause, is a
  must fix, however well it measures.
- **Place every new thing** (function, type, rule, state, call across a
  boundary) where the architecture puts it, by what it is about, not where
  it was first needed, and check it is there.
- **Weigh the change as a whole:**
  - **Better:** moves something where it belongs, removes a copy, or
    closes a named breach.
  - **The same:** everything it adds is where the architecture says.
  - **Worse:** adds to or builds on a named breach, puts a rule in the
    wrong layer, copies a rule across a boundary without a check holding
    the copies together, gives state a second home, or lets a part know
    the one over it.
- **Worse is a must fix** in the same pull request, never an issue for
  later. An older breach is not this change's unless it builds on it or
  adds to it.
- **No written architecture:** judge by the layering the code shows. The
  missing map is a finding.
- **A new kind of thing with no place** is a finding: add the line in the
  same pull request, or put the change where a line exists.
- **A design change is judged by its candidates,** as many as the project
  asks for. Reason through each before reading the author's verdict: a
  real alternative from its own principle, not a straw man or a variant of
  the winner? Worked through on the code as it is, with what moves, what
  crosses each new boundary, what it costs? Does the choice follow, and is
  a better candidate missing? Missing candidates are a finding.

## 8. Security

Every review has a security section, whatever the change is about: a
test, a build script or a doc can open a hole as easily as a login. Ask
one question, once this is merged, can someone do something they could
not do before, or see something they should not? Start from who that
someone could be. Then:

- **Go through the questions in the security skill's
  [checklist](../security/checklist.md):** what comes in, what it runs,
  what it fetches and from whom, what it lets out, what it may do, who
  may give instructions, and whether each gate guards what really runs.
- **Read the work's own security line as a claim.** Missing, it is a
  must fix.
- **Say safer, the same or less safe, with the evidence,** as section 6
  of the [security skill](../security/SKILL.md) says. Less safe is a must
  fix in the same pull request, unless the user accepted the trade in
  words.
- **A finding says who, from where, with what input, and what they
  get.** A hole nobody can reach is not a finding. A hole that was there
  before is logged on its own, unless the change builds on it or makes it
  worse, which makes it this change's must fix. A missing scanner or fuzz target is a
  finding.

## 9. The design and the code

- **One thing, one primitive.** A new use goes through what exists. A
  second copy with its own rules is a finding.
- **In the right place,** the part that owns the idea, section 7.
- **Correct when it fails too:** edge cases, error paths, concurrency,
  state kept twice. No swallowed error, no fallback hiding a broken
  invariant.
- **Safe from misuse,** section 8.
- **Usual async bugs:** a request built from state read before an awaited
  update it needs. An undo that can run before its action answered. A
  fan-out that lost the loop's stop on the first failure or on cancel. A
  write that wakes the writer's own watcher. An effect that writes what it
  reads. Work sent after its owner closed.
- **Nothing left behind,** see the maintenance skill.
- **A cache and a change stamp cover exactly what they serve.** A
  persisted cache's key includes the version of the code computing the
  value, checked in the shipped build, where flags may strip it. A stamp
  is made from exactly what its reader reads: too broad recomputes
  everything, too coarse misses changes.
- **A persisted format says what an older reader does with it.** An older
  build may rewrite a new file and lose what it did not understand. Keep
  old readers working, or name the break for the user to accept.
- **Docs and comments say what the code does now.** Search every renamed
  term and altered behaviour. Docs left false are among the most common
  findings.
- **The project's written rules hold,** quoted exactly when cited. Where a
  tool could check one, the missing check is the finding.
- **Name the move,** the restructuring that fixes it, not only the problem.
- **Not a finding:** what a linter holds, a taste the project does not
  state, an older problem. One worth fixing is logged on its own.

## 10. The tests

The testing skill wins here. Look for a test that passes by coincidence,
checks the code against a copy of its own rule, waits on the clock, or
guards nothing anyone would miss, and for the missing test of what the
person saw.

The cheapest, most productive check: for each behaviour the change adds,
remove the line that makes it in a scratch copy and run its test. Still
green guards nothing. Check the fixture can show the case at all, and that
any probe or benchmark the work relied on is committed.

## 11. Proof, at a fair price

- **Each finding says how it is known:** run, broken, measured, or read,
  with file and line.
- **The cheapest proof that settles it.** Read before running, one test
  before the suite, one broken line before a harness. Read CI's results,
  do not rerun them.
- **Try once to disprove each finding,** and drop it if it falls. Findings
  answered as wrong failed four ways, check those first: a mechanism
  assumed, a platform's behaviour not known, a fix costing more than it
  buys, a clock flagged as a wait when it only paces a live input.
- **A finding known only by reading says how sure it is,** sure, likely or
  unsure, and what would settle it.
- **Judge before reading the author's judgement.** For labels, triage,
  severity or design candidates, do your own pass, then compare, and
  explain each disagreement by the rule that reads two ways.
- **Expensive proof is a finding in itself:** the product is hard to check
  there. Report how sure you are and what would settle it, and ask before
  spending hours.
- **Spend time and tokens on purpose.** A review that takes hours, or
  repeats the same costly runs every time, is doing something wrong, and
  says what.

## 12. Writing the review

Short: what is going on and where to go next. One screen is read, more is
skimmed. The review is a file where the project says, never where it
could read as the user's instruction. In this order:

1. One line: the commit read, whether it merges, whether CI passed.
2. The goal, in a sentence.
3. The verdict, in two or three sentences: on the goal, the distance from
   what matters, the work as a whole.
4. The architecture, always, under its own heading: which side should own
   the work and whether it does, then better, the same or worse, with
   where each new thing went and where it belongs. Worse is also a must
   fix finding.
5. The performance, always, under its own heading: faster, slower or the
   same, for whom, by how much, measured or read from the code. Whether the
   work's own answer holds.
6. The security, always, under a heading of its own: safer, the same or
   less safe, in a sentence, with what the change lets in or out and who
   could use it. Whether the work's own answer holds. A less safe is
   also a must fix finding.
7. The findings, numbered, most important first, each led by 30 words or
   fewer, then a few lines: weight,
   where, what is wrong and how a person would hit it, how it is known, the
   fix.
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
- **Evidence is a line:** what was run and what came out, never the log.
- **No praise, no "looks good".** Say what is right only where it decides
  something.

## 13. Answering a review

The author acts on it at once. The user sees it but does not approve it,
and nobody waits for them.

- **Post the review for the user,** the file and the findings by number,
  and start on it in the same turn.
- **Check each finding before acting.** Reviewers are wrong too. Agree
  with nothing for its own sake.
- **Each finding is fixed, shown wrong with the reason, or logged when it
  lies outside this work and people using the product can run into it,**
  and reported by number when done. None is skipped to save time, and one
  left undone is said and logged.
- **Only a choice that changes what the user asked for goes to the user,**
  as one question with a recommendation. The rest goes ahead meanwhile.
- **Follow up the reviewer's unchecked questions like findings.** One
  about a download that failed halfway turned out to block every later
  download of that file.

## 14. Where other advice says otherwise

| Common advice | Why not | Instead |
| --- | --- | --- |
| approve when the change improves code health | healthy code can make a worse product | approve when the product is better and the way there simple |
| flag only what is on the changed lines | the goal, the distance, the history, the cost, the architecture and what an attacker could do are on no line | they are findings, sections 2 to 5, 7 and 8 |
| verify every finding fully first | proof can cost more than the finding is worth | the cheapest settling proof, section 11 |
| leave test coverage out unless asked | an untested promise breaks unseen | the testing skill |
| give the reviewer the author's plan or summary | it inherits the author's assumptions | the user's words and the sources, plus a design change's candidates as a claim |
| always say what was done well | praise is noise in a to-do list | what is right only where it decides something |
| judge the design by the code around the change | that code may be what not to copy | the written architecture, section 7 |
| weigh performance only when the change is about speed | any change can slow something, unasked | a performance section in every review, section 5 |
| look at security only when the change touches a login, a key or input | a workflow, a log line or a new package opens a hole as easily | a security section in every review, section 8 |
| review after every task | many reviews of half a design | once, when the work is done |
| post the review as pull request comments | they may read as instructions | the place the project names |

## Before saying the review is done

1. The goal is written, and its verdict comes first.
2. The work was followed to what people care about, and a simpler way is
   named or ruled out.
3. The history was read for repeated fixes.
4. Every added cost is named with its trade-off.
5. The architecture section says which side owns each piece and whether
   it is there, then better, the same or worse. Worse is a must fix.
6. The performance section says faster, slower or the same, and how it is
   known. Slower without the user's yes is a finding.
7. The security section says safer, the same or less safe, and what
   the change lets in or out, and a less safe is a must fix.
8. Every assumption is checked, wrong or named as unchecked.
9. Every finding says how it is known, at a fair price.
10. It fits on a screen.
