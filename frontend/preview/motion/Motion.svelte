<script lang="ts">
  // Every way the app says work is in hand, on one page, so the five can be
  // looked at side by side and compared without starting five jobs in the
  // app and catching each at the right moment.
  //
  // This is preview material. It is never built into the app: the product
  // only serves making shorts.
  import { onMount } from "svelte";
  import Busy from "../../src/components/Busy.svelte";
  import Icon from "../../src/components/Icon.svelte";

  let accent = $state("#942192");
  $effect(() => document.documentElement.style.setProperty("--accent", accent));

  // One share that keeps moving, so the fill can be watched filling rather
  // than read at a number somebody typed. It runs to the end and starts
  // over, the way a job does.
  let share = $state(0);
  onMount(() => {
    const timer = setInterval(() => (share = share >= 1 ? 0 : Math.min(share + 0.01, 1)), 70);
    return () => clearInterval(timer);
  });

  const ghosts = [0, 1, 2, 3, 4, 5];

  // The four ways a place waiting to be filled can say so. Three are other
  // people's, copied as they ship them, with the app's colours in place of
  // theirs. The fourth is ours.
  const ways = [
    {
      key: "sweep",
      name: "The sweep",
      from: "react-loading-skeleton",
      step: 800,
      what: "A highlight as wide as the place itself, soft at both ends, crossing once every second and a half and easing at each end. Wide is the point: a highlight with no edges to catch reads as the place brightening rather than as a stripe sliding past.",
    },
    {
      key: "wave",
      name: "The wave",
      from: "MUI Skeleton, animation=\"wave\"",
      step: 800,
      what: "The same idea with a rest in it: over in half the round, then still for the other half. Two seconds, at one speed.",
    },
    {
      key: "breath",
      name: "The breath",
      from: "shadcn/ui and Tailwind, animate-pulse",
      step: 800,
      what: "No light at all. The place itself dims to half and comes back, two seconds, in and out. What most of the web wears in 2026.",
    },
    {
      key: "both",
      name: "The breath and the wave",
      from: "the two above, together",
      step: 800,
      what: "The two above exactly as they are, at the same time. The only change to either is that the wave is half as wide again as the place it crosses, so it has no edges to catch.",
    },
    {
      key: "foil",
      name: "The foil",
      from: "ours, after simeydotme/pokemon-cards-css",
      step: 1600,
      what: "A grating of fine lines cut to the shape of a lens, wandering over the place with a white gleam going the other way. Two rounds that come back together once every ten minutes.",
    },
  ];

  // What wears what, in the app. The same table is in docs/APP.md.
  const map = [
    ["The beam", "New, Cancel, Pause, Render, Check again, and the pause mark in the clip list head"],
    ["The motes", "wherever the beam is, they are part of it"],
    ["The fill", "in New, in Pause and in Render while their work has a number, and as a track in Activity"],
    ["The shimmer", "the clips not found yet, the part of the clip timeline the transcript has not reached, the window on the range picker while a search runs"],
    ["The pulse", "the dot beside an episode in the sidebar, the dot on Activity on the rail, and the dot on a running job"],
  ];
</script>

<main>
  <header>
    <h1>Work in hand</h1>
    <p class="muted">
      The five ways the app says something is running. Everything here is the
      app's own <code>Busy.svelte</code> and the app's own
      <code>app.css</code>, so what is on this page is what is in the
      workspace.
    </p>
    <div class="row tools">
      <label class="row">
        <span>The app's colour</span>
        <input type="color" bind:value={accent} />
      </label>
      <span class="muted"
        >Everything below is mixed from it. For the still version, turn
        Reduce motion on in System Settings, Accessibility, Display.</span
      >
    </div>
  </header>

  <section>
    <h2>The beam, and the motes it sheds</h2>
    <p class="muted">
      Light runs clockwise round the edge of the control the work was started
      from, for as long as the work runs, and specks of that light drift up
      through the control behind its own words. It is not one turn at one
      speed: a round is five turns and no two of them alike, and a broad
      faint wash goes round three times in the same time, so the two are
      never in the same place twice. It never stands still and it never goes
      back, because a light that hesitates on a border reads as broken and
      one that backs up reads as a stutter: the slowest part is still
      three fifths of the round's own pace and the quickest is a little over
      one and a half. On a control that is already the app's colour the light
      is white, because the app's colour cannot be seen on itself.
    </p>
    <div class="row wrap">
      <button><Busy />Cancel</button>
      <button class="primary"><Busy />Render</button>
      <button class="on"><Busy />Looping</button>
      <button class="quiet"><Busy />Pausing</button>
      <button disabled><Busy />Cancelling</button>
      <button class="glyph"><Busy /><Icon name="pause" /></button>
      <button class="wide"><Busy />A button with a lot to say</button>
    </div>
  </section>

  <section>
    <h2>The fill, inside the control</h2>
    <p class="muted">
      How far the work has come, when that is known. A wash with a bright head
      at the front, so where the work has got to is a line rather than the
      place one shade becomes another. The left one runs on its own, the rest
      stand still at a share.
    </p>
    <div class="row wrap">
      <button class="wide"><Busy fraction={share} />Finding clips</button>
      <button><Busy fraction={0} />0%</button>
      <button><Busy fraction={0.25} />25%</button>
      <button><Busy fraction={0.6} />60%</button>
      <button><Busy fraction={1} />100%</button>
      <button class="primary"><Busy fraction={share} />Rendering</button>
    </div>
  </section>

  <section>
    <h2>The fill, as a track</h2>
    <p class="muted">
      The same fill where a job has no control of its own, which is Activity.
      A light travels over what is already done, so it lives even when the
      number stands still. Work that cannot say how far it has come shuttles
      across the track instead of standing at a number it does not have.
    </p>
    <div class="tracks">
      <div class="progress"><i style="width: {share * 100}%"></i></div>
      <div class="progress"><i style="width: 35%"></i></div>
      <div class="progress unknown"><i></i></div>
    </div>
  </section>

  <section>
    <h2>The shimmer, five ways</h2>
    <p class="muted">
      A place that is not filled yet. Three of these are what other people
      ship, copied faithfully and named, the fourth is two of those three
      together, and the fifth is ours. The breath is the one the app wears.
      They all stay here, where they cost the app nothing, so the choice
      can be looked at again. Pick one and it
      becomes the only one, everywhere a place waits: the clips not found
      yet, the part of the clip timeline the transcript has not reached, the
      window on the range picker while a search runs.
    </p>

    {#each ways as way (way.key)}
      <div class="way">
        <div class="row wayhead">
          <h3>{way.name}</h3>
          <span class="muted">{way.from}</span>
          {#if way.key === "breath"}<span class="now">in the app now</span>{/if}
        </div>
        <p class="muted small">{way.what}</p>
        <ol class="cards">
          {#each ghosts as row (row)}
            <li class="ghost {way.key}" style="--wait-in: {row * way.step}ms"></li>
          {/each}
        </ol>
      </div>
    {/each}
  </section>

  <section>
    <h2>The pulse</h2>
    <p class="muted">
      Work running somewhere else. The dot keeps its size and its place, and a
      ring widens out of it and fades, so a job running behind the window is
      seen from the corner of the eye and nothing moves for it.
    </p>
    <div class="row dots">
      <span class="dot busy"></span>
      <span class="muted">running</span>
      <span class="dot ok"></span>
      <span class="muted">finished</span>
      <span class="dot warn"></span>
      <span class="muted">out of date</span>
      <span class="dot err"></span>
      <span class="muted">missing</span>
      <span class="dot"></span>
      <span class="muted">nothing yet</span>
    </div>
  </section>

  <section>
    <h2>Where each one is worn</h2>
    <table>
      <tbody>
        {#each map as [what, where] (what)}
          <tr>
            <th>{what}</th>
            <td class="muted">{where}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </section>
</main>

<style>
  /* The app never scrolls. This page is a page, so it does. */
  :global(html),
  :global(body) {
    height: auto;
    overflow: auto;
  }

  main {
    max-width: 900px;
    margin: 0 auto;
    padding: var(--edge);
    display: flex;
    flex-direction: column;
    gap: calc(2 * var(--edge));
  }

  h1 {
    font-size: var(--size-xl);
  }

  h2 {
    font-size: var(--size-l);
  }

  header,
  section {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
  }

  p {
    max-width: 68ch;
  }

  code {
    color: var(--text);
  }

  .tools {
    gap: var(--gap);
    flex-wrap: wrap;
  }

  .tools input[type="color"] {
    width: 48px;
    padding: 2px;
  }

  .wrap {
    flex-wrap: wrap;
    gap: var(--gap);
  }

  .wide {
    min-width: 260px;
  }

  .glyph {
    display: flex;
    align-items: center;
    justify-content: center;
    width: var(--control-h);
    padding: 0;
  }

  .tracks {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
  }

  .way {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding-top: var(--gap);
    border-top: 1px solid var(--line);
  }

  .wayhead {
    gap: 10px;
  }

  h3 {
    font-size: var(--size-m);
  }

  .small {
    font-size: var(--size-s);
    max-width: 78ch;
  }

  .now {
    padding: 2px 8px;
    border-radius: var(--radius-s);
    background: var(--accent-wash);
    box-shadow: inset 0 0 0 1px var(--accent);
    font-size: var(--size-s);
  }

  /* Six cards of the size the clip list holds open, so every way is judged
     on the thing it will actually lie on. */
  .cards {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--gap);
  }

  .ghost {
    position: relative;
    height: 56px;
    border-radius: var(--radius-m);
    background: var(--ink-2);
    box-shadow: inset 0 0 0 1px var(--ink-3);
    overflow: hidden;
  }

  /* The sweep, react-loading-skeleton. This one is app.css's, so the card
     only needs the class the app gives it. */
  .ghost.sweep::after {
    content: "";
    position: absolute;
    inset: 0;
    background-repeat: no-repeat;
    background-image: linear-gradient(
      90deg,
      transparent 0%,
      var(--accent-faint) 22%,
      var(--accent-wash) 42%,
      rgba(255, 255, 255, 0.13) 50%,
      var(--accent-wash) 58%,
      var(--accent-faint) 78%,
      transparent 100%
    );
    transform: translateX(-100%);
    animation: sweep 1.5s ease-in-out infinite;
    animation-delay: var(--wait-in, 0ms);
  }

  @keyframes sweep {
    to {
      transform: translateX(100%);
    }
  }

  /* The wave, MUI Skeleton. Over in half the round, still for the rest. */
  .ghost.wave::after {
    content: "";
    position: absolute;
    inset: 0;
    background: linear-gradient(
      90deg,
      transparent,
      var(--accent-wash),
      transparent
    );
    transform: translateX(-100%);
    animation: wave 2s linear infinite;
    animation-delay: var(--wait-in, 0ms);
  }

  @keyframes wave {
    0% {
      transform: translateX(-100%);
    }
    50% {
      transform: translateX(100%);
    }
    100% {
      transform: translateX(100%);
    }
  }

  /* The breath, shadcn/ui and Tailwind. No light, the place itself dims. */
  .ghost.breath {
    animation: breath 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
    animation-delay: var(--wait-in, 0ms);
  }

  @keyframes breath {
    0%,
    100% {
      opacity: 1;
    }
    50% {
      opacity: 0.45;
    }
  }

  /* The two together, which is what the app wears. The wave is wider than
     the card, so it has no edges to catch. */
  .ghost.both {
    animation: breath 2s cubic-bezier(0.4, 0, 0.6, 1) infinite;
    animation-delay: var(--wait-in, 0ms);
  }

  .ghost.both::after {
    content: "";
    position: absolute;
    top: 0;
    bottom: 0;
    left: -25%;
    width: 150%;
    background: linear-gradient(
      90deg,
      transparent,
      var(--accent-wash),
      transparent
    );
    transform: translateX(-100%);
    animation: bothwave 2s linear infinite;
    animation-delay: var(--wait-in, 0ms);
  }

  @keyframes bothwave {
    0% {
      transform: translateX(-100%);
    }
    50% {
      transform: translateX(100%);
    }
    100% {
      transform: translateX(100%);
    }
  }

  /* The foil, ours. A grating cut to the shape of a lens, with a white
     gleam going the other way. */
  .ghost.foil {
    isolation: isolate;
    --foil-1: rgba(150, 80, 240, 0.34);
    --foil-2: rgba(240, 96, 165, 0.31);
    --foil-3: rgba(205, 88, 232, 0.27);
  }

  .ghost.foil::before,
  .ghost.foil::after {
    content: "";
    position: absolute;
    mix-blend-mode: screen;
    animation-iteration-count: infinite;
  }

  .ghost.foil::before {
    top: -160%;
    left: -110%;
    width: 320%;
    height: 420%;
    background-image: repeating-linear-gradient(
      100deg,
      transparent 0px,
      var(--foil-1) 2px,
      transparent 5px,
      var(--foil-3) 9px,
      transparent 13px,
      var(--foil-2) 18px,
      transparent 21px,
      rgba(255, 255, 255, 0.24) 26px,
      transparent 31px,
      var(--foil-2) 37px,
      transparent 40px,
      var(--foil-3) 46px,
      transparent 51px,
      var(--foil-1) 58px,
      transparent 61px,
      transparent 70px
    );
    -webkit-mask-image: radial-gradient(
      11% 26% at 50% 50%,
      #000 0%,
      rgba(0, 0, 0, 0.66) 42%,
      transparent 78%
    );
    -webkit-mask-repeat: no-repeat;
    mask-image: radial-gradient(
      11% 26% at 50% 50%,
      #000 0%,
      rgba(0, 0, 0, 0.66) 42%,
      transparent 78%
    );
    mask-repeat: no-repeat;
    transform: translate3d(-17%, -6%, 0) rotate(-7deg);
    animation-name: foil;
    animation-duration: 9.7s;
    animation-timing-function: cubic-bezier(0.42, 0, 0.35, 1);
    animation-delay: var(--wait-in, 0ms);
  }

  .ghost.foil::after {
    top: -90%;
    left: -80%;
    width: 260%;
    height: 300%;
    background-image: radial-gradient(
      4% 90% at 50% 50%,
      rgba(255, 255, 255, 0.32) 0%,
      rgba(255, 255, 255, 0.1) 44%,
      rgba(255, 255, 255, 0) 78%
    );
    background-repeat: no-repeat;
    transform: translate3d(12%, 6%, 0) rotate(11deg);
    animation-name: glare;
    animation-duration: 6.3s;
    animation-timing-function: cubic-bezier(0.45, 0, 0.3, 1);
    animation-delay: calc(var(--wait-in, 0ms) * 1.7);
  }

  @keyframes foil {
    0% {
      transform: translate3d(-17%, -6%, 0) rotate(-7deg);
    }
    27% {
      transform: translate3d(-3%, 5%, 0) rotate(4deg);
    }
    53% {
      transform: translate3d(15%, -5%, 0) rotate(11deg);
    }
    74% {
      transform: translate3d(2%, 6%, 0) rotate(2deg);
    }
    100% {
      transform: translate3d(-17%, -6%, 0) rotate(-7deg);
    }
  }

  @keyframes glare {
    0% {
      transform: translate3d(12%, 6%, 0) rotate(11deg) scale(1);
    }
    31% {
      transform: translate3d(-1%, -6%, 0) rotate(2deg) scale(1.2);
    }
    58% {
      transform: translate3d(-16%, 5%, 0) rotate(-9deg) scale(0.9);
    }
    81% {
      transform: translate3d(-3%, -2%, 0) rotate(-1deg) scale(1.1);
    }
    100% {
      transform: translate3d(12%, 6%, 0) rotate(11deg) scale(1);
    }
  }

  .dots {
    gap: var(--gap);
    flex-wrap: wrap;
  }

  table {
    border-collapse: collapse;
    text-align: left;
  }

  th,
  td {
    border-top: 1px solid var(--line);
    padding: 8px 12px 8px 0;
    vertical-align: top;
    font-weight: 600;
  }

  td {
    font-weight: 400;
  }

  /* The bench says what the app says, so nothing moves here either. Two
     classes, because every candidate above names itself with two and a
     media query adds no weight of its own. */
  @media (prefers-reduced-motion: reduce) {
    .cards .ghost,
    .cards .ghost::before,
    .cards .ghost::after {
      animation: none;
    }
  }
</style>
