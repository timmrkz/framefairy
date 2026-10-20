// Where the app's updates stand, read in two places: the Updates page,
// which says it in words, and Updates at the foot of the sidebar, which
// wears a dot for it. Both read the one state kept here and the one mark
// worked out from it, so the sidebar never says less than the page, or
// something else. Tim followed a pull request that closed, and the page
// wore the warning while the sidebar showed nothing, issue #406. See
// docs/UPDATES.md.

import { api, onUpdates, size, type UpdateState } from "./api";
import { said } from "./said";

// A check that finds nothing is over in a tenth of a second, which is
// a flash nobody can read: the button lit and went out and the line
// under the build changed and changed back. So looking is shown for at
// least this long, the answer after it, on the page and in the sidebar.
const LOOK_AT_LEAST = 1400;

// What the line under the build starts with. A dot is work, running or
// waiting to be picked up: look pulses, new stands still. Something that
// needs a look is the triangle, warn or err. ok is the check, and idle and
// off show nothing.
export type Mark = "" | "off" | "look" | "new" | "err" | "warn" | "ok" | "idle";

// A channel is what it is and nothing more: a branch or a pull request
// by its number. The pull request's title says what it is about, which
// is not what is being picked here. Customers will see releases the same
// way, by their version.
export const channelName = (id: string) => (id.startsWith("pr-") ? `PR #${id.slice(3)}` : `Branch ${id}`);

// The channel followed, as updates.Followed has it: the one picked, or
// the one a build from a channel came from. Empty for a build made on
// the Mac until a channel is chosen.
export const following = (u: UpdateState | null) => (u ? u.picked || u.follows || u.gone || u.channel || "" : "");

// The pull request followed no longer merges into main. Its builds
// still come, but CI does not run on them, so it wears the colour of a
// warning, the way one that closed does.
export const conflicted = (u: UpdateState | null) => {
  const f = following(u);
  return !!u?.channels?.some((c) => c.id === f && c.conflict);
};

function when(checked: string | undefined, now: number): string {
  if (!checked) return "";
  const at = Date.parse(checked);
  // Go's zero time is the year 1.
  if (!isFinite(at) || at < Date.UTC(2000, 0, 1)) return "";
  if (now - at < 60_000) return "Checked just now.";
  const t = new Date(at);
  return `Checked at ${String(t.getHours()).padStart(2, "0")}:${String(t.getMinutes()).padStart(2, "0")}.`;
}

const short = (commit: string) => commit.slice(0, 7);

// Where things stand, in the words of the line under the build: its mark,
// what it is in a few words, then what that means. working is the work
// Relaunch waits for, in its own words, empty when there is none.
export function standing(
  u: UpdateState | null,
  how: { working?: string; now?: number } = {},
): { mark: Mark; head: string; more: string } {
  if (!u) return { mark: "", head: "", more: "" };
  if (u.off) return { mark: "off", head: "This build does not update itself", more: u.off };
  const now = how.now ?? Date.now();
  const working = how.working ?? "";
  const followingName = following(u) ? channelName(following(u)) : "";
  const next = u.next ?? "";
  // A commit pushed to the channel whose build has not come yet. The build
  // there is is still offered, it is the newest there is, but never as
  // the newest commit: a build of the commit before was once tested as
  // the fix it did not have yet.
  const building = u.building ? short(u.building) : "";
  const stillBuilding = building ? ` ${building} is being built.` : "";
  switch (u.phase) {
    case "checking":
      return { mark: "look", head: "Looking for a newer build", more: followingName ? `Of ${followingName}.` : "" };
    case "downloading": {
      const part = u.total > 0 ? `${Math.floor((u.written / u.total) * 100)} % of ${size(u.total)}.` : "";
      // Work running, so the pulse, the same as looking.
      return { mark: "look", head: "A newer build is downloading", more: `${next}. ${part}${stillBuilding}`.trim() };
    }
    case "ready":
      return {
        mark: "new",
        head: "A newer build is ready",
        more: working
          ? `${next}. Relaunch once ${working} is done, or it goes in when you quit.${stillBuilding}`
          : `${next}. Relaunch to finish updating, or it goes in when you quit.${stillBuilding}`,
      };
    case "failed":
      return { mark: "err", head: "The check did not get through", more: `${u.problem} ${when(u.checked, now)}`.trim() };
    // The newest build is here, and a newer commit is on its way. Work
    // running somewhere else, so the pulse.
    case "current":
      if (building)
        return {
          mark: "look",
          head: "A newer commit is being built",
          more: `${building}. This is the build before it, the newest of ${followingName} so far. ${when(u.checked, now)}`.trim(),
        };
      if (conflicted(u))
        return {
          mark: "warn",
          head: "Up to date",
          more: `This is the newest build of ${followingName}. It conflicts with main, so CI has not tested it. ${when(u.checked, now)}`.trim(),
        };
      return {
        mark: "ok",
        head: "Up to date",
        more: `This is the newest build of ${followingName}. ${when(u.checked, now)}`.trim(),
      };
    // The pull request followed was merged or closed. Nothing downloads
    // by itself: which channel to follow next is the person's to say.
    case "gone":
      return {
        mark: "warn",
        head: `${channelName(u.gone)} is closed`,
        more: "Nothing downloads until you choose what to follow next.",
      };
  }
  // A build made on the Mac follows nothing until a channel is chosen,
  // whatever an earlier build picked. Choosing one follows it fully, so
  // looking is the next thing this line says.
  if (!followingName) {
    return {
      mark: "idle",
      head: "Built on this Mac",
      more: "It stays as it is until you choose a channel to follow.",
    };
  }
  // A build from a channel, in the moment before its first look has
  // answered.
  return { mark: "idle", head: "Not checked yet", more: "It looks when the app starts and every ten minutes." };
}

// The dot Updates in the sidebar wears for where things stand, as the
// classes of app.css's .dot, empty for none. It is the page's mark in the
// colour the page wears it in: work pulses and a build ready stands still,
// in the accent or, on a pull request in conflict, in the colour of a
// warning, and a triangle on the page is a dot that stands still in the
// triangle's colour. All is well, the check, is no dot: the sidebar only
// speaks when there is something to look at.
export function railDot(u: UpdateState | null): string {
  const { mark } = standing(u);
  const warn = conflicted(u) ? " warn" : "";
  switch (mark) {
    case "look":
      return `busy${warn}`;
    case "new":
      return `ready${warn}`;
    case "warn":
      return "warn";
    case "err":
      return "err";
  }
  return "";
}

class Updates {
  // What is on screen, which trails what the Go side last said while a
  // check is held.
  shown = $state<UpdateState | null>(null);
  #latest: UpdateState | null = null;
  #lookingSince = 0;
  #held: ReturnType<typeof setTimeout> | undefined;
  // Events heard so far, so an answer to an asking that an event came
  // after is known to be the older of the two.
  #told = 0;
  #dot = "";

  // Takes what the Go side said, or what a click shows at once.
  heard(u: UpdateState) {
    this.#latest = u;
    if (u.phase === "checking") {
      if (this.shown?.phase !== "checking") this.#lookingSince = Date.now();
      clearTimeout(this.#held);
      this.#show(u);
      return;
    }
    // Only the looking is held. Everything else the answer brings, the
    // channel list above all, is shown at once: a list held back with
    // the looking offered the channels from before the check, which the
    // app now listens to from its start, so the page opened in the middle
    // of a check that began before it.
    const left = LOOK_AT_LEAST - (Date.now() - this.#lookingSince);
    if (this.shown?.phase === "checking" && left > 0) {
      clearTimeout(this.#held);
      this.#show({ ...u, phase: "checking" });
      this.#held = setTimeout(() => this.#latest && this.#show(this.#latest), left);
      return;
    }
    this.#show(u);
  }

  #show(u: UpdateState) {
    this.shown = u;
    const dot = railDot(u);
    if (dot === this.#dot) return;
    this.#dot = dot;
    const s = standing(u);
    said(`updates: the sidebar's dot is ${dot || "none"}, ${s.head}`, { level: "debug" });
  }

  // Asks the Go side where things stand. Asking also reads the channel
  // list, and what that read found is sent as an event, which can arrive
  // before the answer to the asking does. The answer is then the older of
  // the two, and it emptied the list again, so it only counts while no
  // event has come since it was asked.
  ask() {
    const at = this.#told;
    api
      .updates()
      .then((u) => this.#told === at && this.heard(u))
      .catch(() => {});
  }

  // Listens from the start of the app until it closes.
  listen(): () => void {
    const off = onUpdates((u) => {
      this.#told++;
      this.heard(u);
    });
    this.ask();
    return () => {
      off();
      clearTimeout(this.#held);
    };
  }
}

export const updates = new Updates();
