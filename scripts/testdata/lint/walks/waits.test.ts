// Every way of waiting on the clock that ESLint refuses in a test.
import { setTimeout as sleep } from "node:timers/promises";

export async function waits(): Promise<number> {
  await sleep(10);
  await new Promise<void>((done) => setInterval(done, 10));
  await new Promise<void>((done) => window.setTimeout(done, 10));
  return performance.now();
}
