import { describe, expect, test } from "vitest";
import { pressedAlone } from "./keys";

describe("pressedAlone", () => {
  const key = { metaKey: false, ctrlKey: false, altKey: false, shiftKey: false, repeat: false };

  test.each([
    ["alone", key, false, true],
    ["with Cmd", { ...key, metaKey: true }, false, false],
    ["with Ctrl", { ...key, ctrlKey: true }, false, false],
    ["with Alt", { ...key, altKey: true }, false, false],
    ["held down, repeating", { ...key, repeat: true }, false, false],
    ["with shift", { ...key, shiftKey: true }, false, false],
    // A French keyboard gives its digits only with shift, issue #539.
    ["with shift, where shift is let through", { ...key, shiftKey: true }, true, true],
    ["with shift and Cmd, where shift is let through", { ...key, shiftKey: true, metaKey: true }, true, false],
    ["held down with shift, where shift is let through", { ...key, shiftKey: true, repeat: true }, true, false],
  ])("%s", (_, event, withShift, alone) => {
    expect(pressedAlone(event, withShift)).toBe(alone);
  });
});
