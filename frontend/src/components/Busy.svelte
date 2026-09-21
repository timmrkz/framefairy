<script lang="ts">
  // Work running in the control it was started from. Everything in the app
  // that runs wears this, so running work looks the same wherever it is:
  //
  //   the beam   light travelling clockwise round the edge of the control,
  //              for as long as the work runs
  //   the motes  specks of that light drifting up through the control,
  //              behind its own words
  //   the fill   how far the work has come, when that is known
  //
  // The control it sits in needs nothing of its own: app.css gives any
  // button holding a beam its rounded clip and a stacking context, so the
  // beam lies over the control's background and under its words.
  let { fraction = -1 }: { fraction?: number } = $props();

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
  <span class="ring"></span>
  {#each motes as m (m.at)}
    <i
      class="mote"
      style="left: {m.at}%; animation-delay: {m.wait}ms; animation-duration: {m.over}s; --sway: {m.sway}px"
    ></i>
  {/each}
  {#if fraction >= 0}
    <span class="fill"><i style="width: {Math.min(fraction, 1) * 100}%"></i></span>
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
     wash behind it, at three turns and two turns of the same round. Both
     are short arcs of the circle: a wide button's long edge is nearly half
     the circle from the middle, so an arc of half the circle would light
     a whole edge at once and read as a border that is simply on. They
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
    animation-duration: 3.9s;
    animation-iteration-count: infinite;
  }

  /* The comet. Three turns to the round, and not at one speed: it gets
     away quickly, drifts through the long side, tugs back twice and
     catches up at the end, so the slow part falls somewhere else on the
     edge each time round. A plain turn for an engine that cannot read the
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
      0.065 5%,
      0.145 13%,
      0.208 22%,
      0.252 30%,
      0.329 37%,
      0.385 41%,
      0.37 44%,
      0.436 50%,
      0.527 57%,
      0.583 64%,
      0.618 71%,
      0.696 77%,
      0.756 81%,
      0.738 84%,
      0.822 90%,
      0.912 95%,
      1
    );
  }

  /* The wash behind it. Two turns to the round, at one speed, so it and
     the comet meet at a different place every time. */
  .ring::after {
    background: conic-gradient(
      from 0deg,
      transparent 0%,
      transparent 42%,
      var(--accent-wash) 53%,
      var(--accent-wash) 59%,
      transparent 70%,
      transparent 100%
    );
    animation-name: lap;
    animation-timing-function: linear;
  }

  @keyframes turn {
    from {
      transform: translate(-50%, -50%) rotate(0turn);
    }
    to {
      transform: translate(-50%, -50%) rotate(3turn);
    }
  }

  @keyframes lap {
    from {
      transform: translate(-50%, -50%) rotate(0turn);
    }
    to {
      transform: translate(-50%, -50%) rotate(2turn);
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
    height: 100%;
    background: linear-gradient(
      90deg,
      var(--wash-from, var(--accent-faint)) 0%,
      var(--wash-to, var(--accent-wash)) 100%
    );
    /* The head of the fill: a line of light with a glow thrown ahead of
       it, so where the work has got to is something to look at rather
       than the place one shade becomes another. */
    box-shadow:
      inset -2px 0 var(--lit, var(--accent-lit)),
      10px 0 14px -6px var(--lit, var(--accent-lit));
    transition: width 0.25s cubic-bezier(0.4, 0, 0.2, 1);
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
