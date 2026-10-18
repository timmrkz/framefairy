import { said } from "./said";

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
// them, nor an arrow walk the clips of the one going. A workspace kept
// for later is out of the page altogether, and listens all the same.
export function asleep(here: Element | null | undefined): boolean {
  return !here || !here.isConnected || !!here.closest("[inert]");
}

// Escape lets go of the field being typed in, the way it closes a list,
// so the ring goes and the workspace has its keys again. What is typed
// stays, as it does when the field is clicked away from: a report lost
// to an Escape is what issue #355 was. Heard last, so a field or a layer
// that takes Escape for itself, a word being corrected, a list, a pop-up,
// the report while it listens, says so with preventDefault and keeps it.
// Issue #370.
export function escapeLetsGo(e: KeyboardEvent) {
  if (e.key !== "Escape" || e.defaultPrevented || e.isComposing || !typing()) return;
  const field = document.activeElement as HTMLElement;
  field.blur();
  said(`keys: Escape let go of ${field.getAttribute("aria-label") || field.id || field.tagName.toLowerCase()}`);
}
