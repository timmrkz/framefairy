import { afterEach, describe, expect, test, vi } from "vitest";
import { AppFrames } from "./app";
import { dressedOf, keyOf } from "./captions";
import type { VideoTrack } from "./movie";
import { Picture } from "./picture";
import { frames as framePool } from "./pool";
import { FrameTable } from "./table";

describe("AppFrames", () => {
  afterEach(() => vi.unstubAllGlobals());

  // Found by the review of #385: a card's frame was read while a clip
  // played, a second decoder seeking in the file the play decodes.
  test("are read only while the video preview stands still", () => {
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    const { app } = frames(() => ({ width: 4, height: 2 }));
    let paused = false;
    app.paused = () => paused;
    app.cards([150]);
    expect(app.stats.ahead).toBe(0);
    paused = true;
    app.aheadNow();
    expect(app.stats.ahead).toBe(1);
    app.close();
  });

  // A play asked for holds the reading ahead until it has begun and
  // stopped, and a play asked for that never began until the hand asks
  // again.
  test("wait for a play asked for, and are read once it has played and stopped", () => {
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    const { app } = frames(() => ({ width: 4, height: 2 }));
    let paused = true;
    app.paused = () => paused;
    app.hold();
    app.cards([150]);
    expect(app.stats.ahead).toBe(0);
    paused = false;
    app.began();
    app.aheadNow();
    expect(app.stats.ahead).toBe(0);
    paused = true;
    app.aheadNow();
    expect(app.stats.ahead).toBe(1);
    app.close();
  });

  // Issue #596: soon after a video opens its cards are still being read,
  // and a card's seek stood between a drag and the frame under the hand.
  test("wait while the hand holds the playhead, and are read once it lets go", () => {
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    const { app } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.grip(true);
    app.cards([150]);
    app.aheadNow();
    expect(app.stats.ahead).toBe(0);
    app.grip(false);
    expect(app.stats.ahead).toBe(1);
    app.close();
  });

  // Issue #970: on letting go of a drag the Go side was first handed the
  // words of the place the drag left, and began reading them, and the
  // words of the place the hand rests on only a moment later.
  test("read nothing after a drag until the words of the place it let go of come", () => {
    const asked: URL[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      if (url.startsWith("/frames/ahead")) asked.push(new URL(url, "http://x"));
      return new Promise(() => {});
    });
    const { app } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.ahead([10]);
    app.grip(true);
    void app.frame(60, 0);
    void app.frame(70, 0);
    const was = asked.length;
    app.grip(false);
    expect(asked.length).toBe(was);
    app.ahead([72]);
    const after = asked.slice(was);
    expect(after.length).toBe(1);
    expect(after[0].searchParams.get("held")).toBe(null);
    expect(after[0].searchParams.get("frames")).toBe("72");
    app.close();
  });

  // A press that moves nothing leaves the words where they are: they are
  // the words of the place it rests on.
  test("read the words beside at once after a press that moved nothing", () => {
    const asked: URL[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      if (url.startsWith("/frames/ahead")) asked.push(new URL(url, "http://x"));
      return new Promise(() => {});
    });
    const { app } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.ahead([10]);
    app.grip(true);
    const was = asked.length;
    app.grip(false);
    expect(asked.slice(was).map((u) => [u.searchParams.get("held"), u.searchParams.get("frames")])).toEqual([[null, "10"]]);
    app.close();
  });

  // Issue #886: a dragged app edge read the cards' frames again at every
  // size it passed. They wait until the size comes to rest.
  test("wait while the app is resized, and are read once its size rests", () => {
    vi.stubGlobal("fetch", () => new Promise(() => {}));
    const { app } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.resizing(true);
    app.cards([150]);
    app.aheadNow();
    expect(app.stats.ahead).toBe(0);
    app.resizing(false);
    expect(app.stats.ahead).toBe(1);
    app.close();
  });

  // The frames a step with the arrow keys goes to are kept here: the
  // frame on screen is shown at once, and the Go side is only told where
  // the hand is, and which frames around it are kept here, so it reads
  // the others.
  test("a place kept here is shown at once, and the Go side told where it is", async () => {
    const asked: string[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      asked.push(url);
      return new Promise(() => {});
    });
    const { app, picture } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    const was = asked.length;
    app.keep(39, picture(39, 4, 2));
    app.keep(90, picture(90, 4, 2));
    app.keep(40, picture(40, 4, 2));
    const f = await app.frame(40, 0);
    expect(f).not.toBe(null);
    expect(app.stats.kept).toBe(1);
    expect(asked.length).toBe(was + 1);
    const told = new URL(asked[was], "http://x");
    expect(told.pathname).toBe("/frames/look");
    expect(told.searchParams.get("frame")).toBe("40");
    expect(told.searchParams.get("shown")).toBe("40");
    expect(told.searchParams.get("kept")).toBe("1");
    expect(told.searchParams.get("has")).toBe("39,40");
    app.close();
  });

  // A frame from before a crop moved or a word corrected is dressed again,
  // the picture kept, as the frames are dressed now, see
  // FrameQueue.dressedAsNow.
  test("dresses a frame as the frames are dressed now", async () => {
    const ask = { path: "/v.mp4", clip: "a", framing: "f2", drafts: {} };
    const dressed: URL[] = [];
    vi.stubGlobal("fetch", async (url: string) => {
      if (!url.startsWith("/frames/dress")) return new Promise(() => {});
      dressed.push(new URL(url, "http://x"));
      const sent = {
        piece: 0,
        in: true,
        clip: true,
        turn: 0,
        width: 4,
        height: 2,
        cropWidth: 2,
        left: 1,
        moved: true,
        mark: null,
        movable: true,
        key: keyOf(ask),
      };
      return new Response(null, { status: 200, headers: { "X-Frame-Dress": JSON.stringify(sent) } });
    });
    const { app, picture } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    const kept = picture(40, 4, 2);
    kept.over = dressedOf(
      JSON.stringify({
        piece: 0,
        in: true,
        clip: true,
        turn: 0,
        width: 4,
        height: 2,
        cropWidth: 2,
        left: 0,
        moved: false,
        mark: null,
        movable: true,
        key: "old",
      }),
      null,
      () => null,
    );
    app.dressWith(ask);
    await app.dress(kept, app.frames.begins(40));
    expect(kept.over?.dress?.left).toBe(1);
    expect(kept.over?.dress?.key).toBe(app.dressKey);
    expect(dressed[0].searchParams.get("framing")).toBe("f2");
    expect(dressed[0].searchParams.get("at")).toBe(String(app.frames.begins(40)));
    app.close();
  });

  test("a hand that lets go is told to the Go side, with no frame asked", async () => {
    const asked: string[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      // The looks only: the reading ahead starts as the hand lets go.
      if (url.startsWith("/frames/look")) asked.push(url);
      return new Promise(() => {});
    });
    const { app, picture } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.keep(40, picture(40, 4, 2));
    app.keep(41, picture(41, 4, 2));
    app.grip(true);
    await app.frame(41, 0);
    await app.frame(40, 0);
    const was = asked.length;
    app.grip(false);
    // The Go side heard the hand drag to 40, and now that it rests there.
    expect(asked.length).toBe(was + 1);
    const told = new URL(asked[was], "http://x");
    expect(told.searchParams.get("frame")).toBe("40");
    expect(told.searchParams.get("move")).toBe(null);
    // The frame asked there after it tells nothing more.
    await app.frame(40, 0);
    expect(asked.length).toBe(was + 1);
    app.close();
  });
});

// Frames of a 200 frame track on a Go side that never answers, so a frame
// not kept is never handed over.
function frames(size: () => { width: number; height: number }, rate = 25) {
  const n = 200;
  const samples = { count: n, timescale: rate, start: 0, step: 1 };
  const track: VideoTrack = { kind: "video", width: 4, height: 2, samples, frame: 1 / rate };
  // The frame on screen is drawn at once.
  const app = new AppFrames("/v.mp4", track, tableOf(n, rate), size, () => {}, (then) => then());
  // Pulls go to the fetch the test stubs, and bring no frames.
  (app as unknown as { puller: unknown }).puller = {
    pull: (url: string) =>
      (fetch(url) as Promise<{ status: number }>).then((r) => ({
        status: r.status,
        end: false,
        error: "",
        closed: false,
        times: "",
        light: "",
        exact: false,
        kept: false,
        near: false,
        keep: [],
        spare: [],
        frames: [],
      })),
    close() {},
  };
  const picture = (rank: number, w: number, h: number) => new Picture(new Uint8Array(w * h * 4), w, h, rank * 40000);
  return { app, picture };
}

// A stream on a Go side the test answers by hand: each pull waits until
// the test hands it the ranks it brings, each frame's first byte its rank,
// and the times the Go side tells with a stream's first frame.
function goSide(rate = 25) {
  vi.stubGlobal("fetch", (url: string) =>
    Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ id: `s${url.length}` }), text: () => Promise.resolve("") }),
  );
  const { app } = frames(() => ({ width: 4, height: 2 }), rate);
  app.paused = () => false;
  const asked: { url: string; give: (ranks: number[], times?: string, exact?: boolean, keep?: number[]) => void; shut: () => void; overtaken: () => void }[] = [];
  const came: (() => void)[] = [];
  (app as unknown as { puller: unknown }).puller = {
    pull: (url: string) =>
      new Promise((done) => {
        asked.push({
          url,
          give: (ranks, times = "", exact = false, keep = []) =>
            done({
              status: 200,
              end: false,
              error: "",
              closed: false,
              times,
              light: "",
              exact,
              kept: false,
              near: false,
              keep,
              spare: [],
              frames: ranks.map((r) => ({ at: r / rate, frame: new Picture(new Uint8Array(4 * 2 * 4).fill(r), 4, 2, (r * 1e6) / rate) })),
            }),
          // Closed on the Go side, to make room for newer streams.
          shut: () => done({ status: 200, end: false, error: "", closed: true, times: "", light: "", exact: false, kept: false, near: false, keep: [], spare: [], frames: [] }),
          // A newer look took this one's place.
          overtaken: () => done({ status: 204, end: false, error: "", closed: false, times: "", light: "", exact: false, kept: false, near: false, keep: [], spare: [], frames: [] }),
        });
        for (const c of came.splice(0)) c();
      }),
    close() {},
  };
  // Settles once the Go side has been asked n times in all.
  const pulls = (n: number) =>
    new Promise<void>((done) => {
      const look = () => (asked.length >= n ? done() : came.push(look));
      look();
    });
  const rankOf = async (p: Promise<Picture | null>) => (await p)?.data?.[0] ?? null;
  return { app, asked, pulls, rankOf };
}

describe("a place while the video preview stands still", () => {
  afterEach(() => vi.unstubAllGlobals());

  // The Go side decides how its decoder gets to the place, see
  // engine.Schedule: the window says where the hand is and which frame is
  // on screen, puts up a frame the Go side sends on the way, and asks for
  // the place again until its own frame comes.
  test("is a look, and a frame on the way is put up before the place's own", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    app.paused = () => true;
    app.shownRank = () => 3;
    const passed: number[] = [];
    app.onPassing = (rank) => passed.push(rank);
    const place = app.frame(40, 0);
    await pulls(1);
    expect(asked[0].url).toContain("/frames/look?");
    expect(asked[0].url).toContain("frame=40");
    expect(asked[0].url).toContain("shown=3");
    asked[0].give([25]);
    await pulls(2);
    expect(passed).toEqual([25]);
    expect(asked[1].url).toContain("frame=40");
    asked[1].give([40], "", true);
    expect(await rankOf(place)).toBe(40);
    app.close();
  });

  // The frame under the hand and the first frame of the play cued there
  // as the hand lets go are the same frame: it is asked for once and sent
  // once, and both get it.
  test("asked twice at once is one look", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    app.paused = () => true;
    const still = app.frame(40, 0);
    await pulls(1);
    const cue = app.frame(40, 0);
    asked[0].give([40], "", true);
    expect(await rankOf(still)).toBe(40);
    expect(await rankOf(cue)).toBe(40);
    expect(asked.length).toBe(1);
    app.close();
  });

  // The Go side serves each request on a goroutine of its own, so they may
  // come in another order than they were sent: each look and each ask to
  // read ahead says which window sent it and how many it had sent, and
  // the Go side goes by the one sent last, see engine.Sent.
  test("says where it stands among what the window sent", async () => {
    const { app, asked, pulls } = goSide();
    app.paused = () => true;
    void app.frame(40, 0);
    await pulls(1);
    app.ahead([60]);
    await pulls(2);
    void app.frame(80, 0);
    await pulls(3);
    const sent = asked.map((a) => new URL(a.url, "http://x").searchParams);
    expect(new Set(sent.map((q) => q.get("win"))).size).toBe(1);
    expect(sent[0].get("win")).not.toBe("");
    expect(sent.map((q) => Number(q.get("n")))).toEqual([1, 2, 3]);
    app.close();
  });

  // A newer place took this one's: the Go side answers with nothing, and
  // the frame under the hand comes for the newer one.
  test("overtaken by a newer one gets nothing", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    app.paused = () => true;
    const old = app.frame(40, 0);
    await pulls(1);
    asked[0].overtaken();
    expect(await rankOf(old)).toBe(null);
    expect(app.stats.overtaken).toBe(1);
    app.close();
  });
});

describe("the frames read while the hand rests", () => {
  afterEach(() => vi.unstubAllGlobals());

  // The Go side sends what it read ahead, and a step to the word beside or
  // a card clicked then shows its frame with no request. A card's frame
  // stays while its card is there, and is not sent again.
  test("are kept here, the cards' while their cards are there", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    app.paused = () => true;
    app.cards([150]);
    await pulls(1);
    app.ahead([60]);
    await pulls(2);
    const read = asked[1];
    expect(read.url).toContain("/frames/ahead?");
    read.give([59, 60, 150]);
    await app.rested();
    let before = asked.length;
    expect(await rankOf(app.frame(60, 0))).toBe(60);
    expect(await rankOf(app.frame(150, 0))).toBe(150);
    // Only where the hand is goes to the Go side, with no frame asked for.
    expect(
      asked
        .slice(before)
        .filter((a) => a.url.startsWith("/frames/look?"))
        .map((a) => new URL(a.url, "http://x").searchParams.get("shown")),
    ).toEqual(["60", "150"]);
    before = asked.length;
    app.ahead([70]);
    await pulls(before + 1);
    expect(new URL(asked[before].url, "http://x").searchParams.get("has")?.split(",")).toContain("150");
    // Its card gone, the clip of a card clicked, its frame is kept on as
    // any other.
    app.cards([]);
    asked[before].give([]);
    before = asked.length;
    expect(await rankOf(app.frame(150, 0))).toBe(150);
    expect(asked.slice(before).filter((a) => a.url.startsWith("/frames/look?") && !a.url.includes("kept=1"))).toEqual([]);
    app.close();
  });

  // A card's frame dressed for its clip as it was, its captions not come
  // yet, is read again once they are, and not named as held.
  test("a card's frame is read again once its clip is dressed otherwise", async () => {
    const { app, asked, pulls } = goSide();
    app.paused = () => true;
    app.cards([150]);
    await pulls(1);
    asked[0].give([150]);
    await app.rested();
    app.cards([150], [{ path: "/v.mp4", clip: "a", framing: "f", drafts: {} }]);
    await pulls(2);
    const q = new URL(asked[1].url, "http://x").searchParams;
    expect(q.get("has")?.split(",")).not.toContain("150");
    expect(q.get("cardFramings")).toBe("f");
    app.close();
  });

  // A word's frames held here are not sent again, and the frames that
  // come with them make room by letting go of the frames used longest
  // ago. The word's frame, kept first, was let go of for them, and the
  // word key found it missing. The Go side names the frames read for the
  // words, see engine.Ahead.Keep.
  test("keep the frame of a word held here, whatever comes with them", async () => {
    const { app, asked, pulls, rankOf } = goSide();
    app.paused = () => true;
    const pic = (r: number) => new Picture(new Uint8Array(4 * 2 * 4).fill(r), 4, 2, (r * 1e6) / 25);
    app.keep(60, pic(60));
    for (let r = 200; r < 263; r++) app.keep(r, pic(r));
    app.ahead([60]);
    await pulls(1);
    asked[0].give([300, 301, 302, 303, 304, 305], "", false, [59, 60, 61, 62, 63, 64]);
    await app.rested();
    // Shown from here at once, with no look at the Go side.
    const kept = app.stats.kept;
    const shown = app.frame(60, 0);
    expect(app.stats.kept).toBe(kept + 1);
    expect(await rankOf(shown)).toBe(60);
    app.close();
  });

  // Each pull once took as many kept buffers as the n of its URL said,
  // which is where the ask stands among what the app sent, not how many
  // frames it asks for: the hundredth look took the whole pool, and the
  // play that came next found it empty.
  test("take one kept buffer for a look, whatever its number", async () => {
    vi.stubGlobal("fetch", () => Promise.resolve(new Response(null, { status: 204 })));
    const took = vi.spyOn(framePool, "takeUpTo");
    const samples = { count: 200, timescale: 25, start: 0, step: 1 };
    const app = new AppFrames("/v.mp4", { kind: "video", width: 4, height: 2, samples, frame: 1 / 25 }, tableOf(200, 25), () => ({ width: 4, height: 2 }), () => {});
    await app.puller.pull("/frames/look?n=100", 4, 2);
    expect(took).toHaveBeenCalledWith(4 * 2 * 4, 1);
    took.mockRestore();
    app.close();
  });

  // The walks wait on rested before a step to the word beside, and the
  // step must find its frame kept. Asked again for other words while the
  // first ask was on its way, rested let the walk go before the newer
  // ask was read, so the walk pressed the key with the frame still to
  // come, on the CI run of #757.
  test("keep the walks waiting while they are asked again for other words", async () => {
    const answers: ((status: number) => void)[] = [];
    vi.stubGlobal("fetch", (url: string) =>
      url.startsWith("/frames/ahead")
        ? new Promise((done) => answers.push((status) => done({ ok: true, status, headers: new Headers(), text: () => Promise.resolve("") })))
        : new Promise(() => {}),
    );
    const { app } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.ahead([75]);
    const order: string[] = [];
    const rested = app.rested().then(() => order.push("rested"));
    app.ahead([75, 30]);
    // The first ask is answered as the Go side answers one a newer ask
    // took the place of.
    answers[0](204);
    for (let i = 0; i < 4; i++) await Promise.resolve();
    order.push("first answered");
    answers[1](200);
    await rested;
    expect(order).toEqual(["first answered", "rested"]);
    app.close();
  });
});

// A table of n frames at rate a second from the start of the file, as the
// engine would send it: each holds from a little before it begins.
function tableOf(n: number, rate: number): FrameTable {
  const from = Float64Array.from({ length: n }, (_, r) => (r === 0 ? -Infinity : r / rate - 0.0015));
  return new FrameTable(from, Float64Array.from({ length: n }, (_, r) => r / rate));
}
