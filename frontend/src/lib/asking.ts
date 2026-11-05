// A read the page asks for again while the same read is still on its way
// shares the answer on its way, instead of crossing to the Go side twice.
// Opening a video asked for the captions and the sentences of its clip
// twice each, as its clips came in, and on a machine that knew nothing
// yet the Go side measured the captions twice.
//
// Only a read with the same arguments is shared, and only while the same
// moment holds: any other call, which may change what a read says, and any
// news from the Go side, which says something changed, let every read on
// its way go, so a read asked after them is asked again.
export class Asking {
  private onTheWay = new Map<string, Promise<unknown>>();

  constructor(private reads: ReadonlySet<string>) {}

  // The answer to method with args, sent by send, or the one on its way.
  ask<T>(method: string, args: unknown[], send: () => Promise<T>): Promise<T> {
    if (!this.reads.has(method)) {
      this.forget();
      return send();
    }
    const key = `${method}\u0000${JSON.stringify(args)}`;
    const on = this.onTheWay.get(key);
    if (on) return on as Promise<T>;
    const p = send();
    this.onTheWay.set(key, p);
    const done = () => {
      if (this.onTheWay.get(key) === p) this.onTheWay.delete(key);
    };
    p.then(done, done);
    return p;
  }

  // Lets every read on its way go: the next is asked again.
  forget() {
    this.onTheWay.clear();
  }
}
