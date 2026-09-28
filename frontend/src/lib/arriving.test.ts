import { describe, expect, test } from "vitest";
import type { Job } from "./api";
import { arriving } from "./arriving";
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

  test("a clip whose job was cut off or failed stays, still, and carries on with a click", () => {
    let carried = "";
    const cut = job({ id: "c", kind: "clip", state: "interrupted", underway: [{ n: 1, start: 600, end: 600, step: "hearing" }] });
    const failed = job({ id: "f", kind: "clip", state: "failed", error: "nothing is said after 1:00:00", underway: [{ n: 1, start: 3600, end: 3600, step: "failed" }] });
    const rows = arriving([cut, failed], never, (j) => (carried = j.id));
    expect(rows.map((r) => [r.key, r.stopped, r.what])).toEqual([
      ["c/1", true, "Interrupted. Click to carry on"],
      ["f/1", true, "Failed. Click to try again"],
    ]);
    expect(rows[1].left).toBe("nothing is said after 1:00:00");
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
