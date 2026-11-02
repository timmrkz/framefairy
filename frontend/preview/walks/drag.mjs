// How the video preview follows a drag of the playhead, slow and fast,
// timed on a video the size a camera makes, against the real Go side. Not
// a walk: it checks no rule and fails on no number, it prints what the
// line each drag ends with in the app's log says, see dragged in
// lib/frames/queue.ts. Run it by hand, before and after a change to how
// frames follow the hand, see docs/TESTING.md. Issue #596: a slow drag
// stuttered on Tim's Mac where a fast one did not.
//
//     BRIDGE_URL=http://127.0.0.1:8123/ node frontend/preview/walks/drag.mjs
//
// It adds a video of 1920 by 1080 at 25 frames a second, picks the clip its
// search finds, and drags the playhead over the clip timeline ROUNDS times,
// 3 unless set: slowly, a pixel every frame of the page, to the right, and
// then to the left, and fast, 30 pixels every frame of the page, to the
// right and back. A frame of
// the page is the browser's own pace, the way a hand's moves come. VIEWPORT
// and SCALE as in speed.mjs.
import { control, fromSidebar, handles, high, nextPaint, open, pageUntil, settle } from "./bridge.mjs";

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
const rounds = Number(process.env.ROUNDS ?? 3);
const scale = Number(process.env.SCALE ?? 2);
const [vw, vh] = (process.env.VIEWPORT ?? "1728x1080").split("x").map(Number);

const { browser, page } = await open(url, { viewport: { width: vw, height: vh }, scale });
try {
  await control(url, "/pick?seconds=60&size=1080");
  await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
  // The clip list has a clip of the video added: the page changing is
  // the signal, nothing waits on the clock.
  await page.evaluate(
    () =>
      new Promise((done) => {
        const there = () => {
          const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
          const on = document.querySelector("aside button.episode.current .name")?.textContent ?? "";
          return on.startsWith("folge-") && pane?.querySelectorAll("ol li[data-key]").length > 0;
        };
        if (there()) return done();
        const watch = new MutationObserver(() => {
          if (!there()) return;
          watch.disconnect();
          done();
        });
        watch.observe(document.body, { childList: true, subtree: true, characterData: true });
      }),
  );
  // The first clip picked, and its edges on the clip timeline.
  await page.locator("aside ol li[data-key] button.pick").first().click();
  await pageUntil(page, () => !!document.querySelector('.clip-timeline [aria-label="Clip start"]'));
  await settle(page);

  // The line the next drag ends with, as the app hands it to the Go side:
  // every call goes onto window.__calls, see wails-bridge.ts, so its push
  // is the signal.
  const nextLine = (from) =>
    page.evaluate(
      (from) =>
        new Promise((done) => {
          const calls = window.__calls;
          const push = calls.push;
          const look = () => {
            for (const c of calls.slice(from)) {
              if (c.name !== "Said") continue;
              for (const l of c.args[0] ?? []) {
                if (!/a drag of the playhead let go/.test(l.text)) continue;
                calls.push = push;
                done(l.text);
                return true;
              }
            }
            return false;
          };
          if (look()) return;
          calls.push = function (...xs) {
            const n = push.apply(this, xs);
            look();
            return n;
          };
        }),
      from,
    );

  const drag = async (step, ways) => {
    const g = await handles(page);
    const left = g.start.x + g.start.w;
    const span = g.end.x - left;
    const y = high(g.track);
    const from = left + 0.2 * span;
    const moves = Math.max(2, Math.round((0.6 * span) / step));
    const before = await page.evaluate(() => window.__calls.length);
    await page.mouse.move(ways[0] > 0 ? from : from + moves * step, y);
    await page.mouse.down();
    for (const way of ways) {
      for (let i = 1; i <= moves; i++) {
        await page.mouse.move(way > 0 ? from + i * step : from + (moves - i) * step, y);
        await nextPaint(page);
      }
    }
    await page.mouse.up();
    const line = await nextLine(before);
    await settle(page);
    return line;
  };

  const rows = [];
  for (let r = 0; r < rounds; r++) {
    rows.push(["slow, right", await drag(1, [1])]);
    rows.push(["slow, left", await drag(1, [-1])]);
    rows.push(["fast, right and back", await drag(30, [1, -1])]);
  }
  for (const [how, l] of rows) console.log(`${how}: ${l.replace(/^.*?let go at /, "")}\n`);
} finally {
  await browser.close();
}
