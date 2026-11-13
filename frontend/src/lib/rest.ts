// A wait for something to come to rest before the work it holds back:
// the hand that types or swipes, or a row that slides. What then runs is
// what would run a moment later all the same, so a walk that has
// settled, with nothing on its way and nothing moving, runs it at once
// rather than spending its time on the clock, see preview/settled.ts.
// It waits as setTimeout does, and clearTimeout calls it off.
export function afterRest(fn: () => void, ms: number): number {
  const run = () => fn();
  (run as unknown as { rest: boolean }).rest = true;
  return window.setTimeout(run, ms);
}
