// Opens the built preview and hands back a page with an episode workspace
// already on screen, so a probe is a few lines rather than fifty.
//
//   npx vite build --config frontend/preview/vite.config.ts
//   node frontend/preview/probe.mjs        (whatever you called yours)
//
// See .claude/skills/interface/SKILL.md for how to write one and, more to
// the point, for what to measure.
import { chromium } from "/opt/node22/lib/node_modules/playwright/index.mjs";
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import { mkdir, readFile, rename, stat } from "node:fs/promises";
import { tmpdir } from "node:os";
import { extname, join, resolve } from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);
const dist = resolve(import.meta.dirname, "dist");
const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };

// An episode the harness can really play. The video element asks the Go
// side for /media/?path=..., which this server knew nothing about, so
// playing failed every time and everything that happens on the edge from
// paused to playing could not be reached at all. A probe about pressing
// the space bar passed against broken code because of it.
//
// Four hours of black at a frame a second, which ffmpeg makes in under two
// seconds and which comes to 320 KB, kept in the temp folder and made once.
// It is not in the repository: it is built output like everything else in
// dist/. Without ffmpeg there is no video, playing fails the way it did
// before, and a probe that needs it has to say so rather than pass.
//
// VP9 in a WebM, not H.264 in an MP4, because the Chromium that comes with
// Playwright is built without the proprietary codecs: canPlayType for
// avc1 answers with an empty string, so an MP4 loads, errors and never
// plays, which looks exactly like a video that is simply paused. The app
// itself runs in a WebKit view and plays the MP4 the Go side serves. This
// file only has to be a video the harness can play.
const episodeAt = join(tmpdir(), "framefairy-preview-episode-lit2.webm");
const stillsAt = join(tmpdir(), "framefairy-preview-stills");

// Not black any more, but a grey that climbs steadily from the first
// second to the last. Black was unreadable in the one way that matters: a
// black picture and no picture at all look the same, so a probe could not
// tell a video preview that had gone blank from one showing a frame, and
// that is the whole question about the two bugs the player had. Grey
// answers it, and because the grey climbs, a probe can also say roughly
// where in the episode the picture is and that it moved.
//
// It is rough on purpose. The luma comes back through a limited range and
// a lossy encoder, so it is good for "it is showing something" and "it
// moved about that far", and not for a second exactly. Anything needing
// the exact second should read the app's own clock, which is what the app
// shows anyway.
//
// Four hours at a frame a second, which ffmpeg makes in under five
// seconds and which comes to 400 KB, because a slow ramp barely changes
// from one frame to the next. Kept in the temp folder and made once.
//
// Sixteen by nine, which is the shape the fake Go side says the episode
// is. A square picture in a wide preview is letterboxed, and a probe that
// samples anywhere but the middle then reads the black bars and calls the
// picture blank.
const litFilter = "geq=lum='40+160*T/14423':cb=128:cr=128";

// ?fps=25 plays the same episode at 25 frames a second, each second's
// frame shown 25 times, the way an episode on the Mac is. At one frame a
// second the browser says a new frame is on screen once a second, and
// anything that follows the frames, the playhead while it plays above all,
// can only be seen moving in steps of a second. Made once like the other,
// in about a minute, and about 8 MB.
function episodeFile(fps) {
  return fps > 1 ? episodeAt.replace(".webm", `-${fps}fps.webm`) : episodeAt;
}

async function episode(fps = 1) {
  const file = episodeFile(fps);
  try {
    await stat(file);
    return file;
  } catch {}
  try {
    await run("ffmpeg", [
      "-v", "error",
      "-f", "lavfi", "-i", "color=c=black:s=192x108:r=1",
      "-t", "14423",
      "-vf", fps > 1 ? `${litFilter},fps=${fps}` : litFilter,
      "-c:v", "libvpx-vp9", "-b:v", "12k",
      "-deadline", "realtime", "-cpu-used", "8", "-g", String(fps > 1 ? 10 * fps : 120),
      "-pix_fmt", "yuv420p",
      file, "-y",
    ]);
    return file;
  } catch {
    return null;
  }
}

let framesBytes = null;
async function framesBody() {
  const file = await framesEpisode();
  if (!file) return null;
  framesBytes ??= await readFile(file);
  return framesBytes;
}

// The whole file, read once. It was read from disk on every range request,
// and a video element asks for a great many of them.
const episodeBytes = new Map();
async function episodeBody(fps) {
  const file = await episode(fps);
  if (!file) return null;
  if (!episodeBytes.has(file)) episodeBytes.set(file, await readFile(file));
  return episodeBytes.get(file);
}

// A frame of the episode as a picture, which is what the workspace puts
// over the video preview while the picture is stale. The Go side answers
// Still with a path per second, so this answers that path with that
// second, cut out of the same episode. Without it the fallback asked for a
// jpg and was handed a video, so the one thing drawn over a blank picture
// could not be reached in the harness at all.
async function still(res, path) {
  const hit = /still-(\d+)/.exec(path ?? "");
  const file = await episode();
  if (!hit || !file) {
    res.writeHead(404).end("no still");
    return;
  }
  const at = Number(hit[1]);
  const out = join(stillsAt, `${at}.png`);
  try {
    await stat(out);
  } catch {
    try {
      await mkdir(stillsAt, { recursive: true });
      await run("ffmpeg", ["-v", "error", "-i", file, "-ss", String(at), "-frames:v", "1", out, "-y"]);
    } catch {
      res.writeHead(404).end("no still");
      return;
    }
  }
  const body = await readFile(out);
  res.writeHead(200, { "content-type": "image/png", "content-length": body.length });
  res.end(body);
}

// The episode the frame queue plays, see frames() below. VP9 and Opus,
// which the Chromium Playwright brings can decode, in an MP4, which is
// what the frame queue reads, made the way a real episode is shaped: 25
// frames a second and a key frame every four seconds, so a cut lands in
// the middle of a group of pictures the way it does in a camera's file.
//
// The picture says which frame it is. Its top half is the frame number in
// sixteen bars, the highest bit on the left, so a probe reads back from
// the canvas what was drawn rather than trusting what it was told. The
// bottom half is the grey that climbs, as in the other episode.
//
// The sound says where it is. A tone whose pitch climbs, 200 Hz at the
// start and 2 Hz higher every second, with a little noise under it that
// is never the same twice. The tone is what a person hears move. The
// noise is what makes every few milliseconds of the sound unlike every
// other, so a probe can lay what was played over the episode's own sound,
// decoded by ffmpeg, and find a single sample missing, doubled or taken
// from inside a cut.
//
// Ten minutes, made in about half a minute, 8 MB, kept in the temp folder.
export const framesAt = join(tmpdir(), "framefairy-preview-frames-v1.mp4");

export async function framesEpisode() {
  try {
    await stat(framesAt);
    return framesAt;
  } catch {}
  try {
    await run("ffmpeg", [
      "-v", "error",
      "-f", "lavfi", "-i", "color=c=black:s=192x108:r=25",
      "-f", "lavfi", "-i", "aevalsrc=0.35*sin(2*PI*(200*t+t*t))+0.15*(random(0)*2-1):s=48000:c=mono",
      "-t", "600",
      "-vf", "geq=lum='if(lt(Y\\,54)\\,if(bitand(N\\,pow(2\\,15-floor(X/12)))\\,220\\,30)\\,40+160*T/600)':cb=128:cr=128",
      "-c:v", "libvpx-vp9", "-b:v", "200k", "-deadline", "realtime", "-cpu-used", "8",
      "-g", "100", "-keyint_min", "100", "-pix_fmt", "yuv420p",
      "-c:a", "libopus", "-b:a", "96k",
      "-movflags", "+faststart",
      framesAt + ".part.mp4", "-y",
    ]);
    await rename(framesAt + ".part.mp4", framesAt);
    return framesAt;
  } catch {
    return null;
  }
}

// The video element asks for a stretch at a time and will not seek at all
// without a 206, so the range is answered rather than the whole file.
async function media(res, range, fps, frames = false) {
  const body = frames ? await framesBody() : await episodeBody(fps);
  const type = frames ? "video/mp4" : "video/webm";
  if (!body) {
    res.writeHead(404).end("no ffmpeg, so no episode to play");
    return;
  }
  const hit = /bytes=(\d*)-(\d*)/.exec(range ?? "");
  if (!hit) {
    res.writeHead(200, { "content-type": type, "content-length": body.length, "accept-ranges": "bytes" });
    res.end(body);
    return;
  }
  const from = hit[1] ? Number(hit[1]) : 0;
  const to = hit[2] ? Number(hit[2]) : body.length - 1;
  const part = body.subarray(from, to + 1);
  res.writeHead(206, {
    "content-type": type,
    "content-length": part.length,
    "accept-ranges": "bytes",
    "content-range": `bytes ${from}-${to}/${body.length}`,
  });
  res.end(part);
}

// A port of its own per run, so two probes never collide.
let port = 4300 + Math.floor(Math.random() * 400);

// The app as it opens, with nothing clicked. For anything that is not the
// episode workspace: the setup on a new machine, the settings, the app
// with no episode open. workspace() is this with an episode picked.
export async function screen({
  // What the fake Go side should pretend. The modes are in wails-stub.ts.
  query = "",
  width = 1500,
  height = 1000,
  // 2 for a retina picture, which is what Tim sees. 1 makes a measurement
  // in whole pixels easier to read.
  scale = 1,
  // A script to run in the page before anything of the app does, with its
  // argument, the way page.addInitScript takes them: a probe's own
  // recorder, or window.__pieces set before the episode is opened.
  init = null,
  initArg = undefined,
  // The page to open, the app unless it says otherwise.
  page: path = "",
  // More for the browser's command line.
  args = [],
} = {}) {
  // ?slowread=200 answers every range of the frame queue's episode 200 ms
  // late, the way a busy disk or a long episode can. The frame queue reads
  // ahead so a cut never waits on the file, and this is how to show it.
  const slowread = Number(/[?&]slowread=(\d+)/.exec(query)?.[1] ?? 0);
  // ?unread=12 holds the episode back from the video for the first 12
  // seconds after it first asks for it, the way the webview cannot read the
  // file while the machine transcribes an episode just added. The video
  // has read nothing, HAVE_NOTHING, and its clock says zero, while the
  // stills, which the engine reads, still come. It is this server that
  // holds the file, not the fake Go side, so it reads the query here. The
  // first press of the space bar after a search took the playhead to the
  // start of the episode in that state, see playingAt in lib/flow.ts.
  const unread = Number(/[?&]unread=(\d+)/.exec(query)?.[1] ?? 0) * 1000;
  const fps = Number(/[?&]fps=(\d+)/.exec(query)?.[1] ?? 1);
  let unreadFrom = 0;
  const server = createServer(async (req, res) => {
    const url = new URL(req.url, "http://x");
    if (url.pathname.startsWith("/media/")) {
      const asked = url.searchParams.get("path") ?? "";
      if (/still-\d+/.test(asked)) {
        await still(res, asked);
        return;
      }
      if (asked === "/frames.mp4") {
        if (slowread) await new Promise((r) => setTimeout(r, slowread));
        await media(res, req.headers.range, fps, true);
        return;
      }
      if (unread) {
        unreadFrom ||= Date.now();
        const wait = unreadFrom + unread - Date.now();
        if (wait > 0) await new Promise((r) => setTimeout(r, wait));
      }
      await media(res, req.headers.range, fps);
      return;
    }
    const file = url.pathname === "/" ? "/index.html" : url.pathname;
    try {
      const body = await readFile(join(dist, file));
      res.writeHead(200, { "content-type": types[extname(file)] ?? "application/octet-stream" });
      res.end(body);
    } catch {
      res.writeHead(404).end("no");
    }
  });
  const at = port++;
  await new Promise((r) => server.listen(at, r));

  const browser = await chromium.launch({ args });
  const page = await browser.newPage({ viewport: { width, height }, deviceScaleFactor: scale });
  // ?slowseek=150 makes every seek take 150 ms to land, the way WebKit's
  // can on a Mac with a long episode, where the picture is decoded from the
  // last keyframe before it. Chromium lands a seek in this small file in
  // about 3 ms, well inside one frame, so a press of the space bar could
  // never fall in the middle of a jump over a cut, which is where the frame
  // loop used to leave the jump unfinished, see jumpStep in lib/flow.ts.
  // The element answers the way a browser does while a seek is on its way:
  // seeking says yes and the clock says where it was sent. The real seek,
  // with its own seeking and seeked, is made when the time is up. A seek
  // asked for meanwhile replaces it, and load() drops it.
  //
  // A seek made while the element plays holds the picture and the sound
  // where they were until it lands, and then plays on from where it was
  // sent, the way a browser does. The element still says it plays. It went
  // on playing at first, so a jump over a cut showed the frames inside the
  // cut for 150 ms, which no browser does. Every element is slowed, the
  // video preview's waiting element too, see Playback in the interface
  // skill: a cut it absorbs is a seek it made slowly ahead of time.
  const slowseek = Number(/[?&]slowseek=(\d+)/.exec(query)?.[1] ?? 0);
  if (slowseek) {
    await page.addInitScript((ms) => {
      const proto = HTMLMediaElement.prototype;
      const clock = Object.getOwnPropertyDescriptor(proto, "currentTime");
      const seeking = Object.getOwnPropertyDescriptor(proto, "seeking");
      const pausedOf = Object.getOwnPropertyDescriptor(proto, "paused");
      const load = proto.load;
      const play = proto.play;
      const pause = proto.pause;
      const pending = new WeakMap();
      // The play and pause events of the hold itself are the element's
      // business, not the page's.
      const quiet = new WeakMap();
      for (const type of ["play", "pause"]) {
        window.addEventListener(
          type,
          (e) => {
            const q = quiet.get(e.target);
            if (q?.[type]) {
              q[type]--;
              e.stopImmediatePropagation();
            }
          },
          true,
        );
      }
      const hush = (el, type) => {
        const q = quiet.get(el) ?? { play: 0, pause: 0 };
        q[type]++;
        quiet.set(el, q);
      };
      Object.defineProperty(proto, "currentTime", {
        configurable: true,
        get() {
          const p = pending.get(this);
          return p ? p.to : clock.get.call(this);
        },
        set(to) {
          const was = pending.get(this);
          clearTimeout(was?.timer);
          const resume = was ? was.resume : !pausedOf.get.call(this);
          if (!was && resume) {
            hush(this, "pause");
            pause.call(this);
          }
          const p = { to, resume, timer: 0 };
          p.timer = setTimeout(() => {
            pending.delete(this);
            clock.set.call(this, to);
            if (p.resume) {
              hush(this, "play");
              play.call(this).catch(() => {});
            }
          }, ms);
          pending.set(this, p);
        },
      });
      Object.defineProperty(proto, "seeking", {
        configurable: true,
        get() {
          return pending.has(this) || seeking.get.call(this);
        },
      });
      Object.defineProperty(proto, "paused", {
        configurable: true,
        get() {
          const p = pending.get(this);
          return p ? !p.resume : pausedOf.get.call(this);
        },
      });
      proto.play = function () {
        const p = pending.get(this);
        if (!p) return play.call(this);
        if (!p.resume) {
          p.resume = true;
          this.dispatchEvent(new Event("play"));
        }
        return Promise.resolve();
      };
      proto.pause = function () {
        const p = pending.get(this);
        if (!p) return pause.call(this);
        if (p.resume) {
          p.resume = false;
          this.dispatchEvent(new Event("pause"));
        }
      };
      proto.load = function () {
        clearTimeout(pending.get(this)?.timer);
        pending.delete(this);
        return load.call(this);
      };
    }, slowseek);
  }

  // ?framelag=1500 puts the frame a seek lands on on screen 1.5 seconds
  // after the video says it has landed, the way Safari does: seeked comes
  // first and the new frame after it, 1 to 15 ms later on an idle Mac,
  // 110 ms for a cold first frame, and longer while the machine places the
  // crop of every clip a search found. Chromium puts the frame up before
  // it says seeked, so without this nothing here could ever be in the
  // state Tim saw. What is on screen is what requestVideoFrameCallback
  // says, so that is what is held back, for the app and for a probe alike:
  // window.__presented is the moment of the frame on screen.
  const framelag = Number(/[?&]framelag=(\d+)/.exec(query)?.[1] ?? 0);
  if (framelag) {
    await page.addInitScript((lag) => {
      const own = HTMLVideoElement.prototype.requestVideoFrameCallback;
      let landed = -Infinity;
      document.addEventListener("seeked", () => (landed = performance.now()), true);
      window.__presented = -1;
      // Every frame, the probe's own watch of what is on screen.
      const watch = (v) =>
        own.call(v, (now, meta) => {
          const wait = landed + lag - performance.now();
          const show = () => (window.__presented = meta.mediaTime);
          if (wait > 0) setTimeout(show, wait);
          else show();
          watch(v);
        });
      new MutationObserver(() => {
        const v = document.querySelector("video");
        if (v && !v.__watched) {
          v.__watched = true;
          watch(v);
        }
      }).observe(document, { childList: true, subtree: true });
      HTMLVideoElement.prototype.requestVideoFrameCallback = function (cb) {
        return own.call(this, (now, meta) => {
          const wait = landed + lag - performance.now();
          if (wait > 0) setTimeout(() => cb(now, meta), wait);
          else cb(now, meta);
        });
      };
    }, framelag);
  }
  // A probe that reports nothing because the page threw is worse than no
  // probe at all, so anything thrown is printed.
  page.on("pageerror", (e) => console.log("pageerror", String(e)));
  if (init) await page.addInitScript(init, initArg);
  await page.goto(`http://127.0.0.1:${at}/${path}${query}`);
  await page.waitForTimeout(700);

  return {
    page,
    width,
    height,
    async stop() {
      await browser.close();
      server.close();
    },
  };
}

// The episode workspace, open on an episode and on a clip of it, which is
// what nearly every probe is about.
export async function workspace({
  query = "",
  width = 1500,
  height = 1000,
  scale = 1,
  // Picking a clip is what puts the caption settings and the clip panel on
  // screen. Without it half the workspace is not there to look at.
  clip = true,
  init = null,
  initArg = undefined,
} = {}) {
  const open = await screen({ query, width, height, scale, init, initArg });
  const { page } = open;
  await page.getByText("Mein Arm ist zersprungen").first().click();
  await page.waitForTimeout(1100);

  // The sidebar lies over the settings column when it is open, so it is
  // put away first or it covers half of what is being looked at.
  if (await page.evaluate(() => document.querySelector("aside")?.classList.contains("open"))) {
    await page.locator("aside .head .glyph").click();
    await page.mouse.move(width / 2, height / 2);
    await page.waitForTimeout(400);
  }
  if (clip) {
    const first = page.locator("aside .list li button.pick").first();
    if (await first.count()) {
      await first.click();
      await page.waitForTimeout(800);
      // And onto the clip's first word. A clip starts a little before the
      // first thing said in it, the way the engine cuts it, so a playhead
      // sitting on the clip's own start is sitting in silence and the
      // caption box is not on screen at all. Anything about the captions
      // would then be measuring an empty picture. Shift and an arrow step
      // a word, which is exactly the one step needed.
      await page.evaluate(() => document.body.focus());
      await page.keyboard.press("Shift+ArrowRight");
      await page.waitForTimeout(400);
    }
  }

  return open;
}

// Where things are, to the hundredth of a pixel, and whether they sit
// between two of them. An element on a fraction is painted differently
// once a fade puts it on a surface of its own, which is how the info marks
// came to step as they arrived.
export function boxes(page, named) {
  return page.evaluate(
    (list) =>
      list
        .map(([what, sel]) => {
          const el = document.querySelector(sel);
          if (!el) return `${what.padEnd(16)} not on screen`;
          const b = el.getBoundingClientRect();
          const off = Math.abs(b.top - Math.round(b.top));
          return (
            `${what.padEnd(16)} top ${b.top.toFixed(2).padStart(9)}` +
            ` h ${b.height.toFixed(2).padStart(8)}` +
            (off > 0.01 ? `   off a whole pixel by ${off.toFixed(2)}` : "")
          );
        })
        .join("\n"),
    named,
  );
}

// What is actually painted in a stretch of the page, rather than where the
// layout says it is. getBoundingClientRect cannot see a painting bug: it
// gives the layout position, which never moves. This takes the picture and
// reads the pixels back, one row at a time.
export async function rows(page, clip) {
  const png = (await page.screenshot({ clip })).toString("base64");
  return page.evaluate(async (data) => {
    const img = new Image();
    img.src = "data:image/png;base64," + data;
    await img.decode();
    const c = document.createElement("canvas");
    c.width = img.width;
    c.height = img.height;
    const ctx = c.getContext("2d");
    ctx.drawImage(img, 0, 0);
    const d = ctx.getImageData(0, 0, c.width, c.height).data;
    const out = [];
    for (let y = 0; y < c.height; y++) {
      let sum = 0;
      for (let x = 0; x < c.width; x++) sum += d[(y * c.width + x) * 4];
      out.push(Math.round(sum / c.width));
    }
    return out;
  }, png);
}

// The frame queue on its own, see frontend/src/lib/frames/ and "The frame
// queue" in the interface skill: one canvas playing the episode
// framesEpisode() makes, and window.__queue to drive it. ?read and ?hear
// write down what was drawn and what was heard, see preview/frames/main.ts,
// and ?slowread slows the file. The sound card is let go without a gesture,
// which the app gets from the space bar.
export async function frames({ query = "", width = 900, height = 520 } = {}) {
  if (!(await framesEpisode())) throw new Error("no ffmpeg, so no episode for the frame queue");
  const open = await screen({
    query,
    width,
    height,
    page: "preview/frames/index.html",
    args: ["--autoplay-policy=no-user-gesture-required"],
  });
  await open.page.waitForFunction(() => window.__ready, null, { timeout: 20000 });
  const ready = await open.page.evaluate(() => window.__ready);
  if (ready !== true) throw new Error(`the frame queue did not open: ${ready}`);
  return open;
}
