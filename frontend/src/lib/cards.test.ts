import { describe, expect, test } from "vitest";
import { captionsAlone, piecesOf } from "./cards";

// With the transcript search open, every save of an edge read the whole
// transcript twice, once for the clip's pieces and once more for its
// captions that came after, issue #895.
describe("the cards of what was said", () => {
  const clip = { start: 12, end: 31.5, segments: [] };
  const cut = { start: 12, end: 31.5, segments: [{ start: 12, end: 20.125 }, { start: 22, end: 31.5 }] };

  test("are read for the pieces a clip keeps, or the clip where it has no cuts", () => {
    expect(piecesOf(clip)).toBe("12.00-31.50");
    expect(piecesOf(cut)).toBe("12.00-20.13 22.00-31.50");
  });

  test("are not read again for captions that changed with the pieces", () => {
    expect(captionsAlone({ words: "1.00 Mein Arm", pieces: "12.00-31.50" }, { words: "1.00 Mein Arm ist", pieces: "12.00-33.00" })).toBe(false);
  });

  test("are read again for captions that changed while the pieces stayed", () => {
    expect(captionsAlone({ words: "1.00 Mein Arm", pieces: "12.00-31.50" }, { words: "1.00 Mein Bein", pieces: "12.00-31.50" })).toBe(true);
    expect(captionsAlone({ words: "1.00 Mein Arm", pieces: "12.00-31.50" }, { words: "1.20 Mein Arm", pieces: "12.00-31.50" })).toBe(true);
  });

  test("are not read again for captions that say what they said", () => {
    expect(captionsAlone({ words: "1.00 Mein Arm", pieces: "12.00-31.50" }, { words: "1.00 Mein Arm", pieces: "12.00-31.50" })).toBe(false);
  });

  test("are not read again for the first captions of a clip", () => {
    expect(captionsAlone(undefined, { words: "1.00 Mein Arm", pieces: "12.00-31.50" })).toBe(false);
  });
});
