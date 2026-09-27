// What finds the clips, as the settings put it: one list of every model,
// the ones in the cloud from each company and the ones that run here, and
// one line under it that says how the choice stands. The decisions are
// here rather than in the page, so the tests in finding.test.ts can walk
// every state somebody can click their way into: a model in the cloud
// with and without its company's key, a model here that is chosen and
// not downloaded, downloading, failed, or ready.
import { clock, fitNote, memorySize, size, type CloudModel, type LanguageModel, type Provider } from "./api";

// The company a model in the cloud belongs to. One the app offers is
// known by the list. One written in by hand is what the Go side said, and
// Anthropic before it has said anything, which is what every model was
// before there was a choice.
export function providerOf(
  model: string,
  cloud: CloudModel[],
  providers: Provider[],
  said = "",
): Provider {
  const name = cloud.find((m) => m.model === model)?.provider || said || "anthropic";
  return (
    providers.find((p) => p.name === name) ?? {
      name,
      title: name === "openai" ? "OpenAI" : "Anthropic",
      env: name === "openai" ? "OPENAI_API_KEY" : "ANTHROPIC_API_KEY",
      keysAt: name === "openai" ? "platform.openai.com" : "console.anthropic.com",
    }
  );
}

// A model in the cloud is picked as "cloud:" and its name, so it can never
// be taken for the file name of a model here.
export const cloudValue = (model: string) => `cloud:${model}`;
export const cloudModelIn = (value: string) =>
  value.startsWith("cloud:") ? value.slice("cloud:".length) : null;

export interface FinderOption {
  value: string;
  label: string;
  detail?: string;
  warn?: boolean;
  group: string;
}

// The list: every company's model with the company beside it, a model
// written in by hand as well so the list never names nothing, and then
// the models that run here, those that are here first and the biggest
// first, each with what it would cost to fetch or whether it fits.
export function finderOptions(
  cloud: CloudModel[],
  providers: Provider[],
  language: LanguageModel[],
  cloudModel: string,
): FinderOption[] {
  const offered = cloud.some((m) => m.model === cloudModel)
    ? cloud
    : [...cloud, { model: cloudModel, title: cloudModel, provider: providerOf(cloudModel, cloud, providers).name }];
  return [
    ...offered.map((m) => ({
      value: cloudValue(m.model),
      label: m.title,
      detail: providers.find((p) => p.name === m.provider)?.title ?? "",
      group: "In the cloud",
    })),
    ...[...language]
      .sort((a, b) => Number(b.installed) - Number(a.installed) || b.needs - a.needs)
      .map((m) => {
        const { note, warn } = fitNote(m.fit, m.recommended);
        const fetch = m.installed ? "" : `${size(m.download)} download`;
        return {
          value: m.name,
          label: m.title,
          detail: warn ? note : m.recommended ? [fetch, "best here"].filter(Boolean).join(", ") : fetch,
          warn,
          group: "On this machine",
        };
      }),
  ];
}

// Everything the line under the choice depends on.
export interface FinderFacts {
  planner: "api" | "local";
  // The company of the model in the cloud, and whether its key is here.
  provider: Provider;
  hasKey: boolean;
  // The model here that is chosen, if one is.
  inUse?: LanguageModel;
  // The model being downloaded, and how far it has come.
  fetching?: LanguageModel;
  progress?: { fraction: number; remaining: number };
  // Why the last download stopped.
  failed?: string;
  // What the check said about the model file and about llama-server.
  modelProblem?: string;
  serverMissing?: boolean;
}

export type Tone = "" | "warn" | "err";
export type Standing = "ok" | "busy" | "warn" | "err";

// The one line under Find clips with, and how the choice stands, which the
// mark before it and the frame round the list both show. The line is one
// line in every state, short enough to fit beside the list and a button.
export function finderStanding(f: FinderFacts): { text: string; tone: Tone; state: Standing } {
  if (f.planner === "api") {
    return {
      text: `By ${f.provider.title}. A few cents an episode.`,
      tone: "",
      state: f.hasKey ? "ok" : "warn",
    };
  }
  if (f.fetching) {
    const p = f.progress;
    const total = f.fetching.download;
    const done = p && p.fraction > 0 ? `${size(p.fraction * total)} of ${size(total)}` : size(total);
    const left = p && p.remaining > 0 ? `, ${clock(p.remaining)} left` : "";
    return { text: `Downloading, ${done}${left}`, tone: "", state: "busy" };
  }
  if (f.failed) return { text: f.failed, tone: "err", state: "err" };
  if (!f.inUse) return { text: "Choose a model to find clips with.", tone: "warn", state: "warn" };
  if (!f.inUse.installed) {
    return { text: `Not downloaded yet. ${size(f.inUse.download)}.`, tone: "warn", state: "warn" };
  }
  if (f.modelProblem) return { text: f.modelProblem, tone: "err", state: "err" };
  const { note } = fitNote(f.inUse.fit, f.inUse.recommended);
  return {
    text: `By ${f.inUse.maker}. ${memorySize(f.inUse.needs)} of memory.${note ? ` ${note}.` : ""}`,
    tone: "",
    state: f.serverMissing ? "err" : "ok",
  };
}

// Whether the settings keep the app where it is: while what finds the clips
// cannot find any, and never while a download is putting it right or the
// page is still reading what it has to show.
export function holdsTheApp(state: Standing, loaded: boolean): boolean {
  return loaded && (state === "warn" || state === "err");
}
