import { describe, expect, test } from "vitest";
import type { CloudModel, LanguageModel, Provider, SpeechModel, Standing } from "./api";
import {
  cloudModelIn,
  cloudValue,
  finderOptions,
  finderStanding,
  holdsTheApp,
  providerOf,
  speechOptions,
  speechStanding,
} from "./finding";

const anthropic: Provider = { name: "anthropic", title: "Anthropic", env: "ANTHROPIC_API_KEY", keysAt: "platform.claude.com" };
const openai: Provider = { name: "openai", title: "OpenAI", env: "OPENAI_API_KEY", keysAt: "platform.openai.com" };
const providers = [anthropic, openai];
const cloud: CloudModel[] = [
  { model: "claude-sonnet-5", title: "Claude Sonnet 5", provider: "anthropic" },
  { model: "gpt-6-sol", title: "GPT-6 Sol", provider: "openai" },
];

function model(name: string, over: Partial<LanguageModel> = {}): LanguageModel {
  return {
    name,
    title: name,
    maker: "Google",
    about: "",
    download: 9e9,
    needs: 20 * 2 ** 30,
    url: "",
    installed: true,
    fit: "fits",
    recommended: false,
    inUse: false,
    standing: { state: "ok", says: "ready" },
    ...over,
  };
}

// How a choice stands at rest, as the Go side judges it. Which choice
// stands how is tested there, in standing_test.go, and here only how
// the settings word it and lay the work in hand over it.
const cloudOK: Standing = { state: "ok", says: "cloud" };
const cloudNoKey: Standing = { state: "warn", says: "cloud" };
const fetch: Standing = { state: "warn", says: "fetch" };
const ready: Standing = { state: "ok", says: "ready" };

describe("which company a model in the cloud belongs to", () => {
  test("a model the app offers is known by the list", () => {
    expect(providerOf("gpt-6-sol", cloud, providers).name).toBe("openai");
    expect(providerOf("claude-sonnet-5", cloud, providers).name).toBe("anthropic");
  });

  test("one written in by hand is what the Go side said, and Anthropic before it says", () => {
    expect(providerOf("gpt-6-luna", cloud, providers, "openai").name).toBe("openai");
    expect(providerOf("something", cloud, providers).name).toBe("anthropic");
  });

  test("a company missing from the list still has a name and a place for keys", () => {
    const p = providerOf("gpt-6-sol", cloud, []);
    expect(p.title).toBe("OpenAI");
    expect(p.keysAt).toBe("platform.openai.com");
  });

  test("a choice in the cloud is never taken for a file here", () => {
    expect(cloudModelIn(cloudValue("gpt-6-sol"))).toBe("gpt-6-sol");
    expect(cloudModelIn("gemma-4-26B_q4_0-it.gguf")).toBeNull();
  });
});

describe("the list to choose from", () => {
  const language = [
    model("small", { title: "Small", needs: 10 * 2 ** 30, download: 7.0e9 }),
    model("big", { title: "Big", recommended: true, download: 14.4e9 }),
    model("far", { title: "Far", installed: false, download: 5.2e9 }),
    // Needs the most memory and is not the biggest download, the way
    // Qwen3 14B against Gemma 4 26B A4B is.
    model("huge", { title: "Huge", installed: false, fit: "too big", needs: 40 * 2 ** 30 }),
  ];

  test("every company's model is offered, with the company beside it", () => {
    const options = finderOptions(cloud, providers, language, "claude-sonnet-5");
    const inCloud = options.filter((o) => o.group === "In the cloud");
    expect(inCloud.map((o) => [o.label, o.detail])).toEqual([
      ["Claude Sonnet 5", "Anthropic"],
      ["GPT-6 Sol", "OpenAI"],
    ]);
  });

  test("a model written in by hand is in the list too, so it never names nothing", () => {
    const options = finderOptions(cloud, providers, language, "gpt-6-luna");
    expect(options.find((o) => o.value === cloudValue("gpt-6-luna"))).toBeTruthy();
  });

  // Tim saw a size on the models still to fetch, none on the ones here,
  // and "best here" on one, 2026-10-27. Then a group of their own for the
  // models to download, 2026-10-28, where he wanted one list with a mark.
  test("every model here in one group, the biggest first, each saying its size and whether it is downloaded", () => {
    const here = finderOptions(cloud, providers, language, "claude-sonnet-5").filter(
      (o) => o.group !== "In the cloud",
    );
    // By the size the row shows, not the memory the model needs: Tim saw
    // 9.0 GB over 14.4 GB as out of order, 2026-10-28.
    expect(here.map((o) => [o.group, o.label, o.detail, o.mark?.icon])).toEqual([
      ["On this machine", "Big", "14.4 GB", "check"],
      ["On this machine", "Huge", "9.0 GB, too big for this machine", "download"],
      ["On this machine", "Small", "7.0 GB", "check"],
      ["On this machine", "Far", "5.2 GB", "download"],
    ]);
    expect(here.find((o) => o.label === "Huge")?.warn).toBe(true);
    expect(here.filter((o) => o.mark?.faint).map((o) => o.label)).toEqual(["Huge", "Far"]);
    expect(here.some((o) => /best/i.test(o.detail ?? ""))).toBe(false);
  });
});

describe("how the choice stands", () => {
  const gemma = model("gemma", { title: "Gemma", inUse: true, recommended: true });
  const far = model("far", { installed: false, inUse: true, download: 9e9, standing: fetch });

  test("a model in the cloud is said by its company, in the state the Go side judged", () => {
    expect(finderStanding({ rest: cloudNoKey, provider: openai })).toEqual({
      text: "By OpenAI. A few cents a video.",
      tone: "warn",
      state: "warn",
    });
    expect(finderStanding({ rest: cloudOK, provider: openai }).state).toBe("ok");
  });

  test("a model here not downloaded says its size", () => {
    expect(finderStanding({ rest: fetch, provider: anthropic, inUse: far })).toEqual({
      text: "Not downloaded yet. 9.0 GB.",
      tone: "warn",
      state: "warn",
    });
  });

  test("a model here that is ready says who made it and what it needs", () => {
    expect(finderStanding({ rest: ready, provider: anthropic, inUse: gemma })).toEqual({
      text: "By Google. 20 GB of memory.",
      tone: "",
      state: "ok",
    });
  });

  test("one that cannot run is said the same way, in red", () => {
    const s = finderStanding({ rest: { state: "err", says: "ready" }, provider: anthropic, inUse: gemma });
    expect(s).toEqual({ text: "By Google. 20 GB of memory.", tone: "err", state: "err" });
  });

  test("a problem the Go side found is said in its words", () => {
    const rest: Standing = { state: "err", says: "problem", detail: "/models/gone.gguf" };
    expect(finderStanding({ rest, provider: anthropic, inUse: gemma })).toEqual({
      text: "/models/gone.gguf",
      tone: "err",
      state: "err",
    });
  });

  test("while it downloads the line says how far and how long", () => {
    const s = finderStanding({
      rest: fetch,
      provider: anthropic,
      inUse: far,
      fetching: far,
      progress: { fraction: 0.5, remaining: 130 },
    });
    expect(s.state).toBe("busy");
    expect(s.text).toBe("Downloading, 4.5 GB of 9.0 GB, 2:10 left");
  });

  test("a download that stopped says why", () => {
    const s = finderStanding({ rest: ready, provider: anthropic, inUse: gemma, failed: "The download stopped." });
    expect(s).toEqual({ text: "The download stopped.", tone: "err", state: "err" });
  });

  test("nothing chosen asks for a choice", () => {
    expect(finderStanding({ provider: anthropic })).toEqual({
      text: "Choose a model to find clips with.",
      tone: "warn",
      state: "warn",
    });
  });

  test("while a search runs, a choice that works says it is in use and holds nothing", () => {
    const s = finderStanding({ rest: ready, provider: anthropic, inUse: gemma, searching: true });
    expect(s).toEqual({ text: "A search is finding clips with it now.", tone: "", state: "ok", using: true });
    expect(holdsTheApp(s.state, true)).toBe(false);
    expect(finderStanding({ rest: cloudOK, provider: openai, searching: true }).using).toBe(true);
  });

  test("while a search runs, a choice that cannot work still says why", () => {
    const s = finderStanding({ rest: cloudNoKey, provider: openai, searching: true });
    expect(s.state).toBe("warn");
    expect(s.using).toBeUndefined();
  });

  test("every line is one short line", () => {
    const lines = [
      finderStanding({ rest: cloudNoKey, provider: openai }).text,
      finderStanding({ rest: ready, provider: anthropic, inUse: gemma }).text,
      finderStanding({ rest: fetch, provider: anthropic, inUse: far }).text,
      finderStanding({ rest: ready, provider: anthropic, inUse: gemma, searching: true }).text,
    ];
    for (const line of lines) expect(line.length).toBeLessThanOrEqual(60);
  });
});

describe("the line under the speech model", () => {
  const parakeet: SpeechModel = {
    name: "parakeet",
    title: "Parakeet",
    about: "",
    maker: "NVIDIA",
    languages: "25 European languages",
    download: 487e6,
    unpacked: 671e6,
    url: "",
    recommended: true,
    installed: true,
    inUse: true,
    standing: { state: "ok", says: "ready" },
  };
  const idle = { starting: false, failed: "" };
  const shown = (path: string) => path.replace("~", "/Users/tim");

  test("a model that is here says who made it and what it hears", () => {
    expect(speechStanding(parakeet, ready, idle, shown)).toEqual({
      text: "By NVIDIA. 25 European languages.",
      tone: "",
      state: "ok",
    });
  });

  test("a folder named by hand is said, written out in full", () => {
    const rest: Standing = { state: "ok", says: "folder", detail: "~/speech" };
    expect(speechStanding(parakeet, rest, idle, shown).text).toBe("From /Users/tim/speech.");
  });

  test("what the check found wrong is said in its words", () => {
    const rest: Standing = { state: "err", says: "problem", detail: "not found in /speech" };
    expect(speechStanding(parakeet, rest, idle, shown)).toEqual({ text: "not found in /speech", tone: "err", state: "err" });
  });

  test("a download on its way, then one that stopped, come before how it stands", () => {
    const running = { progress: { fraction: 0.5, remaining: 10 } };
    expect(speechStanding(parakeet, fetch, { running, starting: false, failed: "" }, shown).state).toBe("busy");
    expect(speechStanding(parakeet, fetch, { starting: false, failed: "No room left." }, shown)).toEqual({
      text: "No room left.",
      tone: "err",
      state: "err",
    });
  });

  test("with no speech model offered, it says so", () => {
    expect(speechStanding(undefined, ready, idle, shown).state).toBe("err");
  });
});

describe("when the settings keep the app where it is", () => {
  const notHere = finderStanding({ rest: fetch, provider: anthropic, inUse: model("far", { installed: false, inUse: true }) });

  test("a model chosen and not downloaded holds it, the trap Tim walked into", () => {
    expect(holdsTheApp(notHere.state, true)).toBe(true);
  });

  test("a model in the cloud without its key holds it", () => {
    expect(holdsTheApp(finderStanding({ rest: cloudNoKey, provider: openai }).state, true)).toBe(true);
  });

  test("a download on its way holds it too, since the model is no use until it has arrived", () => {
    const far = model("far", { installed: false, inUse: true });
    const downloading = finderStanding({ rest: fetch, provider: anthropic, inUse: far, fetching: far });
    expect(downloading.state).toBe("busy");
    expect(holdsTheApp(downloading.state, true)).toBe(true);
  });

  test("a working choice lets it go", () => {
    expect(holdsTheApp(finderStanding({ rest: cloudOK, provider: openai }).state, true)).toBe(false);
  });

  test("a page still reading never holds anybody", () => {
    expect(holdsTheApp(notHere.state, false)).toBe(false);
  });
});

// The speech model's list looks and works like the clip model's, Tim
// 2026-10-27: one group, the biggest first by the size it shows, the size
// unpacked once a model is here and the download until then.
describe("the speech models to choose from", () => {
  const speech = (name: string, download: number, unpacked: number, installed: boolean): SpeechModel => ({
    name,
    title: name,
    about: "",
    maker: "",
    languages: "",
    download,
    unpacked,
    url: "",
    recommended: false,
    installed,
    inUse: false,
    standing: installed ? { state: "ok", says: "ready" } : { state: "warn", says: "fetch" },
  });

  test("every one in one group, the biggest first by the size it shows, with its mark", () => {
    const list = speechOptions([
      speech("here-small", 9_000_000_000, 700_000_000, true),
      speech("to-fetch", 640_000_000, 2_000_000_000, false),
      speech("here-big", 100_000_000, 1_200_000_000, true),
    ]);
    expect(list.map((o) => [o.group, o.label, o.detail, o.mark?.icon])).toEqual([
      ["On this machine", "here-big", "1.2 GB", "check"],
      ["On this machine", "here-small", "700 MB", "check"],
      ["On this machine", "to-fetch", "640 MB", "download"],
    ]);
  });
});
