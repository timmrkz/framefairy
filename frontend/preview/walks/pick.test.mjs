// What pick.mjs picks for a change, on a checkout made for
// each test with a folder of walks of its own.
//
// On the real disk: the picker asks git what changed, and git reads a
// checkout.
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, expect, test } from "vitest";
import { pick, stepsOf } from "./pick.mjs";

const walks = "frontend/preview/walks";
let roots = [];
afterEach(() => {
  for (const r of roots) rmSync(r, { recursive: true, force: true });
  roots = [];
});

const git = (root, ...args) => execFileSync("git", args, { cwd: root, encoding: "utf8" });

// A checkout with files in it, committed, and the commit it is at.
function checkout(files) {
  const root = mkdtempSync(join(tmpdir(), "pick-test-"));
  roots.push(root);
  git(root, "init", "-q");
  git(root, "config", "user.email", "walks@example.com");
  git(root, "config", "user.name", "walks");
  write(root, files);
  git(root, "add", "-A");
  git(root, "commit", "-q", "-m", "walks");
  return { root, base: git(root, "rev-parse", "HEAD").trim() };
}

function write(root, files) {
  for (const [file, text] of Object.entries(files)) {
    mkdirSync(dirname(join(root, file)), { recursive: true });
    writeFileSync(join(root, file), text);
  }
}

const sequences = (list) => `// The sequences.\nexport default ${JSON.stringify(list, null, 2)};\n`;

const steps = `// The steps.
import { a } from "../bridge.mjs";

export const steps = {
  // ["click", what]
  //                    clicks it
  async click(walk, arg) {
    await a(walk, arg);
  },
  // ["see", what]
  //                    sees it
  async "see"(walk, arg) {
    const seen = await a(walk, arg);
    if (!seen) walk.wrong = "not seen";
  },
};
steps["look"] = steps["see"];

function helper() {
  return 1;
}
`;

const folder = {
  [`${walks}/clips/sequences.mjs`]: sequences([
    { name: "one clicks", steps: [["click", "a"]] },
    { name: "one sees", steps: [["see", "b"]] },
    { name: "one looks", steps: [["look", "c"]] },
    { name: "one waits", steps: [["wait"]] },
  ]),
  [`${walks}/clips/steps.mjs`]: steps,
  [`${walks}/common/steps.mjs`]: `export const steps = {\n  // ["wait"]\n  async wait() {},\n};\n\nexport function shared() {}\n`,
  [`${walks}/timeline.mjs`]: `import { walk } from "./walk.mjs";\n`,
  [`${walks}/walk-times.tsv`]: "  3\tsequences.mjs SEQUENCE=one clicks\n",
  [`${walks}/bridge.mjs`]: "export const a = 1;\n",
};

const changedTo = async (files) => {
  const { root, base } = checkout(folder);
  write(root, files);
  return pick(Object.keys(files), { base, root });
};

describe("a walk file picks the runs it changes", () => {
  test("a sequence whose steps changed, and not one that gained a comment", async () => {
    const changed = sequences([
      { name: "one clicks", steps: [["click", "a"], ["click", "b"]] },
      { name: "one sees", steps: [["see", "b"]] },
      { name: "one looks", steps: [["look", "c"]] },
      { name: "one waits", steps: [["wait"]] },
      { name: "one is new", steps: [["see", "d"]] },
    ]).replace("// The sequences.", "// The sequences, said again.");
    expect(await changedTo({ [`${walks}/clips/sequences.mjs`]: changed })).toEqual(["sequence one clicks", "sequence one is new"]);
  });

  test("a step whose body changed, under each name it has", async () => {
    const changed = steps.replace('walk.wrong = "not seen"', 'walk.wrong = "nothing seen"');
    expect(await changedTo({ [`${walks}/clips/steps.mjs`]: changed })).toEqual(["sequence one looks", "sequence one sees"]);
  });

  test("the comment over a step is the step's", async () => {
    const changed = steps.replace("clicks it", "clicks on it");
    expect(await changedTo({ [`${walks}/clips/steps.mjs`]: changed })).toEqual(["sequence one clicks"]);
  });

  test("a helper of a folder's steps takes every sequence of those steps", async () => {
    const changed = steps.replace("return 1", "return 2");
    expect(await changedTo({ [`${walks}/clips/steps.mjs`]: changed })).toEqual(["sequence one clicks", "sequence one looks", "sequence one sees"]);
  });

  test("a helper another folder imports takes every run", async () => {
    const { root, base } = checkout({
      ...folder,
      [`${walks}/videos/sequences.mjs`]: sequences([{ name: "two adds", steps: [["add"]] }]),
      [`${walks}/videos/steps.mjs`]: `import { helper } from "../clips/steps.mjs";\nexport const steps = {\n  // ["add"]\n  async add() {\n    helper();\n  },\n};\n`,
    });
    write(root, { [`${walks}/clips/steps.mjs`]: steps.replace("return 1", "return 2") });
    expect(await pick([`${walks}/clips/steps.mjs`], { base, root })).toBe("all");
  });

  test("a helper of common/ takes every run", async () => {
    const changed = folder[`${walks}/common/steps.mjs`].replace("shared() {}", "shared() { return 1; }");
    expect(await changedTo({ [`${walks}/common/steps.mjs`]: changed })).toBe("all");
  });

  test("a walk takes itself, the times and a test of the harness take nothing and the bridge takes every run", async () => {
    expect(await changedTo({ [`${walks}/timeline.mjs`]: `import { walk } from "./walk.mjs";\nwalk();\n` })).toEqual(["script timeline.mjs"]);
    expect(await changedTo({ [`${walks}/pick.test.mjs`]: `const s = 'import { walk } from "./walk.mjs";';\n` })).toEqual([]);
    expect(await changedTo({ [`${walks}/walk-times.tsv`]: "  4\tsequences.mjs SEQUENCE=one clicks\n" })).toEqual([]);
    expect(await changedTo({ [`${walks}/bridge.mjs`]: "export const a = 2;\n" })).toBe("all");
  });

  test("the interface and the Go side take every run, and what picks the walks takes none", async () => {
    expect(await changedTo({ "frontend/src/lib/a.ts": "export const a = 1;\n" })).toBe("all");
    expect(await changedTo({ "engine/a.go": "package engine\n" })).toBe("all");
    expect(await changedTo({ [`${walks}/pick.mjs`]: "export const a = 1;\n" })).toEqual([]);
  });
});

test("the steps of a file are found from the comment over each to its end", () => {
  const { spans, aliases } = stepsOf(steps);
  expect(spans.map((s) => [s.name, s.from, s.to])).toEqual([
    ["click", 5, 9],
    ["see", 10, 15],
  ]);
  expect(aliases).toEqual([{ name: "look", of: "see", line: 17 }]);
});

// The picker finds a step by the shape it is written in, so every step
// there is has to be found that way, or a change to it would walk none
// of the sequences that take it.
test("every step of the walks is found where the picker looks for it", async () => {
  const here = import.meta.dirname;
  const dirs = readdirSync(here).filter((d) => existsSync(join(here, d, "steps.mjs")));
  expect(dirs.length).toBeGreaterThan(1);
  for (const d of dirs) {
    const file = join(here, d, "steps.mjs");
    const { steps: real } = await import(file);
    const { spans, aliases } = stepsOf(readFileSync(file, "utf8"));
    const found = [...spans.map((x) => x.name), ...aliases.map((x) => x.name)].sort();
    expect(found, d).toEqual(Object.keys(real).sort());
  }
});
