// Which frame of a video holds a moment, the frame the video preview shows
// with the playhead there, found by looking the moment up in where the
// engine says each frame holds from, see Grid.From in internal/framewire.
// The window counts no frame by a rule of its own: a moment kept to the
// millisecond a hair before a frame's start is in that frame, and only
// the engine says so.

import type { Grid } from "./movie";

export class FrameTable {
  constructor(
    readonly grid: Grid,
    // Where each frame of the grid holds from, in order, the first from
    // the start of time.
    private readonly from: Float64Array,
  ) {}

  // How long a frame lasts, in seconds.
  get length(): number {
    return this.grid.den / this.grid.num;
  }

  // The frame that holds a moment: the last whose hold has begun by it.
  holding(at: number): number {
    let lo = 0;
    let hi = this.from.length - 1;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (this.from[mid] <= at) lo = mid;
      else hi = mid - 1;
    }
    return lo;
  }

  // Where frame r begins, one every den/num seconds from the grid's start,
  // as the engine names the frames, see Grid.At.
  begins(r: number): number {
    const k = Math.max(0, Math.min(r, this.grid.count - 1));
    return this.grid.start + (k * this.grid.den) / this.grid.num;
  }

  // Where the frame that holds a moment begins, where a moment that
  // stands for a frame is drawn.
  startOf(at: number): number {
    return this.begins(this.holding(at));
  }
}

// The tables of the videos opened last, by their file and grid, each
// asked of the Go side once: the frame queue and the workspace share one.
const tables = new Map<string, Promise<FrameTable>>();
const KEEP = 4;

// The frame table of a video, see FrameTable.
export function frameTable(path: string, grid: Grid): Promise<FrameTable> {
  const text = [grid.origin, grid.start, grid.num, grid.den, grid.count].join(",");
  const key = `${path}\n${text}`;
  let got = tables.get(key);
  if (got) {
    tables.delete(key);
    tables.set(key, got);
    return got;
  }
  got = fetch(`/frames/froms?${new URLSearchParams({ path, grid: text })}`).then(async (r) => {
    if (!r.ok) throw new Error(`the frames of the video could not be read: ${r.status}`);
    const from = new Float64Array(await r.arrayBuffer());
    if (from.length !== grid.count) throw new Error("the frames of the video came short");
    return new FrameTable(grid, from);
  });
  got.catch(() => tables.delete(key));
  tables.set(key, got);
  while (tables.size > KEEP) tables.delete(tables.keys().next().value!);
  return got;
}
