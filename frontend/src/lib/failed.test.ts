import { describe, expect, test } from "vitest";

import type { Job } from "./api";
import { Failures } from "./failed";

type J = Pick<Job, "id" | "state">;

function teller() {
  const said: string[] = [];
  return { said, failures: new Failures<J>((job) => said.push(job.id)) };
}

describe("a failed job says why", () => {
  test("once it was seen running, and only once", () => {
    const { said, failures } = teller();
    failures.seen({ id: "a", state: "queued" });
    failures.seen({ id: "a", state: "running" });
    failures.seen({ id: "a", state: "failed" });
    failures.seen({ id: "a", state: "failed" });
    expect(said).toEqual(["a"]);
  });

  test("not when it was only ever seen failed, as the app opened", () => {
    const { said, failures } = teller();
    failures.seen({ id: "a", state: "failed" });
    expect(said).toEqual([]);
  });

  test("a clip asked for here that failed in the store before its call answered", () => {
    const { said, failures } = teller();
    // The events came first, all at once, so the store was failed already.
    failures.seen({ id: "a", state: "failed" });
    failures.asked({ id: "a", state: "failed" });
    expect(said).toEqual(["a"]);
  });

  test("a clip whose call answers that it failed, the same way as one that fails later", () => {
    const { said, failures } = teller();
    failures.asked({ id: "a", state: "failed" });
    failures.seen({ id: "a", state: "failed" });
    failures.asked({ id: "b", state: "queued" });
    failures.seen({ id: "b", state: "failed" });
    expect(said).toEqual(["a", "b"]);
  });

  test("a failure seen running and told before the call answered is not told again", () => {
    const { said, failures } = teller();
    failures.seen({ id: "a", state: "running" });
    failures.seen({ id: "a", state: "failed" });
    failures.asked({ id: "a", state: "failed" });
    expect(said).toEqual(["a"]);
  });

  test("a job that ended any other way is forgotten, and says nothing", () => {
    const { said, failures } = teller();
    for (const state of ["done", "cancelled", "interrupted"] as const) {
      failures.seen({ id: state, state: "running" });
      failures.seen({ id: state, state });
      // Seen failed later, as a record read again, it was not seen running.
      failures.seen({ id: state, state: "failed" });
    }
    expect(said).toEqual([]);
  });
});
