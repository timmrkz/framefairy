// What pick.mjs and walkmap.mjs pick for a change, on a checkout made for
// each test with a folder of walks of its own.
//
// On the real disk: the picker asks git what changed, and git reads a
// checkout.
import { execFileSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { afterEach, describe, expect, test } from "vitest";
import { pick, stepsOf } from "./pick.mjs";
import { pickByMap } from "./walkmap.mjs";

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

const changedTo = async (files, map = null) => {
  const { root, base } = checkout(folder);
  write(root, files);
  return pick(Object.keys(files), { base, map, root });
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

  test("a helper of common/ takes every run", async () => {
    const changed = folder[`${walks}/common/steps.mjs`].replace("shared() {}", "shared() { return 1; }");
    expect(await changedTo({ [`${walks}/common/steps.mjs`]: changed })).toBe("all");
  });

  test("a walk takes itself, the times take nothing and the bridge takes every run", async () => {
    expect(await changedTo({ [`${walks}/timeline.mjs`]: `import { walk } from "./walk.mjs";\nwalk();\n` })).toEqual(["script timeline.mjs"]);
    expect(await changedTo({ [`${walks}/walk-times.tsv`]: "  4\tsequences.mjs SEQUENCE=one clicks\n" })).toEqual([]);
    expect(await changedTo({ [`${walks}/bridge.mjs`]: "export const a = 2;\n" })).toBe("all");
  });

  test("the interface with no map takes every run", async () => {
    expect(await changedTo({ "frontend/src/lib/a.ts": "export const a = 1;\n" })).toBe("all");
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

describe("the map picks the runs that ran a line that changed", () => {
  const ts = ["export function a() {", "  return 1;", "}", "", "export function b() {", "  return 2;", "}", ""].join("\n");
  const go = ["package engine", "", "const limit = 3", "", "func A() int {", "\treturn limit", "}", ""].join("\n");
  const files = {
    "frontend/src/lib/a.ts": ts,
    "frontend/src/lib/App.svelte": "<script>\n  let x = 1;\n</script>\n\n<style>\n  p { color: red; }\n</style>\n",
    "frontend/src/app.css": ":root { --gap: 8px; }\n",
    "frontend/src/lib/worker.ts": "onmessage = () => {};\n",
    "engine/a.go": go,
    "engine/b.go": "package engine\n\nfunc B() int {\n\treturn 2\n}\n",
  };
  const mapAt = (base) => ({
    version: 1,
    commit: base,
    runs: ["sequence one", "sequence two", "script timeline.mjs"],
    sets: [[], [0], [1], [0, 2]],
    lines: {
      "frontend/src/lib/a.ts": [
        [1, 3, 1],
        [5, 7, 2],
      ],
      "frontend/src/lib/App.svelte": [[2, 2, 3]],
      "engine/a.go": [[5, 7, 1]],
      "engine/b.go": [[3, 5, 2]],
    },
    bundled: new Set(["frontend/src/lib/a.ts", "frontend/src/lib/App.svelte", "frontend/src/lib/worker.ts"]),
  });
  const after = (changed) => {
    const { root, base } = checkout(files);
    write(root, changed);
    const map = mapAt(base);
    return Object.fromEntries(Object.keys(changed).map((f) => [f, pickByMap(map, f, root)]));
  };

  test("a line of the interface, a line of its style, a style sheet and a file only a worker runs", () => {
    expect(
      after({
        "frontend/src/lib/a.ts": ts.replace("return 2", "return 3"),
        "frontend/src/lib/App.svelte": files["frontend/src/lib/App.svelte"].replace("red", "blue"),
        "frontend/src/app.css": ":root { --gap: 10px; }\n",
        "frontend/src/lib/worker.ts": "onmessage = () => 1;\n",
      }),
    ).toEqual({
      "frontend/src/lib/a.ts": ["sequence two"],
      "frontend/src/lib/App.svelte": ["sequence one", "script timeline.mjs"],
      "frontend/src/app.css": "all",
      "frontend/src/lib/worker.ts": "all",
    });
  });

  test("a line of Go, a declaration its package uses, a new file and the bridge", () => {
    expect(
      after({
        "engine/b.go": "package engine\n\nfunc B() int {\n\treturn 3\n}\n",
        "engine/a.go": go.replace("limit = 3", "limit = 4"),
        "engine/c.go": "package engine\n\nfunc C() {}\n",
        "cmd/framefairy-app/bridge_test.go": "package main\n",
        "engine/a_test.go": "package engine\n",
      }),
    ).toEqual({
      "engine/b.go": ["sequence two"],
      "engine/a.go": ["sequence one", "sequence two"],
      "engine/c.go": ["sequence one", "sequence two"],
      "cmd/framefairy-app/bridge_test.go": "all",
      "engine/a_test.go": [],
    });
  });

  test("a map recorded on a commit that is not here takes every run", () => {
    const { root } = checkout(files);
    write(root, { "frontend/src/lib/a.ts": ts.replace("return 2", "return 3") });
    expect(pickByMap(mapAt("0".repeat(40)), "frontend/src/lib/a.ts", root)).toBe("all");
  });
});
