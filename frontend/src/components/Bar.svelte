<script lang="ts">
  // The bar across the top of a window of the app, see the comment on the
  // markup. The app's window and the report window both wear it, inside a
  // .shell that carries titleBar.style, see lib/chrome.svelte.ts.
  import type { Label } from "../lib/api";

  // The label of the video on screen is its pill beside its name, the one
  // the sidebar shows, so the two say the same thing.
  let { title, label = null }: { title: string; label?: Label | null } = $props();
</script>

<!-- The bar across the top: the close, minimise and zoom buttons, and
     the name of what is on screen beside them. It is the whole width of
     the app, so the sidebar opens under it and never covers the name. -->
<!-- The bar is the title bar. macOS lays that out and puts its three
     buttons on its middle, and the app takes the height it was given
     rather than asking for one.
     This used to say that nothing the app can set moves those buttons,
     which is false and cost a great deal: Electron apps move them and
     VS Code does, and chrome_darwin.go already holds the handle, since
     it asks the app's window for standardWindowButton: and reads the
     frame.
     What was really wrong was asking for a toolbar, which makes the bar
     taller and makes macOS inset the buttons to centre them in it. See
     the WebviewWindowOptions in main.go.
     Then the buttons are on the bar's middle because the bar is what
     they were centred in, so the name is centred in the bar too. It once
     went in a box on the middle of the close button's frame, which
     AppKit makes taller than the circle it draws, and the name and its
     label sat a point above the circles, #399. Where the system draws
     its own title bar there is nothing to agree with and the stylesheet
     keeps its own numbers. -->
<header class="bar">
  <span class="name"
    ><span class="what">{title}</span>{#if label}<span class="tag" style="--tag: {label.colour}">{label.text}</span>{/if}</span
  >
</header>

<style>
  /* The bar belongs to the app as a whole: it is what you drag the app
     by, it holds the close, minimise and zoom buttons on macOS, and it
     says what is on screen. */
  /* Exactly as tall as the title bar macOS laid out, which the shell sets
     from what macOS answers, or the token where the system draws its own
     bar. The line at the foot is drawn inside that height rather than
     under it, so the bar is the title bar and nothing else.
     The line was taken out once and put back. Read off VS Code and
     Terminal: both draw one under their title bar, and VS Code's is
     brighter against its own bar than ours is against ours. It was never
     what made the app look unlike theirs. */
  .bar {
    position: relative;
    flex: none;
    height: var(--bar-h);
    box-shadow: inset 0 -1px var(--line);
    background: var(--ink-1);
    --wails-draggable: drag;
  }

  /* The name stands in a box as tall as the bar, so it is centred on the
     bar's middle, which is the buttons' middle. It starts one space after
     the last of them. Nothing is moved by a transform, so nothing is put
     on a compositing surface of its own and the whole pixel rule holds. */
  .bar .name {
    position: absolute;
    top: 0;
    left: calc(var(--lights-r, 0px) + var(--gap));
    right: var(--gap);
    height: var(--bar-h);
    display: flex;
    align-items: center;
    gap: 8px;
    overflow: hidden;
  }

  /* The name itself. Two things have to be true of it at once and they
     pull against each other.

     A line box keeps room below the baseline for the letters that go
     there and room above the capitals that no capital uses, and there is
     more of the first than of the second, so a name centred by its line
     box reads high. Trimming the box to the capitals and the baseline is
     what puts the letters themselves on the middle.

     But the box that is trimmed is also the box that is clipped, and the
     name is clipped so a long one ends in an ellipsis rather than running
     under the sidebar. Trimmed and clipped together is what cut the tail
     off the y in YouTube.mp4.

     So the box is trimmed to the capitals and then padded back out. The
     padding is even, so the middle of the box is still the middle of the
     capitals, and it is deeper than any letter goes, so nothing is cut.
     Clipping happens at the padding, which is the whole of the name
     again. */
  .bar .what {
    min-width: 0;
    font-size: var(--size-m);
    font-weight: 600;
    line-height: 1;
    text-box: trim-both cap alphabetic;
    padding: 6px 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Where the browser cannot trim a text box there is nothing to pad back
     out, and a plain line box centred on the buttons is a small fraction
     high rather than wrong. */
  @supports not (text-box: trim-both cap alphabetic) {
    .bar .what {
      line-height: normal;
      padding: 0;
    }
  }

  /* Where the system draws its own title bar the name keeps the left of
     the bar, because there is nothing at the left to make room for. */
  :global(.shell:not([style*="--lights-r"])) .bar .name {
    left: var(--gap);
  }

</style>
