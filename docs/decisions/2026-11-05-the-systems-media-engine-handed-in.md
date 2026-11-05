# The system's media engine, handed in

5 November 2026, for #656.

## The problem

Tim, 5 November 2026: "when an issue comes up over and over again, we
have to like really fix it and not just move it away so that it fails
somewhere else. For instance, why is it so important that we are
encoding and decoding anything in CI? ... How does this verify our app?
And why wouldn't we abstract the encoder and decoder completely away
... maybe all we just have to do is to make sure that we initiate that
process properly so that we can verify we are handing over all of the
necessary inputs to that particular interface."

Measured on main at c714bca:

- The engine reaches Apple's media engine, VideoToolbox, from three
  places, and decides it by itself from `runtime.GOOS`:
  - the encoder of a short, `encoderOrderFor` in `engine/encode.go`,
    VideoToolbox first on a Mac, then Apple's encoder on the processor,
    then libx264,
  - ffmpeg's decoding for framing, `-hwaccel auto` from `decodeFlags`
    in `engine/analysis.go`, used in three places,
  - the episode's decoder, `cmd/framefairy-frames`, which opens a
    VideoToolbox device in `main` whenever it runs on a Mac.
- Every test that renders, frames or streams on CI's Mac went through
  it, because a test's machine reaches the same media engine as the app.
  56 calls of `ffmpegtest.Need` in 37 test files run ffmpeg, and 6
  test files start the episode's decoder.
- Every incident of #656, eleven failed runs since October, hung inside
  VideoToolbox, in `VTParavirtualizationGuestSendMessageWithIOSurfacesToHostAndCopyReplySync`,
  the call of GitHub's virtual Mac to the machine it runs on. None on
  Linux, where the same tests run ffmpeg's own code on the processor.
  None in Tim's 104 report logs.
- The ffmpeg we build is LGPL, without libx264, so on a Mac it has no
  encoder of H.264 but Apple's. Its own encoder of MPEG-4 part 2,
  `mpeg4`, is in every ffmpeg.
- One seam is there already: `e.system` on `Engine`, which a test sets
  to `"darwin"` to run the order of a Mac against a stand-in ffmpeg,
  four times in `engine/encoderfails_test.go`.

## Five simulations

### 1. Who may know about whom: a Go interface for all of ffmpeg

The engine knows an interface, `Media`, with `Encode`, `Decode` and
`Probe`, and only an adapter in a package of its own knows ffmpeg. A
test hands in a fake that records what it was asked.

- Layers: `engine/media` under the engine, ffmpeg under it.
- What moves: every call of `e.FFmpeg` and `e.FFprobe`, 19 in
  `engine/ffmpeg.go`, `render.go`, `analysis.go`, `ass.go`,
  `previewsound.go` and `encode.go`, and the filter graphs they carry.
- Crossing: a method for each kind of call, with the graph as its
  argument, so the graph is still built by the engine and the interface
  is ffmpeg's argument list in another shape.
- Next changes: a new filter crosses the interface.
- Cost: most of the render and the framing rewritten, a thousand lines
  and more. And a test of what a short looks like, the captions libass
  drew, a crop, a turned phone video, would test the fake.

### 2. What changes together: only what differs between machines

ffmpeg's own code on the processor is the same on every machine,
built by `make`, and never hung. What differs between machines and is
not ours is the system's media engine. That is the part to put behind
a seam, and nothing else.

- Layers: none new.
- What moves: the three places above read one value, what media engine
  the machine has, instead of `runtime.GOOS`.
- Crossing: a list of encoders, best first, for standard video and for
  HDR, and whether decoding goes to the system's decoder.
- Next changes: a Windows or Linux build's own encoders are a new value
  of it, written once.
- Cost: three files of the engine, the decoder's `main`, the tests that
  set `e.system`.

### 3. Where state lives: on the machine, empty until handed

The keychain and the network are on `engine.Machine`, empty in a
machine `NewMachine` makes, and each program hands them over with
`UseSystem` in `main`, see
[2026-10-31-running-programs-and-what-outlives-a-job.md](2026-10-31-running-programs-and-what-outlives-a-job.md#the-keys-and-the-network-648).
The media engine is the same kind of thing: the system's, outside the
program, and what a test must not reach.

- Layers: none new. `engine.Machine` gains one field.
- What moves: `e.system` on `Engine` goes, the machine's media takes
  its place, and `UseSystem` hands it the system's.
- Crossing: `UseSystem`, already called by both programs.
- Next changes: none outside the machine.
- Cost: no change in either `main`.

### 4. What runs together: the decoder is a program of its own

The episode's decoder runs in its own process and cannot read the
machine. Whether it may use the graphics chip has to cross the process
boundary, the way the episode's path does.

- Layers: none new.
- What moves: the decoder opens VideoToolbox only when it is started
  with `-chip`, and the engine passes it when its machine has the
  system's media engine.
- Crossing: one argument, `-chip`, before the episode.
- Next changes: a Windows decoder of its own would read the same flag.
- Cost: the decoder's `main`, `EpisodeFrames.start`, and the one place
  the app makes an `EpisodeFrames`.

### 5. What a test has to build: nothing of the system

A test makes `NewMachine`, and gets ffmpeg's own codecs on the
processor: libx264 where its ffmpeg has it, as on Linux, and `mpeg4`
otherwise, as on a Mac. What we hand the media engine is checked as an
argument list, against a stand-in ffmpeg that records it, the way
`fakeFFmpeg` and `failingMediaEngine` do today. No test runs
VideoToolbox.

- Layers: none new.
- What moves: the tests that check what Apple's media engine was given
  make a machine handed the Mac's media and a stand-in ffmpeg. A test
  that ran Apple's encoder for its own sake,
  `TestTheProcessorEncodesWhatAShortAsks`, goes: it tested Apple.
- Crossing: nothing new.
- What is given up: no test runs VideoToolbox any more. A change to what
  we hand it that Apple refuses shows first on a real Mac. What it is
  handed is pinned by tests to what ran on Tim's Mac.
- Cost: the tests of `encoderfails_test.go` and three tolerances in the
  decoder's tests that only allowed for the chip's rounding.

## What converges

Simulations 2 to 5 put the seam in the same place, around the system's
media engine and not around ffmpeg. 3 says where it lives, 4 how it
reaches the decoder, 5 what a test then builds. Simulation 1 is
dropped: ffmpeg is already reached through an argument list, which a
stand-in can record, and replacing it with a Go interface rewrites the
render while making the tests of what a short looks like test a fake.

The one thing only one simulation needs, `mpeg4`, is kept for a reason:
on a Mac without the media engine and without libx264 a short needs an
encoder, and `mpeg4` is in every ffmpeg. No program takes it, because
each hands its machine the system's media engine.

## The decision

```
both programs' main        UseSystem: the keychain, the network, the media engine
engine.Machine             media: encoders best first, and whether decoding goes to the system's decoder
                           empty: ffmpeg's own codecs on the processor
engine                     the encoder of a short, framing's decoding, the -chip of the episode's decoder
framefairy-frames          VideoToolbox only with -chip
ffmpeg, VideoToolbox       outside
```

- `engine.Media` is the system's media engine as the engine uses it:
  `Encoders` and `HDREncoders`, best first, and `Decodes`, whether
  ffmpeg and the episode's decoder hand decoding to the system's
  decoder. `SystemMedia(goos)` is the one of a system, the table
  `encoderOrderFor` was.
- `engine.Machine` keeps it. Empty, as `NewMachine` makes it, it is
  ffmpeg's own: libx264, then `mpeg4`, for standard video, libx265 for
  HDR, and decoding on the processor. `UseSystem` hands it the
  system's.
- `Machine.EpisodeFrames` makes the episode's decoder, started with
  `-chip` when the machine's media decodes.
- The tests of what the media engine is handed set `media` on their
  own machine, as they set `system` on their engine before.

## What follows

- Costs: the engine's tests on a Mac render with `mpeg4`, not H.264, so
  a test that reads the codec of a short reads the encoder's. An HDR
  short needs an encoder of HEVC in 10 bits, which a Mac has only in its
  media engine, so the test of HDR shorts runs where ffmpeg has libx265,
  on Linux.
- Buys: no test of our code waits on Apple's media engine, on any
  machine. A push stops failing for #656 without a CI change.
- Wrong if: a short or the video preview breaks on a real Mac in a way
  a test of what we hand VideoToolbox would not have caught. Then the
  argument it needed is added to that test, not a run of VideoToolbox.
- Not touched: what the app does when the media engine fails on a real
  Mac, the fallbacks to the processor, stays as it is.
