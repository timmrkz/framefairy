// A path the way the app shows it, one way everywhere: inside the home
// folder from ~, like ~/.framefairy/models, and anywhere else in full.
// The settings file, the checks and a folder just chosen hand paths over
// in either form, and Tim saw both side by side on one page, 2026-10-27.
// Only shown, never saved: what is saved stays as it came.
export function shownPath(path: string, home = ""): string {
  const root = home.replace(/[\\/]+$/, "");
  if (!path || !root) return path;
  if (path === root) return "~";
  const sep = path.charAt(root.length);
  if (path.startsWith(root) && (sep === "/" || sep === "\\")) return "~" + path.slice(root.length);
  return path;
}
