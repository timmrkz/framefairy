// How evenly the video preview plays, timed on a video the size a camera
// makes, against the real Go side. Not a walk: it checks no rule and fails
// on no number, it prints a table. Run it by hand, before and after a
// change to how a play is drawn, see docs/TESTING.md.
//
//     BRIDGE_URL=http://127.0.0.1:8123/ node frontend/preview/walks/smooth.mjs
//
// It adds a video of 1920 by 1080 at 25 frames a second, plays the clip its
// search finds to the end ROUNDS times, 3 unless set, and prints what the
// line each play ends with in the app's log says, see lib/frames/even.ts:
// how many refreshes were missed, and where the time of an animation frame
// went, typical, most and on the animation frames before a missed refresh.
// GPU=1 draws on the WebGPU canvas the app draws on, where the walks draw
// on a 2D one to read the frame back. VIEWPORT and SCALE as in speed.mjs.
import { fromSidebar, open } from "./bridge.mjs";
import { control } from "./bridge.mjs";

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
const rounds = Number(process.env.ROUNDS ?? 3);
const scale = Number(process.env.SCALE ?? 2);
const [vw, vh] = (process.env.VIEWPORT ?? "1728x1080").split("x").map(Number);

// On the sound card's clock, as the app plays without a walk: how evenly
// it plays is what is timed.
const { browser, page } = await open(url, { viewport: { width: vw, height: vh }, scale, clock: "sound" });
try {
  if (process.env.GPU === "1") {
    await page.addInitScript(() => (window.__flatScreen = false));
    await page.reload();
  }
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
  const rows = [];
  for (let r = 0; r < rounds; r++) {
    const before = await page.evaluate(() => window.__calls.length);
    await page.locator("ol li[data-key]").first().click();
    await page.mouse.move(vw / 2, vh / 2);
    await page.keyboard.press("Space");
    // The line the play ends with, as the app hands it to the Go side:
    // every call goes onto window.__calls, see wails-bridge.ts, so its
    // push is the signal.
    const line = await page.evaluate(
      (from) =>
        new Promise((done) => {
          const calls = window.__calls;
          const push = calls.push;
          const look = () => {
            for (const c of calls.slice(from)) {
              if (c.name !== "Said") continue;
              for (const l of c.args[0] ?? []) {
                if (!/video preview: played to the end/.test(l.text)) continue;
                calls.push = push;
                done(l.text);
                return;
              }
            }
          };
          calls.push = function (...xs) {
            const n = push.apply(this, xs);
            look();
            return n;
          };
          look();
        }),
      before,
    );
    rows.push(line);
  }
  for (const [i, l] of rows.entries()) console.log(`play ${i + 1}: ${l.replace(/^.*?played to the end, /, "")}\n`);
} finally {
  await browser.close();
}
