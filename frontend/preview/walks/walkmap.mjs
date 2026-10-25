// The map of what every run of the walks went through: for each line of
// the interface and of the Go side, which runs ran it. It is recorded
// from every run at night, see walks-nightly.yml, and pick.mjs reads it, so a
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
import { existsSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { basename, dirname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const top = join(import.meta.dirname, "../../..");
const git = (root, ...args) => execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], maxBuffer: 1 << 28 });

// The map in file, or null when there is none or it cannot be read.
export function readMap(file) {
  try {
    const map = JSON.parse(readFileSync(file, "utf8"));
    // A map of no runs knows nothing, and would pick nothing.
    if (map.version !== 1 || !map.commit || !Array.isArray(map.runs) || map.runs.length === 0) return null;
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

// ---- Recording what a run's pages ran.

const B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
const digit = new Map([...B64].map((c, i) => [c, i]));

// The mappings of a source map, decoded: for every piece of the generated
// code, its line and column, and the source and line it came from.
export function decodeMappings(mappings) {
  const out = [];
  let src = 0;
  let srcLine = 0;
  let srcCol = 0;
  let name = 0;
  let line = 0;
  for (const group of mappings.split(";")) {
    let col = 0;
    if (group) {
      for (const seg of group.split(",")) {
        if (!seg) continue;
        const v = [];
        let value = 0;
        let shift = 0;
        for (const c of seg) {
          const d = digit.get(c);
          value += (d & 31) << shift;
          if (d & 32) shift += 5;
          else {
            v.push(value & 1 ? -(value >>> 1) : value >>> 1);
            value = 0;
            shift = 0;
          }
        }
        col += v[0];
        if (v.length >= 4) {
          src += v[1];
          srcLine += v[2];
          srcCol += v[3];
          if (v.length >= 5) name += v[4];
          out.push({ line, col, src, srcLine });
        }
      }
    }
    line++;
  }
  return out;
}

// The interface's own files a source map names, from root, the top of
// the repository, by their place in its sources, and null for a file of a
// package or of the preview.
//
// A source is named from the map's folder, as source maps say, except in
// the maps of Vite's workers, which name it from the folder above, the
// build's own. So it is the first of the two where the file is.
function sourcesOf(map, mapFile, root) {
  const bases = [dirname(mapFile), dirname(dirname(mapFile))].map((d) => join(d, map.sourceRoot ?? ""));
  return map.sources.map((s) => {
    const at = bases.map((b) => resolve(b, s)).find((f) => existsSync(f));
    if (!at) return null;
    const file = relative(root, at).split(sep).join("/");
    return file.startsWith("frontend/src/") && !file.includes("/node_modules/") ? file : null;
  });
}

// Every file of the interface the build in dist holds, workers among them.
export function bundleOf(dist, root = top) {
  const files = new Set();
  for (const f of readdirSync(join(dist, "assets"))) {
    if (!f.endsWith(".map")) continue;
    const mapFile = join(dist, "assets", f);
    for (const s of sourcesOf(JSON.parse(readFileSync(mapFile, "utf8")), mapFile, root)) if (s) files.add(s);
  }
  return [...files].sort();
}

// The count at each offset of a script, from V8's ranges of what ran: the
// innermost range holding an offset says it, since ranges nest.
function countsAt(functions, offsets) {
  const ranges = functions.flatMap((f) => f.ranges).sort((a, b) => a.startOffset - b.startOffset || b.endOffset - a.endOffset);
  const out = new Array(offsets.length);
  const open = [];
  let next = 0;
  for (let i = 0; i < offsets.length; i++) {
    const at = offsets[i];
    while (next < ranges.length && ranges[next].startOffset <= at) {
      while (open.length && open.at(-1).endOffset <= ranges[next].startOffset) open.pop();
      open.push(ranges[next++]);
    }
    while (open.length && open.at(-1).endOffset <= at) open.pop();
    out[i] = open.length ? open.at(-1).count : 0;
  }
  return out;
}

const decoded = new Map();

// What the pages of a run ran, from Playwright's JS coverage: the lines of
// each file of the interface that ran, and the lines that are code at all,
// through the source maps of the build in dist.
export function linesRan(entries, dist, root = top) {
  const ran = new Map();
  const known = new Map();
  const add = (m, file, line) => {
    if (!m.has(file)) m.set(file, new Set());
    m.get(file).add(line);
  };
  for (const entry of entries) {
    let path;
    try {
      path = new URL(entry.url).pathname;
    } catch {
      continue;
    }
    if (!path.startsWith("/assets/") || !path.endsWith(".js")) continue;
    const mapFile = join(dist, path + ".map");
    if (!decoded.has(mapFile)) {
      let map = null;
      try {
        map = JSON.parse(readFileSync(mapFile, "utf8"));
      } catch {
        // A script with no map holds nothing of ours.
      }
      decoded.set(mapFile, map && { sources: sourcesOf(map, mapFile, root), segments: decodeMappings(map.mappings) });
    }
    const map = decoded.get(mapFile);
    if (!map) continue;
    const starts = [0];
    for (let i = 0; i < entry.source.length; i++) if (entry.source.charCodeAt(i) === 10) starts.push(i + 1);
    const segments = map.segments.filter((s) => map.sources[s.src]);
    const counts = countsAt(
      entry.functions,
      segments.map((s) => (starts[s.line] ?? 0) + s.col),
    );
    segments.forEach((s, i) => {
      const file = map.sources[s.src];
      add(known, file, s.srcLine + 1);
      if (counts[i] > 0) add(ran, file, s.srcLine + 1);
    });
  }
  const spans = (m) => Object.fromEntries([...m].map(([f, lines]) => [f, toSpans([...lines])]));
  return { ran: spans(ran), known: spans(known) };
}

// Lines as spans of lines that follow each other, [from, to].
export function toSpans(lines) {
  const out = [];
  for (const l of lines.sort((a, b) => a - b)) {
    if (out.length && l <= out.at(-1)[1] + 1) out.at(-1)[1] = Math.max(out.at(-1)[1], l);
    else out.push([l, l]);
  }
  return out;
}

// ---- Making the map from what the runs wrote.

// Every file named name under dir, however deep, the way the records of
// each machine come down into a folder of their own.
function filesUnder(dir, test) {
  const out = [];
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, e.name);
    if (e.isDirectory()) out.push(...filesUnder(p, test));
    else if (test(p)) out.push(p);
  }
  return out;
}

// The map from the records in dir, see make walkmap, recorded on commit.
export function buildMap(dir, commit) {
  const records = filesUnder(dir, (p) => basename(dirname(p)) === "runs" && p.endsWith(".json")).map((f) => JSON.parse(readFileSync(f, "utf8")));
  const knowns = filesUnder(dir, (p) => /^known-\d+\.json$/.test(basename(p))).map((f) => JSON.parse(readFileSync(f, "utf8")));
  const runs = [...new Set(records.map((r) => r.run))].sort();
  const index = new Map(runs.map((r, i) => [r, i]));
  // For every file, for every line, the runs that ran it.
  const byLine = new Map();
  const at = (file, line) => {
    if (!byLine.has(file)) byLine.set(file, new Map());
    const lines = byLine.get(file);
    if (!lines.has(line)) lines.set(line, new Set());
    return lines.get(line);
  };
  const bundle = new Set();
  for (const k of knowns) {
    for (const [file, spans] of Object.entries(k.known ?? {})) for (const [a, b] of spans) for (let l = a; l <= b; l++) at(file, l);
    for (const f of k.bundle ?? []) bundle.add(f);
  }
  for (const r of records) {
    const n = index.get(r.run);
    for (const [file, spans] of Object.entries(r.ran)) for (const [a, b] of spans) for (let l = a; l <= b; l++) at(file, l).add(n);
  }
  const sets = [];
  const setOf = new Map();
  const lines = {};
  for (const file of [...byLine.keys()].sort()) {
    const out = [];
    for (const [line, set] of [...byLine.get(file)].sort((x, y) => x[0] - y[0])) {
      const key = [...set].sort((x, y) => x - y).join(",");
      if (!setOf.has(key)) {
        setOf.set(key, sets.length);
        sets.push(key ? key.split(",").map(Number) : []);
      }
      const s = setOf.get(key);
      const last = out.at(-1);
      if (last && last[1] === line - 1 && last[2] === s) last[1] = line;
      else out.push([line, line, s]);
    }
    lines[file] = out;
  }
  return { version: 1, commit, runs, sets, lines, bundle: [...bundle].sort() };
}

if (process.argv[1] === fileURLToPath(import.meta.url) && process.argv[2] === "build") {
  const [, , , dir, out] = process.argv;
  const commit = git(top, "rev-parse", "HEAD").trim();
  const map = buildMap(dir, commit);
  if (map.runs.length === 0) {
    console.error(`no run of the walks was recorded in ${dir}, so there is no map`);
    process.exit(1);
  }
  writeFileSync(out, JSON.stringify(map));
  const files = Object.keys(map.lines);
  console.log(
    `walk map of ${commit.slice(0, 7)}: ${map.runs.length} runs, ${files.filter((f) => f.endsWith(".go")).length} files of Go and ${files.filter((f) => !f.endsWith(".go")).length} of the interface, ${map.sets.length} sets of runs`,
  );
}
