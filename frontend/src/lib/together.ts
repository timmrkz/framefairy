// Run-downs that tell one time in several places, by a name they share.
// The work behind a removal is one job, and its time is one time wherever
// it shows: the hand on any of its run-downs holds them all, so none runs
// out while another stands still under the hand. See Busy.

// One run-down: whether the hand is on it, and what it does when told to
// look again, hold or run.
export type Member = { holding: () => boolean; settle: () => void };

const groups = new Map<string, Set<Member>>();

// together puts self in the group of that name, or alone with no name.
// holding says whether the hand is on any of the group. settleAll tells
// every one of them to look again, which is called when the hand comes onto
// self or leaves it. leave takes self out, and tells the rest to look
// again, because the one the hand held them by may be the one going.
export function together(name: string, self: Member) {
  if (name) {
    if (!groups.has(name)) groups.set(name, new Set());
    groups.get(name)!.add(self);
  }
  const members = () => (name ? (groups.get(name) ?? new Set([self])) : new Set([self]));
  return {
    holding: () => [...members()].some((m) => m.holding()),
    settleAll: () => {
      for (const m of members()) m.settle();
    },
    leave: () => {
      if (!name) return;
      const left = groups.get(name);
      left?.delete(self);
      if (!left?.size) groups.delete(name);
      else for (const m of left) m.settle();
    },
  };
}
