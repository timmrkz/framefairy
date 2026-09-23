# Third-party notices

Frame Fairy is made with other people's work: Go and the modules compiled
into the app, the packages its interface is built from, the speech runtime,
ffmpeg and llama-server with the libraries inside them, the caption fonts,
code taken into the engine by hand, and the models the app fetches. Almost
all of it may be used in a paid product on one condition, that its
copyright notice and licence text go with every copy. A link is not that:
the text itself goes.

## Where the notices are

In `notices/`, and nowhere else. `notices/notices.json` lists every piece
of work with its version, its licence, where it comes from, where it is in
Frame Fairy and anything the licence asks to be said besides its text, and
`notices/texts/` holds the texts. The app has them built in and shows them
under **Licences** on the rail, below **Settings**.

| Part | What |
| --- | --- |
| The app | Go, the Go modules compiled into the app, and pigo and pico, whose face detector and face data the engine carries |
| The app's interface | the npm packages in the interface bundle |
| Speech recognition | the sherpa-onnx Go bindings and ONNX Runtime with its own third-party notices |
| ffmpeg | ffmpeg, FreeType, FriBidi, HarfBuzz and libass |
| llama-server | llama.cpp and the eight small libraries inside the server |
| Caption fonts | Inter, Anton and Archivo Black, under the SIL Open Font Licence |
| Fetched by the app from its maker | the speech model, credited as CC BY 4.0 asks, and the language models |

## How they are made, and kept complete

Nothing in `notices/` is written by hand except `notices/kept/`, the two
notices no module brings: pigo and pico. `make notices` writes the rest
with `notices/gen`, from what the programs are really built from:

- `go list -deps` of `framefairy-app` and `framefairy`, for macOS, Linux
  and Windows, and each module's own licence file from the module cache
- the source maps of an interface build, which name every file of every
  package that went into the bundle, and each package's licence file
- the source trees in `.build/` that `make` builds ffmpeg and llama-server
  from, at the versions their build scripts pin
- the few texts nothing on disk holds, fetched from their makers: ONNX
  Runtime's licence and third-party notices at the version the speech
  library carries, and the texts of CC BY 4.0 and Apache 2.0 from the
  SPDX licence list

It refuses what it cannot place, a package with no licence file or a
licence it cannot name, rather than write a notice that says less than the
licence asks.

Three checks say when it is due:

- `TestEveryGoModuleCompiledInHasANotice` fails when a Go module is
  compiled into the app without a notice, or at another version. CI runs it
  on Linux and on macOS.
- `TestTheToolsNoticesAreForTheVersionsBuilt` fails when ffmpeg, one of its
  four libraries or llama.cpp is pinned to another version than its notice.
- The interface build fails when it bundles a package without a notice, or
  at another version, see `frontend/vite.config.ts`.

## Shipped beside the program: ffmpeg and llama-server

Two programs travel with `framefairy` rather than inside it. Both are
separate processes, started and stopped like any other, and nothing from
either is compiled into the `framefairy` binary. We build both ourselves,
from source, so what ships is a build we can describe rather than one
somebody else made.

**ffmpeg and ffprobe**, LGPL 2.1, https://ffmpeg.org

ffmpeg is LGPL until it is configured with `--enable-gpl`, and that flag
exists to allow GPL-licensed external encoders. The only one anybody wants
is libx264, and ours is built without it: the H.264 encoder comes from the
system instead, `h264_videotoolbox` on macOS. The build refuses to finish
if the result comes out GPL or if libx264 got in, and the finished binary
reports both facts about itself:

```
ffmpeg -L          the licence
ffmpeg -buildconf  the configure line it was built with
```

Shipping an unmodified LGPL binary asks for two things and we do both. The
licence text travels with it, as `LICENSE-ffmpeg.txt`, together with the
notices of the four libraries built into it. And the source it
was built from is published as `framefairy-tools-source.tar.gz` on the same
release as the binary, holding the upstream archives at the pinned versions
and the script that configured them.

It is built by `scripts/build-ffmpeg.sh`, which also carries the versions
of the four libraries inside it: freetype, fribidi, harfbuzz and libass.
libass is ISC and is not in ffmpeg's GPL list, which is checked against
ffmpeg's own configure in [PACKAGING.md](PACKAGING.md).

**llama-server**, MIT, https://github.com/ggml-org/llama.cpp

What runs a language model on the user's own machine. MIT asks for its
notice to travel with every copy, and it does, as `LICENSE-llama.cpp`,
together with the notices of the eight small libraries llama.cpp builds
into the server. It
is built by `scripts/build-llama.sh` from a pinned tag, without OpenSSL and
without the web interface, neither of which we use.

**How a copy is checked.** Every release of these carries a manifest with
the sha256 of each binary, and `scripts/verify-tools.sh` compares the copy
inside a built app against it. Each archive is also signed by GitHub with a
build attestation, so anyone can check where it came from without taking
our word for it:

```
gh attestation verify <the archive> --repo timmrkz/framefairy
```

## Not included: the models

No speech model and no language model ships. The app fetches them from
whoever published them, on its first run and with consent, so their
licences are between the user and their makers. They are credited under
**Licences** all the same, and the speech model with what CC BY 4.0 asks
for: its maker, its licence and what was changed, which is that the
sherpa-onnx project converted it to ONNX and quantised it to int8.

## Open: espeak-ng inside the speech library

The sherpa-onnx libraries as published, in the `sherpa-onnx-go-macos`,
`-linux` and `-windows` modules, have espeak-ng compiled into them, for
speech synthesis. espeak-ng is GPL 3. Frame Fairy never synthesises speech,
but a licence applies to what is shipped, not to what is used, and a GPL 3
library shipped inside a closed, paid app is what that licence does not
allow. sherpa-onnx builds without it when `SHERPA_ONNX_ENABLE_TTS` is off.
Until the speech library is built that way, it is not fit to ship.
