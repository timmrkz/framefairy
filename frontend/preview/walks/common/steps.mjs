// The steps the sequences of every folder take: every part of the app:
// keys, buttons, fields, undo and redo, and the marks the other steps
// compare with. Each is said above it, a list with its name first.
// walkSequence in ../sequences.mjs walks it, and hands it the walk so far,
// the step's argument, what the caption box showed before it and the step's
// name. Below them the helpers the steps of more than one folder use. See
// docs/TESTING.md.
import { ask, control, engineState, episodeOn, fakeClock, handles, high, hold, holdClock, middle, news, nextNews, pageUntil, settle } from "../bridge.mjs";
import { describe, same } from "../rules.mjs";

export const steps = {
  // ["button", label]  clicks the button that says label, and waits until
  //                    it has done its work
  async button(walk, arg) {
    const { page } = walk;
    const button = page.getByRole("button", { name: arg, exact: true });
    await button.click();
    await pageUntil(page, (label) => ![...document.querySelectorAll("button")].some((b) => b.textContent.trim() === label && b.disabled), arg);
  },
  // ["press twice", key]
  //                    presses a key twice, with nothing heard by dictation
  //                    let in between
  async "press twice"(walk, arg, s) {
    const { page, watch } = walk;
    // Nothing heard comes in between, however busy the machine.
    await page.evaluate(() => window.__pauseEvents(true));
    await page.keyboard.press(arg);
    await page.keyboard.press(arg);
    await page.evaluate(() => window.__pauseEvents(false));
    await watch.step("press", s, page);
  },
  // ["press", key]     presses a key, as Playwright names it
  async press(walk, arg, s) {
    const { page, watch } = walk;
    await page.keyboard.press(arg);
    await watch.step(arg === "Enter" && !s.some((w) => w.focused) ? "open" : "press", s, page);
  },
  // ["type", text]     types
  async type(walk, arg, s) {
    const { page, watch } = walk;
    await page.keyboard.type(arg);
    await watch.step("type", s, page);
  },
  // ["field", selector, text]
  //                    clicks the field selector names and types text
  async field(walk, arg) {
    const { page } = walk;
    const [selector, text] = arg;
    await page.locator(selector).click();
    await page.keyboard.type(text);
  },
  // ["escape lets go", selector]
  //                    clicks the control or field, presses Escape, and
  //                    sees no list or pop-up open, the keyboard let go
  //                    of and no ring on anything
  async "escape lets go"(walk, arg) {
    const { page } = walk;
    const control = page.locator(arg).first();
    await control.click();
    await settle(page);
    await page.keyboard.press("Escape");
    await settle(page);
    const after = await control.evaluate((el) => ({
      open: document.querySelectorAll(".pick-list, .colour-pop").length,
      kept: document.activeElement === el,
      lit: [...document.querySelectorAll(":focus-visible")].map((x) => x.id || x.getAttribute("aria-label") || x.className || x.tagName),
    }));
    if (after.open) walk.wrong = `${arg}: a list or pop-up is still open after Escape`;
    else if (after.kept) walk.wrong = `${arg} still has the keyboard after Escape`;
    else if (after.lit.length) walk.wrong = `${arg}: a ring is left on ${after.lit.join(", ")} after Escape`;
  },
  // ["field reads", selector, text]
  //                    the field selector names holds text
  async "field reads"(walk, arg) {
    const { page } = walk;
    const [selector, text] = arg;
    const have = await page.locator(selector).inputValue();
    if (have !== text) walk.wrong = `${selector} reads ${JSON.stringify(have)}, not ${JSON.stringify(text)}`;
  },
  // ["undo refused"]   the menu bar's Undo, which the Go side refuses
  //                    because what it would put back has changed since
  async "undo refused"(walk, arg, s) {
    const { page, watch } = walk;
    const from = await page.evaluate(() => window.__calls.length);
    await page.evaluate(() => window.__menu("undo"));
    await settle(page);
    // The refusal is the answer here, so it is taken off the calls
    // that failed before the rules read them.
    const said = await page.evaluate((from) => {
      const c = window.__calls.slice(from).find((c) => c.name === "Undo" && c.failed);
      if (!c) return null;
      const failed = c.failed;
      c.failed = null;
      return String(failed);
    }, from);
    if (!said?.includes("changed since")) walk.wrong = `Undo was not refused as changed since: ${said}`;
    await watch.step("undo", s, page);
  },
  // ["undo"], ["redo"] the menu bar's Undo and Redo
  async redo(walk, arg, s, what) {
    const { page, watch } = walk;
    await page.evaluate((m) => window.__menu(m), what);
    await watch.step(what, s, page);
  },
  // ["undo then redo"] the menu bar's Undo and, before anything Undo
  //                    asked of the Go side is answered, Redo
  async "undo then redo"(walk, arg, s) {
    const { page, watch } = walk;
    await page.evaluate(() => {
      window.__menu("undo");
      window.__menu("redo");
    });
    await watch.step("undo then redo", s, page);
  },
  // ["reset", edge]    double-clicks the clip's edge, which puts it back
  //                    where the clip was found
  async reset(walk, arg, s) {
    const { page, watch } = walk;
    const g = await handles(page);
    await page.mouse.dblclick(middle(g[arg]), high(g.track));
    await watch.step("reset", s);
  },
  // ["hold", call]     the next call of this name waits before it reaches
  //                    the Go side, the way a busy machine delivers it
  //                    late, until the next step has looked at what the
  //                    app shows meanwhile
  async hold(walk, arg) {
    const { page } = walk;
    await hold(page, arg);
  },
  // ["fake clock"]     gives the app Playwright's clock, running as the real
  //                    one does, so a timer the app starts from now on can
  //                    be held and run on by hand later, see fakeClock
  async "fake clock"(walk) {
    const { page } = walk;
    await fakeClock(page);
  },
  // ["hold the clock"] holds the app's time where it is, see holdClock in
  //                    bridge.mjs, so a step after it runs the time of a
  //                    toast or a removed row on by hand
  async "hold the clock"(walk) {
    const { page } = walk;
    await holdClock(page);
  },
  // ["tab", to]        presses Tab, and the focus is on to, with its ring
  //                    drawn: "Report", "the field" or "picture i", from 1
  // ["shift tab", to]  the same with Shift+Tab
  async "shift tab"(walk, arg, s, what) {
    const { page } = walk;
    await page.keyboard.press(what === "tab" ? "Tab" : "Shift+Tab");
    const on = await page.evaluate(() => {
      const e = document.activeElement;
      const tiles = [...document.querySelectorAll("li.tile")];
      const box = e?.getBoundingClientRect();
      return {
        name: e?.classList.contains("telling")
          ? "the field"
          : e?.classList.contains("report")
            ? "Report"
            : tiles.some((t) => t.contains(e))
              ? `picture ${tiles.findIndex((t) => t.contains(e)) + 1}`
              : (e?.outerHTML.slice(0, 80) ?? "nothing"),
        ring: !!e?.matches(":focus-visible"),
        seen: !!box && box.width > 0 && box.height > 0,
        // The field is written in, and WebKit rings it however the
        // focus came. Anything else the page moved the focus to rings
        // on a Mac only when the page asked.
        asked: !!e?.isContentEditable || e?.tagName === "TEXTAREA" || (window.__focusAsked?.on === e && window.__focusAsked.ring),
      };
    });
    if (on.name !== arg) walk.wrong = `${what === "tab" ? "Tab" : "Shift+Tab"} put the focus on ${on.name}, not ${arg}`;
    else if (!on.ring || !on.seen) walk.wrong = `the focus is on ${arg} with no ring to see it by`;
    else if (!on.asked) walk.wrong = `the page moved the focus to ${arg} without asking for its ring, which WebKit then does not draw after a click into the field`;
  },
  // ["mark", label]    remembers the engine's captions and pieces
  async mark(walk, arg) {
    const { marks, page, watch } = walk;
    marks[arg] = await engineState(page, watch.at);
  },
  // ["same", label]    the engine's captions and pieces are what they were
  //                    at the mark
  async same(walk, arg) {
    const { marks, page, watch } = walk;
    const now = await engineState(page, watch.at);
    if (!same(now, marks[arg])) walk.wrong = `the captions are not what they were at "${arg}"\n${describe(marks[arg], now)}`;
  },
  // ["differs from", label]
  //                    the engine's captions, their look or the pieces are
  //                    no longer what they were at the mark, so a step
  //                    that was meant to change the clip did
  async "differs from"(walk, arg) {
    const { marks, page, watch } = walk;
    const now = await engineState(page, watch.at);
    if (same(now, marks[arg])) walk.wrong = `the clip is still what it was at "${arg}"\n${describe(marks[arg], now)}`;
  },
};

// Steps that take the same body as the step they are walked by.
steps["undo"] = steps["redo"];
steps["tab"] = steps["shift tab"];

// Until the interface has made a call of name since the call numbered
// after, counted from 0, seen as the call is made, before it is answered.
export function nextCall(page, name, after) {
  return page.evaluate(([name, after]) => window.__nextCall(after, (c) => c.name === name), [name, after]);
}

export const pieceCount = (n) => `${n} piece${n === 1 ? "" : "s"}`;

// A moment the way the clip list says it, clock in lib/api.ts.
export const clockOf = (t) => {
  const h = Math.floor(t / 3600);
  const m = Math.floor((t % 3600) / 60);
  const ss = String(Math.floor(t % 60)).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
};

// Frames a second of the episode on screen.
export const rate = async (page) => (await ask(page, "Source", await episodeOn(page))).fps;

// Where the clip's short is, as the clip says, or empty when it has none.
export async function shortOf(page, at) {
  const clip = (await ask(page, "Clips", at.path)).find((c) => c.plan === at.plan && c.id === at.clip);
  return clip?.short ?? "";
}

export const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";

// The steps that look at how the app moves: the caption blocks and the
// typed word that pop, the bounce over a paused frame, the word that grows
// as it is opened, the card that shakes and the microphone that grows as
// it hears. A sequence with none of them asks for less motion, which the
// app honours by leaving out what it animates, so the settle waits for
// none of it: the pops and the info marks' fade were 179 s of the 1976 s
// all runs took on main. A sequence with one of them, and every random
// walk, walks the app the way it moves on Tim's Mac. A new step that looks
// at motion goes in here, or it passes without ever seeing what it checks.
export const moving = new Set([
  "count pops",
  "pops",
  "pill pops",
  "grow",
  "step onto words and see them pop",
  "step frame by frame and see nothing pop",
  "refused",
  "microphone moves",
]);

// STILL=none walks every sequence with the motion, for a failure that
// might hide behind it.
export function still(seq) {
  return process.env.STILL !== "none" && !seq.steps.some(([what]) => moving.has(what));
}

// Holds files over the app and drops them, the way the runtime reports a
// drag from Finder, its start and where it is, and a drop, see wails-bridge.ts, somewhere left of the
// middle and below it, over whatever the page has there. Says what was
// wrong, or null: the app has to wear its frame while the files are held
// over it, take the drop, and wear no frame after it.
// Lets the held speech hear one piece of audio, and waits until the news
// of the work on the video says something changed for it: how far a job
// heard, its step or its state. Whatever hears next hears it, the search
// or the video heard by itself. A hearing called off between two pieces
// changes its state without hearing one, and the piece is left for the
// next, so a walk allows for a piece more than it counted. With save, what
// is heard so far is saved once the piece is heard, which the bridge
// otherwise only does once the hearing ends.
export async function onePiece(page, url, path, { save = false } = {}) {
  const now = async () =>
    JSON.stringify((await ask(page, "Jobs")).filter((j) => j.episode === path).map((j) => [j.id, j.state, j.step, j.progress?.covered]));
  const before = await now();
  let seen = await news(page);
  await control(url, save ? "/speech?step=1&save" : "/speech?step=1");
  while ((await now()) === before) seen = await nextNews(page, seen);
}

// What the toasts over the window say, one after the other.
// What the toasts over the app say. One that is fading out has gone,
// though it is still in the page for its fade: Svelte marks it inert the
// moment it starts to leave.
export async function toastSays(page) {
  return (await toastsSay(page)).join(" ");
}

// The same, each toast on its own, newest first.
export async function toastsSay(page) {
  return page.evaluate(() => [...document.querySelectorAll(".toast:not([inert]) .text")].map((t) => t.textContent.trim()));
}

// Where what a toast must not move stands: the workspace in front, the
// report's field, and the main column under the bar.
export async function standsAt(page) {
  return page.evaluate(() =>
    [".workspace.front .stage", "main", ".telling"].map((q) => {
      const r = document.querySelector(q)?.getBoundingClientRect();
      return r ? [q, r.x, r.y, r.width, r.height] : [q];
    }),
  );
}

// Whether nothing matches selector within ms.
// Says what moved since standsAt was read, or null.
export async function stillStands(page, before) {
  if (!before) return null;
  const now = await standsAt(page);
  for (let i = 0; i < before.length; i++) {
    if (JSON.stringify(before[i]) !== JSON.stringify(now[i])) return `${before[i][0]} moved from ${before[i].slice(1).join(", ")} to ${now[i].slice(1).join(", ")} when the toast came`;
  }
  return null;
}

export async function dropOn(page, files) {
  const { width, height } = page.viewportSize();
  const [x, y] = [Math.round(width * 0.4), Math.round(height * 0.6)];
  const landing = () => page.evaluate(() => {
      const frame = document.querySelector(".landing");
      return frame ? getComputedStyle(frame).opacity : "nothing, there is no frame";
    });
  // On once the frame's fade has ended, which its animation says, not a
  // deadline: a busy machine read the frame at 0.94 when 300 ms of
  // waiting ran out with the fade not yet drawn to its end, issue #386.
  // A fade cancelled ends the wait too, and the opacity read then says so.
  // An animation that never ends is said, not waited on for good.
  const comes = () =>
    page.evaluate(() => {
      const frame = document.querySelector(".landing");
      if (!frame) return null;
      // Reading the style starts the fade, so getAnimations() has it.
      getComputedStyle(frame).opacity;
      const all = frame.getAnimations();
      const endless = all.find((a) => a.playState === "paused" || a.effect?.getComputedTiming().endTime === Infinity);
      if (endless) return `the frame wears an animation that never ends, ${endless.animationName ?? endless.transitionProperty}`;
      return Promise.all(all.map((a) => a.finished.catch(() => {}))).then(() => null);
    });
  await page.evaluate(([x, y]) => {
    window._wails.handleDragEnter();
    window._wails.handleDragOver(x, y);
  }, [x, y]);
  const endless = await comes();
  if (endless) return endless;
  const held = await landing();
  if (held !== "1") return `the app wore no frame while a file was held over it, its opacity was ${held}`;
  const took = await page.evaluate(([f, x, y]) => window._wails.handlePlatformFileDrop(f, x, y), [files, x, y]);
  if (!took) return `the drop at ${x}, ${y} landed on no drop target`;
  const ends = await comes();
  if (ends) return ends;
  const after = await landing();
  if (after !== "0") return `the app still wore its frame after the drop, its opacity was ${after}`;
  return null;
}

// What is wrong with how the crop frame is drawn over the video preview,
// or null, read off a screenshot. The picture under it is made white for
// the look, so the shade over it is a grey of its own and a speck of the
// picture it missed is white, and a note over the picture, like the one
// the bridge's sound gives, is put away: the frame, the shade and the
// viewer are drawn as they are. See ["frame whole"].
// The walks of issue #383 wait on what the app tells, never on the clock.
// Until the workspace in front has asked the captions of every card and
// read ahead what it reads for them, see rested in Episode.svelte.
export function restAhead(page) {
  return page.evaluate(() => window.__readAheadRested());
}

// The keys of the cards of the clips in the clip list, not of one on its
// way.
function cardKeys(page) {
  return page.evaluate(() => [...document.querySelectorAll("aside ol li[data-key]:not(.next):not(.ghost)")].map((li) => li.dataset.key));
}

// Does make, then the keys of the n cards that are not among was, once the
// clip list shows them.
export async function newCards(page, n, make) {
  const was = await cardKeys(page);
  const shown = page.evaluate(
    ({ was, n }) =>
      new Promise((done) => {
        const look = () => [...document.querySelectorAll("aside ol li[data-key]:not(.next):not(.ghost)")].map((li) => li.dataset.key).filter((k) => !was.includes(k));
        const o = new MutationObserver(() => {
          const keys = look();
          if (keys.length < n) return;
          o.disconnect();
          done(keys);
        });
        o.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ["data-key", "class"] });
        if (look().length >= n) {
          o.disconnect();
          done(look());
        }
      }),
    { was, n },
  );
  await make();
  return shown;
}

// Makes a clip in each shot of the video of two cameras with Mark In, so
// two clips have their crops far apart, each starting on a caption, and
// waits until the clip list shows them. Every clip the engine has, or what went wrong.
export async function shotClips(page) {
  const path = await episodeOn(page);
  const keys = await newCards(page, 2, async () => {
    for (const at of [8, 86]) await ask(page, "MakeClip", path, at, false);
  });
  const all = await ask(page, "Clips", path);
  const clips = all.filter((c) => keys.includes(c.key));
  if (clips.length < 2) return `Mark In made ${clips.length} clips the engine has, not 2`;
  const lefts = new Set(clips.map((c) => Math.round(c.cropLefts?.[0] ?? -1)));
  if (lefts.size < 2) return `every clip has its crop at ${[...lefts][0]}, so the crop frame cannot be seen to move`;
  return all;
}

// Has the page tell what the video preview shows, window.__look.
export async function canLook(page) {
  await page.evaluate(() => {
    // What the video preview shows now: the crop frame's place, and a hash
    // of every pixel of the canvas, which is the picture and the captions
    // libass drew over it. And whether a caption stands at the playhead,
    // so the walk knows it had captions to see.
    window.__look = () => {
      const c = document.querySelector(".workspace.front .screen canvas") ?? document.querySelector(".screen canvas");
      const frame = document.querySelector(".atop .frame");
      const crop = frame ? `${frame.style.left} ${frame.style.width}` : "none";
      if (!c?.width || !c.height) return { crop, canvas: 0, captions: false };
      const s = (window.__lookCanvas ??= document.createElement("canvas"));
      s.width = c.width;
      s.height = c.height;
      const t = s.getContext("2d", { willReadFrequently: true });
      t.drawImage(c, 0, 0);
      const d = t.getImageData(0, 0, c.width, c.height).data;
      let h = 2166136261;
      for (let i = 0; i < d.length; i += 4) h = Math.imul(h ^ (d[i] | (d[i + 1] << 8) | (d[i + 2] << 16)), 16777619) >>> 0;
      return { crop, canvas: h, captions: !!document.querySelector(".screen .captions") };
    };
  });
}

// Clicks the card of key and reads what the video preview shows on every
// animation frame from the click, until it has changed, the workspace has
// read ahead again what the click asks it to and no layer of captions is
// on its way, and ten frames after. What it showed, with how many
// milliseconds after the click, and of the clip's first frame, at the
// moment start, whether it was handed over from the frames kept.
export function clickCard(page, key, start) {
  return page.evaluate(
    ({ key, start }) =>
      new Promise((done) => {
        const frames = window.__appFrames;
        const first = start === null ? null : window.__frameQueue.firstFrameAt(start);
        const kept = frames.stats.kept;
        // open says whether the Go side's answers about captions were let
        // through yet, see pickCards.
        const looks = [{ ...window.__look(), t: 0, open: !!window.__captionsOpen }];
        const t0 = performance.now();
        document.querySelector(`aside ol li[data-key="${CSS.escape(key)}"] button.pick`).click();
        let rested = false;
        void window.__readAheadRested().then(() => (rested = true));
        let after = 0;
        const next = () => {
          const t = performance.now() - t0;
          const l = { ...window.__look(), t, open: !!window.__captionsOpen };
          const was = looks[looks.length - 1];
          looks.push(l);
          const changed = looks.some((x, j) => j > 0 && (x.crop !== looks[j - 1].crop || x.canvas !== looks[j - 1].canvas));
          window.__clickChanged ||= changed;
          // The walk waiting for the frame to wait for what lies over it,
          // or for the click to change the video preview, is told here.
          const gate = window.__gate;
          if (gate && (changed || window.__frameQueue.stats.waitedForDress > gate.was)) {
            window.__gate = null;
            gate.done();
          }
          if (l.crop !== was.crop || l.canvas !== was.canvas) after = 0;
          else if (changed && rested && !window.__captionLayers.busy) after++;
          if (after < 10) return requestAnimationFrame(next);
          // The frames handed over from the kept ones since the click.
          const since = frames.stats.keptRanks.slice(-(frames.stats.kept - kept) || frames.stats.keptRanks.length);
          done({
            first,
            kept: first !== null && frames.stats.kept > kept && since.some((r) => Math.abs(frames.ptsOf(r) - first) < 1e-6),
            looks,
          });
        };
        requestAnimationFrame(next);
      }),
    { key, start },
  );
}

// The animation frames on which what the video preview shows changed.
export function changesOf(looks) {
  return looks.filter((l, j) => j > 0 && (l.crop !== looks[j - 1].crop || l.canvas !== looks[j - 1].canvas));
}

// Clicks the card of key, of the clip at start, and checks it was put up
// from what was read ahead, in one change. What went wrong, or null.
export async function clickedFromAhead(page, key, start, card) {
  // The captions the page asks the Go side to draw, by the moment.
  const drawn = [];
  const asked = (r) => r.url().includes("/frames/captions?") && drawn.push(new URL(r.url()).searchParams.get("at"));
  page.on("request", asked);
  try {
    const got = await clickCard(page, key, start);
    const changes = changesOf(got.looks);
    const late = drawn.filter((at) => Math.abs(Number(at) - got.first) < 1e-6).length;
    const problems = [];
    if (!got.kept) problems.push(`${card}: its first frame, at ${got.first.toFixed(3)} s, was decoded after the click, not read ahead`);
    if (late) problems.push(`${card}: its captions were drawn after the click, not read ahead`);
    if (changes.length !== 1) problems.push(`${card}: the video preview changed ${changes.length} times, not once`);
    if (problems.length) return problems.join("\n");
    console.log(`        ${card}: put up from what was read ahead, ${Math.round(changes[0].t)} ms after the click`);
    return null;
  } finally {
    page.off("request", asked);
  }
}

// Whether one colour is the other at half its strength, the colours as
// getComputedStyle writes them, rgb() or color(srgb).
export function halfOf(half, whole) {
  const rgba = (c) => {
    const m = (c ?? "").match(/rgba?\(([^)]*)\)/);
    if (m) {
      const [r, g, b, a = 1] = m[1].split(",").map(Number);
      return [r, g, b, a];
    }
    const n = (c ?? "").match(/color\(srgb ([\d.]+) ([\d.]+) ([\d.]+)(?: \/ ([\d.]+))?\)/);
    return n ? [...n.slice(1, 4).map((v) => Math.round(Number(v) * 255)), n[4] === undefined ? 1 : Number(n[4])] : [];
  };
  const [h, w] = [rgba(half), rgba(whole)];
  return h.length === 4 && w.length === 4 && h.slice(0, 3).join() === w.slice(0, 3).join() && Math.abs(h[3] - w[3] / 2) < 0.02;
}
