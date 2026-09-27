import { describe, expect, test } from "vitest";
import type { CloudModel, LanguageModel, Provider } from "./api";
import { cloudModelIn, cloudValue, finderOptions, finderStanding, holdsTheApp, providerOf } from "./finding";

const anthropic: Provider = { name: "anthropic", title: "Anthropic", env: "ANTHROPIC_API_KEY", keysAt: "console.anthropic.com" };
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
    ...over,
  };
}

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
    model("small", { title: "Small", needs: 10 * 2 ** 30 }),
    model("big", { title: "Big", recommended: true }),
    model("far", { title: "Far", installed: false, download: 5.2e9 }),
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

  test("the models here: those that are here first, the biggest first, each saying its cost", () => {
    const here = finderOptions(cloud, providers, language, "claude-sonnet-5").filter(
      (o) => o.group === "On this machine",
    );
    expect(here.map((o) => o.label)).toEqual(["Big", "Small", "Huge", "Far"]);
    expect(here.find((o) => o.label === "Big")?.detail).toBe("best here");
    expect(here.find((o) => o.label === "Far")?.detail).toBe("5.2 GB download");
    expect(here.find((o) => o.label === "Huge")).toMatchObject({ warn: true, detail: "Too big for this machine" });
  });
});

describe("how the choice stands", () => {
  const ready = model("gemma", { title: "Gemma", inUse: true, recommended: true });

  test("a model in the cloud without its company's key needs one", () => {
    const s = finderStanding({ planner: "api", provider: openai, hasKey: false });
    expect(s.state).toBe("warn");
    expect(s.text).toBe("By OpenAI. A few cents an episode.");
  });

  test("with its key it is ready", () => {
    expect(finderStanding({ planner: "api", provider: openai, hasKey: true }).state).toBe("ok");
  });

  test("a model here, chosen and not downloaded, says so and is not ready", () => {
    const s = finderStanding({
      planner: "local",
      provider: anthropic,
      hasKey: true,
      inUse: model("far", { installed: false, inUse: true, download: 9e9 }),
    });
    expect(s).toEqual({ text: "Not downloaded yet. 9.0 GB.", tone: "warn", state: "warn" });
  });

  test("while it downloads the line says how far and how long", () => {
    const far = model("far", { installed: false, inUse: true, download: 9e9 });
    const s = finderStanding({
      planner: "local",
      provider: anthropic,
      hasKey: true,
      inUse: far,
      fetching: far,
      progress: { fraction: 0.5, remaining: 130 },
    });
    expect(s.state).toBe("busy");
    expect(s.text).toBe("Downloading, 4.5 GB of 9.0 GB, 2:10 left");
  });

  test("a download that stopped says why", () => {
    const s = finderStanding({ planner: "local", provider: anthropic, hasKey: true, inUse: ready, failed: "The download stopped." });
    expect(s).toMatchObject({ tone: "err", state: "err" });
  });

  test("nothing chosen asks for a choice", () => {
    expect(finderStanding({ planner: "local", provider: anthropic, hasKey: true }).state).toBe("warn");
  });

  test("a model that is here is ready, and says who made it and what it needs", () => {
    const s = finderStanding({ planner: "local", provider: anthropic, hasKey: false, inUse: ready });
    expect(s).toEqual({ text: "By Google. 20 GB of memory. Best for this machine.", tone: "", state: "ok" });
  });

  test("without llama-server it is not ready, whatever the model", () => {
    expect(finderStanding({ planner: "local", provider: anthropic, hasKey: true, inUse: ready, serverMissing: true }).state).toBe("err");
  });

  test("every line is one short line", () => {
    const lines = [
      finderStanding({ planner: "api", provider: openai, hasKey: false }).text,
      finderStanding({ planner: "local", provider: anthropic, hasKey: true, inUse: ready }).text,
      finderStanding({
        planner: "local",
        provider: anthropic,
        hasKey: true,
        inUse: model("far", { installed: false, inUse: true }),
      }).text,
    ];
    for (const line of lines) expect(line.length).toBeLessThanOrEqual(60);
  });
});

describe("when the settings keep the app where it is", () => {
  const notHere = finderStanding({
    planner: "local",
    provider: anthropic,
    hasKey: true,
    inUse: model("far", { installed: false, inUse: true }),
  });

  test("a model chosen and not downloaded holds it, the trap Tim walked into", () => {
    expect(holdsTheApp(notHere.state, true)).toBe(true);
  });

  test("a model in the cloud without its key holds it", () => {
    expect(holdsTheApp(finderStanding({ planner: "api", provider: openai, hasKey: false }).state, true)).toBe(true);
  });

  test("a download on its way and a working choice let it go", () => {
    const far = model("far", { installed: false, inUse: true });
    const downloading = finderStanding({ planner: "local", provider: anthropic, hasKey: true, inUse: far, fetching: far });
    expect(holdsTheApp(downloading.state, true)).toBe(false);
    expect(holdsTheApp(finderStanding({ planner: "api", provider: openai, hasKey: true }).state, true)).toBe(false);
  });

  test("a page still reading never holds anybody", () => {
    expect(holdsTheApp(notHere.state, false)).toBe(false);
  });
});
