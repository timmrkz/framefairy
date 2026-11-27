// Which frame of a video holds a moment, the frame the video preview shows
// with the playhead there, found by looking the moment up in where the
// engine says each frame holds from, and where it begins, see WriteTable
// in internal/framewire.
// The window counts no frame by a rule of its own: a moment kept to the
// millisecond a hair before a frame's start is in that frame, and only
// the engine says so.

export class FrameTable {
  constructor(
    // Where each frame of the video holds from, in order, the first from
    // the start of time.
    private readonly from: Float64Array,
    // Where each frame begins, the moment it stands for.
    private readonly at: Float64Array,
  ) {}

  // How many frames the video has.
  get count(): number {
    return this.from.length;
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

  // Where frame r begins, the first or the last frame outside the video.
  begins(r: number): number {
    return this.at[Math.max(0, Math.min(r, this.at.length - 1))];
  }

  // Where the frame that holds a moment begins, where a moment that
  // stands for a frame is drawn.
  startOf(at: number): number {
    return this.begins(this.holding(at));
  }

  // Where the frame beside the one that holds a moment begins, one each
  // way, where a step of the arrow keys goes.
  beside(at: number, back: boolean): number {
    return this.begins(this.holding(at) + (back ? -1 : 1));
  }
}

// The tables of the videos opened last, by their file, each asked of the
// Go side once: the frame queue and the workspace share one.
const tables = new Map<string, Promise<FrameTable>>();
const KEEP = 4;

// The frame table of a video, see FrameTable.
export function frameTable(path: string): Promise<FrameTable> {
  let got = tables.get(path);
  if (got) {
    tables.delete(path);
    tables.set(path, got);
    return got;
  }
  got = fetch(`/frames/table?${new URLSearchParams({ path })}`).then(async (r) => {
    if (!r.ok) throw new Error(`the frames of the video could not be read: ${r.status}`);
    const both = new Float64Array(await r.arrayBuffer());
    const n = both.length / 2;
    if (!n || !Number.isInteger(n)) throw new Error("the frames of the video came short");
    return new FrameTable(both.subarray(0, n), both.subarray(n));
  });
  got.catch(() => tables.delete(path));
  tables.set(path, got);
  while (tables.size > KEEP) tables.delete(tables.keys().next().value!);
  return got;
}
