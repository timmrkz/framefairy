import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, test } from "vitest";
import { keep, report } from "./counts.mjs";

// One page's counts the way the walks keep them, counted in
// walks/bridge.mjs: a script of the build as Playwright's
// stopJSCoverage gave it, with its source map, the sources named from
// the top of the repository. Of the component's three lines the first
// and the last were run, and the second, the function b, was not.
const source = "function a(){return 1}\nfunction b(){return 2}\na();\n";
function page(component) {
  return [
    {
      url: "http://bridge/assets/index.js",
      scriptId: "4",
      source,
      functions: [
        { functionName: "", isBlockCoverage: true, ranges: [{ startOffset: 0, endOffset: source.length, count: 1 }] },
        { functionName: "a", isBlockCoverage: true, ranges: [{ startOffset: 0, endOffset: 22, count: 1 }] },
        { functionName: "b", isBlockCoverage: false, ranges: [{ startOffset: 23, endOffset: 45, count: 0 }] },
      ],
      sourceMap: { version: 3, sources: [component], sourcesContent: [source], names: [], mappings: "AAAA;AACA;AACA" },
    },
  ];
}

// On the real disk: monocart-coverage-reports keeps the counts in a
// folder and writes the report as a file, which is what is tested.
let folder = "";
function fresh() {
  folder = mkdtempSync(join(tmpdir(), "framefairy-counts-"));
  return folder;
}
afterEach(() => {
  if (folder) rmSync(folder, { recursive: true, force: true });
  folder = "";
});

describe("the walks' coverage of the interface", () => {
  // The nightly is the only run that counts, so a new version of
  // monocart-coverage-reports, like the major one Dependabot brought in
  // #619, was checked by nothing on its pull request.
  test("is kept page by page and reported as lcov of frontend/src, line by line", async () => {
    const top = fresh();
    const kept = join(top, "kept");
    await keep(kept, page("frontend/src/components/Card.svelte"));
    const out = join(top, "walks.lcov");
    expect(await report(kept, out)).toBe(1);
    const lcov = readFileSync(out, "utf8");
    expect(lcov).toContain("SF:frontend/src/components/Card.svelte\n");
    expect(lcov.match(/^DA:.*$/gm)).toEqual(["DA:1,1", "DA:2,0", "DA:3,1"]);
    expect(lcov).toContain("LH:2\n");
  });

  test("leaves out what the bundle carries from outside frontend/src", async () => {
    const top = fresh();
    const kept = join(top, "kept");
    await keep(kept, page("frontend/node_modules/bits-ui/dist/Menu.svelte"));
    await expect(report(kept, join(top, "walks.lcov"))).rejects.toThrow("counts no line of any component");
  });

  test("fails when the walks kept nothing", async () => {
    const top = fresh();
    await expect(report(join(top, "kept"), join(top, "walks.lcov"))).rejects.toThrow("the walks kept no counts");
  });
});
