// Which walks a change needs: the runs of TestWalks that go through what
// changed, and no others. scripts/changed.sh asks it, for make changed on
// any machine and for CI's walks on a pull request, so the walks are
// picked one way everywhere. See docs/TESTING.md.
//
//   node pick.mjs < files      the changed files that reach the walks, one
//                              a line, in. The runs out, one a line, as
//                              "sequence <name>" or "script <file>", or
//                              "all" for every run
//   node pick.mjs --shares 4 < runs
//                              how many machines of the 4 CI has the runs
//                              need, by walk-times.tsv: as many as a
//                              quarter of every run fills
//
// BASE is the commit the change is compared with, origin/main's merge
// base unless set.
//
// A walk file is read for what it changed:
//
//   <folder>/sequences.mjs  the sequences that are new or changed, by
//                           name and steps, and not those that only
//                           moved or gained a comment
//   <folder>/steps.mjs      the sequences that take a step whose lines
//                           changed. A line outside every step, an import
//                           or a helper, takes every sequence that takes a
//                           step of that file, and of every folder for
//                           common/steps.mjs, whose helpers they all use
//   a walk, like timeline.mjs
//                           that walk, for every seed
//   walk-times.tsv          nothing, it is only how long the runs took
//
// Anything else in the folder, the runner, the bridge or the rules, is
// what every run runs, and so is a change to the interface or the Go
// side: every run goes through the app, and no name says which part.
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, relative } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = import.meta.dirname;
const top = join(here, "../../..");
const walks = relative(top, here);

const git = (root, ...args) => execFileSync("git", args, { cwd: root, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"], maxBuffer: 1 << 28 });

// The folders of sequences under root.
const foldersOf = (root) =>
  readdirSync(join(root, walks), { withFileTypes: true })
    .filter((e) => e.isDirectory() && existsSync(join(root, walks, e.name, "sequences.mjs")))
    .map((e) => e.name);

// The lines of file that changed against base, as the lines they were on
// before and the lines they are on now. A line added has no line before,
// and a line taken away none now.
export function hunks(root, base, file) {
  let diff = "";
  try {
    diff = git(root, "diff", "-U0", "--no-renames", base, "--", file);
  } catch {
    return null;
  }
  const was = [];
  const now = [];
  for (const m of diff.matchAll(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/gm)) {
    const [a, b, c, d] = [m[1], m[2] ?? "1", m[3], m[4] ?? "1"].map(Number);
    for (let k = 0; k < b; k++) was.push(a + k);
    for (let k = 0; k < d; k++) now.push(c + k);
  }
  return { was, now };
}

// The file as it was at base, or null where it was not there.
function before(root, base, file) {
  try {
    return git(root, "show", `${base}:${file}`);
  } catch {
    return null;
  }
}

// A module of data, the way a folder's sequences.mjs is, read from its
// text: it imports nothing, so it can be read from anywhere.
async function dataOf(text) {
  const dir = mkdtempSync(join(tmpdir(), "pick-"));
  try {
    const f = join(dir, "data.mjs");
    writeFileSync(f, text);
    return (await import(pathToFileURL(f).href)).default;
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

// Where each step of a steps.mjs is, from the comment above it to the line
// that closes it, by its name, and the names it is known by as well.
export function stepsOf(text) {
  const lines = text.split("\n");
  const spans = [];
  let open = null;
  let doc = null;
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const n = i + 1;
    if (!open && /^ {2}\/\//.test(line)) doc ??= n;
    else if (!open) {
      const m = line.match(/^ {2}async (?:"((?:[^"\\]|\\.)*)"|([\w$]+))\(/);
      if (m) open = { name: m[1] !== undefined ? JSON.parse(`"${m[1]}"`) : m[2], from: doc ?? n };
      else doc = null;
    }
    if (open && line === "  },") {
      spans.push({ ...open, to: n });
      open = null;
      doc = null;
    }
  }
  // steps["edge in"] = steps["edge to word"]: one step under another name.
  const aliases = [];
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(/^steps\[("(?:[^"\\]|\\.)*")\] = steps\[("(?:[^"\\]|\\.)*")\];$/);
    if (m) aliases.push({ name: JSON.parse(m[1]), of: JSON.parse(m[2]), line: i + 1 });
  }
  return { spans, aliases };
}

// The steps a changed line of a steps.mjs lies in, or null for a line
// outside every step.
function stepsAt(text, lines) {
  const { spans, aliases } = stepsOf(text);
  const names = new Set();
  let outside = false;
  for (const l of lines) {
    const s = spans.find((x) => x.from <= l && l <= x.to);
    const a = aliases.find((x) => x.line === l);
    if (s) names.add(s.name);
    else if (a) names.add(a.name);
    else outside = true;
  }
  // A step under another name is the same step.
  for (const a of aliases) if (names.has(a.of)) names.add(a.name);
  return { names, outside };
}

// Every sequence now, with the folder it is kept in.
async function sequencesNow(root) {
  const all = [];
  for (const f of foldersOf(root)) {
    const file = join(root, walks, f, "sequences.mjs");
    for (const q of await dataOf(readFileSync(file, "utf8"))) all.push({ ...q, folder: f });
  }
  return all;
}

const sequence = (name) => `sequence ${name}`;

// Whether another folder's steps import the steps of folder.
const importedElsewhere = (root, folder) =>
  foldersOf(root).some((f) => {
    const file = join(root, walks, f, "steps.mjs");
    return f !== folder && existsSync(file) && readFileSync(file, "utf8").includes(`"../${folder}/steps.mjs"`);
  });

const takes = (q, names) => q.steps.some(([s]) => names.has(s));

// The runs files need, or "all".
export async function pick(files, { base, root = top }) {
  const runs = new Set();
  let all = false;
  let sequences = null;
  const now = async () => (sequences ??= await sequencesNow(root));
  const folders = foldersOf(root);
  for (const file of files) {
    if (all) break;
    const inWalks = file.startsWith(`${walks}/`) ? file.slice(walks.length + 1) : null;
    const [folder, name] = inWalks?.split("/") ?? [];
    if (inWalks === "walk-times.tsv") continue;
    if (inWalks && name === "sequences.mjs" && folders.includes(folder)) {
      const was = before(root, base, file);
      const old = new Map((was === null ? [] : await dataOf(was)).map((q) => [q.name, JSON.stringify(q)]));
      for (const q of (await now()).filter((x) => x.folder === folder)) {
        const { folder: _, ...plain } = q;
        if (old.get(q.name) !== JSON.stringify(plain)) runs.add(sequence(q.name));
      }
      continue;
    }
    if (inWalks && name === "steps.mjs" && (folders.includes(folder) || folder === "common")) {
      const changed = hunks(root, base, file);
      if (!changed) {
        all = true;
        break;
      }
      const text = existsSync(join(root, file)) ? readFileSync(join(root, file), "utf8") : "";
      const was = before(root, base, file) ?? "";
      const a = stepsAt(was, changed.was);
      const b = stepsAt(text, changed.now);
      const names = new Set([...a.names, ...b.names]);
      if (a.outside || b.outside) {
        // A helper is its folder's, unless another folder imports it.
        if (folder === "common" || importedElsewhere(root, folder)) {
          all = true;
          break;
        }
        for (const s of [...stepsOf(was).spans, ...stepsOf(text).spans]) names.add(s.name);
        for (const al of [...stepsOf(was).aliases, ...stepsOf(text).aliases]) names.add(al.name);
      }
      for (const q of await now()) if (takes(q, names)) runs.add(sequence(q.name));
      continue;
    }
    // A test of the harness is Vitest's, run by make interface, no walk,
    // and so is what picks the walks, which walks nothing itself.
    if (inWalks && (inWalks.endsWith(".test.mjs") || inWalks === "pick.mjs")) continue;
    if (inWalks && !inWalks.includes("/") && inWalks.endsWith(".mjs")) {
      const text = existsSync(join(root, file)) ? readFileSync(join(root, file), "utf8") : (before(root, base, file) ?? "");
      if (inWalks !== "walk.mjs" && text.includes(`from "./walk.mjs"`)) {
        runs.add(`script ${inWalks}`);
        continue;
      }
    }
    all = true;
  }
  if (all) return "all";
  return [...runs].sort();
}

// How long each run takes, by walk-times.tsv, its seeds together.
export function secondsOf() {
  const took = new Map();
  let text = "";
  try {
    text = readFileSync(join(here, "walk-times.tsv"), "utf8");
  } catch {
    return took;
  }
  for (const line of text.split("\n")) {
    const m = line.match(/^\s*(\d+)\t(\S+)(?: (.*))?$/);
    if (!m) continue;
    const [, s, script, env = ""] = m;
    const id = script === "sequences.mjs" && env.startsWith("SEQUENCE=") ? sequence(env.slice("SEQUENCE=".length)) : `script ${script}`;
    took.set(id, (took.get(id) ?? 0) + Number(s));
  }
  return took;
}

// How many of most machines the runs need: as many as a share of every
// run, spread over most, fills.
export function sharesFor(runs, most) {
  const took = secondsOf();
  const every = [...took.values()].reduce((a, b) => a + b, 0);
  if (runs === "all" || !every) return most;
  const middle = [...took.values()].sort((a, b) => a - b)[Math.floor(took.size / 2)] ?? 1;
  const these = runs.reduce((sum, r) => sum + (took.get(r) ?? middle), 0);
  return Math.max(1, Math.min(most, Math.ceil((these / every) * most)));
}

const input = () => readFileSync(0, "utf8").split("\n").map((l) => l.trim()).filter(Boolean);

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  if (process.argv[2] === "--shares") {
    const most = Number(process.argv[3]);
    const lines = input();
    console.log(sharesFor(lines[0] === "all" ? "all" : lines, most));
  } else {
    let base = process.env.BASE;
    if (!base) {
      try {
        base = git(top, "merge-base", "HEAD", "origin/main").trim();
      } catch {
        base = "HEAD";
      }
    }
    const runs = await pick(input(), { base });
    console.log(runs === "all" ? "all" : runs.join("\n"));
  }
}
