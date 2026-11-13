import { expect, test, vi } from "vitest";

import { said } from "./said";
import { toasts } from "./toast.svelte";

vi.mock("./said", () => ({ said: vi.fn() }));

// Every toast goes into the log marked as one, with its words as they
// show and the video it is about, which is how Settings finds the last
// toasts again once they are gone, see toastsIn in applog.go. Issue #421.
test("a toast is written to the log as a toast", () => {
  toasts.say("drop", "  The file notes.pdf is no video.  ", { video: "/videos/talk.mp4" });
  expect(said).toHaveBeenCalledWith("The file notes.pdf is no video.", {
    level: "warn",
    video: "/videos/talk.mp4",
    toast: true,
  });
});
