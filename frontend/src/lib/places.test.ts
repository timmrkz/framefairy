import { describe, expect, test } from "vitest";
import { Places, settle } from "./places";

// A playhead that stands at a moment long enough to rest there, the way
// the workspace tells Places about it, a tenth of a second at a time.
class Hand {
  places = new Places();
  now = 0;
  at = 0;
  clip = "a";
  playing = false;
  wait(ms: number) {
    for (let t = 0; t < ms; t += 100) {
      this.now += 100;
      if (this.playing) this.at += 0.1;
      this.places.see(this.at, this.clip, this.playing, this.now);
    }
  }
  rest(at: number, clip = this.clip) {
    this.at = at;
    this.clip = clip;
    this.wait(settle + 200);
  }
  back() {
    const to = this.places.back({ at: this.at, clip: this.clip }, this.now);
    if (to) [this.at, this.clip] = [to.at, to.clip];
    this.wait(200);
    return to;
  }
  forward() {
    const to = this.places.forward({ at: this.at, clip: this.clip }, this.now);
    if (to) [this.at, this.clip] = [to.at, to.clip];
    this.wait(200);
    return to;
  }
}

describe("Back and Forward go to where the playhead rested", () => {
  test("back through every place and forward again, clip and all", () => {
    const h = new Hand();
    h.rest(10, "a");
    h.rest(50, "b");
    h.rest(90, "c");
    expect(h.back()).toEqual({ at: 50, clip: "b" });
    expect(h.back()).toEqual({ at: 10, clip: "a" });
    expect(h.back()).toBeUndefined();
    expect(h.forward()).toEqual({ at: 50, clip: "b" });
    expect(h.forward()).toEqual({ at: 90, clip: "c" });
    expect(h.forward()).toBeUndefined();
  });

  test("a moment passed through on the way is not a place", () => {
    const h = new Hand();
    h.rest(10);
    // A click, and another before the first has settled.
    h.at = 30;
    h.wait(300);
    h.rest(70);
    expect(h.back()?.at).toBe(10);
  });

  test("a drag goes back to where it started, not to any moment of it", () => {
    const h = new Hand();
    h.rest(20);
    for (let i = 1; i <= 40; i++) {
      h.at = 20 + i * 0.25;
      h.wait(100);
    }
    h.wait(settle + 200);
    expect(h.back()?.at).toBe(20);
  });

  test("playing on from a place and stopping is two places", () => {
    const h = new Hand();
    h.rest(100);
    h.playing = true;
    h.wait(5000);
    h.playing = false;
    h.wait(settle + 200);
    expect(h.at).toBeCloseTo(105, 5);
    expect(h.back()?.at).toBe(100);
    expect(h.forward()?.at).toBeCloseTo(105, 5);
  });

  test("a small step, like a word walked to, stays the same place", () => {
    const h = new Hand();
    h.rest(10);
    h.rest(40);
    h.rest(40.4);
    expect(h.back()?.at).toBe(10);
    // And where it rested last is the place Forward comes back to.
    expect(h.forward()?.at).toBe(40.4);
  });

  test("going somewhere new after Back leaves nothing to go forward to", () => {
    const h = new Hand();
    h.rest(10);
    h.rest(50);
    h.back();
    h.rest(200);
    expect(h.forward()).toBeUndefined();
    expect(h.back()?.at).toBe(10);
  });

  test("the moments a jump of Back passes on its way are not the hand's", () => {
    const h = new Hand();
    h.rest(10);
    h.rest(50);
    const to = h.places.back({ at: h.at, clip: h.clip }, h.now);
    expect(to?.at).toBe(10);
    // The seek has not landed yet, and the playhead still reads 50.
    h.wait(400);
    h.at = 10;
    h.wait(settle + 200);
    expect(h.forward()?.at).toBe(50);
  });

  test("back from where the playhead stands, before it has settled, comes forward to it", () => {
    const h = new Hand();
    h.rest(10);
    h.rest(50);
    h.at = 80;
    h.wait(300);
    expect(h.back()?.at).toBe(50);
    expect(h.back()?.at).toBe(10);
    expect(h.forward()?.at).toBe(50);
    expect(h.forward()?.at).toBe(80);
  });
});
