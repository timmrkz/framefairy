# Testing the interface

What runs where, and how to see the interface do what a person does with
it, without a screen. The engine's own tests, the fuzz targets and the
path tests are in [BUILD.md](BUILD.md). This page is about the interface.

## Two ways to open it in a browser

The app cannot be started in a cloud session, so the interface is opened
in Chromium, through Playwright, with something answering the calls it
makes to the Go side.

| | Answers the calls | Good for |
| --- | --- | --- |
| the preview, `frontend/preview/vite.config.ts` | `wails-stub.ts`, a stand-in written in TypeScript | the layout, a picture of any state, states the engine is slow to reach |
| the bridge, `frontend/preview/bridge.config.ts` | the real Go side, `cmd/framefairy-app/bridge_test.go` | what the app does: the words, the captions, the clips, undo |

The preview is quick and can be made to show anything, but it is a second
engine, and it drifts from the first. It laid captions out eight words at
a time and knew nothing of pauses, so a caption breaking where a removed
word had been could not be seen in it at all. Anything about what the
engine answers is tried against the bridge.

## The bridge

The bridge serves the interface with the service the app runs, over a desk
from the path tests in `driver_test.go`: the real queue, engine, words,
waveform and ffmpeg, with stand-ins only for the two models. The speech
stand-in says a few German sentences over and over, with a pause after
each, so a word on screen twice can be told from the words around it. The
language model stand-in finds one clip. The episode is two minutes of a
grey that grows lighter, in VP9 and Opus, because the Chromium Playwright
brings has no H.264 and would not play it.

In the browser, `wails-bridge.ts` takes the place of the Wails runtime: a
call is a POST to `/call`, and what the Go side tells the interface comes
as server-sent events on `/events`. `window.__calls` lists every call and
how it ended, and `window.__menu("undo")` and `window.__menu("redo")` are
the menu bar, which a browser does not have.

Calls that reach beyond the work folder are answered by the bridge with
nothing: the network, the keychain, a box from the system, another app.
So nothing a walk presses downloads a model or installs a build. The setup
counts the two stand-ins as installed, so the app opens on its episodes
rather than on its first-run screen.

To open it by hand:

    cd frontend && npx vite build --config preview/bridge.config.ts
    FRAMEFAIRY_BRIDGE=127.0.0.1:8123 go test -run '^TestBridge$' -timeout 0 ./cmd/framefairy-app

It hears and searches the episode first, which takes a few seconds, then
says where it is and serves until it is stopped.
