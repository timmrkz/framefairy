# Licences

The spec for selling Frame Fairy: how a licence is made and checked, every
way one is issued or changed, and the service that does it. It is batch
5.8 in [GUI-PLAN.md](GUI-PLAN.md), built last. The update policy and why
the code is public are in [UPDATES.md](UPDATES.md).

## Objective

The licence service turns every sale of Frame Fairy into a licence key,
and keeps those keys trustworthy for as long as the product lives. It is
a small Go program beside the app.

Who it serves:

- **Buyers** get their key within seconds of paying, can get it back when
  it is lost, and never need the network to use it.
- **Partners**, bundles, deal sites and resellers, sell Frame Fairy
  through a small, stable API, or with keys made in advance.
- **We** issue keys by hand for press and giveaways, revoke abused keys,
  and replace a signing key without any buyer noticing.

It is done when every use case below passes its acceptance criteria
against Paddle's sandbox, and the app accepts exactly the keys the service
issued and did not revoke.

## Decisions already made

| Topic | Decision |
| --- | --- |
| What a licence buys | Bought once, every update for ever. Without a key the app does everything, and renders carry the Frame Fairy mark |
| Who sells | Paddle, as merchant of record: payment, VAT and sales tax everywhere, invoices, chargebacks |
| Who makes keys | We do. A key is a licence signed with our Ed25519 private key |
| How the app checks | Offline, against public keys built into the app. No network, no machine binding, no activation count |
| Refunds | None once the key has been delivered. The buyer agrees at checkout that delivery starts at once and ends the right of withdrawal, as Paddle's buyer terms allow |
| Revocation | Only chargebacks and keys posted in public. The list travels in the signed update feed |
| Affiliates | Tolt, on top of Paddle. The licence service knows nothing about them |

On refunds: the decision was no refunds for verified keys. The app never
phones home, so whether a key was entered is something we cannot know.
Delivery is the moment we can see, and the one Paddle's terms rest on.

## The licence and the key

A licence is a few fields, signed. The key is `FF1-` followed by the fields
and a 64-byte signature in base64url, 111 characters without a name and
about 125 with one. It is pasted, never typed.

| Field | Size | Meaning |
| --- | --- | --- |
| format | 1 byte | Key format, 1 today. A later format is a superset, and every format ever issued stays readable |
| signer | 1 byte | Which signing key signed it, so a key pair can be replaced |
| id | 8 bytes | Which licence it is, derived from the sale, see below |
| issue | 2 bytes | 0 for a licence's first key, one more each time it is issued again |
| edition | 1 byte | What it unlocks. 0 is Frame Fairy as sold today |
| issued | 2 bytes | Day of issue, counted from 2026-01-01 |
| name length | 1 byte | How long the name is, 0 to 64 |
| name | 0 to 64 bytes | Who it is licensed to, in UTF-8, shown in the settings |

The signature covers the line `framefairy licence v1`, a newline, and the
fields. The line keeps a licence signature from ever being mistaken for
any other thing we sign.

### The licence ID

**The ID comes from the sale, not from a counter.** It is the first 8
bytes of SHA-256 over the source, the reference and the seat, each followed
by a zero byte: `paddle` and the transaction, `partner:<name>` and its
order, or `manual` and a name we choose. An Ed25519 signature over the
same bytes is always the same signature. So the same sale always yields
the same key, issuing is safe to repeat, and a lost key can always be made
again.

People see it written in four groups, `36A3-CE90-838C-1499`. The settings
show it under the name, and it is what support asks for. It stays the same
when a licence is issued again: only the issue number and the key change.

**Collisions.** Two sales could in principle get the same 8 bytes. With n
licences the chance that any two share an ID is about n² / 2⁶⁵:

| Licences | Chance of any shared ID |
| ---: | ---: |
| 10,000 | 1 in 370 billion |
| 100,000 | 1 in 3.7 billion |
| 1,000,000 | 1 in 37 million |

Even then nothing breaks for the buyer, because the app checks each key on
its own and revokes by fingerprint, not by ID. It would only confuse our
records. So the service looks the ID up in the ledger before it signs, and
an ID already taken by another sale stops the issue with an error for us
to see. It is a check that should never fire.

An attacker gains nothing from an ID. It is not secret and proves nothing,
the signature does.

### The fingerprint

A fingerprint is the first 16 bytes of SHA-256 over the whole key text. The
ledger, the revocation list and the genuine lists hold fingerprints, never
keys. It is 16 bytes, not 8, because after a signing key leaks, whoever
holds it could try to sign keys until one matches a genuine fingerprint.
At 8 bytes that is within reach of a large computer, at 16 it is not.

### Examples

Three real keys, signed with the test key. Its public half is
`b959ea225fd22150952415eceb782b933bb8651d2df926492b93d8c205e32ee1`, made
from the seed SHA-256 of `framefairy test signer, never shipped`, and no
build we ship accepts it. The tests check these three keys.

**Bought through Paddle**, transaction `txn_01k6x8m2c4p9qz7t3w5v8n1b2d`,
one seat, for Anna Schmidt on 3 November 2026:

```
FF1-AQA2o86Qg4wUmQAAAAEyDEFubmEgU2NobWlkdFkdkI_qnqYrRgNuL5B8bfECf5p0llTcEHcvGuLbPPH5tG5gf-C-NkEFD2EgAa2E3B5F4xadSChwcWP-7uKo_gY
```

| Field | Bytes | Value |
| --- | --- | --- |
| format | `01` | 1 |
| signer | `00` | 0, the test key |
| id | `36A3CE90838C1499` | `36A3-CE90-838C-1499` |
| issue | `0000` | 0, the first key |
| edition | `00` | Frame Fairy |
| issued | `0132` | day 306, 3 November 2026 |
| name | `0C` and 12 bytes | Anna Schmidt |
| signature | 64 bytes | the rest |

**The same licence issued again** on 14 February 2027, after the first key
was posted in public. Same ID, issue 1, a different key:

```
FF1-AQA2o86Qg4wUmQABAAGZDEFubmEgU2NobWlkdIjq6v1QgAiOgnd9Wd2ZF-ZnwSX59h_ldqgSpV616jsjnXJmRImHwDgLbEuEBl_O90AikubelUg-baYO89ZiLwk
```

**Sold by a partner**, order `BH-20261120-0042` at `partner:bundlehunt`,
no name, on 20 November 2026. ID `287D-E26F-06FC-3F5E`:

```
FF1-AQAofeJvBvw_XgAAAAFDAHdte4o6QuRT3Fc9AzsMjeGbV6287v7tHzEp1MaD7cAR5eYuVOjTTsyJZdIOMXkbeHEAou8fYiBGp5W3kCymxQA
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
    "307dcf717b5fc2c26b3949a4dd12dfe1"
  ],
  "genuine": {},
  "ledger": {"seq": 1043, "head": "e7d19a…"}
}
```

`revoked` holds fingerprints, here the first example key above, as it would read after its key was posted in public. `genuine` is empty until a signing key leaks,
and then holds that signer's genuine fingerprints under its number.
`ledger` is the newest link of the ledger's chain when the feed was
published, shortened here, see [The ledger](#the-ledger).

1. The app fetches the feed and checks the feed's signature.
2. If it holds, the app keeps the licence list in a file in its own
   folder, replacing the one it had.
3. Before every render, `Check` reads the key, checks its signature, then
   looks its fingerprint up in the list it keeps.

A key taken off the list, after a chargeback the bank reversed, works again
at the next fetch. A feed that does not arrive, does not parse or whose
signature fails changes nothing: the app goes on with the last list it
trusted. A Mac that is never online keeps the list it last had, and a key
revoked after that keeps working there. That is accepted: it only ever
costs us a sale we had already lost, and never locks out someone who paid.

## Use cases

Eighteen use cases cover everything the service does. Each row is one
requirement, and its last column is the test that proves it.

| # | Use case | Starts with | Engine call | Accepted when |
| --- | --- | --- | --- | --- |
| 1 | Bought through Paddle | Paddle `transaction.completed` | `Issue` | One key per seat is recorded and emailed. The same webhook twice yields the same keys and no second email. A price ID that is not ours is refused |
| 2 | Key on the thank-you page | Our page, with the transaction | `Keys` | The page shows the keys once Paddle has reported the sale, and says it is waiting until then |
| 3 | Several seats | A purchase with quantity n | `Issue` | n licences, n different IDs, all sent to the buyer |
| 4 | A gift | Checkout field for the licensee's name | `Issue` | The key carries that name. Empty means the buyer's own name |
| 5 | Sold by a partner, live | Partner API `POST /v1/orders` | `Issue` | Keys come back in the response. The same partner order twice gives the same keys. Another partner's token cannot reach them |
| 6 | Sold by a partner, in advance | Command line: a batch for a partner | `Issue` per seat | A file of n keys, each recorded with the partner and its index |
| 7 | Given away | Command line: a named manual licence | `Issue` | A key from the source `manual`, recorded with its purpose |
| 8 | Lost key | Our lost-key page, an email address | `Resend` | Keys go only to that address. The page answers the same whether or not the address bought anything. Limited per address and per caller |
| 9 | Wrong name | Command line | `Reissue` | A new key with the next issue number. The old key keeps working |
| 10 | Chargeback | Paddle adjustment, action chargeback | `Revoke` | Every key of that transaction is on the next revocation list |
| 11 | Chargeback reversed | Paddle adjustment updated | `Restore` | Those keys leave the next revocation list |
| 12 | Key posted in public | Command line | `Revoke` and `Reissue` | That key is revoked. The buyer gets a new key, same licence ID, next issue number |
| 13 | Refund made anyway | Paddle adjustment, action refund | `Revoke` | Handled exactly like a chargeback |
| 14 | Planned key rotation | Config switch to the next signer | none | The next public key shipped in a release first. Keys of the old signer still check |
| 15 | Leaked signing key | Command line | `Genuine` | The genuine fingerprints of that signer are published. Keys of that signer check only if listed. No buyer acts |
| 16 | Publish revocations | Release workflow | `Revocations` | The feed carries the list, signed with the update key, which the licence service never holds |
| 17 | Sandbox | Paddle sandbox and a test signer | as above | The whole sale works end to end. No shipped build accepts the test signer |
| 18 | Mail fails | Mailer error | retry | Sending is retried. The thank-you page and the lost-key page still deliver |

## Architecture

One Go program, `cmd/framefairy-licence`, in three layers. A new shop or
partner is a new adapter, and the engine never changes for it.

```
Paddle webhook   our website   partner API call   command line
      \               |               |                /
       adapters: check Paddle's signature, the partner's token
                 and the price, then call the engine
                              |
       engine, licence/sell: Issue, Keys, Resend, Reissue,
                 Revoke, Restore, Revocations, Genuine
                              |
          Signer        Ledger        Orders        Mailer

key package, licence/: Sign and Check, the same code in the service and the app
```

### Key package: `licence/`

The format and nothing else, shared by the app and the service, so making
a key and checking one are the same code.

```go
type Licence struct {
	Format, Signer, Edition uint8
	ID                      [8]byte
	Issue                   uint16
	Issued                  time.Time // a day
	Name                    string
}

func Sign(l Licence, key ed25519.PrivateKey) (Key, error) // same input, same key
func Check(k Key, t Trust) (Licence, error)                // public keys, revocations, genuine lists
func IDFor(source, ref string, seat int) [8]byte
func (k Key) Fingerprint() Fingerprint
```

### Engine: `licence/sell/`

Every rule lives here: idempotence, one licence per seat, the next issue
number, what may be revoked and restored.

```go
type Order struct {
	Source, Ref string // "paddle" | "partner:<name>" | "manual", and its reference
	Seats       int
	Edition     uint8
	Name, Email string // Email is used to send, never stored
	At          time.Time
}

func (e *Engine) Issue(ctx context.Context, o Order) ([]licence.Key, error)
func (e *Engine) Keys(ctx context.Context, source, ref string) ([]licence.Key, error)
func (e *Engine) Resend(ctx context.Context, email string) error
func (e *Engine) Reissue(ctx context.Context, id [8]byte, name, why string) (licence.Key, error)
func (e *Engine) Revoke(ctx context.Context, t Target, why string) error
func (e *Engine) Restore(ctx context.Context, t Target, why string) error
func (e *Engine) Revocations(ctx context.Context) ([]licence.Fingerprint, error)
func (e *Engine) Genuine(ctx context.Context, signer uint8) ([]licence.Fingerprint, error)
```

### Ports: what the engine needs from outside

| Port | Does | First implementation |
| --- | --- | --- |
| `Signer` | Signs with the current key and names it | A key file readable only by the service. A hardware key later, same interface |
| `Ledger` | Appends events and reads them back | One file of JSON lines, see [The ledger](#the-ledger) |
| `Orders` | Finds the orders of an email address | Paddle's API |
| `Mailer` | Sends keys | A European mail service |

A Paddle key can always be made again from its transaction. The ledger is
for what only a record can do: revocations, genuine lists after a leak,
the next issue number, and orders Paddle never saw.

### Adapters: the network layer

| Adapter | Receives | Calls |
| --- | --- | --- |
| Paddle | `transaction.completed`, `adjustment.created`, `adjustment.updated`, signed by Paddle | `Issue`, `Revoke`, `Restore` |
| Thank-you page | A transaction, from our website | `Keys` |
| Lost key | An email address, from our website | `Resend` |
| Partner API | An order, with the partner's own token | `Issue`, `Keys`, `Revoke` |
| Command line | Us, on the server | Everything, and batches for partners |

### Partner API

The one surface other programs build against: small, versioned, the same
for everyone.

```
POST /v1/orders                {ref, seats, name, email, edition} -> {keys}
GET  /v1/orders/{ref}                                             -> {keys}
POST /v1/orders/{ref}/revoke   {why}                              -> {}
```

Each partner has its own token, which reaches only its own orders. The
same `ref` twice gives the same keys. A partner with its own webhook
format gets an adapter that turns it into these calls.

## The ledger

The ledger is the service's memory: one file, one line for every event,
in the order they happened. Nothing is ever changed or removed in it. A
correction is a new line, a `restore` after a `revoke`.

```json
{"seq":1042,"at":"2026-11-03T14:22:07Z","op":"issue","source":"paddle","ref":"txn_01k6x8m2c4p9qz7t3w5v8n1b2d","seat":1,"id":"36A3CE90838C1499","issue":0,"signer":0,"fp":"307dcf717b5fc2c26b3949a4dd12dfe1","prev":"5b0e…"}
{"seq":1043,"at":"2026-12-09T08:01:44Z","op":"revoke","id":"36A3CE90838C1499","issue":0,"fp":"307dcf717b5fc2c26b3949a4dd12dfe1","why":"posted on a forum","prev":"c41a…"}
```

The `prev` values are shortened here. In the file each is a whole SHA-256.

**What it holds.** Licence IDs, issue numbers, signers, fingerprints,
sources, references, seats, dates and reasons. No keys, no names, no email
addresses. A key can always be signed again from its line, and the buyer's
details stay at Paddle.

**Replaying it.** Everything the service knows is worked out by reading
the ledger from the first line to the last when it starts: which IDs
exist, the next issue number of each licence, what is revoked, which keys
each signer issued. There is no second store to drift from it. At 300 bytes
a line, 100,000 sales and their events are about 40 MB and read in about
a second, so it never needs compacting.

**Integrity.** Every line carries `prev`, the SHA-256 of the line before
it, so the lines form a chain. A line changed, removed or slipped in breaks
the chain from that point on, and the service refuses to start on a broken
chain. The newest link of the chain is also written into every update feed
the release workflow publishes, as `licences.ledger`, with its `seq`. The
feed is signed with the update key, on another machine, and every feed
stays in this repository's releases. So even someone who takes over the
server cannot rewrite history before the last published feed without it
showing.

**Writing it.** One process writes, holding a lock, so two copies of the
service can never both append. A line is appended and flushed to disk
before the service answers anyone, Paddle included, so nothing is
answered that is not recorded.

**Securing it.** The server runs the service and nothing else, reached
only over SSH with keys. The file belongs to the service's own user and no
one else can read it. It holds no personal data and no secrets: what an
intruder could do with it is read which transactions bought a licence.
The signing key is what an intruder would want, and it never touches the
ledger.

**Backing it up.** After every line, the file goes to object storage at a
second provider, in a bucket that keeps every version and locks each one
for a year. The service's credentials can write versions but not delete
them, so a taken-over server cannot destroy the backups. Two more copies
exist without anyone keeping them: Paddle knows every sale, and the
published feeds hold every revocation.

**Restoring it.** Take the newest backup, check its chain, and check its
newest link against the last published feed. Then the service asks
Paddle for every transaction and adjustment since the backup's last line
and runs each through the engine again. Keys are the same every time, so
replaying a sale recreates exactly the key the buyer already has, and
nothing is sent twice. Only partner orders and manual licences issued
after the last backup could be lost, and a backup after every line makes
that window seconds long.

**Checking itself every day.** Once a day the service compares Paddle's
completed transactions and adjustments with the ledger and runs anything
missing through the engine. That catches a webhook Paddle gave up on, and
a mistake of our own.

## When the service is down

The service does not need to be always up, and it is not built to be.

| What | While the service is down |
| --- | --- |
| The app | Unaffected. Keys are checked offline and never expire |
| A buyer paying | The payment completes at Paddle. The thank-you page says the key is on its way by email |
| The key for that sale | Paddle retries its webhook 60 times over 3 days. The first retry that lands issues the key and sends it. The daily check catches anything later |
| A lost key | The page answers once the service is back |
| Partners | Their calls fail and they retry, or they sell from keys made in advance |
| Revocations | Wait for the service. The last published list stays in force |

An outside check calls the service every few minutes and emails us when it
does not answer. Back within a day is good enough, and a server restarted
by the operating system after a crash is back within seconds.

## How the app checks a key

The app calls the same `licence.Check` the service is tested against,
offline, before every render.

- **Entering it.** A Licence row in the settings takes a pasted key and
  checks it at once. A refused key shakes the field and keeps what was
  typed, like a refused API key. A good key shows "Licensed to …".
- **Keeping it.** Where the API key is kept: the keychain on macOS.
- **Using it.** The engine checks before every render, so the app and the
  command line obey one rule: the mark goes on, or it does not.
- **Trust.** Public keys are built into the app. The revocation list and
  any genuine lists arrive with the update feed, see
  [How revocation reaches the app](#how-revocation-reaches-the-app).
- **No way around it in our own code.** Development builds need a key like
  any customer. No switch in any build turns the check off.

## Security, privacy, reliability, testing

**Security**

- The private signing key exists only in the service's `Signer`. Never in
  the repository, never in CI, never in the app. The update-signing key is
  a different key pair, held by the release workflow.
- Nothing reaches the engine unchecked: Paddle's webhook signature, the
  partner's token, the price ID, and the size of every field.
- The admin surface is the command line on the server, not an HTTP
  endpoint.
- The next signer's public key ships in a release before it is ever used.

**Privacy**

- The ledger holds licence IDs, fingerprints, sources, references and
  dates. No email addresses.
- Paddle holds the buyer's data. A deletion request is carried out there,
  and nothing of ours changes.

**Reliability**

- Every engine call is safe to repeat. Paddle retries webhooks, and a
  retry never makes a second key or a second email.
- The ledger is one chained JSON line per event, flushed before any
  answer, and backed up after every line, see [The ledger](#the-ledger).
- If the service is down, sales still complete at Paddle, see
  [When the service is down](#when-the-service-is-down).

**Testing**

- The engine is tested with the ports in memory and a fixed clock. No
  network, no Paddle, no mail.
- `licence.Check` has a fuzz target, as the repository requires for
  everything that reads untrusted text.
- Every use case has a test named after it. Asynchronous parts run under
  the race detector with concurrent calls.
- The whole sale runs once against Paddle's sandbox before launch.

## Binding a key to a machine

Possible, and decided against. It would work like this: the app sends an
identifier of the Mac to the service, the service answers with a signed
activation for that Mac, and the app only accepts the key alongside an
activation for the Mac it runs on. "One place at a time" needs more: the
app has to ask the service again every few days, or the activation runs
out.

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

What we do instead: the name on the key, the licence ID support can look
up, and revoking a key posted in public. If this is ever wanted, the gentle
form is an activation count, for example three Macs per licence, each
activated once online and then offline for good, with the list of Macs
shown to the buyer so they can remove one. It would be an engine call and
an adapter more, and the key format would not change.

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
- Which European host runs the service. It is a small server, not a
  serverless function, because the ledger is a file that has to stay.
- The business the Paddle account belongs to.
- Whether partners are wanted at launch, or the partner API waits for the
  first one.
