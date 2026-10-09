// Who a key belongs to. The workspace takes keys of its own, the space
// bar, I and O, the arrows, Undo from the menu, but only when nothing
// else has them: a field being typed in has its own keys for its text,
// and the box over the workspace asking something takes every key until
// it is answered. Six places asked this each in their own words, and one
// of them, Undo, forgot the box, so Cmd+Z changed the episode behind a
// box asking whether to remove it.

// A field being typed in holds the keyboard.
export function typing(): boolean {
  const on = document.activeElement as HTMLElement | null;
  const tag = on?.tagName;
  return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || !!on?.isContentEditable;
}

// The box over the workspace is asking something, see Confirm.svelte.
export function asking(): boolean {
  return !!document.querySelector("dialog[open]");
}

// The key is not the workspace's.
export function keysElsewhere(): boolean {
  return typing() || asking();
}

// A workspace that is not the one in front of the person takes no keys.
// Opening another episode builds its workspace behind the one on screen,
// and the two stand together until the new one is ready, see App.svelte.
// Both are inert meanwhile, and the space bar must not play either of
// them, nor an arrow walk the clips of the one going.
export function asleep(here: Element | null | undefined): boolean {
  return !here || !!here.closest("[inert]");
}
