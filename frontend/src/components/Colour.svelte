<script lang="ts">
  // A colour to pick, wearing the app's own clothes.
  //
  // A plain colour field is drawn by the system: on macOS the webview hands
  // the click to AppKit, which opens a pop-up of its own with a grid of
  // colours and a button to the colour panel. Nothing in the stylesheet
  // reaches it, it looks like another program, and it never says which of
  // its colours is the one in use. It is the same story as the <select>
  // that became Pick.svelte, and the same answer: the pop-up is the app's
  // own, on bits-ui, which brings the focus, the keyboard and the floating
  // placement and no look at all.
  //
  // In it, from the quickest pick to the most exact: the colours offered
  // first, with a ring on the one in use, a square for the shade and a
  // strip for the hue for any other, the colour as hex for one that is
  // known by its number, and the pipette, which takes a colour from the
  // video preview. Everything the hand does is shown while it does it, and
  // saved when it lets go.
  //
  // This is the one colour picker in the app.
  import { untrack } from "svelte";
  import { Popover } from "bits-ui";
  import Icon from "./Icon.svelte";
  import { hexToHSV, hsvToHex, readHex, type HSV } from "../lib/colour";

  let {
    value,
    label,
    title = "",
    presets = [],
    oninput = undefined,
    onchange,
    onsample = undefined,
    face = "field",
    on = false,
    disabled = false,
  }: {
    // The colour in use, as #rrggbb. Like Pick's value it goes one way
    // only: what the trigger shows is what the caller says is true.
    value: string;
    // What the colour is for, said out loud for anything reading the
    // screen.
    label: string;
    title?: string;
    // The colours offered first, a ring on whichever is in use.
    presets?: { hex: string; name: string }[];
    // A colour on its way, while the hand is still moving, to be drawn and
    // not yet saved. Nothing means what is saved, after a pick from the
    // video preview was left.
    oninput?: (hex: string | null) => void;
    // A colour picked, to be saved.
    onchange: (hex: string) => void;
    // Where there is a video preview to take a colour from, the way to
    // ask it for one: over is told the colour under the pointer as it
    // moves, or nothing when it is off the picture, and done the colour
    // clicked, or nothing when the pick was left.
    onsample?: (over: (hex: string | null) => void, done: (hex: string | null) => void) => void;
    // A square field as tall as a row, for a column of settings, or the
    // round well at the end of a row of round colours, the way the Mac
    // ends its row of accent colours with a wheel.
    face?: "field" | "well";
    // Whether the well holds the colour in use, rather than one of the
    // round colours before it.
    on?: boolean;
    disabled?: boolean;
  } = $props();

  let open = $state(false);
  // Taking a colour from the video preview. The pop-up closes so the
  // picture can be seen, and the trigger wears a ring in the meantime, so
  // it is clear which colour the picture is giving.
  let sampling = $state(false);

  const hex = $derived(readHex(value) ?? "#ffffff");

  // Where the square and the strip stand. It follows the colour, unless it
  // already says it: a grey has no hue of its own, and a strip that jumped
  // back to red every time the square was dragged to grey would lose the
  // hue that was picked.
  let hsv = $state<HSV>({ h: 0, s: 0, v: 1 });
  $effect(() => {
    const want = hex;
    if (untrack(() => hsvToHex(hsv)) !== want) hsv = hexToHSV(want);
  });

  // What the hex field says while it is typed in. Nothing means it shows
  // the colour in use.
  let typed = $state<string | null>(null);

  // Whether the pop-up was left with the pointer. The keyboard goes back
  // to the trigger only when it was left with the keyboard: a colour
  // picked with the pointer lets go of the keyboard, the way every other
  // field in the column does, so Shift and the arrows walk the words again
  // and Enter opens the word rather than this pop-up a second time.
  let byPointer = false;

  function draw(next: HSV) {
    hsv = next;
    oninput?.(hsvToHex(next));
  }

  function pick(colour: string) {
    hsv = hexToHSV(colour);
    typed = null;
    oninput?.(colour);
    onchange(colour);
  }

  const clamp = (n: number) => Math.min(1, Math.max(0, n));

  // The square and the strip follow the hand from the moment it presses,
  // and what they stand on when it lets go is saved.
  function drag(event: PointerEvent, at: (x: number, y: number) => HSV) {
    if (event.button !== 0) return;
    const el = event.currentTarget as HTMLElement;
    el.setPointerCapture(event.pointerId);
    el.focus();
    const follow = (e: PointerEvent) => {
      const r = el.getBoundingClientRect();
      draw(at(clamp((e.clientX - r.left) / r.width), clamp((e.clientY - r.top) / r.height)));
    };
    follow(event);
    const up = () => {
      el.removeEventListener("pointermove", follow);
      el.removeEventListener("pointerup", up);
      el.removeEventListener("pointercancel", up);
      typed = null;
      onchange(hsvToHex(hsv));
    };
    el.addEventListener("pointermove", follow);
    el.addEventListener("pointerup", up);
    el.addEventListener("pointercancel", up);
  }

  // The arrows move them too, in small steps or with Shift in large ones,
  // and the colour is saved once the key is let go.
  const steps: Record<string, [number, number]> = {
    ArrowLeft: [-1, 0],
    ArrowRight: [1, 0],
    ArrowUp: [0, 1],
    ArrowDown: [0, -1],
  };
  function squareKey(event: KeyboardEvent) {
    const step = steps[event.key];
    if (!step) return;
    event.preventDefault();
    const by = event.shiftKey ? 0.1 : 0.02;
    draw({ h: hsv.h, s: clamp(hsv.s + step[0] * by), v: clamp(hsv.v + step[1] * by) });
  }
  function hueKey(event: KeyboardEvent) {
    const step = steps[event.key];
    if (!step) return;
    event.preventDefault();
    const by = event.shiftKey ? 15 : 2;
    draw({ ...hsv, h: Math.min(360, Math.max(0, hsv.h + (step[0] || step[1]) * by)) });
  }
  function keyUp(event: KeyboardEvent) {
    if (steps[event.key]) onchange(hsvToHex(hsv));
  }

  // The colours offered first are one group the arrows walk, the way a row
  // of radio buttons is walked.
  function presetKey(event: KeyboardEvent, i: number) {
    const by = event.key === "ArrowRight" || event.key === "ArrowDown" ? 1 : event.key === "ArrowLeft" || event.key === "ArrowUp" ? -1 : 0;
    if (!by) return;
    event.preventDefault();
    const row = (event.currentTarget as HTMLElement).parentElement;
    const next = row?.children[(i + by + presets.length) % presets.length] as HTMLElement | undefined;
    next?.focus();
  }

  // The hex field is drawn as it is typed, once it is a colour, and saved
  // when it is left or Enter is pressed. Anything that is not a colour
  // goes back to the colour in use.
  function typeHex(text: string) {
    typed = text;
    const colour = readHex(text);
    if (colour && text.replace(/^#/, "").length === 6) {
      hsv = hexToHSV(colour);
      oninput?.(colour);
    }
  }
  // What was typed is saved even when it is the colour shown: the colour
  // shown is the one being typed, drawn on the way, and not yet saved.
  function leaveHex() {
    if (typed === null) return;
    const colour = readHex(typed);
    typed = null;
    if (colour) pick(colour);
    else oninput?.(null);
  }

  function sample() {
    if (!onsample) return;
    byPointer = true;
    open = false;
    sampling = true;
    onsample(
      (colour) => oninput?.(colour),
      (colour) => {
        sampling = false;
        if (colour) pick(colour);
        else oninput?.(null);
      },
    );
  }

  // The shortcuts of the workspace stand aside while the pop-up has the
  // keyboard: an arrow walking the colours here is not an arrow walking
  // the words of the caption or the playhead. Escape still reaches the
  // pop-up's own layer, which closes it.
  function keepKeys(event: KeyboardEvent) {
    if (event.key !== "Escape") event.stopPropagation();
  }
</script>

<Popover.Root
  bind:open
  onOpenChange={(now) => {
    if (now) byPointer = false;
    else leaveHex();
  }}
>
  <Popover.Trigger {disabled}>
    {#snippet child({ props })}
      <button
        {...props}
        class="colour"
        class:well={face === "well"}
        class:on
        class:sampling
        aria-label={label}
        {title}
        style="--colour: {hex}"
      >
        {#if face === "field"}<span class="chip"></span>{/if}
      </button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Portal>
    <Popover.Content
      sideOffset={4}
      align="start"
      onInteractOutside={() => (byPointer = true)}
      onCloseAutoFocus={(e) => {
        if (!byPointer) return;
        e.preventDefault();
        (document.activeElement as HTMLElement | null)?.blur?.();
      }}
    >
      {#snippet child({ props, wrapperProps, open: shown })}
        {#if shown}
          <div {...wrapperProps}>
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div {...props} class="colour-pop" aria-label={label} onkeydown={keepKeys}>
              {#if presets.length}
                <div class="presets" role="radiogroup" aria-label="Colours">
                  {#each presets as p, i (p.hex)}
                    {@const chosen = p.hex === hex}
                    <button
                      class="preset"
                      class:chosen
                      role="radio"
                      aria-checked={chosen}
                      aria-label={p.name}
                      title={p.name}
                      tabindex={chosen || (i === 0 && !presets.some((q) => q.hex === hex)) ? 0 : -1}
                      style="--colour: {p.hex}"
                      onclick={(e) => {
                        if (e.detail > 0) byPointer = true;
                        pick(p.hex);
                      }}
                      onkeydown={(e) => presetKey(e, i)}
                    ></button>
                  {/each}
                </div>
              {/if}
              <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
              <div
                class="square"
                role="slider"
                tabindex="0"
                aria-label="Shade"
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={Math.round(hsv.s * 100)}
                aria-valuetext="{Math.round(hsv.s * 100)}% colour, {Math.round(hsv.v * 100)}% light"
                title="Drag for the shade: colour to the right, light to the top"
                style="--hue: {hsvToHex({ h: hsv.h, s: 1, v: 1 })}"
                onpointerdown={(e) => drag(e, (x, y) => ({ h: hsv.h, s: x, v: 1 - y }))}
                onkeydown={squareKey}
                onkeyup={keyUp}
              >
                <span
                  class="knob"
                  style="left: {hsv.s * 100}%; top: {(1 - hsv.v) * 100}%; --colour: {hex}"
                ></span>
              </div>
              <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
              <div
                class="hue"
                role="slider"
                tabindex="0"
                aria-label="Hue"
                aria-valuemin={0}
                aria-valuemax={360}
                aria-valuenow={Math.round(hsv.h)}
                title="Drag for the hue"
                onpointerdown={(e) => drag(e, (x) => ({ ...hsv, h: x * 360 }))}
                onkeydown={hueKey}
                onkeyup={keyUp}
              >
                <span
                  class="knob"
                  style="left: {(hsv.h / 360) * 100}%; --colour: {hsvToHex({ h: hsv.h, s: 1, v: 1 })}"
                ></span>
              </div>
              <div class="row">
                {#if onsample}
                  <button
                    class="sample"
                    title="Take a colour from the video preview. Click the picture to take it, Escape to leave it as it was"
                    aria-label="Take a colour from the video preview"
                    onclick={sample}><Icon name="pipette" /></button
                  >
                {/if}
                <label class="hex" title="The colour as hex, the way a brand colour is written">
                  <span class="hash">#</span>
                  <input
                    type="text"
                    spellcheck="false"
                    autocomplete="off"
                    maxlength="7"
                    aria-label="Hex"
                    value={typed ?? hex.slice(1).toUpperCase()}
                    oninput={(e) => typeHex(e.currentTarget.value)}
                    onkeydown={(e) => {
                      if (e.key === "Enter") {
                        e.preventDefault();
                        leaveHex();
                      }
                    }}
                    onblur={leaveHex}
                  />
                </label>
              </div>
            </div>
          </div>
        {/if}
      {/snippet}
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>

<style>
  /* The field: the colour and nothing else, in a square as tall as the
     row, so the number beside it keeps the room it needs. */
  .colour {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    box-sizing: border-box;
    width: var(--control-h);
    height: var(--control-h);
    padding: 3px;
  }

  .chip {
    display: block;
    width: 100%;
    height: 100%;
    border-radius: calc(var(--radius-s) - 1px);
    background: var(--colour);
    /* A colour as dark as the field still shows where it ends. */
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12);
  }

  /* The same ring every field wears when it has the keyboard, drawn inside
     the box, and while the pop-up is open, or the video preview is giving
     this colour, the ring says which colour is being picked. */
  .colour:focus-visible,
  .colour[aria-expanded="true"],
  .colour.sampling {
    outline: 2px solid var(--accent-hi);
    outline-offset: -1px;
  }

  /* The well at the end of a row of round colours: the wheel the Mac draws
     for any other colour, with the ring the chosen one wears when it holds
     the colour in use. */
  .colour.well {
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: conic-gradient(#e0605a, #d9b83c, #5fa37a, #3c9fd9, #8a5cd9, #d95cb5, #e0605a);
  }

  .colour.well:hover:not(:disabled) {
    background: conic-gradient(#e0605a, #d9b83c, #5fa37a, #3c9fd9, #8a5cd9, #d95cb5, #e0605a);
    filter: brightness(1.15);
  }

  .colour.well.on {
    outline: none;
    box-shadow:
      0 0 0 2px var(--ink-1),
      0 0 0 4px var(--colour);
  }

  .colour.well:focus-visible,
  .colour.well[aria-expanded="true"] {
    outline: 2px solid var(--accent-hi);
    outline-offset: 3px;
  }

  /* The pop-up lies over the workspace, so it wears the panel colour the
     lists wear, with their border, corners and shadow. */
  .colour-pop {
    z-index: 60;
    display: flex;
    flex-direction: column;
    gap: 12px;
    width: 232px;
    box-sizing: border-box;
    padding: 12px;
    color: var(--text);
    background: var(--ink-1);
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.55);
  }

  /* The colours offered first, round, the way the Mac shows a row of
     colours, and the one in use with the ring the accent colours in the
     settings wear. */
  .presets {
    display: flex;
    justify-content: space-between;
  }

  .preset {
    width: 18px;
    height: 18px;
    padding: 0;
    border: none;
    border-radius: 50%;
    background: var(--colour);
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.16);
  }

  .preset:hover:not(:disabled) {
    background: var(--colour);
    filter: brightness(1.15);
  }

  .preset.chosen {
    box-shadow:
      inset 0 0 0 1px rgba(255, 255, 255, 0.16),
      0 0 0 2px var(--ink-1),
      0 0 0 4px var(--text);
  }

  .preset:focus-visible {
    outline: 2px solid var(--accent-hi);
    outline-offset: 3px;
  }

  /* The shade: the hue from white at the left to the full colour at the
     right, and black coming up from the bottom over both. */
  .square {
    position: relative;
    height: 128px;
    border-radius: var(--radius-s);
    background:
      linear-gradient(to top, #000, transparent),
      linear-gradient(to right, #fff, var(--hue));
    cursor: crosshair;
    touch-action: none;
  }

  .hue {
    position: relative;
    height: 12px;
    border-radius: 6px;
    background: linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00);
    cursor: pointer;
    touch-action: none;
  }

  .square:focus-visible,
  .hue:focus-visible {
    outline: 2px solid var(--accent-hi);
    outline-offset: 2px;
  }

  /* Where the square or the strip stands: a ring in white with a shadow,
     so it is seen on any colour, filled with the colour it stands for. */
  .knob {
    position: absolute;
    width: 14px;
    height: 14px;
    margin: -7px 0 0 -7px;
    border-radius: 50%;
    background: var(--colour);
    box-shadow:
      0 0 0 2px #fff,
      0 1px 4px 2px rgba(0, 0, 0, 0.45);
    pointer-events: none;
  }

  .hue .knob {
    top: 50%;
  }

  .row {
    display: flex;
    gap: 8px;
  }

  .sample {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: var(--control-h);
    padding: 0;
    color: var(--muted);
  }

  .sample:hover:not(:disabled) {
    color: var(--text);
  }

  /* The hex as one field with its # in front, the way a unit stands
     beside a number, in the colour of a unit. */
  .hex {
    display: flex;
    align-items: center;
    flex: 1;
    min-width: 0;
    box-sizing: border-box;
    height: var(--control-h);
    padding: 0 0 0 10px;
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    background: var(--ink-2);
  }

  .hex:focus-within {
    outline: 2px solid var(--accent-hi);
    outline-offset: -1px;
  }

  .hash {
    color: var(--muted);
  }

  .hex input {
    flex: 1;
    min-width: 0;
    height: 100%;
    padding: 0 8px 0 2px;
    border: none;
    background: transparent;
    font-variant-numeric: tabular-nums;
    text-transform: uppercase;
    outline: none;
  }
</style>
