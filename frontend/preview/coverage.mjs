// Turns the counts the walks kept, see counted in walks/bridge.mjs, into
// one lcov file of the interface's own code, the components among it,
// for the nightly snapshot in Codecov. See Coverage in docs/TESTING.md
// and report in counts.mjs.
//
//   node preview/coverage.mjs <the folder the walks kept> <the lcov file>
import { report } from "./counts.mjs";

const [kept, out] = process.argv.slice(2);
if (!kept || !out) throw new Error("usage: node preview/coverage.mjs <the folder the walks kept> <the lcov file>");
const components = await report(kept, out);
console.log(`ok  \tthe walks ran lines of ${components} components`);
