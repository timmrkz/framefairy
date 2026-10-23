// How quickly the video preview follows the keys, timed on a video the size
// a camera makes, against the real Go side. Not a walk: it checks no rule
// and fails on no number, it prints a table. Run it by hand, before and
// after a change to the frame queue, see docs/TESTING.md.
//
//     BRIDGE_URL=http://127.0.0.1:8123/ node frontend/preview/walks/speed.mjs
//
// It adds a video of 1920 by 1080 at 25 frames a second, picks the clip its
// search finds and presses the keys a person walks a clip with: Shift and
// an arrow from word to word, and an arrow a frame at a time. For each
// press it times, in the page, how long from the key going down until the
// video preview shows the frame that holds the playhead, and until that
// frame has its own captions, which libass draws on the Go side. Between presses
// the hand rests for REST_FRAMES frames of the page, 15 unless set, a
// quarter of a second on a screen of 60 Hz, about as long as a person takes
// to look and press again. That rest is the one pause on the clock made on
// purpose: the browser's frames come by its own timer, and the rest is part
// of what is measured, a person's pace, not a wait for work. ROUNDS, 5
// unless set, walks the
// clip that many times. CODEC=hevc makes the video HEVC with 10-bit colour.
// SCALE, 2 unless set, is the device pixels of a CSS pixel, a Mac's screen.
import { control, fromSidebar, handles, high, nextPaint, open, pageUntil, settle, shownOnPlayhead } from "./bridge.mjs";

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
// REST was the same pause in milliseconds, and 250 of them now would be
// four seconds of frames.
if (process.env.REST) throw new Error("REST is now REST_FRAMES, the rest between presses in frames of the page, 15 unless set");
const rest = Number(process.env.REST_FRAMES ?? 15);
const rounds = Number(process.env.ROUNDS ?? 5);
const scale = Number(process.env.SCALE ?? 2);
const codec = process.env.CODEC === "hevc" ? "&codec=hevc" : "";

// A window as big as a 16 inch MacBook Pro's screen, which has the video
// preview draw frames about the size Tim's do.
// VIEWPORT, as 2560x1440, sets another size.
const [vw, vh] = (process.env.VIEWPORT ?? "1728x1080").split("x").map(Number);
const { browser, page, errors } = await open(url, { viewport: { width: vw, height: vh }, scale });
try {
  await control(url, `/pick?seconds=60&size=1080${codec}`);
  await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
  // The page is opened afresh for every round, so no round finds the frames
  // the one before kept, and each times what a first walk of a clip costs.
  // The Go side stays as it is, the way the app stays open.
  // Until the video on screen is the one added and its search has put a
  // clip in the list, read each time the page changes, never on the clock.
  const ready = async () => {
    await pageUntil(page, () => {
      const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
      const on = document.querySelector("aside button.episode.current .name")?.textContent ?? "";
      return on.startsWith("folge-") && pane?.querySelectorAll("ol li[data-key]").length > 0;
    });
    await page.evaluate((fps) => {
      // The moment a key goes down, before the app hears it.
      window.addEventListener("keydown", () => (window.__keyAt = performance.now()), { capture: true });
      // Resolves with the milliseconds from the last key until the video
      // preview shows the frame that holds the playhead, and until that
      // frame has its own captions, the word a step lands on lifted out to
      // bounce included, read on every frame of the page. Each is -1 when
      // it never comes: the frame when the frame queue has done all the
      // press asked of it and the picture standing is another frame, see
      // standing in lib/frames/queue.ts, and the captions when a layer of
      // them is given up on, data-captions-lost. A press that left the
      // playhead where it was, once the app has done all it set going,
      // window.__settled, is a press that did nothing, and fails the run,
      // so the table never counts it.
      window.__followed = (before, still = false) =>
        new Promise((done, failed) => {
          let frame = -1;
          let over = false;
          const end = (r) => {
            over = true;
            done(r);
          };
          if (!still)
            window.__settled(false).then(() => {
              if (!over && Number(document.querySelector(".screen")?.dataset.playhead) === before) {
                over = true;
                failed(new Error(`a press left the playhead at ${before}`));
              }
            });
          const lost = document.querySelector(".screen")?.dataset.captionsLost ?? "";
          const check = () => {
            const s = document.querySelector(".screen");
            const q = window.__frameQueue;
            const at = Number(s?.dataset.playhead);
            const shown = Number(s?.dataset.shown);
            const holds = Math.floor(at * fps + 1e-6) / fps;
            const took = performance.now() - window.__keyAt;
            const layers = window.__captionLayers;
            const captioned = !layers || (!layers.waiting && !(layers.lift && !layers.lift.word));
            const moved = still || at !== before;
            if (frame < 0 && moved && Math.abs(shown - holds) < 1e-3) frame = took;
            if (over) return;
            if (frame >= 0 && captioned) end([frame, took]);
            else if (frame >= 0 && (s?.dataset.captionsLost ?? "") !== lost) end([frame, -1]);
            else if (frame < 0 && moved && q?.standing) end([-1, -1]);
            else requestAnimationFrame(check);
          };
          check();
        });
    }, fps);
  };
  const fps = 25;
  await ready();

  const times = new Map();
  const captions = new Map();
  // How the frames of each kind of press came, from the moment it went
  // down until the next: a stream opened, read on in an open one, or kept.
  const ways = new Map();
  const how = () =>
    page.evaluate(() => {
      const s = window.__appFrames?.stats;
      return [s?.streams ?? 0, s?.continued ?? 0, s?.kept ?? 0];
    });
  const press = async (name, key) => {
    const before = await page.evaluate(() => Number(document.querySelector(".screen")?.dataset.playhead));
    const was = await how();
    await page.keyboard.press(key);
    const [took, captioned] = await page.evaluate((b) => window.__followed(b), before);
    if (!times.has(name)) times.set(name, []);
    times.get(name).push(took);
    if (!captions.has(name)) captions.set(name, []);
    captions.get(name).push(captioned);
    await nextPaint(page, rest);
    const now = await how();
    const w = ways.get(name) ?? [0, 0, 0];
    ways.set(name, w.map((n, i) => n + now[i] - was[i]));
  };
  // The playhead dragged over the clip timeline, across the clip and back,
  // a step every frame of the page the way a hand moves. Every frame of the page notes
  // how many frames the frame on screen is from the playhead, and once the
  // hand lets go, how long until the frame that holds the playhead shows.
  const apart = [];
  const settled = [];
  const dragAcross = async () => {
    const g = await handles(page);
    const left = g.start.x + g.start.w;
    const span = g.end.x - left;
    const y = high(g.track);
    await page.evaluate((fps) => {
      window.__dragged = [];
      window.__dragging = true;
      const tick = () => {
        if (!window.__dragging) return;
        const el = document.querySelector(".screen");
        const at = Number(el.dataset.playhead);
        const shown = Number(el.dataset.shown);
        window.__dragged.push(Math.abs(Math.floor(at * fps + 1e-6) - Math.round(shown * fps)));
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    }, fps);
    const path = [0.1, 0.9, 0.3];
    await page.mouse.move(left + path[0] * span, y);
    await page.mouse.down();
    for (let leg = 1; leg < path.length; leg++) {
      const [a, b] = [path[leg - 1], path[leg]];
      for (let i = 1; i <= 60; i++) {
        await page.mouse.move(left + (a + ((b - a) * i) / 60) * span, y);
        await nextPaint(page);
      }
    }
    const before = await page.evaluate(() => {
      window.__dragging = false;
      return Number(document.querySelector(".screen")?.dataset.playhead);
    });
    apart.push(...(await page.evaluate(() => window.__dragged)));
    await page.evaluate(() => (window.__keyAt = performance.now()));
    await page.mouse.up();
    const [took] = await page.evaluate((b) => window.__followed(b, true), before);
    settled.push(took);
    await settle(page);
  };
  // A clip picked, and the hand rests until its first frame is up with
  // its captions and what is read ahead for the next step is in, the way
  // a person looks at a clip before walking it.
  const pickClip = async () => {
    await page.locator("aside ol li[data-key] button.pick").first().click();
    await settle(page);
    await page.evaluate(() => document.body.focus());
    await shownOnPlayhead(page, { captions: true });
    await page.evaluate(() => window.__readAheadRested());
  };

  const seen = { streams: 0, continued: 0, kept: 0, parked: 0, ahead: 0, aheadLost: 0, size: "" };
  const count = async () => {
    const s = await page.evaluate(() => {
      const s = window.__appFrames?.stats;
      return s ? { streams: s.streams, continued: s.continued, kept: s.kept, parked: s.parked, ahead: s.ahead, aheadLost: s.aheadLost, size: s.first } : null;
    });
    if (!s) return;
    for (const k of ["streams", "continued", "kept", "parked", "ahead", "aheadLost"]) seen[k] += s[k];
    seen.size ||= s.size;
  };
  for (let r = 0; r < rounds; r++) {
    if (r > 0) {
      await count();
      await page.reload();
      await ready();
    }
    await pickClip();
    for (let i = 0; i < 8; i++) await press("Shift+Right, a word on", "Shift+ArrowRight");
    for (let i = 0; i < 3; i++) await press("Left, a frame back", "ArrowLeft");
    for (let i = 0; i < 3; i++) await press("Right, a frame on", "ArrowRight");
    // Seven back, not eight: the clip is picked with the playhead before
    // its first word, so eight on lands on the eighth word, and an eighth
    // step back from the first word has no word to go to and does nothing.
    for (let i = 0; i < 7; i++) await press("Shift+Left, a word back", "Shift+ArrowLeft");
    for (let i = 0; i < 4; i++) {
      await press("Shift+Right, a word on", "Shift+ArrowRight");
      await press("Left after a word", "ArrowLeft");
    }
    await dragAcross();
  }

  await count();
  const stats = seen;
  const q = (xs, p) => xs[Math.min(xs.length - 1, Math.floor(p * xs.length))];
  const rows = [...times].map(([name, xs]) => {
    const ok = xs.filter((x) => x >= 0).sort((a, b) => a - b);
    const [opened, readOn, kept] = ways.get(name) ?? [0, 0, 0];
    const cs = captions.get(name).filter((x) => x >= 0).sort((a, b) => a - b);
    const ms = (v) => (v === undefined ? "-" : String(Math.round(v)));
    return [name, String(xs.length), ms(q(ok, 0.5)), ms(q(ok, 0.9)), ms(ok[ok.length - 1]), String(xs.length - ok.length), ms(q(cs, 0.5)), ms(q(cs, 0.9)), String(opened), String(readOn), String(kept)];
  });
  const head = ["What", "Presses", "Median ms", "90th ms", "Most ms", "Never", "Captions median ms", "Captions 90th ms", "Opened", "Read on", "Kept"];
  const widths = head.map((h, i) => Math.max(h.length, ...rows.map((r) => r[i].length)));
  const line = (r) => "| " + r.map((c, i) => (i === 0 ? c.padEnd(widths[i]) : c.padStart(widths[i]))).join(" | ") + " |";
  console.log(line(head));
  console.log("| " + widths.map((w, i) => (i === 0 ? "-".repeat(w) : "-".repeat(w - 1) + ":")).join(" | ") + " |");
  for (const r of rows) console.log(line(r));
  console.log(`frames at ${stats?.size}, ${stats?.streams} streams opened, ${stats?.continued} frames waited for on an open stream, ${stats?.kept} from the kept frames, ${stats?.parked} parked, ${stats?.ahead} read ahead, ${stats?.aheadLost} of them let go of before the hand asked for them`);
  const sorted = (xs) => xs.filter((x) => x >= 0).sort((a, b) => a - b);
  const a = sorted(apart);
  const z = sorted(settled);
  console.log(
    `dragging the playhead: the frame on screen ${q(a, 0.5)} frames from it on the median, ${q(a, 0.9)} at the 90th, ${a[a.length - 1]} at most, ` +
      `and the frame that holds it ${Math.round(q(z, 0.5))} ms after letting go on the median, ${Math.round(z[z.length - 1])} ms at most`,
  );
  if (errors.length) console.log("errors on the page:", errors.join("\n"));
} finally {
  await browser.close();
}
