// The map of what every run of the walks went through: for each line of
// the interface and of the Go side, which runs ran it. It is recorded
// from every run at night, see walkmap.yml, and pick.mjs reads it, so a
// change to a few lines runs the walks that go through them and no
// others. See docs/TESTING.md.
//
// The map is JSON:
//
//   commit  the commit it was recorded on, which a change is compared
//           with, so its lines are the lines the map knows
//   runs    every run, as "sequence <name>" or "script <file>"
//   sets    sets of runs, by their place in runs
//   lines   for each file, spans of lines [from, to, set]: the runs of
//           that set ran every line of the span. A set that is empty is
//           code no run ran, and a line in no span is no code the
//           recording saw: a comment, a type, a declaration
//   bundle  every file of the interface the walks' build holds, those
//           only a worker runs among them, which the recording cannot see
import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";

const top = join(import.meta.dirname, "../../..");
const git = (root, ...args) => execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], maxBuffer: 1 << 28 });

// The map in file, or null when there is none or it cannot be read.
export function readMap(file) {
  try {
    const map = JSON.parse(readFileSync(file, "utf8"));
    if (map.version !== 1 || !map.commit || !Array.isArray(map.runs)) return null;
    map.bundled = new Set(map.bundle ?? []);
    return map;
  } catch {
    return null;
  }
}

// Whether the commit the map was recorded on is here, fetched if it is
// not, since a pull request's checkout holds only its own two commits.
function have(map, root) {
  if (map.here !== undefined) return map.here;
  const is = () => {
    try {
      git(root, "cat-file", "-e", `${map.commit}^{commit}`);
      return true;
    } catch {
      return false;
    }
  };
  if (!is()) {
    try {
      git(root, "fetch", "-q", "--depth=1", "origin", map.commit);
    } catch {
      // Not there to fetch, and is() says so below.
    }
  }
  map.here = is();
  return map.here;
}

// The lines of file the map knows that changed since it was recorded.
function changedSince(map, file, root) {
  let diff;
  try {
    diff = git(root, "diff", "-U0", "--no-renames", map.commit, "--", file);
  } catch {
    return null;
  }
  const lines = [];
  for (const m of diff.matchAll(/^@@ -(\d+)(?:,(\d+))? \+\d+(?:,\d+)? @@/gm)) {
    const a = Number(m[1]);
    const b = Number(m[2] ?? "1");
    for (let k = 0; k < b; k++) lines.push(a + k);
    // Lines only added go between two lines that were there.
    if (b === 0) lines.push(a, a + 1);
  }
  return lines;
}

// The runs in a set, by name.
const runsOf = (map, set) => map.sets[set].map((r) => map.runs[r]);

// Every run that ran a line of file.
function fileUnion(map, file) {
  const out = new Set();
  for (const [, , set] of map.lines[file] ?? []) for (const r of runsOf(map, set)) out.add(r);
  return out;
}

// Every run that ran a line of a file in dir, which for Go is the
// package: a constant or a type of one file is used by all of them.
function dirUnion(map, dir) {
  const out = new Set();
  for (const file of Object.keys(map.lines)) {
    if (dirname(file) === dir) for (const r of fileUnion(map, file)) out.add(r);
  }
  return out;
}

// The Go package a file belongs to, testdata and what a package embeds
// as well.
const packageOf = (file) => {
  const at = file.indexOf("/testdata/");
  return at >= 0 ? file.slice(0, at) : dirname(file);
};

// The runs a change to file needs by the map, "all" for every run, or
// null for a file the map does not answer for.
export function pickByMap(map, file, root = top) {
  const go = file.endsWith(".go");
  const ui = file.startsWith("frontend/src/");
  const inPackage = !go && !ui && Object.keys(map.lines).some((f) => f.endsWith(".go") && dirname(f) === packageOf(file));
  if (!go && !ui && !inPackage) return null;
  // The bridge is the app's test code, which the recording does not see,
  // and a style sheet is code no line of the recording runs.
  if (file.startsWith("cmd/framefairy-app/") && file.endsWith("_test.go")) return "all";
  if (go && file.endsWith("_test.go")) return [];
  if (ui && file.endsWith(".css")) return "all";
  if (ui && /\.test\.ts$/.test(file)) return [];
  if (!have(map, root)) return "all";
  if (inPackage) return [...dirUnion(map, packageOf(file))];
  const spans = map.lines[file];
  if (!spans) {
    // A file of the interface only a worker runs.
    if (ui) return map.bundled.has(file) ? "all" : [];
    // A Go file the map does not know is new: its package, if the map
    // knows that.
    return [...dirUnion(map, dirname(file))];
  }
  const changed = changedSince(map, file, root);
  if (!changed) return "all";
  const out = new Set();
  let wholeFile = false;
  let wholePackage = false;
  for (const line of changed) {
    const span = spans.find(([from, to]) => from <= line && line <= to);
    if (!span) {
      // No code the recording saw: for Go a declaration the whole
      // package may use, for the interface a type, an import or the
      // style of a component.
      if (go) wholePackage = true;
      else wholeFile = true;
    } else if (map.sets[span[2]].length === 0) {
      // Code no run ran, which may still run in a worker, or behind a
      // change elsewhere in the file.
      wholeFile = true;
    } else for (const r of runsOf(map, span[2])) out.add(r);
  }
  if (wholePackage) for (const r of dirUnion(map, dirname(file))) out.add(r);
  else if (wholeFile) for (const r of fileUnion(map, file)) out.add(r);
  return [...out];
}
