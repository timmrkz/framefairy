# Jev for clip choice: a concept

A study of whether Jev, the decision model TypeSafe AI released in September
2026, can choose clips in framefairy, and what it would take. Nothing here is
built. Research from 23 September 2026, from the sources listed at the end.
Jev is in early access and several numbers below come from third parties, so
check them against TypeSafe's own docs before building anything.

## The short answer

Jev cannot choose a clip the way the model does today. It never writes, so it
cannot answer with `"keep": [[12, 18], [24, 27]]`. It can only answer
questions we write in advance, with a fixed set of possible answers. So Jev
would never find the story in the transcript. The engine would have to
propose every possible clip first, and Jev would judge each one.

That shape works, and it is cheap and fast. But Jev is a hosted, closed
model with no weights to download, its German is weaker than its English,
and it does not know what the whole episode is about. It fits as a
**ranker next to the language model**, or as an experiment for our own
trained model. It does not fit as a replacement, and it is not a new local
option.

## What Jev is

- **Who.** TypeSafe AI, co-founded by Diogo Almeida, formerly of OpenAI.
  The Hugging Face post that pointed us to it is a community post from an
  account that is not TypeSafe's.
- **What.** A transformer that is not a language model. It reads a text,
  the *state*, and answers typed questions about it in one pass, without
  writing token by token. TypeSafe calls it a System One model.
- **The three questions.**
  - **Noul**: yes or no, answered as the probability that a statement is
    true.
  - **Choice**: one of up to 255 options, with a probability for each and a
    confidence.
  - **Score**: a rating on 2 to 10 ordered levels, as a continuous score and
    its distribution.
- **Many questions at once.** All questions in a request are judged against
  the same state in parallel. More questions barely add time and only cost
  their own tokens.
- **Limits.** 64,000 tokens per request in total, and 32,000 for the state
  plus the longest question.
- **Speed.** 70 to 500 ms per request.
- **Price.** USD 0.042 per million input tokens. Answers are free.
- **Languages.** Trained mostly on English, where it is best. Other
  languages work, but not equally well. No figures for German.
- **Access.** A hosted API only, `POST https://api.typesafe.ai/v1/systemone`
  with the model `jev-latest` and an API key. No weights, no self-hosting,
  no fine-tuning per customer. TypeSafe says it does not train on requests.
- **How to ask.** One broad question does poorly, several narrow ones do
  well. In a published test on phishing mails, one question scored 62.6 %.
  The same task split into five narrow questions scored 89.4 %, and 95.0 %
  once the five answers were weighted by a regression fitted on labelled
  examples.

### About "Jeff"

There is also a project called **Jeff**: an open, MIT-licensed stand-in for
Jev's API that runs locally, on a 400M model called GLiFormer, and on Apple
silicon too. It is a few commits old, the licence of the model weights is not
stated, and its own benchmarks put it well below Jev (66.9 against 75.3 on
JevBench). OpenJev, another stand-in, needs an NVIDIA card with 24 GB. Neither
is ready to ship in a paid app.

## Why the task does not fit as it is

What framefairy asks of the model today:

1. Read the numbered lines of a stretch of the episode.
2. Find the moments worth a short. That is an open search: where does the
   story start, where is the payoff.
3. Condense each moment by choosing which runs of lines to keep.

Jev can do none of these directly. It picks from answers we list, so it can
only judge a clip that already exists. Step 2 has to become "here are 500
possible clips, how good is each", and step 3 "here is one clip, which of its
lines belong to the story".

## The concept

### 1. The engine proposes every clip

A new step in the engine walks the numbered lines and writes out
**candidates**: every run of whole lines that starts at a line start, ends at
a line end and lasts between the shortest and the longest clip length, 18 to
36 seconds with the defaults. Start only where a line follows a pause, and
end only at a sentence end, so the list stays in the hundreds and not the
thousands.

A rough count for an hour: about 700 lines, a start every few lines, a
handful of ends each, so 500 to 1,500 candidates.

### 2. Jev judges each candidate

One request per candidate. The state is the candidate's lines, plus a few
lines before and after, marked as context, so Jev can tell a clip that
starts mid-thought. The questions are the clip rules from `SystemPrompt` in
`engine/select.go`, split into narrow ones, because narrow questions are what
Jev is good at:

| Key | Type | Question |
| --- | --- | --- |
| `payoff` | Noul | The clip contains its own payoff: something happened, a line lands, or it turns out to mean something. |
| `stands_alone` | Noul | A stranger understands the clip without the rest of the conversation. |
| `hook` | Noul | The first sentence makes a stranger want to keep watching. |
| `concrete` | Noul | The clip rests on something specific the guest saw, did or felt. |
| `cut_off` | Noul | The clip stops before the thought it started is finished. |
| `starts_mid` | Noul | The clip starts in the middle of a thought. |
| `quality` | Score, 5 levels | How good a short this would be. |

A request looks roughly like this. The field names follow the third-party
guides and have to be checked against TypeSafe's reference:

```json
{
  "model": "jev-latest",
  "state": "Before:\n[16] ...\nClip:\n[17] Ich habe immer eine tolle Idee.\n...\nAfter:\n[31] ...",
  "questions": {
    "payoff":       {"type": "noul",  "question": "The clip contains its own payoff ..."},
    "stands_alone": {"type": "noul",  "question": "A stranger understands the clip ..."},
    "quality":      {"type": "score", "question": "How good a short is this?",
                     "levels": ["poor", "weak", "fair", "good", "great"]}
  }
}
```

The answers become one number per candidate, a weighted sum. At first the
weights are set by hand. Later they come from our own records, see step 5.

### 3. The engine picks the clips

The best candidates that do not overlap, up to 12, the default count. A
greedy pass over the sorted list does it: take the best, drop everything
that overlaps it, repeat. This is plain Go and needs no model.

### 4. Condensing, the crisp part

For each chosen clip, one more request. The state is the clip's lines, and
there is one Noul per line: "Line 23 carries the story, rather than hedging,
a restart or filler." Lines under a threshold are dropped, the rest become
the runs, and the engine's usual rules take over from there: `--keep-pause`,
whole words, trimmed hesitation at the edges. A dropped line in the middle of
a clause would break the rule to keep whole clauses, so a line only goes when
both neighbours stay whole sentences, or when it is filler by itself.

This is the step Jev is weakest at. Each line is judged on its own, while
whether a line can go depends on what the lines beside it say. Keep the
language model's condensing if this does not hold up.

### 5. Weights from our own records

The training records in `<episode>.framefairy/training/` already say which
clips were rendered, edited, played and passed over, or removed. That is
exactly the labelled data the phishing test used to go from 89 % to 95 %:
fit the weights of step 2 on it. This belongs in `framefairy-train`, as a
command that runs the questions over recorded candidates and writes the
weights, never in the app.

### What it costs

For an hour, 1,000 candidates at about 400 tokens each, context included,
are 400,000 tokens, under 2 cents. Condensing 12 clips adds almost nothing.
Sent 20 at a time at 70 to 500 ms each, it takes seconds to half a minute.
Today's local run reads the transcript in about 45 seconds before it writes
anything.

### Where it plugs in

A third way to choose clips, next to the Anthropic API and the local model
in `engine/language.go`, ending in the same `PlanEntry` list that
`ValidatePlan` returns today. Everything after that, plans, windows, edits,
cuts, renders and training records, stays as it is. Answers are untrusted
like any model answer: a probability outside 0 to 1 or a missing key refuses
the candidate. A new answer shape means a new `PromptVersion` and a new kind
of record in `docs/TRAINING.md`.

## What speaks against it

- **Not local.** Jev is a hosted API with no weights, like the Anthropic
  option. It does not give the customer a free local path, which is the
  point of the local model. Customers would need a TypeSafe account and key
  as well as, or instead of, an Anthropic one.
- **Early access.** Prices, limits and the API can still change.
- **German.** Tim's podcast is half German, and Jev is mostly English. This
  has to be measured on real episodes before anything else.
- **No sense of the whole.** Each candidate is judged alone. Jev cannot
  know that the same story is told better at minute 40, or what the episode
  is about. The language model reads the whole stretch at once.
- **The engine does the finding.** The engine has to list the candidates,
  and a clip that is not in the list can never be found. A good short that
  cuts a long middle out of a 60 second moment is hard to list in advance.
- **Condensing per line** is fragile, see step 4.

## Recommendation

1. **Do not replace the language model with Jev.**
2. **A cheap experiment is worth it**, in `framefairy-train` and not in the
   app: take the clips already recorded on real episodes, run steps 1 to 3
   over them and see whether Jev ranks the clips Tim kept above the ones he
   removed, in German and in English. That answers the only open question,
   quality, for a few cents and without touching the product.
3. **If Jev ranks well**, the first use is as a ranker behind the language
   model: the model proposes 30 clips, Jev orders them and the best 12 show.
   The language model keeps the part Jev cannot do, which is finding and
   condensing the story.
4. **The idea is worth more than the product.** A small model that judges a
   given clip against narrow questions, rather than writing a whole plan, is
   a good shape for our own trained selection model. It would be small, run
   on any machine, and learn from exactly the records we already keep. That
   is the part to carry into `docs/TRAINING.md` if the experiment is
   promising.

## Sources

- [The Hugging Face post that started this](https://huggingface.co/blog/sora-2/jev-ai-vs-llms-when-should-you-use-a-decision-mode)
- [TypeSafe: Introducing System One models and Jev](https://typesafe.ai/blog/introducing-system-one-models-and-jev)
- [TypeSafe: Models](https://docs.typesafe.ai/models)
- [Tom's Hardware on Jev](https://www.tomshardware.com/tech-industry/artificial-intelligence/typesafe-ais-jev-offers-an-alternative-to-llms-that-claims-to-be-193x-faster-and-445x-cheaper-system-one-type-model-is-bespoke-for-probabilistic-decision-making)
- [heise online on Jev](https://www.heise.de/en/news/AI-model-Jev-to-make-machines-decide-faster-11457071.html)
- [The phishing test: 62.6 % asked once, 95 % split five ways](https://www.beri.net/article/typesafe-jev-typed-decision-model-calibration-decomposition-shadow-eval)
- [Is Jev open source?](https://jevmodel.org/is-jev-open-source/)
- [Jeff, a local stand-in on GLiFormer](https://github.com/jarihu/jeff)
- [A reference on Jev's primitives and limits](https://gist.github.com/pjburnhill/adf8d28efcad9df037bfdece178ef965)
