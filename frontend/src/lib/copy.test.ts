import { describe, expect, it } from "vitest";
import { between, joined } from "./copy";

describe("between", () => {
  const keys = ["a", "b", "c", "d", "e"];
  it.each([
    ["down the list", "b", "d", ["b", "c", "d"]],
    ["up the list", "d", "b", ["b", "c", "d"]],
    ["one card", "c", "c", ["c"]],
    ["with nothing to start from", null, "c", ["c"]],
    ["from a card no longer in the list", "x", "c", ["c"]],
    ["to a card not in the list", "b", "x", []],
  ] as const)("picks %s", (_, from, to, want) => {
    expect(between(keys, from, to)).toEqual(want);
  });
});

describe("joined", () => {
  it("reads the cards as one running text", () => {
    expect(joined([{ start: 1, text: "Ich war sechs." }, { start: 3, text: " Das weiß ich noch. " }])).toBe("Ich war sechs. Das weiß ich noch.");
  });

  it("copies words two clips over each other both have once", () => {
    expect(
      joined([
        { start: 1, text: "Ich war sechs." },
        { start: 1, text: "Ich war sechs." },
        { start: 3, text: "Ich war sechs." },
      ]),
    ).toBe("Ich war sechs. Ich war sechs.");
  });

  it("leaves out a card with no words", () => {
    expect(joined([{ start: 1, text: "Ja." }, { start: 2, text: " " }, { start: 3, text: "Nein." }])).toBe("Ja. Nein.");
  });

  it("copies nothing from no cards", () => {
    expect(joined([])).toBe("");
  });
});
