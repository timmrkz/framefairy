import { describe, expect, test } from "vitest";
import type { Job } from "./api";
import { arriving, Carrier, OnTheWay, type Arriving } from "./arriving";
import { arrivalLine } from "./steps";

function job(over: Partial<Job>): Job {
  return { id: "j", episode: "e", kind: "search", label: "Find clips", state: "running", queued: "", lane: "finding", ...over };
}

const never = () => false;
const nothing = () => {};

describe("every clip on its way comes in the same way, whoever proposed it", () => {
  test("a search's clip and a clip made by hand, each by what is done to it", () => {
    const search = job({
      id: "s",
      step: "finding",
      progress: { kind: "progress", text: "3 of 12 found", fraction: 0.4, remaining: 61, elapsed: 1, time: "" },
      underway: [{ n: 4, start: 900, end: 925, title: "Ein Moment", step: "framing" }],
    });
    const hand = job({
      id: "h",
      kind: "clip",
      lane: "framing",
      step: "framing",
      underway: [{ n: 1, start: 1000, end: 1025, title: "Von 0:16:38 an", step: "framing" }],
    });
    const rows = arriving([search, hand], never, nothing);
    expect(rows.map((r) => r.key)).toEqual(["s/4", "h/1"]);
    // The same words and the same fill, and never the search's time left,
    // which is the whole search's and not its clip's.
    for (const r of rows) {
      expect(r.what).toBe("Placing the crop");
      expect(r.left).toBe("");
      expect(r.fraction).toBe(-1);
    }
  });

  test("a clip made by hand where nothing is heard yet says how far the hearing is", () => {
    const hearing = job({
      kind: "clip",
      lane: "hearing",
      step: "hearing",
      progress: { kind: "progress", text: "transcribing", fraction: 0.3, remaining: 20, elapsed: 1, time: "" },
      underway: [{ n: 1, start: 5000, end: 5000, step: "hearing" }],
    });
    expect(arrivalLine(hearing, hearing.underway![0])).toEqual({
      what: "Transcribing",
      left: "About 0:20 left",
      fraction: 0.3,
    });
    const waiting = job({ kind: "clip", lane: "framing", step: "waiting", underway: [{ n: 1, start: 5000, end: 5000, step: "waiting" }] });
    expect(arrivalLine(waiting, waiting.underway![0]).what).toBe("Waiting to place the crop");
  });

  test("a clip made by hand fills while its crop is placed, a search's clip does not", () => {
    const progress = { kind: "progress", text: "placing the crop", fraction: 0.6, remaining: 2, elapsed: 1, time: "" };
    const hand = job({ kind: "clip", lane: "framing", step: "framing", progress, underway: [{ n: 1, start: 1000, end: 1025, step: "framing" }] });
    expect(arrivalLine(hand, hand.underway![0])).toEqual({ what: "Placing the crop", left: "About 0:05 left", fraction: 0.6 });
    const search = job({ step: "finding", progress, underway: [{ n: 1, start: 1000, end: 1025, step: "framing" }] });
    expect(arrivalLine(search, search.underway![0]).fraction).toBe(-1);
  });

  test("a clip whose job was cut off or failed stays, still, and carries on with a click", () => {
    let carried = "";
    const cut = job({ id: "c", kind: "clip", state: "interrupted", underway: [{ n: 1, start: 600, end: 600, step: "hearing" }] });
    const called = job({ id: "s", kind: "clip", state: "interrupted", step: "stopped", underway: [{ n: 1, start: 900, end: 900, step: "stopped" }] });
    const failed = job({ id: "f", kind: "clip", state: "failed", error: "nothing is said after 1:00:00", underway: [{ n: 1, start: 3600, end: 3600, step: "failed" }] });
    const rows = arriving([cut, called, failed], never, (j) => (carried = j.id));
    expect(rows.map((r) => [r.key, r.stopped, r.what])).toEqual([
      ["c/1", true, "Interrupted. Click Continue"],
      ["s/1", true, "Stopped. Click Continue"],
      ["f/1", true, "Failed. Click Continue"],
    ]);
    expect(rows[2].left).toBe("nothing is said after 1:00:00");
    rows[0].oncontinue?.();
    expect(carried).toBe("c");
  });

  test("done, called off and job without clips on the way have none", () => {
    expect(arriving([job({ state: "done", underway: [{ n: 1, start: 1, end: 2, step: "framing" }] })], never, nothing)).toEqual([]);
    expect(arriving([job({ state: "cancelled" })], never, nothing)).toEqual([]);
    expect(arriving([job({ kind: "render", lane: "rendering", step: "rendering" })], never, nothing)).toEqual([]);
  });

  test("a job told to stop keeps its clips' place and stops moving", () => {
    const s = job({ underway: [{ n: 2, start: 10, end: 30, step: "framing" }] });
    const [row] = arriving([s], () => true, nothing);
    expect(row.what).toBe("Stopping");
    expect(row.still).toBe(true);
  });
});

describe("a card is held from the moment its clip is written until the list has it", () => {
  const card = (key: string, over: Partial<Arriving> = {}): Arriving => ({
    key, job: "s", n: 1, start: 10, end: 30, title: "", what: "Placing the crop", left: "", fraction: -1, ...over,
  });

  test("in the same pass it leaves, not a render later", () => {
    const way = new OnTheWay();
    way.cards([card("s/1")], 5, 4);
    // Written: gone from the job's list, and still on screen in that pass.
    const now = way.cards([], 5, 4);
    expect(now.map((a) => [a.key, a.held])).toEqual([["s/1", true]]);
    expect(way.unread(5)).toBe(true);
    // A read asked before it left knows nothing of it and keeps it.
    expect(way.cards([], 6, 4).length).toBe(1);
    expect(way.unread(6)).toBe(false);
    // The first read asked after it left holds the clip: it goes.
    expect(way.cards([], 6, 5)).toEqual([]);
  });

  test("a card that stopped is not held, it says so where it is", () => {
    const way = new OnTheWay();
    way.cards([card("s/1", { stopped: true })], 3, 2);
    expect(way.cards([], 3, 2)).toEqual([]);
  });

  test("a card back on the way is live again, not held twice", () => {
    const way = new OnTheWay();
    way.cards([card("s/1")], 3, 2);
    way.cards([], 3, 2);
    expect(way.cards([card("s/1")], 3, 2).map((a) => a.held)).toEqual([undefined]);
  });
});

describe("one card carries the search's work, from the first card to the last", () => {
  const card = (key: string, start: number, over: Partial<Arriving> & { held?: boolean } = {}) => ({
    key, job: "s", n: Number(key.split("/")[1]), start, end: start + 25, title: "", what: "Fitting to the length", left: "", fraction: -1, ...over,
  });

  test("the first of the search's cards in the list, and only that one", () => {
    const carrier = new Carrier();
    const cards = [card("s/3", 900), card("s/2", 400), card("s/1", 100), card("h/1", 50, { job: "h" })];
    expect(carrier.pick(cards, "s")).toBe("s/1");
  });

  test("it keeps it while it is on the way, and passes it down once its clip is written", () => {
    const carrier = new Carrier();
    carrier.pick([card("s/2", 400), card("s/3", 900)], "s");
    // A card named above it does not take it away.
    expect(carrier.pick([card("s/1", 100), card("s/2", 400), card("s/3", 900)], "s")).toBe("s/2");
    // Written, and held until the list reads it: its work is done.
    expect(carrier.pick([card("s/1", 100), card("s/2", 400, { held: true }), card("s/3", 900)], "s")).toBe("s/1");
    expect(carrier.pick([card("s/3", 900)], "s")).toBe("s/3");
    expect(carrier.pick([], "s")).toBe("");
  });

  test("a card still being fitted is a card like any other", () => {
    const cards = [
      card("s/1", 100, { step: "framing" }),
      card("s/2", 400, { step: "fitting" }),
      card("s/3", 900, { step: "fitting" }),
    ];
    expect(new Carrier().pick(cards, "s")).toBe("s/1");
  });

  test("first is where the list puts a card, when it says", () => {
    // The list keeps a search's cards in the places they came into, so the
    // first card named is first, wherever in the episode it lies.
    const order = new Map([["s/3", 0], ["s/1", 1]]);
    const cards = [card("s/1", 100), card("s/3", 900)];
    expect(new Carrier().pick(cards, "s", (a) => order.get(a.key) ?? Infinity)).toBe("s/3");
  });

  test("a card that stopped carries nothing", () => {
    expect(new Carrier().pick([card("s/1", 100, { stopped: true }), card("s/2", 400)], "s")).toBe("s/2");
  });
});
