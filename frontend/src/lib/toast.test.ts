import { beforeEach, describe, expect, test, vi } from "vitest";

// What a toast writes into the app's log goes nowhere here. That every
// toast is in the log is said below, by what reaches said, a toast marked
// as one, which is how Settings finds it again, see toastsIn in
// applog.go.
const logged: string[] = [];
const told: unknown[] = [];
vi.mock("./said", () => ({
  said: (text: string, how: { toast?: boolean } = {}) => {
    logged.push(`${how.toast ? "[toast] " : ""}${text}`);
    told.push(how);
  },
}));

import { toasts } from "./toast.svelte";

const texts = () => toasts.list.map((t) => `${t.from}: ${t.text}`);

describe("toasts", () => {
  beforeEach(() => {
    toasts.list = [];
    logged.length = 0;
    told.length = 0;
  });

  // The way macOS stacks its notes: Tim dropped one picture after another
  // and saw only the last, because a newer reason from the same place
  // once replaced the older.
  test("stack newest on top, each a toast of its own", () => {
    toasts.say("app", "a.png is not a video");
    toasts.say("app", "b.png is not a video");
    toasts.say("/v.mp4", "The file is gone", { video: "/v.mp4" });
    expect(texts()).toEqual(["/v.mp4: The file is gone", "app: b.png is not a video", "app: a.png is not a video"]);
  });

  test("the same words from the same place come in on top once, not twice", () => {
    toasts.say("app", "a.png is not a video");
    toasts.say("app", "b.png is not a video");
    const first = toasts.list[1].id;
    toasts.say("app", "a.png is not a video");
    expect(texts()).toEqual(["app: a.png is not a video", "app: b.png is not a video"]);
    // A toast of its own, so its time starts again.
    expect(toasts.list[0].id).not.toBe(first);
  });

  test("the same words from another place are a toast of their own", () => {
    toasts.say("app", "The call failed");
    toasts.say("report", "The call failed");
    expect(texts()).toEqual(["report: The call failed", "app: The call failed"]);
  });

  test("say nothing for empty words, and trim the rest", () => {
    toasts.say("app", "   ");
    toasts.say("app", "  Saved nothing \n");
    expect(texts()).toEqual(["app: Saved nothing"]);
  });

  test("keep what they are about and whether they offer a report", () => {
    toasts.say("/v.mp4", "The search failed", { video: "/v.mp4", report: true });
    expect(toasts.list[0]).toMatchObject({ video: "/v.mp4", report: true });
  });

  test("a place's are taken away together, and only its own", () => {
    toasts.say("app", "one");
    toasts.say("/v.mp4", "two");
    toasts.say("app", "three");
    toasts.clear("app");
    expect(texts()).toEqual(["/v.mp4: two"]);
  });

  test("the newest a place said is its text", () => {
    toasts.say("app", "older");
    toasts.say("/v.mp4", "elsewhere");
    toasts.say("app", "newer");
    expect(toasts.text("app")).toBe("newer");
    expect(toasts.text("report")).toBe("");
  });

  // Every toast is in the log, so one gone unread is not lost.
  test("every toast, and how it went, is in the log", () => {
    toasts.say("app", "a.png is not a video");
    toasts.close(toasts.list[0].id, "ran out");
    toasts.close(12345, "closed");
    expect(logged).toEqual(["[toast] a.png is not a video", "toast ran out: a.png is not a video"]);
    expect(toasts.list).toEqual([]);
  });

  // Settings lists the last toasts again, with the video each was about.
  // Issue #421.
  test("a toast is in the log with its words and its video", () => {
    toasts.say("/v.mp4", "  The file notes.pdf is no video.  ", { video: "/v.mp4" });
    expect(logged).toEqual(["[toast] The file notes.pdf is no video."]);
    expect(told).toEqual([{ level: "warn", video: "/v.mp4", toast: true }]);
  });
});
