// The steps of the sequences about the caption box and the captions drawn
// over the video preview: words, blocks, pills, fonts and styles. Each is
// said above it, a list with its name first. walkSequence in
// ../sequences.mjs walks it, and hands it the walk so far, the step's
// argument, what the caption box showed before it and the step's name. A
// sequence of any folder can take it. Below them the helpers only these
// steps use. See docs/TESTING.md.
import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { basename } from "node:path";
import { ask, clickOnTrack, control, cropPixels, engineState, episodeOn, fromSidebar, handles, high, news, nextNews, nextPaint, open, pageUntil, playUntil, seekTo, settle, shortFramesFrom, shown, shownOnPlayhead, xOf } from "../bridge.mjs";
import { canLook, clickCard, clickedFromAhead, clock, rate, restAhead, shortOf, shotClips, still, url } from "../common/steps.mjs";
import { describe } from "../rules.mjs";

export const steps = {
  // ["frame", word]    walks the playhead with Shift and → until the word is
  //                    in the caption box, clicks it and leaves it with
  //                    Escape, so it wears the keyboard's frame
  async frame(walk, arg, s) {
    const { page, stepOn, watch, word } = walk;
    for (let i = 0; i < 80 && !(await word(arg).count()); i++) await stepOn();
    await word(arg).click();
    await page.keyboard.press("Escape");
    await watch.step("frame", s);
  },
  // ["block", word]    clicks the block on the clip timeline of the caption
  //                    the engine has the word in, which picks it
  async block(walk, arg, s) {
    const { page, watch } = walk;
    const now = await engineState(page, watch.at);
    const cue = now.captions.find((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(arg))));
    if (!cue) {
      walk.wrong = `no caption has "${arg}"`;
      return;
    }
    await page.locator(`.clip-timeline .caption[data-first="${cue.first}"]`).first().click();
    await watch.step("block", s);
  },
  // ["every block"]    clicks every caption block on the clip timeline in
  //                    turn, and each lights a word of its caption
  async "every block"(walk) {
    const { page, watch } = walk;
    // Each caption block clicked in turn, and the caption box read for
    // the word it lights.
    const now = await engineState(page, watch.at);
    const firsts = [...new Set(await page.locator(".clip-timeline .caption").evaluateAll((b) => b.map((x) => x.dataset.first)))];
    for (const first of firsts) {
      const cue = now.captions.find((c) => String(c.first) === first);
      const words = cue?.lines.flatMap((l) => l.words) ?? [];
      const lost = await captionsLost(page);
      await page.locator(`.clip-timeline .caption[data-first="${first}"]`).first().click();
      // Once the frame the click put the playhead on is up with its
      // captions, since the word the last click lit stays until the
      // frame of this one is drawn, and the caption box lights the word
      // drawn on it.
      await settle(page);
      if (!(await shownOnPlayhead(page, { captions: true, lost }))) {
        walk.wrong = `after a click on the caption block of "${words.map((w) => w.text).join(" ")}", a layer of captions was given up on`;
        break;
      }
      // A word is known by where it starts as well as by what it says:
      // two captions can share a word, and the word the click before
      // lit then passed for this caption's.
      const ours = words.map((w) => [w.text, w.start]);
      const ofOurs = (ours) => {
        const el = document.querySelector(".captions .word.now");
        const at = Number(el?.dataset.at);
        return !!el && ours.some(([text, start]) => text === el.textContent && Math.abs(start - at) < 0.0005);
      };
      if (!(await page.evaluate(ofOurs, ours))) {
        const lit = await page.evaluate(() => {
          const el = document.querySelector(".captions .word.now");
          return el ? `"${el.textContent}" at ${el.dataset.at}` : null;
        });
        walk.wrong = `a click on the caption block of "${words.map((w) => `${w.text} at ${w.start}`).join(" ")}" lit ${lit ?? "no word"}`;
        break;
      }
    }
    console.log(`    ${firsts.length} blocks clicked`);
  },
  async "edge to word"(walk, arg, s, what) {
    const { marks, page, watch } = walk;
    // An edge of the caption holding a word dragged: in, just over
    // the middle of the word at that edge, out, back past where that
    // word was at "start", a little in, short of any word's middle, or
    // with shift to a little past where the word after the first
    // starts, or before where the word before the last ends, nearer
    // that than any other, where it has to land.
    const [edge, text] = arg;
    const now = await engineState(page, watch.at);
    const was = what === "edge out" ? marks.start : now;
    const i = was.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
    if (i < 0) {
      walk.wrong = `no caption has "${text}"`;
      return;
    }
    const words = was.captions[i].lines.flatMap((l) => l.words);
    const w = edge === "end" ? words.at(-1) : words[0];
    const mid = (w.start + w.end) / 2;
    const into = (t) => {
      let sum = 0;
      for (const p of now.segments) {
        if (t <= sum + p.end - p.start) return p.start + t - sum;
        sum += p.end - p.start;
      }
      return now.segments.at(-1).end;
    };
    const c = now.captions[i];
    const from = edge === "end" ? c.end : c.start;
    let to;
    let landing = null;
    if (what === "edge in") to = edge === "end" ? (w.start + mid) / 2 : (mid + w.end) / 2;
    else if (what === "edge out") to = edge === "end" ? was.captions[i].end : was.captions[i].start;
    else if (what === "edge to word") {
      const next = edge === "end" ? words.at(-2) : words[1];
      if (!next) {
        walk.wrong = `the caption with "${text}" has one word`;
        return;
      }
      landing = edge === "end" ? next.end : next.start;
      to = landing + (edge === "end" ? -0.04 : 0.04);
    } else to = edge === "end" ? Math.max(mid + 0.05, from - 0.2) : Math.min(mid - 0.05, from + 0.2);
    const label = `When caption ${i + 1} ${edge === "end" ? "goes" : "appears"}`;
    const box = await page.locator(`.clip-timeline [aria-label="${label}"]`).first().boundingBox();
    if (!box) {
      walk.wrong = `there is no edge "${label}"`;
      return;
    }
    // From the middle of the edge's handle, as far as from the edge to
    // the moment, since the timeline reads how far the hand went from
    // the press. Held there a moment before it is let go, so what the
    // timeline draws under the hand can be held against what it saves:
    // the edge is where the drag showed it, and does not jump when let
    // go.
    const x = box.x + box.width / 2;
    const y = box.y + box.height / 2;
    const target = x + (await xOf(page, into(to))) - (await xOf(page, into(from)));
    if (landing !== null) await page.keyboard.down("Shift");
    await page.mouse.move(x, y);
    await page.mouse.down();
    for (let k = 1; k <= 6; k++) {
      await page.mouse.move(x + ((target - x) * k) / 6, y);
      await nextPaint(page);
    }
    await settle(page);
    const handle = page.locator(`.clip-timeline [aria-label="${label}"]`).first();
    const held = await handle.boundingBox();
    await page.mouse.up();
    if (landing !== null) await page.keyboard.up("Shift");
    await watch.step(what, s);
    const left = await handle.boundingBox();
    if (!held || !left || Math.abs(left.x - held.x) > 1) {
      walk.wrong = `the edge was drawn at ${held?.x.toFixed(1)} px while dragged and at ${left?.x.toFixed(1)} px once let go`;
      return;
    }
    if (landing !== null) {
      const after = await engineState(page, watch.at);
      const c = after.captions[i];
      const landed = edge === "end" ? c?.end : c?.start;
      if (landed === undefined || Math.abs(landed - landing) > 0.002) {
        walk.wrong = `dragged with shift to ${to.toFixed(3)}, the ${edge} of the caption is at ${landed?.toFixed(3)}, not on the word at ${landing.toFixed(3)}`;
      }
    }
  },
  async pops(walk, arg, s, what) {
    const { page } = walk;
    // How often a caption block on the clip timeline pops, counted from
    // "count pops" on. The playhead stands still, so any pop is one an
    // edit made.
    if (what === "count pops") {
      await settle(page);
      await page.evaluate(() => {
        window.__pops = 0;
        window.__pillPops = 0;
        if (!window.__popsHeard) {
          window.__popsHeard = true;
          document.addEventListener("animationstart", (e) => {
            if (e.animationName !== "pop") return;
            if (e.target.closest?.(".clip-timeline .caption")) window.__pops++;
            if (e.target.closest?.(".screen .captions")) window.__pillPops++;
          });
        }
      });
    } else {
      await settle(page);
      const n = await page.evaluate(() => window.__pops);
      if (n !== arg) walk.wrong = `a caption block popped ${n} times, ${arg} wanted`;
    }
  },
  // ["pill pops", n]   the pill round the lit word in the video preview
  //                    popped n times since "count pops"
  async "pill pops"(walk, arg) {
    const { page } = walk;
    await settle(page);
    const n = await page.evaluate(() => window.__pillPops);
    if (n !== arg) walk.wrong = `the pill in the video preview popped ${n} times, ${arg} wanted`;
  },
  async "edge put back"(walk, arg, s, what) {
    const { page, watch } = walk;
    // A double-click on an edge of the caption holding a word.
    const [edge, text] = arg;
    const now = await engineState(page, watch.at);
    const i = now.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
    const label = `When caption ${i + 1} ${edge === "end" ? "goes" : "appears"}`;
    const box = await page.locator(`.clip-timeline [aria-label="${label}"]`).first().boundingBox();
    if (i < 0 || !box) {
      walk.wrong = `there is no edge "${label}"`;
      return;
    }
    await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
    await watch.step(what, s);
  },
  async "drawn in"(walk, arg) {
    const { marks, page, watch } = walk;
    // The captions read as they did at a mark, and the caption holding
    // a word draws in at its first or last edge from where it was at
    // "start", never out, its other edge where it was.
    const [label, text, which] = arg;
    const read = (caps) => JSON.stringify(caps.captions.map((c) => c.lines.flatMap((l) => l.words.map((w) => w.text))));
    const now = await engineState(page, watch.at);
    const i = marks.start.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
    const was = marks.start.captions[i];
    const c = now.captions[i];
    const near = (a, b) => Math.abs(a - b) < 1e-3;
    const inward = which === "last" ? near(c.start, was.start) && c.end < was.end - 1e-3 : near(c.end, was.end) && c.start > was.start + 1e-3;
    if (read(now) !== read(marks[label]) || !inward) {
      walk.wrong = `the caption with "${text}" did not draw in at its ${which} edge as it did at "${label}"\n${describe(marks[label], now)}`;
    }
  },
  async "gone first"(walk, arg, s, what) {
    const { marks, page, watch } = walk;
    // The caption holding a word at a mark reads as it did with its
    // last or first word left out, and every other caption as it did.
    const [label, text] = arg;
    const read = (c) => c.lines.flatMap((l) => l.words.map((w) => w.text));
    const was = marks[label];
    const now = await engineState(page, watch.at);
    const i = was.captions.findIndex((c) => read(c).some((w) => w.includes(text)));
    const want = what === "gone last" ? read(was.captions[i]).slice(0, -1) : read(was.captions[i]).slice(1);
    if (now.captions.length !== was.captions.length || read(now.captions[i]).join(" ") !== want.join(" ")) {
      walk.wrong = `the caption with "${text}" did not lose its ${what.slice(5)} word\n${describe(was, now)}`;
    }
  },
  async "remove edge word"(walk, arg, s) {
    const { page, stepOn, watch, word } = walk;
    // The first or last word of the caption holding a word framed and
    // removed in the caption box.
    const [which, text] = arg;
    const now = await engineState(page, watch.at);
    const c = now.captions.find((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
    const all = c.lines.flatMap((l) => l.words);
    const edgeWord = (which === "last" ? all.at(-1) : all[0]).text;
    for (let i = 0; i < 80 && !(await word(edgeWord).count()); i++) await stepOn();
    await word(edgeWord).click();
    await page.keyboard.press("Escape");
    await watch.step("frame", s);
    await page.keyboard.press("Backspace");
    await watch.step("press", await shown(page));
  },
  async others(walk, arg) {
    const { marks, page, watch } = walk;
    // Every caption but the one holding a word at a mark appears and
    // goes when it did then.
    const [label, text] = arg;
    const was = marks[label];
    const now = await engineState(page, watch.at);
    const mine = was.captions.find((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
    const span = (c) => `${c.start.toFixed(3)}-${c.end.toFixed(3)}`;
    const moved = was.captions
      .filter((c) => c !== mine)
      .filter((c) => {
        const n = now.captions.find((d) => Math.abs(d.first - c.first) < 1e-6);
        return !n || span(n) !== span(c);
      });
    if (moved.length) {
      walk.wrong = `captions beside the one with "${text}" changed: ${moved.map(span).join(" ")}\nthen ${was.captions.map(span).join(" ")}\nnow  ${now.captions.map(span).join(" ")}`;
    }
  },
  // ["holds", word]    a caption the engine has holds the word
  async holds(walk, arg) {
    const { marks, page, watch } = walk;
    const now = await engineState(page, watch.at);
    if (!now.captions.some((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(arg))))) {
      walk.wrong = `no caption has "${arg}"\n${describe(marks.start ?? now, now)}`;
    }
  },
  // ["gone", word]     no caption the engine has holds the word
  async gone(walk, arg) {
    const { marks, page, watch } = walk;
    const now = await engineState(page, watch.at);
    if (now.captions.some((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(arg))))) {
      walk.wrong = `a caption still has "${arg}"\n${describe(marks.start ?? now, now)}`;
    }
  },
  // ["fewer", label, n] the engine's captions hold n words fewer than at
  //                    the mark, and as many captions
  async fewer(walk, arg) {
    const { marks, page, watch } = walk;
    const [label, n] = arg;
    const count = (caps) => caps.captions.flatMap((c) => c.lines.flatMap((l) => l.words)).length;
    const now = await engineState(page, watch.at);
    const was = marks[label];
    if (count(was) - count(now) !== n || now.captions.length !== was.captions.length) {
      walk.wrong = `${count(was) - count(now)} words and ${was.captions.length - now.captions.length} captions fewer than at "${label}", not ${n} and none\n${describe(was, now)}`;
    }
  },
  // ["blocks"]         the clip timeline draws a block for every caption the
  //                    engine has, and for no other
  async blocks(walk) {
    const { page, watch } = walk;
    const now = await engineState(page, watch.at);
    const want = [...new Set(now.captions.map((c) => String(c.first)))].join(" ");
    await settle(page);
    const drawn = await page.evaluate(() =>
      [...new Set([...document.querySelectorAll(".clip-timeline .caption")].map((b) => b.dataset.first))].join(" "),
    );
    if (drawn !== want) walk.wrong = `the clip timeline draws captions at ${drawn}, the engine has them at ${want}`;
  },
  // ["click", word]    clicks the word, which opens it
  async click(walk, arg, s) {
    const { watch, word } = walk;
    await word(arg).click();
    await watch.step("click", s);
  },
  // ["open in place", word]
  //                    clicks the word and leaves it with Escape, and sees
  //                    no word of the caption box move, libass draw the
  //                    caption without that word's letters while it is
  //                    open and with the rest where they were, its letters
  //                    on the rows libass drew them on, and nothing pop
  //                    when it is left
  async "open in place"(walk, arg, s) {
    const { captioned, letters, page, settled, watch, word } = walk;
    // The words where they stand in the caption box, and the white of
    // the letters libass draws on the canvas inside the word and round
    // it, with the word shut, open, and shut again.
    const look = () =>
      page.evaluate((text) => {
        const words = [...document.querySelectorAll(".captions .word")];
        const canvas = document.querySelector(".screen canvas");
        const it = words.find((w) => w.textContent === text);
        if (!it || !canvas?.width) return null;
        const c = canvas.getBoundingClientRect();
        const sx = canvas.width / c.width;
        const sy = canvas.height / c.height;
        const s = document.createElement("canvas");
        s.width = canvas.width;
        s.height = canvas.height;
        const g = s.getContext("2d", { willReadFrequently: true });
        g.drawImage(canvas, 0, 0);
        const f = it.getBoundingClientRect();
        const x0 = Math.floor((f.left - c.left) * sx);
        const x1 = Math.ceil((f.right - c.left) * sx);
        const y0 = Math.floor((f.top - c.top) * sy);
        const y1 = Math.ceil((f.bottom - c.top) * sy);
        // Near white, the letters, inside the word and in a band as
        // wide again on each side of it.
        const white = (a, b) => {
          const d = g.getImageData(a, y0, Math.max(1, b - a), Math.max(1, y1 - y0)).data;
          let n = 0;
          for (let i = 0; i < d.length; i += 4) if (d[i] > 220 && d[i + 1] > 220 && d[i + 2] > 220) n++;
          return n;
        };
        const w = x1 - x0;
        return {
          rects: words.map((e) => {
            const r = e.getBoundingClientRect();
            return [r.left, r.top, r.width, r.height].map((v) => Math.round(v * 10) / 10);
          }),
          inside: white(x0, x1),
          around: white(Math.max(0, x0 - w), x0) + white(x1, Math.min(canvas.width, x1 + w)),
          popped: window.__captionLayers?.stats.popped ?? 0,
        };
      }, arg);
    if (!(await captioned())) {
      walk.wrong = `the captions over the frame of "${arg}" did not come`;
      return;
    }
    await settled();
    const shut = await look();
    if (!shut) {
      walk.wrong = `"${arg}" is not in the caption box`;
      return;
    }
    // The rows of the word's letters on screen, libass's while it is
    // shut and the browser's while it is open, without the caret.
    const b = await word(arg).boundingBox();
    const place = b && { left: b.x, top: b.y, width: b.width, height: b.height };
    const inked = async () => {
      await word(arg).evaluate((e) => (e.style.caretColor = "transparent"));
      const got = await letters(place);
      await word(arg).evaluate((e) => (e.style.caretColor = ""));
      return got;
    };
    const drawn = await inked();
    await word(arg).click();
    await watch.step("click", s);
    await settled();
    const open = await look();
    const typed = await inked();
    console.log(`    letters from row ${drawn?.top} to ${drawn?.bottom} shut, ${typed?.top} to ${typed?.bottom} open`);
    await page.keyboard.press("Escape");
    await settled();
    const again = await look();
    const moved = (a, b) => a.rects.findIndex((r, i) => r.some((v, k) => Math.abs(v - (b.rects[i]?.[k] ?? NaN)) > 0.6));
    console.log(`    shut ${shut.inside} white inside, ${shut.around} round, open ${open?.inside} and ${open?.around}`);
    if (!open || !again) walk.wrong = `"${arg}" left the caption box`;
    else if (moved(shut, open) >= 0) walk.wrong = `opening "${arg}" moved word ${moved(shut, open) + 1} of the caption box from ${shut.rects[moved(shut, open)]} to ${open.rects[moved(shut, open)]}`;
    else if (moved(shut, again) >= 0) walk.wrong = `leaving "${arg}" moved word ${moved(shut, again) + 1} of the caption box`;
    else if (shut.inside < 20 || open.inside > shut.inside / 10)
      walk.wrong = `libass drew ${shut.inside} white pixels of "${arg}" shut and ${open.inside} open, where the caption box draws it`;
    else if (Math.abs(open.around - shut.around) > shut.around / 20 + 5)
      walk.wrong = `round "${arg}" libass drew ${shut.around} white pixels shut and ${open.around} open: the caption moved`;
    else if (again.popped !== shut.popped) walk.wrong = `leaving "${arg}" popped the word lit`;
    // Found by Tim on #201: the word opened dropped a few pixels,
    // set in the middle of its pill rather than on libass's baseline.
    else if (!drawn || !typed) walk.wrong = `the letters of "${arg}" were not found on screen`;
    else if (Math.abs(typed.top - drawn.top) > 1 || Math.abs(typed.bottom - drawn.bottom) > 1)
      walk.wrong = `"${arg}" opened stands from row ${typed.top} to ${typed.bottom}, where libass drew it from ${drawn.top} to ${drawn.bottom}`;
  },
  // ["one pill", word] the word opened while another is lit sits on the
  //                    colour of the pill libass draws round it lit, with no
  //                    ring of another shade round it and no focus ring
  async "one pill"(walk, arg, s) {
    const { letters, page, settled, watch, word } = walk;
    // The colour of the pill libass draws round the word lit, and the
    // colour behind its letters opened while the next word is lit.
    const it = word(arg);
    for (let i = 0; i < 20 && !(await it.evaluate((e) => e.classList.contains("now"))); i++) {
      await page.keyboard.press("Shift+ArrowRight");
      await settled();
    }
    await watch.step("press", s);
    await settled();
    const box = async () => {
      const r = await it.boundingBox();
      return r && { left: r.x + 1, top: r.y + 1, width: r.width - 2, height: r.height - 2 };
    };
    const shut = await box();
    const lit = shut && (await letters(shut));
    await page.keyboard.press("Shift+ArrowRight");
    await watch.step("press", s);
    await settled();
    await it.click();
    await watch.step("click", s);
    await settled();
    await it.evaluate((e) => (e.style.caretColor = "transparent"));
    const open = await box();
    // The whole box, its edge where a ring would be drawn included.
    const typed = open && (await letters({ ...open, left: open.left - 1, top: open.top - 1, width: open.width + 2, height: open.height + 2 }));
    const state = await it.evaluate((e) => ({
      open: e.classList.contains("fixing"),
      lit: e.classList.contains("now"),
      shadow: getComputedStyle(e).boxShadow,
      outline: `${getComputedStyle(e).outlineStyle} ${getComputedStyle(e).outlineWidth}`,
    }));
    await it.evaluate((e) => (e.style.caretColor = ""));
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    console.log(`    lit on ${lit?.pill}, typed unlit on ${typed?.pill}, ring ${state.shadow}, outline ${state.outline}`);
    const rgb = (c) => (c ?? "").split(",").map(Number);
    const unlike = (a, b) => rgb(a).length !== 3 || rgb(a).some((v, i) => Math.abs(v - rgb(b)[i]) > 4);
    if (!lit?.pill) walk.wrong = `libass drew no pill round "${arg}" lit`;
    else if (!state.open || state.lit) walk.wrong = `"${arg}" is ${state.open ? "" : "not "}open and ${state.lit ? "" : "not "}lit`;
    else if (!typed) walk.wrong = `the letters typed into "${arg}" were not found on screen`;
    else if (unlike(lit.pill, typed.pill))
      walk.wrong = `"${arg}" typed unlit sits on ${typed.pill || "no colour"}, and libass's pill is ${lit.pill}`;
    else if (/rgb|color\(/.test(state.shadow.replaceAll("rgba(0, 0, 0, 0)", "")))
      walk.wrong = `"${arg}" typed has a ring round its pill, ${state.shadow}`;
    // The focus ring the app gives a control, a pixel off the pill,
    // which left a dark line between the two.
    else if (!state.outline.startsWith("none")) walk.wrong = `"${arg}" typed has a focus ring round its pill, ${state.outline}`;
  },
  // ["grow", word]     walks the playhead with Shift and → until the word
  //                    is lit and a frame on with →, into its pop, clicks
  //                    the word, types after it and leaves it with Escape,
  //                    and sees it open where it stood, at the scale it
  //                    was drawn at, and its box and pill grow round their
  //                    middle by what was typed
  async grow(walk, arg, s) {
    const { letters, page, settled, watch, word } = walk;
    // The word as the screen shows it: its box, the scale it is
    // drawn at, whether it is open and lit, the width of its letters
    // and the colour behind them.
    const look = () =>
      page.evaluate(() => {
        const it = document.querySelector(".captions .word.fixing") ?? window.__growing;
        if (!it?.isConnected) return null;
        const r = it.getBoundingClientRect();
        const letters = document.createRange();
        letters.selectNodeContents(it);
        const style = getComputedStyle(it);
        return {
          left: r.left,
          width: r.width,
          top: r.top,
          height: r.height,
          letters: letters.getBoundingClientRect().width,
          pop: style.getPropertyValue("--pop") || getComputedStyle(it.closest(".captions")).getPropertyValue("--pop"),
          open: it.classList.contains("fixing"),
          lit: it.classList.contains("now"),
          behind: style.backgroundColor,
        };
      });
    const it = word(arg);
    for (let i = 0; i < 20 && !(await it.evaluate((e) => e.classList.contains("now"))); i++) {
      await page.keyboard.press("Shift+ArrowRight");
      await settled();
    }
    await page.keyboard.press("ArrowRight");
    await watch.step("press", s);
    await it.evaluate((e) => (window.__growing = e));
    await settled();
    const shut = await look();
    // Inside the pill, which lies over the words beside it.
    const around = shut && { left: shut.left + 1, top: shut.top + 1, width: shut.width - 2, height: shut.height - 2 };
    const drawn = around && (await letters(around));
    await it.click();
    await watch.step("click", s);
    await settled();
    const open = await look();
    // Without the caret, which is the colour of the letters and would
    // count as one.
    await it.evaluate((e) => (e.style.caretColor = "transparent"));
    const typing = around && (await letters(around));
    await it.evaluate((e) => (e.style.caretColor = ""));
    console.log(`    libass's letters ${JSON.stringify(drawn)}, the caption box's ${JSON.stringify(typing)}`);
    const rgb = (c) => (c ?? "").split(",").map(Number);
    const unlike = (a, b) => rgb(a).length !== 3 || rgb(a).some((v, i) => Math.abs(v - rgb(b)[i]) > 4);
    await page.keyboard.press("End");
    await page.keyboard.type("ungen");
    await watch.step("type", s);
    await settled();
    const typed = await look();
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    const mid = (l) => l.left + l.width / 2;
    const off = (a, b) => ["left", "width", "top", "height"].find((k) => Math.abs(a[k] - b[k]) > 0.6);
    console.log(
      `    scale ${shut?.pop}, ${shut?.lit ? "lit" : "not lit"}, ${shut?.width.toFixed(1)} px shut, ${open?.width.toFixed(1)} open, ${typed?.width.toFixed(1)} typed round ${typed?.letters.toFixed(1)} px of letters`,
    );
    if (!shut || !open || !typed) walk.wrong = `"${arg}" left the caption box`;
    else if (!shut.lit || !(Number(shut.pop) > 1)) walk.wrong = `"${arg}" is not lit on a frame of its pop, scale ${shut.pop}`;
    else if (!open.open) walk.wrong = `"${arg}" did not open`;
    else if (!drawn || !typing) walk.wrong = `"${arg}" shows no letters ${drawn ? "open" : "shut"}`
    // The Mac thickened the face in the caption box where libass did not,
    // and the box was drawn without the look the frame has.
    else if (Math.abs(drawn.width - typing.width) > 2 || Math.abs(drawn.height - typing.height) > 1)
      walk.wrong = `"${arg}" opened is ${typing.width} by ${typing.height} px of letters and ${drawn.width} by ${drawn.height} shut`;
    else if (unlike(drawn.pill, typing.pill))
      walk.wrong = `"${arg}" opened sits on a pill of ${typing.pill} and the pill shut is ${drawn.pill}`;
    else if (unlike(drawn.ink, typing.ink))
      walk.wrong = `"${arg}" opened is written in ${typing.ink} and shut in ${drawn.ink}`;
    else if (off(shut, open))
      walk.wrong = `opening "${arg}" moved its ${off(shut, open)} from ${shut[off(shut, open)].toFixed(1)} to ${open[off(shut, open)].toFixed(1)} px`;
    else if (typed.width < open.width + (typed.letters - open.letters) - 1.5)
      walk.wrong = `"${arg}" typed longer by ${(typed.letters - open.letters).toFixed(1)} px of letters grew its box by ${(typed.width - open.width).toFixed(1)} px`;
    else if (typed.letters > typed.width) walk.wrong = `the letters typed run out of their box, ${typed.letters.toFixed(1)} px in ${typed.width.toFixed(1)}`;
    else if (Math.abs(mid(typed) - mid(open)) > 0.6)
      walk.wrong = `"${arg}" grew round ${mid(typed).toFixed(1)} px and not round the middle of its pill at ${mid(open).toFixed(1)}`;
    else if (Math.abs(typed.top - open.top) > 0.6 || Math.abs(typed.height - open.height) > 0.6)
      walk.wrong = `typing into "${arg}" moved it up or down`;
    else if (typed.lit && /rgba\(0, 0, 0, 0\)|transparent/.test(typed.behind))
      walk.wrong = `"${arg}" is the word lit and has no pill behind what is typed`;
  },
  // ["click heard"]    clicks into the first word still being heard and
  //                    sees it settled there, with the cursor in it
  async "click heard"(walk) {
    const { page } = walk;
    // Inside the first word still being heard that has three letters
    // or more, just after its second letter, so the X typed there has
    // letters of the word on either side. A click a fixed 12 px in
    // landed after a short word like "er" on CI, not in it.
    // What is heard is held back from the window from the reading of
    // the word to the click, see __pauseEvents in wails-bridge.ts. The
    // stand-in speech model hears new words each time, and on CI
    // "erzählt," was read and "er wäre" clicked, which put the X
    // after "er", in one run of three on a busy machine.
    await page.evaluate(() => window.__pauseEvents(true));
    const at = await page.evaluate(() => {
      const mark = document.querySelector(".telling .hearing");
      const text = mark?.firstChild;
      if (!text) return null;
      const word = /\p{L}{3,}/u.exec(text.textContent);
      if (!word) return null;
      const third = document.createRange();
      third.setStart(text, word.index + 2);
      third.setEnd(text, word.index + 3);
      const r = third.getClientRects()[0];
      return r && { x: r.left + r.width * 0.25, y: r.top + r.height / 2 };
    });
    if (!at) {
      await page.evaluate(() => window.__pauseEvents(false));
      walk.wrong = "nothing was being heard to click into";
      return;
    }
    await page.mouse.click(at.x, at.y);
    // Read before what waited is let through, which may be heard
    // words that are new.
    const where = await page.evaluate(() => {
      const sel = getSelection();
      const mark = document.querySelector(".telling .hearing");
      const where = {
        heard: mark?.textContent.trim() ?? "",
        inMark: !!mark?.contains(sel.anchorNode),
        inSaid: !!document.querySelector(".telling .dictated")?.contains(sel.anchorNode),
      };
      window.__pauseEvents(false);
      return where;
    });
    if (where.heard) walk.wrong = `the words clicked into are still being heard: "${where.heard}"`;
    else if (where.inMark || !where.inSaid) walk.wrong = "the cursor is not in the words clicked into";
  },
  // ["cut on caption", n]
  //                    double-clicks the middle of the caption block n,
  //                    from 0
  async "cut on caption"(walk, arg, s) {
    const { page, watch } = walk;
    const box = await page.locator(".clip-timeline .caption").nth(arg).boundingBox();
    if (!box) {
      walk.wrong = `there is no caption ${arg + 1} to double-click`;
      return;
    }
    await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
    await watch.step("cut", s);
  },
  // ["block after", n] clicks the caption block of the first caption that
  //                    appears after the clip's nth cut, from 1, which puts
  //                    the playhead on its first word
  async "block after"(walk, arg, s) {
    const { page, watch } = walk;
    const now = await engineState(page, watch.at);
    // Where the cut ends, on the clip's clock, which the captions are on.
    const back = now.segments.slice(0, arg).reduce((sum, p) => sum + p.end - p.start, 0);
    const cue = now.captions.find((c) => c.start >= back + 0.01);
    if (!cue) {
      walk.wrong = `no caption appears after cut ${arg}`;
      return;
    }
    await page.locator(`.clip-timeline .caption[data-first="${cue.first}"]`).first().click();
    await watch.step("block after", s);
  },
  // ["captions like the short"]
  //                    puts the playhead on the first frame of the clip's
  //                    words and in their middle, and holds the lower half
  //                    of the crop in the video preview to the frame of
  //                    the short Render wrote that shows the same moment
  async "captions like the short"(walk) {
    const { page, seq, watch } = walk;
    const fps = await rate(page);
    const state = await engineState(page, watch.at);
    const short = await shortOf(page, watch.at);
    if (!short || !existsSync(short)) {
      walk.wrong = "the clip has no short to compare with";
      return;
    }
    const pieces = state.segments;
    const source = await ask(page, "Source", await episodeOn(page));
    // The video preview at the short's own size, so the two are drawn
    // at one size and compared pixel for pixel: on a page of its own,
    // made taller until the crop is as tall as the short.
    const tall = Number(execFileSync("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries",
      "stream=height", "-of", "csv=p=0", short]).toString().trim());
    const sized = (await open(url, { viewport: { width: 2400, height: 1000 }, again: page.context().browser(), still: still(seq), clock: clock(seq) })).page;
    const canvasTall = () => sized.evaluate(() => document.querySelector(".screen canvas").height);
    for (let i = 0, high = 1000; i < 12 && (await canvasTall()) !== tall; i++) {
      high += Math.round((tall - (await canvasTall())) * 1.4) || Math.sign(tall - (await canvasTall()));
      await sized.setViewportSize({ width: 2400, height: high });
      await settle(sized);
    }
    if ((await canvasTall()) !== tall) {
      walk.wrong = `the video preview could not be made ${tall} pixels tall, it is ${await canvasTall()}`;
      return;
    }
    // A moment of the clip's clock in the episode, and the frame of the
    // short that shows the frame of the episode that holds it: the
    // frames of the pieces before, and the frames into its own, each
    // piece the frames from its start up to the one before its end,
    // see pieceFrames in engine/render.go.
    const into = (t) => {
      let sum = 0;
      for (const p of pieces) {
        if (t < sum + p.end - p.start) return p.start + t - sum;
        sum += p.end - p.start;
      }
      return null;
    };
    // The frame that holds a moment, a frame that begins a millisecond
    // or less after it included, as seekTo waits for it.
    const shortIndex = (at) => {
      const k = Math.floor((at + 0.001) * fps);
      let n = 0;
      for (const p of pieces) {
        const [first, end] = [Math.round(p.start * fps), Math.round(p.end * fps)];
        if (k < end) return n + k - first;
        n += end - first;
      }
      return -1;
    };
    // The first frame of every word, where the highlight bounces, and
    // a frame in its middle, where it has settled.
    const words = (state.captions ?? []).flatMap((c) => c.lines.flatMap((l) => l.words));
    const moments = words
      .flatMap((w) => [w.start + 0.5 / fps, (w.start + w.end) / 2])
      .map(into)
      .filter((at) => at !== null);
    let compared = 0;
    let apart = 0;
    const report = [];
    for (const at of moments.slice(0, 12)) {
      if (!(await seekTo(sized, at, fps))) {
        walk.wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
        break;
      }
      // The click changed nothing of the clip: two clicks a few pixels
      // apart were once a double-click, which cut a part out, and the clip
      // compared was no longer the one the short was made from, issue #560.
      const now = (await engineState(page, watch.at)).segments;
      if (JSON.stringify(now) !== JSON.stringify(pieces)) {
        walk.wrong = `the click that put the playhead at ${at.toFixed(2)} s changed the clip, its pieces were ${JSON.stringify(pieces)} and are ${JSON.stringify(now)}`;
        break;
      }
      // Twice: the frame's own layer, then the pop it may start over
      // the frame once it is drawn, see "the pop over a paused frame".
      for (let i = 0; i < 2; i++) {
        await sized.evaluate(() => window.__captionLayers?.rested());
        await sized.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
      }
      // Nothing but the canvas draws them: the caption box over it
      // shows no word and no pill unless a word is being typed.
      const painted = await sized.evaluate(() => {
        const alpha = (c) => {
          const m = c.match(/rgba?\(([^)]*)\)/);
          const parts = m ? m[1].split(/[\s,/]+/).filter(Boolean) : [];
          return parts.length > 3 ? Number(parts[3]) : 1;
        };
        return [...document.querySelectorAll(".captions .line, .captions .line *")]
          .filter((e) => {
            const s = getComputedStyle(e);
            return (e.textContent.trim() && alpha(s.color) > 0) || alpha(s.backgroundColor) > 0;
          })
          .map((e) => e.textContent.trim());
      });
      if (painted.length) {
        walk.wrong = `the caption box draws "${painted[0]}" over the video preview, a second drawing of the captions`;
        break;
      }
      const got = await cropPixels(sized, source);
      // A click lands on a whole pixel of the clip timeline, so the
      // playhead is where it landed, near the moment asked for.
      const landed = Number(await sized.evaluate(() => document.querySelector(".screen").dataset.playhead));
      const n = shortIndex(landed);
      if (!got || n < 0) continue;
      // The frame and the two either side of it, in one read.
      const near = shortFramesFrom(short, Math.max(0, n - 1), n + 1, got.w, got.h);
      const ofShort = (m) => near[m - Math.max(0, n - 1)] ?? Buffer.alloc(0);
      const want = ofShort(n);
      const d = captionDiff(got.pixels, want, got.w, got.h);
      compared++;
      report.push(`${landed.toFixed(2)} s, frame ${n} of the short: ${d.mean.toFixed(2)} on average`);
      // The short is made with H.264 and keeps its colour at a quarter
      // of its pixels, so it is never the same to the level: 3 on
      // average where the preview draws what the short does, against
      // 4.5 and more for the frame before or after with another word
      // lit, and 41 for the canvas before 2.211, which drew none and
      // left the captions to the browser.
      if (d.mean > 3.6) {
        // Issue #547: this failed once on CI and never here. What the
        // video preview had up says which way it went wrong: the
        // frame it showed, how its captions came, and the frame of
        // the short nearby it looks most like, another moment's
        // captions or none at all.
        const had = await sized.evaluate(() => {
          const l = window.__captionLayers;
          const screen = document.querySelector(".screen");
          return {
            pictured: window.__pictured(),
            busy: l?.busy,
            late: l?.late,
            popping: l?.popping,
            stats: l ? { ...l.stats } : null,
            lost: Number(screen?.dataset.captionsLost ?? 0),
          };
        });
        const from = Math.max(0, n - 3);
        const around = shortFramesFrom(short, from, n + 3, got.w, got.h)
          .map((f, i) => [from + i, f.length === want.length ? captionDiff(got.pixels, f, got.w, got.h).mean : Infinity])
          .sort((a, b) => a[1] - b[1]);
        walk.wrong =
          `at ${landed.toFixed(2)} s the captions in the video preview differ from frame ${n} of the short by ${d.mean.toFixed(2)} on average and ${d.most} at most, where the captions are. ` +
          `It showed frame ${had.pictured} of the video, the caption layers were ${had.busy ? "busy" : "at rest"}${had.late ? ", late" : ""}${had.popping ? ", popping" : ""}, ${had.lost} layers lost, ` +
          `${JSON.stringify(had.stats)}, and it looks most like frame ${around[0][0]} of the short, by ${around[0][1].toFixed(2)}`;
        break;
      }
      // The frames of the short either side, where their captions are
      // drawn otherwise, are told apart from it, so a frame off in the
      // highlight would not pass.
      for (const m of [n - 1, n + 1]) {
        if (m < 0) continue;
        const other = ofShort(m);
        if (other.length !== want.length) continue;
        const o = captionDiff(want, other, got.w, got.h, 3);
        if (o.mean < 1) continue;
        const off = captionDiff(got.pixels, other, got.w, got.h);
        report.push(`  frame ${m}, drawn otherwise by ${o.mean.toFixed(2)}: ${off.mean.toFixed(2)} on average`);
        if (off.mean > d.mean + 0.5) apart++;
      }
    }
    if (process.env.VERBOSE) console.log(`        ${report.join("\n        ")}`);
    if (!walk.wrong && apart < 4) walk.wrong = `only ${apart} frames of the short either side were told apart from the video preview, so a frame off would pass`;
    if (!walk.wrong && compared < 6) walk.wrong = `only ${compared} frames of the short were compared`;
    // A comparison that failed leaves its page open, so its trace is kept
    // with the walk's, see keepTraces. Closed, the trace of the page the
    // comparison was made on went with it, and the one kept held only the
    // render, issue #560.
    if (!walk.wrong) await sized.close();
  },
  // ["caption edge", [edge, word]]
  //                    clicks the "start" or "end" edge of the caption
  //                    holding the word on the clip timeline, without moving
  async "caption edge"(walk, arg, s, what) {
    const { page, watch } = walk;
    const [edge, text] = arg;
    const now = await engineState(page, watch.at);
    const i = now.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
    if (i < 0) {
      walk.wrong = `no caption has "${text}"`;
      return;
    }
    const label = `When caption ${i + 1} ${edge === "end" ? "goes" : "appears"}`;
    await page.locator(`.clip-timeline [aria-label="${label}"]`).first().click();
    await watch.step(what, s);
  },
  // ["drawn", word]    the video preview shows the caption holding the
  //                    word where the playhead stands, letters drawn and the
  //                    caption box holding the word
  async drawn(walk, arg) {
    const { captioned, page, settled } = walk;
    // The video preview shows the caption holding the word where the
    // playhead stands: libass has drawn letters on the frame, and the
    // caption box over them holds the word. An edge dragged with shift
    // took the playhead to a moment between two frames, and the frame
    // shown was the one before the caption appears. Tim saw nothing to
    // snap to on #294.
    await settled();
    await captioned();
    await settled();
    const white = await page.evaluate(() => {
      const canvas = document.querySelector(".screen canvas");
      if (!canvas?.width) return 0;
      const c = document.createElement("canvas");
      c.width = canvas.width;
      c.height = canvas.height;
      const g = c.getContext("2d", { willReadFrequently: true });
      g.drawImage(canvas, 0, 0);
      const d = g.getImageData(0, 0, c.width, c.height).data;
      let n = 0;
      for (let i = 0; i < d.length; i += 4) if (d[i] > 220 && d[i + 1] > 220 && d[i + 2] > 220) n++;
      return n;
    });
    const box = (await shown(page)).map((w) => w.text).join(" ");
    const at = await page.evaluate(() => Number(document.querySelector(".screen")?.dataset.playhead));
    if (white < 100 || !box.includes(arg)) {
      walk.wrong = `with the playhead at ${at.toFixed(3)} the video preview shows ${white} white pixels under the caption box "${box}", not the caption with "${arg}"`;
    }
  },
  // ["captions drawn"] once the layers have come, the picture shows the
  //                    letters of a caption
  async "captions drawn"(walk) {
    const { captioned, page, settled } = walk;
    // The letters libass draws on the canvas, under the caption box,
    // once the layers asked for have come.
    await settled();
    await captioned();
    await settled();
    const white = await page.evaluate(() => {
      const canvas = document.querySelector(".screen canvas");
      if (!canvas?.width) return 0;
      const c = document.createElement("canvas");
      c.width = canvas.width;
      c.height = canvas.height;
      const g = c.getContext("2d", { willReadFrequently: true });
      g.drawImage(canvas, 0, 0);
      const d = g.getImageData(0, 0, c.width, c.height).data;
      let n = 0;
      for (let i = 0; i < d.length; i += 4) if (d[i] > 220 && d[i + 1] > 220 && d[i + 2] > 220) n++;
      return n;
    });
    const box = (await shown(page)).map((w) => w.text).join(" ");
    console.log(`    ${white} white pixels on the canvas under "${box}"`);
    if (white < 100) walk.wrong = `the picture shows no captions, ${white} white pixels, under the caption box "${box}"`;
  },
  // ["box", text]      the caption box reads this, word by word
  async box(walk, arg, s) {
    const box = s.map((w) => w.text).join(" ");
    if (box !== arg) walk.wrong = `the caption box reads "${box}", not "${arg}"`;
  },
  // ["font", name]     picks this family in the captions column, and waits
  //                    for the captions drawn in it
  // ["style", name]    picks this style of the family, and waits for the
  //                    captions drawn in it
  async style(walk, arg, s, what) {
    const { page, settled, watch } = walk;
    const field = what === "style" ? "Style" : "Font";
    await page.locator(`button.pick[aria-label="${field}"]`).click();
    await page.locator(".pick-row", { hasText: new RegExp(`^\\s*${arg}\\s*$`) }).first().click();
    await watch.step("click", s);
    await pageUntil(page, (name) => document.querySelector(".captions")?.style.fontFamily.includes(name), arg);
    await settled();
  },
  // ["search font", "letters|name"]
  //                    types the letters in the Font list's search field,
  //                    finds the family first, picks it with Enter and
  //                    waits for the captions drawn in it
  async "search font"(walk, arg, s) {
    const { page, settled, watch } = walk;
    const [letters, name] = arg.split("|");
    await page.locator('button.pick[aria-label="Font"]').click();
    await watch.step("click", s);
    await page.keyboard.type(letters);
    await watch.step("type", s);
    const first = await page.evaluate(() => document.querySelector(".pick-list .pick-row")?.textContent.trim());
    if (first !== name) {
      walk.wrong = `${letters} found ${first ?? "nothing"} first, not ${name}`;
      await page.keyboard.press("Escape");
      return;
    }
    await page.keyboard.press("Enter");
    await watch.step("press", s);
    await pageUntil(page, (name) => document.querySelector(".captions")?.style.fontFamily.includes(name), name);
    await settled();
  },
  async "parts give way"(walk, arg, s) {
    const { page, watch } = walk;
    // With the transcript open while the work hears, where the first
    // part not transcribed yet begins, against how far the saved
    // transcript reaches, over nine of the job's news.
    const path = await episodeOn(page);
    await page.evaluate(() => document.querySelector('.listhead button[aria-label="Search the transcript"]').click());
    await watch.step("click", s);
    // Where its dots end on the right, too, which must not move while
    // its left edge does: centred, the pattern walked back and forth
    // as the edge passed over it, found by Tim. Read in every frame
    // until the work has told the app how far it heard nine times,
    // with what the saved transcript holds asked after each.
    const from = await news(page);
    const times = 9;
    const sampled = page.evaluate(
      ([from, times]) =>
        new Promise((done) => {
          const seen = [];
          const tick = () => {
            const u = document.querySelector(".track .unheard");
            let now = null;
            if (u) {
              const box = u.getBoundingClientRect();
              const cs = getComputedStyle(u, "::before");
              now = {
                left: Math.round(box.left * 10) / 10,
                end: Math.round((box.left + parseFloat(cs.left) + parseFloat(cs.width)) * 100) / 100,
                side: parseFloat(cs.left),
              };
            }
            if (!seen.length || JSON.stringify(seen.at(-1)) !== JSON.stringify(now)) seen.push(now);
            if ((window.__news.job ?? 0) >= from + times) done(seen);
            else requestAnimationFrame(tick);
          };
          requestAnimationFrame(tick);
        }),
      [from, times],
    );
    const saves = new Set();
    for (let seen = from; seen < from + times; ) {
      // With the speech held, a piece more is heard for each news.
      if (walk.speechHeld) await control(url, "/speech?step=1");
      seen = await nextNews(page, seen);
      const status = await ask(page, "Episode", path).catch(() => null);
      saves.add(JSON.stringify(status?.heard ?? null));
    }
    const samples = await sampled;
    const edges = samples.map((d) => d?.left ?? null);
    const dots = samples.filter(Boolean);
    await page.keyboard.press("Escape");
    const moves = edges.filter((v, i) => i && v !== null && edges[i - 1] !== null && v !== edges[i - 1]).length;
    if (edges.every((v) => v === null)) walk.wrong = "no part was not transcribed yet while the work heard";
    else if (moves < Math.max(4, 2 * saves.size))
      walk.wrong = `the first part not transcribed yet moved ${moves} times over ${times} of the job's news while the saved transcript changed ${saves.size - 1} times, so it gives way as the transcript is saved, not as the work hears`;
    else {
      // The samples taken while the edge moved, from the first move
      // to the last, the dots' right end in each.
      const moving = dots.filter((d, i) => i && d.left !== dots[i - 1].left || dots[i + 1] && dots[i + 1].left !== d.left);
      const ends = [...new Set(moving.map((d) => d.end))];
      console.log(JSON.stringify({ moves, ends: ends.slice(0, 8), sides: [...new Set(moving.map((d) => d.side))].slice(0, 8) }));
      if (ends.length > 1)
        walk.wrong = `the dots of the part giving way moved while its edge did, their right end at ${ends.slice(0, 6).join(", ")} px`;
      else if (moving.some((d) => d.side !== 0))
        walk.wrong = `the dots of the part giving way stop short of its edge, ${moving.find((d) => d.side !== 0).side} px in, so the edge does not wipe them`;
    }
  },
  async "edges alike"(walk, arg, s) {
    const { page, watch } = walk;
    // The edge of a card of the chosen clip looks the same in the
    // transcript and in the clip list: a bar down the left 4 px wide,
    // cut off straight by the card's rounded corners. In the
    // transcript it curved round them and was a pixel narrower, found
    // by Tim. Read off a screenshot, row by row, as how many pixels of
    // the app's colour each row holds at the card's left.
    const edgeOf = async (selector) => {
      const box = await page.evaluate((selector) => {
        const r = document.querySelector(selector)?.getBoundingClientRect();
        return r ? { x: r.left, y: r.top, height: r.height } : null;
      }, selector);
      if (!box) return null;
      const clip = { x: Math.floor(box.x), y: Math.floor(box.y), width: 12, height: Math.floor(box.height) };
      const shot = (await page.screenshot({ clip })).toString("base64");
      return page.evaluate(async (data) => {
        const img = new Image();
        img.src = `data:image/png;base64,${data}`;
        await img.decode();
        const c = document.createElement("canvas");
        c.width = img.width;
        c.height = img.height;
        const g = c.getContext("2d", { willReadFrequently: true });
        g.drawImage(img, 0, 0);
        const d = g.getImageData(0, 0, c.width, c.height).data;
        const rows = [];
        for (let y = 0; y < c.height; y++) {
          let n = 0;
          for (let x = 0; x < c.width; x++) {
            const i = (y * c.width + x) * 4;
            if (d[i] - d[i + 1] > 40 && d[i + 2] - d[i + 1] > 40) n++;
          }
          rows.push(n);
        }
        return { rows, scale: img.width / 12 };
      }, shot);
    };
    await page.mouse.move(1, 1);
    await page.evaluate(() => document.querySelector('.listhead button[aria-label="Search the transcript"]').click());
    await watch.step("click", s);
    await page.waitForSelector("ol.said li[data-clip] button.pick");
    // A card of a clip wholly in view, so nothing of the veil lies
    // over it.
    const card = await page.evaluate(() => {
      const list = document.querySelector(".scroll.list").getBoundingClientRect();
      const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
      const li = [...document.querySelectorAll("ol.said li[data-clip]")].find((li) => {
        const r = li.getBoundingClientRect();
        return r.top >= list.top + pad && r.bottom <= list.bottom - pad;
      });
      return li?.dataset.row ?? null;
    });
    if (!card) {
      walk.wrong = "no card of a clip wholly in view in the transcript";
      return;
    }
    const said = await edgeOf(`ol.said li[data-row="${card}"] button.pick`);
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    await page.waitForSelector("aside ol li .pick.current");
    await page.mouse.move(1, 1);
    const listed = await edgeOf("aside ol li .pick.current");
    const ends = (e) => [...e.rows.slice(0, 10), ...e.rows.slice(-10)].join(",");
    const middle = (e) => e.rows[Math.floor(e.rows.length / 2)];
    console.log(JSON.stringify({ said: { ends: ends(said), middle: middle(said) }, listed: { ends: ends(listed), middle: middle(listed) } }));
    if (middle(listed) !== 4 * listed.scale) walk.wrong = `the edge of the chosen clip in the clip list is ${middle(listed) / listed.scale} px wide, not 4`;
    else if (middle(said) !== 4 * said.scale) walk.wrong = `the edge of a clip's card in the transcript is ${middle(said) / said.scale} px wide, not 4`;
    else if (ends(said) !== ends(listed))
      walk.wrong = `the edge of a clip's card in the transcript ends at its corners in rows of ${ends(said)} pixels, and in the clip list in rows of ${ends(listed)}`;
  },
  // ["font then transcript"]
  //                    "on" makes every read of the cards of what was said
  //                    fail. "check" opens the search and sees the list say
  //                    the transcript could not be read, with the reason
  //                    under the pointer, then lets the reads work, opens it
  //                    again and sees the cards with nothing said of it
  async "font then transcript"(walk, arg, s) {
    const { page, watch } = walk;
    // The font list open, then the transcript button pressed, found
    // by Tim: the list stayed open over the workspace with the
    // keyboard still in its field, and Escape closed the list first.
    await page.locator('button.pick[aria-label="Font"]').click();
    await watch.step("click", s);
    await page.waitForSelector(".pick-list .pick-row");
    await page.locator('.listhead button[aria-label="Search the transcript"]').click();
    await watch.step("click", s);
    const seen = await page.evaluate(() => ({
      list: !!document.querySelector(".pick-list"),
      focused: document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.placeholder ?? document.activeElement?.tagName ?? null,
      field: !!document.querySelector(".listhead .search input"),
    }));
    console.log(JSON.stringify(seen));
    if (seen.list) walk.wrong = "the font list stayed open after the transcript button was pressed";
    else if (!seen.field) walk.wrong = "the transcript button opened no search";
    else if (seen.focused !== "Search transcript") walk.wrong = `the keyboard is in ${seen.focused}, not the transcript's field`;
    if (!walk.wrong) {
      await page.keyboard.press("Escape");
      await watch.step("press", s);
      const after = await page.evaluate(() => !!document.querySelector(".listhead .search input"));
      if (after) walk.wrong = "Escape did not close the transcript search";
    }
  },
  // ["font list"]      the Font list hangs on the field's left edge with
  //                    its corners, every row is as tall as the field and
  //                    square, nothing stands between the search field and
  //                    the rows or under them, each family is drawn in its
  //                    own face, ours by name first, the field is drawn
  //                    in the face picked, the search field is a row tall
  //                    above its line, the line over Installed sits on the
  //                    last of ours, and every name drawn in its face has
  //                    its capitals in the middle of its box
  async "font list"(walk, arg, s) {
    const { page, watch } = walk;
    await page.locator('button.pick[aria-label="Font"]').click();
    await watch.step("click", s);
    await page.waitForSelector(".pick-list .pick-row");
    // Each family's face is fetched as its row is drawn, so the rows
    // are read once the faces have come.
    await page.evaluate(() => document.fonts.ready);
    const seen = await page.evaluate(() => {
      const trigger = document.querySelector('button.pick[aria-label="Font"]');
      const field = trigger.getBoundingClientRect();
      const list = document.querySelector(".pick-list").getBoundingClientRect();
      const rowsBox = document.querySelector(".pick-list .pick-rows");
      // Where the capitals of a name stand against the middle of the
      // box it is in: the baseline, read from a mark of no size sat on
      // it, less half the height of a capital in that face.
      const offCentre = (text, box) => {
        const mark = document.createElement("span");
        mark.style.cssText = "display:inline-block;width:0;height:0;vertical-align:baseline";
        text.append(mark);
        const baseline = mark.getBoundingClientRect().bottom;
        // And the height of a capital, from the face's own figures.
        mark.style.height = "1cap";
        const known = mark.getBoundingClientRect().height;
        mark.remove();
        const look = getComputedStyle(text);
        const pen = document.createElement("canvas").getContext("2d");
        pen.font = `${look.fontStyle} ${look.fontWeight} ${look.fontSize} ${look.fontFamily}`;
        const drawn = pen.measureText("H").actualBoundingBoxAscent;
        const r = box.getBoundingClientRect();
        // The capital the face says it has. The H drawn on a canvas is
        // rounded to whole pixels, and differently by each version of
        // Chromium, so measured by it Inter stood 0.3 px high here and
        // 0.8 px in CI, with the same layout.
        const by = baseline - known / 2 - (r.top + r.height / 2);
        return [by, `${Math.abs(by).toFixed(1)} px ${by < 0 ? "above" : "below"}, with capitals ${known.toFixed(2)} high, drawn ${drawn.toFixed(2)}`];
      };
      const said = trigger.querySelector(".said > span:last-child");
      const centres = [[said.textContent.trim() + " in the field", ...offCentre(said, trigger)]];
      for (const row of document.querySelectorAll(".pick-list .pick-row")) {
        const what = row.querySelector(".what.shown");
        if (what) centres.push([row.textContent.trim(), ...offCentre(what, row)]);
      }
      const search = document.querySelector(".pick-list .search");
      const head = [...document.querySelectorAll(".pick-list .pick-group")].find((it) => it.textContent.trim() === "Installed");
      const ourLast = [...document.querySelectorAll(".pick-list .pick-row")]
        .filter((row) => row.compareDocumentPosition(head) & Node.DOCUMENT_POSITION_FOLLOWING)
        .at(-1);
      const first = document.querySelector(".pick-list .pick-rows .pick-row, .pick-list .pick-rows .pick-group");
      const searchBox = search.getBoundingClientRect();
      const corner = (el) => getComputedStyle(el).borderTopLeftRadius;
      const above = first.getBoundingClientRect().top - searchBox.bottom;
      // The last row, with the rows scrolled to their end, to within
      // the part of a pixel a scroll is rounded to.
      rowsBox.scrollTop = rowsBox.scrollHeight;
      const all = [...document.querySelectorAll(".pick-list .pick-row")];
      const look = {
        corners: [corner(trigger), corner(document.querySelector(".pick-list"))],
        rowCorner: corner(all[0]),
        above,
        below: rowsBox.getBoundingClientRect().bottom - all.at(-1).getBoundingClientRect().bottom,
        face: getComputedStyle(trigger.querySelector(".said > span:last-child")).fontFamily,
        // The search field above its line, and the line over the head
        // of the installed fonts against the last of ours.
        searchHigh: searchBox.height - parseFloat(getComputedStyle(search).borderBottomWidth),
        headGap: head.getBoundingClientRect().top - ourLast.getBoundingClientRect().bottom,
        ourLast: ourLast.textContent.trim(),
        centres,
      };
      const rows = [...document.querySelectorAll(".pick-list .pick-row")].map((row) => {
        const what = row.querySelector(".what");
        const family = getComputedStyle(what).fontFamily.split(",")[0].replace(/"/g, "").trim();
        return {
          name: row.textContent.trim(),
          height: row.getBoundingClientRect().height,
          family,
          drawn: document.fonts.check(`13px "${family}"`, what.textContent),
        };
      });
      return { field: { left: field.left, height: field.height }, list: { left: list.left, height: list.height }, rows, look };
    });
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    const narrow = seen.rows.find((row) => row.name === "Bridge Narrow");
    const squeezed = seen.rows.find((row) => Math.abs(row.height - seen.field.height) > 0.5);
    if (seen.rows.length < 20) walk.wrong = `the Font list has ${seen.rows.length} rows, too few to be longer than it may be tall`;
    else if (Math.abs(seen.list.left - seen.field.left) > 0.5)
      walk.wrong = `the Font list starts at ${seen.list.left.toFixed(1)}, its field at ${seen.field.left.toFixed(1)}`;
    else if (squeezed) walk.wrong = `${squeezed.name} is ${squeezed.height.toFixed(1)} high, its field ${seen.field.height.toFixed(1)}`;
    else if (!narrow) walk.wrong = "the Font list has no Bridge Narrow";
    else if (narrow.family !== "Bridge Narrow" || !narrow.drawn)
      walk.wrong = `Bridge Narrow is drawn in ${narrow.family}${narrow.drawn ? "" : ", which has not loaded"}`;
    else if (seen.rows.slice(0, 3).map((row) => row.name).join(", ") !== "Anton, Archivo Black, Inter")
      walk.wrong = `the app's fonts stand as ${seen.rows.slice(0, 3).map((row) => row.name).join(", ")}, not by name`;
    else if (seen.look.corners[0] !== seen.look.corners[1])
      walk.wrong = `the list's corners are ${seen.look.corners[1]}, its field's ${seen.look.corners[0]}`;
    else if (seen.look.rowCorner !== "0px") walk.wrong = `a row has corners of ${seen.look.rowCorner}`;
    else if (Math.abs(seen.look.above) > 0.5 || Math.abs(seen.look.below) >= 1)
      walk.wrong = `${seen.look.above.toFixed(1)} px between the search field and the first row, ${seen.look.below.toFixed(1)} under the last`;
    else if (!seen.look.face.startsWith('"Inter Black"'))
      walk.wrong = `the Font field is drawn in ${seen.look.face}, not in Inter Black`;
    else if (Math.abs(seen.look.searchHigh - seen.field.height) > 0.5)
      walk.wrong = `the search field is ${seen.look.searchHigh.toFixed(1)} high above its line, a row ${seen.field.height.toFixed(1)}`;
    else if (Math.abs(seen.look.headGap) > 0.5)
      walk.wrong = `${seen.look.headGap.toFixed(1)} px between ${seen.look.ourLast} and the line over Installed`;
    else {
      const off = seen.look.centres.filter(([, by]) => Math.abs(by) > 0.5);
      if (off.length)
        walk.wrong = `the capitals stand off the middle: ${off
          .map(([name, , said]) => `${name} ${said}`)
          .join(", ")}`;
    }
  },
  // ["style list"]     the Style list of one style ends on the bottom of
  //                    the field under the Style field
  async "style list"(walk, arg, s) {
    const { page, watch } = walk;
    // A list of one style hung under its field covers the field under
    // that exactly, its bottom on that field's bottom, so no second
    // border shows under its own.
    await page.locator('button.pick[aria-label="Style"]').click();
    await watch.step("click", s);
    await page.waitForSelector(".pick-list .pick-row");
    const seen = await page.evaluate(() => {
      const setting = document.getElementById("caption-style").closest(".setting");
      const under = setting.nextElementSibling.querySelector("input, button");
      return {
        rows: document.querySelectorAll(".pick-list .pick-row").length,
        list: document.querySelector(".pick-list").getBoundingClientRect().bottom,
        under: under.getBoundingClientRect().bottom,
      };
    });
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    if (seen.rows !== 1) walk.wrong = `the Style list has ${seen.rows} rows, not one`;
    else if (Math.abs(seen.list - seen.under) > 0.5)
      walk.wrong = `the Style list ends ${Math.abs(seen.list - seen.under).toFixed(1)} px ${seen.list < seen.under ? "above" : "below"} the bottom of the field under it`;
  },
  // ["nothing found", letters]
  //                    typing the letters in the Font list finds nothing,
  //                    said in the middle of a list as wide as its field,
  //                    or as the words where they need more
  async "nothing found"(walk, arg, s) {
    const { page, watch } = walk;
    // Letters that find nothing leave the list as wide as its field,
    // or as the words that say so where they need more, said in its
    // middle.
    await page.locator('button.pick[aria-label="Font"]').click();
    await watch.step("click", s);
    await page.keyboard.type(arg);
    await watch.step("type", s);
    const seen = await page.evaluate(() => {
      const none = document.querySelector(".pick-list .pick-none");
      if (!none) return null;
      const text = document.createRange();
      text.selectNodeContents(none);
      const t = text.getBoundingClientRect();
      const list = document.querySelector(".pick-list").getBoundingClientRect();
      const field = document.querySelector('button.pick[aria-label="Font"]').getBoundingClientRect();
      return {
        side: t.left - list.left - (list.right - t.right),
        list: list.width,
        field: field.width,
        needs: Math.max(field.width, t.width + 16 + 2),
        high: none.getBoundingClientRect().height,
      };
    });
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    if (!seen) walk.wrong = `${arg} found something`;
    else if (Math.abs(seen.side) > 1) walk.wrong = `Nothing matches stands ${seen.side.toFixed(1)} px off the middle of the list`;
    else if (seen.list - seen.needs > 1)
      walk.wrong = `the list saying Nothing matches is ${seen.list.toFixed(1)} wide, its field ${seen.field.toFixed(1)}`;
    else if (Math.abs(seen.high - seen.field) > 0.5 && Math.abs(seen.high - 32) > 0.5)
      walk.wrong = `Nothing matches is ${seen.high.toFixed(1)} high, not a row`;
  },
  // ["search order", letters]
  //                    what typing the letters in the Font list finds is
  //                    in the order of the whole list, under its own head
  async "search order"(walk, arg, s) {
    const { page, watch } = walk;
    // What a search finds stands in the order of the whole list, each
    // row under the head of its own group.
    const order = () =>
      page.evaluate(() =>
        [...document.querySelectorAll(".pick-list .pick-group, .pick-list .pick-row")].map((it) =>
          (it.classList.contains("pick-group") ? "# " : "") + it.textContent.trim(),
        ),
      );
    await page.locator('button.pick[aria-label="Font"]').click();
    await watch.step("click", s);
    const whole = await order();
    await page.keyboard.type(arg);
    await watch.step("type", s);
    const found = await order();
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    const head = (list, name) => list.slice(0, list.indexOf(name)).filter((it) => it.startsWith("# ")).at(-1) ?? "";
    const rows = found.filter((it) => !it.startsWith("# "));
    const places = rows.map((name) => whole.indexOf(name));
    if (rows.length < 2) walk.wrong = `${arg} found ${rows.join(", ") || "nothing"}`;
    else if (places.some((at, i) => at < 0 || (i > 0 && at < places[i - 1])))
      walk.wrong = `${arg} found ${rows.join(", ")}, not in the order of the list`;
    else {
      const moved = rows.find((name) => head(found, name) !== head(whole, name));
      if (moved) walk.wrong = `${moved} stands under ${head(found, moved) || "no head"} after ${arg}`;
    }
  },
  // ["style is", name] the Style field says this
  async "style is"(walk, arg) {
    const { page } = walk;
    const said = await page.locator('button.pick[aria-label="Style"]').textContent();
    // The trigger keeps room for its longest style, drawn and hidden.
    const shown = await page.locator('button.pick[aria-label="Style"] .said > span:last-child').textContent();
    if (shown.trim() !== arg) walk.wrong = `the Style field says ${shown.trim() || said.trim()}, not ${arg}`;
  },
  async "listed under"(walk, arg, s) {
    const { page, watch } = walk;
    // The Font list puts a face under a head of its group.
    const [head, name] = arg.split("|");
    await page.locator('button.pick[aria-label="Font"]').click();
    await watch.step("click", s);
    const order = await page.evaluate(() =>
      [...document.querySelectorAll(".pick-list .pick-group, .pick-list .pick-row")].map((it) =>
        (it.classList.contains("pick-group") ? "# " : "") + it.textContent.trim(),
      ),
    );
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    const at = order.indexOf(name);
    const heads = order.slice(0, at).filter((it) => it.startsWith("# "));
    if (at < 0) walk.wrong = `the Font list has no ${name}: ${order.join(", ")}`;
    else if (heads.at(-1) !== `# ${head}`) walk.wrong = `${name} is under ${heads.at(-1) ?? "no head"}, not ${head}`;
  },
  // ["thickened as libass", word]
  //                    the word opened is drawn at its face's own weight
  //                    with no bold the browser fakes, and its letters
  //                    grown by an em over 64 where libass thickens the
  //                    face, see engine.Thickening
  async "thickened as libass"(walk, arg, s) {
    const { page, watch, word } = walk;
    // Read off the word opened, since the pixels of letters differ from
    // one machine's rendering to another's more than the bold does:
    // Chromium faked the bold of Archivo Black 18 % heavier than
    // libass, the Mac 16 % lighter, and CI draws Inter Black, which
    // nothing thickens, 10 % heavier than here.
    await word(arg).click();
    await watch.step("click", s);
    const look = await page.evaluate(async () => {
      const it = document.querySelector(".captions .word.fixing");
      if (!it) return null;
      const style = getComputedStyle(it);
      const face = style.fontFamily.split(",")[0].replace(/["']/g, "").trim();
      const fonts = await fetch("/call", { method: "POST", body: JSON.stringify({ name: "Fonts", args: [] }) })
        .then((r) => r.json())
        .catch(() => null);
      return {
        face,
        weight: Number(style.fontWeight),
        synthesis: style.fontSynthesis || style.getPropertyValue("font-synthesis"),
        stroke: parseFloat(style.webkitTextStrokeWidth),
        size: parseFloat(style.fontSize),
        declared: (fonts ?? []).find((f) => f.name === face)?.weight,
      };
    });
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    console.log(`    ${JSON.stringify(look)}`);
    if (!look) walk.wrong = `"${arg}" did not open`;
    else if (!/none/.test(look.synthesis)) walk.wrong = `the browser may fake a bold of its own, font-synthesis is "${look.synthesis}"`;
    else if (look.declared !== undefined && look.weight !== look.declared)
      walk.wrong = `"${arg}" is drawn at weight ${look.weight} and ${look.face} is a face of ${look.declared}`;
    else if (Math.abs(look.stroke - look.size / 64) > 0.01)
      walk.wrong = `"${arg}" is grown by ${look.stroke} px and libass grows ${look.face} at ${look.size} px by ${(look.size / 64).toFixed(3)}`;
  },
  // ["selection in the pill"]
  //                    the word open, all its letters selected paint
  //                    nothing outside its pill, above it or under it
  async "selection in the pill"(walk) {
    const { page } = walk;
    // The strips over and under the pill as far as the letters' line
    // reaches, shot with the caret alone and again with every letter
    // selected: what the selection paints there is outside the pill.
    const open = page.locator(".captions .word.fixing");
    const box = await open.evaluate((e) => {
      const pill = e.getBoundingClientRect();
      const r = document.createRange();
      r.selectNodeContents(e);
      const line = r.getBoundingClientRect();
      return { left: pill.left, width: pill.width, top: pill.top, bottom: pill.bottom, over: line.top, under: line.bottom };
    });
    const strips = [
      { x: box.left, y: Math.min(box.over, box.top - 6), width: box.width, height: box.top - 1 - Math.min(box.over, box.top - 6) },
      { x: box.left, y: box.bottom + 1, width: box.width, height: Math.max(box.under, box.bottom + 6) - box.bottom - 1 },
    ].filter((c) => c.height >= 1);
    const shoot = async () => Promise.all(strips.map(async (clip) => (await page.screenshot({ clip })).toString("base64")));
    await open.evaluate((e) => (e.style.caretColor = "transparent"));
    const before = await shoot();
    await page.keyboard.press("ControlOrMeta+A");
    await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
    const after = await shoot();
    await open.evaluate((e) => (e.style.caretColor = ""));
    const changed = await page.evaluate(async ([before, after]) => {
      const read = async (shot) => {
        const img = new Image();
        img.src = `data:image/png;base64,${shot}`;
        await img.decode();
        const c = document.createElement("canvas");
        c.width = img.naturalWidth;
        c.height = img.naturalHeight;
        const g = c.getContext("2d", { willReadFrequently: true });
        g.drawImage(img, 0, 0);
        return g.getImageData(0, 0, c.width, c.height).data;
      };
      let n = 0;
      for (let k = 0; k < before.length; k++) {
        const [a, b] = [await read(before[k]), await read(after[k])];
        for (let i = 0; i < a.length; i += 4) {
          if (Math.max(Math.abs(a[i] - b[i]), Math.abs(a[i + 1] - b[i + 1]), Math.abs(a[i + 2] - b[i + 2])) > 24) n++;
        }
      }
      return n;
    }, [before, after]);
    console.log(`        the letters' line reaches ${(box.top - box.over).toFixed(1)} px over the pill and ${(box.under - box.bottom).toFixed(1)} px under it`);
    if (changed) walk.wrong = `the selection painted ${changed} pixels outside the pill`;
  },
  // ["no caret"]       no word of the caption box holds the keyboard or the
  //                    selection, where WebKit would draw a caret
  async "no caret"(walk) {
    const { page } = walk;
    const held = await page.evaluate(() => {
      const box = document.querySelector(".captions");
      const sel = getSelection();
      return {
        selection: !!(box && sel?.rangeCount && box.contains(sel.anchorNode)),
        focus: !!(box && box.contains(document.activeElement)),
      };
    });
    if (held.selection) walk.wrong = "the selection stayed in the caption box, where WebKit draws the caret";
    else if (held.focus) walk.wrong = "a word of the caption box still holds the keyboard";
  },
  // ["open", word]     this word is open for typing
  async open(walk, arg, s) {
    const o = s.find((w) => w.focused);
    if (o?.text !== arg) walk.wrong = `"${o?.text ?? "nothing"}" is open, not "${arg}"`;
  },
  // ["spans", label]   every caption appears and goes when it did at the mark
  async spans(walk, arg) {
    const { marks, page, watch } = walk;
    const spans = (caps) => caps.captions.map((c) => `${c.start.toFixed(3)}-${c.end.toFixed(3)}`).join(" ");
    const now = await engineState(page, watch.at);
    if (spans(now) !== spans(marks[arg])) {
      walk.wrong = `the captions do not appear and go when they did at "${arg}"\nthen ${spans(marks[arg])}\nnow  ${spans(now)}\n${describe(marks[arg], now)}`;
    }
  },
  // ["wake on a caption"]
  //                    puts the playhead on a clip that starts on a
  //                    caption, adds another video and goes back to the
  //                    first from the sidebar, its frames slow to open:
  //                    the video preview keeps its frame and never goes
  //                    black on the way, and a card clicked then is put up
  //                    from what was read ahead
  async "wake on a caption"(walk, arg, s) {
    const { page, watch } = walk;
    walk.wrong = await wakeOnCaption(page);
    await watch.step("press", s);
  },
  // ["layers late", ms]
  //                    every caption layer the video preview asks the Go
  //                    side for comes ms late, the way it does on a busy
  //                    machine
  async "layers late"(walk, arg) {
    const { page } = walk;
    await page.evaluate((ms) => {
      const real = window.fetch;
      window.fetch = async (input, init) => {
        const answer = real(input, init);
        if (String(input).startsWith("/frames/captions?")) await new Promise((r) => setTimeout(r, ms));
        return answer;
      };
    }, arg);
  },
  // ["frame on the pill while it plays", seconds]
  //                    puts the playhead a second before the end of the
  //                    first caption that ends well inside the clip and
  //                    plays for so many seconds, and on every frame the
  //                    browser paints reads the pill libass drew off the
  //                    canvas: where a pill is drawn, the caption box
  //                    lights a word and its frame stands on the pill,
  //                    across within 3 px of the pill's middle and down
  //                    inside it, and where no pill is drawn no word is
  //                    lit. The play is on the walk's clock, which stands
  //                    on each frame until it is up, so every frame of the
  //                    play is shown and read on any machine
  async "frame on the pill while it plays"(walk, arg) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const state = await engineState(page, watch.at);
    const pieces = state.segments;
    const into = (t) => {
      let sum = 0;
      for (const p of pieces) {
        if (t < sum + p.end - p.start) return p.start + t - sum;
        sum += p.end - p.start;
      }
      return null;
    };
    const length = pieces.reduce((n, p) => n + p.end - p.start, 0);
    // The first caption that ends with room to play on both sides of
    // its end, and room after it for the next one.
    const end = (state.captions ?? []).map((c) => c.end).find((e) => e > 1.6 && e < length - arg);
    const from = end === undefined ? null : into(end - 1);
    if (from === null) {
      walk.wrong = "no caption ends far enough inside the clip to play over its end";
      return;
    }
    if (!(await seekTo(page, from, fps))) {
      walk.wrong = `the video preview never showed the frame at ${from.toFixed(2)}`;
      return;
    }
    await page.evaluate(() => window.__captionLayers?.rested());
    // How many paints wait for their layer is how fast libass answers on
    // the machine, not a rule of the app: playing, a frame whose layer is
    // on its way is drawn with the one before rather than none, see
    // CaptionLayers.last, and only the first frame of a play waits for
    // its own, see FrameQueue.dress. So it is told and not judged. That
    // every frame of a play asks for its layer ahead, the fix for issue
    // #288, is held in captions.test.ts and queue.test.ts.
    const stop = await watchPills(page);
    try {
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Space");
      // So many seconds of the episode, read off the playhead.
      await settle(page);
      const played = await playUntil(page, { past: from + arg - 1e-6 });
      if (!(played.at >= from + arg - 1e-6)) walk.wrong = `the play from ${from.toFixed(2)} stopped at ${played.at.toFixed(2)}, before it went on ${arg} s`;
    } finally {
      // Paused, however the play ended.
      await page.evaluate(() => {
        if (document.querySelector('button[aria-label="Play"]')) return;
        const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
        document.body.dispatchEvent(new KeyboardEvent("keydown", key));
        document.body.dispatchEvent(new KeyboardEvent("keyup", key));
      });
    }
    const frames = await stop();
    if (walk.wrong) return;
    const off = frames.find((f) => framesOffPill(f, "now"));
    const lit = new Set(frames.filter((f) => f.pill).map((f) => `${Math.round(f.pill.y)}:${f.now?.text}`));
    const behind = laggingFrames(frames);
    console.log(`    ${frames.length} frames painted, ${frames.filter((f) => f.pill).length} with a pill, ${lit.size} words lit`);
    console.log(`    ${behind.late} of ${behind.painted} painted with captions not their own, at most ${behind.ms.toFixed(0)} ms on end`);
    if (off) walk.wrong = framesOffPill(off, "now");
    else if (frames.filter((f) => f.pill).length < fps) walk.wrong = `only ${frames.length} frames were read while it played`;
    else if (lit.size < 3) walk.wrong = `the play lit only ${lit.size} words, it never went from one caption to the next`;
  },
  // ["walk the words with the frame on the pill", n]
  //                    from the clip's first frame, Shift and the right
  //                    arrow n times, and on every frame the browser paints
  //                    after each press reads the pill libass drew off the
  //                    canvas: the keyboard's frame stands on the pill,
  //                    within 3 px, on every frame it shows, never on a
  //                    word the picture has not lit yet, and is there once
  //                    the captions of the frame have come
  async "walk the words with the frame on the pill"(walk, arg) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const state = await engineState(page, watch.at);
    const first = state.segments[0];
    // From the clip's first frame, so the walk goes over the words of
    // the first captions and from one caption to the next.
    if (!(await seekTo(page, first.start + 0.5 / fps, fps))) {
      walk.wrong = "the video preview never showed the clip's first frame";
      return;
    }
    await page.evaluate(() => window.__captionLayers?.rested());
    await page.evaluate(() => document.body.focus());
    const report = [];
    for (let k = 0; k < arg && !walk.wrong; k++) {
      const stop = await watchPills(page);
      await page.keyboard.press("Shift+ArrowRight");
      await settle(page);
      await page.evaluate(() => window.__captionLayers?.rested());
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
      const frames = await stop();
      const off = frames.find((f) => framesOffPill(f, "keyed"));
      const last = frames[frames.length - 1];
      if (off) walk.wrong = `after Shift and the right arrow ${k + 1} times, ${framesOffPill(off, "keyed")}`;
      else if (!last?.keyed) walk.wrong = `after Shift and the right arrow ${k + 1} times, no word wears the keyboard's frame`;
      else report.push(`"${last.keyed.text}" ${frames.filter((f) => !f.keyed).length} frames without the frame`);
    }
    console.log(`    ${report.join(", ")}`);
  },
  async "step onto words and see them pop"(walk, arg) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const state = await engineState(page, watch.at);
    const first = state.segments[0];
    if (!(await seekTo(page, first.start + 0.5 / fps, fps))) {
      walk.wrong = "the video preview never showed the clip's first frame";
      return;
    }
    await page.evaluate(() => window.__captionLayers?.rested());
    await page.evaluate(() => document.body.focus());
    const report = [];
    let popped = 0;
    // The most a pop grew.
    let grew = 1;
    // The bounce is drawn on frames the walk gives it, one at a time,
    // each a sixtieth of a second after the one before, see Frames in
    // lib/frames/captions.ts. On the screen's own, a machine so busy
    // that it drew two frames in the 220 ms of a bounce, or none round
    // its peak, showed a bounce cut short, issues #328, #329 and #460,
    // and the step was taken again up to three times in the hope of
    // more frames. Given its frames, a bounce is drawn on the same
    // frames on any machine, and the pills are read off the screen
    // after each.
    await page.evaluate(() => {
      const given = { waiting: new Map(), n: 0, asked: null };
      given.next = (draw) => {
        given.waiting.set(++given.n, draw);
        const asked = given.asked;
        given.asked = null;
        asked?.();
        return given.n;
      };
      given.cancel = (id) => given.waiting.delete(id);
      window.__bounceFrames = given;
      window.__screenFrames = window.__captionLayers.frames;
      window.__captionLayers.frames = given;
    });
    // One step with key, watched: whether a pop started, the frames
    // read, and how many frames the bounce was drawn on.
    const stepWatched = async (key) => {
      const count = await page.evaluate(() => window.__captionLayers?.stats.popped ?? 0);
      const drawn = await page.evaluate(() => window.__captionLayers?.stats.bounced ?? 0);
      const stop = await watchPills(page);
      await page.keyboard.press(key);
      // A pop starts when the new frame is drawn with its own layer, so
      // once that frame is up, a bounce that was to start has started.
      await settle(page);
      await shownOnPlayhead(page, { captions: true });
      const started = await page.evaluate((n) => (window.__captionLayers?.stats.popped ?? 0) > n, count);
      // Its parts come from the Go side, and then it asks for its first
      // frame, or they did not come and the layers rest. Then it is
      // given its frames until it asks for no more, each read on the
      // screen's next frame, which watchPills reads first: the pills
      // are read once for each frame given.
      await page.evaluate(async () => {
        const given = window.__bounceFrames;
        const real = window.__real?.frame ?? requestAnimationFrame;
        if (!given.waiting.size) await Promise.race([new Promise((done) => (given.asked = done)), window.__captionLayers.rested()]);
        given.asked = null;
        for (let now = 0; given.waiting.size; now += 1000 / 60) {
          const draws = [...given.waiting.values()];
          given.waiting.clear();
          for (const draw of draws) draw(now);
          await new Promise((done) => real(done));
        }
      });
      await page.evaluate(() => window.__captionLayers?.rested());
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
      const frames = await stop();
      const bounced = (await page.evaluate(() => window.__captionLayers?.stats.bounced ?? 0)) - drawn;
      return { started, frames, bounced };
    };
    // arg steps to the right, then as many back to the first word.
    for (let k = 0; k < 2 * arg && !walk.wrong; k++) {
      const key = k < arg ? "Shift+ArrowRight" : "Shift+ArrowLeft";
      const { started, frames, bounced } = await stepWatched(key);
      const off = frames.find((f) => framesOffPill(f, "keyed"));
      const last = frames[frames.length - 1];
      if (off) {
        walk.wrong = `after ${key} on step ${k + 1}, ${framesOffPill(off, "keyed")}`;
        break;
      }
      if (!last?.keyed || !last.pill) {
        walk.wrong = `after ${key} on step ${k + 1}, no word wears the keyboard's frame on a pill`;
        break;
      }
      // The paints of the word stepped onto, its pill's width on each.
      const on = frames.filter((f) => f.pill && f.keyed?.text === last.keyed.text);
      const widths = on.map((f) => f.pill.w);
      const least = Math.min(...widths);
      const most = Math.max(...widths);
      report.push(
        `"${last.keyed.text}" ${started ? `popped from ${least.toFixed(0)} to ${most.toFixed(0)} px over ${bounced} frames` : "did not pop"}`,
      );
      if (!started) continue;
      popped++;
      grew = Math.max(grew, most / least);
      // The word and its pill grow together, to 115 % of rest, from
      // the word's first frame, where the step lands, at rest, and
      // back to rest, however briefly the short lights it, every word
      // alike. Tim saw it stop grown, a frame into the word, with the
      // frame round it at rest, and some words grow by a few pixels on
      // one frame, and then a word of one frame stop at its peak.
      // On the screen's frames, one scale each: Tim found the bounce
      // laggy when it was the short's frames of the pop, 25 a second.
      // The walk gives the bounce a frame every sixtieth of a second,
      // so it is drawn on 15 frames on any machine, from its start to
      // the first frame past its 220 ms, the one nearest its peak
      // within 0.2 % of it, and every one of them is read off the
      // screen. A busy machine drew
      // two before, and missed the peak, issues #328, #329 and #460.
      // That every frame gets its own scale is held by "the bounce
      // after a word step" in captions.test.ts. A word holds that its
      // pill grew by 12 % of the 15 %, its edges read to a whole pixel.
      if (bounced !== 15) {
        walk.wrong = `"${last.keyed.text}" bounced on ${bounced} frames given it a sixtieth of a second apart, where its 220 ms take 15`;
      } else if (most < least * 1.12) {
        walk.wrong = `"${last.keyed.text}" popped, but its pill grew only from ${least.toFixed(1)} to ${most.toFixed(1)} px`;
      } else if (Math.abs(widths[0] - least) > 1.5 || Math.abs(last.pill.w - least) > 1.5) {
        walk.wrong = `"${last.keyed.text}" popped from a pill ${widths[0].toFixed(1)} px wide to one ${last.pill.w.toFixed(1)} px wide, not from rest, ${least.toFixed(1)} px, back to rest`;
      } else {
        // Round its middle, in every direction alike, as Tim asked: the
        // pill at its largest grew as far up as down and as far left
        // as right, and in height by the scale it grew in width. A
        // bounce that grew one way only passed this walk before, found
        // by the second review. Each edge is read to a whole pixel, so
        // the two sides of a pill grown round its middle differ by a
        // pixel or two, 2 up and 1 down here, and a pill grown upward
        // only by 4, 3 up and 1 in.
        const rest = on[0].pill;
        const peak = on[widths.indexOf(most)].pill;
        const up = rest.t - peak.t;
        const down = peak.b - rest.b;
        const left = rest.x - rest.w / 2 - (peak.x - peak.w / 2);
        const right = peak.x + peak.w / 2 - (rest.x + rest.w / 2);
        const tall = (rest.b - rest.t) * (peak.w / rest.w - 1);
        if (Math.abs(up - down) > 2 || Math.abs(left - right) > 2) {
          walk.wrong = `"${last.keyed.text}" grew ${up.toFixed(1)} px up and ${down.toFixed(1)} down, ${left.toFixed(1)} left and ${right.toFixed(1)} right, not round its middle`;
        } else if (Math.abs(up + down - tall) > 2) {
          walk.wrong = `"${last.keyed.text}" grew ${(up + down).toFixed(1)} px in height where its width's scale makes ${tall.toFixed(1)}`;
        }
      }
    }
    // The screen's own frames back for the bounce.
    await page.evaluate(() => {
      window.__captionLayers.frames = window.__screenFrames;
    });
    console.log(`    ${report.join(", ")}`);
    if (!walk.wrong && popped < 2 * arg) walk.wrong = `only ${popped} of ${2 * arg} steps onto a word popped it`;
    else if (!walk.wrong && grew < 1.14) walk.wrong = `no pop grew by more than ${((grew - 1) * 100).toFixed(0)} %`;
  },
  async "step frame by frame and see nothing pop"(walk, arg) {
    const { page, watch } = walk;
    // arg steps of a frame from the clip's first frame, each onto the
    // frame as the short shows it, and nothing played over it, a step
    // onto a new word too.
    const fps = await rate(page);
    const state = await engineState(page, watch.at);
    if (!(await seekTo(page, state.segments[0].start + 0.5 / fps, fps))) {
      walk.wrong = "the video preview never showed the clip's first frame";
      return;
    }
    await page.evaluate(() => window.__captionLayers?.rested());
    await page.evaluate(() => document.body.focus());
    const count = await page.evaluate(() => window.__captionLayers?.stats.popped ?? 0);
    const words = new Set();
    for (let k = 0; k < arg && !walk.wrong; k++) {
      await page.keyboard.press("ArrowRight");
      // Once the frame is drawn with its own layer, which is when a
      // pop would start, and the layers rest.
      await settle(page);
      await shownOnPlayhead(page, { captions: true });
      await page.evaluate(() => window.__captionLayers?.rested());
      const now = await page.evaluate(() => ({
        popped: window.__captionLayers?.stats.popped ?? 0,
        word: document.querySelector(".captions .word.now")?.textContent ?? "",
      }));
      if (now.word) words.add(now.word);
      if (now.popped !== count) walk.wrong = `step ${k + 1} of a frame, onto "${now.word}", played a bounce over the frame`;
    }
    console.log(`    ${arg} frames over ${words.size} words, nothing played over them`);
    if (!walk.wrong && words.size < 2) walk.wrong = `${arg} frames lit only ${words.size} word`;
  },
  // ["frames on the pills", n]
  //                    puts the playhead in the middle of each of the first
  //                    n words with a click on the clip timeline, and reads
  //                    the pill libass drew behind the lit word off the
  //                    canvas: the frame the caption box puts round that
  //                    word stands on it, its middle within 1.5 px of the
  //                    pill's and as wide within 2 px, and the pill is no
  //                    lower than the frame
  async "frames on the pills"(walk, arg) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const state = await engineState(page, watch.at);
    const pieces = state.segments;
    // A moment of the clip's clock in the episode.
    const into = (t) => {
      let sum = 0;
      for (const p of pieces) {
        if (t < sum + p.end - p.start) return p.start + t - sum;
        sum += p.end - p.start;
      }
      return null;
    };
    // Away from the clip's edges, where a click takes the edge, and
    // once the word's pop has settled, 220 ms after it lights: the
    // frame is where the pill rests. In its pop the pill grows round
    // the same middle by up to 15 %.
    // A word stays lit until the next one starts.
    const settled = (w) => w.start + 0.22 + 2 / fps;
    const length = pieces.reduce((n, p) => n + p.end - p.start, 0);
    const words = (state.captions ?? [])
      .flatMap((c) => {
        const ws = c.lines.flatMap((l) => l.words);
        return ws.map((w, i) => ({ ...w, lit: i + 1 < ws.length ? ws[i + 1].start : c.end }));
      })
      .filter((w) => w.start > 0.6 && w.lit < length - 0.6)
      .map((w) => ({ ...w, rests: settled(w) < w.lit - 1 / fps }))
      // The words whose pop settles first, then words in their pop.
      .sort((a, b) => Number(b.rests) - Number(a.rests))
      .slice(0, arg);
    if (words.filter((w) => w.rests).length < 2) {
      walk.wrong = `only ${words.filter((w) => w.rests).length} words are lit long enough for their pop to settle`;
      return;
    }
    const report = [];
    for (const w of words) {
      const at = into(w.rests ? Math.max((w.start + w.lit) / 2, settled(w)) : (w.start + w.lit) / 2);
      if (at === null) continue;
      if (!(await seekTo(page, at, fps))) {
        walk.wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
        break;
      }
      await page.evaluate(() => window.__captionLayers?.rested());
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
      const got = await page.evaluate(() => {
        const word = document.querySelector(".captions .word.now");
        const canvas = document.querySelector(".screen canvas");
        if (!word || !canvas?.width) return { none: !word ? "no word is lit" : "there is no canvas" };
        const c = canvas.getBoundingClientRect();
        const f = word.getBoundingClientRect();
        const sx = canvas.width / c.width;
        const sy = canvas.height / c.height;
        const pill = getComputedStyle(word).getPropertyValue("--pill").match(/[\d.]+/g).map(Number);
        // Round the frame by its own height, so a pill beside it is
        // found too, but no other word's.
        const x0 = Math.max(0, Math.floor((f.left - c.left - f.height / 2) * sx));
        const x1 = Math.min(canvas.width, Math.ceil((f.right - c.left + f.height / 2) * sx));
        const y0 = Math.max(0, Math.floor((f.top - c.top - f.height) * sy));
        const y1 = Math.min(canvas.height, Math.ceil((f.bottom - c.top + f.height) * sy));
        const s = document.createElement("canvas");
        s.width = x1 - x0;
        s.height = y1 - y0;
        const g = s.getContext("2d", { willReadFrequently: true });
        g.drawImage(canvas, x0, y0, s.width, s.height, 0, 0, s.width, s.height);
        const d = g.getImageData(0, 0, s.width, s.height).data;
        let [l, r, t, b] = [Infinity, -Infinity, Infinity, -Infinity];
        for (let y = 0; y < s.height; y++) {
          for (let x = 0; x < s.width; x++) {
            const i = (y * s.width + x) * 4;
            if ([0, 1, 2].every((k) => Math.abs(d[i + k] - pill[k]) < 40)) {
              [l, r, t, b] = [Math.min(l, x), Math.max(r, x + 1), Math.min(t, y), Math.max(b, y + 1)];
            }
          }
        }
        if (l === Infinity) return { none: "no pill is drawn round the lit word" };
        // Both in CSS pixels from the canvas's corner.
        return {
          text: word.textContent,
          frame: { l: f.left - c.left, r: f.right - c.left, t: f.top - c.top, b: f.bottom - c.top },
          pill: { l: (x0 + l) / sx, r: (x0 + r) / sx, t: (y0 + t) / sy, b: (y0 + b) / sy },
        };
      });
      if (got.none) {
        walk.wrong = `at ${at.toFixed(2)} s ${got.none}`;
        break;
      }
      const { frame, pill, text } = got;
      const mid = (a, b) => (a + b) / 2;
      const across = mid(frame.l, frame.r) - mid(pill.l, pill.r);
      const down = mid(frame.t, frame.b) - mid(pill.t, pill.b);
      const wider = frame.r - frame.l - (pill.r - pill.l);
      const lower = frame.b - frame.t - (pill.b - pill.t);
      report.push(`"${text}"${w.rests ? "" : " in its pop"}: ${across.toFixed(1)} px across, ${down.toFixed(1)} px down, ${wider.toFixed(1)} px wider, ${lower.toFixed(1)} px taller`);
      // A word lit too briefly for its pop to settle is read in its
      // pop, where the frame grows with the pill round its middle.
      if (Math.abs(across) > 1.5 || Math.abs(down) > 1.5 || Math.abs(wider) > 2 || Math.abs(lower) > 2) {
        walk.wrong = `the frame round "${text}" stands ${across.toFixed(1)} px across and ${down.toFixed(1)} px down from the middle of its pill, and is ${(frame.r - frame.l).toFixed(1)}x${(frame.b - frame.t).toFixed(1)} px round a pill of ${(pill.r - pill.l).toFixed(1)}x${(pill.b - pill.t).toFixed(1)}`;
        break;
      }
    }
    if (!walk.wrong && report.length < Math.min(arg, words.length)) walk.wrong = `only ${report.length} words were measured`;
    console.log(`    ${report.join("\n    ")}`);
  },
  // ["open a word, then the mark", kind]
  //                    clicks the first "thumbnail" or "crop" mark on the
  //                    clip timeline and reads the caption there, clicks
  //                    the clip timeline at the other end of the clip and
  //                    opens the first word of the caption there, then
  //                    clicks the mark again: the playhead is on the mark,
  //                    no word is open, and the caption box holds the
  //                    caption on the mark again, not the one the word was
  //                    opened in
  async "open a word, then the mark"(walk, arg) {
    const { page, watch } = walk;
    const kind = arg === "crop" ? ".cropdot" : ".thumb";
    const mark = await page.locator(`.clip-timeline ${kind}`).first().boundingBox();
    if (!mark) {
      walk.wrong = `the clip timeline shows no ${arg} mark`;
      return;
    }
    const mx = mark.x + mark.width / 2;
    const my = mark.y + mark.height / 2;
    const playhead = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
    const words = async () => (await shown(page)).map((w) => w.at).join(" ");
    // The caption box shows the caption drawn on the frame on screen,
    // which on a busy machine comes a moment after the playhead, so
    // each caption is read once its frame is up with its captions. Read
    // when the calls had settled, the caption on the mark was now and
    // then the caption still up from the click before, issue #359.
    const clickUp = async (x, y, where) => {
      const lost = await captionsLost(page);
      await clickOnTrack(page, x, y);
      await settle(page);
      if (!(await shownOnPlayhead(page, { captions: true, lost }))) {
        walk.wrong = `after the click on ${where}, a layer of captions was given up on`;
        return false;
      }
      return true;
    };
    if (!(await clickUp(mx, my, `the ${arg} mark`))) return;
    const onMark = await playhead();
    const there = await words();
    const g = await handles(page);
    const from = g.start.x + g.start.w;
    const other = mx < (from + g.end.x) / 2 ? 0.75 : 0.25;
    if (!(await clickUp(from + other * (g.end.x - from), high(g.track), "the clip away from the mark"))) return;
    const first = page.locator(".captions .word.correctable").first();
    if (!(await first.count())) {
      walk.wrong = "no word in the caption box to open";
      return;
    }
    const before = await shown(page);
    await first.click();
    await watch.step("click", before);
    if (!(await shown(page)).some((w) => w.focused)) {
      walk.wrong = "the word clicked did not open";
      return;
    }
    if ((await words()) === there) {
      walk.wrong = "the caption on the mark is the caption elsewhere too, so this proves nothing";
      return;
    }
    if (!(await clickUp(mx, my, `the ${arg} mark with the word open`))) return;
    // The caption box follows the frame on screen, the way it does
    // after a click on the clip timeline, so with the mark's frame up
    // with its captions it shows the caption on the mark.
    const followed = await page.evaluate(
      (there) => {
        const words = [...document.querySelectorAll(".captions .word")];
        return (
          !document.querySelector(".captions.typing") &&
          !words.includes(document.activeElement) &&
          words.map((w) => Number(w.dataset.at)).join(" ") === there
        );
      },
      there,
    );
    const now = await shown(page);
    const still = now.find((w) => w.focused);
    const at = await playhead();
    if (Math.abs(at - onMark) > 0.05) walk.wrong = `a click on the ${arg} mark put the playhead at ${at}, not on the mark at ${onMark}`;
    else if (still) walk.wrong = `after a click on the ${arg} mark, "${still.text}" is still open`;
    else if (!followed)
      walk.wrong = `after a click on the ${arg} mark, the caption box holds "${now.map((w) => w.text).join(" ")}", not the caption on the mark`;
  },
};

// Steps that take the same body as the step they are walked by.
steps["edge in"] = steps["edge to word"];
steps["edge out"] = steps["edge to word"];
steps["edge nudge"] = steps["edge to word"];
steps["count pops"] = steps["pops"];
steps["gone last"] = steps["gone first"];
steps["font"] = steps["style"];

// How many layers of captions the video preview has given up on, as
// data-captions-lost says, read before a gesture for shownOnPlayhead.
function captionsLost(page) {
  return page.locator(".screen").evaluate((screen) => screen.dataset.captionsLost ?? "");
}

// How far the captions in the crop of the video preview are from a frame
// of the short at the same size, in brightness, on average over every
// pixel and at most, in steps of 8 bits. The captions are the rows of the
// lower half of the short that have white text in them, and a few pixels
// round them. The short is given the video preview's look first, see lift
// in lib/frames/light.ts. The two are laid on each other a pixel either
// way too and the closest is taken: libass puts a drawing on whole pixels,
// and the short, made smaller to compare, lands half a pixel from where
// the preview draws it at the size it shows it.
const look = (v) => {
  const c = Math.min(Math.max(v / 255, 0), 1);
  return 255 * Math.pow(c, Math.max(0.891, 0.98 - 0.31 * c));
};

function captionDiff(rgba, rgb, w, h, channels = 4) {
  const luma = (d, i, n, f) => 0.2126 * f(d[n * i]) + 0.7152 * f(d[n * i + 1]) + 0.0722 * f(d[n * i + 2]);
  const a = new Float32Array(w * h);
  const b = new Float32Array(w * h);
  for (let i = 0; i < w * h; i++) {
    a[i] = luma(rgba, i, channels, channels === 3 ? look : (v) => v);
    b[i] = luma(rgb, i, 3, look);
  }
  let top = h;
  let bottom = -1;
  for (let y = Math.floor(h / 2); y < h; y++) {
    for (let x = 0; x < w; x++) {
      if (luma(rgb, y * w + x, 3, (v) => v) > 200) {
        top = Math.min(top, y);
        bottom = Math.max(bottom, y);
      }
    }
  }
  if (bottom < 0) return { mean: Infinity, most: Infinity };
  let best = { mean: Infinity, most: Infinity };
  for (let dy = -1; dy <= 1; dy++) {
    for (let dx = -1; dx <= 1; dx++) {
      let sum = 0;
      let most = 0;
      let n = 0;
      for (let y = Math.max(1, top - 8); y < Math.min(h - 1, bottom + 8); y++) {
        for (let x = 1; x < w - 1; x++) {
          const d = Math.abs(a[y * w + x] - b[(y + dy) * w + x + dx]);
          sum += d;
          most = Math.max(most, Math.round(d));
          n++;
        }
      }
      if (sum / n < best.mean) best = { mean: sum / n, most };
    }
  }
  return best;
}

// Reads, on every frame the browser paints, the pill libass drew off the
// canvas and the frames the caption box puts round words: the lit word's,
// now, and the keyboard's, keyed. The pill is looked for in a band round
// the caption box, so a pill of a caption the box does not hold is found
// too. Says how to stop, which hands back what was read.
async function watchPills(page) {
  await page.evaluate(() => {
    // Its own, so a watch that was stopped stops at its next frame even
    // when the next watch has begun by then. It read the new watch's on
    // before and went on, so every step of a walk added a reading of the
    // canvas to every frame of the screen, and on a busy machine the
    // screen drew a bounce on fewer frames each step, issues #328 and #329.
    const watch = { frames: [], on: true };
    window.__pillWatch = watch;
    const s = document.createElement("canvas");
    const g = s.getContext("2d", { willReadFrequently: true });
    const look = () => {
      if (!watch.on) return;
      requestAnimationFrame(look);
      const box = document.querySelector(".captions");
      const canvas = document.querySelector(".screen canvas");
      if (!box || !canvas?.width) return;
      const pill = (getComputedStyle(box.querySelector(".word") ?? box).getPropertyValue("--pill").match(/[\d.]+/g) ?? []).map(Number);
      if (pill.length < 3) return;
      const c = canvas.getBoundingClientRect();
      const b = box.getBoundingClientRect();
      const sx = canvas.width / c.width;
      const sy = canvas.height / c.height;
      const y0 = Math.max(0, Math.floor((b.top - c.top - b.height) * sy));
      const y1 = Math.min(canvas.height, Math.ceil((b.bottom - c.top + b.height) * sy));
      if (y1 <= y0) return;
      s.width = canvas.width;
      s.height = y1 - y0;
      g.drawImage(canvas, 0, y0, s.width, s.height, 0, 0, s.width, s.height);
      const d = g.getImageData(0, 0, s.width, s.height).data;
      let [l, r, t, bt] = [Infinity, -Infinity, Infinity, -Infinity];
      for (let y = 0; y < s.height; y++) {
        for (let x = 0; x < s.width; x++) {
          const i = (y * s.width + x) * 4;
          if ([0, 1, 2].every((k) => Math.abs(d[i + k] - pill[k]) < 40)) {
            [l, r, t, bt] = [Math.min(l, x), Math.max(r, x + 1), Math.min(t, y), Math.max(bt, y + 1)];
          }
        }
      }
      const frame = (word) => {
        if (!word) return null;
        const f = word.getBoundingClientRect();
        return {
          text: word.textContent,
          x: (f.left + f.right) / 2 - c.left,
          y: (f.top + f.bottom) / 2 - c.top,
          t: f.top - c.top,
          b: f.bottom - c.top,
          w: f.right - f.left,
        };
      };
      watch.frames.push({
        at: Number(document.querySelector(".screen").dataset.playhead),
        pictured: window.__pictured?.(),
        t: performance.now(),
        late: window.__captionLayers?.late ?? false,
        bounced: window.__captionLayers?.stats.bounced ?? 0,
        now: frame(box.querySelector(".word.now")),
        keyed: frame(box.querySelector(".word.keyed")),
        pill:
          l === Infinity
            ? null
            : { x: (l + r) / 2 / sx, y: (y0 + (t + bt) / 2) / sy, t: (y0 + t) / sy, b: (y0 + bt) / sy, w: (r - l) / sx },
      });
    };
    requestAnimationFrame(look);
  });
  return () =>
    page.evaluate(() => {
      window.__pillWatch.on = false;
      return window.__pillWatch.frames;
    });
}

// What is wrong on a frame read by watchPills with the frame round a word,
// the lit word's, "now", or the keyboard's, "keyed", or null. A lit word
// stands on the pill, and where a pill is drawn a word is lit. The
// keyboard's frame, where there is one, stands on the pill too.
// How long the video preview showed captions not their own, read off
// what watchPills saw: the paints made with the layer of a frame before,
// the frame's own still on its way, out of all, and the longest time they
// went on one after another, in milliseconds.
function laggingFrames(frames) {
  let late = 0;
  let since = null;
  let most = 0;
  for (const f of frames) {
    if (f.late) {
      late++;
      since ??= f.t;
      most = Math.max(most, f.t - since);
    } else since = null;
  }
  return { painted: frames.length, late, ms: most };
}

function framesOffPill(f, which) {
  const word = f[which];
  const where = `at ${f.at.toFixed(3)} s, frame ${f.pictured} on the video preview`;
  if (which === "now" && f.pill && !word) return `${where}, a pill is drawn and no word is lit in the caption box`;
  if (!word) return null;
  if (!f.pill) return `${where}, no pill is drawn and the frame stands round "${word.text}"`;
  // Across, the frame stands on the middle of the pill. Down, it stands
  // inside it, and it is as wide as the pill: the pill pops round the
  // frame's middle and the frame grows with it. Tim saw the frame stay at
  // rest round a pill still grown.
  const across = word.x - f.pill.x;
  const above = f.pill.t - word.t;
  const below = word.b - f.pill.b;
  const narrower = f.pill.w - word.w;
  if (Math.abs(across) > 3 || above > 3 || below > 3 || Math.abs(narrower) > 3) {
    return `${where}, the frame round "${word.text}" stands ${across.toFixed(1)} px across from the pill, ${above.toFixed(1)} px over its top and ${below.toFixed(1)} px under its bottom, and is ${narrower.toFixed(1)} px narrower`;
  }
  return null;
}

// Puts the playhead on a clip that starts on a caption, adds another
// video, and goes back to the first from the sidebar with its frames slow
// to open, the way a video of hours opens them, and on every animation
// frame reads the video preview in front. The workspace woken keeps the
// frame and the captions it had until its own frame is drawn again, and
// never shows a black canvas between them. Tim saw it go black for a
// frame on the video whose frame had captions, in #385. Then a card of
// the video gone back to is put up from what was read ahead, which it was
// not until the pointer came to another card, found by the review of
// #385.
async function wakeOnCaption(page) {
  const clips = await shotClips(page);
  if (typeof clips === "string") return clips;
  await canLook(page);
  const clip = clips.find((c) => Math.abs(c.start - 8) < 2) ?? clips[0];
  const other = clips.find((c) => Math.abs(c.start - 86) < 2) ?? clips.find((c) => c !== clip);
  await clickCard(page, clip.key, null);
  const first = await episodeOn(page);
  const name = basename(first);
  await control(url, "/pick?seconds=20");
  const front = page.evaluate(
    (name) =>
      new Promise((done) => {
        // The video added is in front once its workspace is the only one
        // in the page, the one before put to sleep, see App.svelte.
        const there = () => {
          const on = document.querySelector("aside button.episode.current .name")?.textContent;
          return on && on !== name && document.querySelectorAll(".workspace").length === 1 && !document.querySelector(".workspace.behind");
        };
        const o = new MutationObserver(() => {
          if (!there()) return;
          o.disconnect();
          done();
        });
        o.observe(document.body, { subtree: true, childList: true, characterData: true, attributes: true });
      }),
    name,
  );
  await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
  await front;
  await restAhead(page);
  await page.evaluate(() => {
    for (const [i, w] of [...document.querySelectorAll(".workspace")].entries()) w.dataset.walk = String(i);
    window.__looks = [];
    window.__looking = true;
    const step = () => {
      if (!window.__looking) return;
      const w = document.querySelector(".workspace.front");
      const c = w?.querySelector(".screen canvas");
      const look = { at: performance.now(), front: w?.dataset.walk ?? "", canvas: 0, light: 0, captions: !!w?.querySelector(".screen .captions") };
      if (c?.width && c.height) {
        const s = (window.__lookCanvas ??= document.createElement("canvas"));
        s.width = c.width;
        s.height = c.height;
        const t = s.getContext("2d", { willReadFrequently: true });
        t.drawImage(c, 0, 0);
        const d = t.getImageData(0, 0, c.width, c.height).data;
        let h = 2166136261;
        for (let i = 0; i < d.length; i += 4) {
          h = Math.imul(h ^ (d[i] | (d[i + 1] << 8) | (d[i + 2] << 16)), 16777619) >>> 0;
          look.light = Math.max(look.light, d[i], d[i + 1], d[i + 2]);
        }
        look.canvas = h;
      }
      window.__looks.push(look);
      requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  });
  // The frames of the video gone back to open only once the page has drawn
  // twenty animation frames since they were asked, about 330 ms. That is
  // a slow Go side put on the app on purpose, not a wait of the walk's:
  // the woken video preview has no frame of its own for that long, which
  // is when what lies over it was drawn over nothing and went black.
  const slow = async (route) => {
    await nextPaint(page, 20).catch(() => {});
    await route.continue().catch(() => {});
  };
  await page.route(/\/frames\/open\?/, slow);
  try {
    await fromSidebar(page, () => page.locator("aside li", { hasText: name }).first().click());
    await restAhead(page);
    // Until the woken video preview stands on its own frame, which comes
    // only once the slowed frames have opened, and the frames drawn then.
    await settle(page);
    await page.evaluate(() => window.__standing());
    await nextPaint(page, 2);
  } finally {
    await page.unroute(/\/frames\/open\?/, slow);
  }
  const looks = await page.evaluate(() => {
    window.__looking = false;
    return window.__looks;
  });
  const was = looks[0]?.front;
  const i = looks.findIndex((l) => l.front !== was && l.canvas);
  if (i < 0) return `the video preview of ${name} never came in front`;
  const woken = looks.slice(i).filter((l) => l.front === looks[i].front);
  const t0 = woken[0].at;
  const say = (l) => `${Math.round(l.at - t0)} ms`;
  if (woken[0].light < 32) return `${name} came back with a black canvas`;
  if (!woken.at(-1).captions) return `${name} came back on a frame with no caption, so a redraw for its captions cannot be seen`;
  const dark = woken.filter((l) => l.light < 32);
  if (dark.length) return `the video preview went black for ${dark.length} animation frames, from ${say(dark[0])} after ${name} came back`;
  const changes = woken.filter((l, j) => j > 0 && l.canvas !== woken[j - 1].canvas);
  if (changes.length > 1) return `the video preview changed ${changes.length} times after ${name} came back: ${changes.map(say).join(", ")}`;
  console.log(`        ${name} came back on its own frame, ${woken.length} animation frames read, ${changes.length} changes`);
  return clickedFromAhead(page, other.key, other.start, `after ${name} came back, the card of the clip at ${other.start.toFixed(2)} s`);
}
