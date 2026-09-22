import { describe, expect, test } from "vitest";
import { fitNote, memorySize, size } from "./api";

// A download and an amount of memory are both a number of bytes and they
// are quoted two different ways, everywhere, by everybody. Getting that
// wrong does not break anything: it just makes the app say a number nobody
// recognises, which is worse, because it is believed.
describe("sizes", () => {
  test("a download counts in thousands, the way a download is always quoted", () => {
    expect(size(487_170_055)).toBe("487 MB");
    expect(size(671_239_000)).toBe("671 MB");
    expect(size(15_461_882_265)).toBe("15.5 GB");
    expect(size(1_000_000_000)).toBe("1.0 GB");
  });

  test("memory counts in 1024s, the way a machine is sold", () => {
    // The one that matters. This is a 32 GB Mac, and calling it 34.4 GB
    // would be true and unrecognisable.
    expect(memorySize(34_359_738_368)).toBe("32 GB");
    expect(memorySize(17_179_869_184)).toBe("16 GB");
    expect(memorySize(19_327_352_832)).toBe("18 GB");
    // Under ten it is worth a decimal, over it is not.
    expect(memorySize(8_589_934_592)).toBe("8 GB");
    expect(memorySize(6_442_450_944)).toBe("6 GB");
    expect(memorySize(1_610_612_736)).toBe("1.5 GB");
  });

  test("nothing is said about a size there is none of", () => {
    expect(size(0)).toBe("");
    expect(size(-1)).toBe("");
    expect(size(NaN)).toBe("");
    expect(memorySize(0)).toBe("");
    expect(memorySize(-1)).toBe("");
    expect(memorySize(NaN)).toBe("");
  });
});

// What a machine can do with a model is said in one place, so the setup
// and the settings never word the same fact two ways.
describe("what a machine can do with a model", () => {
  test("a fit is a fact and the other two are warnings", () => {
    expect(fitNote("fits")).toEqual({ note: "Fits this machine", warn: false });
    expect(fitNote("tight").warn).toBe(true);
    expect(fitNote("too big").warn).toBe(true);
  });

  test("a machine that will not say is not warned about", () => {
    expect(fitNote("unknown")).toEqual({ note: "", warn: false });
  });

  // The one this machine is offered says so. What it must never do is
  // swallow the warning with it: the best of them on a small machine is
  // still tight on it, and somebody about to spend a download on it
  // deserves to know that before they start.
  test("the one on offer says so, and still warns", () => {
    expect(fitNote("fits", true)).toEqual({ note: "Best for this machine", warn: false });
    expect(fitNote("tight", true).warn).toBe(true);
    expect(fitNote("tight", true).note).not.toBe(fitNote("tight").note);
    expect(fitNote("unknown", true).note).not.toBe("");
  });
});
