<script lang="ts">
  // Asks before something that cannot be taken back. A real modal, because
  // a click that throws work away has to be a click you meant.
  import type { Snippet } from "svelte";

  let {
    title,
    oncancel,
    children,
    actions,
  }: {
    title: string;
    oncancel: () => void;
    // What is about to happen, in plain words.
    children: Snippet;
    // The buttons, the one that does the thing last.
    actions: Snippet;
  } = $props();

  let panel: HTMLDialogElement;

  $effect(() => {
    if (!panel) return;
    panel.showModal();
    // The safe answer is where the keyboard starts.
    panel.querySelector("button")?.focus();
  });

  // macOS only tabs between buttons when full keyboard access is on, so the
  // box moves the focus itself. The arrow keys do the same, because the
  // buttons sit in a row and that is what a row of buttons looks like.
  function move(event: KeyboardEvent) {
    const steps: Record<string, number> = {
      ArrowLeft: -1,
      ArrowUp: -1,
      ArrowRight: 1,
      ArrowDown: 1,
      Tab: event.shiftKey ? -1 : 1,
    };
    const step = steps[event.key];
    if (!step) return;
    const buttons = [...panel.querySelectorAll("button")];
    if (buttons.length < 2) return;
    event.preventDefault();
    const here = buttons.indexOf(document.activeElement as HTMLButtonElement);
    const next = (here + step + buttons.length) % buttons.length;
    buttons[next].focus();
  }
</script>

<dialog bind:this={panel} onkeydown={move} oncancel={oncancel} onclose={oncancel}>
  <h2>{title}</h2>
  <div class="body">{@render children()}</div>
  <div class="row buttons">
    <span class="grow"></span>
    {@render actions()}
  </div>
</dialog>

<style>
  dialog {
    width: min(440px, calc(100vw - 48px));
    padding: 18px 20px 16px;
    border: 1px solid var(--line);
    border-radius: var(--radius-m);
    background: var(--ink-1);
    color: var(--text);
    box-shadow: 0 18px 48px rgba(0, 0, 0, 0.6);
  }

  dialog::backdrop {
    background: rgba(0, 0, 0, 0.55);
  }

  h2 {
    font-size: var(--size-l);
    font-weight: 600;
    margin: 0 0 8px;
  }

  .body {
    color: var(--muted);
    margin-bottom: 16px;
  }

  .buttons {
    gap: 8px;
  }

  .grow {
    flex: 1;
  }
</style>
