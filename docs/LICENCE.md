# Licences

How Frame Fairy is sold, how a licence key is made and checked, and what
the licence service is built from. The licence is batch 5.8 in
[GUI-PLAN.md](GUI-PLAN.md), and it is built last. The reasons behind the
update policy and the public code are in [UPDATES.md](UPDATES.md).

## What is decided

- **A licence is bought once and gets every update, for ever.** The key
  only decides whether a render carries the Frame Fairy mark.
- **Paddle sells the licence**, as merchant of record: payment, VAT and
  sales tax in every country, invoices, chargebacks.
- **We make the keys ourselves.** A key is a licence signed with our
  Ed25519 private key. The app checks it offline against the public key
  built into it. No network, no machine binding, no activation count.
- **No refunds once the key has been delivered.** The buyer agrees at
  checkout that delivery starts at once and that the right of withdrawal
  ends with it, the way Paddle's buyer terms put it. The app never phones
  home, so whether a key was entered is something we cannot know, and
  delivery is the moment the policy can stand on.
- **Revocation is narrow.** A chargeback and a key posted in public are
  revoked, nothing else. The list travels in the signed update feed.
- **Affiliates are Tolt's**, on top of Paddle. The licence service knows
  nothing about them.

## What a licence is

A licence is a few fields, signed.

| field | size | what it is |
| --- | --- | --- |
| format | 1 byte | the key format, 1 today. A later format is a superset, and every format ever issued is read |
| signer | 1 byte | which signing key signed it, so a key pair can be replaced |
| number | 8 bytes | the licence, derived from where it came from, see below |
| issue | 2 bytes | 0 for the first key of a licence, one more for every key issued again for it |
| edition | 1 byte | what the licence unlocks. 0 is Frame Fairy as it is sold today |
| issued | 2 bytes | the day it was issued, counted from 2026-01-01 |
| name | up to 64 bytes | who it is licensed to, as the settings show it. Empty is allowed |

The signature is 64 bytes over all of it. The key is `FF1-` followed by
the fields and the signature in base64url, about 200 characters. Nobody
types it, it is pasted.

**The number comes from the sale, not from a counter.** It is the first
8 bytes of SHA-256 over where the licence came from: `paddle` and the
transaction, `partner`, the partner and its order, or `manual` and the
name it was given, each with the seat. The same sale always gives the
same number, and an Ed25519 signature over the same fields is always the
same signature, so the same sale always gives the same key. That is what
makes issuing safe to repeat and a lost key easy to give back.

A **fingerprint** is the first 8 bytes of SHA-256 over the whole key. It
is what the ledger, the revocation list and the list of genuine keys
hold, never the key itself.

## Every way a licence comes into being or changes

Everything the licence service does is one of these.

**Selling**

1. **Bought through Paddle.** Paddle's `transaction.completed` arrives.
   The service checks Paddle's signature on it and that the price is one
   of ours, by price ID and not by amount, because a discount changes the
   amount. It issues one licence per seat, records them and emails them.
   The same message arriving twice gives the same keys and sends nothing
   twice.
2. **Shown on the thank-you page.** Checkout sends the buyer to our page
   with the transaction. The page asks the service for the keys of that
   transaction, which answers once Paddle has reported the sale, and a
   few seconds of waiting are shown as waiting. The email is the second
   copy, not the only one.
3. **Several seats in one purchase.** An agency buying five gets five
   licences, one key each, all sent to the buyer, each with its own
   number. Nothing enforces seats, a seat is a key.
4. **Bought as a gift.** Checkout asks whose name goes on the licence, and
   leaves it empty when it is the buyer's own.
5. **Sold by a partner.** A bundle, a deal site or a reseller sells
   Frame Fairy with its own checkout. Either it calls our partner API on
   every sale, with its order and a token of its own, or it takes a batch
   of keys made in advance. Both are licences like any other, from the
   source `partner`.
6. **Given away.** Press, reviewers, affiliates who want to try it,
   giveaways on a podcast. We issue them by hand, from the source
   `manual`, with a name that says what they were for.

**After the sale**

7. **A lost key.** The buyer enters their email on our website. The page
   always answers the same thing, that the keys have been sent if there
   are any, so it never says whether an address bought anything. The
   service asks Paddle for the transactions of that address and emails
   their keys to it, never to anyone else and never on screen.
8. **A name that is wrong.** The licence is issued again with the right
   name and the next issue number. The old key keeps working, because
   nothing about it was wrong in a way that matters.
9. **A chargeback.** Paddle reports an adjustment with the action
   chargeback. Every key of that transaction is revoked.
10. **A chargeback reversed.** The bank decides for us, Paddle reports it,
    and the revocation is taken back.
11. **A key posted in public.** We revoke that key by hand and issue the
    licence again with the next issue number, and the buyer gets the new
    key by email. The number stays, so everything about the licence stays
    one licence.
12. **A refund made anyway.** Paddle may still refund, for example before
    the key was delivered. It is revoked like a chargeback.

**Keys and trust**

13. **A new signing key, planned.** The next public key goes into a
    release long before it signs anything. Then the service switches to
    it. Every key signed before keeps working.
14. **A signing key that leaked.** The ledger lists every key that key
    really signed. The service writes their fingerprints out, the feed
    carries them, and from then on a key of that signer counts only if
    its fingerprint is on that list. The service switches to the next
    signing key. No buyer does anything.
15. **The revocation list.** The service writes it out from the ledger.
    The release workflow puts it into the update feed and signs the feed
    with the update key, which the licence service never holds.

**Running it**

16. **Paddle's sandbox.** A test of the whole sale goes through Paddle's
    sandbox and a test signing key. No build we ship accepts the test
    key, because its private half is in this repository for the tests.
17. **Mail that failed.** Sending is retried, and the thank-you page and
    the lost-key page are always there as well.
18. **Personal data.** The ledger holds numbers, fingerprints, sources and
    dates, and no email address. Paddle holds the buyer. A request to
    delete someone's data is Paddle's to carry out, and nothing of ours
    needs changing.

## The licence service

One Go program, `cmd/framefairy-licence`, in this repository. It is two
layers: an engine that knows what a licence is and what may happen to
one, and adapters that turn whatever arrives, a Paddle webhook, a
partner's call, a command typed by us, into a call to the engine. A new
partner or a new shop is a new adapter. The engine does not change.

### The key package

`licence/` holds the format and nothing else, and it is the one package
both the app and the service use, so a key is made and checked by the
same code.

```go
package licence

type Licence struct {
	Format  uint8
	Signer  uint8
	Number  uint64
	Issue   uint16
	Edition uint8
	Issued  time.Time // a day
	Name    string
}

// Sign makes the key. The same licence and the same key give the same
// key, every time.
func Sign(l Licence, key ed25519.PrivateKey) (Key, error)

// Check reads a key and says what it licenses, or why it does not. It is
// what the app calls. Trust holds the public keys, the revocation list
// and, for a signer that leaked, the genuine fingerprints.
func Check(k Key, trust Trust) (Licence, error)

// NumberFor is where a licence number comes from.
func NumberFor(source, ref string, seat int) uint64

func (k Key) Fingerprint() Fingerprint
```

`Check` reads untrusted text, so it has a fuzz target.

### The engine

`licence/sell/` is everything a sale can do, behind one type. It holds
the rules: idempotence, one licence per seat, the next issue number,
what may be revoked and taken back.

```go
package sell

// An order is a sale from anywhere, already checked by the adapter that
// received it.
type Order struct {
	Source  string // "paddle", "partner:<name>", "manual"
	Ref     string // the transaction, the partner's order, our own name for it
	Seats   int
	Edition uint8
	Name    string // on the licence, may be empty
	Email   string // where the keys go, never stored
	At      time.Time
}

type Engine struct{ /* the ports below */ }

// Issue gives the keys of an order, issuing them the first time.
// Calling it again gives the same keys and sends nothing again.
func (e *Engine) Issue(ctx context.Context, o Order) ([]licence.Key, error)

// Keys gives the keys of an order that was issued, for the thank-you
// page and for support.
func (e *Engine) Keys(ctx context.Context, source, ref string) ([]licence.Key, error)

// Resend emails every key bought with an address to that address.
func (e *Engine) Resend(ctx context.Context, email string) error

// Reissue gives a licence a new key, with a new name or after its key
// was posted in public.
func (e *Engine) Reissue(ctx context.Context, number uint64, name string, why string) (licence.Key, error)

// Revoke and Restore act on every key of an order, or on one key.
func (e *Engine) Revoke(ctx context.Context, t Target, why string) error
func (e *Engine) Restore(ctx context.Context, t Target, why string) error

// Revocations and Genuine are what the release workflow publishes.
func (e *Engine) Revocations(ctx context.Context) ([]licence.Fingerprint, error)
func (e *Engine) Genuine(ctx context.Context, signer uint8) ([]licence.Fingerprint, error)
```

### What the engine needs from outside

Four small interfaces, so the engine is tested with nothing but memory
and a clock, and so where things live can change without the engine
knowing.

| port | what it does | first implementation |
| --- | --- | --- |
| `Signer` | signs with the current key and says which one it is | a key file readable only by the service. A hardware key or a cloud key service later, behind the same interface |
| `Ledger` | appends what happened and reads it back | one JSON line per event, `issue`, `reissue`, `revoke`, `restore`, in a file on the server with a copy in object storage that keeps versions |
| `Orders` | finds the orders of an email address | Paddle's API |
| `Mailer` | sends the keys | a European mail service, Brevo or Mailjet |

The ledger is not needed to make a Paddle key, which can always be made
again from the transaction. It is needed for what only a record can do:
the revocation list, the genuine fingerprints after a leak, the next
issue number of a licence, and the orders of partners and giveaways,
which Paddle knows nothing about.

### The adapters

| adapter | what arrives | what it calls |
| --- | --- | --- |
| Paddle | `transaction.completed`, `adjustment.created`, `adjustment.updated`, signed by Paddle | `Issue`, `Revoke`, `Restore` |
| thank-you page | a transaction, from our website | `Keys` |
| lost key | an email address, from our website, limited per address and per caller | `Resend` |
| partner API | an order, with the partner's own token | `Issue`, `Keys`, `Revoke` |
| the command line on the server | us | everything, and batches of keys for a partner |

**The partner API** is the one other programs build against, so it is
small, versioned and the same for everyone.

```
POST /v1/orders            {ref, seats, name, email, edition}  -> {keys}
GET  /v1/orders/{ref}                                          -> {keys}
POST /v1/orders/{ref}/revoke  {why}                            -> {}
```

Each partner gets its own token, which only reaches its own orders. The
same `ref` twice gives the same keys. A partner with its own webhook
format, as deal sites tend to have, gets an adapter of its own that
turns it into these calls, and the engine never sees the difference.

Nothing reaches the engine over the network without being checked
first: Paddle's signature, the partner's token, the price ID, the size
of every field.

## How the app checks a key

- The Licence row in the settings takes a pasted key and calls `Check`
  at once. A key that is refused shakes the field and keeps what was
  typed, the way a refused API key does.
- The key is kept where the API key is kept, in the keychain on macOS.
- The engine asks `Check` before every render, so the command line and
  the app obey the same rule, and the mark goes on or it does not.
- The public keys are built into the app. The revocation list and any
  genuine fingerprints come with the update feed, and only a feed whose
  signature holds is read. A feed that is missing, broken or unsigned
  changes nothing: the last list that held stays.
- Development builds need a key like any customer, and no switch in any
  build turns the check off.

## What is left open

- The price, and whether there is ever a second edition.
- The mail service, Brevo or Mailjet, and where the service runs.
- The business the Paddle account belongs to.
- Whether partners are wanted at launch, or the partner API waits for
  the first one.
