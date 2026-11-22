# Signing, updates, licences and payments

Part of the security skill. Read it before touching a signing key, an
update path, a release workflow or a licence check. Every rule here
comes from a flaw found in a real update and release pipeline.

## Signing, updates and releases

- **Verify the whole artifact before unpacking it,** with a public key
  built into the program. Parts fetched separately are checked through
  the hash of the whole, and a failure falls back to the whole download,
  never to trusting the part.
- **Sign what a build claims to be,** its channel, version, commit, size
  and hash together, and check the claim before downloading. A signature
  over the file alone lets any build ever signed be offered as any
  channel and any version.
- **Every signature starts with a fixed line naming its use and format
  version,** so a signature made for one thing never verifies another.
- **Never run a build's code with the signing key in reach.** The step
  that reads the untrusted build holds no secret and no write token. The
  sign step runs after it, tied by an explicit condition on its result,
  never by step order alone.
- **The judge runs from the protected branch,** so a branch that loosens
  the signing step's lists changes nothing about how its own build is judged.
- **Identity comes from the CI system's record of the run,** not from
  anything the build wrote, and a ref is compared by its commit, never by
  its name: a tag named like the main branch once made a branch's build
  read as main's.
- **No rollback.** A build older than the one published for its channel
  is not published again, so re-running an old job replaces nothing.
- **Refuse capabilities before signing.** Read every file of the build
  for what it imports, links and names: input taps, screen capture,
  camera, privilege elevation, set-user-ID bits, new frameworks,
  entitlements. A file that looks like a program but will not parse
  fails. Each change to the lists is a reviewed line with its reason.
- **Strangers' code is never signed.** Only the repository's own
  branches get development builds, never forks.
- **Attest what CI built,** and verify the attestation pinned to the
  workflow that made it, not to any workflow of the repository.

- **One key per job.** The key for what customers install is not the key
  for development builds, so a leak reaches less. An admin token stays
  offline.
- **Plan rotation before you need it.** The next public key ships in a
  release before it signs anything, because a key every installed copy
  trusts cannot be changed from outside.

## Licences and payments

- Verify every webhook's signature and every field's size before any
  other code sees it. Replayed, late, reordered and duplicated events must
  never sell twice, so every step is idempotent.
- The private key that signs licences lives on one machine that accepts
  no connections.
- **Weigh a check by who it stops.** With public source, a check one
  deleted line removes stops nobody determined and burdens every honest
  buyer. Machine binding and online checks were dropped for that.
  Revoking a posted key, and a key ID support can look up, do the work.
- Keep no names or emails you do not need. The payment provider holds
  them.
