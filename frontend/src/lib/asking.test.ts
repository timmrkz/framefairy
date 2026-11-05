import { describe, expect, test } from "vitest";
import { Asking } from "./asking";

// A call that answers when told to, and counts how often it was sent.
function sender() {
  const sent: string[] = [];
  const answers: ((v: string) => void)[] = [];
  const send = (what: string) => () => {
    sent.push(what);
    return new Promise<string>((done) => answers.push(done));
  };
  return { sent, answers, send };
}

describe("asking the Go side", () => {
  test("a read asked again while it is on its way is sent once and both hear the answer", async () => {
    const asking = new Asking(new Set(["Captions"]));
    const { sent, answers, send } = sender();
    const first = asking.ask("Captions", ["a.mp4", "plan", "1"], send("first"));
    const second = asking.ask("Captions", ["a.mp4", "plan", "1"], send("second"));
    expect(sent).toEqual(["first"]);
    answers[0]("the captions");
    expect(await first).toBe("the captions");
    expect(await second).toBe("the captions");
  });

  test("a read with other arguments is sent", () => {
    const asking = new Asking(new Set(["Captions"]));
    const { sent, send } = sender();
    void asking.ask("Captions", ["a.mp4", "plan", "1"], send("first"));
    void asking.ask("Captions", ["a.mp4", "plan", "2"], send("second"));
    expect(sent).toEqual(["first", "second"]);
  });

  test("a read asked after its answer came is sent again", async () => {
    const asking = new Asking(new Set(["Fonts"]));
    const { sent, answers, send } = sender();
    const first = asking.ask("Fonts", [], send("first"));
    answers[0]("fonts");
    await first;
    void asking.ask("Fonts", [], send("second"));
    expect(sent).toEqual(["first", "second"]);
  });

  test("a read asked after a failed one is sent again", async () => {
    const asking = new Asking(new Set(["Fonts"]));
    let tries = 0;
    const failing = () => {
      tries++;
      return Promise.reject(new Error("no"));
    };
    await expect(asking.ask("Fonts", [], failing)).rejects.toThrow("no");
    await expect(asking.ask("Fonts", [], failing)).rejects.toThrow("no");
    expect(tries).toBe(2);
  });

  test("any other call lets the reads on their way go, since it may change what they say", () => {
    const asking = new Asking(new Set(["GetSettings"]));
    const { sent, send } = sender();
    void asking.ask("GetSettings", [], send("read"));
    void asking.ask("UpdateSettings", [{ captionY: 0.7 }], send("write"));
    void asking.ask("GetSettings", [], send("read after the write"));
    expect(sent).toEqual(["read", "write", "read after the write"]);
  });

  test("news from the Go side lets the reads on their way go", () => {
    const asking = new Asking(new Set(["Sentences"]));
    const { sent, send } = sender();
    void asking.ask("Sentences", ["a.mp4"], send("before the news"));
    asking.forget();
    void asking.ask("Sentences", ["a.mp4"], send("after the news"));
    expect(sent).toEqual(["before the news", "after the news"]);
  });
});
