import { afterEach, describe, expect, test, vi } from "vitest";
import { AppFrames } from "./app";
import type { VideoTrack } from "./movie";
import { Picture } from "./picture";

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
  // the hand is, so it reads the frames around it.
  test("a place kept here is shown at once, and the Go side told where it is", async () => {
    const asked: string[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      asked.push(url);
      return new Promise(() => {});
    });
    const { app, picture } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    const was = asked.length;
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
    app.close();
  });

  test("a hand that lets go on a kept frame is told to the Go side", async () => {
    const asked: string[] = [];
    vi.stubGlobal("fetch", (url: string) => {
      asked.push(url);
      return new Promise(() => {});
    });
    const { app, picture } = frames(() => ({ width: 4, height: 2 }));
    app.paused = () => true;
    app.keep(40, picture(40, 4, 2));
    app.keep(41, picture(41, 4, 2));
    app.grip(true);
    await app.frame(41, 0);
    await app.frame(40, 0);
    app.grip(false);
    const was = asked.length;
    await app.frame(40, 0);
    // The Go side heard the hand drag to 40, and now that it rests there.
    expect(asked.length).toBe(was + 1);
    const told = new URL(asked[was], "http://x");
    expect(told.searchParams.get("frame")).toBe("40");
    expect(told.searchParams.get("move")).toBe(null);
    app.close();
  });
});

// Frames of a 200 frame track on a Go side that never answers, so a frame
// not kept is never handed over.
function frames(size: () => { width: number; height: number }, rate = 25) {
  const n = 200;
  const samples = { count: n, timescale: rate, start: 0, step: 1 };
  const track: VideoTrack = { kind: "video", width: 4, height: 2, samples, frame: 1 / rate };
  const app = new AppFrames("/v.mp4", track, size, () => {});
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
  const asked: { url: string; give: (ranks: number[], times?: string, exact?: boolean) => void; shut: () => void; overtaken: () => void }[] = [];
  const came: (() => void)[] = [];
  (app as unknown as { puller: unknown }).puller = {
    pull: (url: string) =>
      new Promise((done) => {
        asked.push({
          url,
          give: (ranks, times = "", exact = false) =>
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
              spare: [],
              frames: ranks.map((r) => ({ at: r / rate, frame: new Picture(new Uint8Array(4 * 2 * 4).fill(r), 4, 2, (r * 1e6) / rate) })),
            }),
          // Closed on the Go side, to make room for newer streams.
          shut: () => done({ status: 200, end: false, error: "", closed: true, times: "", light: "", exact: false, kept: false, near: false, spare: [], frames: [] }),
          // A newer look took this one's place.
          overtaken: () => done({ status: 204, end: false, error: "", closed: false, times: "", light: "", exact: false, kept: false, near: false, spare: [], frames: [] }),
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
    expect(asked.slice(before).map((a) => new URL(a.url, "http://x").searchParams.get("shown"))).toEqual(["60", "150"]);
    before = asked.length;
    app.ahead([70]);
    await pulls(before + 1);
    expect(new URL(asked[before].url, "http://x").searchParams.get("has")?.split(",")).toContain("150");
    app.cards([]);
    asked[before].give([]);
    const look = app.frame(150, 0);
    await pulls(before + 3);
    expect(asked.slice(before).some((a) => a.url.includes("/frames/look?") && a.url.includes("frame=150"))).toBe(true);
    app.close();
    void look;
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
