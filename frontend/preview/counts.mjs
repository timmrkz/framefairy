// The counts of the interface's lines the walks keep, and the lcov file
// made from them for the nightly snapshot in Codecov, see Coverage in
// docs/TESTING.md. Every call to monocart-coverage-reports is here, so
// counts.test.mjs runs the very calls the night runs, and an update of
// the package is checked by the interface's tests on its pull request
// rather than first by a night on main.
import { copyFileSync, existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { CoverageReport } from "monocart-coverage-reports";

// keep adds the counts of one page to the folder: its scripts as
// Playwright's stopJSCoverage gives them, each with its source map, see
// counted in walks/bridge.mjs.
export async function keep(folder, scripts) {
  await new CoverageReport({ outputDir: folder, logging: "error" }).add(scripts);
}

// report turns the counts kept in the folder into one lcov file of the
// interface's own code, and says how many components it ran lines of.
// Only the code under frontend/src is counted, by the source maps, so a
// library the bundle carries is not. A report that counts no line of
// any component fails, since the walks reach every one, and a report
// like that means the counting broke, not that the walks stopped
// reaching the app.
export async function report(kept, out) {
  if (!existsSync(resolve(kept, ".cache"))) throw new Error(`the walks kept no counts in ${kept}`);
  await new CoverageReport({
    outputDir: kept,
    logging: "error",
    // The sources are named from the top of the repository, see counted
    // in walks/bridge.mjs.
    sourceFilter: (path) => path.startsWith("frontend/src/"),
    // istanbul writes only inside the folder it reports into.
    reports: [["lcovonly", { file: "lcov.info" }]],
  }).generate();
  copyFileSync(resolve(kept, "lcov.info"), out);

  // A component a line of which was run: lcov says how many lines of a
  // file were run as LH, after the file's name, SF.
  let file = "";
  let components = 0;
  for (const line of readFileSync(out, "utf8").split("\n")) {
    if (line.startsWith("SF:")) file = line.slice(3);
    else if (line.startsWith("LH:") && file.endsWith(".svelte") && Number(line.slice(3)) > 0) components++;
  }
  if (components === 0) throw new Error(`${out} counts no line of any component, so the counting in the walks broke`);
  return components;
}
