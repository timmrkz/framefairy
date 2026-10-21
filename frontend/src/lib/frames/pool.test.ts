import { afterEach, describe, expect, test, vi } from "vitest";
import { Picture } from "./picture";
import { frames, Pool } from "./pool";
import { pull } from "./pull";

describe("Pool", () => {
  test("gives back the buffer it was given, by its size", () => {
    const p = new Pool(1000);
    const a = new ArrayBuffer(100);
    p.give(a);
    expect(p.take(50)).toBeUndefined();
    expect(p.take(100)).toBe(a);
    expect(p.take(100)).toBeUndefined();
    expect(p.bytes).toBe(0);
  });

  test("keeps no more than its limit", () => {
    const p = new Pool(250);
    for (let i = 0; i < 4; i++) p.give(new ArrayBuffer(100));
    expect(p.bytes).toBe(200);
    expect(p.takeUpTo(100, 5)).toHaveLength(2);
  });

  test("lets go of another size first, since the app changed size", () => {
    const p = new Pool(250);
    p.give(new ArrayBuffer(100));
    p.give(new ArrayBuffer(100));
    p.give(new ArrayBuffer(120));
    expect(p.takeUpTo(100, 2)).toHaveLength(1);
    expect(p.take(120)).toBeDefined();
  });

  test("passes over a buffer handed to a Worker", () => {
    const p = new Pool(1000);
    const a = new ArrayBuffer(100);
    structuredClone(a, { transfer: [a] });
    p.give(a);
    expect(p.bytes).toBe(0);
  });
});

describe("Picture's bytes", () => {
  afterEach(() => {
    while (frames.take(16)) {
      // Empties the pool for the next test.
    }
  });

  test("go back to the pool once the last holder lets go", () => {
    const b = new ArrayBuffer(16);
    const one = new Picture(b, 2, 2, 0);
    const two = one.clone(40000);
    one.close();
    expect(one.data).toBeNull();
    expect(two.data?.buffer).toBe(b);
    expect(frames.take(16)).toBeUndefined();
    two.close();
    expect(frames.take(16)).toBe(b);
  });

  test("closed twice go back once", () => {
    const b = new ArrayBuffer(16);
    const one = new Picture(b, 2, 2, 0);
    const two = one.clone();
    one.close();
    one.close();
    expect(frames.take(16)).toBeUndefined();
    two.close();
    expect(frames.take(16)).toBe(b);
    expect(frames.take(16)).toBeUndefined();
  });

  test("someone else's never go to the pool", () => {
    new Picture(new Uint8Array(16), 2, 2, 0).close();
    expect(frames.take(16)).toBeUndefined();
  });
});

describe("pull", () => {
  afterEach(() => vi.unstubAllGlobals());

  // A reply of the Go side with frames of 2 by 2, each its time and its
  // bytes, in pieces of 5 bytes as a network hands them.
  function reply(times: number[], cut = 0) {
    const parts: number[] = [];
    for (const [i, t] of times.entries()) {
      parts.push(...new Uint8Array(new Float64Array([t]).buffer));
      parts.push(...new Array(16).fill(i + 1));
    }
    const all = new Uint8Array(parts.slice(0, parts.length - cut));
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        for (let at = 0; at < all.length; at += 5) c.enqueue(all.slice(at, at + 5));
        c.close();
      },
    });
    vi.stubGlobal("fetch", () => Promise.resolve(new Response(body)));
  }

  test("reads the frames into the buffers it is handed and gives back the rest", async () => {
    reply([0, 0.04]);
    const spare = [new ArrayBuffer(16), new ArrayBuffer(16), new ArrayBuffer(16)];
    const got = await pull("/frames/read", 2, 2, [...spare]);
    expect(got.frames.map((f) => f.at)).toEqual([0, 0.04]);
    for (const f of got.frames) expect(spare).toContain(f.data);
    expect(new Uint8Array(got.frames[1].data)).toEqual(new Uint8Array(16).fill(2));
    expect(got.spare).toHaveLength(1);
    expect(spare).toContain(got.spare[0]);
  });

  test("makes buffers of its own once the handed ones run out", async () => {
    reply([0, 0.04]);
    const one = new ArrayBuffer(16);
    const got = await pull("/frames/read", 2, 2, [one, new ArrayBuffer(8)]);
    expect(got.frames).toHaveLength(2);
    expect(got.frames.filter((f) => f.data === one)).toHaveLength(1);
    expect(got.spare).toEqual([]);
  });

  test("gives back the buffer of a frame cut short", async () => {
    reply([0, 0.04], 3);
    const spare = [new ArrayBuffer(16), new ArrayBuffer(16)];
    const got = await pull("/frames/read", 2, 2, [...spare]);
    expect(got.frames).toHaveLength(1);
    expect(got.spare).toHaveLength(1);
  });
});
