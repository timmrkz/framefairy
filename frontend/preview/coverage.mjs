// Turns the counts the walks kept, see counted in walks/bridge.mjs, into
// one lcov file of the interface's own code, the components among it,
// for the nightly snapshot in Codecov. See Coverage in docs/TESTING.md.
//
//   node preview/coverage.mjs <the folder the walks kept> <the lcov file>
//
// Only the code under frontend/src is counted, by the source maps the
// build left beside each script, so a library the bundle carries is not.
// A report that counts no line of any component fails, since the walks
// reach every one, and a report like that means the counting broke, not
// that the walks stopped reaching the app.
import { copyFileSync, existsSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import { CoverageReport } from "monocart-coverage-reports";

const [kept, out] = process.argv.slice(2);
if (!kept || !out) throw new Error("usage: node preview/coverage.mjs <the folder the walks kept> <the lcov file>");
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
console.log(`ok  \tthe walks ran lines of ${components} components`);
