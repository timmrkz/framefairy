# Decision records

One file for each architectural change, written before its code and
merged with it, see Changing the architecture in
[ARCHITECTURE.md](../ARCHITECTURE.md#changing-the-architecture). A record
is never changed after its pull request is merged. A later decision that
undoes it is a record of its own, which names the one it replaces.

The shape is Michael Nygard's
[architecture decision record](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions),
with the five simulations in the middle. A file is named by the date and
what it decides, `2026-10-30-numbers-and-text-in-a-package.md`, and holds:

- **The problem**, in the words of the person who asked, and what was
  measured on the code as it is.
- **Five simulations**, one section each: the principle, the layers it
  gives, what moves, the names that cross each new boundary, what the
  next changes would touch, and what it costs.
- **What converges**, what came back in most of them and what in only
  one.
- **The decision**, the final candidate drawn as layers, each using only
  the ones under it.
- **What follows**, what it costs, what it buys, and what would show it
  was wrong.
