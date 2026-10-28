// The steps of the sequences about the clip list: cards, clips made,
// removed and put back, and the shorts rendered from them. Each is said
// above it, a list with its name first. walkSequence in ../sequences.mjs
// walks it, and hands it the walk so far, the step's argument, what the
// caption box showed before it and the step's name. A sequence of any
// folder can take it. Below them the helpers only these steps use. See
// docs/TESTING.md.
import { existsSync, readdirSync, realpathSync, rmSync, statSync } from "node:fs";
import { basename, dirname, extname, join } from "node:path";
import { ask, caughtUp, chosen, clickOnTrack, clipList, control, engineState, episodeOn, foldsDone, fromSidebar, handles, high, holdClock, letClockGo, middle, news, nextNews, nextPaint, overlay, pageUntil, playUntil, pressHead, realFrames, recordFrames, runClock, seenUntil, settle, shortFrames, shortSound } from "../bridge.mjs";
import { canLook, changesOf, clickCard, clickedFromAhead, clockOf, halfOf, newCards, pieceCount, rate, restAhead, shortOf, shotClips, url } from "../common/steps.mjs";

export const steps = {
  // ["clips with", [word, n]]
  //                    n clips of the episode on screen, of those not
  //                    removed, have a caption with this very word
  async "clips with"(walk, arg) {
    const { page } = walk;
    const [word, n] = arg;
    const path = await episodeOn(page);
    const bare = (text) => text.replace(/[^\p{L}\p{N}]/gu, "");
    let have = 0;
    for (const c of (await ask(page, "Clips", path)).filter((c) => !c.rejected)) {
      const view = await ask(page, "Captions", path, c.plan, c.id);
      if (view.captions.some((cap) => cap.lines.some((l) => l.words.some((w) => bare(w.text) === word)))) have++;
    }
    if (have !== n) walk.wrong = `${have} clips have "${word}", not ${n}`;
  },
  // ["make while paused", key]
  //                    the same with the video paused, and no play
  // ["make while playing", key]
  //                    clicks the clip timeline before the clip, which
  //                    puts the playhead on the video, presses the space
  //                    bar, and a second and a half into the play presses
  //                    "i" or "o", Mark In or Mark Out, which makes a clip
  //                    there while the episode plays on. It reads what the
  //                    clip timeline and the video preview show on every
  //                    animation frame until the clip made is written and
  //                    chosen, and two seconds after, then pauses
  async "make while playing"(walk, arg, s, what) {
    const { page, watch } = walk;
    const playing = what === "make while playing";
    const g = await handles(page);
    if (g.start.x - g.track.x < 40) {
      walk.wrong = "there is no room on the clip timeline before the clip";
      return;
    }
    await clickOnTrack(page, (g.track.x + g.start.x) / 2, high(g.track));
    await settle(page);
    const path = await episodeOn(page);
    const was = (await overlay(page)).card;
    const stop = await recordFrames(page, 1 / (await rate(page)));
    await page.evaluate(() => document.body.focus());
    // The play's waits are in the episode's time, read off the
    // playhead, see playUntil.
    if (playing) {
      await page.keyboard.press("Space");
      await settle(page);
      await playUntil(page, { most: 1.5 });
    }
    await page.keyboard.press(arg);
    // Until the card chosen is a clip the engine has written, the one
    // made, which is not the one chosen before.
    const chosenCard = async () => {
      const card = (await overlay(page)).card;
      return card && card !== was && (await ask(page, "Clips", path)).some((c) => c.key === card) ? card : "";
    };
    const written = await seenUntil(page, url, chosenCard, (card) => !!card);
    // And two seconds more of the play. Paused, until the app has done
    // all the clip made set going and the picture stands, and the
    // frames drawn then: a clip dimmed after it was written is dimmed
    // by then, and nothing is on its way to dim it later.
    if (playing) await playUntil(page, { most: 2 });
    else {
      await settle(page);
      await page.evaluate(() => window.__standing());
      await nextPaint(page, 2);
    }
    await page.evaluate(() => {
      if (document.querySelector('button[aria-label="Play"]')) return;
      const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
      document.body.dispatchEvent(new KeyboardEvent("keydown", key));
      document.body.dispatchEvent(new KeyboardEvent("keyup", key));
    });
    await settle(page);
    walk.made = { playing, was, written, overlays: (await stop()).overlays };
    await watch.step("play", s);
  },
  // ["render"]         presses Render and waits until the short is written
  async render(walk, arg, s) {
    const { page, watch } = walk;
    const on = await chosen(page);
    await pageUntil(
      page,
      (r) => {
        const b = document.querySelector(r);
        return b && !b.disabled && b.textContent.trim().startsWith("Render");
      },
      "button.primary.render",
    );
    const renders = async () => (await ask(page, "Jobs")).filter((j) => j.kind === "render" && j.episode === on.path);
    const before = (await renders()).length;
    let heard = await news(page);
    await page.locator("button.primary.render").click();
    // Until the render has ended, read each time a job has news.
    let job = null;
    for (;;) {
      job = (await renders()).slice(before).find((j) => j.state === "done" || j.state === "failed") ?? null;
      if (job) break;
      heard = await nextNews(page, heard);
    }
    if (job.state !== "done") walk.wrong = `Render failed: ${job.error}`;
    // And until the interface has heard that news too, which is where
    // the Render button reads that the render ended.
    await caughtUp(page, url, [job]);
    await watch.step("render", s);
  },
  // ["head"]           presses the clip list's head button, Search, Cancel or
  //                    Continue
  async head(walk, arg, s) {
    const { page, watch } = walk;
    await pressHead(page);
    await watch.step("head", s);
  },
  // ["card says"]      the chosen clip's card says where the clip starts,
  //                    how long it comes out and in how many pieces, the
  //                    engine's numbers and as many pieces as the clip
  //                    timeline draws
  async "card says"(walk) {
    const { page, watch } = walk;
    const { segments } = await engineState(page, watch.at);
    const seconds = Math.round(segments.reduce((sum, p) => sum + p.end - p.start, 0));
    const n = await page.locator(".clip-timeline .piece").count();
    const want = `${clockOf(segments[0].start)}, ${seconds} s, ${n} ${n === 1 ? "piece" : "pieces"}`;
    await settle(page);
    const ok = await page.evaluate((want) => document.querySelector(".pick.current .meta")?.textContent.replace(/\s+/g, " ").trim() === want, want);
    if (!ok) {
      const got = await page.evaluate(() => document.querySelector(".pick.current .meta")?.textContent.replace(/\s+/g, " ").trim() ?? "nothing");
      walk.wrong = `the chosen clip's card says "${got}", not "${want}"`;
    }
    if (!walk.wrong && n !== segments.length) walk.wrong = `the clip timeline draws ${pieceCount(n)} and the engine keeps ${segments.length}`;
  },
  // ["card follows", px]
  //                    drags the clip's end by px with the button held,
  //                    and the chosen clip's card says the clip's new
  //                    length before the hand lets go
  async "card follows"(walk, arg, s) {
    const { page, watch } = walk;
    const card = () => page.evaluate(() => document.querySelector(".pick.current .meta")?.textContent.replace(/\s+/g, " ").trim() ?? "");
    const before = await card();
    const g = await handles(page);
    const x = middle(g.end);
    const y = high(g.track);
    await page.mouse.move(x, y);
    await page.mouse.down();
    for (let i = 1; i <= 6; i++) {
      await page.mouse.move(x + (arg * i) / 6, y);
      await nextPaint(page);
    }
    await nextPaint(page, 2);
    const held = await card();
    await page.mouse.up();
    if (held === before) walk.wrong = `with the clip's end dragged ${arg} px and the button still held, the card still says "${before}"`;
    await watch.step("trim", s);
  },
  // ["row", words]     a row of the clip list says this
  async row(walk, arg) {
    const { page } = walk;
    const list = await clipList(page);
    if (!list?.rows.some((r) => r.what === arg)) walk.wrong = `no row says "${arg}": ${JSON.stringify(list?.rows)}`;
  },
  // ["no row", words] no row of the clip list says this
  async "no row"(walk, arg) {
    const { page } = walk;
    const list = await clipList(page);
    if (list?.rows.some((r) => r.what === arg)) walk.wrong = `a row says "${arg}": ${JSON.stringify(list?.rows)}`;
  },
  // ["no put back"]    no row of the clip list offers a removed clip back
  async "no put back"(walk) {
    const { page } = walk;
    const offered = await page.evaluate(
      () => [...document.querySelectorAll("aside ol button.back")].filter((b) => b.textContent.includes("Put it back")).length,
    );
    if (offered) walk.wrong = `${offered} removed clip(s) are offered back`;
  },
  // ["row closes", [words, s]]
  //                    a row of the clip list says this and runs down in s
  //                    seconds. The clock is held, and the row is still
  //                    there half a second before its time runs out and
  //                    gone half a second after, then the clock goes on
  async "row closes"(walk, arg) {
    const { page } = walk;
    const [words, after] = arg;
    // A row on its way out has gone, though it is still in the list
    // for its fold: Svelte marks it inert the moment it starts to go.
    const says = async () => !!(await clipList(page))?.rows.some((r) => r.what === words && !r.leaving);
    if (!(await says())) {
      walk.wrong = `no row says "${words}": ${JSON.stringify((await clipList(page))?.rows)}`;
      return;
    }
    const drains = await page.evaluate(() => !!document.querySelector("aside ol li.told .fill .run"));
    if (!drains) {
      walk.wrong = `the row that says "${words}" does not run down`;
      return;
    }
    if (!page.__clock) {
      walk.wrong = "the sequence did not begin with the fake clock, so the row's time runs on the real one";
      return;
    }
    // Its time started when the search ended, on the fake clock the
    // sequence began with, see fakeClock. The clock is held now, and
    // what is left of the time is read from the row's fill, which
    // started with it and runs on the browser's own clock. Then the
    // time is run on by hand, so nothing waits on it.
    await holdClock(page);
    try {
      const time = await page.evaluate(() => {
        const t = document.querySelector("aside ol li.told .fill .run")?.getAnimations()[0]?.effect.getComputedTiming();
        return t ? { whole: t.duration, left: t.duration - t.localTime } : null;
      });
      if (!time) {
        walk.wrong = `the row that says "${words}" does not run down`;
        return;
      }
      if (Math.abs(time.whole - after * 1000) > 50) {
        walk.wrong = `the row that says "${words}" runs down in ${(time.whole / 1000).toFixed(1)} s, not ${after}`;
        return;
      }
      await runClock(page, Math.max(0, time.left - 500));
      if (!(await says())) {
        walk.wrong = `the row that says "${words}" closed half a second before its ${after} s ran out`;
        return;
      }
      await runClock(page, 1000);
      if (await says()) walk.wrong = `the row that says "${words}" is still there half a second after its ${after} s`;
      else await foldsDone(page);
    } finally {
      await letClockGo(page);
    }
  },
  // ["cards", n]       the clip list holds n clips
  async cards(walk, arg) {
    const { page } = walk;
    const list = await clipList(page);
    if (list?.cards.length !== arg) walk.wrong = `the clip list holds ${list?.cards.length} clips, not ${arg}`;
  },
  // ["whole clip", s]  the episode on screen, s seconds long, has one clip
  //                    in the list, and it runs from its first second to
  //                    its last
  async "whole clip"(walk, arg) {
    const { page } = walk;
    const clips = (await ask(page, "Clips", await episodeOn(page)))?.filter((c) => !c.rejected);
    if (clips?.length !== 1) walk.wrong = `the episode has ${clips?.length} clips, not its one`;
    else if (clips[0].start > 1 || clips[0].end < arg - 1.5) {
      walk.wrong = `the clip runs ${clips[0].start} to ${clips[0].end}, not the episode of ${arg} s`;
    }
  },
  async "walk the clip list"(walk, arg, s) {
    const { page, watch } = walk;
    // Shift and the arrows walk the clip list the way they walk the
    // transcript, turning the page: a clip walked down to below the
    // foot of the list comes to the top, and one walked up to above
    // its top comes to the foot. It was brought just into view, and
    // went on standing at the foot as the walk went down, found by
    // Tim. Three clips are made by hand after the one there is, so the
    // list has pages, and the list is made two cards high.
    const path = await episodeOn(page);
    const have = (await ask(page, "Clips", path)) ?? [];
    const after = Math.max(0, ...have.map((c) => c.end));
    for (let k = 1; k <= 3; k++) await ask(page, "MakeClip", path, after + 30 * k, false);
    const cards = "aside ol li[data-key]:not(.next)";
    await pageUntil(page, ({ cards, n }) => document.querySelectorAll(cards).length >= n, { cards, n: have.length + 3 });
    await settle(page);
    const keys = await page.evaluate((cards) => [...document.querySelectorAll(cards)].map((li) => li.dataset.key), cards);
    await page.locator(`aside ol li[data-key="${keys[0]}"] button.pick`).first().click();
    await watch.step("click", s);
    await settle(page);
    // The same veil the list fades its ends with, see showChosen.
    const veil = 16;
    await page.evaluate(
      ({ cards, veil }) => {
        const list = document.querySelector(".pane .list");
        const [one, two] = document.querySelectorAll(cards);
        list.scrollTop += one.getBoundingClientRect().top - list.getBoundingClientRect().top - veil;
        const high = two.getBoundingClientRect().bottom - one.getBoundingClientRect().top;
        list.style.flex = "none";
        list.style.height = `${Math.ceil(high + 2 * veil)}px`;
        list.scrollTop += one.getBoundingClientRect().top - list.getBoundingClientRect().top - veil;
      },
      { cards, veil },
    );
    await page.evaluate(() => document.activeElement?.blur());
    // Where a card stands in the list once the list has had a frame
    // to follow the clip chosen, from its top and from its foot.
    const stood = (key) =>
      page.evaluate(
        ({ key, veil }) =>
          new Promise((done) =>
            requestAnimationFrame(() =>
              requestAnimationFrame(() => {
                const list = document.querySelector(".pane .list");
                const box = list.getBoundingClientRect();
                const r = document.querySelector(`aside ol li[data-key="${CSS.escape(key)}"]`).getBoundingClientRect();
                const chosen = document.querySelector("aside ol li .pick.current")?.closest("li")?.dataset.key ?? null;
                done({ top: r.top - (box.top + veil), foot: box.bottom - veil - r.bottom, scroll: list.scrollTop, chosen });
              }),
            ),
          ),
        { key, veil },
      );
    const press = async (key) => {
      await page.keyboard.press(key);
      await watch.step("press", s);
      await settle(page);
    };
    const before = await stood(keys[0]);
    await press("Shift+ArrowDown");
    const second = await stood(keys[1]);
    await press("Shift+ArrowDown");
    const down = await stood(keys[2]);
    await press("Shift+ArrowUp");
    const up = await stood(keys[1]);
    await page.evaluate(() => {
      const list = document.querySelector(".pane .list");
      list.style.flex = "";
      list.style.height = "";
    });
    console.log(JSON.stringify({ keys, before, second, down, up }));
    if (second.chosen !== keys[1]) walk.wrong = `Shift and the down arrow chose ${second.chosen}, not ${keys[1]}`;
    else if (Math.abs(second.scroll - before.scroll) > 0.5) walk.wrong = `the clip walked to in view moved the clip list ${(second.scroll - before.scroll).toFixed(0)} px`;
    else if (down.chosen !== keys[2]) walk.wrong = `Shift and the down arrow twice chose ${down.chosen}, not ${keys[2]}`;
    else if (Math.abs(down.top) > 1)
      walk.wrong = `the clip walked down to below the foot of the clip list stands ${down.top.toFixed(0)} px below the top, not turned to the top`;
    else if (up.chosen !== keys[1]) walk.wrong = `Shift and the up arrow chose ${up.chosen}, not ${keys[1]}`;
    else if (Math.abs(up.foot) > 1)
      walk.wrong = `the clip walked up to above the top of the clip list stands ${up.foot.toFixed(0)} px above the foot, not turned to the foot`;
  },
  async "walk two at once"(walk, arg, s) {
    const { page, watch } = walk;
    // Two clips made by hand at the same moment, past the clips there
    // are, so each has a card of the same words, beginning together.
    // Shift and the down arrow from the card before them goes onto
    // each and then past them, and Shift and the up arrow back.
    const path = await episodeOn(page);
    const have = (await ask(page, "Clips", path)) ?? [];
    const at = Math.max(0, ...have.map((c) => c.end)) + 30;
    await ask(page, "MakeClip", path, at, false);
    await ask(page, "MakeClip", path, at, false);
    await pageUntil(page, (n) => document.querySelectorAll("aside ol li[data-key]:not(.next)").length >= n, have.length + 2);
    await settle(page);
    await page.mouse.move(1, 1);
    await page.evaluate(() => document.querySelector('.listhead button[aria-label="Search the transcript"]').click());
    await watch.step("click", s);
    await page.waitForSelector("ol.said li[data-clip]");
    const pair = await page.evaluate(() => {
      const cards = [...document.querySelectorAll("ol.said li[data-row]")];
      const i = cards.findIndex(
        (li, i) => i > 0 && cards[i + 1] && li.dataset.clip && cards[i + 1].dataset.clip && li.dataset.clip !== cards[i + 1].dataset.clip && li.dataset.at === cards[i + 1].dataset.at,
      );
      return i < 0 ? null : [cards[i - 1], cards[i], cards[i + 1], cards[i + 2]].map((li) => li?.dataset.row ?? null);
    });
    if (!pair) {
      walk.wrong = "no two cards of two clips that begin at the same moment";
      return;
    }
    // A step is done once the list has had a frame to follow.
    const followed = () =>
      page.evaluate(() => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(() => done(document.querySelector("ol.said").dataset.followed)))));
    await page.locator(`ol.said li[data-row="${pair[0]}"] button.pick`).click();
    await watch.step("click", s);
    const went = [await followed()];
    await page.mouse.move(1, 1);
    for (const key of ["Shift+ArrowDown", "Shift+ArrowDown", "Shift+ArrowDown", "Shift+ArrowUp", "Shift+ArrowUp"]) {
      await page.evaluate(() => document.activeElement?.blur());
      await page.keyboard.press(key);
      await watch.step("press", s);
      await settle(page);
      went.push(await followed());
    }
    const want = [pair[0], pair[1], pair[2], pair[3], pair[2], pair[1]];
    console.log(JSON.stringify({ pair, went }));
    if (went.join(" ") !== want.join(" ")) walk.wrong = `Shift and the arrows over two cards that begin together went ${went.join(", ")}, not ${want.join(", ")}`;
  },
  async "one light"(walk, arg, s) {
    const { page, watch } = walk;
    // What is lit is lit one way, whether the hand is on it or the
    // keys brought it there, found by Tim: a card under the hand was a
    // step darker than the card the keys were on, the clip chosen was
    // a step darker on the range picker and the clip timeline than the
    // hand on its card made it, and a part not transcribed yet walked
    // to lit nothing on the range picker.
    const look = (selector, prop) =>
      page.evaluate(
        ({ selector, prop }) =>
          new Promise((done) =>
            requestAnimationFrame(() =>
              requestAnimationFrame(() => {
                const el = document.querySelector(selector);
                done(el ? getComputedStyle(el)[prop] : null);
              }),
            ),
          ),
        { selector, prop },
      );
    if (arg === "said") {
      await page.mouse.move(1, 1);
      await page.evaluate(() => document.querySelector('.listhead button[aria-label="Search the transcript"]').click());
      await watch.step("click", s);
      await page.waitForSelector("ol.said li.hole");
      const around = await page.evaluate(() => {
        const cards = [...document.querySelectorAll("ol.said li[data-row]")];
        const i = cards.findIndex((li, i) => i > 1 && li.classList.contains("hole") && !cards[i - 1].classList.contains("hole") && !cards[i - 2].classList.contains("hole"));
        return i < 0 ? null : [cards[i - 2], cards[i - 1], cards[i]].map((li) => li.dataset.row);
      });
      if (!around) {
        walk.wrong = "no part not transcribed yet with two cards before it";
        return;
      }
      const [other, before, hole] = around.map((key) => `ol.said li[data-row="${key}"]`);
      await page.locator(`${before} button.pick`).click();
      await watch.step("click", s);
      await page.mouse.move(1, 1);
      const keyed = await look(`${before} button.pick`, "backgroundColor");
      await page.locator(`${other} button.pick`).hover();
      await watch.step("hover", s);
      const handed = await look(`${other} button.pick`, "backgroundColor");
      // The keys onto the part, with the hand away.
      await page.mouse.move(1, 1);
      await page.evaluate(() => document.activeElement?.blur());
      await page.keyboard.press("Shift+ArrowDown");
      await watch.step("press", s);
      const part = await page.evaluate(
        (hole) =>
          new Promise((done) =>
            requestAnimationFrame(() =>
              requestAnimationFrame(() => {
                const li = document.querySelector(hole);
                const parts = [...document.querySelectorAll(".track .unheard")];
                const lit = parts.filter((u) => u.classList.contains("lit"));
                done({
                  current: li.classList.contains("current"),
                  card: getComputedStyle(li).backgroundColor,
                  // The part lit is the one the card stands for, the
                  // parts and their cards in the same order.
                  lit: lit.map((u) => parts.indexOf(u)),
                  mine: [...document.querySelectorAll("ol.said li.hole")].indexOf(li),
                  fill: lit.length ? getComputedStyle(lit[0]).backgroundColor : null,
                });
              }),
            ),
          ),
        hole,
      );
      console.log(JSON.stringify({ keyed, handed, part }));
      if (keyed !== handed) walk.wrong = `the card under the hand is ${handed} and the card the keys are on is ${keyed}`;
      else if (!part.current) walk.wrong = "Shift and the down arrow did not go onto the part not transcribed yet";
      else if (part.card !== keyed) walk.wrong = `the part not transcribed yet the keys are on is ${part.card} in the list, not the grey of every lit card, ${keyed}`;
      else if (part.lit.join() !== String(part.mine))
        walk.wrong = `the part not transcribed yet the keys are on lit parts ${JSON.stringify(part.lit)} on the range picker, not its own, ${part.mine}`;
      // On the range picker in half that grey: the whole of it was far
      // too strong there, Tim's report of 24 October.
      else if (!halfOf(part.fill, keyed)) walk.wrong = `the part not transcribed yet lit on the range picker is ${part.fill}, not half the grey of its card, ${keyed}`;
      return;
    }
    // A clip's card under the hand and the chosen clip's card, and the
    // chosen clip on the range picker and the clip timeline, with the
    // hand away and with the hand on its card.
    const path = await episodeOn(page);
    let have = (await ask(page, "Clips", path)) ?? [];
    if (have.length < 2) {
      await ask(page, "MakeClip", path, Math.max(0, ...have.map((c) => c.end)) + 30, false);
      await pageUntil(page, () => document.querySelectorAll("aside ol li[data-key]:not(.next)").length >= 2);
    }
    await settle(page);
    const keys = await page.evaluate(() => [...document.querySelectorAll("aside ol li[data-key]:not(.next)")].map((li) => li.dataset.key));
    const card = (key) => `aside ol li[data-key="${key}"] button.pick`;
    await page.locator(card(keys[0])).click();
    await watch.step("click", s);
    await settle(page);
    await page.mouse.move(1, 1);
    const away = {
      card: await look(card(keys[0]), "backgroundColor"),
      mark: await look(".track .clipmark.selected", "borderTopColor"),
      frame: await look(".clip-timeline .span.frame", "borderTopColor"),
    };
    await page.locator(card(keys[0])).hover();
    await watch.step("hover", s);
    const on = {
      mark: await look(".track .clipmark.selected", "borderTopColor"),
      frame: await look(".clip-timeline .span.frame", "borderTopColor"),
    };
    await page.locator(card(keys[1])).hover();
    await watch.step("hover", s);
    const other = await look(card(keys[1]), "backgroundColor");
    await page.mouse.move(1, 1);
    console.log(JSON.stringify({ away, on, other }));
    if (other !== away.card) walk.wrong = `a clip's card under the hand is ${other} and the chosen clip's card is ${away.card}`;
    else if (away.mark !== on.mark) walk.wrong = `the chosen clip's mark on the range picker is ${away.mark}, and ${on.mark} with the hand on its card`;
    else if (away.frame !== on.frame) walk.wrong = `the chosen clip's frame on the clip timeline is ${away.frame}, and ${on.frame} with the hand on its card`;
  },
  async "walk over a part"(walk, arg, s) {
    const { page, watch } = walk;
    // Shift and the arrows walk onto a part not transcribed yet and off
    // it again, one card at a step both ways. Down stopped at the card
    // before it, and up went past it, found by Tim: that card ran on a
    // little past where the hearing reached, as a word heard at the
    // edge can, since the engine keeps a word by where it starts. So
    // here the card before the part runs half a second on into it, and
    // begins just before it. The speech model is held, so the edge
    // stays where it is, and the app is opened again so it reads the
    // cards that way from the start.
    const path = await episodeOn(page);
    const call = async (route, name, args) => (await route.fetch({ postData: JSON.stringify({ name, args }) })).json();
    const later = async (route) => {
      const body = route.request().postDataJSON?.() ?? {};
      if (!String(body.name ?? "").endsWith("Sentences")) return route.fallback();
      const answer = await route.fetch();
      const cards = await answer.json();
      // Where the hearing has got to, saved or not, which is where the
      // part begins.
      const jobs = (await call(route, "Jobs", [])) ?? [];
      const saved = (await call(route, "Episode", [path]))?.heard ?? [];
      const edge = Math.max(
        ...jobs.filter((j) => j.episode === path && j.progress?.covered).map((j) => j.progress.covered),
        ...saved.filter(([a]) => a < 1).map(([, b]) => b),
      );
      const last = cards.filter((c) => c.start < edge).at(-1);
      if (last && Number.isFinite(edge)) {
        last.end = edge + 0.5;
        // And it starts just before the edge, the way a card of one
        // word heard at the edge does, so the playhead put on the card
        // by a click lands in the part after it as well. The part lit
        // then, not the card clicked, found by Tim.
        last.start = edge - 0.02;
      }
      return route.fulfill({ response: answer, json: cards });
    };
    await page.route("**/call", later);
    await page.reload();
    await page.locator("aside li", { hasText: path.split("/").pop() }).first().waitFor();
    await fromSidebar(page, () => page.locator("aside li", { hasText: path.split("/").pop() }).first().click());
    await settle(page);
    await page.evaluate(() => document.querySelector('.listhead button[aria-label="Search the transcript"]').click());
    await watch.step("click", s);
    await page.waitForSelector("ol.said li.hole");
    const around = await page.evaluate(() => {
      const cards = [...document.querySelectorAll("ol.said li[data-row]")];
      const i = cards.findIndex((li, i) => i && li.classList.contains("hole") && !cards[i - 1].classList.contains("hole"));
      if (i < 0) return null;
      return [cards[i - 1], cards[i], cards[(i + 1) % cards.length]].map((li) => li.dataset.row);
    });
    if (!around) {
      walk.wrong = "no part not transcribed yet with a card before it";
      return;
    }
    // A step is done once the list has had a frame to follow the
    // playhead: the key moves it at once, and the list follows in the
    // same frame.
    const stepped = async (press) => {
      await press();
      return page.evaluate(
        () => new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(() => done(document.querySelector("ol.said").dataset.followed)))),
      );
    };
    const keys = async (key) => {
      await page.evaluate(() => document.activeElement?.blur());
      await page.keyboard.press(key);
      await watch.step("press", s);
    };
    const went = [
      await stepped(async () => {
        await page.locator(`ol.said li[data-row="${around[0]}"] button.pick`).click();
        await watch.step("click", s);
      }),
    ];
    for (const key of ["Shift+ArrowDown", "Shift+ArrowDown", "Shift+ArrowUp", "Shift+ArrowUp"]) went.push(await stepped(() => keys(key)));
    await page.unroute("**/call", later);
    const want = [around[0], around[1], around[2], around[1], around[0]];
    console.log(JSON.stringify({ around, went }));
    if (went.join(" ") !== want.join(" "))
      walk.wrong = `Shift and the arrows from the card before a part not transcribed yet went ${went.join(", ")}, not ${want.join(", ")}`;
  },
  // ["rendered"]       the clip says its short is in its episode's folder
  //                    inside the folder the settings name for shorts, the
  //                    file is there and the app serves it, Render says
  //                    Render again and Show in folder is there
  async rendered(walk) {
    const { page } = walk;
    const on = await chosen(page);
    const short = await shortOf(page, on);
    // The episode's own folder in the folder for shorts, named after
    // its file, see EpisodeName in engine/run.go.
    const shorts = realpathSync((await ask(page, "GetSettings")).outputDir);
    const folder = join(shorts, basename(on.path, extname(on.path)));
    const render = page.locator("button.primary.render");
    await settle(page);
    const again = (await render.textContent())?.trim() === "Render again";
    if (!short) walk.wrong = "the clip says it has no short";
    else if (dirname(short) !== folder) walk.wrong = `the short is at ${short}, not in the folder for shorts, ${folder}`;
    else if (!existsSync(short)) walk.wrong = `the clip says its short is at ${short}, and nothing is there`;
    else if (!again) walk.wrong = `the Render button says ${(await render.textContent())?.trim()}, not Render again`;
    else if (!(await page.getByRole("button", { name: "Show in folder" }).count())) walk.wrong = "Show in folder is not there";
  },
  // ["remove clip"]    presses the bin of the clip on screen in the clip
  //                    list, and waits until the Go side has it removed
  async "remove clip"(walk, arg, s) {
    const { page, watch } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    // The bin shows while the pointer is on the card, as it does for
    // a hand.
    const bin = page.locator(`button[aria-label="Remove ${entry.title || entry.slug}"]`);
    await page.hover(`li:has(> button[aria-label="Remove ${entry.title || entry.slug}"])`);
    await bin.click();
    await settle(page);
    const gone = (await ask(page, "Clips", on.path)).some((c) => c.plan === on.plan && c.id === on.clip && c.rejected);
    if (!gone) walk.wrong = "the bin left the clip in the list";
    walk.removedLast = on;
    await watch.step("remove clip", s);
  },
  // ["put back"]       presses Put it back on the row of the clip removed
  //                    last, and waits until the Go side has it back
  async "put back"(walk, arg, s) {
    const { page, watch } = walk;
    const on = walk.removedLast;
    const back = page.locator("aside ol button.back", { hasText: "Put it back" }).first();
    await back.click();
    await settle(page);
    const kept = (await ask(page, "Clips", on.path)).some((c) => c.plan === on.plan && c.id === on.clip && !c.rejected);
    if (!kept) walk.wrong = "Put it back left the clip removed";
    await watch.step("put back", s);
  },
  // ["put back another while playing"]
  //                    plays the clip on screen with the space bar,
  //                    removes another clip with its bin and puts it back,
  //                    and sees the clip chosen stay chosen, the play go
  //                    on, and the playhead never go back while it plays
  async "put back another while playing"(walk, arg, s) {
    const { page, watch } = walk;
    const path = await episodeOn(page);
    const was = await page.evaluate(() => document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "");
    const other = await page.evaluate(
      (was) =>
        [...document.querySelectorAll("aside ol li[data-key]")].find((li) => li.dataset.key !== was && li.querySelector("button.drop"))?.dataset.key ?? "",
      was,
    );
    if (!was || !other) {
      walk.wrong = `there is no other clip to remove beside the one chosen, ${was || "none"}`;
      return;
    }
    await page.evaluate(() => document.body.focus());
    await page.keyboard.press("Space");
    await settle(page);
    if (!(await page.locator('button[aria-label="Pause"]').count())) {
      walk.wrong = "the space bar did not start the play";
      return;
    }
    // Every playhead and every clip chosen from here on, on every
    // animation frame, so a jump back is seen however short it was.
    await page.evaluate(() => {
      window.__heads = [];
      const read = () => {
        if (!window.__heads) return;
        const at = Number(document.querySelector(".screen")?.dataset.playhead);
        const on = document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "";
        const playing = !!document.querySelector('button[aria-label="Pause"]');
        window.__heads.push([at, on, playing]);
        requestAnimationFrame(read);
      };
      requestAnimationFrame(read);
    });
    // Half a second of the play before the hand comes, and a second
    // after the clip is put back, in the episode's time: a jump back
    // or a stop comes as the app takes the answers in, before the app
    // settles, and the play going on after shows it was not stopped.
    await playUntil(page, { most: 0.5 });
    await page.hover(`aside ol li[data-key="${other}"]`);
    await page.locator(`aside ol li[data-key="${other}"] button.drop`).click();
    await settle(page);
    const gone = (await ask(page, "Clips", path)).some((c) => c.key === other && c.rejected);
    if (!gone) {
      walk.wrong = "the bin left the other clip in the list";
      return;
    }
    await page.locator(`aside ol li[data-key="${other}"] button.back`, { hasText: "Put it back" }).click();
    await settle(page);
    const kept = (await ask(page, "Clips", path)).some((c) => c.key === other && !c.rejected);
    if (!kept) walk.wrong = "Put it back left the other clip removed";
    await playUntil(page, { most: 1 });
    const heads = await page.evaluate(() => {
      const h = window.__heads;
      delete window.__heads;
      return h;
    });
    await page.evaluate(() => {
      if (document.querySelector('button[aria-label="Play"]')) return;
      const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
      document.body.dispatchEvent(new KeyboardEvent("keydown", key));
      document.body.dispatchEvent(new KeyboardEvent("keyup", key));
    });
    for (let i = 1; i < heads.length && !walk.wrong; i++) {
      const [at, on, playing] = heads[i];
      const [at0] = heads[i - 1];
      if (on !== was) walk.wrong = `the clip chosen went from ${was} to ${on || "none"} when the other clip, ${other}, was put back`;
      else if (!playing) walk.wrong = `the play stopped at ${at.toFixed(3)} when the other clip was removed or put back`;
      else if (at < at0 - 1e-6) walk.wrong = `the playhead went back from ${at0.toFixed(3)} to ${at.toFixed(3)} when the other clip was removed or put back`;
    }
    if (!walk.wrong && heads.length < 30) walk.wrong = `only ${heads.length} animation frames were read`;
    await watch.step("put back", s);
  },
  // ["remove the chosen", end]
  //                    clicks the "first" or "last" clip in the list, which
  //                    chooses it, removes it with its bin, and sees the
  //                    clip beside it chosen: the next, or for the last
  //                    the one before it
  async "remove the chosen"(walk, arg, s) {
    const { page, watch } = walk;
    const path = await episodeOn(page);
    const order = () =>
      page.evaluate(() =>
        [...document.querySelectorAll("aside ol li[data-key]")].filter((li) => li.querySelector("button.pick")).map((li) => li.dataset.key),
      );
    const chosenKey = () =>
      page.evaluate(() => document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "");
    const list = await order();
    if (list.length < 2) {
      walk.wrong = `the list has ${list.length} clips, the step needs two`;
      return;
    }
    const key = arg === "last" ? list[list.length - 1] : list[0];
    const beside = arg === "last" ? list[list.length - 2] : list[1];
    await page.locator(`aside ol li[data-key="${key}"] button.pick`).click();
    await settle(page);
    if ((await chosenKey()) !== key) {
      walk.wrong = `clicking ${key} did not choose it`;
      return;
    }
    await page.hover(`aside ol li[data-key="${key}"]`);
    await page.locator(`aside ol li[data-key="${key}"] button.drop`).click();
    await settle(page);
    const gone = (await ask(page, "Clips", path)).some((c) => c.key === key && c.rejected);
    if (!gone) {
      walk.wrong = "the bin left the clip in the list";
      return;
    }
    const on = await chosenKey();
    if (on !== beside) walk.wrong = `removing the chosen ${arg} clip, ${key}, left ${on || "nothing"} chosen, not ${beside}`;
    walk.removedChosen = key;
    const entry = (await ask(page, "Clips", path)).find((c) => c.key === key);
    walk.removedLast = { path, plan: entry.plan, clip: entry.id };
    await watch.step("remove clip", s);
  },
  // ["chosen again"]   the clip the last "remove the chosen" removed is
  //                    chosen again
  async "chosen again"(walk) {
    const { page } = walk;
    await settle(page);
    const on = await page.evaluate(() => document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "");
    if (on !== walk.removedChosen) walk.wrong = `${on || "nothing"} is chosen after ${walk.removedChosen} was put back, not ${walk.removedChosen} again`;
  },
  // ["put back the chosen"]
  //                    removes the clip chosen with its bin, paused, puts
  //                    it back, and sees it chosen again with the
  //                    playhead where it was
  async "put back the chosen"(walk, arg, s) {
    const { page, watch } = walk;
    const path = await episodeOn(page);
    const read = () =>
      page.evaluate(() => [
        document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "",
        Number(document.querySelector(".screen")?.dataset.playhead),
      ]);
    const [was, at] = await read();
    if (!was) {
      walk.wrong = "no clip is chosen";
      return;
    }
    await page.hover(`aside ol li[data-key="${was}"]`);
    await page.locator(`aside ol li[data-key="${was}"] button.drop`).click();
    await settle(page);
    const gone = (await ask(page, "Clips", path)).some((c) => c.key === was && c.rejected);
    if (!gone) {
      walk.wrong = "the bin left the clip in the list";
      return;
    }
    await page.locator(`aside ol li[data-key="${was}"] button.back`, { hasText: "Put it back" }).click();
    await settle(page);
    const [on, now] = await read();
    const back = on === was && (await ask(page, "Clips", path)).some((c) => c.key === was && !c.rejected);
    if (!back) walk.wrong = `the clip chosen is ${on || "none"} after ${was} was put back, not ${was} again`;
    else if (Math.abs(now - at) > 1e-3) walk.wrong = `the playhead went from ${at.toFixed(3)} to ${now.toFixed(3)} when the clip was put back`;
    await watch.step("put back", s);
  },
  // ["pick rested cards", n]
  //                    the same clips, then clicks the cards in turn n
  //                    times, each after the hand rested, and checks each
  //                    clip is put up from the frame and the captions read
  //                    ahead, in one change, see pickRested
  async "pick rested cards"(walk, arg, s) {
    const { page, watch } = walk;
    walk.wrong = await pickRested(page, arg);
    await watch.step("press", s);
  },
  // ["pick cards", n]  makes a clip in each shot with Mark In, then clicks
  //                    the clip cards in turn n times, every other time on a
  //                    clip made just then, with the Go side's answers about
  //                    captions held back, and on
  //                    every animation frame from the click reads the crop
  //                    frame and the canvas: the picture, the crop frame and
  //                    the captions over it change once, together, issue
  //                    #383. The frames wait for their captions as long as
  //                    they take, so a busy machine cannot make them late
  async "pick cards"(walk, arg, s) {
    const { page, watch } = walk;
    walk.wrong = await pickCards(page, arg);
    await watch.step("press", s);
  },
  // ["make here", key] presses "i" or "o", Mark In or Mark Out, with the
  //                    playhead where it is, and waits until the clip made
  //                    is written and chosen
  async "make here"(walk, arg, s) {
    const { page, watch } = walk;
    const path = await episodeOn(page);
    const was = (await ask(page, "Clips", path)).map((c) => c.key);
    await page.evaluate(() => document.body.focus());
    await page.keyboard.press(arg);
    // Until a clip the engine did not have before is chosen.
    const made = async () => {
      const on = await chosen(page);
      const key = on && (await ask(page, "Clips", path)).find((c) => c.plan === on.plan && c.id === on.clip)?.key;
      return key && !was.includes(key) ? on : null;
    };
    await seenUntil(page, url, made, (on) => !!on);
    await watch.step("wait", s);
  },
  // ["remove two, hand still"]
  //                    presses the bins of the first two clips in the clip
  //                    list, one after the other, and leaves the hand where
  //                    it pressed the second while the first one's row runs
  //                    out and closes, which moves the second row out from
  //                    under the hand. The page hears nothing of the
  //                    pointer meanwhile, the way WebKit tells it nothing
  //                    of a row moving from under a hand that stands
  //                    still, where Chromium makes up a move for it. Then
  //                    the hand goes to the clip timeline, and the second
  //                    row's time has to run down and close it
  async "remove two, hand still"(walk, arg, s) {
    const { page, removeTime, watch } = walk;
    const bins = page.locator("aside ol li button.drop");
    if ((await bins.count()) < 2) {
      walk.wrong = `the clip list has ${await bins.count()} clips to remove, not two`;
      return;
    }
    const press = async (bin) => {
      const box = await bin.boundingBox();
      await page.mouse.move(box.x - 40, box.y + box.height / 2, { steps: 4 });
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 4 });
      await page.mouse.down();
      await page.mouse.up();
    };
    // The time a removed row has is run on by hand, so nothing waits
    // ten seconds on the clock, see holdClock. Its time only starts
    // once the hand leaves the row, so the clock is held before.
    await holdClock(page);
    try {
      await press(bins.nth(0));
      await page.waitForSelector("aside ol .gone");
      // The second bin is where it was: the first clip's row stays open
      // while it can be put back.
      await press(bins.nth(0));
      const rows = () =>
        page.evaluate(() =>
          [...document.querySelectorAll("aside ol .gone")].map((g) => {
            const run = g.querySelector(".fill .run");
            return { top: Math.round(g.getBoundingClientRect().top), running: run.getAnimations().length > 0, at: getComputedStyle(run).transform };
          }),
        );
      await page.locator("aside ol .gone").nth(1).waitFor();
      let now = await rows();
      if (now.length < 2) {
        walk.wrong = `${now.length} rows can be put back after two clips were removed`;
        return;
      }
      if (now[1].running) {
        walk.wrong = "the second row's time runs down under the hand that removed it";
        return;
      }
      const second = now[1].top;
      // What WebKit says while the hand stands still: nothing.
      await page.evaluate(() => {
        const names = ["pointerover", "pointerout", "pointerenter", "pointerleave", "pointermove"];
        const still = (e) => e.stopImmediatePropagation();
        window.__stillHand = () => names.forEach((n) => window.removeEventListener(n, still, true));
        names.forEach((n) => window.addEventListener(n, still, true));
      });
      // The first row's time runs out with the hand still where it was,
      // and it closes on the browser's own frames.
      await runClock(page, (removeTime + 0.5) * 1000);
      if ((await page.locator("aside ol li:not([inert]) .gone").count()) !== 1) {
        walk.wrong = `the first row did not close after its time: ${JSON.stringify(await rows())}`;
        return;
      }
      await foldsDone(page);
      now = await rows();
      if (now.length !== 1 || now[0].top >= second) {
        walk.wrong = `the first row did not close and move the second up: ${JSON.stringify(now)}`;
        return;
      }
      await runClock(page, 400);
      await page.evaluate(() => window.__stillHand());
      const line = await page.locator(".clip-timeline").boundingBox();
      await page.mouse.move(line.x + line.width / 2, line.y + line.height / 2, { steps: 8 });
      // Its fill moves on the browser's own frames, held clock or not,
      // so two of them show it moving. It runs the whole ten seconds the
      // row has, since the hand held it full until now.
      const a = await rows();
      await realFrames(page);
      const b = await rows();
      if (!a[0]?.running || a[0].at === b[0]?.at) {
        walk.wrong = `the row left to put back stands still with the hand on the clip timeline: ${JSON.stringify(b)}`;
        return;
      }
      const whole = await page.evaluate(() => document.querySelector("aside ol .gone .fill .run").getAnimations()[0]?.effect.getComputedTiming().duration);
      if (Math.abs(whole - removeTime * 1000) > 50) {
        walk.wrong = `the row left to put back runs down in ${(whole / 1000).toFixed(1)} s, not ${removeTime}`;
        return;
      }
      await runClock(page, (removeTime - 0.5) * 1000);
      // A row that is closing has gone, though it is still in the list
      // for its fold: Svelte marks it inert the moment it starts to leave.
      if (!(await page.locator("aside ol li:not([inert]) .gone").count())) {
        walk.wrong = "the row left to put back went before its time ran out";
        return;
      }
      await runClock(page, 1000);
      if (await page.locator("aside ol li:not([inert]) .gone").count()) {
        walk.wrong = "the row left to put back is still there after its time";
        return;
      }
      await foldsDone(page);
    } finally {
      await letClockGo(page);
    }
    await watch.step("remove two, hand still", s);
  },
  // ["row folds"]      presses the bin of a clip with another clip's card
  //                    under it, takes the hand to the clip timeline, and
  //                    runs the removed row's time down. The row's fold is
  //                    held as it starts and stepped through by hand in
  //                    twenty steps, and where the card under it stands is
  //                    read at each, in the moment the row leaves and on
  //                    the ten frames after. The card has to rise by the
  //                    row and the space after it, never go back down, not
  //                    move as the row leaves, and at every step have
  //                    risen by as much of both as the row has folded. The
  //                    row has to leave the list only once its fold has
  //                    finished
  async "row folds"(walk, arg, s) {
    const { page, removeTime, watch } = walk;
    const bins = page.locator("aside ol li:has(+ li) button.drop");
    if (!(await bins.count())) {
      walk.wrong = "no clip in the clip list has a card under it";
      return;
    }
    const box = await bins.first().boundingBox();
    // The row's time is run on by hand, see holdClock, and it starts
    // once the hand leaves the row, so the clock is held before.
    await holdClock(page);
    await page.mouse.move(box.x - 40, box.y + box.height / 2, { steps: 4 });
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 4 });
    await page.mouse.down();
    await page.mouse.up();
    await page.waitForSelector("aside ol li:not([inert]) .gone");
    // The fold runs on the browser's own time, which the held clock
    // does not hold, and a busy machine may draw no frame of it at
    // all, #469. So the fold is held where it starts the moment it is
    // made and stepped through by hand below, each step read off the
    // page, and the row and the card under it are read again in the
    // moment the row leaves the list and on the frames after.
    await page.evaluate(() => {
      const frame = window.__real?.frame ?? ((f) => requestAnimationFrame(f));
      const row = document.querySelector("aside ol li:not([inert]) .gone").closest("li");
      const under = row.nextElementSibling;
      const gap = parseFloat(getComputedStyle(row.parentElement).rowGap);
      const seen = [];
      const folds = [];
      let found;
      let ended;
      const look = (step) => seen.push({ step, there: row.isConnected, top: under.getBoundingClientRect().top, height: row.isConnected ? row.getBoundingClientRect().height : 0 });
      window.__fold = { seen, look, height: row.getBoundingClientRect().height, gap, row, left: null, held: new Promise((r) => (found = r)), ended: new Promise((r) => (ended = r)) };
      look();
      // Svelte takes the row out when its animation says it has
      // finished, and cancels it as it does, so whether it had is
      // heard before Svelte hears it: a listener added here, as the
      // animation is made, comes before the one Svelte sets after.
      // Svelte first makes an animation of no length and makes the
      // fold when that ends, so the fold is the one with a length.
      row.animate = function (...args) {
        const fold = Element.prototype.animate.apply(this, args);
        const last = { finished: false };
        fold.addEventListener("finish", () => (last.finished = true));
        folds.push(last);
        if (fold.effect.getComputedTiming().duration > 0) {
          fold.pause();
          found(fold);
        }
        return fold;
      };
      // The moment the row leaves, read before the browser draws it,
      // then ten frames more, to see that nothing moves after.
      new MutationObserver((_, watching) => {
        if (row.isConnected) return;
        watching.disconnect();
        window.__fold.left = { folded: folds.length > 0, finished: !!folds.at(-1)?.finished };
        found(null);
        let frames = 10;
        const after = () => {
          look();
          if (--frames) frame(after);
          else ended();
        };
        after();
      }).observe(row.parentElement, { childList: true });
    });
    const line = await page.locator(".clip-timeline").boundingBox();
    await page.mouse.move(line.x + line.width / 2, line.y + line.height / 2, { steps: 8 });
    await runClock(page, (removeTime + 0.5) * 1000);
    // Its time has run out, so it is on its way out, or out already.
    const going = await page.evaluate(() => !window.__fold.row.isConnected || window.__fold.row.inert);
    if (going) {
      // Twenty steps from where the fold starts to where it ends, each
      // read as the page stands then, and then the fold is let finish.
      await page.evaluate(async () => {
        const fold = await window.__fold.held;
        if (fold) {
          const end = fold.effect.getComputedTiming().endTime;
          for (let i = 1; i <= 20; i++) {
            fold.currentTime = (end * i) / 20;
            window.__fold.look(i);
          }
          fold.finish();
        }
        await window.__fold.ended;
      });
    }
    await letClockGo(page);
    if (!going) {
      walk.wrong = "the removed row was still in the list after its time";
      return;
    }
    const { seen, height, gap, left } = await page.evaluate(() => ({ seen: window.__fold.seen, height: window.__fold.height, gap: window.__fold.gap, left: window.__fold.left }));
    const tops = seen.map((f) => f.top);
    const rose = tops[0] - tops[tops.length - 1];
    const back = tops.findIndex((t, i) => i && t > tops[i - 1] + 0.5);
    const leaving = seen.findIndex((f) => !f.there);
    const jump = leaving > 0 ? tops[leaving - 1] - tops[leaving] : 0;
    // The space goes with the row: at every step of the fold, the card
    // under it has risen by as much of the row and the space after it
    // as the row has folded.
    const steps = seen.filter((f) => f.step);
    const off = steps.map((f) => ({ i: f.step, by: tops[0] - f.top - (height + gap) * (1 - f.height / height) })).find((f) => Math.abs(f.by) > 1);
    if (!left) walk.wrong = "the removed row was never seen leaving the list";
    else if (!left.folded) walk.wrong = "the removed row left the list with no fold";
    else if (!left.finished) walk.wrong = "the removed row left the list before its fold had finished";
    else if (steps.length !== 20) walk.wrong = `the removed row's fold was stepped through ${steps.length} times, not 20`;
    else if (!steps.every((f) => f.there)) walk.wrong = `the removed row left the list at step ${steps.find((f) => !f.there).step} of its fold, not after it`;
    else if (off) walk.wrong = `the space under the removed row did not fold with it: ${Math.abs(off.by).toFixed(1)} px ${off.by < 0 ? "short of" : "past"} where the card under it stands at step ${off.i} of the fold`;
    else if (Math.abs(jump) > 0.5) walk.wrong = `the card under the removed row jumped ${jump.toFixed(1)} px in the moment the row left the list`;
    else if (Math.abs(rose - (height + gap)) > 1) walk.wrong = `the card under the removed row rose ${rose.toFixed(1)} px, not the row's ${height} and the ${gap} after it`;
    else if (back >= 0) walk.wrong = `the card under the removed row went back down ${(tops[back] - tops[back - 1]).toFixed(1)} px at step ${back}`;
    await watch.step("row folds", s);
  },
  // ["thumbnail"]      presses T, which makes the frame under the playhead
  //                    a thumbnail of the clip, and waits for the clip to
  //                    have one more
  async thumbnail(walk, arg, s) {
    const { page, watch } = walk;
    const on = await chosen(page);
    const count = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.thumbnails.length ?? 0;
    const before = await count();
    await page.keyboard.press("t");
    await settle(page);
    const now = await count();
    if (now !== before + 1) walk.wrong = `T left the clip with ${now} thumbnails, not ${before + 1}`;
    await watch.step("press", s);
  },
  // ["keep", label]    remembers the clip on screen, its short and its
  //                    pictures on disk, and the shorts in its folder
  async keep(walk, arg) {
    const { kept, page } = walk;
    const on = await chosen(page);
    const short = await shortOf(page, on);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const file = short && existsSync(short) ? statSync(short) : null;
    kept[arg] = { ...on, name: entry?.slug, short, file };
    if (!file) {
      walk.wrong = `the clip has no short to keep${short ? `, nothing is at ${short}` : ""}`;
      return;
    }
    kept[arg].shorts = shortsIn(dirname(short));
    kept[arg].pictures = picturesOf(short).map((p) => ({ path: p, file: statSync(p) }));
  },
  // ["by its words"]   the clip's short is named after the clip's words
  //                    alone, <slug>.mp4, with nothing of its id
  async "by its words"(walk) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const short = await shortOf(page, on);
    if (!short || !existsSync(short)) walk.wrong = "the clip has no short in its folder";
    else if (basename(short) !== `${entry?.slug}.mp4`) walk.wrong = `the short is called ${basename(short)}, not ${entry?.slug}.mp4`;
  },
  // ["short removed"]  the chosen clip's short and its pictures are
  //                    deleted from the disk, the way a person cleans up
  //                    after uploading them
  async "short removed"(walk) {
    const { page } = walk;
    const short = await shortOf(page, await chosen(page));
    if (!short || !existsSync(short)) {
      walk.wrong = "the clip has no short to remove";
      return;
    }
    for (const file of [short, ...picturesOf(short)]) rmSync(file);
  },
  // ["rendered, short gone"]
  //                    the clip still says it was rendered, its card's
  //                    green dot and its green mark on the range picker
  //                    among it, Render says Render again and Show in
  //                    folder is not there
  async "rendered, short gone"(walk) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const seen = await page.evaluate(() => ({
      dot: !!document.querySelector("button.pick.current .dot.ok"),
      mark: !!document.querySelector(".clipmark.selected.rendered"),
      render: document.querySelector("button.primary.render")?.textContent.trim(),
      folder: [...document.querySelectorAll("button")].some((b) => b.textContent.trim() === "Show in folder"),
    }));
    if (!entry?.rendered) walk.wrong = "the clip says it was never rendered";
    else if (entry.short) walk.wrong = `the clip says its short is at ${entry.short}, which was removed`;
    else if (!seen.dot) walk.wrong = "the clip's card has no green dot";
    else if (!seen.mark) walk.wrong = "the clip's mark on the range picker is not green";
    else if (seen.render !== "Render again") walk.wrong = `the Render button says ${seen.render}, not Render again`;
    else if (seen.folder) walk.wrong = "Show in folder is there, with no short to show";
  },
  // ["pictured"]       the short's pictures, one for each thumbnail of the
  //                    clip, lie beside it in its episode's folder, and no
  //                    picture more
  async pictured(walk) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const short = await shortOf(page, on);
    const want = entry?.thumbnails.length ?? 0;
    const got = short ? picturesOf(short) : [];
    const named = got.map((p) => basename(p)).join(", ");
    if (!short) walk.wrong = "the clip says it has no short";
    else if (want === 0) walk.wrong = "the clip has no thumbnails, so there is nothing to look for";
    else if (got.length !== want || got.some((p, i) => basename(p) !== `${basename(short, ".mp4")}-${i + 1}.jpg`)) {
      walk.wrong = `beside ${short} lie ${named || "no pictures"}, not the clip's ${want}`;
    }
  },
  // ["in place", label]
  //                    the clip's short is the one kept, under the same
  //                    name with no number, written again since, and no
  //                    other short appeared in its folder
  async "in place"(walk, arg) {
    const { kept, page } = walk;
    const was = kept[arg];
    const on = await chosen(page);
    const short = await shortOf(page, on);
    const now = short && existsSync(short) ? statSync(short) : null;
    const added = short ? shortsIn(dirname(short)).filter((n) => !was.shorts.includes(n)) : [];
    if (short !== was.short) walk.wrong = `rendered again, the short is at ${short || "nowhere"}, not ${was.short}`;
    else if (!now || now.mtimeMs === was.file.mtimeMs) walk.wrong = `rendered again, ${short} was not written again`;
    else if (added.length) walk.wrong = `rendered again, ${added.join(", ")} appeared beside ${basename(short)}`;
  },
  // ["apart", label]   the clip on screen, of another episode with a clip
  //                    of the same name as the one kept, has a short of its
  //                    own, the short kept is still the same file, and the
  //                    clip kept still says it is its short
  // ["apart", label, number]
  //                    the same, with the short on screen called as the
  //                    one kept with this after it, " 2". With both, the
  //                    pictures kept are all still there, untouched
  async apart(walk, arg) {
    const { kept, page } = walk;
    const [label, number] = [arg].flat();
    const was = kept[label];
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const short = await shortOf(page, on);
    const now = existsSync(was.short) ? statSync(was.short) : null;
    const theirs = await shortOf(page, was);
    if (on.path === was.path) walk.wrong = `the clip on screen is of the episode kept at "${label}"`;
    else if (entry?.slug !== was.name) walk.wrong = `the clips are named ${was.name} and ${entry?.slug}, which never meet in one folder`;
    else if (!short) walk.wrong = "the clip on screen says it has no short";
    else if (short === was.short) walk.wrong = `both episodes' shorts are ${short}`;
    else if (!now || now.ino !== was.file.ino || now.mtimeMs !== was.file.mtimeMs) {
      walk.wrong = `the second render wrote over the first episode's short at ${was.short}`;
    } else if (theirs !== was.short) {
      walk.wrong = `the first episode's clip says its short is ${theirs || "nowhere"}, not ${was.short}`;
    } else if (number && basename(short) !== `${was.name}${number}.mp4`) {
      walk.wrong = `the second short is called ${basename(short)}, not ${was.name}${number}.mp4`;
    } else {
      const gone = was.pictures.filter((p) => {
        const now = existsSync(p.path) ? statSync(p.path) : null;
        return !now || now.ino !== p.file.ino || now.mtimeMs !== p.file.mtimeMs;
      });
      if (gone.length) walk.wrong = `the second render took or wrote over the first episode's pictures ${gone.map((p) => basename(p.path)).join(", ")}`;
    }
  },
  // ["short", cuts]    the clip has at least so many cuts, none of them on
  //                    a frame that starts on a whole millisecond, and the
  //                    short Render wrote, read back from disk, holds
  //                    exactly the frames of the clip's pieces, each the
  //                    right frame of the episode, with its sound as long
  //                    as its picture and quiet at a cut for no longer
  //                    than the render's fade
  async short(walk, arg) {
    const { page } = walk;
    const on = await chosen(page);
    const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
    const { fps } = await ask(page, "Source", on.path);
    const short = await shortOf(page, on);
    walk.wrong = shortProblem(entry, short, fps, arg);
  },
};

// Steps that take the same body as the step they are walked by.
steps["make while paused"] = steps["make while playing"];

// The shorts in a folder, by name, finished ones only.
const shortsIn = (dir) => readdirSync(dir).filter((n) => n.endsWith(".mp4") && !n.endsWith(".part.mp4")).sort();

// The pictures beside a short, <name>-1.jpg and on, in their order.
function picturesOf(short) {
  const stem = basename(short, ".mp4");
  const mine = new RegExp(`^${stem.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}-(\\d+)\\.jpg$`);
  return readdirSync(dirname(short))
    .map((n) => [n, mine.exec(n)])
    .filter(([, m]) => m)
    .sort((a, b) => Number(a[1][1]) - Number(b[1][1]))
    .map(([n]) => join(dirname(short), n));
}

// What is wrong with the short Render wrote for a clip, read back from
// disk, or null. Its pieces are cut the way the render cuts them, see
// cutOf in engine/render.go: the frames from the one a piece's start is on
// up to the one before its end, at least one. Which frame of the episode a
// frame of the short is comes from its bands, see shortFrames, and is
// compared with the frame it should be, so a frame too many, one lost or
// one from the wrong place shows. The episode's sound is a steady tone, so
// padded silence at a cut, or sound that runs on after its picture, shows.
// The engine's own test of this is TestARenderCutsOnWholeFrames.
function shortProblem(entry, short, fps, cutsWanted) {
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const pieces = entry.segments;
  const problems = [];
  // The case is the one it says: a clip with cuts whose edges, on the
  // frames of the episode where the render puts them, fall between
  // milliseconds, where rounding to one moved a piece by a frame. The
  // engine keeps an edge where a word starts or ends, which here is a
  // whole millisecond, so it is the frame it lands on that counts.
  const edges = pieces.slice(1).flatMap((p, i) => [pieces[i].end, p.start]).map((t) => Math.round(t * fps) / fps);
  if (pieces.length - 1 < cutsWanted) problems.push(`the clip has ${pieces.length - 1} cuts, not ${cutsWanted}`);
  const whole = edges.filter((t) => Math.abs(t * 1000 - Math.round(t * 1000)) < 0.01);
  if (whole.length) problems.push(`cuts on frames that start on a whole millisecond, ${whole.join(" ")}, not the case this is about`);

  // Seen: the frames of each piece in order, each once, and nothing else.
  const want = [];
  const cuts = [];
  for (const p of pieces) {
    const first = Math.round(p.start * fps);
    const end = Math.max(Math.round(p.end * fps), first + 1);
    for (let k = first; k < end; k++) want.push(k);
    cuts.push(want.length / fps);
  }
  cuts.pop();
  const { frames, modulo } = shortFrames(short);
  if (frames.length !== want.length) problems.push(`the short has ${frames.length} frames, its pieces hold ${want.length}`);
  let wrongFrames = 0;
  for (let i = 0; i < Math.min(frames.length, want.length); i++) {
    if (frames[i] !== want[i] % modulo && ++wrongFrames <= 5) {
      problems.push(`frame ${i} of the short is frame ${frames[i]} of the episode, not ${want[i]}`);
    }
  }
  if (wrongFrames > 5) problems.push(`and ${wrongFrames - 5} frames more are the wrong frame`);

  // Heard: as long as the picture, the tone right up to each cut, and
  // quiet there only for the fade out and in, 15 ms, Fade in
  // engine/render.go, and that quiet where the picture cuts.
  const rate = 48000;
  const samples = shortSound(short);
  const heard = samples.length / rate;
  const pictured = want.length / fps;
  if (Math.abs(heard - pictured) > 0.025) {
    problems.push(`the sound is ${heard.toFixed(3)} s long, the picture ${pictured.toFixed(3)} s`);
  }
  // The loudest sample in the 2.5 ms around each one, more than one swing
  // of the tone, so how loud the tone is there.
  const loud = (k) => {
    let most = 0;
    for (let j = Math.max(0, k - 60); j < Math.min(samples.length, k + 60); j++) most = Math.max(most, Math.abs(samples[j]));
    return most;
  };
  const steady = loud(Math.round(0.5 * rate));
  cuts.forEach((cut, n) => {
    const from = Math.round((cut - 0.06) * rate);
    const to = Math.round((cut + 0.06) * rate);
    if (to > samples.length) {
      problems.push(`the sound ends before cut ${n + 1} at ${cut.toFixed(3)} s`);
      return;
    }
    let first = -1;
    let last = -1;
    for (let k = from; k < to; k++) {
      if (loud(k) < 0.25 * steady) {
        if (first < 0) first = k;
        last = k;
      }
    }
    if (first < 0) {
      problems.push(`cut ${n + 1} at ${cut.toFixed(3)} s has no fade in its sound`);
      return;
    }
    const quiet = (last - first) / rate;
    const mid = (first + last) / 2 / rate;
    if (quiet > 0.015) problems.push(`the sound is quiet for ${(1000 * quiet).toFixed(1)} ms at cut ${n + 1}, more than its fade`);
    if (Math.abs(mid - cut) > 0.003) {
      problems.push(`the sound cuts at ${mid.toFixed(4)} s, the picture at ${cut.toFixed(4)} s, cut ${n + 1}`);
    }
  });
  if (!problems.length) return null;
  const said = pieces.map((p) => `${p.start}-${p.end}`).join(" ");
  return `${problems.join("\n")}\npieces ${said} at ${fps} fps, the short ${short}`;
}

// The key of the card chosen, and of the cards in the clip list.
async function cardsNow(page) {
  const cards = (await clipList(page)).cards;
  const current = await page.evaluate(
    () => document.querySelector("aside ol li[data-key].current, aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "",
  );
  return { cards, current };
}

// Clicks the clip cards in turn, n times, and reads on every animation
// frame from the click what the video preview shows: the crop frame's place
// and the canvas whole, the picture with the captions libass drew over it.
// What is shown before the click and what is shown once it has settled are
// two moments, and every animation frame in between is one or the other.
// Tim saw the crop frame jump first, then the picture, then the captions,
// issue #383. Every other click is on a clip made just then, whose
// captions were not read ahead, with the Go side's answers about captions
// held back, the way a video of hours answers: the frame waits on the Go
// side for its captions, captionsDue in Episode.svelte, rather than coming
// first. The others are on the clips read ahead.
async function pickCards(page, n) {
  const clips = await shotClips(page);
  if (typeof clips === "string") return clips;
  const path = await episodeOn(page);
  await canLook(page);
  // Where the new clips are made.
  // Each gives a clip with a caption at its start, so its captions can be
  // seen to come, and one that starts where no other clip does, which
  // could put up the very picture on screen, so nothing would change
  // until its captions came. Checked against the bridge's speech stand-in
  // on 22 October 2026: 16, 28, 40, 56, 64, 76, 84, 88, 92 and 108 give a
  // clip where one of these or of shotClips does, and 44, 68, 72, 80, 96
  // and 104 one whose first word comes more than a tenth of a second
  // after its start, with no caption there yet, issue #438.
  const fresh = [20, 60, 100, 24, 32, 112, 36, 48, 52];
  const problems = [];
  // The new clip at the next place, or a problem when the place gives a
  // clip where another starts.
  const freshClip = async () => {
    const at = fresh.shift();
    const [key] = await newCards(page, 1, () => ask(page, "MakeClip", path, at, false));
    const all = await ask(page, "Clips", path);
    const clip = all.find((c) => c.key === key);
    if (all.some((c) => c.key !== key && Math.abs(c.start - clip.start) < 0.05))
      return { problem: `the new clip at ${at} s starts at ${clip.start.toFixed(2)} s, where another clip does, so pick another place for it` };
    return { key };
  };
  // What went to and from the Go side about captions, and what the video
  // preview said in the log, each by the clock of this machine: told with
  // a click that changed more than once, to say which of them came late.
  const heard = [];
  const callOf = (r) => {
    try {
      return JSON.parse(r.postData() ?? "");
    } catch {
      return null;
    }
  };
  const asked = (r) => {
    const u = r.url();
    if (u.includes("/frames/captions?")) heard.push({ at: Date.now(), what: `layer at ${new URL(u).searchParams.get("at")} asked` });
    const call = u.endsWith("/call") ? callOf(r) : null;
    if (call?.name?.endsWith("Captions")) heard.push({ at: Date.now(), what: "Captions asked" });
    if (call?.name?.endsWith("Said"))
      for (const l of call.args?.[0] ?? []) if (String(l.text).startsWith("video preview")) heard.push({ at: l.at, what: l.text });
  };
  const answered = (r) => {
    const u = r.url();
    if (u.includes("/frames/captions?"))
      heard.push({ at: Date.now(), what: `layer at ${new URL(u).searchParams.get("at")} answered ${r.response() ? "" : "nothing"}` });
    if (u.endsWith("/call") && callOf(r)?.name?.endsWith("Captions")) heard.push({ at: Date.now(), what: "Captions answered" });
  };
  const failed = (r) => {
    if (r.url().includes("/frames/captions?")) heard.push({ at: Date.now(), what: `layer failed, ${r.failure()?.errorText}` });
  };
  page.on("request", asked);
  page.on("requestfinished", answered);
  page.on("requestfailed", failed);
  // A frame put up for a click waits for its captions as long as they
  // take: the walk hands the video preview a wait that never runs out, see
  // dressWait in lib/frames/queue.ts. On the browser's clock it runs out
  // after 600 ms, and on a busy machine the walk's own steps between the
  // frame starting to wait and the gate opening used most of it, and
  // libass drawing a layer once took 590 ms, so the frame came without
  // its captions and they came after it, issue #468. The click was then
  // taken again on another new clip, up to three times. That the frame
  // does not wait for good, but 600 ms of the browser's clock, is held by
  // "a frame waiting for its captions" in queue.test.ts.
  await page.evaluate(() => {
    window.__dressWait = window.__frameQueue.dressWait;
    window.__frameQueue.dressWait = () => () => {};
  });
  const late = [];
  const clicks = [];
  for (let i = 0; i < n; i++) {
    const held = i % 2 === 1;
    let key;
    if (held) {
      // Held behind a gate until the clip's frame waits for its captions,
      // and only then let through, so the frame is seen to wait for them
      // on any machine. They were held 100 ms, and on a busy machine they
      // were read ahead before the click, and the clip came at once,
      // issue #438.
      await control(url, "/hold?call=Captions&gate=shut");
      // Both before the click, so what the card before left in them
      // cannot open the gate early, whatever order the page runs the
      // walk's evaluations in.
      await page.evaluate(() => {
        window.__captionsOpen = false;
        window.__clickChanged = false;
      });
      const made = await freshClip();
      if (made.problem) {
        problems.push(`card ${i + 1}: ${made.problem}`);
        await control(url, "/hold?call=Captions&gate=open");
        break;
      }
      key = made.key;
    } else {
      await restAhead(page);
      const { cards, current } = await cardsNow(page);
      const others = cards.filter((k) => k !== current);
      key = others[i % others.length];
    }
    const clicked = Date.now();
    clicks.push(clicked);
    const { waitedForDress } = await page.evaluate(() => window.__frameQueue.stats);
    const clicking = clickCard(page, key, null);
    if (held) {
      // The frame of the clip waits for what lies over it, dress in
      // queue.ts, and then the gate opens. A clip that came without
      // waiting has changed the video preview by then, and fails below.
      // Open at once when that has happened already, else from the
      // click's own frame loop.
      await page.evaluate(
        (was) =>
          new Promise((done) =>
            window.__clickChanged || window.__frameQueue.stats.waitedForDress > was ? done() : (window.__gate = { was, done }),
          ),
        waitedForDress,
      );
      await page.evaluate(() => (window.__captionsOpen = true));
      await control(url, "/hold?call=Captions&gate=open");
    }
    const { looks } = await clicking;
    const first = looks[0];
    const last = looks[looks.length - 1];
    const changes = changesOf(looks);
    const say = (l) => `${Math.round(l.t)} ms, crop ${l.crop}`;
    const card = `card ${i + 1}${held ? ", a new clip with its captions held" : ""}`;
    if (!last.captions) problems.push(`${card}: no caption showed at the start of the clip`);
    else if (changes.length !== 1) {
      problems.push(
        `${card}: the video preview changed ${changes.length} times, not once: ${changes
          .map((l) => {
            const was = looks[looks.indexOf(l) - 1];
            const what = [l.crop !== was.crop ? "crop frame" : "", l.canvas !== was.canvas ? "canvas" : ""];
            return `${say(l)} (${what.filter(Boolean).join(" and ")})`;
          })
          .join(", ")}`,
      );
      late.push({ card: i + 1, clicked });
    } else if (held && !changes[0].open) problems.push(`${card}: the clip came ${Math.round(changes[0].t)} ms after the click, before its captions were let through`);
    else if (first.canvas === last.canvas && first.crop === last.crop) problems.push(`${card}: the video preview shows what it showed before`);
    else console.log(`        ${card}: one change, ${say(changes[0])}`);
  }
  await page.evaluate(() => {
    window.__frameQueue.dressWait = window.__dressWait;
  });
  // The log is written a quarter of a second after it is said, said.ts,
  // so what is told of the last click may be short of its last lines.
  page.off("request", asked);
  page.off("requestfinished", answered);
  page.off("requestfailed", failed);
  for (const { card, clicked } of late) {
    const next = clicks.find((t) => t > clicked) ?? Infinity;
    const near = heard.filter((h) => h.at >= clicked - 50 && h.at < next - 50).sort((a, b) => a.at - b.at);
    problems.push(`card ${card}, from its click: ${near.map((h) => `${h.at - clicked} ms ${h.what}`).join(", ")}`);
  }
  return problems.length ? problems.join("\n") : null;
}

// Clicks the cards in turn n times, each once the workspace has read
// ahead the first frame of every clip on the cards and its captions, the
// second time after the clip chosen played a hundred frames, more than the
// video preview keeps besides the cards'.
// Each card clicked must put up its clip from what was read, its frame
// from the frames kept and its captions from the layers kept, so nothing
// is decoded or drawn for it after the click, and in one change. Issue
// #383. And nothing is read ahead while the clip plays, found by the
// review of #385.
async function pickRested(page, n) {
  const clips = await shotClips(page);
  if (typeof clips === "string") return clips;
  await canLook(page);
  const problems = [];
  for (let i = 0; i < n; i++) {
    if (i === 1) {
      const ahead = await page.evaluate(() => window.__appFrames.stats.ahead);
      await page.keyboard.press("Space");
      await page.evaluate(
        () =>
          new Promise((done) => {
            const q = window.__frameQueue;
            const from = q.stats.drawn;
            const off = q.listen(() => {
              if (q.stats.drawn - from < 100) return;
              off();
              done();
            });
          }),
      );
      const read = (await page.evaluate(() => window.__appFrames.stats.ahead)) - ahead;
      await page.keyboard.press("Space");
      if (read) problems.push(`${read} frames were read ahead while the clip played`);
    }
    await restAhead(page);
    const { cards, current } = await cardsNow(page);
    const others = cards.filter((k) => k !== current);
    const key = others[i % others.length];
    const start = clips.find((c) => c.key === key)?.start;
    if (start === undefined) return `card ${i + 1}: the clip of card ${key} is not one the engine has`;
    const card = `card ${i + 1}${i === 1 ? ", after a play" : ""}, the clip at ${start.toFixed(2)} s`;
    const problem = await clickedFromAhead(page, key, start, card);
    if (problem) problems.push(problem);
  }
  return problems.length ? problems.join("\n") : null;
}
