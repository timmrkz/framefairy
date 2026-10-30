# Numbers and text the Python way get packages of their own

Step 1 of splitting the engine into packages, after `engine/timed`
(#621). Decided on 30 October 2026, in the pull request that makes it.

## The problem

Tim asked for the engine to be split into packages that follow the map
in [ARCHITECTURE.md](../ARCHITECTURE.md), one step at a time, and said go
on this order on 30 October. The first part to stand alone, framing,
cannot move while it uses helpers that live in the engine.

The helpers in `engine/util.go` are that first obstacle. Measured on
main at b22df65 with the type checker, over the files of `engine/`:

| Helper | Uses | Areas that use it |
| --- | ---: | ---: |
| `fixed`, a number with n decimals | 104 | 9 |
| `roundTo`, Python's round(x, n) | 71 | 9 |
| `Scrub`, text made safe to print and store | 52 | 8 |
| `PyFloat`, a float written to JSON as Python writes it | 31 | 2 |
| `runeLen`, Python's len() on a str | 28 | 5 |
| `pyround`, Python's round(x) | 25 | 3 |
| `toFloat`, `toInt`, a number read from a JSON value | 39 | 7 |
| `pyStr`, `pyRepr`, `pyListRepr`, a value written as Python's str or repr | 38 | 5 |
| `strip`, `fields`, `isPySpace`, Python's whitespace | 26 | 8 |
| `isFinite`, `pysum`, `parsePyFloat`, `pyFloatRepr`, `runePrefix` | 35 | 6 |
| `shellQuote`, a command written for a shell | 4 | 3 |

Every area of the engine uses some of them, and `util.go` changed once in
the 149 commits to `engine/` since the app was brought over. So they are
used by everything and change with nothing.

`toFloat` and `toInt` read more than plans: the probes of ffmpeg, a
transcript, crop marks, a rotation and the caption settings.
`pyRepr` and `pyListRepr` write error messages. `pyStr` writes a plan's
values into the prompt and the log. None of them belongs to one file.

## Five simulations

### 1. One package of numbers and text

All of `util.go` but `shellQuote` becomes one package at the bottom of
the engine, named for what it is, the Python version's behaviour, under
every area. It exports 20 names. One package is the least to learn and
holds no edge inside it. But a reader looking for how a float is written
finds the scrubbing of a model's text beside it, and an area that only
counts characters imports the rounding too. Go's own library keeps the
same two apart, `strconv` and `strings`.

### 2. What changes together

The helpers changed once in a month, so history cannot group them. What
they work on does: numbers (`fixed`, `roundTo`, `pyround`, `pysum`,
`isFinite`, `PyFloat`, `pyFloatRepr`, `parsePyFloat`, and `toFloat`,
`toInt` and `pyStr`, which read a number from a JSON value or write
one), text (`runeLen`, `runePrefix`, `strip`, `fields`, `isPySpace`,
`Scrub`, `pyRepr`, `pyListRepr`), and the shell (`shellQuote`). Three
groups: numbers and text to the bottom now, the shell with running
programs in step 3. One edge joins the first two: `parsePyFloat` and
`toInt` trim with Python's whitespace, so numbers sit over text.

### 3. Where state lives

The helpers hold no state, and none decides where state lives. A file
that is read with `toFloat` is still read by its own area, the helper
only turns one JSON value into a number. So nothing ties a helper to
the plan file or to any other home of state, and all of them but
`shellQuote`, which runs nothing either, can sit under everything. This
principle does not choose between one package and two.

### 4. What Go already has

Four helpers are one line over the standard library: `runeLen` is
`utf8.RuneCountInString`, `fixed` is `strconv.FormatFloat(x, 'f', n,
64)`, `pyround` is `int(math.RoundToEven(x))`, `isFinite` is two `math`
calls. Inlining them removes 173 uses from any package. But `fixed`
alone is 104 places where a short name would become a long call, and the
names say why they exist: the Python version counted characters, not
bytes, and rounded halves to even. Inlining hides that reason at every
call. It is worth it for none of them, and it groups nothing.

### 5. What a test has to build, and where a library lives

Framing, step 6, has to build without the engine. It uses `fixed`,
`roundTo`, `pyround`, `pysum`, `isFinite`, `parsePyFloat`, `Scrub`,
`runeLen`, `strip`, `fields`, `toFloat` and `toInt`. So every one of
those must sit under framing by then, the JSON values too. A small
library could go under `internal/`, beside `internal/wake`, where
nothing outside the module can import it. But these helpers answer to
the engine's files, written the Python way, and nothing but the engine
and the app's Go side uses them. Under `engine/` they say whose they
are. A test of the helpers builds nothing else, so each package's tests
run in milliseconds alone.

## What converges

- All of it but `shellQuote` goes under every area now: 2, 3 and 5. No
  helper is tied to the plan file, which an earlier draft of this record
  claimed.
- Numbers and text are two packages, numbers over text: 2, and 1 by what
  it gives up. 3 and 5 do not choose.
- Under `engine/`, not `internal/`: 5.
- The names stay short and say they follow Python: 1 and 4.
- `shellQuote` goes with running programs in step 3: 2.

Only simulation 4 inlines, and only 1 keeps one package. The choice of
two packages rests on 2 alone, with 1 weighing what one package would
cost a reader.

## The decision

Two packages at the bottom of the engine, beside `engine/timed`:

- `engine/pynum`, numbers rounded, summed, read and written the way the
  Python version did: `Round`, `RoundTo`, `Fixed`, `Sum`, `IsFinite`,
  `Float`, `FloatRepr`, `ParseFloat`, and for JSON values `FloatOf`,
  `IntOf` and `Str`.
- `engine/pytext`, text counted, cut, trimmed and quoted the way the
  Python version did, and scrubbed before it is printed or stored:
  `Len`, `Prefix`, `Strip`, `Fields`, `IsSpace`, `IsControl`, `Scrub`,
  `Repr`, `ListRepr`.

`shellQuote` stays in `engine/util.go` until step 3. `pynum` and
`pytext` were checked as names: neither is used for anything else in the
engine, where `num` and `text` are common local names.

The layers, each using only the ones under it:

```
engine          every area, still one package
engine/pynum    numbers the Python way
engine/pytext   text the Python way
                engine/timed beside them, using neither
```

## What follows

- It costs one pull request of renames, about 450 uses in about 60
  files, done by the type checker like #621.
- It buys the first obstacle to framing cleared whole, and an area's
  numbers and text now come from one place it can see.
- It was wrong if a later step has to reach back into `pynum` or
  `pytext` for something only one area uses, or has to split them again.
