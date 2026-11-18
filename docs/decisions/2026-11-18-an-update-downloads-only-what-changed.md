# An update downloads only what changed

Issue #922, in one pull request, #923.

## The problem

Tim, 18 November: "Could you investigate why the app is made up of 67
megabytes to download when the app updates? Why is it just so much data
that needs to be downloaded? And where can we maybe compress this amount
of data?" And after the findings: "I want you to work on all of those
options in one pull request. Like everything that's reasonable to do,
everything that gets the data to be downloaded down."

What an update was on main at 6268d4f, read from the zip on the release
`dev`, 67.5 MB, 159 MB unpacked. Each file's share of the zip, in MB:

| File | MB | Changed from the build before |
| --- | ---: | --- |
| `framefairy-frames` | 15.2 | only Go's commit stamp, its code in 4 of the last 50 commits |
| `framefairy-app` | 14.3 | yes, 6.5 MB of it debug info |
| `ffmpeg` | 11.3 | no |
| `ffprobe` | 11.2 | no |
| onnxruntime and sherpa-onnx | 8.9 | no |
| `llama-server` | 6.3 | no |
| the rest | 0.1 | the signature list |

Three builds side by side, main, #905 and #919, differ in the app
program, the decoder and the signature list, nothing else. `ffmpeg`,
`ffprobe` and the decoder each hold their own static copy of ffmpeg's
libraries. Wails' updater takes one `.zip` or `.tar.gz`, hashes the bytes
`Source.Download` hands it, checks the signature over that hash, unpacks
it and swaps the app. It knows no delta.

Packed other ways, measured once on the cloud machine. A file packed the
same way comes out the same size every time.

| What | MB |
| --- | ---: |
| the zip as it is | 67.5 |
| without debug info | 57.3 |
| without debug info, one tar packed with zstd 19 and a long window | 28.9 |
| without debug info, each file packed alone with Go's zstd, best | 52.3 |
| the app program alone, packed alone | 7.2 |
| a binary diff of the app program against the build before | 6.6 |

The long window finds the three copies of ffmpeg's libraries, which a
file packed alone cannot. A binary diff of a Go program is no smaller
than the program packed, because a change moves every address after it.

## Five simulations

1. **Who may know about whom.** The trust stays where it is: the
   signature covers the whole zip, and Wails checks it over the bytes it
   is handed. What changes is how those bytes reach it. The zip becomes
   one whose files are stored, not deflated, so each file's bytes lie in
   it as they are. Beside it the build publishes a parts file, the zip as
   a list of parts in order: a few bytes of zip headers written out, or a
   file named by its SHA-256. Each file is uploaded once, packed with
   zstd, as `piece-<sha256>.zst`, and a build that has it already uploads
   nothing. `Source.Download` writes the parts in order, taking a file
   from the running app when it has the same bytes and downloading the
   piece when not. Names that cross: `updates.Parts`, `Part`, `Pack`,
   `Split`, `PieceName` and `PartsName`, between `cmd/framefairy-release`
   and the app, both in `updates/`. The parts file is untrusted like the
   list: a wrong part makes a zip whose hash fails, and the app then
   downloads the whole zip. Cost: about 400 lines of Go and tests, one
   command and one workflow step more. The next change, a new file in
   the bundle, needs nothing.
2. **What changes together.** Each build is published with a diff
   against the channel's build before it, zstd's `--patch-from`, and the
   app downloads the diff when it runs that build, the whole zip
   otherwise. Cost: a diff for every pair a person might update between,
   main to a pull request and back, or only the one pair, which misses
   most updates Tim makes, from one pull request to another. Measured: a
   diff of the app program is no smaller than the program packed.
3. **Where state lives.** The app keeps a store of files by their
   SHA-256, as git and OSTree do, and an update is a signed list of the
   files the new app holds. The app checks each file against the list,
   builds the new app out of the store and swaps it itself. Cost: the
   trust moves out of Wails' checked path into a list and a swap of our
   own, a second updater beside Wails' to keep safe. It shares the store
   by SHA-256 with the first.
4. **What runs together.** The programs that rarely change, ffmpeg,
   ffprobe, llama-server and the speech libraries, leave the update. The
   app fetches them once by checksum, like the models, and an update is
   only the app program and the decoder. Cost: an app that is no longer
   whole, signing and notarising programs that live outside it, and
   docs/UPDATES.md's first rule, an update replaces the app as a whole,
   given up. The insight, that these files never change, is what the
   first gets by itself.
5. **What a test has to build.** One stored zip and an index of where
   each file lies in it, and the app asks for byte ranges of that zip for
   the files it does not have, the way zsync does. Nothing else is
   uploaded. Cost: what is downloaded is unpacked, 19.5 MB for the app
   program where a packed piece is 7.2, and a test needs a server that
   answers ranges.

## What converges

- **Files by their SHA-256**, so the same bytes are never downloaded
  twice, in 1, 3 and 5.
- **The signature stays over the whole zip, checked by Wails**, in 1, 2
  and 5. Only 3 moves it, and that costs a second updater.
- **Each file packed with zstd**, in 1, 3 and 4. Go reads zstd with
  `klauspost/compress`, which is established and pure Go.
- **The app stays whole**, in all but 4.

Dropped: the diff of 2, which measured no smaller, the store of 3,
because the running app is the store that already exists, and the ranges
of 5, which download unpacked bytes.

Kept beside it, because no simulation removes them and each is a
smaller download in every case: the programs ship without their debug
info, Go's commit stamp leaves the decoder, so a build whose decoder did
not change has the same decoder bytes as the one before, and the three
copies of ffmpeg's libraries become one, which also answers what
docs/VIDEO-PREVIEW.md left open for the first release, that LGPL asks
for a linked library to be replaceable.

## The decision

The first simulation, drawn as layers, each using only the ones under it:

```
publish.yml           check, pack, sign, publish, one command a step
cmd/framefairy-release  pack: the stored zip, its parts, its pieces
                      publish: the zip, the pieces not there yet, the parts, the entry
                      channels: removes zips, parts and pieces no entry reaches
cmd/framefairy-app    hands Source the running app's folder
updates/              Parts, Part, Pack, Split, ParseParts, the assembling Download
Wails' pkg/updater    hashes, checks the signature, unpacks, swaps, as before
```

The zip stays at its address, so an app from before this change still
updates, downloading the stored zip whole once. Whenever the parts are
missing, wrong or a piece cannot be had, the app downloads that zip too.

## What follows

- A usual update downloads the app program, about 7 MB, where it was
  67.5. A change to the decoder adds its own Go part, the libraries are
  its no more. A build made on the Mac, with nothing to take from, gets
  every piece, about 30 MB.
- The kept build in the user's caches is the stored zip, about 160 MB
  where it was 67.
- Every publish uploads the stored zip and only the pieces not there yet.
- It was wrong if updates still download most of the app: the parts file
  and the pieces of a published build then say which file was not found.
