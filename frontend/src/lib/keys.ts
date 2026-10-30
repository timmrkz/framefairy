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

// A key pressed alone: no Cmd, Ctrl or Alt, and not a key held down
// repeating. Shift only where withShift says so: a French or Belgian
// keyboard gives its digits only with shift, and a MacBook has no number pad.
export function pressedAlone(
  event: Pick<KeyboardEvent, "metaKey" | "ctrlKey" | "altKey" | "shiftKey" | "repeat">,
  withShift = false,
): boolean {
  if (event.metaKey || event.ctrlKey || event.altKey || event.repeat) return false;
  return withShift || !event.shiftKey;
}

// A key of the workspace's own, the one key a button stands for, L for
// loop, C for a keyframe, a number key for a share of the clip: pressed
// alone, not taken already, not a field's or the box's, and the workspace
// here is the one in front. Six keys said this each in their own four
// lines, and one of them, L, in other words again.
export function workspaceKey(event: KeyboardEvent, here: Element | null | undefined, withShift = false): boolean {
  return pressedAlone(event, withShift) && !event.defaultPrevented && !keysElsewhere() && !asleep(here);
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

// A key that moves the focus shows where it went. Where macOS would not
// move it, Tab to a button or an arrow along a row of them, the page moves
// it itself, and WebKit draws no ring on a focus a script moved when the
// focus before it came from a click, see Element::focus in WebKit. Tim
// clicked into the report, pressed Tab, and Report had the focus with no
// ring to show it. Chromium, which the walks drive, rings it anyway, so
// the move asks for the ring and does not leave it to the engine's guess.
export function keyFocus(to: HTMLElement) {
  to.focus({ focusVisible: true });
}
