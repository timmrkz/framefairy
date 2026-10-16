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
// video preview shows the frame that holds the playhead. Between presses
// the hand rests for REST milliseconds, 250 unless set, about as long as a
// person takes to look and press again. ROUNDS, 3 unless set, walks the
// clip that many times. CODEC=hevc makes the video HEVC with 10-bit colour.
// SCALE, 2 unless set, is the device pixels of a CSS pixel, a Mac's screen.
import { control, fromSidebar, open, settle } from "./bridge.mjs";

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
const rest = Number(process.env.REST ?? 250);
const rounds = Number(process.env.ROUNDS ?? 3);
const scale = Number(process.env.SCALE ?? 2);
const codec = process.env.CODEC === "hevc" ? "&codec=hevc" : "";

const { browser, page, errors } = await open(url, { scale });
try {
  await control(url, `/pick?seconds=60&size=1080${codec}`);
  await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
  await page.waitForFunction(
    () => {
      const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
      const on = document.querySelector("aside button.episode.current .name")?.textContent ?? "";
      return on.startsWith("folge-") && pane?.querySelectorAll("ol li[data-key]").length > 0;
    },
    null,
    { timeout: 180000, polling: 250 },
  );
  const fps = 25;
  await page.evaluate((fps) => {
    // The moment a key goes down, before the app hears it.
    window.addEventListener("keydown", () => (window.__keyAt = performance.now()), { capture: true });
    // Resolves with the milliseconds from the last key until the video
    // preview shows the frame that holds the playhead, or -1 after 3 s.
    window.__followed = (before) =>
      new Promise((done) => {
        const check = () => {
          const s = document.querySelector(".screen");
          const at = Number(s?.dataset.playhead);
          const shown = Number(s?.dataset.shown);
          const holds = Math.floor(at * fps + 1e-6) / fps;
          const took = performance.now() - window.__keyAt;
          if (at !== before && Math.abs(shown - holds) < 1e-3) done(took);
          else if (took > 3000) done(-1);
          else requestAnimationFrame(check);
        };
        check();
      });
  }, fps);

  const times = new Map();
  const press = async (name, key) => {
    const before = await page.evaluate(() => Number(document.querySelector(".screen")?.dataset.playhead));
    await page.keyboard.press(key);
    const took = await page.evaluate((b) => window.__followed(b), before);
    if (!times.has(name)) times.set(name, []);
    times.get(name).push(took);
    await page.waitForTimeout(rest);
  };
  const pickClip = async () => {
    await page.locator("aside ol li[data-key] button.pick").first().click();
    await settle(page);
    await page.evaluate(() => document.body.focus());
    await page.waitForTimeout(500);
  };

  for (let r = 0; r < rounds; r++) {
    await pickClip();
    for (let i = 0; i < 8; i++) await press("Shift+Right, a word on", "Shift+ArrowRight");
    for (let i = 0; i < 3; i++) await press("Left, a frame back", "ArrowLeft");
    for (let i = 0; i < 3; i++) await press("Right, a frame on", "ArrowRight");
    for (let i = 0; i < 8; i++) await press("Shift+Left, a word back", "Shift+ArrowLeft");
    for (let i = 0; i < 4; i++) {
      await press("Shift+Right, a word on", "Shift+ArrowRight");
      await press("Left after a word", "ArrowLeft");
    }
  }

  const stats = await page.evaluate(() => {
    const s = window.__appFrames?.stats;
    return s ? { streams: s.streams, continued: s.continued, kept: s.kept, parked: s.parked, size: s.first } : null;
  });
  const q = (xs, p) => xs[Math.min(xs.length - 1, Math.floor(p * xs.length))];
  const rows = [...times].map(([name, xs]) => {
    const ok = xs.filter((x) => x >= 0).sort((a, b) => a - b);
    return [name, String(xs.length), String(Math.round(q(ok, 0.5))), String(Math.round(q(ok, 0.9))), String(Math.round(ok[ok.length - 1])), String(xs.length - ok.length)];
  });
  const head = ["What", "Presses", "Median ms", "90th ms", "Most ms", "Never"];
  const widths = head.map((h, i) => Math.max(h.length, ...rows.map((r) => r[i].length)));
  const line = (r) => "| " + r.map((c, i) => (i === 0 ? c.padEnd(widths[i]) : c.padStart(widths[i]))).join(" | ") + " |";
  console.log(line(head));
  console.log("| " + widths.map((w, i) => (i === 0 ? "-".repeat(w) : "-".repeat(w - 1) + ":")).join(" | ") + " |");
  for (const r of rows) console.log(line(r));
  console.log(`frames at ${stats?.size}, ${stats?.streams} streams opened, ${stats?.continued} frames waited for on an open stream, ${stats?.kept} from the kept frames, ${stats?.parked} parked`);
  if (errors.length) console.log("errors on the page:", errors.join("\n"));
} finally {
  await browser.close();
}
