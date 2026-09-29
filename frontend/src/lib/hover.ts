// A clip is shown in three places at once: its card in the clip list, its
// mark on the range picker and its mark or frame on the clip timeline. The
// hand on any of them lights all three, so the eye finds the same clip in
// the other two without reading anything.
import type { Attachment } from "svelte/attachments";

// Tells onhover when the pointer comes onto the element and goes off it.
// An element taken away from under the pointer never hears the pointer
// leave, so it says so itself as it goes, or the clip would stay lit in
// the other places until the hand touched another one.
export function hoverClip(
  key: string,
  onhover?: (key: string, on: boolean) => void,
): Attachment<HTMLElement> {
  return (node) => {
    if (!onhover) return;
    let inside = false;
    const enter = () => {
      inside = true;
      onhover(key, true);
    };
    const leave = () => {
      inside = false;
      onhover(key, false);
    };
    node.addEventListener("pointerenter", enter);
    node.addEventListener("pointerleave", leave);
    // Set up again under a pointer that is already on it, which happens
    // every time the clip is read anew, as it is again and again while a
    // search writes its clips: the one before said the pointer had gone,
    // and no pointerenter comes until the hand moves off and back, so the
    // clip went dark in the other two places under a hand that never
    // moved.
    if (node.matches(":hover")) enter();
    return () => {
      node.removeEventListener("pointerenter", enter);
      node.removeEventListener("pointerleave", leave);
      if (inside) onhover(key, false);
    };
  };
}
