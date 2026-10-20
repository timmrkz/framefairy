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
