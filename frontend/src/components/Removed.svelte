<script lang="ts">
  import Busy from "./Busy.svelte";
  import Icon from "./Icon.svelte";
  import { secondThoughts } from "../lib/removed";

  // Something removed, in the place it had, saying so and offering itself
  // back for secondThoughts seconds, in the one fill the app has, running
  // down to nothing, see Busy. Then onend, and the place closes. A clip's
  // row in the clip list and a picture's tile in a report wear it, so the
  // two look the same. spent is how many seconds of the time had run when
  // it was drawn, and onresume says how many are left when it starts
  // again. Given together, every Removed of that name tells one time, see
  // Busy: a search's row and its cards are removed as one.
  //
  // It takes the place of what was removed in one frame and stands still:
  // the fill running down is its one movement. It slid in from the side
  // once, which across a tile read as a swipe to Tim, and in the clip list
  // slid in again every time its video was opened while it ran down,
  // because an animation starts over whenever its element is shown again,
  // issue #431.
  let {
    spent = 0,
    tile = false,
    together = "",
    onputback,
    onend,
    onresume,
  }: {
    spent?: number;
    tile?: boolean;
    together?: string;
    onputback?: () => void;
    onend?: () => void;
    onresume?: (left: number) => void;
  } = $props();
</script>

<div class="gone busyhost" class:filling={tile}>
  <Icon name="trash" />
  <span class="what">Removed</span>
  {#if !tile}<span class="grow"></span>{/if}
  <button class="quiet back" onclick={() => onputback?.()}>Put it back</button>
  <Busy drain={secondThoughts} {spent} {together} {onend} {onresume} />
</div>

<style>
  .gone {
    display: flex;
    align-items: center;
    gap: 8px;
    height: var(--card-h);
    padding: 0 8px;
    background: color-mix(in srgb, var(--err) 14%, transparent);
    color: var(--err);
  }

  /* In a tile it fills the tile, the words over each other in its
     middle. */
  .gone.filling {
    position: absolute;
    inset: 0;
    height: auto;
    flex-direction: column;
    justify-content: center;
    gap: 6px;
    border-radius: var(--radius-s);
    overflow: hidden;
  }

  /* The fill that runs down wears the colour of what was taken away,
     over the light app.css gives every control that holds a Busy. */
  .gone.busyhost {
    --lit: var(--err);
    --wash-from: color-mix(in srgb, var(--err) 12%, transparent);
    --wash-to: color-mix(in srgb, var(--err) 24%, transparent);
  }

  .gone .what {
    font-weight: 600;
  }

  .gone .grow {
    flex: 1;
  }

  .back {
    color: var(--text);
  }

</style>
