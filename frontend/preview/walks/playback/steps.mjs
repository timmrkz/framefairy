// The steps of the sequences about the video preview: playing, looping, the
// frames it shows and the crop frame. Each is said above it, a list with
// its name first. walkSequence in ../sequences.mjs walks it, and hands it
// the walk so far, the step's argument, what the caption box showed before
// it and the step's name. A sequence of any folder can take it. Below them
// the helpers only these steps use. See docs/TESTING.md.
import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { addHEVC, ask, chosen, clickOnTrack, cropFrame, drag, engineState, episodeOn, filmedOnScreen, fromSidebar, handles, high, loudness, middle, news, nextNews, nextPaint, playUntil, preview, readShort, recordFrames, sameLook, sayLook, seekTo, settle, shortFrames, shownOnPlayhead, xOf } from "../bridge.mjs";
import { nextCall, onePiece, pieceCount, rate, restAhead, shortOf, url } from "../common/steps.mjs";
import { overlaid, stillFrame } from "../rules.mjs";

export const steps = {
  // ["read ahead", key] rests until what the video preview reads ahead is
  //                    in, presses the key and checks that the frame it
  //                    lands on was there already, read while the hand
  //                    rested: it came from the frames kept, see
  //                    AppFrames.ahead
  async "read ahead"(walk, arg, s) {
    const { page, watch } = walk;
    // The hand rests until what the video preview reads ahead is in.
    await settle(page);
    await restAhead(page);
    const came = await page.evaluate(async (key) => {
      const frames = window.__appFrames;
      const before = Number(document.querySelector(".screen")?.dataset.playhead);
      window.dispatchEvent(new KeyboardEvent("keydown", { key: key.replace("Shift+", ""), shiftKey: key.startsWith("Shift+"), bubbles: true }));
      // How the frames came by the time the video preview shows the
      // frame that holds the playhead, its first frame of the word.
      // Read each time the video preview says another playhead or
      // another frame shown.
      const el = document.querySelector(".screen");
      return new Promise((done) => {
        const check = () => {
          const at = Number(el?.dataset.playhead);
          const shown = Number(el?.dataset.shown);
          const holds = at !== before && shown <= at + 1e-6 && at - shown < frames.track.frame - 1e-6;
          if (!holds) return false;
          // The rank of the frame on screen, and whether it was one
          // handed over from the frames kept.
          let rank = 0;
          while (rank < frames.track.samples.count - 1 && frames.ptsOf(rank + 1) <= shown + 1e-6) rank++;
          done({ at, fromKept: frames.stats.keptRanks.includes(rank) });
          return true;
        };
        if (check()) return;
        const o = new MutationObserver(() => check() && o.disconnect());
        o.observe(el, { attributes: true, attributeFilter: ["data-playhead", "data-shown"] });
      });
    }, arg);
    // The frames after it, which the step's cue reads ahead, and the
    // frame of the word after the one it lands on, which is read ahead
    // now, may come from a stream. The frame it lands on may not.
    if (!came.fromKept) walk.wrong = `${arg} to ${came.at.toFixed(3)} s had to fetch its frame, though the hand rested until what is read ahead was in`;
    await watch.step("press", s);
  },
  // ["heard"]          rests until the app has settled, plays with the
  //                    space bar and asks that the sound card was handed
  //                    sound for most of the next second and a half of
  //                    sound, counted in the sound card's own quanta, then
  //                    pauses
  async heard(walk) {
    const { page } = walk;
    await settle(page);
    // What the sound card is handed, a render quantum at a time, the
    // way "loop off near the end" records it.
    await page.evaluate(async () => {
      const q = window.__frameQueue;
      if (!window.__loudness) {
        const code = `registerProcessor("loudness", class extends AudioWorkletProcessor {
              process(inputs) {
                const ch = inputs[0][0];
                if (ch) { let sum = 0; for (const v of ch) sum += v * v; this.port.postMessage(sum / ch.length); }
                return true;
              }
            });`;
        await q.audio.audioWorklet.addModule(URL.createObjectURL(new Blob([code], { type: "text/javascript" })));
        const node = new AudioWorkletNode(q.audio, "loudness", { numberOfOutputs: 1 });
        node.port.onmessage = (e) => {
          window.__loudness.push(e.data);
          const w = window.__loudnessWanted;
          // Enough, or the play stopped, after which no more come.
          if (w && (window.__loudness.length >= w.n || q.state !== "playing")) {
            window.__loudnessWanted = null;
            w.done();
          }
        };
        const hush = q.audio.createGain();
        hush.gain.value = 0;
        q.out.connect(node).connect(hush).connect(q.audio.destination);
        // Held, or the recorder is collected while it records.
        window.__loudnessNodes = [node, hush];
      }
      window.__loudness = [];
    });
    await page.keyboard.press("Space");
    await settle(page);
    // The app has done what the space bar set going, but a play
    // starting waits for its first frames and the sound card, which
    // are no call and no timer, so it is heard from the queue: the
    // first report once it has started or given up. On a busy machine
    // it was still starting here. A play that never starts is a walk
    // that hangs, ended by the -timeout of make walks.
    const playing = await page.evaluate(
      () =>
        new Promise((done) => {
          const q = window.__frameQueue;
          if (q.state !== "starting") return done(q.state);
          const off = q.listen(() => {
            if (q.state === "starting") return;
            off();
            done(q.state);
          });
        }),
    );
    if (playing !== "playing") {
      walk.wrong = `the space bar left the play ${playing}`;
      return;
    }
    // A second and a half is about 560 quanta at 48 kHz, which the
    // sound card hands over at its own pace whatever the machine does.
    await page.evaluate(
      () =>
        new Promise((done) => {
          window.__loudness = [];
          window.__loudnessWanted = { n: Math.round((1.5 * window.__frameQueue.audio.sampleRate) / 128), done };
        }),
    );
    const got = await page.evaluate(() => {
      const xs = window.__loudness;
      return { quanta: xs.length, loud: xs.filter((x) => x > 1e-6).length, state: window.__frameQueue.state };
    });
    await page.keyboard.press("Space");
    await settle(page);
    // The test video's sound is a tone from its first moment to its
    // last.
    if (got.state !== "playing") walk.wrong = `the space bar left the play ${got.state}`;
    else if (got.loud < got.quanta * 0.8)
      walk.wrong = `the sound card was handed sound in ${got.loud} of ${got.quanta} render quanta of a second and a half of play`;
  },
  // ["loop off near the end", seconds]
  //                    records every sample handed to the sound card,
  //                    plays the clip from where the playhead is, presses
  //                    L to loop it, and once the playhead is so many
  //                    seconds before the clip's end presses L five times
  //                    quickly, the loop off after the last, then reads
  //                    the playhead, the frame on screen and the play
  //                    button on every animation frame until the play
  //                    stops, and a second after
  async "loop off near the end"(walk, arg) {
    const { page, watch } = walk;
    const end = (await engineState(page, watch.at)).segments.at(-1).end;
    await preview(page, 0.2);
    // Every sample the sound card is handed, with the sound card's own
    // frame it plays at, the way the frame queue's probe records it.
    await page.evaluate(async () => {
      const q = window.__frameQueue;
      const code = `registerProcessor("recorder", class extends AudioWorkletProcessor {
            constructor() {
              super();
              // Asked to flush, it posts what it holds and then says so.
              // Messages on one port come in order, so after "flushed"
              // every sample it was handed has been heard.
              this.port.onmessage = () => {
                if (this.batch?.n) this.port.postMessage({ frame: this.batch.frame, data: this.batch.data.slice(0, this.batch.n) });
                this.batch = null;
                this.port.postMessage("flushed");
              };
            }
            process(inputs) {
              const ch = inputs[0][0];
              // Chromium now and then hands two render quanta the same
              // currentFrame and the one after that two on, see the frame
              // queue's probe, preview/frames/main.ts.
              const frame = Math.max(currentFrame, (this.last ?? -Infinity) + 128);
              this.last = frame;
              // Sent 32 quanta at a time: a message for each, 375 a second,
              // stopped coming after about 8000 of them.
              if (!ch) return true;
              if (!this.batch) this.batch = { frame, data: new Float32Array(4096), n: 0 };
              if (this.batch.frame + this.batch.n !== frame && this.batch.n) {
                this.port.postMessage({ frame: this.batch.frame, data: this.batch.data.slice(0, this.batch.n) });
                this.batch = { frame, data: new Float32Array(4096), n: 0 };
              }
              this.batch.data.set(ch, this.batch.n);
              this.batch.n += ch.length;
              if (this.batch.n + 128 > 4096) {
                this.port.postMessage({ frame: this.batch.frame, data: this.batch.data.slice(0, this.batch.n) });
                this.batch = null;
              }
              return true;
            }
          });`;
      await q.audio.audioWorklet.addModule(URL.createObjectURL(new Blob([code], { type: "text/javascript" })));
      const node = new AudioWorkletNode(q.audio, "recorder", { numberOfOutputs: 1 });
      window.__heard = [];
      node.port.onmessage = (e) => (e.data === "flushed" ? window.__flushed?.() : window.__heard.push(e.data));
      const hush = q.audio.createGain();
      hush.gain.value = 0;
      q.out.connect(node).connect(hush).connect(q.audio.destination);
      // Held, or the recorder is collected while it records.
      window.__recorder = [node, hush];
    });
    if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
      await page.keyboard.press("Space");
    }
    // Each wait is in the episode's time, read off the playhead, see
    // playUntil, once the app has settled on the press.
    await settle(page);
    await playUntil(page, { most: 0.3 });
    // What the play had started, thrown away and handed over twice
    // before the presses of L, see the frame queue's stats.
    const counts = () =>
      page.evaluate(() => {
        const { starts, soundThrown, soundTwice } = window.__frameQueue.stats;
        return { starts, thrown: soundThrown, twice: soundTwice };
      });
    const before = await counts();
    await page.keyboard.press("l");
    await playUntil(page, { past: end - arg });
    await page.evaluate(() => {
      window.__played = [];
      window.__playWatch = true;
      const tick = () => {
        if (!window.__playWatch) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        const playing = !!document.querySelector('button[aria-label="Pause"]');
        window.__played.push([performance.now(), at, window.__pictured(), playing]);
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
    // L pressed quickly five times, the way Tim did, the loop off after
    // the last.
    for (let i = 0; i < 5; i++) {
      await page.keyboard.press("l");
      await playUntil(page, { most: 0.12 });
    }
    const pressed = await counts();
    // Until the play stops at the end, or has gone on by what was left
    // of the clip and eight seconds more, and then until the queue
    // holds the sound card, see end in lib/frames/queue.ts: whatever
    // the sound card is handed after the end, where the start played
    // on when Tim heard it, comes before that, and nothing comes
    // after. Then the recorder is flushed, so every sample it was
    // handed is in. A play that did not end holds nothing, and is said
    // below.
    await playUntil(page, { most: arg + 8 });
    await page.evaluate(
      () =>
        new Promise((done) => {
          const q = window.__frameQueue;
          const a = q.audio;
          let off = () => {};
          const look = () => {
            if (q.state === "ended" && a.state === "running") return;
            a.removeEventListener("statechange", look);
            off();
            const [node] = window.__recorder;
            window.__flushed = done;
            node.port.postMessage("flush");
          };
          a.addEventListener("statechange", look);
          off = q.listen(look);
          look();
        }),
    );
    const q = await page.evaluate(() => {
      window.__playWatch = false;
      const q = window.__frameQueue;
      return {
        starts: q.stats.starts,
        c0: q.c0,
        m0: q.m0,
        rate: q.audio.sampleRate,
        programEnd: q.program.end,
        late: q.stats.lateSoundAt,
        heard: window.__heard.map((h) => ({ frame: h.frame, data: Array.from(h.data) })),
      };
    });
    walk.loopEnd = { end, fps: await rate(page), ...q, starts: q.starts - before.starts, thrown: pressed.thrown - before.thrown, twice: pressed.twice - before.twice };
    await settle(page);
  },
  // ["loop through the end", seconds]
  //                    puts the playhead so many seconds before the clip's
  //                    end, plays, switches the loop on with L at once and
  //                    lets it run round the end and three seconds into
  //                    the next time through, then asks that from half a
  //                    second after it came round the video preview shows
  //                    the frame of the playhead, give or take two
  async "loop through the end"(walk, arg) {
    const { page, watch } = walk;
    // Played with the loop switched on early, before the play works
    // out the next time through, and left to run on through the
    // clip's end into its start. Tim saw the sound go round while the
    // video preview stood on the clip's last frame (2.264).
    const segs = (await engineState(page, watch.at)).segments;
    const start = segs[0].start;
    const end = segs.at(-1).end;
    const fps = await rate(page);
    await seekTo(page, end - arg, fps);
    await preview(page, 1 / fps);
    if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
      await page.keyboard.press("Space");
    }
    await settle(page);
    await playUntil(page, { most: 0.3 });
    await page.keyboard.press("l");
    await page.evaluate(() => {
      window.__played = [];
      window.__playWatch = true;
      const tick = () => {
        if (!window.__playWatch) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        window.__played.push([performance.now(), at, window.__pictured()]);
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
    // Round the end and three seconds into the next time through, read
    // each time the video preview says another playhead, or false once
    // the play has stopped on the way.
    const wrapped = await page.evaluate(
      ([start, end]) =>
        new Promise((done) => {
          let near = false;
          const look = () => {
            const at = Number(document.querySelector(".screen")?.dataset.playhead);
            near ||= at > end - 1;
            if (near && at > start + 3 && at < end - 2) return true;
            if (document.querySelector('button[aria-label="Play"]')) return false;
            return null;
          };
          const now = look();
          if (now !== null) return done(now);
          const o = new MutationObserver(() => {
            const now = look();
            if (now === null) return;
            o.disconnect();
            done(now);
          });
          o.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ["data-playhead", "aria-label"] });
        }),
      [start, end],
    );
    const played = await page.evaluate(() => {
      window.__playWatch = false;
      return window.__played;
    });
    await page.keyboard.press("Space");
    await page.keyboard.press("l");
    await settle(page);
    if (!wrapped) {
      walk.wrong = `the playhead did not go round the clip's end, it got to ${Math.max(...played.map(([, at]) => at)).toFixed(3)}`;
      return;
    }
    // From half a second after the playhead came round, the picture
    // shows the frame that holds the playhead, give or take two.
    const i = played.findIndex(([, at]) => at > end - 1);
    const round = i + played.slice(i).findIndex(([, at]) => at < end - 2);
    const after = played.slice(round).filter(([t]) => t > played[round][0] + 500);
    const off = after.find(([, at, n]) => !(n >= 0) || Math.abs(n / fps - at) > 2.5 / fps);
    if (off) {
      walk.wrong = `${(off[0] - played[round][0]).toFixed(0)} ms after the playhead came round to ${played[round][1].toFixed(3)}, it stood at ${off[1].toFixed(3)} and the video preview showed ${off[2] >= 0 ? (off[2] / fps).toFixed(3) : "nothing it could read"}`;
    }
  },
  // ["loop off as the sound crosses the end"]
  //                    puts the playhead near the clip's end, plays it with
  //                    the loop on, holds the page 250 ms as the playhead
  //                    comes to the end and presses L in that moment, and
  //                    asks that the sound had crossed the end, the loop is
  //                    off and the play ends ahead of the sound card
  async "loop off as the sound crosses the end"(walk) {
    const { page, watch } = walk;
    // The playhead moves on with the page's frames, and the sound card
    // plays on its own clock. A busy machine holds the page for a
    // moment as the sound crosses the clip's end, and L comes in that
    // moment, before the playhead has caught up. The loop off ends the
    // play at the end of the time through the sound is in, never at an
    // end the sound card has already played past.
    const end = (await engineState(page, watch.at)).segments.at(-1).end;
    const g = await handles(page);
    await clickOnTrack(page, middle(g.end) - 80, high(g.track));
    await settle(page);
    if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
      await page.keyboard.press("Space");
    }
    await settle(page);
    await playUntil(page, { most: 0.3 });
    await page.keyboard.press("l");
    // The playhead moves once a frame of the video (#446), so it is on
    // the clip's last frame up to a frame before the end. The page is
    // held from there until the sound card has played past the end,
    // which is what the step is about, so no wait is long or short.
    const frame = 1 / (await rate(page));
    await playUntil(page, { past: end - frame - 0.03 });
    const r = await page.evaluate(async () => {
      const q = window.__frameQueue;
      const sounded = () => q.m0 / q.sampleRate + (q.audio.currentTime - q.c0);
      while (q.c0 !== null && sounded() < q.program.length) {}
      const crossed = q.c0 !== null && sounded() >= q.program.length;
      // The loop button says the loop is off in the same pass that
      // hands the queue its program, so once it has changed, the
      // queue has the program without the loop.
      const button = document.querySelector('button[aria-label="Loop the clip"]');
      const switched = new Promise((done) => {
        const seen = new MutationObserver(() => {
          if (button.getAttribute("aria-pressed") !== "false") return;
          seen.disconnect();
          done();
        });
        seen.observe(button, { attributes: true, attributeFilter: ["aria-pressed"] });
      });
      document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "l", bubbles: true }));
      // An L the app did not take changes nothing, and the app then
      // settles with the loop still on, which is said below.
      await Promise.race([switched, window.__settled(false)]);
      return { crossed, loop: q.program.loop, ends: q.program.end, sounded: sounded() };
    });
    await page.keyboard.press("Space");
    await settle(page);
    if (!r.crossed) walk.wrong = "the sound had not crossed the clip's end when L came";
    else if (r.loop) walk.wrong = "L did not switch the loop off";
    else if (!(r.ends > r.sounded)) {
      walk.wrong = `the play was ended ${((r.sounded - r.ends) * 1000).toFixed(0)} ms behind the sound, which had played on into the clip's start`;
    }
  },
  // ["press while playing", px]
  //                    plays the clip if it does not play, presses the
  //                    clip timeline halfway along the clip and holds the
  //                    button down still, then moves the hand by px and
  //                    holds it still again, then lets go, reading the
  //                    playhead, the frame on screen and the play button on
  //                    every animation frame
  async "press while playing"(walk, arg) {
    const { page } = walk;
    await preview(page, 0.2);
    if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
      await page.keyboard.press("Space");
    }
    // The play's waits in the episode's time, read off the playhead,
    // and the hand's in the page's frames, since a press may stop the
    // playhead.
    await settle(page);
    await playUntil(page, { most: 0.7 });
    await page.evaluate(() => {
      window.__played = [];
      window.__hand = {};
      window.__playWatch = true;
      const tick = () => {
        if (!window.__playWatch) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        const playing = !!document.querySelector('button[aria-label="Pause"]');
        window.__played.push([performance.now(), at, window.__pictured ? window.__pictured() : -1, playing]);
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    });
    await playUntil(page, { most: 0.3 });
    const g = await handles(page);
    const x = (g.start.x + g.start.w + g.end.x) / 2;
    const y = high(g.track);
    const hand = (what) => page.evaluate((w) => (window.__hand[w] = performance.now()), what);
    await page.mouse.move(x, y);
    await hand("down");
    await page.mouse.down();
    // The hand holds still for 42 of the page's frames, about 0.7 s,
    // here and after it moves. That is the gesture itself, a press
    // held the way Tim held it, not a wait for the app: "nailed" asks
    // that the playhead stood under the hand for all of it.
    await nextPaint(page, 42);
    await hand("moves");
    for (let i = 1; i <= 10; i++) {
      await page.mouse.move(x + (arg * i) / 10, y);
      await nextPaint(page);
    }
    await hand("still");
    await nextPaint(page, 42);
    await hand("up");
    await page.mouse.up();
    // After the hand lets go, the play goes on for a second of the
    // episode's own time, which "nailed" reads from 300 ms on. A play
    // that stopped ends this at once, and "nailed" says so.
    await playUntil(page, { most: 1 });
    await page.evaluate(() => (window.__playWatch = false));
    await settle(page);
  },
  // ["play from", where, seconds]
  //                    clicks the clip timeline "before" the clip, halfway
  //                    between the track's left end and the clip's start,
  //                    which puts the playhead on the video, or "in" it, a
  //                    fifth of the way along, which puts it on the clip,
  //                    presses the space bar, reads what the video preview
  //                    lays over the picture on every animation frame for
  //                    so many seconds of play, and pauses
  // ["play from", where, seconds, key]
  //                    the same, pressing this key every 400 ms of the
  //                    play, "Shift+ArrowRight" to skip ahead by words,
  //                    and from "before" the playhead has to have gone a
  //                    second or more further than the play takes it
  async "play from"(walk, arg, s) {
    const { page, watch } = walk;
    const [where, seconds, key] = arg;
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    const x = where === "before" ? (g.track.x + g.start.x) / 2 : from + 0.2 * (g.end.x - from);
    if (where === "before" && g.start.x - g.track.x < 40) {
      walk.wrong = "there is no room on the clip timeline before the clip";
      return;
    }
    await clickOnTrack(page, x, high(g.track));
    await settle(page);
    const state = await engineState(page, watch.at);
    const stop = await recordFrames(page, 1 / (await rate(page)));
    await page.evaluate(() => document.body.focus());
    const began = (await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead))) || 0;
    await page.keyboard.press("Space");
    await settle(page);
    // With a key, it is pressed every 0.4 s of the play, counted from
    // where the skip before landed, the way a hand skips ahead while
    // it watches.
    if (key) {
      for (let t = 0; t + 0.4 <= seconds; t += 0.4) {
        const now = await playUntil(page, { most: 0.4 });
        if (!now.playing) break;
        await page.keyboard.press(key);
        await settle(page);
      }
    } else {
      // So many seconds of the play, read off the playhead, not off the
      // wall: a busy machine takes a moment longer to start the play,
      // and a play timed by the wall then stood inside the clip for
      // 1.83 s where the rules ask 2. The play may end sooner, at the
      // clip's end.
      const r = await playUntil(page, { past: began + seconds - 1e-6 });
      if (r.gone === 0) walk.wrong = `the play from ${began.toFixed(2)} s neither went on ${seconds} s nor ended`;
    }
    await page.evaluate(() => {
      if (document.querySelector('button[aria-label="Play"]')) return;
      const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
      document.body.dispatchEvent(new KeyboardEvent("keydown", key));
      document.body.dispatchEvent(new KeyboardEvent("keyup", key));
    });
    walk.played = { overlays: (await stop()).overlays, state, fps: await rate(page) };
    // The steps have to have skipped: on the video the playhead went
    // further than the play alone takes it.
    const ended = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
    if (key === "Shift+ArrowRight" && where === "before" && ended - began < seconds + 1) {
      walk.wrong = `${key} while playing moved the playhead ${(ended - began).toFixed(2)} s in ${seconds} s of play, no skip ahead`;
    }
    await watch.step("play", s);
  },
  // ["look", label]    remembers, for every piece of the clip, the crop
  //                    frame in the video preview with the playhead in the
  //                    middle of the piece, put there with a click on the
  //                    clip timeline: where it stands and what it shows.
  //                    A piece the camera switches in is looked at in the
  //                    middle of each camera
  async look(walk, arg) {
    const { looks, page, watch } = walk;
    const fps = await rate(page);
    const pieces = (await engineState(page, watch.at)).segments;
    looks[arg] = [];
    looks[arg].pieces = pieces.length;
    for (const p of pieces) {
      const parts =
        walk.cameraSwitch !== null && walk.cameraSwitch > p.start + 0.5 && walk.cameraSwitch < p.end - 0.5
          ? [
              { start: p.start, end: walk.cameraSwitch },
              { start: walk.cameraSwitch, end: p.end },
            ]
          : [{ start: p.start, end: p.end }];
      for (const part of parts) {
        const at = (part.start + part.end) / 2;
        if (!(await seekTo(page, at, fps))) {
          walk.wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
          break;
        }
        looks[arg].push({ ...part, at, frame: await cropFrame(page) });
      }
      if (walk.wrong) break;
    }
  },
  // ["one crop", label]
  //                    at every moment looked at, the crop frame stood in
  //                    the same place
  async "one crop"(walk, arg) {
    const { looks } = walk;
    const then = looks[arg];
    const first = then[0]?.frame;
    const other = then.find((l) => !l.frame || !first || Math.abs(l.frame.left - first.left) > 0.005);
    if (!first) walk.wrong = `at "${arg}" there was no crop frame to look at`;
    else if (other) {
      walk.wrong = `at ${other.at.toFixed(2)} the crop frame stood ${other.frame ? (100 * other.frame.left).toFixed(1) + "% in" : "nowhere"}, and at ${then[0].at.toFixed(2)} ${(100 * first.left).toFixed(1)}%, at "${arg}"`;
    }
  },
  // ["framed", label]  the clip timeline draws as many pieces as at the
  //                    look, and at each moment looked at the crop frame
  //                    stands where it stood and shows what it showed
  async framed(walk, arg) {
    const { looks, page } = walk;
    // What a person sees first: the crop frame on every shot, where
    // it stood and with the same picture inside it. Then the pieces.
    const then = looks[arg];
    const fps = await rate(page);
    for (const was of then) {
      if (!(await seekTo(page, was.at, fps))) {
        walk.wrong = `the video preview never showed the frame at ${was.at.toFixed(2)}`;
        break;
      }
      const now = await cropFrame(page);
      if (!now) {
        walk.wrong = `at ${was.at.toFixed(2)} the video preview has no crop frame`;
      } else if (Math.abs(now.left - was.frame.left) > 0.005) {
        walk.wrong = `at ${was.at.toFixed(2)} the crop frame stands ${(100 * now.left).toFixed(1)}% in, not ${(100 * was.frame.left).toFixed(1)}% as at "${arg}", and shows ${sayLook(now.look)}, not ${sayLook(was.frame.look)}`;
      } else if (!sameLook(now.look, was.frame.look)) {
        walk.wrong = `at ${was.at.toFixed(2)} the crop frame shows ${sayLook(now.look)}, not ${sayLook(was.frame.look)} as at "${arg}"`;
      }
      if (walk.wrong) break;
    }
    const n = await page.locator(".clip-timeline .piece").count();
    if (!walk.wrong && n !== then.pieces) {
      walk.wrong = `the clip timeline draws ${pieceCount(n)}, not ${then.pieces} as at "${arg}"`;
    }
  },
  // ["shots", label]   the short, read back from disk: every frame shows
  //                    what the crop frame showed on its camera at the
  //                    look, and where two pieces meet the sound runs
  //                    straight on, no 10 ms of it quieter than 80% of the
  //                    tone
  async shots(walk, arg) {
    const { looks, page, watch } = walk;
    const pieces = (await engineState(page, watch.at)).segments;
    const short = readShort(await shortOf(page, watch.at));
    // Each frame by the moment of the episode in its middle, through
    // the pieces, and what the crop frame showed there at the look.
    let into = 0;
    const placed = pieces.map((p) => {
      const q = { ...p, from: into };
      into += p.end - p.start;
      return q;
    });
    let compared = 0;
    for (let i = 0; i < short.frames.length && !walk.wrong; i++) {
      const t = (i + 0.5) / short.fps;
      const p = placed.find((q) => t < q.from + q.end - q.start) ?? placed[placed.length - 1];
      const at = p.start + t - p.from;
      const was = looks[arg].find((l) => at >= l.start && at < l.end);
      if (was) compared++;
      if (was && !sameLook(short.frames[i], was.frame.look)) {
        walk.wrong = `frame ${i} of the short, ${at.toFixed(2)} in the episode, shows ${sayLook(short.frames[i])}, not ${sayLook(was.frame.look)} as the crop frame did at "${arg}"`;
      }
    }
    // A short with no frame to compare proves nothing.
    if (!walk.wrong && !compared) walk.wrong = `no frame of the short was compared, of ${short.frames.length}`;
    if (process.env.VERBOSE) {
      console.log(`      ${compared} frames of ${short.frames.length} compared, pieces ${placed.map((p) => `${p.start.toFixed(2)}-${p.end.toFixed(2)}`).join(" ")}`);
    }
    // Heard: where one piece runs straight into the next, no 10 ms
    // within a tenth of a second either side is quieter than 80% of
    // the tone half a second before.
    for (const p of placed.slice(1)) {
      const before = placed[placed.indexOf(p) - 1];
      if (walk.wrong || p.start - before.end > 0.0005) continue;
      const steady = loudness(short, p.from - 0.5);
      let least = { l: Infinity, t: 0 };
      for (let t = p.from - 0.1; t < p.from + 0.1; t += 0.0025) {
        const l = loudness(short, t);
        if (l < least.l) least = { l, t };
      }
      if (least.l < 0.8 * steady) {
        walk.wrong = `at ${least.t.toFixed(4)} s into the short, by ${p.start.toFixed(2)} in the episode where two pieces meet, the sound is down to ${((100 * least.l) / steady).toFixed(0)}% of the tone`;
      }
    }
  },
  // ["episode watched"]
  //                    the same, reading back from the video preview every
  //                    frame it draws for the episode from the click on
  async "episode watched"(walk) {
    const { page } = walk;
    // Every frame the new episode's queue draws, read back from the
    // canvas on every animation frame until well after it settled. The
    // canvas is the one the episode before drew on, so only frames
    // drawn once the new episode's frames are open count.
    await preview(page, 0.04);
    await page.evaluate(() => {
      const was = window.__appFrames;
      window.__drawn = [];
      window.__watching = true;
      // Every call to the Go side, with where the playhead stood as it
      // was made.
      window.__asked = [];
      if (!window.__askWatched) {
        window.__askWatched = true;
        const real = window.fetch;
        window.fetch = (input, init) => {
          if (window.__watching && String(input).endsWith("/call") && init?.body) {
            const playhead = Number(document.querySelector(".screen")?.dataset.playhead);
            window.__asked.push([String(JSON.parse(init.body).name).split(".").pop(), playhead]);
          }
          return real(input, init);
        };
      }
      const watch = () => {
        if (!window.__watching) return;
        if (window.__appFrames && window.__appFrames !== was) {
          const n = window.__pictured();
          if (n >= 0 && n !== window.__drawn.at(-1)) window.__drawn.push(n);
        }
        requestAnimationFrame(watch);
      };
      requestAnimationFrame(watch);
    });
    await fromSidebar(page, () => page.locator("aside li", { hasText: walk.episodeBefore.split("/").pop() }).first().click());
    await page.locator(".listhead button.search").first().waitFor();
    await settle(page);
    // Until the picture stands on the clip's frame, and the frame drawn
    // then: a frame of the episode's start comes on the way there, not
    // after it.
    await page.evaluate(() => window.__standing());
    await nextPaint(page, 2);
    await page.evaluate(() => (window.__watching = false));
  },
  // ["screen light"]   draws pixels of standard video, of PQ and of HLG with
  //                    the video preview's GPU drawing, and holds what the
  //                    GPU gives the canvas to light.ts worked out on the
  //                    processor, standard video to QuickTime's look with
  //                    every 10-bit step a step of its own, and HDR to
  //                    BT.2100's own anchors: HDR's
  //                    reference white is the app's white, its highlights
  //                    are brighter, and a green of BT.2020 lies outside
  //                    BT.709. A word lifted out and grown by 1.15 and 2
  //                    is held to laidPart's on every pixel, so the GPU
  //                    grows it round its middle. Says so and checks
  //                    nothing without WebGPU
  async "screen light"(walk) {
    const { page } = walk;
    // A pixel of 10 bits a colour, red lowest, as the Go side sends
    // every frame.
    const word = (r, g, b) => {
      const w = (r | (g << 10) | (b << 20)) >>> 0;
      return [w & 255, (w >>> 8) & 255, (w >>> 16) & 255, w >>> 24];
    };
    // PQ's signals for 203 and 1000 nits, and HLG's 75 percent.
    const pqWhite = Math.round(0.5806 * 1023);
    const pq1000 = Math.round(0.7518 * 1023);
    const hlgWhite = Math.round(0.75 * 1023);
    const pixels = [
      { light: "", bytes: word(514, 257, 803), name: "a colour of standard video" },
      // A grey of 22 in 255 is shown as 25, QuickTime's look.
      { light: "", bytes: word(88, 88, 88), name: "a dark grey of standard video", shown: 25 / 255 },
      // The next value in 10 bits: in floats it is a step of its own,
      // where 8 bits made a step of 3 in the dark and banded.
      { light: "", bytes: word(89, 89, 89), name: "the next dark grey", above: 1 },
      { light: "pq", bytes: word(pqWhite, pqWhite, pqWhite), name: "PQ's reference white", white: true },
      { light: "pq", bytes: word(pq1000, pq1000, pq1000), name: "PQ at 1000 nits", bright: true },
      { light: "hlg", bytes: word(hlgWhite, hlgWhite, hlgWhite), name: "HLG's reference white", white: true },
      { light: "hlg", bytes: word(1023, 1023, 1023), name: "HLG's peak", bright: true },
      { light: "pq", bytes: word(0, pqWhite, 0), name: "a green of BT.2020", outside: true },
      // Captions laid over the frame before its look, the way the
      // render burns them in, see lib/frames/captions.ts: the purple
      // of a highlight over grey, and a box of black at half that lets
      // half of the grey through.
      { light: "", bytes: word(512, 512, 512), name: "a middle grey" },
      { light: "", bytes: word(512, 512, 512), over: [148, 33, 146, 255], name: "a highlight over standard video" },
      { light: "", bytes: word(512, 512, 512), over: [0, 0, 0, 128], name: "a box at half over standard video", darker: 8 },
      { light: "pq", bytes: word(pqWhite, pqWhite, pqWhite), over: [255, 255, 255, 255], name: "a white caption over PQ" },
      // A word lifted out of its caption after a word step, laid over
      // the rest grown to its peak.
      { light: "", bytes: word(512, 512, 512), over: [0, 0, 0, 128], lift: [148, 33, 146, 255], name: "a word bouncing over a box at half" },
    ];
    const got = await page.evaluate((p) => window.__checkPixels?.(p) ?? null, pixels.map(({ light, bytes, over, lift }) => ({ light, bytes, over, lift })));
    if (!got) {
      console.log("        no WebGPU in this browser, so what the GPU draws is not checked");
      return;
    }
    const problems = [];
    pixels.forEach((p, i) => {
      const gpu = got.gpu[i];
      const cpu = got.cpu[i];
      // Half floats hold 3 figures, and 10 bits sampled on the GPU may
      // round a step apart.
      const off = Math.max(...gpu.map((v, c) => Math.abs(v - cpu[c]) / Math.max(1, Math.abs(cpu[c]))));
      if (!(off < 0.01)) problems.push(`${p.name}: the GPU gives ${gpu.map((v) => v.toFixed(4))}, light.ts ${cpu.map((v) => v.toFixed(4))}`);
      if (p.white && gpu.some((v) => Math.abs(v - 1) > 0.01)) problems.push(`${p.name} is ${gpu.map((v) => v.toFixed(4))}, not the app's white`);
      if (p.bright && gpu.some((v) => v < 1.5)) problems.push(`${p.name} is ${gpu.map((v) => v.toFixed(4))}, not brighter than white`);
      if (p.outside && !(gpu[0] < 0)) problems.push(`${p.name} has a red of ${gpu[0].toFixed(4)}, inside BT.709`);
      if (p.shown !== undefined && gpu.some((v) => Math.abs(v - p.shown) > 0.5 / 255)) problems.push(`${p.name} is ${gpu.map((v) => (v * 255).toFixed(2))} in 255, not ${(p.shown * 255).toFixed(0)}`);
      if (p.darker !== undefined && !(gpu[0] < got.gpu[p.darker][0] * 0.9)) problems.push(`${p.name} is ${gpu.map((v) => v.toFixed(4))}, no darker than the grey under it`);
      if (p.above !== undefined) {
        const step = (gpu[0] - got.gpu[p.above][0]) * 255;
        if (!(step > 0 && step < 1)) problems.push(`${p.name} is ${step.toFixed(2)} above the one before in 255, not a step of its own below 1`);
      }
    });
    // A word lifted out and grown, every pixel of it, on the GPU and
    // by laidPart alike, so the shader grows it round its middle as
    // the 2D canvas does. Both sample the word between its four
    // nearest pixels, the GPU's filter in steps of its own.
    const LIFT_OFF = 0.01;
    const lifted = await page.evaluate(() => window.__checkLift?.([1.15, 2]) ?? null);
    let most = 0;
    lifted?.gpu.forEach((pixels, k) =>
      pixels.forEach((gpu, i) => {
        const off = Math.max(...gpu.map((v, c) => Math.abs(v - lifted.cpu[k][i][c])));
        most = Math.max(most, off);
        if (!(off < LIFT_OFF))
          problems.push(`the word grown by ${[1.15, 2][k]} at ${i % 8},${i >> 3}: the GPU gives ${gpu.map((v) => v.toFixed(4))}, laidPart ${lifted.cpu[k][i].map((v) => v.toFixed(4))}`);
      }),
    );
    if (lifted) console.log(`        a word grown on the GPU is at most ${most.toFixed(4)} from laidPart's, over every pixel`);
    // The canvas of standard video is in the standard mode, the one
    // the rest of the app is drawn in. In the extended mode it turned
    // the Mac's HDR on, and Tim's pictures on #201 showed the captions
    // on the canvas darker than the box a word is typed in.
    const mode = await page.evaluate(
      () => document.querySelector(".screen canvas")?.getContext("webgpu")?.getConfiguration?.()?.toneMapping?.mode ?? null,
    );
    console.log(`        the canvas of the bridge's standard video is in the ${mode} mode`);
    if (mode && mode !== "standard") problems.push(`the canvas of standard video is in the ${mode} mode`);
    if (problems.length) walk.wrong = problems.join("\n");
  },
  // ["past the end"]
  //                    drags the clip's end far to the right three times,
  //                    each from where the end is drawn then, and checks
  //                    after each that the clip ends where the video does
  //                    at the latest, in the engine's plan
  async "past the end"(walk, arg, s) {
    const { page, watch } = walk;
    const { duration, fps } = await ask(page, "Source", await episodeOn(page));
    // The end of the video is the frame after its last, which a length
    // that falls inside a frame puts up to a frame later.
    const last = duration + 1 / fps + 0.001;
    for (let i = 1; i <= 3 && !walk.wrong; i++) {
      const g = await handles(page);
      await drag(page, middle(g.end), high(g.track), 4000, false);
      await watch.step("trim", s);
      const { segments } = await engineState(page, watch.at);
      const end = segments[segments.length - 1].end;
      if (end > last) {
        walk.wrong = `dragged past the end ${i} times, the clip ends at ${end.toFixed(2)}, after the video, which ends at ${duration.toFixed(2)}`;
      }
    }
  },
  // ["play to the end"]
  //                    drags the clip's end to the end of the episode,
  //                    puts the playhead three seconds before it with a
  //                    click on the clip timeline and plays until the play
  //                    stops there, so the episode's decoder reads to the
  //                    end of the file
  async "play to the end"(walk, arg, s) {
    const { page, watch } = walk;
    // Far to the right first, which the clip timeline takes as far as
    // it shows, and then onto the end of the episode where the clip
    // timeline shows it now.
    const { duration, fps } = await ask(page, "Source", await episodeOn(page));
    let g = await handles(page);
    await drag(page, middle(g.end), high(g.track), 4000, false);
    await watch.step("trim", s);
    g = await handles(page);
    await drag(page, middle(g.end), high(g.track), (await xOf(page, duration)) - middle(g.end), false);
    await watch.step("trim", s);
    const { segments } = await engineState(page, watch.at);
    const end = segments[segments.length - 1].end;
    if (Math.abs(end - duration) > 0.05) {
      walk.wrong = `the clip ends at ${end.toFixed(2)}, not at the end of the episode, ${duration.toFixed(2)}`;
      return;
    }
    // Three seconds before the end, so the click lands on the clip
    // timeline clear of the end's handle.
    if (!(await seekTo(page, end - 3, fps))) {
      walk.wrong = `the video preview never showed the frame at ${(end - 3).toFixed(2)}`;
      return;
    }
    await page.keyboard.press("Space");
    // Until the play stops, or has gone on three seconds past the end,
    // in the episode's time, see playUntil.
    await settle(page);
    const now = await playUntil(page, { most: 6 });
    const stopped = !now.playing && now.at >= end - 0.1;
    if (!stopped) {
      walk.wrong = `the play did not stop at the end of the episode, ${end.toFixed(2)}`;
      return;
    }
    // The playhead stays at the end, drawn on the clip timeline and on
    // the range picker, where the hand can take hold of it again. A
    // playhead a hair past the end was drawn on neither, plan row 2.172.
    const lost = await page.evaluate((duration) => {
      const at = Number(document.querySelector(".screen").dataset.playhead);
      if (at > duration) return `the playhead stopped at ${at.toFixed(3)}, past the end of the episode, ${duration.toFixed(3)}`;
      const on = (line, track) => {
        if (!line || !track) return false;
        const l = line.getBoundingClientRect();
        const t = track.getBoundingClientRect();
        const x = l.left + l.width / 2;
        return l.width > 0 && l.height > 0 && x >= t.left - 1 && x <= t.right + 1;
      };
      const timeline = document.querySelector(".clip-timeline");
      if (!on(timeline?.querySelector(".at .playhead"), timeline?.querySelector(".track")))
        return "the clip timeline shows no playhead at the end of the episode";
      const picker = [...document.querySelectorAll(".playhead")].find((e) => !e.closest(".clip-timeline"));
      const track = [...document.querySelectorAll(".track")].find((e) => !e.closest(".clip-timeline"));
      if (!on(picker, track)) return "the range picker shows no playhead at the end of the episode";
      return "";
    }, duration);
    if (lost) walk.wrong = lost;
  },
  // ["add hevc"]       adds a video of 40 seconds whose picture is HEVC with
  //                    10-bit colour in open GOPs, as x265 writes it, and
  //                    chooses the clip its first search finds
  async "add hevc"(walk, arg, s) {
    const { page, watch } = walk;
    await addHEVC(url, page);
    await watch.step("wait", s);
  },
  // ["leading frames"] puts the playhead with a click on the clip timeline
  //                    on three frames of the clip that are shown before
  //                    the key frame they are decoded after, read from the
  //                    file by ffprobe, waits for each in the video
  //                    preview, and plays from the last
  async "leading frames"(walk) {
    const { page, watch } = walk;
    const path = await episodeOn(page);
    const fps = await rate(page);
    // In the order the frames are decoded: a frame shown before the
    // key frame decoded last leads up to it.
    const packets = execFileSync("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", path])
      .toString()
      .trim()
      .split("\n");
    let key = -Infinity;
    const leading = [];
    for (const line of packets) {
      const [t, flags] = line.split(",");
      if (flags.startsWith("K")) key = Number(t);
      else if (Number(t) < key) leading.push(Number(t));
    }
    const { segments } = await engineState(page, watch.at);
    const first = segments[0].start;
    const last = segments[segments.length - 1].end;
    const on = leading.filter((t) => t > first + 1 && t < last - 3).slice(0, 3);
    if (on.length < 3) {
      walk.wrong = `the clip from ${first.toFixed(2)} to ${last.toFixed(2)} holds ${on.length} frames shown before their key frame, not 3`;
      return;
    }
    // In the middle of the frame, so the click lands on it.
    for (const t of on) {
      if (!(await seekTo(page, t + 0.5 / fps, fps))) {
        const at = await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
        walk.wrong = `the video preview never showed the frame at ${t.toFixed(3)}, shown before its key frame, with the playhead at ${at.toFixed(3)}`;
        break;
      }
    }
    if (walk.wrong) return;
    const from = await page.evaluate(() => ({ at: Number(document.querySelector(".screen").dataset.playhead), frame: window.__pictured() }));
    await page.evaluate(() => document.body.focus());
    await page.keyboard.press("Space");
    // Half a second of the episode, read off the playhead, and then
    // whether the picture went on with it.
    await settle(page);
    await playUntil(page, { past: from.at + 0.5 });
    await settle(page);
    const plays = await page.evaluate((from) => window.__pictured() > from.frame + 1, from);
    await page.keyboard.press("Space");
    if (!plays) walk.wrong = `the space bar did not play from the frame at ${from.at.toFixed(3)}, shown before its key frame`;
  },
  // ["no browser decoder"]
  //                    takes the browser's picture and sound decoders away,
  //                    every one, and opens the episode again
  async "no browser decoder"(walk) {
    const { page } = walk;
    await page.addInitScript(() => {
      for (const name of ["VideoDecoder", "AudioDecoder", "EncodedVideoChunk", "EncodedAudioChunk"]) {
        Object.defineProperty(window, name, {
          get() {
            throw new Error(`${name} is not to be used: the Go side decodes`);
          },
        });
      }
    });
    const name = (await episodeOn(page)).split("/").pop();
    await page.reload();
    await page.locator("aside li", { hasText: name }).first().waitFor();
    await fromSidebar(page, () => page.locator("aside li", { hasText: name }).first().click());
    await page.locator("aside ol li[data-key] button.pick").first().click();
    await settle(page);
  },
  // ["on frames"]      every edge of the clip the hand put, all but its
  //                    end and any before the picture starts, lands on
  //                    one of the picture's own frames, counted from where
  //                    the picture starts, the frames the render cuts on
  async "on frames"(walk) {
    const { page, watch } = walk;
    // Where the picture starts, read from the file by ffprobe rather
    // than from the app, which is what is being checked: the picture's
    // first timestamp after the file's own start.
    const path = await episodeOn(page);
    const { fps } = await ask(page, "Source", path);
    const read = (args) => Number(execFileSync("ffprobe", ["-v", "error", ...args, "-of", "csv=p=0", path]).toString().trim());
    const videoStart = read(["-select_streams", "v:0", "-show_entries", "stream=start_time"]) - read(["-show_entries", "format=start_time"]);
    const { segments } = await engineState(page, watch.at);
    // Before the picture starts there is no frame to land on: a start
    // dragged there takes the picture from its first frame.
    const edges = segments
      .flatMap((p) => [p.start, p.end])
      .slice(0, -1)
      .filter((t) => t >= videoStart - 1e-6);
    // Edges are kept to the millisecond, a thirtieth of a frame here.
    const off = edges
      .map((t) => ({ t, by: Math.abs((t - videoStart) * fps - Math.round((t - videoStart) * fps)) }))
      .filter((e) => e.by > 0.001 * fps + 1e-6);
    if (off.length) {
      walk.wrong = `the picture starts at ${videoStart.toFixed(4)} s, and ${off
        .map((e) => `${e.t.toFixed(3)} is ${e.by.toFixed(2)} of a frame from its frames`)
        .join(", ")}`;
    }
  },
  // ["sharp"]          the frame on the video preview, as the workspace
  //                    opened and before anything played, was decoded at
  //                    the size of the canvas, no larger than the file's
  //                    own picture
  // ["sharp", "opened"] the same, and the first frame the workspace asked
  //                    for since it opened was asked at that size
  async sharp(walk, arg) {
    const { page } = walk;
    // The playhead stands still and nothing plays, so the frame on the
    // canvas is the one asked for while paused. After the app was
    // resized that frame is asked for again at the new size, over a
    // fetch the settle does not count, so it is read once the frame
    // queue says the picture stands, see stillFrame in rules.mjs. On a
    // busy machine it was read before it came, issue #538.
    const got = await stillFrame(page);
    if (!got) {
      walk.wrong = "the video preview has no frame on its canvas";
      return;
    }
    const { frame, canvas, file, want, first } = got;
    if (frame[0] !== want[0] || frame[1] !== want[1]) {
      walk.wrong = `the frame on the video preview is ${frame.join(" by ")}, decoded for a canvas of another size: the canvas is ${canvas.join(" by ")} and the file ${file.join(" by ")}, so it should be ${want.join(" by ")}`;
    } else if (arg === "opened" && first !== want.join("x")) {
      // Nothing has resized the app since it opened, so the first
      // frame it asked for was asked at this size too, not at the size
      // of a canvas the stylesheet had not laid out yet and then again.
      walk.wrong = `the first frame was asked for at ${first}, before the canvas had its size, and only then at ${want.join("x")}`;
    }
  },
  // ["only the video", "across cuts"], ["the clip shown", "across cuts"]
  //                    the same, and the play crossed a cut: it stood in
  //                    two pieces of the clip or more, and on the video
  //                    went through the part cut out between them
  // ["only the video"] since "play from", the playhead was on the video
  //                    on every frame and played two seconds or more of
  //                    the clip's own moments, and the video preview laid
  //                    nothing over the picture, no crop frame, no shade
  //                    and no captions
  // ["the clip shown"] since "play from", the playhead was on the clip on
  //                    every frame and played two seconds or more inside
  //                    it, the crop frame stood over the picture on every
  //                    one of them, and the captions on every one where the
  //                    engine has a caption, a few at least
  async "the clip shown"(walk, arg, s, what) {
    if (!walk.played) {
      walk.wrong = `${what} with nothing played`;
      return;
    }
    const { overlays, state, fps } = walk.played;
    const video = what === "only the video";
    const frame = 1 / fps;
    // With "across cuts" the play has to have crossed a cut: the
    // episode through the part cut out, from a piece before it to a
    // piece after it, and the clip from one piece on into the next.
    const pieceOf = (at) => state.segments.findIndex((p) => at >= p.start + frame && at < p.end - frame);
    const played_ = overlays.filter((o) => o.playing);
    const pieces = new Set(played_.map((o) => pieceOf(o.at)).filter((i) => i >= 0));
    const inCut = played_.some((o) => state.segments.some((p, i) => i > 0 && o.at > state.segments[i - 1].end + frame && o.at < p.start - frame));
    if (arg === "across cuts" && state.segments.length < 2) {
      walk.wrong = `the clip has ${state.segments.length} piece, so no cut to play across`;
      return;
    }
    if (arg === "across cuts" && pieces.size < 2) {
      walk.wrong = `the play stood in ${pieces.size} piece of the clip, so it crossed no cut`;
      return;
    }
    if (arg === "across cuts" && video && !inCut) {
      walk.wrong = "the episode played through no part cut out of the clip";
      return;
    }
    // The frames of the play that stood in one of the clip's pieces,
    // which is where a clip's frame and captions could show.
    const inside = overlays.filter((o) => o.playing && state.segments.some((p) => o.at >= p.start + frame && o.at < p.end - frame));
    const span = inside.length ? inside[inside.length - 1].at - inside[0].at : 0;
    const elsewhere = overlays.filter((o) => o.playing && o.video !== video);
    if (elsewhere.length) {
      walk.wrong = `the playhead was on the ${video ? "clip" : "video"} at ${elsewhere[0].at.toFixed(3)} while it played`;
    } else if (span < 2) {
      walk.wrong = `the play stood inside the clip for ${span.toFixed(2)} s of the episode, in ${inside.length} animation frames`;
    } else if (!video && !inside.some((o) => o.captions)) {
      walk.wrong = "the clip played with no caption shown";
    } else {
      walk.wrong = overlaid(overlays, state, frame);
    }
  },
  // ["opened on the clip"]
  //                    since "episode watched", the video preview drew the
  //                    first frame of the chosen clip and no other, and the
  //                    playhead was on the clip before the workspace asked
  //                    for the window
  async "opened on the clip"(walk) {
    const { page } = walk;
    // The video preview of an episode that opens on a clip draws the
    // clip's first frame and nothing before it: no frame of the
    // episode's start on the way to the clip.
    const on = await chosen(page);
    const clip = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const fps = await rate(page);
    const first = Math.floor((clip.start + 0.001) * fps);
    const drawn = await page.evaluate(() => window.__drawn ?? []);
    if (!drawn.length) walk.wrong = "the video preview drew no frame of the episode it opened";
    else if (drawn.some((n) => n !== first)) {
      walk.wrong = `the episode opened on its clip at frame ${first} and the video preview drew frames ${drawn.join(", ")} on the way`;
    }
    // Where the playhead opens comes before anything else the
    // workspace reads as it opens: the window, which it reads last and
    // only as it opens, is asked with the playhead on the clip. The
    // coverage and the room are read again whenever work elsewhere
    // ends, a search of the episode added before, so they say nothing
    // about the order the workspace opens in. A workspace kept from
    // before, see App.svelte, is woken rather than built and asks
    // neither the clip it opens on nor the window: it has them.
    const asked = await page.evaluate(() => window.__asked ?? []);
    const after = ["ChosenWindow"];
    const built = asked.some(([name]) => name === "ChosenClip");
    const early = asked.filter(([name, at]) => after.includes(name) && Math.abs(at - clip.start) > 0.002);
    if (!walk.wrong && built && !asked.some(([name]) => after.includes(name))) walk.wrong = `the workspace opened asking none of ${after.join(", ")}`;
    else if (!walk.wrong && early.length) {
      walk.wrong = `the workspace asked ${early.map(([n, at]) => `${n} with the playhead at ${at}`).join(", ")}, before the playhead was on its clip at ${clip.start}`;
    }
  },
  // ["played through"] since "drag while playing", the play went on as if
  //                    no hand were there: the playhead never went back nor
  //                    stood still for longer than a frame of the video and
  //                    150 ms, since it moves once a frame (#446), and no
  //                    frame drawn was older than the one before it
  async "played through"(walk) {
    const { page } = walk;
    const played = await page.evaluate(() => window.__played ?? []);
    const frame = 1000 / (await rate(page));
    let still = 0;
    let longest = 0;
    for (let i = 1; i < played.length && !walk.wrong; i++) {
      const [t, at, n] = played[i];
      const [t0, at0, n0] = played[i - 1];
      if (at < at0 - 1e-6) walk.wrong = `the playhead went back from ${at0.toFixed(3)} to ${at.toFixed(3)} while the hand was on the edge`;
      else if (n >= 0 && n0 >= 0 && n < n0) walk.wrong = `frame ${n} was drawn after frame ${n0} while the hand was on the edge`;
      if (at === at0) still += t - t0;
      else still = 0;
      longest = Math.max(longest, still);
    }
    if (!walk.wrong && played.length < 30) walk.wrong = `only ${played.length} animation frames were read`;
    if (!walk.wrong && longest > frame + 150) walk.wrong = `the playhead stood still for ${longest.toFixed(0)} ms while the hand was on the edge, a frame is ${frame.toFixed(0)} ms`;
  },
  // ["stopped at its end"]
  //                    since "loop off near the end", the presses of L
  //                    started no play over, threw no sound away and
  //                    handed none over twice, the sound did not click but
  //                    at the ends of a silence where the frame queue says
  //                    it handed the sound card its sound late, nothing
  //                    was heard after the clip's end, the play
  //                    stopped within 400 ms of the playhead reaching it,
  //                    and no frame drawn from then on is from before the
  //                    last second of the clip, so nothing of the clip's
  //                    start played again
  async "stopped at its end"(walk) {
    const { page } = walk;
    if (!walk.loopEnd) {
      walk.wrong = "stopped at its end with nothing played";
      return;
    }
    const { end, fps } = walk.loopEnd;
    const played = await page.evaluate(() => window.__played ?? []);
    // Reached once the playhead stands in the clip's last frame.
    const i = played.findIndex(([, at]) => at >= end - 1.5 / fps);
    if (i < 0) {
      walk.wrong = `the playhead never reached the clip's end at ${end.toFixed(3)}, it got to ${Math.max(...played.map(([, at]) => at)).toFixed(3)}`;
      return;
    }
    const [reached] = played[i];
    const stopped = played.slice(i).find(([, , , playing]) => !playing);
    const old = played.slice(i).find(([, , n]) => n >= 0 && n / fps < end - 1);
    // The sound, sample by sample. The bridge's episode sounds a steady
    // tone, so a click is a step in it: from one sample to the next the
    // tone bends by its second difference, never more than a few
    // thousandths of its height, and a stop or a restart jumps by a
    // good part of it. The sound card's time of the program's end is
    // where the sound ends, and after it nothing.
    const { heard, c0, m0, rate: sr, programEnd, late } = walk.loopEnd;
    const xs = [];
    for (const h of heard) for (let k = 0; k < h.data.length; k++) xs.push([h.frame + k, h.data[k]]);
    const endFrame = Math.round((c0 + (Math.round(programEnd * sr) - m0) / sr) * sr);
    let height = 0;
    for (const [f, x] of xs) if (f < endFrame) height = Math.max(height, Math.abs(x));
    // A busy machine can keep the page from handing the sound card its
    // sound in time, and the sound card plays nothing until it comes,
    // which starts and ends with a click. The app counts each such
    // stretch with where it should have started and where it did, see
    // schedule in lib/frames/queue.ts, so a click at either end of a
    // silence heard there is the machine's and not the loop's, issue
    // #466. Where the sound card took up the sound again is read off
    // the silence itself, since it can be a while after the moment
    // the page asked. The page's counts are within a quantum of 128
    // frames of the sound card's. The presses of L are known to have
    // thrown no sound away and handed none over twice, below, so
    // nothing they did made the sound late.
    const silences = [];
    for (let k = 0; k < xs.length; k++) {
      if (xs[k][1] !== 0) continue;
      const from = k;
      while (k + 1 < xs.length && xs[k + 1][1] === 0 && xs[k + 1][0] === xs[k][0] + 1) k++;
      const [a, b] = [xs[from][0], xs[k][0]];
      if (b - a >= 64 && late.some(([p, q]) => a <= q + 128 && b >= p - 128)) silences.push([a, b]);
    }
    const lateHere = (f) => silences.some(([a, b]) => f >= a - 1 && f <= b + 2);
    let clicks = 0;
    let firstClick = null;
    let after = 0;
    let starved = 0;
    for (let k = 2; k < xs.length; k++) {
      const [f, x] = xs[k];
      if (f >= endFrame + 2) {
        if (Math.abs(x) > 1e-3) after++;
        continue;
      }
      if (xs[k - 1][0] !== f - 1 || xs[k - 2][0] !== f - 2) continue;
      if (Math.abs(x - 2 * xs[k - 1][1] + xs[k - 2][1]) <= 0.03 * height) continue;
      if (lateHere(f)) starved++;
      else {
        clicks++;
        firstClick ??= ((f - endFrame) / sr) * 1000;
      }
    }
    if (starved) console.log(`      ${starved} clicks where the page handed the sound card its sound late, ${silences.length} silences heard`);
    if (walk.loopEnd.starts) walk.wrong = `the play started over ${walk.loopEnd.starts} times while L was pressed`;
    else if (walk.loopEnd.thrown) walk.wrong = `the presses of L took ${walk.loopEnd.thrown} stretches of sound off the sound card before they had played`;
    else if (walk.loopEnd.twice) walk.wrong = `the presses of L handed the sound card ${walk.loopEnd.twice} stretches of sound it had already been handed`;
    else if (!(height > 0.01)) walk.wrong = `the sound card was handed nothing to hear, ${xs.length} samples`;
    else if (clicks) walk.wrong = `the sound clicked ${clicks} times, the first ${(-firstClick).toFixed(0)} ms before the end`;
    else if (after) walk.wrong = `${after} samples of sound were heard after the clip's end`;
    else if (!stopped) walk.wrong = `the play went on for ${(played.at(-1)[0] - reached).toFixed(0)} ms with the playhead at the clip's end`;
    else if (stopped[0] - reached > 400) walk.wrong = `the play stopped ${(stopped[0] - reached).toFixed(0)} ms after the playhead reached the clip's end`;
    else if (old) walk.wrong = `frame ${(old[2] / fps).toFixed(3)} was drawn ${(old[0] - reached).toFixed(0)} ms after the playhead reached the clip's end at ${end.toFixed(3)}`;
  },
  // ["heard some"]     waits until the engine has a sentence of the video
  //                    on screen, or n with ["heard some", n], asking again each time the work says how
  //                    far it heard. With the speech held it lets a piece
  //                    of audio through at a time, and stops between two
  async "heard some"(walk, arg) {
    const { page, settled } = walk;
    // Asked again each time the work says how far it heard, until the
    // saved transcript holds a sentence, or as many as asked for.
    const path = await episodeOn(page);
    const least = arg ?? 1;
    // With the speech held, it is let through a piece at a time, and
    // each is saved.
    for (let seen = await news(page); ((await ask(page, "Sentences", path)) ?? []).length < least; ) {
      if (!walk.speechHeld) seen = await nextNews(page, seen);
      else if (!(await onePiece(page, url, path, { save: true }))) {
        walk.wrong = `the engine has ${((await ask(page, "Sentences", path)) ?? []).length} sentences of the ${least} wanted, and no work on the video is left to hear more`;
        return;
      }
    }
    // And the app has read how far the saved transcript reaches since,
    // which it reads again on its own while the work hears, so the
    // cards it reads are those sentences too.
    await nextCall(page, "Episode", await page.evaluate(() => window.__calls.length));
    await settle(page);
    await settled();
  },
  // ["as previewed", rate]
  //                    the short Render wrote, read back from disk, has
  //                    this frame rate and as many frames as the clip's
  //                    pieces last, and at every cut, at the clip's two
  //                    ends and on every tenth frame between, its frame is
  //                    the frame the video preview shows at the moment that
  //                    frame stands for, read off the canvas with the
  //                    playhead put there by a click on the clip timeline
  // ["as previewed", rate, before]
  //                    the same, for an episode whose picture starts after
  //                    its sound: the clip's first piece starts before
  //                    this moment, inside the time before the picture
  async "as previewed"(walk, arg) {
    const { page } = walk;
    const [rate, before] = [arg].flat();
    walk.wrong = await asPreviewed(page, rate, before);
  },
  // ["crop to", side]  drags the crop frame as far as it goes to the
  //                    "left" or the "right" of the picture
  async "crop to"(walk, arg) {
    const { page } = walk;
    // Taken a little below its top, away from the captions.
    const f = await page.locator(".player .frame").boundingBox();
    const w = await page.evaluate(() => document.querySelector(".screen").getBoundingClientRect().width);
    await drag(page, f.x + f.width / 2, f.y + 0.2 * f.height, arg === "left" ? -w : w);
    await settle(page);
  },
  // ["frame whole"]    over a white picture, the crop frame's line is the
  //                    app's colour all the way along its four sides, the
  //                    viewer's rounded corners cutting none of it, the
  //                    shade reaches into the frame's rounded corners with
  //                    nothing bright left there, the inside of the frame
  //                    is not shaded and the picture beside it is, and the
  //                    line stands where the crop the hand moves stands
  // ["frame whole", "turned"]
  //                    the same on a part rotated a quarter, whose picture
  //                    stands inside the viewer rather than filling it
  async "frame whole"(walk, arg) {
    const { page } = walk;
    walk.wrong = await frameWhole(page, arg === "turned");
  },
  // ["on borders"]     the chosen clip's frame on the clip timeline and the
  //                    bars of the window at rest on the range picker lie
  //                    on their track's top and bottom border, read off a
  //                    screenshot: the outermost rows of the track's box
  //                    are the app's colour, as deep as the line is thick,
  //                    and the row just outside it is not, so no grey of
  //                    the border shows beside them, away from the
  //                    playhead and the marks that stand on the lines
  async "on borders"(walk) {
    const { page } = walk;
    walk.wrong = await onBorders(page);
  },
  // ["edges as rendered"]
  //                    the short Render wrote, read back from disk, and the
  //                    video preview agree on the clip's edges: with the
  //                    playhead put on the frame of the clip's start and of
  //                    where each cut ends by a click on it, the video
  //                    preview shows the frame the short starts that piece
  //                    with, and on the clip's last frame, put there by a
  //                    click on its end, the short's last frame. The clip's
  //                    end is where the search put it, and at least one
  //                    edge is kept to the millisecond just before its
  //                    frame's start
  async "edges as rendered"(walk) {
    const { page } = walk;
    walk.wrong = await edgesAsRendered(page);
  },
  // ["drag crop", px]  drags the crop frame in the video preview sideways
  //                    by px, in ten steps, and lets go
  async "drag crop"(walk, arg, s) {
    const { page, watch } = walk;
    const box = await page.locator(".screen .crop").boundingBox();
    if (!box) {
      walk.wrong = "the video preview shows no crop frame to drag";
      return;
    }
    const y = box.y + box.height / 2;
    const x = box.x + box.width / 2;
    await page.mouse.move(x, y);
    await page.mouse.down();
    for (let i = 1; i <= 10; i++) await page.mouse.move(x + (arg * i) / 10, y);
    await page.mouse.up();
    await watch.step("drag", s);
    await settle(page);
  },
  async stamp(walk) {
    const { page } = walk;
    // The rotate icon over the rotated part, worked out from the two
    // layers the stylesheet lays over each piece of it, the icon and
    // the glow round it: how much of what is under the icon shows
    // through, the size it is drawn at and where, against the part's middle,
    // the band the keyframes stand in along the top of the clip
    // timeline and the band the thumbnails stand in along its foot.
    const got = await page.evaluate(() => {
      const track = document.querySelector(".clip-timeline .track").getBoundingClientRect();
      const band = (sel, sample) => {
        const e = document.querySelector(sel);
        if (!e) return sample;
        const r = e.getBoundingClientRect();
        return [r.top, r.bottom];
      };
      const pieces = [...document.querySelectorAll(".clip-timeline .piece.turned")].map((e) => {
        const r = e.getBoundingClientRect();
        const css = getComputedStyle(e, "::after");
        const glow = getComputedStyle(e, "::before");
        const size = parseFloat(css.maskSize);
        const left = parseFloat(css.maskPosition);
        return {
          left: r.left,
          right: r.right,
          top: r.top,
          height: r.height,
          size,
          at: r.left + left,
          image: css.maskImage,
          colour: css.backgroundColor,
          glow: glow.maskImage,
          glowColour: glow.backgroundColor,
          glowCut: glow.maskComposite,
          cut: getComputedStyle(e).maskImage,
        };
      });
      return {
        keys: band(".clip-timeline .cropdot", null),
        thumbs: band(".clip-timeline .thumb", [track.bottom - 10, track.bottom + 9]),
        height: track.height,
        pieces,
      };
    });
    const p = got.pieces[0];
    if (!p) {
      walk.wrong = "the rotated part has no rotate icon on it";
      return;
    }
    if (!/svg/.test(p.image)) {
      walk.wrong = `the rotated part's icon is drawn with ${p.image.slice(0, 40)}, not the rotate icon`;
      return;
    }
    // The accent at half strength, so the waveform shows through it,
    // which a solid dark did not, and was too strong. Its alpha is the
    // last number of the colour, rgba() or color() alike.
    const alpha = (c) => {
      const n = c.match(/[\d.]+/g)?.map(Number) ?? [];
      return /\/|rgba/.test(c) ? n[n.length - 1] : 1;
    };
    if (Math.abs(alpha(p.colour) - 0.5) > 0.02) {
      walk.wrong = `the rotate icon is ${p.colour}, not the accent at half strength`;
      return;
    }
    // A dark glow round it, the icon's lines drawn wider and softened
    // with the icon itself taken out, so it parts the icon from the
    // waveform and the empty track alike and lies only outside it.
    if (!/svg[\s\S]*svg/.test(p.glow) || !/^exclude/.test(p.glowCut)) {
      walk.wrong = `the rotate icon has no glow round it, its mask is ${p.glow.slice(0, 40)} with ${p.glowCut}`;
      return;
    }
    if (alpha(p.glowColour) < 0.5) {
      walk.wrong = `the glow round the rotate icon is ${p.glowColour}, too faint to part it from the waveform`;
      return;
    }
    // Nothing really cut out of the piece, where the track's black and
    // the waveform's grey showed through by turns.
    if (p.cut !== "none") {
      walk.wrong = `the rotated part's wash has ${p.cut.slice(0, 40)} cut out of it`;
      return;
    }
    const top = p.top + (p.height - p.size) / 2;
    const bottom = top + p.size;
    const middle = p.at + p.size / 2;
    if (p.size < got.height / 2) walk.wrong = `the rotate icon is ${p.size.toFixed(1)} px, less than half the clip timeline's ${got.height.toFixed(0)} px`;
    else if (Math.abs(middle - (p.left + p.right) / 2) > 1)
      walk.wrong = `the rotate icon's middle is at ${middle.toFixed(1)} px, not the part's, ${((p.left + p.right) / 2).toFixed(1)} px`;
    else if (got.keys && top < got.keys[1])
      walk.wrong = `the rotate icon, ${top.toFixed(1)} to ${bottom.toFixed(1)} px, reaches into the keyframes' row, to ${got.keys[1].toFixed(1)} px`;
    else if (bottom > got.thumbs[0])
      walk.wrong = `the rotate icon, ${top.toFixed(1)} to ${bottom.toFixed(1)} px, reaches into the thumbnails' row, from ${got.thumbs[0].toFixed(1)} px`;
  },
  // ["glides across the cut"]
  //                    with the playhead put on a frame between the clip's
  //                    first keyframe and its first cut, and on one between
  //                    the cut and its last keyframe, after it, the crop
  //                    frame stands where the glide puts it, on its way
  //                    from the one keyframe to the other
  async "glides across the cut"(walk) {
    const { page } = walk;
    // The first keyframe before the clip's first cut and the last after
    // it: on a frame between the first and the cut, and on one between
    // the cut and the last, the crop frame stands where the glide puts
    // it, on its way from the one to the other.
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const src = await ask(page, "Source", on.path);
    const [p, q] = entry?.segments ?? [];
    const marks = entry?.cropMarks ?? [];
    const a = marks.find((m) => p && m.at >= p.start && m.at < p.end);
    const b = marks.findLast((m) => q && m.at >= q.start && m.at < q.end);
    if (!a || !b) {
      walk.wrong = "the clip has no keyframe either side of its cut";
      return;
    }
    await filmedOnScreen(page);
    const crop = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.crop));
    const problems = [];
    const seen = [];
    for (const [piece, at] of [
      [p, a.at + 0.7 * (p.end - a.at)],
      [q, q.start + 0.3 * (b.at - q.start)],
    ]) {
      // Half a frame in, so the click lands in the frame meant.
      const t = Math.floor(at * src.fps) / src.fps + 0.5 / src.fps;
      const g = await handles(page);
      await clickOnTrack(page, await xOf(page, t), high(g.track));
      await settle(page);
      await shownOnPlayhead(page);
      const came = await page.evaluate((fps) => window.__filmed() === Math.floor((Number(document.querySelector(".screen").dataset.playhead) + 0.001) * fps), src.fps);
      if (!came) {
        problems.push(`no frame came for ${t.toFixed(3)} s`);
        continue;
      }
      const k = await filmedOnScreen(page);
      const got = await crop();
      seen.push(got);
      const want = (piece.marks ?? []).length ? glideOf(piece.marks, src)(k) : NaN;
      if (got !== want) problems.push(`on frame ${k} the crop frame stands at ${got}, the glide puts it at ${want}`);
    }
    const [ax, bx] = [a.x - (a.x % 2), b.x - (b.x % 2)];
    const between = (v) => (v - ax) * (v - bx) < 0;
    if (!problems.length && !seen.every(between)) {
      problems.push(`either side of the cut the crop frame stands at ${seen.join(" and ")}, not on its way from ${ax} to ${bx}`);
    }
    if (problems.length) walk.wrong = problems.join("; ");
  },
  // ["glides"]         with the playhead put by a click on the clip
  //                    timeline on the first keyframe, a quarter, half
  //                    and three quarters of the way to the next and on
  //                    it, the crop frame in the video preview stands on
  //                    each frame read off the canvas where smoothstep from
  //                    the one mark to the next puts it, as the engine's
  //                    render does
  // ["glides while playing"]
  //                    plays from two fifths of the way from the first
  //                    keyframe to the next, on a clock that moves on to
  //                    the next frame only once the one before is up,
  //                    until the video preview has shown ten frames of the
  //                    episode, or the play has come to the next keyframe,
  //                    and reads the frame on the canvas and the crop
  //                    frame on every tick of the play: every frame is
  //                    shown, none skipped, on each the crop frame stands
  //                    where the glide puts it for that frame, and in a
  //                    place of its own on every frame shown, not once
  async "glides while playing"(walk, arg, s, what) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const src = await ask(page, "Source", on.path);
    const piece = entry?.segments.find((p) => (p.marks ?? []).length >= 2);
    if (!piece) {
      walk.wrong = "no piece of the clip has two keyframes to glide between";
      return;
    }
    const glide = glideOf(piece.marks, src);
    const [a, b] = piece.marks;
    const crop = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.crop));
    // The episode is a filmed one, at a camera's rate, whose frame
    // number is read off the canvas from its bands.
    await filmedOnScreen(page);
    if (what === "glides") {
      const problems = [];
      for (const share of [0, 0.25, 0.5, 0.75, 1]) {
        // Half a frame in, so the click lands in the frame meant.
        const at = a.at + share * (b.at - a.at) + (share < 1 ? 0.5 / src.fps : 0);
        const g = await handles(page);
        await clickOnTrack(page, await xOf(page, at), high(g.track));
        await settle(page);
        await shownOnPlayhead(page);
        const came = await page.evaluate((fps) => {
          const at = Number(document.querySelector(".screen").dataset.playhead);
          // The frame that holds the playhead, see frameAt in lib/flow.ts.
          return window.__filmed() === Math.floor((at + 0.001) * fps);
        }, src.fps);
        if (!came) {
          problems.push(`no frame came for ${at.toFixed(3)} s`);
          continue;
        }
        const k = await filmedOnScreen(page);
        const got = await crop();
        if (got !== glide(k)) problems.push(`on frame ${k} the crop frame stands at ${got}, the glide puts it at ${glide(k)}`);
      }
      const ends = [glide(Math.round(a.at * src.fps)), glide(Math.round(b.at * src.fps))];
      if (ends[0] === ends[1]) problems.push(`the two keyframes stand at ${ends[0]} both, so nothing glides`);
      if (problems.length) walk.wrong = problems.join("; ");
      return;
    }
    const g = await handles(page);
    // Two fifths of the way, where smoothstep moves the crop fastest.
    await clickOnTrack(page, await xOf(page, a.at + 0.4 * (b.at - a.at) + 0.5 / src.fps), high(g.track));
    await settle(page);
    // The play starts from the frame on the video preview, so it is
    // started once that is the playhead's frame.
    await shownOnPlayhead(page);
    // The play is read on its clock, which the play asks on every tick
    // before it draws, so what is on screen is what the tick before drew,
    // with the crop frame it was told. The walk's clock shows every frame
    // of the play on any machine, see open in walks/bridge.mjs: by the
    // sound card's a busy one showed two in two seconds of play, issue
    // #464, and two of nine, issue #534. The play is paused by the page
    // itself once the video preview has shown ten frames of the episode
    // or the play has come to the next keyframe, where the glide ends.
    const end = b.at;
    await page.evaluate((end) => {
      const q = window.__frameQueue;
      const clock = q.playClock;
      window.__glide = [];
      window.__glideStarted = false;
      window.__glideRead = new Promise((resolve) => {
        const shown = new Set();
        let over = false;
        const done = () => {
          if (over) return;
          over = true;
          q.playClock = clock;
          resolve();
        };
        window.__glideDone = done;
        q.playClock = (heard, unshown, coming) => {
          window.__glideStarted = true;
          if (over) return clock(heard, unshown, coming);
          const el = document.querySelector(".screen");
          const k = window.__filmed();
          window.__glide.push({ k, crop: Number(el.dataset.crop) });
          if (k >= 0) shown.add(k);
          if (shown.size >= 10 || Number(el.dataset.playhead) >= end) {
            done();
            // Paused after the tick, not in the middle of it.
            queueMicrotask(() => {
              const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
              document.body.dispatchEvent(new KeyboardEvent("keydown", key));
              document.body.dispatchEvent(new KeyboardEvent("keyup", key));
            });
          }
          return clock(heard, unshown, coming);
        };
        // A play that stops by itself asks its clock no more: the reading
        // ends there, with what it read.
        const stopped = () => {
          if (over) return;
          if (window.__glideStarted && document.querySelector('button[aria-label="Play"]')) done();
          else requestAnimationFrame(stopped);
        };
        requestAnimationFrame(stopped);
      });
    }, end);
    await page.keyboard.press(" ");
    // A space bar that started no play ends the reading, once the app
    // has settled on the press.
    await settle(page);
    const played = await page.evaluate(() => {
      const playing = !document.querySelector('button[aria-label="Play"]');
      if (!playing && !window.__glideStarted) {
        window.__glideDone();
        return false;
      }
      return true;
    });
    const seen = await page.evaluate(() => window.__glideRead.then(() => window.__glide));
    if (!played) {
      walk.wrong = "the space bar started no play";
      return;
    }
    await settle(page);
    const off = seen.filter((r) => r.k >= 0 && r.crop !== glide(r.k));
    const places = new Set(seen.filter((r) => r.k >= 0).map((r) => r.crop));
    // The frames of the episode the video preview showed, each with a
    // place of its own on the glide: the crop frame stands in as many
    // places as frames were shown.
    const shown = new Set(seen.filter((r) => r.k >= 0).map((r) => r.k));
    const apart = new Set([...shown].map((k) => glide(k)));
    // Every frame from the first shown on, ten of them or up to the next
    // keyframe, whichever comes first.
    const ks = [...shown].sort((x, y) => x - y);
    const missed = [];
    for (let k = ks[0]; k < ks.at(-1); k++) if (!shown.has(k)) missed.push(k);
    const want = Math.min(10, Math.floor((end + 0.001) * src.fps) - (ks[0] ?? 0));
    if (off.length) {
      walk.wrong = `while playing, ${off.length} of ${seen.length} frames had the crop frame off the glide, the first frame ${off[0].k} at ${off[0].crop} for ${glide(off[0].k)}`;
    } else if (missed.length) {
      walk.wrong = `while playing, the video preview showed frames ${ks[0]} to ${ks.at(-1)} without ${missed.join(", ")}`;
    } else if (shown.size < want) {
      walk.wrong = `while playing up to the next keyframe, the video preview showed ${shown.size} frames, want ${want}`;
    } else if (places.size < apart.size) {
      walk.wrong = `while playing, the crop frame stood in ${places.size} places over ${shown.size} frames, where the glide has ${apart.size}`;
    }
  },
  // ["turned", quarters]
  //                    the engine has the clip's pieces rotated so many
  //                    degrees each, in order, and with the playhead put in
  //                    the middle of each by a click on the clip timeline,
  //                    the video preview shows the piece rotated that far,
  //                    the other way round from the video where a quarter
  //                    turn says so, with no black along the top or the
  //                    bottom of the picture, where a frame squeezed into
  //                    the wrong shape leaves it, and the clip timeline
  //                    draws the rotate icon on every rotated piece
  async turned(walk, arg) {
    const { page } = walk;
    const on = await chosen(page);
    const pieces = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.segments ?? [];
    await settle(page);
    const got = (await pieces()).map((p) => p.rotate ?? 0);
    if (JSON.stringify(got) !== JSON.stringify(arg)) {
      walk.wrong = `the engine has the pieces rotated ${got.join(" ")}, not ${arg.join(" ")}`;
      return;
    }
    const src = await ask(page, "Source", on.path);
    const problems = [];
    for (const [i, p] of (await pieces()).entries()) {
      const g = await handles(page);
      await clickOnTrack(page, await xOf(page, (p.start + p.end) / 2), high(g.track));
      await settle(page);
      await shownOnPlayhead(page);
      const seen = await page.evaluate(() => {
        const el = document.querySelector(".screen");
        const b = el.getBoundingClientRect();
        // How bright the picture is along its top and its bottom, where
        // a frame squeezed into the wrong shape leaves black.
        const c = el.querySelector("canvas");
        const s = document.createElement("canvas");
        s.width = Math.max(1, Math.floor(c.width / 2));
        s.height = 1;
        const t = s.getContext("2d", { willReadFrequently: true });
        const row = (y) => {
          t.drawImage(c, Math.floor(c.width / 4), y, s.width, 1, 0, 0, s.width, 1);
          const d = t.getImageData(0, 0, s.width, 1).data;
          let sum = 0;
          for (let k = 0; k < d.length; k += 4) sum += d[k];
          return sum / (d.length / 4);
        };
        return {
          turn: Number(el.dataset.turn),
          wide: b.width > b.height,
          edges: [row(Math.floor(c.height * 0.03)), row(Math.floor(c.height * 0.97))],
        };
      });
      const wide = arg[i] % 180 === 0 ? src.width > src.height : src.height > src.width;
      if (seen.turn !== arg[i]) problems.push(`piece ${i + 1} shows rotated ${seen.turn}, not ${arg[i]}`);
      if (seen.wide !== wide) problems.push(`piece ${i + 1} shows a ${seen.wide ? "wide" : "tall"} picture, not a ${wide ? "wide" : "tall"} one`);
      if (Math.min(...seen.edges) < 15) {
        problems.push(`piece ${i + 1} shows black along the top or the bottom of its picture, ${seen.edges.map((e) => e.toFixed(0)).join(" and ")}, so its frame is squeezed`);
      }
    }
    // Every rotated piece has the rotate icon drawn on its wash, and
    // no other.
    const turned = arg.filter((t) => t !== 0).length;
    const marked = await page.evaluate(() => document.querySelectorAll(".clip-timeline .piece.turned").length);
    if (marked !== turned) problems.push(`the clip timeline marks ${marked} pieces as rotated, not ${turned}`);
    if (problems.length) walk.wrong = problems.join("\n");
  },
  // ["turned with its frames"]
  //                    since "scrub", of a video standing up by its display
  //                    matrix: every frame read off the canvas was turned
  //                    as far as the Go side has the piece it belongs to,
  //                    the way the short turns it, and a frame of a cut as
  //                    far as the clip's rotations have its start, a few
  //                    hundred frames of the rotated part and of the rest
  async "turned with its frames"(walk) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const { fps } = await ask(page, "Source", on.path);
    const seen = await page.evaluate(() => window.__scrubbed ?? []);
    // The turn the short gives frame n: its piece's, as the Go side
    // reads it, and for a frame of a cut, which no short holds, the
    // stretch of the clip's rotations its start lies in.
    const turnOf = (n) => {
      const at = n / fps;
      const p = entry.segments.find((q) => at >= q.start - 0.001 && at < q.end - 0.001);
      if (p) return p.rotate ?? 0;
      const r = (entry.rotations ?? []).find((q) => at >= (q.from ?? -Infinity) - 0.001 && at < (q.to ?? Infinity) - 0.001);
      return r ? r.deg : 0;
    };
    const read = seen.filter(([, n]) => n >= 0 && n < 30 * fps);
    const bad = read.filter(([turn, n]) => turn !== turnOf(n));
    const rotated = read.filter(([, n]) => turnOf(n) !== 0).length;
    const said = [];
    if (read.length < 0.8 * seen.length || !seen.length) {
      said.push(`only ${read.length} of ${seen.length} frames on the canvas could be read`);
    } else if (!rotated || rotated === read.length) {
      said.push(`the playhead showed ${rotated} frames of the rotated part out of ${read.length}, not both kinds`);
    }
    if (bad.length) {
      said.push(`${bad.length} of ${read.length} animation frames were turned wrong:`);
      for (const [turn, n, at] of bad.slice(0, 6)) {
        said.push(`frame ${n}, of a piece rotated ${turnOf(n)}, was turned ${turn} with the playhead at ${at.toFixed(3)}`);
      }
    }
    if (said.length) walk.wrong = said.join("\n");
  },
  // ["upright"]        the short Render wrote, read back from disk, holds
  //                    as many frames as the clip's pieces, and in every
  //                    piece rotated a quarter each frame is the right
  //                    frame of the episode, read across its top, while in
  //                    the pieces not rotated most are not, since there it
  //                    lies on its side
  async upright(walk) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const { fps } = await ask(page, "Source", on.path);
    const short = await shortOf(page, on);
    if (!short || !existsSync(short)) {
      walk.wrong = `the clip has no short${short ? ` at ${short}` : ""}`;
      return;
    }
    const { frames, modulo } = shortFrames(short);
    const want = [];
    for (const p of entry.segments) {
      const first = Math.round(p.start * fps);
      const end = Math.max(Math.round(p.end * fps), first + 1);
      for (let k = first; k < end; k++) want.push({ k, rotate: p.rotate ?? 0 });
    }
    if (frames.length !== want.length) {
      walk.wrong = `the short has ${frames.length} frames, its pieces hold ${want.length}`;
      return;
    }
    const read = { rotated: [0, 0], lying: [0, 0] };
    want.forEach(({ k, rotate }, i) => {
      const r = read[rotate % 180 ? "rotated" : "lying"];
      r[1]++;
      if (frames[i] === k % modulo) r[0]++;
    });
    if (!read.rotated[1]) walk.wrong = "the clip has no piece rotated a quarter";
    else if (read.rotated[0] !== read.rotated[1]) {
      walk.wrong = `${read.rotated[1] - read.rotated[0]} of the ${read.rotated[1]} frames rotated a quarter are not the right frame read across their top`;
    } else if (read.lying[0] * 2 > read.lying[1]) {
      walk.wrong = `${read.lying[0]} of the ${read.lying[1]} frames not rotated read across their top, so the video did not lie on its side`;
    }
  },
};

// Steps that take the same body as the step they are walked by.
steps["only the video"] = steps["the clip shown"];
steps["glides"] = steps["glides while playing"];

// What is wrong with the short Render wrote, against the video preview, or
// null. The frame of the short that stands for a moment is the frame of
// the episode the video preview shows there: the last one to begin at or
// before it. A short is made at one rate, and the episode's frames, on a
// phone, at uneven times, so a frame of the short stands for the moment
// it starts at, on the grid of the rate from where the picture starts, see
// OnFrames and cutOf in engine/render.go, and the playhead is put where
// the preview shows the frame that holds that moment: after it, and
// before the episode's next frame begins. Which frame begins where is read
// from the episode with ffprobe. The clip timeline is pinched in for it,
// and a frame whose time is still too short to click on is passed over,
// but every piece is looked at where it starts and where it ends.
// The render takes a frame that begins up to a millisecond after a moment
// for that moment, frameHair, and so does the video preview, so the
// playhead goes after that too and ends a millisecond and a half before
// the next frame begins.
async function asPreviewed(page, rate, before) {
  const on = await chosen(page);
  const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
  const { fps } = await ask(page, "Source", on.path);
  const short = await shortOf(page, on);
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const probe = (args) => execFileSync("ffprobe", ["-v", "error", ...args, "-of", "csv=p=0"]).toString().trim();
  const got = probe(["-select_streams", "v:0", "-show_entries", "stream=r_frame_rate", short]);
  if (got !== rate) return `the short has ${got} frames a second, not ${rate}`;
  const begins = probe(["-select_streams", "v:0", "-show_entries", "packet=pts_time", on.path])
    .split(/\s+/).map(Number).filter((t) => Number.isFinite(t)).sort((a, b) => a - b);
  const fileStart = Number(probe(["-show_entries", "format=start_time", on.path])) || 0;
  const v0 = Math.max(0, begins[0] - fileStart);
  const pieces = entry.segments;
  if (before !== undefined && !(pieces[0].start < before && pieces[0].start < v0)) {
    return `the clip starts at ${pieces[0].start.toFixed(3)}, not before the picture at ${v0.toFixed(3)} and ${before}`;
  }
  // The pieces as the render takes them, see OnFrames.
  const grid = (t) => v0 + Math.round((t - v0) * fps) / fps;
  const slots = [];
  pieces.forEach((p, i) => {
    let start = grid(p.start);
    if (start < 0) start += 1 / fps;
    const end = Math.max(grid(p.end), start + 1 / fps);
    const n = Math.round((end - start) * fps);
    for (let q = 0; q < n; q++) slots.push({ piece: i, q, n, at: start + q / fps });
  });
  const { frames } = shortFrames(short);
  if (frames.length !== slots.length) return `the short has ${frames.length} frames, its pieces last ${slots.length}`;
  const after = (t) => begins.find((b) => b > t + 0.001) ?? Infinity;
  // Up close, pinched in to a tenth, so a frame is some pixels wide and
  // the edges of the pieces, which a click takes hold of, cover less than
  // one.
  const track = (await handles(page)).track;
  await page.mouse.move(track.x + track.w / 2, high(track));
  await page.keyboard.down("Control");
  await page.mouse.wheel(0, -230);
  await page.keyboard.up("Control");
  // A wheel moves the view at once, see wheel in ClipTimeline.svelte, so
  // two frames have drawn it.
  await nextPaint(page, 2);
  const problems = [];
  let looked = 0;
  let missed = 0;
  const seenAt = new Set();
  for (let j = 0; j < slots.length; j++) {
    const { q, n, at } = slots[j];
    if (!(q < 3 || q >= n - 3 || q % 5 === 0)) continue;
    // Where the frame that holds the moment is the one on screen: from the
    // moment until the episode's next frame begins.
    const from = at + 0.0015;
    const to = after(at) - 0.0015;
    if (to - from < 0.002) continue;
    // Moved along with two fingers until the moment is well inside.
    for (let tries = 0; tries < 6; tries++) {
      const track = (await handles(page)).track;
      const off = (await xOf(page, (from + to) / 2)) - (track.x + track.w / 2);
      if (Math.abs(off) < track.w / 4) break;
      await page.mouse.move(track.x + track.w / 2, high(track));
      await page.mouse.wheel(off, 0);
      await nextPaint(page, 2);
    }
    const g = await handles(page);
    const x0 = await xOf(page, from);
    const x1 = await xOf(page, to);
    // Clear of the clip's edges and of the edges of its cuts, which a
    // click takes hold of instead.
    const grips = [g.start, g.end, ...g.cuts, ...g.cutEdges].map((r) => [r.x - 3, r.x + r.w + 3]);
    if (x1 - x0 < 0.2) continue;
    // A click lands where the clip timeline's own track says, which can be
    // a pixel or two from where xOf reckons, so it is put right from where
    // the playhead went, as a hand would.
    const target = (from + to) / 2;
    let x = (x0 + x1) / 2;
    let placed = NaN;
    // Where each click put the playhead, which a failure says.
    const tried = [];
    for (let tries = 0; tries < 4; tries++) {
      if (grips.some(([a, b]) => x > a && x < b)) break;
      await clickOnTrack(page, x, high(g.track));
      await settle(page);
      placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
      tried.push(placed.toFixed(4));
      if (placed >= from && placed <= to) break;
      x += ((target - placed) * (x1 - x0)) / Math.max(to - from, 1e-6);
    }
    if (!(placed >= from && placed <= to)) {
      if (!Number.isNaN(placed)) missed++;
      continue;
    }
    looked++;
    seenAt.add(`${slots[j].piece} ${q < 3 ? "first" : q >= n - 3 ? "last" : "inside"}`);
    await filmedOnScreen(page);
    // The frame the preview shows, once the app says the frame it shows
    // is the one that holds the playhead. On a busy machine the frame
    // before it is still up when the app has settled.
    await shownOnPlayhead(page);
    const came = (await page.evaluate(() => window.__filmed?.())) === frames[j];
    const seen = came ? frames[j] : await filmedOnScreen(page);
    if (!came || (await filmedOnScreen(page)) !== frames[j]) {
      // With the clicks that put the playhead there, and where the frame
      // shown begins by the app's count, since this failed only on a busy
      // machine, issue #325, and said nothing of how it came about.
      const shown = await page.evaluate(() => document.querySelector(".screen").dataset.shown);
      problems.push(
        `frame ${j} of the short, for ${at.toFixed(4)} s, is frame ${frames[j]} of the episode, the video preview shows ${seen} at ${placed.toFixed(4)}` +
          `${came ? ", after it showed the right one" : ""}, with the playhead put at ${tried.join(", then ")} and the frame shown beginning at ${shown}`,
      );
    }
  }
  // Every piece looked at where it starts and where it ends, so at both
  // ends of the clip and on both sides of every cut, and inside.
  const unseen = pieces.flatMap((_, i) => ["first", "last"].filter((w) => !seenAt.has(`${i} ${w}`)).map((w) => `the ${w} frames of piece ${i + 1}`));
  if (unseen.length) problems.push(`the playhead could not be put on ${unseen.join(", ")}`);
  if (looked < 20) problems.push(`only ${looked} frames could be looked at, the playhead missed ${missed}`);
  console.log(`      ${looked} frames of ${slots.length} looked at, the playhead missed ${missed} more`);
  return problems.length ? problems.slice(0, 8).join("\n") + (problems.length > 8 ? `\nand ${problems.length - 8} more` : "") : null;
}

// What is wrong with the frames the video preview draws on the clip's
// edges, against the short Render wrote, or null. The playhead is put on
// each edge the way a hand puts it there, a click on the clip's start, on
// where each cut ends and on the clip's end, and the frame the video
// preview draws, read off the canvas, is the frame the short starts that
// piece with, or the short's last at the clip's end. Bug 1 of
// docs/DESIGN-REVIEW.md. An edge is kept to the millisecond, and at 29.97
// fps that lies just before its frame's start for 1400 of every 3000
// frames, where the video preview drew the frame before the one the short
// starts on. And a search left its edges between frames. The clip's end
// has to be where the search put it, and at least one edge just before
// its frame's start, or the walk is not about the case.
async function edgesAsRendered(page) {
  const on = await chosen(page);
  const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
  const { fps, videoStart = 0 } = await ask(page, "Source", on.path);
  const short = await shortOf(page, on);
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const { frames } = shortFrames(short);
  const pieces = entry.segments;
  // The frame an edge means, the one whose start is nearest, and each
  // piece's frames from there, the way the render cuts them, see
  // nearestFrame and cutOf in engine/render.go.
  const nearest = (t) => Math.round((t - videoStart) * fps);
  const firsts = [];
  let held = 0;
  for (const p of pieces) {
    const first = nearest(p.start);
    firsts.push(held);
    held += Math.max(nearest(p.end), first + 1) - first;
  }
  if (frames.length !== held) return `the short has ${frames.length} frames, its pieces hold ${held}`;
  // The clip's start and where each cut ends. Two pieces that meet, at a
  // camera switch or where the picture starts, have no cut between them.
  const edges = [{ what: "the clip's start", label: "Clip start", at: pieces[0].start, want: frames[firsts[0]] }];
  for (let i = 1; i < pieces.length; i++) {
    if (pieces[i].start <= pieces[i - 1].end) continue;
    edges.push({ what: `where cut ${i} ends`, label: `Where cut ${i} ends`, at: pieces[i].start, want: frames[firsts[i]] });
  }
  edges.push({ what: "the clip's end", label: "Clip end", at: pieces[pieces.length - 1].end, want: frames[frames.length - 1] });
  // Kept just before the start of the frame it means, by the rounding to
  // the millisecond.
  const before = edges.filter((e) => {
    const begins = videoStart + nearest(e.at) / fps;
    return e.at < begins - 1e-9 && begins - e.at < 0.0006;
  });
  if (!before.length) {
    return `no edge of the clip, at ${edges.map((e) => e.at).join(", ")}, lies just before its frame's start, the case this is about`;
  }
  if (Math.abs(entry.found[1] - pieces[pieces.length - 1].end) > 1e-9) {
    return `the clip ends at ${pieces[pieces.length - 1].end}, not at ${entry.found[1]} where the search put it, the other case this is about`;
  }
  await filmedOnScreen(page);
  const problems = [];
  for (const e of edges) {
    // High on the track, where the captions' edges below are not.
    const handle = page.locator(`.clip-timeline [aria-label="${e.label}"]`);
    const grip = (await handle.count()) ? await handle.boundingBox() : null;
    if (!grip) {
      problems.push(`there is no ${e.label} to click`);
      continue;
    }
    await clickOnTrack(page, grip.x + grip.width / 2, high((await handles(page)).track));
    await settle(page);
    // On the edge's frame, where the engine puts the playhead for an
    // edge, see edgePlayhead in engine/shape.go: the first frame of the
    // piece an edge starts, and the clip's last frame for its end.
    const placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
    // A millisecond early is on the frame, the rounding the edges are kept
    // to, which is the case this is about.
    const frameOn = nearest(e.at) - (e.label === "Clip end" ? 1 : 0);
    if (Math.floor((placed - videoStart + 0.001) * fps) !== frameOn) {
      problems.push(`a click on ${e.what} put the playhead at ${placed.toFixed(4)}, not on its frame ${frameOn}`);
      continue;
    }
    await settle(page);
    await shownOnPlayhead(page);
    const came = (await page.evaluate(() => window.__filmed?.())) === e.want;
    if (!came) {
      problems.push(`on ${e.what} at ${e.at.toFixed(3)} s the video preview shows frame ${await filmedOnScreen(page)}, the short frame ${e.want}`);
    }
  }
  console.log(`      ${edges.length} edges looked at, ${before.length} kept just before their frame`);
  return problems.length ? problems.join("\n") : null;
}

// The left edge of the crop on each frame of the video, gliding over a
// piece's keyframes by smoothstep, the way the engine's glideX and
// lib/glide.ts work it out, see glide.cases.json, for frame numbers
// counted from the picture's first frame.
function glideOf(marks, src) {
  const v0 = src.videoStart ?? 0;
  const most = src.width - src.cropWidth;
  const on = marks.map((m) => {
    const x = Math.max(0, Math.min(m.x, most));
    return [Math.floor((m.at - v0) * src.fps + 0.5), x - (x % 2)];
  });
  return (k) => {
    if (k <= on[0][0]) return on[0][1];
    for (let i = 1; i < on.length; i++) {
      const [kb, xb] = on[i];
      if (k >= kb) continue;
      const [ka, xa] = on[i - 1];
      const u = (k - ka) / (kb - ka);
      return xa + (xb - xa) * (u * u * (3 - 2 * u));
    }
    return on[on.length - 1][1];
  };
}

async function frameWhole(page, turned) {
  const at = await page.evaluate(() => {
    const box = (e) => e?.getBoundingClientRect().toJSON();
    const frame = document.querySelector(".player .frame");
    if (!frame) return null;
    const line = getComputedStyle(frame);
    const crop = document.querySelector(".screen .crop") ?? frame;
    return {
      viewer: box(document.querySelector(".viewer")),
      screen: box(document.querySelector(".screen")),
      frame: box(frame),
      crop: box(crop),
      accent: line.borderTopColor.match(/\d+/g).slice(0, 3).map(Number),
      line: parseFloat(line.borderTopWidth),
      radius: parseFloat(line.borderTopLeftRadius),
      dpr: devicePixelRatio,
    };
  });
  if (!at) return "the video preview has no crop frame";
  const { viewer, screen, frame, crop } = at;
  const near = (a, b) => Math.abs(a - b) < 0.5;
  if (!near(frame.left, crop.left) || !near(frame.width, crop.width) || !near(frame.top, crop.top) || !near(frame.height, crop.height)) {
    return `the crop frame's line stands at ${frame.left.toFixed(1)} to ${frame.right.toFixed(1)}, the crop the hand moves at ${crop.left.toFixed(1)} to ${crop.right.toFixed(1)}`;
  }
  // A crop frame away from the viewer's corners has nothing there to cut
  // it, and the case would pass without being the case. A part rotated a
  // quarter is narrower than the viewer, and is there for the lid, which
  // has to turn with the picture.
  const corners = [screen.left - viewer.left, viewer.right - screen.right, screen.top - viewer.top, viewer.bottom - screen.bottom];
  if (turned ? corners[0] < 8 : corners.some((d) => d > 1)) {
    return `the picture stands ${corners.map((d) => d.toFixed(1)).join(", ")} px in from the viewer's sides, so its corners are not the viewer's`;
  }
  // Issue #264: a part rotated a quarter stood a pixel above the viewer,
  // its top row cut and a black line under it. It stands inside the
  // viewer, in its middle, on a whole pixel.
  if (turned && (corners[2] < 0 || corners[3] < 0 || Math.abs(corners[2] - corners[3]) > 1 || Math.abs(corners[2] - Math.round(corners[2])) > 0.01)) {
    return `the rotated picture stands ${corners[2].toFixed(2)} px down from the viewer's top and ${corners[3].toFixed(2)} px up from its foot`;
  }
  const look = await page.addStyleTag({ content: ".screen canvas, .screen .trouble, .screen .failed { visibility: hidden !important } .screen { background: #fff !important }" });
  await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
  const clip = { x: viewer.left - 4, y: viewer.top - 4, width: viewer.width + 8, height: viewer.height + 8 };
  const shot = (await page.screenshot({ clip })).toString("base64");
  await look.evaluate((e) => e.remove());
  return page.evaluate(
    async ({ shot, at, clip }) => {
      const img = new Image();
      img.src = `data:image/png;base64,${shot}`;
      await img.decode();
      const c = document.createElement("canvas");
      c.width = img.naturalWidth;
      c.height = img.naturalHeight;
      const g = c.getContext("2d", { willReadFrequently: true });
      g.drawImage(img, 0, 0);
      const data = g.getImageData(0, 0, c.width, c.height).data;
      const d = at.dpr;
      const px = (x, y) => {
        const i = (Math.floor(y) * c.width + Math.floor(x)) * 4;
        return [data[i], data[i + 1], data[i + 2]];
      };
      const say = (p) => `rgb(${p.join(", ")})`;
      const light = (p) => Math.max(...p);
      const accent = (p) => p.every((v, k) => Math.abs(v - at.accent[k]) <= 48);
      // The frame in the screenshot's own pixels.
      const l = (at.frame.left - clip.x) * d;
      const t = (at.frame.top - clip.y) * d;
      const r = (at.frame.right - clip.x) * d;
      const b = (at.frame.bottom - clip.y) * d;
      const w = at.line * d;
      const round = at.radius * d;
      const problems = [];

      // The line, its outermost row of pixels and its innermost, from
      // where one rounded corner ends to where the next begins.
      const sides = [
        ["left", (k, e) => [l + e, k], t, b],
        ["right", (k, e) => [r - e, k], t, b],
        ["top", (k, e) => [k, t + e], l, r],
        ["bottom", (k, e) => [k, b - e], l, r],
      ];
      for (const [side, point, from, to] of sides) {
        const off = [];
        for (let k = Math.ceil(from + round + 1); k < to - round - 1; k++) {
          for (const e of [0.5, w - 0.5]) {
            const p = px(...point(k + 0.5, e));
            if (!accent(p)) off.push({ k: k - from, p });
          }
        }
        if (off.length) {
          problems.push(
            `the crop frame's line is not the app's colour on its ${side} side for ${off.length} pixels, the first ${(off[0].k / d).toFixed(0)} px along it, which is ${say(off[0].p)}`,
          );
        }
      }

      // Each rounded corner, outside its curve: the shade, never the white
      // picture. A pixel the curve passes through is the line's and the
      // shade's both, so only those whose middle is outside it count.
      const corners = [
        ["top left", l, t, 1, 1],
        ["top right", r, t, -1, 1],
        ["bottom left", l, b, 1, -1],
        ["bottom right", r, b, -1, -1],
      ];
      for (const [name, x0, y0, sx, sy] of corners) {
        const cx = x0 + sx * round;
        const cy = y0 + sy * round;
        let worst = null;
        for (let i = 0; i < round; i++) {
          for (let j = 0; j < round; j++) {
            const x = x0 + sx * (i + 0.5);
            const y = y0 + sy * (j + 0.5);
            if (Math.hypot(x - cx, y - cy) < round + 0.25) continue;
            const p = px(x, y);
            if (light(p) > 170 && (!worst || light(p) > light(worst))) worst = p;
          }
        }
        if (worst) problems.push(`the crop frame's ${name} corner has the picture bright outside its curve, ${say(worst)}, where the shade should be`);
      }

      // Inside the frame the picture as it is, beside it under the shade.
      const inside = px((l + r) / 2, t + w + 4 * d);
      if (light(inside) < 230) problems.push(`inside the crop frame the white picture is ${say(inside)}, shaded`);
      // A crop as wide as the picture, on a rotated part, has nothing
      // beside it.
      const right = at.screen.right - at.frame.right;
      const left = at.frame.left - at.screen.left;
      const beside = px(right > left ? r + 3 * d : l - 3 * d, (t + b) / 2);
      if (Math.max(left, right) > 4 && !(light(beside) > 60 && light(beside) < 170)) {
        problems.push(`beside the crop frame the white picture is ${say(beside)}, not shaded`);
      }
      return problems.length ? problems.join("\n") : null;
    },
    { shot, at, clip },
  );
}

// What is wrong with where the frames lie on the two tracks, or null, read
// off a screenshot of each: the chosen clip's frame on the clip timeline
// and the bars of the window at rest on the range picker. Each lies on its
// track's top and bottom border, so the outermost rows of the track's box
// are the app's colour as deep as the line is thick, and the row just
// outside the box is not. Columns near the playhead and the marks standing
// on a line are left out, and so are the frame's rounded ends. See
// ["on borders"].
async function onBorders(page) {
  const at = await page.evaluate(() => {
    const box = (e) => e?.getBoundingClientRect().toJSON();
    const span = document.querySelector(".clip-timeline .span");
    const aim = document.querySelector(".aim");
    const bar = aim?.querySelector(".bar.top");
    // The app's colour and its lighter shade, the frame under the hand,
    // as numbers, whether the browser says them as rgb() or color(srgb).
    const colour = (css) => {
      const e = document.createElement("i");
      e.style.color = css;
      document.body.append(e);
      const said = getComputedStyle(e).color;
      e.remove();
      const n = said.match(/[\d.]+/g).map(Number);
      return said.startsWith("color(") ? n.slice(0, 3).map((v) => Math.round(v * 255)) : n.slice(0, 3);
    };
    const away = [...document.querySelectorAll(".playhead, .head, .cropdot, .thumb, .tip")].map((e) => box(e));
    return {
      dpr: devicePixelRatio,
      accent: [colour("var(--accent)"), colour("var(--accent-hi)")],
      clip: span && { track: box(document.querySelector(".clip-timeline .track")), from: box(span).left, to: box(span).right, deep: parseFloat(getComputedStyle(span).borderTopWidth) },
      range: aim && { track: box(aim.parentElement.querySelector(".track")), from: box(aim).left, to: box(aim).right, deep: parseFloat(getComputedStyle(bar, null).backgroundSize.split(" ")[1]) },
      away,
    };
  });
  if (!at.clip) return "the clip timeline shows no frame for the chosen clip";
  if (!at.range) return "the range picker shows no window at rest";
  const problems = [];
  for (const [name, t] of [["the clip's frame on the clip timeline", at.clip], ["the window's bars on the range picker", at.range]]) {
    const clip = { x: Math.floor(t.track.left) - 2, y: Math.floor(t.track.top) - 4, width: Math.ceil(t.track.width) + 4, height: Math.ceil(t.track.height) + 8 };
    const shot = (await page.screenshot({ clip })).toString("base64");
    const found = await page.evaluate(
      async ({ shot, at, t, clip, name }) => {
        const img = new Image();
        img.src = `data:image/png;base64,${shot}`;
        await img.decode();
        const c = document.createElement("canvas");
        c.width = img.naturalWidth;
        c.height = img.naturalHeight;
        const g = c.getContext("2d", { willReadFrequently: true });
        g.drawImage(img, 0, 0);
        const data = g.getImageData(0, 0, c.width, c.height).data;
        const d = at.dpr;
        // The pixel in the middle of a CSS pixel of the page.
        const px = (x, y) => {
          const i = (Math.floor((y - clip.y + 0.5) * d) * c.width + Math.floor((x - clip.x + 0.5) * d)) * 4;
          return [data[i], data[i + 1], data[i + 2]];
        };
        const accent = (p) => at.accent.some((a) => p.every((v, k) => Math.abs(v - a[k]) <= 24));
        const say = (p) => `rgb(${p.join(", ")})`;
        const near = (x) => at.away.some((b) => b && x > b.left - 4 && x < b.right + 4);
        const problems = [];
        let looked = 0;
        for (let x = Math.ceil(t.from) + 12; x < Math.floor(t.to) - 12; x += 3) {
          if (near(x)) continue;
          looked++;
          for (const [side, edge, inward] of [["top", t.track.top, 1], ["bottom", t.track.bottom - 1, -1]]) {
            for (let k = 0; k < t.deep; k++) {
              const p = px(x, edge + inward * k);
              if (!accent(p)) {
                problems.push(`${name} does not cover the ${side} border at ${(x - t.from).toFixed(0)} px along it: ${k} px in from the track's edge is ${say(p)}`);
                return problems;
              }
            }
            const out = px(x, edge - inward);
            if (accent(out)) {
              problems.push(`${name} stands outside the ${side} border at ${(x - t.from).toFixed(0)} px along it: the row just past the track's edge is the app's colour`);
              return problems;
            }
          }
        }
        if (looked < 5) problems.push(`${name} had only ${looked} places to look at away from the playhead and the marks`);
        return problems;
      },
      { shot, at, t, clip, name },
    );
    problems.push(...found);
  }
  return problems.length ? problems.join("; ") : null;
}
