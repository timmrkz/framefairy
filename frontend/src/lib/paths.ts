// A path the way the app shows it, one way everywhere: in full, from the
// top of the disk, like /Users/tim/.framefairy/models. The settings file
// keeps paths from ~, the checks and the folder dialog hand them over in
// full, and Tim saw both side by side on one page twice, 2026-10-27 and
// 2026-10-28, the second time after inside the home folder had become ~.
// He asked for the full path everywhere. Only shown, never saved: what is
// saved stays as it came.
export function shownPath(path: string, home = ""): string {
  const root = home.replace(/[\\/]+$/, "");
  if (!root || !path.startsWith("~")) return path;
  if (path === "~") return root;
  const sep = path.charAt(1);
  if (sep === "/" || sep === "\\") return root + path.slice(1);
  return path;
}

// A folder the way the app shows it: a path in full, with the slash at
// its end that says it is a folder and not a file, the way Tim asked for
// the models folder, 2026-10-28. The slash is the one the path is written
// with.
export function shownFolder(path: string, home = ""): string {
  const full = shownPath(path, home);
  if (!full || /[\\/]$/.test(full)) return full;
  return full + (full.includes("\\") && !full.includes("/") ? "\\" : "/");
}
