# Skills for any project

The skills in `.claude/skills/` hold what this project learned about
writing tests, reviewing work and keeping a code base easy to change. All
but one are written for any project. They name no person, no product and
no issue, and speak of **the user**, the person the agent works for, who
decides what happens. So the folder of each can be copied into another
repository as it is, and an agent there follows it without wondering who
or what it means.

`internal/naming/skills_test.go` holds them to that: a skill that names a
person, the product or an issue by its number fails `make lint`. The one
skill about this repository alone is `interface`, which says how to change
this app's interface without seeing its window.

| Skill | What it is for | Where it meets this repository |
| --- | --- | --- |
| `testing` | how tests are written, fixed and reviewed, and how a bug is found before it is fixed | [The testing skill in this project](TESTING.md#the-testing-skill-in-this-project) |
| `review` | how a pull request is reviewed by an agent that did none of the work, and how a review is answered | [The review skill in this project](WORKFLOW.md#the-review-skill-in-this-project) |

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
