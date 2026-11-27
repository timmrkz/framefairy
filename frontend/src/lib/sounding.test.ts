import { describe, expect, it } from "vitest";
import { Sounding } from "./sounding";

// A line's style, all the class touches.
function line() {
  const props = new Map<string, string>();
  const style = {
    setProperty: (k: string, v: string) => props.set(k, v),
    removeProperty: (k: string) => props.delete(k),
  };
  return { el: { style } as unknown as HTMLElement, props };
}

describe("Sounding", () => {
  it("writes where the sound is on every line, and takes it off once the play stops", () => {
    const s = new Sounding();
    const a = line();
    const b = line();
    s.line(a.el);
    s.set(1.5);
    s.line(b.el);
    expect(a.props.get("--sound")).toBe("1.5");
    expect(b.props.get("--sound")).toBe("1.5");
    s.set(NaN);
    expect(a.props.has("--sound")).toBe(false);
    expect(b.props.has("--sound")).toBe(false);
  });

  it("leaves a line alone once it is gone", () => {
    const s = new Sounding();
    const a = line();
    const gone = s.line(a.el);
    s.set(2);
    gone();
    expect(a.props.has("--sound")).toBe(false);
    s.set(3);
    expect(a.props.has("--sound")).toBe(false);
  });
});
