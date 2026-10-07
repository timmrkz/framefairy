# Licences

The spec for selling Frame Fairy: how a licence key is made and checked,
every way one is handed out or changed, and the two programs that do it.
It is batch 5.8 in [GUI-PLAN.md](GUI-PLAN.md), built last. The update
policy and why the code is public are in [UPDATES.md](UPDATES.md).

## Objective

Every sale of Frame Fairy turns into a licence key within seconds, even
when the part of our system that can make keys is switched off, and the
keys stay trustworthy for as long as the product lives. Everything about
it is ours, and it needs next to no looking after.

Who it serves:

- **Buyers** get their key on the thank-you page and by email the moment
  they have paid, can get it back when it is lost, and never need the
  network to use it.
- **Partners**, bundles, deal sites and resellers, sell Frame Fairy
  through a small, stable API, or with keys made in advance.
- **We** issue keys by hand for press and giveaways, revoke abused keys,
  and replace a signing key without any buyer noticing.

It is done when every use case below passes its acceptance criteria
against Paddle's sandbox, and the app accepts exactly the keys we signed
and did not revoke.

## Decisions already made

| Topic | Decision |
| --- | --- |
| What a licence buys | Bought once, every update for ever. Without a key the app does everything, and renders carry the Frame Fairy mark |
| Who sells | Paddle, as merchant of record: payment, VAT and sales tax everywhere, invoices, chargebacks |
| Who makes keys | We do. A key is signed with our Ed25519 private key |
| How keys reach buyers | From a pool of keys signed in advance, handed out by the dispenser, so a sale never waits on the signer |
| How the app checks | Offline, against public keys built into the app. No network, no machine binding, no activation count |
| Refunds | None once the key has been delivered. The buyer agrees at checkout that delivery starts at once and ends the right of withdrawal, as Paddle's buyer terms allow |
| Revocation | Only chargebacks, refunds Paddle makes anyway, and keys posted in public. The list travels in the signed update feed |
| Affiliates | Tolt, on top of Paddle. Neither program knows about them |

On refunds: the decision was no refunds for verified keys. The app never
phones home, so whether a key was entered is something we cannot know.
Delivery is the moment we can see, and the one Paddle's terms rest on.

## Two programs

The work is split by what each half must never do.

- **The dispenser is always on and cannot make a key.** It holds a pool of
  keys signed in advance. It takes Paddle's webhooks, hands the next key
  in the pool to each sale, records which sale got which key, shows the
  key on the thank-you page, emails it, and answers partners. It runs on a
  managed serverless platform with a managed database, so there is no
  server and no operating system of ours to look after.
- **The signer makes keys and is almost never on.** It holds the private
  key and nothing else of value. It has no address anyone can reach: it
  only ever calls out, asks the dispenser how full the pool is, signs a
  batch when it runs low and hands it over. Named keys and partner batches
  come from it directly.

What that buys:

- **A sale never waits on the signer.** A pool of a few thousand keys lasts
  months, and the dispenser warns us long before it runs dry.
- **The private key is never on anything the internet can reach.**
- **The worst leak is small.** If the dispenser's database were stolen,
  what is lost is the unsold keys in the pool. We know exactly which those
  are, revoke them all and refill. A stolen signing key would be far
  worse, and it is the half that is hardest to reach.

The first home for the dispenser is Scaleway, in France: Serverless
Containers run it as an ordinary Go program, with CRON triggers for its
daily work, and Serverless SQL Database is a managed PostgreSQL with
automatic backups. See [Serverless SQL Database](https://www.scaleway.com/en/serverless-sql-database/)
and [CRON triggers](https://www.scaleway.com/en/docs/serverless-containers/how-to/add-trigger-to-a-container/).

## The licence and the key

A key is a few fields, signed. It is `FF1-` followed by the fields and a
64-byte signature in base64url: 108 characters without a name, about 125
with one. It is pasted, never typed.

| Field | Size | Meaning |
| --- | --- | --- |
| format | 1 byte | Key format, 1 today. A later format is a superset, and every format ever issued stays readable |
| signer | 1 byte | Which signing key signed it, so a key pair can be replaced |
| id | 8 bytes | Which key it is, drawn at random by the signer |
| edition | 1 byte | What it unlocks. 0 is Frame Fairy as sold today |
| signed | 2 bytes | Day it was signed, counted from 2026-01-01 |
| name length | 1 byte | How long the name is, 0 to 64 |
| name | 0 to 64 bytes | Whom it is licensed to, in UTF-8, shown in the settings. Empty for every key from the pool |

The signature covers the line `framefairy licence v1`, a newline, and the
fields. The line keeps a licence signature from ever being taken for any
other thing we sign.

A key from the pool carries no name and nothing about the sale. Which
sale got which key is the dispenser's record, not the key's business.

### The key ID

The signer draws each ID at random and keeps every ID it has ever used,
so it draws again in the rare case it repeats one. No two keys share an
ID. People see it in four groups, `652B-757A-8DC4-F04A`: the settings show
it, and it is what support asks for. It is not secret and proves nothing,
the signature does.

### The fingerprint

A fingerprint is the first 16 bytes of SHA-256 over the whole key text.
The revocation list, the genuine lists and both programs' records hold
fingerprints. It is 16 bytes, not 8, because after a signing key leaks,
whoever holds it could try to sign keys until one matches a genuine
fingerprint. At 8 bytes that is within reach of a large computer, at 16
it is not.

### Examples

Three real keys, signed with the test key. Its public half is
`b959ea225fd22150952415eceb782b933bb8651d2df926492b93d8c205e32ee1`, made
from the seed SHA-256 of `framefairy test signer, never shipped`, and no
build we ship accepts it. The tests check these three keys.

**From the pool**, signed on 28 October 2026, ID `652B-757A-8DC4-F04A`:

```
FF1-AQBlK3V6jcTwSgABLABmp524aGzqP1MSVRahBH_9Q8Z57h2_HByz9RmWNvbzEge1J5szJ20G0ejY1lZw01BgFbfKP2SMd7PwcgEVB-IB
```

| Field | Bytes | Value |
| --- | --- | --- |
| format | `01` | 1 |
| signer | `00` | 0, the test key |
| id | `652B757A8DC4F04A` | `652B-757A-8DC4-F04A` |
| edition | `00` | Frame Fairy |
| signed | `012C` | day 300, 28 October 2026 |
| name length | `00` | no name |
| signature | 64 bytes | the rest |

Its fingerprint is `62fe5cd6c5086b5e1a4d9e0d2ad392f3`.

**The next key in the same pool**, which is what a buyer gets when their
first key has to be replaced. ID `CB61-6854-4C05-45CA`:

```
FF1-AQDLYWhUTAVFygABLADLKfTX5WVZqY2iCW2mWutlsHNLjU5DfDahvtX0-aTomrqgfF2oSLmNQnYXuYPw9SNJ4vUnkxOb0NAjXe1ATvoH
```

**Issued by hand** for a podcaster reviewing Frame Fairy, Lena Fischer, on
12 November 2026. ID `F625-9A14-E805-9D0D`:

```
FF1-AQD2JZoU6AWdDQABOwxMZW5hIEZpc2NoZXJt4rt_NPo-K8Cm1V_l-TjIzQbpWRc4y2dYCrOPOAEKcTTOWFqCTR6UCV50bvX8hVQL5CkNEaHEcsrMkHnZOTIN
```

## How revocation reaches the app

**The app never sends anything about the licence anywhere.** It does read
one thing from the internet: the update feed, once at start and once a day,
the same public file for everyone, fetched without the key, a name or
anything else that says who is asking. That is how it already finds new
versions, see [UPDATES.md](UPDATES.md). Revocations ride along in it.

**Nothing is baked into the app for good.** The feed carries the whole
current list, signed with the update key:

```json
"licences": {
  "revoked": [
    "62fe5cd6c5086b5e1a4d9e0d2ad392f3"
  ],
  "genuine": {},
  "record": {"seq": 1043, "head": "e7d19a…"}
}
```

`revoked` holds fingerprints, here the first example key, as it would read
after the key was posted in public. `genuine` is empty until a signing key
leaks, and then holds that signer's genuine fingerprints under its number.
`record` is the newest link of the dispenser's record when the feed was
published, shortened here, see [What each program remembers](#what-each-program-remembers).

1. The release workflow asks the dispenser for the list, puts it in the
   feed and signs the feed with the update key, which neither program
   holds.
2. The app fetches the feed and checks the feed's signature.
3. If it holds, the app keeps the licence list in a file in its own
   folder, replacing the one it had.
4. Before every render, the app checks the key's signature, then looks its
   fingerprint up in the list it keeps.

A key taken off the list, after a chargeback the bank reversed, works again
at the next fetch. A feed that does not arrive, does not parse or whose
signature fails changes nothing: the app goes on with the last list it
trusted. A Mac that is never online keeps the list it last had, and a key
revoked after that keeps working there. That is accepted: it only ever
costs a sale already lost, and never locks out someone who paid.

**A key can only be revoked once it is found.** A chargeback or a refund
names its keys, so those are revoked by themselves. A key posted in public
names nothing: somebody has to come across it, on a forum, in a crack or
in a buyer's note, before it can go on the list, and nothing of ours looks
for it. Until then it works everywhere, and afterwards it still works on
every Mac that does not fetch the feed again. Revocation limits a leak
that was noticed. It does not prevent one.

## Use cases

Each row is one requirement, and its last column is the test that proves
it.

| # | Use case | Starts with | Done by | Accepted when |
| --- | --- | --- | --- | --- |
| 1 | Bought through Paddle | Paddle `transaction.completed` | dispenser `Assign` | One pool key per seat is assigned, recorded and emailed. The same webhook twice gives the same keys and no second email. A price ID that is not ours is refused |
| 2 | Key on the thank-you page | Our page, with the transaction | dispenser `Keys` | The page shows the keys as soon as the sale is assigned, and says it is waiting until then |
| 3 | Several seats | A purchase with quantity n | dispenser `Assign` | n keys, all sent to the buyer |
| 4 | Sold by a partner, live | Partner API `POST /v1/orders` | dispenser `Assign` | Keys come back in the response. The same partner order twice gives the same keys. Another partner's token cannot reach them |
| 5 | Sold by a partner, in advance | Command line on the signer | signer `Partner` | A file of n keys, recorded with the partner and its index |
| 6 | Given away | Command line on the signer | signer `Named` | A key with the name we gave, recorded with its purpose |
| 7 | Lost key | Our lost-key page, an email address | dispenser `Resend` | Keys go only to that address. The page answers the same whether or not the address bought anything. Limited per address and per caller |
| 8 | Chargeback | Paddle adjustment, action chargeback | dispenser `Settle` | Every key of that sale is on the next revocation list |
| 9 | Chargeback reversed | Paddle adjustment, action chargeback_reverse | dispenser `Settle` | Those keys leave the next revocation list, whichever of the two webhooks arrives first |
| 10 | Refund made anyway | Paddle adjustment, action refund, then updated when Paddle approves it | dispenser `Settle` | Handled exactly like a chargeback |
| 11 | Key posted in public | Command line, or the admin call | dispenser `Replace` | That key is revoked, the sale gets the next pool key, and the buyer gets it by email |
| 12 | Pool running low | The signer asks for the pool level | signer `Pool`, dispenser `Stock` | Below the line, the signer signs a batch and the dispenser takes it. The dispenser checks every key's signature before it takes it |
| 13 | Pool nearly empty | Dispenser, after any sale | email to us | Below 20 % of a batch, an email, every day until it is refilled |
| 14 | Dispenser's database stolen | Command line | dispenser `RevokeUnsold`, then 12 | Every key still in the pool is revoked and the pool is refilled. Sold keys keep working |
| 15 | Dispenser's database restored from a backup | Command line | dispenser `Retire`, then 12, then `Reconcile` | Every key unsold in the backup is set aside, not revoked, and never handed out again. Sales missing from the backup get fresh keys. No key is ever held by two sales |
| 16 | Planned key rotation | Signer switches to the next key | signer | The next public key shipped in a release first. Keys of the old signer, sold or in the pool, still check |
| 17 | Leaked signing key | Command line | dispenser `Genuine`, signer | Keys of that signer check only if their fingerprint is on its genuine list: every one sold or issued by hand. Its unsold pool keys are dropped and the pool refilled from the next signer. No buyer acts |
| 18 | Publish revocations | Release workflow | dispenser `Revocations` | The feed carries the list, signed with the update key, which neither program holds |
| 19 | Daily check | CRON trigger on the dispenser | dispenser `Reconcile`, `Audit` | Every completed Paddle transaction and adjustment of the last days is in the record. Anything missing is run through as if its webhook had come. The store is what its record adds up to |
| 20 | Sandbox | Paddle sandbox and the test signer | both | The whole sale works end to end. No shipped build accepts the test signer |
| 21 | Mail fails | Mailer error | dispenser | Sending is retried. The thank-you page and the lost-key page still deliver |

## Architecture

Two programs and one package they share:

```
Paddle webhook   thank-you page   lost-key page   partner API   CRON
       \               |                |              |         /
        dispenser, cmd/framefairy-dispenser (serverless, always on)
        checks every request, then calls its engine:
        Assign, Keys, Resend, Replace, Revoke, Restore,
        Stock, PoolLevel, Revocations, Genuine, Reconcile
                              |
              managed PostgreSQL: pool, sales, record
                              ^
                              | asks for the pool level, hands over batches
                              | (the signer calls out, nothing calls in)
        signer, cmd/framefairy-signer (a small machine, mostly off)
        Pool, Partner, Named, its own record of every key it signed

key package, licence/: Sign and Check, the same code in the signer,
the dispenser and the app
```

### Key package: `licence/`

The format and nothing else, so making a key, checking a batch and
checking a pasted key are the same code. Built.

```go
type Licence struct {
	Format, Signer, Edition uint8
	ID                      ID        // shown as 652B-757A-8DC4-F04A
	Signed                  time.Time // a day, midnight UTC
	Name                    string
}

type Trust struct {
	Signers map[uint8]ed25519.PublicKey // public keys, by signer number
	Revoked Set                         // the revocation list
	Genuine map[uint8]Set               // the genuine list of each leaked signer
}

func Sign(l Licence, key ed25519.PrivateKey) (Key, error)
func Check(k Key, t Trust) (Licence, error)
func (k Key) Fingerprint() Fingerprint
func ParseID(s string) (ID, error)
func ParseFingerprint(s string) (Fingerprint, error)
```

The rules it keeps:

- **One spelling per key.** The fingerprint is taken over the text, so a
  key that could be written two ways would have two fingerprints, and a
  revoked key could slip past the list in its other spelling. `Check`
  accepts only the exact text `Sign` writes: no padding, no other
  alphabet, no line breaks or space, no stray bits in the last character,
  nothing after the signature. Whoever reads a pasted key trims the space
  around it first.
- **What `Check` accepts, `Sign` would have written.** A name that is not
  UTF-8, has control characters or characters that turn the direction of
  text, or space at either end is refused by both, even with a good
  signature, so a name is always shown as it reads. So is an ID of all
  zeros, which only a broken random source makes.
- **`Sign` checks its own work.** It runs `Check` on the key it just made
  before it returns it, so a fault while signing never hands out a key
  that does not check.
- **Signer 0 is the test signer.** No shipped build trusts it. Only a
  build made from the code on this machine does, see
  [How the app checks a key](#how-the-app-checks-a-key).
- **The reason is kept.** `Check` refuses with one of `ErrMalformed`,
  `ErrFormat`, `ErrSigner`, `ErrSignature`, `ErrRevoked` and
  `ErrNotGenuine`, so the settings can say why.

`Check` reads untrusted text, so it has three fuzz targets: any text, any
licence signed, and any field bytes signed past `Sign`'s rules, as a
broken signer could. The tests sign the three example keys again and get
the very same text, which pins the whole format, and flip every bit of
them and every value of every field byte, which must all be refused.

### Dispenser: `cmd/framefairy-dispenser`

Every rule about sales lives in its engine: one key per seat, the same
keys for the same order, what may be revoked and restored, when to warn.

```go
type Order struct {
	Source, Ref string // "paddle" | "partner:<name>", and its reference
	Seats       int
	Email       string // where the keys go first. Used to send, never stored
	At          time.Time
}

func (e *Engine) Assign(ctx context.Context, o Order) ([]licence.Key, error)
func (e *Engine) Keys(ctx context.Context, source, ref string) ([]licence.Key, error)
func (e *Engine) Resend(ctx context.Context, email string) error
func (e *Engine) SendMail(ctx context.Context) (sent, failed int, err error)
func (e *Engine) CheckPool(ctx context.Context) error
func (e *Engine) Replace(ctx context.Context, f licence.Fingerprint, why string) (licence.Key, Seat, error)
func (e *Engine) Revoke(ctx context.Context, t Target, why string) (int, error)
func (e *Engine) Restore(ctx context.Context, t Target, why string) (int, error)
func (e *Engine) Stock(ctx context.Context, generation string, batch []licence.Key) (added int, err error)
func (e *Engine) PoolLevel(ctx context.Context) (Level, error) // left, batch, generation
func (e *Engine) RevokeUnsold(ctx context.Context, why string) (int, error)
func (e *Engine) Retire(ctx context.Context, why string) (int, error) // set the unsold pool aside, revoke nothing
func (e *Engine) Revocations(ctx context.Context) ([]licence.Fingerprint, error)
func (e *Engine) Genuine(ctx context.Context, signer uint8) ([]licence.Fingerprint, error)
func (e *Engine) Settle(ctx context.Context, ref string) error
func (e *Engine) Reconcile(ctx context.Context, since time.Time) (int, error)
```

What the engine needs from outside, each behind a small interface so the
engine is tested with memory and a fixed clock:

| Port | Does | First implementation |
| --- | --- | --- |
| `Store` | The pool, the sales and the record, each change in one transaction | Scaleway Serverless SQL Database |
| `Orders` | A Paddle sale as it stands now, the sales completed or adjusted since a time, and the sales of an address | Paddle's API |
| `Mailer` | Sends keys to buyers and warnings to us | A European mail service |

**Settling a sale.** An adjustment webhook does not say what to do, it
says something changed. `Settle` asks Paddle what the sale is now and
makes the dispenser match: its keys assigned, and revoked exactly when
Paddle has the money back. So a chargeback and its reversal end the same
whichever webhook arrives first, and however often each comes. Keys
revoked because they were posted in public are not Paddle's business and
stay revoked. `Reconcile` settles every sale Paddle completed or adjusted
since a time, and goes on past one that fails.

Paddle has the money back when a refund of the sale is approved or a
chargeback has not been reversed. A refund waiting for Paddle's approval
or rejected, and a chargeback warning, do not count. A reversal is an
adjustment of its own, `chargeback_reverse`, and a refund being approved
is the first adjustment updated, see
[Paddle's adjustment events](https://developer.paddle.com/webhooks/adjustments/adjustment-created).
Reading this from Paddle's API is the `Orders` implementation's job, and
the sandbox run checks it.

**Mail.** A Paddle sale's letter is queued in the same transaction that
assigns its keys, then sent at once to the address in the webhook. If that
fails, it waits in the queue and `SendMail`, which runs every few
minutes, tries it again after 1, 5 and 30 minutes, 2 hours, then every 6
hours, with the address Paddle has. After three days we are told, and
then it is given up, in that order, so a crash between the two tells us
twice rather than never: the keys are on the thank-you and lost-key pages meanwhile.
The queue holds no address, only which sale a letter is for, so no
buyer's address is ever kept. A run takes a letter for ten minutes before
it sends it, so two runs never send the same one, and a crash between
sending and ticking it off sends it again later: a buyer can get a letter
twice, never no letter. A partner's buyer gets one try when the partner
gave an address, and nothing waits, because the partner has the keys in
its answer. `Resend` asks Paddle which sales an address bought and sends
their keys to that address only. `CheckPool` warns us once a day while
the pool is below a fifth of a batch.

Handing out a key is one database transaction: take the oldest unsold key,
mark it sold to this order and seat, write the record line. A unique rule
on order and seat makes a second webhook for the same sale find the keys
already given instead of taking new ones.

`Store` is plain storage, `Update` and `View` around a transaction that
adds, finds and moves rows, and every rule is the engine's. A database may
run a transaction twice when it collides with another, so the engine keeps
nothing from a run that did not commit. The store in memory has a mode
that runs every transaction twice, and every engine test runs in it.

Built so far, on the store in memory: `Assign`, `Keys`, `Stock`,
`PoolLevel`, `Revoke`, `Restore`, `Replace`, `RevokeUnsold`, `Retire`,
`Revocations`, `Genuine`, `VerifyRecord`, `Audit`, `SendMail`, `Resend`
and `CheckPool`. A batch handed over
twice adds its keys once, and a key whose ID the pool already has is
refused. A key that was replaced or burned stays revoked whatever happens
to its sale, because the key itself got out. A key whose sale was revoked
is not replaced.

**`Audit`** plays the whole record from its first line, refuses any line
that could not have happened where it stands, and compares what that adds
up to with the pool, the seats and the revocation list in the store, key
by key. It runs every day, and at the end of every engine test.

What reaches it, and who may call what:

| Adapter | Receives | Calls |
| --- | --- | --- |
| Paddle | `transaction.completed`, `adjustment.created`, `adjustment.updated`, signed by Paddle | `Assign` for a sale, `Settle` for an adjustment |
| Thank-you page | A transaction, from our website | `Keys` |
| Lost key | An email address, from our website | `Resend` |
| Partner API | An order, with the partner's own token | `Assign`, `Keys`, `Revoke` |
| Signer | The pool level and batches, with the signer's token | `PoolLevel`, `Stock` |
| Release workflow | A request for the list, with its own read-only token | `Revocations`, `Genuine` |
| CRON | The daily trigger | `Reconcile`, the warning, the export |
| Admin | Us, with a token kept offline | `Replace`, `Revoke`, `Restore`, `RevokeUnsold`, `Retire` |

### Signer: `licence/signer`, `cmd/framefairy-signer`

Built, apart from handing batches to the dispenser, which waits for the
dispenser.

```go
func (s *Signer) Pool(n int, edition uint8) ([]licence.Key, error)
func (s *Signer) Partner(partner string, n int, edition uint8) ([]licence.Key, error)
func (s *Signer) Named(name, purpose string, edition uint8) (licence.Key, error)
func (r *Record) Issued(signer uint8) []licence.Fingerprint
```

It runs on a small machine of ours, which may be switched off for weeks.
Once a day while it is on, it asks the dispenser for the pool level and,
below the line, signs a batch of 1,000 keys and hands them over. Partner
batches and named keys are commands typed on it:

```
framefairy-signer key -signer 1
framefairy-signer named -name "Lena Fischer" -purpose "review copy"
framefairy-signer partner -partner "Bundle Hunt" -n 500 -out bundle.txt
framefairy-signer issued
```

Its folder, `~/.framefairy-signer`, holds two files. `key.json` is the
private half and the signer number that goes with it, readable by its
owner alone, or the signer refuses to start. `record.jsonl` is its record:
one line per key it ever signed, with the day, signer, ID, fingerprint,
edition, batch, place in the batch, size of the batch, kind and note,
never the key and never a name. With `-test` it signs as the test signer
and keeps a separate record in `test/`, so a sandbox key is never on a
real signer's genuine list.

The rules the record keeps:

- **A key is in the record before it leaves.** A batch is written whole,
  in one write, and synced to the disk before any of its keys are
  returned. That is how no ID is ever drawn twice, and no key is ever out
  there that the genuine list would miss.
- **A batch cut short was never handed out.** If the signer stops while it
  writes, the half-written batch is removed when the record is next
  opened, and the signer says so.
- **Anything else that is not what the signer writes is refused.** A line
  changed, removed, doubled or moved, a gap in the batch numbers, an ID
  written another way: the record will not open, and says which line.
- **One signer at a time.** The record is locked while it is open, so two
  signers can never draw IDs past each other.
- **IDs are drawn again when taken.** A random source that keeps giving
  taken IDs stops the signer instead of looping.

### Partner API

The one surface other programs build against: small, versioned, the same
for everyone.

```
POST /v1/orders                {ref, seats, email}  -> {keys}
GET  /v1/orders/{ref}                               -> {keys}
POST /v1/orders/{ref}/revoke   {why}                -> {revoked}
```

Each partner has its own token, which reaches only its own orders: the
token decides the source, and a partner cannot name one. The same `ref`
twice gives the same keys. A partner with its own webhook format gets an
adapter that turns it into these calls.

### Every endpoint

Built, in `licence/dispenser`, in front of the engine.

| Endpoint | Caller | Does |
| --- | --- | --- |
| `POST /paddle` | Paddle, signed | `Settle` the sale the webhook names |
| `GET /v1/thanks/{ref}` | our thank-you page | the sale's keys and the key ID of each, for a day |
| `POST /v1/lost` | our lost-key page | `Resend` |
| `POST /v1/orders`, `GET /v1/orders/{ref}`, `POST /v1/orders/{ref}/revoke` | a partner, with its token | `Assign`, `Keys`, `Revoke` |
| `GET /v1/pool`, `POST /v1/pool` | the signer, with its token | `PoolLevel`, `Stock` |
| `GET /v1/feed` | the release workflow, with its token | revocation list, genuine lists of leaked signers, the record's head |
| `POST /v1/cron/mail` | every few minutes, with the cron token | `SendMail` |
| `POST /v1/cron/daily` | once a day, with the cron token | `Reconcile` the last week, `CheckPool`, `Audit` |
| `POST /v1/admin/{action}` | us, with the admin token | `replace`, `revoke`, `restore`, `revoke-unsold`, `retire`, `settle`, `reconcile`, `audit` |
| `GET /healthz` | the outside check | the database answers |

The rules they keep:

- **A Paddle webhook is read only when Paddle signed it in the last five
  minutes**: an HMAC-SHA256 of `{ts}:{body}` with the webhook's secret, in
  the `Paddle-Signature` header. Two secrets are accepted while one
  replaces the other. Of the webhook, only the reference of the sale is
  read, and `Settle` asks Paddle what the sale is. A webhook that cannot
  be settled yet, because Paddle does not know the sale yet, the pool is
  empty or something is down, is answered with a failure so Paddle sends
  it again. One the dispenser does nothing with is answered with
  success, so Paddle stops. The format is
  [Paddle's](https://developer.paddle.com/webhooks/about/signature-verification).
- **Paddle wants an answer within five seconds**, and otherwise sends the
  webhook again: up to 60 times over three days for a live account, 3
  times in 15 minutes in the sandbox, see
  [Paddle's delivery rules](https://developer.paddle.com/webhooks/about/respond-to-webhooks).
  `Settle` asks Paddle once and writes once, so it fits, and a webhook it
  overran is settled again when it comes back. Paddle's own libraries
  refuse a signature older than five seconds. The dispenser allows five
  minutes, because a webhook replayed inside them only makes `Settle` ask
  Paddle again, and a server clock a little off should not hold up a
  sale. That means each retry is signed when it is sent, which the
  sandbox run checks. Paddle also publishes the addresses its webhooks
  come from. The dispenser trusts the signature and does not check them.
- **The thank-you page shows a sale's keys for a day after the sale**, then
  answers that they went by email. A reference that got out later shows
  nothing.
- **The lost-key page answers the same whatever the address**, and takes
  three asks per address and twenty per caller an hour. The counts live in
  one container's memory: they stop a page being used to flood an inbox,
  not a determined attacker asking a few times more.
- **Only our website's pages may read** the thank-you and lost-key
  answers. No other endpoint answers a page at all.
- **Tokens are kept as their SHA-256**, so the dispenser's settings hold
  nothing a caller could use, and each caller's token reaches only its own
  endpoints. A caller without a token has its endpoints switched off.
- **Every request is read strictly**: one JSON object, no unknown field, at
  most 16 KB, 2 MB for a batch from the signer, 1 MB from Paddle. Every
  answer says not to cache it.
- **A failure says what kind it is and no more**: `invalid`, `not_found`,
  `conflict`, `stale`, `pool_empty`, `unauthorized`, `too_many`,
  `internal`. What went wrong inside is logged, never sent.
- **A daily audit that fails emails us** at once.
- **A panic fails its own request**, and the dispenser goes on.

### On this machine

`make dispenser` runs the dispenser on this machine, at
`http://127.0.0.1:8090`, with a pretend world around it, to try every
sale and every failure by hand before any of it meets Paddle:

- **A pretend Paddle.** Its checkout at `/shop` sells, sends a signed
  `transaction.completed` webhook and sends the buyer to the thank-you
  page with the sale's reference, as Paddle's checkout does. From the
  dev page it refunds, with Paddle's approval or rejection, charges back,
  reverses a chargeback and warns of one, each as the adjustment
  webhooks Paddle sends, and it answers the dispenser's questions about
  a sale through `licence/paddle`, as the client for Paddle's API will.
  A webhook that is not answered with success is sent again after 1, 5
  and 15 minutes, as the sandbox does. Its signature is written from
  Paddle's docs and not taken from the dispenser.
- **The buyer's pages**: the checkout, the thank-you page and the
  lost-key page, on the same address as the endpoints, the way our
  website and the dispenser will share their origin rules. They are
  plain, and say what each answer of the dispenser means to a buyer.
- **A mail service that sends nothing.** Every letter, to a buyer or to
  us, lands in the outbox on the dev page.
- **The test signer**, which refills the pool through the hand-over
  endpoint whenever it is below a batch, 10 keys by default. A Frame
  Fairy built here with `make` takes its keys, so the Unlock button on
  the thank-you page and in every letter on the dev page opens the app
  with the key in its settings, as the real letter will.
- **A clock that can be moved forward**, by minutes, an hour, a day or
  three, so a retry, the thank-you page's day, the mail run every 5
  minutes and the daily run come in a click.
- **Switches for what goes wrong**: Paddle's API down, Paddle's API not
  knowing a sale for 2 minutes after its webhook, Paddle's next webhooks
  sent twice, held to be sent by hand in any order, or lost, the mail
  service and the database down or doing their work and losing the
  answer, the signer and the scheduled runs stopped.

The dev page at `/dev` opens with what a person does first: buy. Each
sale then shows what Paddle says about it and what the dispenser holds,
in words, with Refund and Chargeback beside it and everything else under
More. A sale without keys says why: which answer Paddle's webhook got
and when Paddle tries again. Beside the sales are the letters the mail
service sent, and the switches for what goes wrong, which the top of the
page names while any is not normal. Webhooks, the pool, the runs, what
we do by hand, the partner, the record and the log are behind the
scenes, one tab each. The page reads itself again every two seconds and
keeps what is open. Every button goes through the dispenser's own
endpoints with the token a real caller has: the signer's, the scheduled
runs', the release workflow's, ours and a pretend partner's. It answers
this machine only, keeps nothing, and every start is an empty shop with
the pool already filled.

What it cannot show is whether we read the real Paddle right. That is
the staging dispenser's job: the same program against Paddle's sandbox,
once the PostgreSQL store and the container are there.

## What each program remembers

**The dispenser** keeps three tables in its database:

| Table | One row per | Holds |
| --- | --- | --- |
| pool | key signed and handed over | the key, its fingerprint, ID, signer, batch, and whether it is sold |
| sales | seat sold | source, reference, seat, the key's fingerprint, when, and whether it is revoked |
| record | event | `stock`, `assign`, `revoke`, `restore`, `replace`, in order, never changed |

The record is the history, and the pool and sales are what it adds up to.
Every record line carries the SHA-256 of the line before it, so a line
changed, removed or slipped in breaks the chain, and the dispenser checks
the chain every day. The newest link goes into every update feed, which
is signed elsewhere and kept in this repository's releases, so history
before the last feed cannot be rewritten without it showing.

No names and no email addresses are kept. The buyer's details stay at
Paddle. The keys in the pool are the one thing of value in the database,
and use case 14 is what happens if it is ever stolen.

**The signer** keeps one file, a line for every key it signed, see
[Signer](#signer-licencesigner-cmdframefairy-signer), and a copy of it in
object storage that keeps every version.

**Backups.** The database is backed up by Scaleway automatically. On top
of that, the daily CRON run exports all three tables to object storage at
a second provider, in a bucket that keeps every version and locks each one
for a year. The dispenser's credentials there can add versions but not
delete them.

**Restoring.** Restore the newest backup and check the record's chain
against the last published feed. Then, before anything is sold:

1. **Set the whole pool aside.** Every key still marked unsold in the
   backup is retired: never handed out again, and not revoked. Some of them
   were sold after the backup was taken and are in buyers' hands now, and
   the backup cannot say which. `Retire` also starts a new pool
   generation.
2. **Refill.** The signer signs a fresh batch, and only fresh keys are
   handed out from then on. A batch the signer handed over before the
   restore and never heard back about is refused, because it carries the
   old generation, and the signer drops it: its keys may have been sold
   before the database was lost, and the restored database no longer
   knows them. The simulation found this one, see Testing.
3. **Catch up.** `Reconcile` runs from the backup's last line. A sale Paddle
   knows about and the record does not gets the next fresh key, by email,
   with a line saying any key it had before still works.

An example. The backup runs at 10:00. At 10:30 Anna buys, gets key K1 on
her thank-you page and by email. At 11:00 the database is lost and the
10:00 backup comes back, in which K1 is still unsold and Anna's sale does
not exist. K1 is set aside with the rest of the pool, so no one else is
ever given it. The catch-up finds Anna's sale at Paddle and sends her K2.
She has two working keys, and no key belongs to two people.

So a restore can give a buyer a second key, never leave one without a key,
and never give two buyers the same key. At worst it retires a pool's worth
of keys nobody holds.

What a restore cannot bring back is what we did by hand after the backup.
A key posted in public and replaced after the backup works again, and so
does the first key of a sale made after the backup and charged back
later, because the backup never knew it was sold. Paddle's chargebacks
themselves are caught up by `Reconcile`. Both cost at most a sale that is
already lost, and the revocation list of the last published feed says
which keys were replaced, to be replaced again by hand.

## When something is down

| What is down | What happens |
| --- | --- |
| The signer | Nothing anyone sees. The pool lasts months, and the warning comes at 20 % |
| The dispenser | The app is unaffected. A buyer's payment still completes at Paddle, and the thank-you page, if it loads, says the key is coming by email. Paddle retries its webhook 60 times over 3 days, see [Paddle's webhook docs](https://developer.paddle.com/webhooks/about/respond-to-webhooks/), and the daily check catches anything later |
| Paddle | Nobody can buy. Nothing of ours can change that |
| The mail service | Sending is retried. Keys are on the thank-you page and the lost-key page meanwhile |

The dispenser is a managed service, so it goes down only when Scaleway
does. An outside check calls it every few minutes and emails us when it
does not answer.

## How the app checks a key

The app calls the same `licence.Check` the dispenser checks every batch
with, offline, before every render.

- **Entering it.** Most buyers never type a key: Unlock in the letter or
  on the thank-you page does it, see below. For a key that comes as text,
  from a partner or read off a phone, the Licence row has a field while no
  key is kept, and checks the key when Unlock is pressed. The key shows
  as it is, not as dots, so a person can check it is the one they meant.
  A refused key shakes the field, keeps what was typed and says why, the
  whole of it, like a refused API key. Once a key is kept the row says
  Licensed, with the check mark, the key ID and the name when it has
  one, and nothing else: no field, no button, nothing that reads as a
  step still to take. The key itself is never shown in the app. It is
  long and means nothing to a person, and the key ID is what names it,
  here, on the thank-you page and to support. A key ID is set in one
  width on a block of its own, the way code is set in a README, in the
  grey of a line under a name. The kept key's is a button: a click puts
  the key ID on the clipboard, for a mail to support, and the copy mark
  on it turns into a check for a moment. The key itself is never read
  back out of the keychain. Nothing a customer does needs it: there is no
  activation and no machine to move a licence from, so a new Mac takes
  the same Unlock in the same letter, and the lost-key page sends the
  letter again. Reading it would also have the keychain ask for the
  Mac's password, from every build it does not know yet. That block wears
  the beam from `Busy.svelte`, the same beam, turned down and given
  `seldom`: the comet comes round once every nine seconds, with a
  bounce as it arrives, and the rim rests between, because it is what
  was bought and nothing is running. Running all the time it was too
  busy. It is what was bought: a first try with a light of its own looked like
  nothing else in the app, and Tim saw it. The trash can beside it
  removes the key, after asking, and the field comes back.
- **The Unlock link.** The thank-you page and the letter carry, for
  each key, an Unlock Frame Fairy button, a link
  `framefairy://unlock?key=FF1-…`. The app claims the `framefairy`
  scheme in its `Info.plist`, so macOS opens it, or brings it to the
  front, and hands it the link, and the app checks the key at once,
  `linkOutcome` in `cmd/framefairy-app/licence.go`. A key the app takes
  is kept at once, with nothing to press, and the settings say so,
  "Unlocked from the link. Thank you.". A key kept before is replaced,
  and the row names both, "Key ID B from the link, in place of key ID A".
  Every key unlocks the same app, so the Mac is licensed all the way
  through and nothing is lost, and the key replaced is still in its own
  letter. It used to wait in the field for Unlock instead, because any
  web page can open such a link, but Tim found the field and the button
  a step that did nothing a buyer needs: the most a page can do is put
  in a key of its own, which still unlocks the app, and which the row
  names. The same key again changes nothing. A key the app refuses
  changes nothing either, and the row says why, in full, with the
  warning mark: a Mac that was licensed stays licensed, and says that
  its key still unlocks it. Tim's first try from main looked like a key
  that would not unlock: an app from Updates refuses test keys, and the
  reason was cut off at "takes …". The link is read
  strictly, in `keyFromLink`: the `framefairy` scheme, the host
  `unlock`, no path, no user, no fragment, one `key` and nothing else,
  at most 1024 characters, and a key that looks like one before it is
  even checked. Anything else is dropped and logged. It has a fuzz
  target.
- **Keeping it.** Where the API key is kept: the keychain on macOS. The
  keychain item carries its description, the ID and the name, as its
  comment, so the settings show it without reading the key itself and
  without the keychain asking for a password.
- **Using it.** The engine checks before every render, so the app and the
  command line obey one rule: the mark goes on, or it does not.
- **Trust.** Public keys are built into the app. The revocation list and
  any genuine lists arrive with the update feed, see
  [How revocation reaches the app](#how-revocation-reaches-the-app).
- **Test keys only where anyone could sign their own.** A build made from
  the code on this machine, which the build workflow has given neither a
  channel nor a commit, also trusts the test signer, so the keys of
  `make dispenser` unlock it. Once taken, a test key reads like any
  other, because no other build would have taken it. Anyone who builds from the code could take
  the check out anyway, so this gives nothing away. Every build from the
  workflow, which is every build an update brings, refuses them, and says
  it is a test key. No switch in any build turns the check off.

**Where a key travels.** A key is meant to be read by the person who
bought it, and on its way it passes through these hands:

1. The dispenser, which holds every key it sold, and its database.
2. The thank-you page, in the buyer's browser, for a day after the sale,
   and the browser's history and cache from then on.
3. The mail service that sends the letter, every mail server between it
   and the buyer, and the buyer's mail provider, which keeps it.
4. Anybody who can read the buyer's mailbox, on any device it is open on.
5. The Unlock link: the browser or mail app that opens it, macOS, which
   hands it to the app, and the app's own memory until it is kept, or,
   when it would replace a key, until Unlock is pressed or the app quits.
   It is never written to a log or to a file.
6. The keychain on every Mac it unlocks.

None of these is a server of ours apart from the dispenser, and the app
never sends the key anywhere. A key that leaks from any of them works
until it is found and revoked, see
[How revocation reaches the app](#how-revocation-reaches-the-app). That
is accepted: a key unlocks a watermark and nothing else, it names no
one who did not choose a name, and a leaked one costs a sale, not
anybody's data.

## Security, privacy, testing

**Security**

- The private signing key exists only on the signer. Never on the
  dispenser, never in the repository, never in CI, never in the app. The
  update-signing key is a different key pair, held by the release
  workflow.
- The signer accepts no connections. It only calls the dispenser.
- Nothing reaches the dispenser's engine unchecked: Paddle's webhook
  signature, a partner's or the signer's token, the price ID, the size of
  every field, and the signature of every key handed over in a batch.
- Each token reaches only what its caller needs, as the adapter table
  says. The admin token is kept offline.
- The next signer's public key ships in a release before it is ever used.

**Privacy**

- Neither program keeps names or email addresses. Sales are known by
  Paddle's transaction, which only Paddle can tie to a person.
- A deletion request is carried out at Paddle, and nothing of ours
  changes.

**Testing**

- The dispenser's engine is tested with its ports in memory and a fixed
  clock. No network, no Paddle, no mail.
- Handing out keys has a test that sells from many goroutines at once,
  with webhooks repeated, under the race detector. No key is sold twice
  and no order gets two sets. Another runs every call at once and ends
  with the audit.
- **A second implementation.** The tests hold a model of the dispenser,
  written as plainly as possible from this spec, with no store and no
  record. The engine and the model get the same thousands of random
  calls, and every answer and every error must agree. A mistake has to be
  made twice, the same way, to get through.
- The store in memory can run every transaction twice, as a database does
  when two collide, and every engine test runs both ways.
- `licence.Check` has a fuzz target, as the repository requires for
  everything that reads untrusted text.
- Fuzz targets read Paddle's signature header and body, and send any body
  to every endpoint that takes one from outside. None may be accepted
  without its true signature, and none may fail on our side.
- Every use case has a test named after it.
- **The simulation.** The real engine runs in a world that goes wrong:
  buyers buy, Paddle sends webhooks late, twice, in the wrong order or
  never, takes money back and gives it back, the database is lost before
  and after it commits, the mail service fails or fails after it sent,
  Paddle goes down, the pool runs dry, the signer's hand-over drops, keys
  are posted in public, and the database comes back from a backup. After
  every step: no key is ever given to two sales, no letter carries a key
  of someone else's sale, and the store is what its record adds up to.
  Once the world calms down: every paid sale has its keys and a letter
  with them, or we were told the letter was given up, and the revocation
  list is exactly what Paddle and the posted keys say. One seed decides
  every step, so a failure runs again the same way. It is fuzzing, not a
  unit test: `FuzzSimulation`, whose input is the seed. `make fuzz`
  plays 300 new histories of 400 steps on every run, and a seed that
  fails is kept in `testdata/fuzz/FuzzSimulation/`, where every unit run
  plays it again: `go test ./licence/dispenser -run
  'FuzzSimulation/<its file>' -v -sim.verbose` writes out every step.
  Before it was merged it ran 5,000 seeds and 500 seeds of 2,000 steps.
  It drives the dispenser through its endpoints, as Paddle, the signer, a
  partner, the buyers, the scheduled runs and we do, and the daily audit
  must never fail.
- **The database failing at every moment.** What the simulation reaches
  by chance is tested on purpose too, every run: for ten use cases the
  database fails at each of their writes, before and after it saved, and
  at each step inside a transaction, and the same call made again has to
  end where an undisturbed one ends. Every endpoint is called with the
  database down, and every rule of the audit gets a record it has to
  refuse. These reach every line the simulation does.
- **What these tests found**, each now a rule of the code above. The
  simulation: a batch handed over again after a restore sold keys a
  second time (the pool generation), and a letter given up while the
  database lost its answer left us untold (we are told before the letter
  leaves the queue). The store that runs every transaction twice: a
  failed letter counted its try twice and warned us twice (a transaction
  works everything out before it starts). Both simulation bugs come back
  as failing seeds when their fix is taken out.
- **The dispenser on this machine** has tests that press every button of
  its dev page with the database working and down, and that walk a sale
  from the checkout to a chargeback and back, webhooks held and sent in
  the other order, twice or lost, the mail service, the database and
  Paddle's API failing, the pool running dry, the thank-you page's day
  and the lost-key page's limits. Buyers, Paddle, the signer, the runs
  and the dev page all at once, under the race detector, end with the
  audit.
- The whole sale runs once against Paddle's sandbox before launch.

## Binding a key to a machine

Possible, and decided against. It would work like this: the app sends an
identifier of the Mac to the dispenser, the dispenser answers with a
signed activation for that Mac, and the app only accepts the key alongside
an activation for the Mac it runs on. "One place at a time" needs more:
the app has to ask again every few days, or the activation runs out.

Why not:

- **It breaks the product's promise.** The app would need the network to
  stay licensed, and would tell us which Mac uses which licence.
- **It stops honest buyers first.** A new Mac, a reinstall, a repaired
  logic board or a laptop beside a desktop all need a deactivation and a
  support email.
- **It stops almost no one else.** The code is public, so taking the check
  out is a deleted line, and the people who would share a key are the ones
  who would delete it.
- **An IP address is worse.** It changes at home, on mobile, on a train and
  behind a VPN, and one address is shared by many people behind the same
  router or carrier.

What we do instead: the key ID support can look up, and revoking a key
posted in public. If this is ever wanted, the gentle form is an activation
count, for example three Macs per licence, each activated once online and
then offline for good, with the list of Macs shown to the buyer so they
can remove one. It would be an engine call and an adapter more, and the
key format would not change.

## Out of scope and open questions

**Out of scope**

- Machine binding, activation limits, seat enforcement, and any check that
  needs the network, see [Binding a key to a machine](#binding-a-key-to-a-machine).
- Subscriptions and expiring keys.
- Affiliate tracking and payouts, which are Tolt's.
- Payment, tax and invoices, which are Paddle's.

**Open questions**

- The price, and whether there is ever a second edition.
- The mail service: Brevo or Mailjet.
- Where the signer runs: a small machine of ours, and which one.
- The business the Paddle account belongs to.
- Whether partners are wanted at launch, or the partner API waits for the
  first one.
