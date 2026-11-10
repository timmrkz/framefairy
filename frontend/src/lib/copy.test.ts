import { describe, expect, it } from "vitest";
import { between, joined, pick } from "./copy";

describe("pick", () => {
  it.each([
    ["starts at the current card", null, "b", "d", { from: "b", to: "d" }],
    ["starts at the card clicked where none is current", null, null, "d", { from: "d", to: "d" }],
    ["picks on from where the first started", { from: "d", to: "d" }, null, "f", { from: "d", to: "f" }],
    ["keeps its start when the current card moved", { from: "b", to: "d" }, "e", "a", { from: "b", to: "a" }],
  ] as const)("%s", (_, was, current, clicked, want) => {
    expect(pick(was, current, clicked)).toEqual(want);
  });
});

describe("between", () => {
  const keys = ["a", "b", "c", "d", "e"];
  it.each([
    ["down the list", "b", "d", ["b", "c", "d"]],
    ["up the list", "d", "b", ["b", "c", "d"]],
    ["one card", "c", "c", ["c"]],
    ["from a card no longer in the list", "x", "c", ["c"]],
    ["to a card no longer in the list", "b", "x", []],
  ] as const)("picks %s", (_, from, to, want) => {
    expect(between(keys, from, to)).toEqual(want);
  });
});

describe("joined", () => {
  it("reads the cards as one running text, card by card", () => {
    expect(joined(["Ich war sechs.", " Das weiß ich noch. ", "Ich war sechs."])).toBe("Ich war sechs. Das weiß ich noch. Ich war sechs.");
  });

  it("leaves out a card with no words", () => {
    expect(joined(["Ja.", " ", "Nein."])).toBe("Ja. Nein.");
  });

  it("copies nothing from no cards", () => {
    expect(joined([])).toBe("");
  });
});
