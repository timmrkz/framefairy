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

  // What wears what, in the app. The same table is in docs/APP.md.
  const map = [
    ["The beam", "New, Cancel, Pause, Render, Check again, and the pause mark in the clip list head"],
    ["The motes", "wherever the beam is, they are part of it"],
    ["The fill", "in New, in Pause and in Render while their work has a number, and as a track in Activity"],
    ["The shimmer", "the clips not found yet, the part of the clip timeline the transcript has not reached, the stretch on the range picker while a search runs"],
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
      one that backs up reads as a stutter: the slowest stretch is still
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
    <h2>The shimmer</h2>
    <p class="muted">
      A place that is not filled yet, lit like a card of foil tipped against
      the light. Two things make foil foil. It is a grating, many fine lines
      close together each throwing back a slightly different colour, so what
      you see is lines and not a cloud. And the colour lives inside the
      reflection: lay the lines over a whole surface and you get brushed
      metal, lit everywhere and going nowhere. So a sheet three times the
      size of the place carries the lines, a soft lens is cut out of it, and
      what is seen is that lens wandering over a dark surface carrying its
      lines with it. A second sheet is one narrow white gleam going the other
      way, the way the edge of a card catches the sun before its face does.
      Their rounds are 9.7 and 6.3 seconds and come back together once every
      ten minutes, and in a list each row is a step further into the round
      than the one above, so every card is tipped at its own angle.
    </p>
    <div class="shim">
      <ol>
        {#each ghosts as row (row)}
          <li class="ghost waiting" style="--wait-in: {row * 1600}ms"></li>
        {/each}
      </ol>
      <div class="blocks">
        <div class="block wide-block waiting" style="--wait-in: 3200ms"></div>
        <div class="block waiting" style="--wait-in: 6400ms"></div>
      </div>
    </div>
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

  .shim {
    display: flex;
    gap: var(--gap);
    align-items: flex-start;
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    width: 280px;
  }

  /* The same card the clip list holds open for a clip on its way. */
  .ghost {
    height: 56px;
    border-radius: var(--radius-m);
    background: var(--ink-2);
    box-shadow: inset 0 0 0 1px var(--ink-3);
  }

  .blocks {
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    flex: 1;
  }

  .block {
    height: 112px;
    border-radius: var(--radius-m);
    background: var(--ink-1);
    border: 1px solid var(--line);
  }

  .wide-block {
    height: 56px;
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
</style>
