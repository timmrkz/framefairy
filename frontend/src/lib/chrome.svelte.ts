import { api, onChrome, type Chrome, type ChromeWindow } from "./api";

// Where macOS put the title bar and its buttons. All zeros away from
// macOS, where the system draws its own title bar, and then the bar is
// an ordinary header and the stylesheet keeps its own numbers.
//
// Nothing here works a size out from a measurement of the page. This is
// the one thing the stylesheet cannot know, asked for once and told
// again when it changes, and handed over as four numbers it lays itself
// out from.
//
// The app's window and the report window have the same kind of title
// bar, so both lay their bar out from four numbers, see Bar.svelte, each
// from its own: the app's window can be fullscreen while the report
// window is not.
class TitleBar {
  chrome = $state<Chrome>({ bar: 0, left: 0, right: 0, middle: 0 });
  // The height is given whenever there is one. Where the buttons are is
  // given only when there are buttons, so that in native fullscreen, where
  // macOS takes them away, the stylesheet falls back to centring the name
  // in the bar rather than being told to centre it on nothing.
  style = $derived.by(() => {
    const c = this.chrome;
    const parts: string[] = [];
    if (c.bar > 0) parts.push(`--bar-h: ${c.bar}px`);
    if (c.right > 0) {
      parts.push(`--lights-l: ${c.left}px`);
      parts.push(`--lights-r: ${c.right}px`);
      parts.push(`--lights-y: ${c.middle}px`);
    }
    return parts.join("; ");
  });

  // Asks once for the window this page is in and follows every change
  // after, and gives what stops it.
  follow(window: ChromeWindow): () => void {
    api
      .chrome(window)
      .then((c) => (this.chrome = c))
      .catch(() => {});
    return onChrome(window, (c) => (this.chrome = c));
  }
}

export const titleBar = new TitleBar();
