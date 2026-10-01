<script lang="ts">
  import { untrack } from "svelte";

  // Work running in the control it was started from. Everything in the app
  // that runs wears this, so running work looks the same wherever it is:
  //
  //   the beam   light travelling clockwise round the edge of the control,
  //              for as long as the work runs
  //   the motes  specks of that light drifting up through the control,
  //              behind its own words
  //   the fill   how far the work has come, when that is known, with a
  //              light passing over what is done. Where the work cannot
  //              put a number on it, shuttle sends a short fill across
  //              it instead
  //
  // The control it sits in needs nothing of its own: app.css gives any
  // button holding a beam its rounded clip and a stacking context, so the
  // beam lies over the control's background and under its words.
  //
  // It is the one way work in hand is drawn, so it is used, never copied.
  // Where there is no edge to run round, the range picker, rim is off and
  // the motes and the fill are the same as everywhere else. Where there is
  // no room for the motes either, the bar in Activity is six pixels high,
  // motes is off too, and what is left is the fill, the same fill.
  //
  // Work that is paused is still, not gone. How far it got is as true as
  // it was, so the fill stays as it is, and what says the work is running,
  // the beam, the motes and the light over the fill, stops. The head keeps
  // its line and loses the glow thrown ahead of it, because nothing is
  // going ahead. So running and paused are told apart by movement, and a
  // paused piece of work reads as the same thing, stopped.
  //
  // The same fill tells time running out: given drain, a number of
  // seconds, it starts full and runs down to nothing over that time, and
  // says so with onend. There is no beam and no motes then, because
  // nothing is being worked on. It stands still while the pointer or the
  // keyboard is on the control, so nobody is rushed, and onresume tells
  // the time left each time it runs on. The control gives it its colour,
  // the same way it does for work, through --lit, --wash-from and
  // --wash-to. Given spent, it starts that many seconds in, for time that
  // ran on while it was not on screen.
  let {
    fraction = -1,
    rim = true,
    motes = true,
    shuttle = false,
    still = false,
    drain = 0,
    spent = 0,
    onend,
    onresume,
  }: {
    fraction?: number;
    rim?: boolean;
    motes?: boolean;
    shuttle?: boolean;
    still?: boolean;
    drain?: number;
    spent?: number;
    onend?: () => void;
    onresume?: (left: number) => void;
  } = $props();

  // The time running out, kept here and nowhere else. The clock is the
  // time: it decides when the time is over and how much is left, and the
  // fill is drawn from it. Held, the fill stands where the clock says, as
  // a plain transform with no animation at all. Let go, it is a new
  // animation from there to nothing over the time left, carried by the
  // compositor.
  //
  // An animation is never paused and played again. WebKit, which draws the
  // app on a Mac, plays a paused animation on from its timeline's last
  // frame rather than from now, so the fill jumped ahead as the pointer
  // left the row and back again as it came in: a fill that grew back
  // under the hand. Pausing it with a play state in the stylesheet did
  // the same and gave the time back as well. Seen in WebKitGTK 2.52 under
  // a pointer moved by XTest, and gone with this.
  function runDown(node: HTMLElement) {
    return untrack(() => {
      const host = node.closest(".beam")?.parentElement;
      const whole = drain * 1000;
      let left = Math.max(0, whole - spent * 1000);
      let since = performance.now();
      let timer = 0;
      let held = false;
      let fill: Animation | null = null;
      // Where the fill stands with this much time left.
      const at = () => `translateX(${((whole > 0 ? left / whole : 0) - 1) * 100}%)`;
      const stand = () => {
        fill?.cancel();
        fill = null;
        node.style.transform = at();
      };
      const run = () => {
        stand();
        fill = node.animate([{ transform: at() }, { transform: "translateX(-100%)" }], {
          duration: left,
          easing: "linear",
          fill: "forwards",
        });
      };
      const over = () => onend?.();
      const hold = () => {
        if (held) return;
        held = true;
        left = Math.max(0, left - (performance.now() - since));
        clearTimeout(timer);
        stand();
      };
      const go = () => {
        if (!held) return;
        held = false;
        since = performance.now();
        run();
        timer = window.setTimeout(over, left);
        onresume?.(left / 1000);
      };
      // Held while the pointer or the keyboard is anywhere on the control.
      const out = () => {
        if (!host?.matches(":hover") && !host?.contains(document.activeElement)) go();
      };
      const leftFocus = (e: FocusEvent) => {
        if (!host?.contains(e.relatedTarget as Node | null) && !host?.matches(":hover")) go();
      };
      run();
      timer = window.setTimeout(over, left);
      // A row that appears under the pointer, as a removed clip's row
      // does under the trash can just pressed, starts held.
      if (host?.matches(":hover")) hold();
      host?.addEventListener("pointerenter", hold);
      host?.addEventListener("pointerleave", out);
      host?.addEventListener("focusin", hold);
      host?.addEventListener("focusout", leftFocus);
      return () => {
        clearTimeout(timer);
        fill?.cancel();
        host?.removeEventListener("pointerenter", hold);
        host?.removeEventListener("pointerleave", out);
        host?.removeEventListener("focusin", hold);
        host?.removeEventListener("focusout", leftFocus);
      };
    });
  }

  // Where the motes rise and how long each one takes. Fixed rather than
  // drawn at random, because a random number would be a new one on every
  // render and the motes would jump about while the work ran.
  const specks = [
    { at: 14, wait: 0, over: 2.6, sway: 5 },
    { at: 33, wait: 900, over: 3.1, sway: -4 },
    { at: 52, wait: 400, over: 2.4, sway: 6 },
    { at: 71, wait: 1500, over: 2.9, sway: -6 },
    { at: 88, wait: 1100, over: 2.7, sway: 4 },
  ];
</script>

<span class="beam" class:still aria-hidden="true">
  {#if drain > 0}
    <span class="fill"><i class="run" {@attach runDown}></i></span>
  {/if}
  {#if rim && !still && drain <= 0}<span class="ring"></span>{/if}
  {#if motes && !still && drain <= 0}
    {#each specks as m (m.at)}
      <i
        class="mote"
        style="left: {m.at}%; animation-delay: {m.wait}ms; animation-duration: {m.over}s; --sway: {m.sway}px"
      ></i>
    {/each}
  {/if}
  {#if fraction >= 0}
    <!-- The fill is as wide as the control and slides in from the left,
         so it is moved and never laid out again: a width that changes is
         worked out on the main thread every frame, a transform is carried
         by the compositor, and what moves beside it, the head of the range
         picker's shade, moves the same way and stays with it. -->
    <span class="fill"
      ><i style="transform: translateX({(Math.min(Math.max(fraction, 0), 1) - 1) * 100}%)"
      ></i></span
    >
  {:else if shuttle}
    <span class="fill"><i class="seeking"></i></span>
  {/if}
</span>

<style>
  /* Behind the words of the control and over its background. The control
     makes the stacking context this is negative in, so nothing else on the
     page is disturbed by it. */
  .beam {
    position: absolute;
    inset: 0;
    z-index: -1;
    border-radius: inherit;
    overflow: hidden;
    pointer-events: none;
  }

  /* The beam. A comet of light turns behind the control and only the
     rim of it shows, so what is seen is a light running round the edge,
     clockwise, over and over.

     It is one square turning rather than a gradient whose angle is
     animated: an angle needs @property to animate at all, and a square
     turning is a transform, which the compositor does without painting
     anything again. */
  .ring {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    /* How thick the light is. The mask below keeps this rim and throws
       the middle away. */
    padding: 1px;
    -webkit-mask:
      linear-gradient(#000 0 0) content-box,
      linear-gradient(#000 0 0);
    -webkit-mask-composite: xor;
    mask:
      linear-gradient(#000 0 0) content-box,
      linear-gradient(#000 0 0);
    mask-composite: exclude;
    overflow: hidden;
  }

  /* Twice as wide as the control and square, so it covers every corner
     whatever shape the control is, and the same light reaches all four.
     There are two of these turning, the bright comet and a broad faint
     wash behind it, at five turns and three turns of the same round, so
     they are never in the same place twice inside a round. Both are short
     arcs of the circle: a wide button's long edge is nearly half the
     circle from the middle, so an arc of half the circle would light a
     whole edge at once and read as a border that is simply on. They
     are never in the same place twice inside a round, so the rim never
     shows the same picture twice, and the round is three times what one
     turn used to be. */
  .ring::before,
  .ring::after {
    content: "";
    position: absolute;
    top: 50%;
    left: 50%;
    width: 200%;
    aspect-ratio: 1;
    transform: translate(-50%, -50%);
    animation-duration: 6.5s;
    animation-iteration-count: infinite;
  }

  /* The comet. Five turns to the round, and no two of them alike: it
     gets away, eases right off through one long side, comes back hard
     through the next, sits down again and finishes quickly. Slowest to
     fastest is near three to one, which is enough to see without ever
     looking like a fault. It never stops and it never goes back: a light
     that hesitates on a border reads as broken and a light that backs up
     reads as a stutter, so the slowest part is still three fifths of
     the round's own pace. A plain turn for an engine that cannot read the
     curve, then the curve. */
  .ring::before {
    background: conic-gradient(
      from 0deg,
      transparent 0%,
      transparent 82%,
      var(--accent) 90%,
      var(--accent-hi) 96%,
      var(--lit, var(--accent-lit)) 99%,
      transparent 100%
    );
    animation-name: turn;
    animation-timing-function: linear;
    animation-timing-function: linear(
      0,
      0.073 7%,
      0.206 16%,
      0.294 26%,
      0.366 38%,
      0.43 46%,
      0.534 54%,
      0.681 63%,
      0.764 72%,
      0.823 82%,
      0.883 90%,
      0.956 96%,
      1
    );
  }

  /* The wash behind it. Three turns to the round, at one speed, against
     the comet's five, so the two meet at a different place every time and
     the pair only comes back to where it started once in a round. */
  .ring::after {
    background: conic-gradient(
      from 0deg,
      transparent 0%,
      transparent 40%,
      var(--accent) 52%,
      var(--accent) 58%,
      transparent 71%,
      transparent 100%
    );
    animation-name: lap;
    animation-timing-function: linear;
    opacity: 0.5;
  }

  @keyframes turn {
    from {
      transform: translate(-50%, -50%) rotate(0turn);
    }
    to {
      transform: translate(-50%, -50%) rotate(5turn);
    }
  }

  @keyframes lap {
    from {
      transform: translate(-50%, -50%) rotate(0turn);
    }
    to {
      transform: translate(-50%, -50%) rotate(3turn);
    }
  }

  /* The motes the beam sheds. A speck of light with no edge to it, so a
     fraction of a pixel has nothing to step, and it is gone before it
     reaches the top. */
  .mote {
    position: absolute;
    bottom: -3px;
    width: 3px;
    height: 3px;
    border-radius: 50%;
    background: var(--lit, var(--accent-lit));
    box-shadow: 0 0 4px var(--lit, var(--accent-lit));
    opacity: 0;
    animation-name: drift;
    animation-timing-function: cubic-bezier(0.35, 0, 0.5, 1);
    animation-iteration-count: infinite;
  }

  @keyframes drift {
    from {
      transform: translate(0, 0) scale(0.5);
      opacity: 0;
    }
    18% {
      opacity: 0.85;
    }
    to {
      transform: translate(var(--sway), calc(-1 * var(--control-h))) scale(1);
      opacity: 0;
    }
  }

  /* How far the work has come, inside the control: the app's colour with
     a bright head at the front. A wash rather than the colour itself,
     because the words of the control stand over it. The bar in Activity
     has no words over it, so its host sets the ends of the wash to the
     colour itself, see .progress in app.css. */
  .fill {
    position: absolute;
    inset: 0;
    border-radius: inherit;
    overflow: hidden;
  }

  .fill i {
    display: block;
    position: relative;
    width: 100%;
    height: 100%;
    overflow: hidden;
    /* Square, so the head is a straight line of light. A thin track that
       is round at its ends rounds its head to match, with --fill-round. */
    border-radius: var(--fill-round, 0);
    background: linear-gradient(
      90deg,
      var(--wash-from, var(--accent-fill)) 0%,
      var(--wash-to, var(--accent-fill-hi)) 100%
    );
    /* The head of the fill: a line of light with a glow thrown ahead of
       it, so where the work has got to is something to look at rather
       than the place one shade becomes another. */
    box-shadow:
      inset -2px 0 var(--lit, var(--accent-lit)),
      10px 0 14px -6px var(--lit, var(--accent-lit));
    /* How it follows the number. A control's work reports in steps, so it
       eases to each. The range picker's transcription reports about once
       a second and glides between reports, so it says so. */
    transition: transform var(--fill-glide, 0.25s cubic-bezier(0.4, 0, 0.2, 1));
  }

  /* The light passing over what is done, so what is done is alive to look
     at rather than a flat wash, and a fill never stands still while the
     work runs. A host can make it brighter with --sheen, where the fill
     is the solid colour and a faint light would not show. */
  .fill i::after {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      transparent 0%,
      rgba(255, 255, 255, var(--sheen, 0.16)) 50%,
      transparent 100%
    );
    animation: sheen 2.4s cubic-bezier(0.45, 0, 0.2, 1) infinite;
  }

  /* Work that cannot say how far it has come: a short fill shuttles across
     and stretches as it goes, so it is never a number that is missing, it
     is a thing in motion. It carries no light over it, because nothing
     behind it is done. */
  .fill i.seeking {
    width: 38%;
    transform-origin: left center;
    animation: shuttle 1.6s cubic-bezier(0.65, 0, 0.35, 1) infinite;
    transition: none;
  }

  .fill i.seeking::after {
    animation: none;
    opacity: 0;
  }

  @keyframes shuttle {
    0% {
      transform: translateX(-105%) scaleX(0.55);
    }
    50% {
      transform: translateX(75%) scaleX(1.35);
    }
    100% {
      transform: translateX(270%) scaleX(0.55);
    }
  }

  /* Time running out: the fill from full to nothing, carried by the
     compositor like every other fill, see runDown. */
  .fill i.run {
    transition: none;
  }

  /* The light over it runs the way the time does, right to left. Carried
     over from the fill of work, it ran left to right, against the fill
     it lay on. */
  .fill i.run::after {
    animation-direction: reverse;
  }

  /* Paused: the fill and the line at its head, and nothing that moves. */
  .still .fill i {
    box-shadow: inset -2px 0 var(--lit, var(--accent-lit));
  }

  .still .fill i::after {
    animation: none;
    opacity: 0;
  }

  /* Where the rim cannot be cut out of the square, there is no beam and
     the control wears a steady rim instead. Everything else stays. */
  @supports not ((mask-composite: exclude) or (-webkit-mask-composite: xor)) {
    .ring {
      display: none;
    }

    .beam {
      box-shadow: inset 0 0 0 1px var(--accent-hi);
    }
  }

  /* Nothing moves, and the beam is a steady rim of the app's colour, so a
     control with work in it still says so. */
  @media (prefers-reduced-motion: reduce) {
    .fill i::after {
      animation: none;
      opacity: 0;
    }

    .ring::before {
      animation: none;
      background: var(--accent-hi);
    }

    .ring::after {
      animation: none;
      background: none;
    }

    .mote {
      display: none;
    }

    /* Still work that cannot say how far it has come: the whole track,
       faint. */
    .fill i.seeking {
      animation: none;
      width: 100%;
      opacity: 0.4;
    }
  }
</style>
