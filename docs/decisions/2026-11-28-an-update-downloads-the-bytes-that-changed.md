# An update downloads the bytes that changed

Builds on
[2026-11-18-an-update-downloads-only-what-changed.md](2026-11-18-an-update-downloads-only-what-changed.md).
Speaker Trail, whose updater is ported from this one, made the same
change in its record of 20 November 2026, and this one follows it where
the two apps are the same.

In 30 words: a changed program differs from the one before by a few
hundred kilobytes, not by its whole size. So each build also publishes a
patch of each changed file against builds the app is likely running.

## The problem

The user, 28 November 2026, after reading Speaker Trail's design: "One
thing that the Speaker Trail app does way better, as far as I understand
it, is that the updates are actually way smaller and a bit faster as
well." And then: "I think we should do all of the changes that are worth
taking and do all of them in one pull request here."

An update today downloads every file that changed, whole, as its piece
packed with zstd. The record of 18 November measured a binary diff of the
app program at 6.6 MB, no better than the piece, and dropped patches.
That diff was of the program with its debug information, which the same
pull request then stripped. Debug information names every address, so a
change scrambles all of it. Without it the diff is small.

Measured on 28 November in a cloud session, on the zips the release
`dev` held: main at c7fb725, and the builds of two pull requests open
that day, A at 4427efe, which changed the crop, and B at 5137d65. The
today column is what each build's own parts file says the app downloads,
the pieces of the files that differ. The patches were made with
`github.com/gabstv/go-bsdiff`. One run each, but a patch comes out the
same on every run, so there is no spread.

| Update | Changed programs | Today (MB) | With patches (MB) | Change |
| --- | --- | ---: | ---: | ---: |
| main to B | the app | 7.43 | 0.46 | 94 % smaller |
| main to A | the app, frames | 19.65 | 0.54 | 97 % smaller |

Of the 14 files of a build, only `framefairy-app` differed between main
and B, and `framefairy-frames` and the seal in `_CodeSignature` too
between main and A. Since the Developer ID signs every build again,
`kept.go` keeps the bytes of a library whose code did not change, so the
libraries stay out of an update.

A patch of the 20 MB app program took 9 to 10 s in the cloud session,
on one core.

## Five simulations

1. **Who may know about whom.** The design as it is, files by SHA-256,
   the running app's files kept and the rest downloaded as pieces, with
   the one compression it has, zstd, handed the file the app already
   holds as its dictionary. No new dependency and no new reader of
   bytes from outside. Cost: the app program from main a49d323 to main
   eb33cdf, one run each, packed to 1.33 MB in 0.8 s this way, and a
   bsdiff patch to 0.42 MB in 9.0 s, so about three times what a patch
   downloads.
2. **What changes together.** What changes between two builds is a few
   bytes of a file, not a file. So besides its pieces, a build publishes
   a patch of each of its files against the same file of the builds an
   app is likely to run: every build the channel list holds and the
   newest of each channel that left it, because the app of a pull
   request just merged runs a build the list no longer names. The parts name each patch by
   the SHA-256 of the file it starts from. The app downloads the patch
   when it has that file and the piece when not. Names that cross: a
   `Patches` field on `updates.Part`, `PatchName`, `ApplyPatch`. Cost: the
   publish downloads the pieces it patches from and runs bsdiff for each
   pair.
3. **Where state lives.** The app keeps every file it ever had by its
   SHA-256, as git does, and installs a build from that store with an
   installer of its own. Cost: a second updater beside Wails' to keep
   safe, a store that grows on the customer's disk, and patches still
   need a base, which the running app already is.
4. **What runs together.** The files that rarely change, ffmpeg,
   llama-server and the speech libraries, leave the update and are
   fetched once, like the speech models. Cost: an app that is no longer
   whole, with programs outside its signature. The design as it is
   already never downloads them twice, so this saves nothing an update
   downloads today.
5. **What a test has to build.** Each file is cut into chunks where its
   content says, as casync and restic do, and the app downloads the
   chunks it lacks. Cost: a Go program moves every address after a
   change, so most chunks change, and Speaker Trail measured chunks of an
   exact match at 0.24 to 1.52 MB where bsdiff made 0.07 to 0.79 MB.

## What converges

- **Files by their SHA-256**, in 1, 2, 3 and 5, so the same bytes are
  never downloaded twice.
- **The running app's file is what the new one is made from**, in 1 and
  2. A patch weighs a third of zstd with a dictionary, and the publish,
  not the app, pays the 9 s.
- **The signature over the whole zip, checked by Wails**, in 1, 2 and 5.
  Only 3 moves it, at the cost of a second updater.
- **The app stays whole**, in all but 4.
- **The running app is the base**, in 2 and 3. 2 needs no store.

Dropped: the dictionary of 1 and the chunks of 5, which measure larger
than a patch, the store of 3 and the split of 4.

## The decision

Simulation 2, on top of the parts there are:

```
publish-plan, in the     bases: the parts of the builds to patch against,
steps that hold a token  the channel's listed build first, then main's,
that reads               the other listed builds, and the newest build
                         of up to three channels that left the list,
                         and how many patches the
                         release has room for
publish.yml, publish     pack -bases: reads those parts and their pieces
(Mac, no token in pack)  from the release's public addresses, makes each
                         patch, keeps one only when the app's own code
                         makes the file from it and it weighs less than
                         the piece. publish uploads the pieces and the
                         patches before the entry
publish.yml, list        channels keeps the parts and pieces of the
                         bases, and removes the patches no listed build
                         names
updates/                 Part.Patches, ApplyPatch, the assembling
                         Download takes the patch when the running app
                         holds the file it starts from
Wails' pkg/updater       checks the signature, unpacks, swaps, as before
```

A patch is bsdiff's format, BSDIFF40, made by the release tool, which
only CI runs. The app applies it with code of its own and Go's
`compress/bzip2`, so the bytes it downloads reach no code but ours and
the standard library's. A patch is untrusted like a piece: what it makes
is checked against the file's SHA-256 in the parts, and the zip against
its signature. A patch that cannot make its file costs its piece, never
the update.

Which builds to patch against is worked out by `publish-plan`, in the
step that may read the release's file list with its token, and handed
to `pack` as names. So the step that reads the branch's build still holds
no token. The patches go up before the entry, unlike Speaker Trail's, so
an app that sees the entry finds them there: the app looks every 20 s
while a build is coming, and an entry up before its patches would have
it download the pieces.

## What follows

- A usual update of a pull request downloads a few hundred kilobytes in
  place of 7 to 20 MB.
- An app that runs a build that is neither listed nor the newest of
  its channel among the three that left the list takes the pieces, as
  today.
- The first build published after this merges has no parts with patches
  to point at, but its patches are made from the pieces the release
  already holds, so it has them.
- Every publish downloads the pieces of its bases and runs bsdiff for
  each changed file and each base, side by side on the Mac's cores, for
  90 s at most. A patch not begun by then is not made, so a build that
  changes every program keeps its job under five minutes. The publish is
  slower by that, see the pull request.
- It was wrong if updates still download whole pieces of the app
  program: the app's log says what each update kept, patched and
  fetched.
