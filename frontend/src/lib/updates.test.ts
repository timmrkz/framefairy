import { describe, expect, test } from "vitest";

import type { UpdateState } from "./api";
import { railDot, standing } from "./updates.svelte";

const at = (u: Partial<UpdateState>): UpdateState => ({
  version: "0.3.0-pr400.102dc91",
  commit: "102dc91",
  channel: "pr-400",
  off: "",
  channels: [{ id: "main" }, { id: "pr-400" }] as UpdateState["channels"],
  picked: "",
  follows: "",
  gone: "",
  building: "",
  phase: "current",
  next: "",
  nextName: "",
  nextCommit: "",
  checked: "",
  written: 0,
  total: 0,
  problem: "",
  detail: "",
  ...u,
});

const inConflict = [{ id: "main" }, { id: "pr-400", conflict: true }] as UpdateState["channels"];

// Tim followed a pull request that closed: the Updates page wore the
// warning and the sidebar nothing, issue #406. Every mark the page shows
// gives the sidebar a dot in the page's colour, and only all well gives
// none.
describe("the dot on Updates in the sidebar", () => {
  const cases: [string, Partial<UpdateState>, string, string][] = [
    ["a pull request followed that closed", { phase: "gone", gone: "pr-400", channels: [{ id: "main" }] as UpdateState["channels"] }, "warn", "warn"],
    ["up to date on a pull request in conflict", { channels: inConflict }, "warn", "warn"],
    ["a check that did not get through", { phase: "failed", problem: "No network." }, "err", "err"],
    ["a newer build ready", { phase: "ready", next: "0.3.0-pr400.abcdef0" }, "new", "ready"],
    ["a newer build ready on a pull request in conflict", { phase: "ready", channels: inConflict }, "new", "ready warn"],
    ["looking for a newer build", { phase: "checking" }, "look", "busy"],
    ["a newer build downloading", { phase: "downloading", total: 100, written: 50 }, "look", "busy"],
    ["a newer commit being built", { building: "abcdef0123" }, "look", "busy"],
    ["downloading on a pull request in conflict", { phase: "downloading", channels: inConflict }, "look", "busy warn"],
    ["up to date", {}, "ok", ""],
    ["built on this Mac", { channel: "", phase: "" }, "idle", ""],
    ["not checked yet", { phase: "" }, "idle", ""],
    ["a build that does not update itself", { off: "Made by make." }, "off", ""],
  ];
  for (const [name, u, mark, dot] of cases) {
    test(`${name}: the page marks ${mark}, the sidebar ${dot || "nothing"}`, () => {
      expect(standing(at(u)).mark).toBe(mark);
      expect(railDot(at(u))).toBe(dot);
    });
  }

  test("is no dot before anything is known", () => {
    expect(railDot(null)).toBe("");
  });
});

// The words of the line under the build, one case for each way things
// stand, so a walk of the Updates page reads them once and does not hold
// every wording itself. The time a check was made is given, never read
// from the clock.
describe("the line under the build", () => {
  const now = Date.UTC(2026, 10, 13, 9, 30);
  const justNow = new Date(now - 10_000).toISOString();
  const cases: [string, Partial<UpdateState>, { working?: string }, string, string][] = [
    ["looking", { phase: "checking" }, {}, "Looking for a newer build", "Of PR #400."],
    ["downloading half", { phase: "downloading", next: "0.3.0-pr400.abcdef0", total: 2_000_000, written: 1_000_000 }, {}, "A newer build is downloading", "0.3.0-pr400.abcdef0. 50 % of 2 MB."],
    ["ready", { phase: "ready", next: "0.3.0-pr400.abcdef0" }, {}, "A newer build is ready", "0.3.0-pr400.abcdef0. Relaunch to finish updating, or it goes in when you quit."],
    ["ready while a search runs", { phase: "ready", next: "0.3.0-pr400.abcdef0" }, { working: "the search" }, "A newer build is ready", "0.3.0-pr400.abcdef0. Relaunch once the search is done, or it goes in when you quit."],
    ["ready with a newer commit building", { phase: "ready", next: "0.3.0-pr400.abcdef0", building: "0123456789" }, {}, "A newer build is ready", "0.3.0-pr400.abcdef0. Relaunch to finish updating, or it goes in when you quit. 0123456 is being built."],
    ["failed", { phase: "failed", problem: "No network.", checked: justNow }, {}, "The check did not get through", "No network. Checked just now."],
    ["a newer commit building", { building: "0123456789", checked: justNow }, {}, "A newer commit is being built", "0123456. This is the build before it, the newest of PR #400 so far. Checked just now."],
    ["up to date in conflict", { channels: inConflict }, {}, "Up to date", "This is the newest build of PR #400. It conflicts with main, so CI has not tested it."],
    ["up to date", { follows: "main", channel: "main" }, {}, "Up to date", "This is the newest build of Branch main."],
    ["closed", { phase: "gone", gone: "pr-400" }, {}, "PR #400 is closed", "Nothing downloads until you choose what to follow next."],
    ["built on this Mac", { channel: "", phase: "" }, {}, "Built on this Mac", "It stays as it is until you choose a channel to follow."],
    ["not checked yet", { phase: "" }, {}, "Not checked yet", "It looks when the app starts and every ten minutes."],
    ["off", { off: "Made by make." }, {}, "This build does not update itself", "Made by make."],
  ];
  for (const [name, u, how, head, more] of cases) {
    test(`${name}: ${head}`, () => {
      expect(standing(at(u), { ...how, now })).toEqual(expect.objectContaining({ head, more }));
    });
  }

  test("says when it last checked, by the hour and minute", () => {
    const earlier = new Date(2026, 10, 13, 7, 5).toISOString();
    expect(standing(at({ checked: earlier }), { now: Date.parse(earlier) + 3_600_000 }).more).toBe("This is the newest build of PR #400. Checked at 07:05.");
  });
});
