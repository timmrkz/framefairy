<script lang="ts">
  // Work running in the control it was started from. Everything in the app
  // that runs wears this, so running work looks the same wherever it is:
  //
  //   the beam   light travelling clockwise round the edge of the control,
  //              for as long as the work runs
  //   the motes  specks of that light drifting up through the control,
  //              behind its own words
  //   the fill   how far the work has come, when that is known, with a
  //              light passing over what is done
  //
  // The control it sits in needs nothing of its own: app.css gives any
  // button holding a beam its rounded clip and a stacking context, so the
  // beam lies over the control's background and under its words.
  //
  // It is the one way work in hand is drawn, so it is used, never copied.
  // Where there is no edge to run round, the range picker, rim is off and
  // the motes and the fill are the same as everywhere else.
  let { fraction = -1, rim = true }: { fraction?: number; rim?: boolean } = $props();

  // Where the motes rise and how long each one takes. Fixed rather than
  // drawn at random, because a random number would be a new one on every
  // render and the motes would jump about while the work ran.
  const motes = [
    { at: 14, wait: 0, over: 2.6, sway: 5 },
    { at: 33, wait: 900, over: 3.1, sway: -4 },
    { at: 52, wait: 400, over: 2.4, sway: 6 },
    { at: 71, wait: 1500, over: 2.9, sway: -6 },
    { at: 88, wait: 1100, over: 2.7, sway: 4 },
  ];
</script>

<span class="beam" aria-hidden="true">
  {#if rim}<span class="ring"></span>{/if}
  {#each motes as m (m.at)}
    <i
      class="mote"
      style="left: {m.at}%; animation-delay: {m.wait}ms; animation-duration: {m.over}s; --sway: {m.sway}px"
    ></i>
  {/each}
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

  /* How far the work has come, inside the control. The same fill as the
     bar in app.css: the app's colour with a bright head at the front. A
     wash rather than the colour itself, because the words of the control
     stand over it. */
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

  /* The light passing over what is done, the same light the bar in
     Activity carries, so what is done is alive to look at rather than a
     flat wash, and a fill never stands still while the work runs. */
  .fill i::after {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      transparent 0%,
      rgba(255, 255, 255, 0.16) 50%,
      transparent 100%
    );
    animation: sheen 2.4s cubic-bezier(0.45, 0, 0.2, 1) infinite;
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
  }
</style>
