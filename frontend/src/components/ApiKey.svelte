<script lang="ts">
  // The row where a company's API key is typed, in the settings and on the
  // first run of a machine that can hold no clip model of its own. One row,
  // used in both, so a key is saved, refused and removed the same way
  // wherever it is asked for.
  //
  // The key goes in the keychain the moment it is saved, not with the rest
  // of the settings, because it never lands in the settings file. An app
  // opened from Finder has no shell environment, so this is the only way to
  // give it one. The Go side asks the company whether it takes the key
  // before keeping it, so a key it refuses is found here, where it was
  // typed.
  import { api, errorText, type Provider } from "../lib/api";
  import Busy from "./Busy.svelte";
  import Confirm from "./Confirm.svelte";
  import Icon from "./Icon.svelte";

  let {
    provider,
    source,
    hint,
    refuse,
    onchange,
  }: {
    provider: Provider;
    // Where the key is found, "keychain" or "environment", or nothing.
    source: string;
    // The key in short, as the Go side read it without the secret.
    hint: string;
    // Says no, and says why where it can, while the key cannot be changed,
    // a search running on it among the reasons.
    refuse?: () => boolean;
    // Once a key was saved or removed, so the page reads what is here.
    onchange?: () => unknown;
  } = $props();

  const hasKey = $derived(!!source);

  let key = $state("");
  let saving = $state(false);
  let saved = $state(false);
  // Why the company refused the key just typed, said in this row until it
  // is typed again.
  let refused = $state("");
  // The field, and whether it is shaking off a key that was refused.
  let field = $state<HTMLInputElement>();
  let shaking = $state(false);
  let removing = $state(false);

  // The Go side's errors start small, to sit inside a sentence.
  const sentence = (err: unknown) => {
    const said = errorText(err);
    return said.charAt(0).toUpperCase() + said.slice(1);
  };

  async function save() {
    if (refuse?.()) return;
    saving = true;
    refused = "";
    try {
      await api.saveAPIKey(provider.name, key);
      key = "";
      saved = true;
      setTimeout(() => (saved = false), 1800);
    } catch (err) {
      refused = sentence(err);
      // The way the Mac's password fields take a wrong password: the field
      // shakes, keeps what was typed, selected, with the keyboard in it,
      // so the next paste replaces it. Save waits for something new.
      saving = false;
      shaking = false;
      requestAnimationFrame(() => {
        shaking = true;
        field?.focus();
        field?.select();
      });
    }
    saving = false;
    await onchange?.();
  }

  // An empty key takes the saved one off the machine. It cannot be taken
  // back, the key is not kept anywhere else, so it asks first.
  async function remove() {
    removing = false;
    refused = "";
    try {
      await api.saveAPIKey(provider.name, "");
    } catch (err) {
      refused = sentence(err);
    }
    await onchange?.();
  }
</script>

<div class="item">
  <span class="mark" class:ok={hasKey && !refused} class:err={!!refused} class:warn={!hasKey && !refused} aria-hidden="true">
    {#if hasKey && !refused}<Icon name="check" />{:else}<Icon name="warn" />{/if}
  </span>
  <div class="words">
    <span class="head">{provider.title} API key</span>
    <!-- A saved key is shown in short, the way the companies list keys,
         and where it comes from is in the title. -->
    <span
      class="small line"
      class:muted={hasKey && !refused}
      class:hint={hasKey && !refused && !saved && !!hint}
      class:warn={!hasKey && !refused}
      class:error={!!refused}
      title={refused ||
        (source === "environment"
          ? `From ${provider.env}`
          : hasKey
            ? "In the keychain, where only this app may read it"
            : undefined)}
    >
      {#if refused}{refused}{:else if saved}Saved in the keychain.{:else if hint}{hint}{:else if source === "environment"}From
        {provider.env}.{:else if hasKey}In the keychain.{:else}None yet. Get one at <button
          class="link"
          title="Opens the page where {provider.title} makes keys"
          onclick={() => api.openKeysPage(provider.name).catch(() => {})}>{provider.keysAt}</button
        >.{/if}
    </span>
  </div>
  <input
    class="key shakes"
    class:shaking
    onanimationend={() => (shaking = false)}
    type="password"
    bind:this={field}
    bind:value={key}
    oninput={() => (refused = "")}
    placeholder={hasKey ? "New key" : `${provider.name === "openai" ? "sk-proj" : "sk-ant"}-...`}
    aria-label="{provider.title} API key"
    title="It goes in the keychain and nowhere else"
    autocomplete="off"
    spellcheck="false"
    onkeydown={(e) => e.key === "Enter" && key.trim() && !refused && save()}
  />
  <!-- A key that was refused is refused again, so Save waits until the
       field holds something else. -->
  <button class="act" disabled={saving || !key.trim() || !!refused} onclick={save}>
    {#if saving}<Busy />{/if}
    {saving ? "Checking" : "Save key"}
  </button>
  <!-- Only a key the app keeps can be removed here. One in the environment
       is the terminal's to take away. Without a trash can the last control
       of the row ends at the card's edge, the way every row's does, rather
       than before a strip of nothing kept for a trash can. -->
  {#if source === "keychain"}
    <button
      class="quiet danger bin"
      title="Remove the key from this machine"
      aria-label="Remove the {provider.title} API key"
      aria-haspopup="dialog"
      disabled={saving}
      onclick={() => refuse?.() || (removing = true)}
    >
      <Icon name="trash" />
    </button>
  {/if}
</div>

{#if removing}
  <Confirm title="Remove the {provider.title} API key?" oncancel={() => (removing = false)}>
    <p>
      The key leaves this machine's keychain, and clips cannot be found in the cloud until a key is
      saved again. The key itself stays valid at {provider.keysAt}.
    </p>
    {#snippet actions()}
      <button onclick={() => (removing = false)}>Cancel</button>
      <button class="danger" onclick={remove}>Remove</button>
    {/snippet}
  </Confirm>
{/if}

<style>
  .act {
    flex: none;
  }

  /* Narrow, since what is typed shows as dots, and the room goes to the
     key in short beside it. */
  .key {
    width: 150px;
    flex: none;
  }

  /* The trash can: square at the height of every control. */
  .bin {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--control-h);
    padding: 0;
    flex: none;
  }

  /* A key in short reads as a key: the figures and letters at one width. */
  .hint {
    font-family: ui-monospace, "SF Mono", Menlo, monospace;
  }

  /* A line under a name is one line. It is cut short, with the whole of it
     in its title, rather than wrapped, because a line that wraps when a
     button arrives beside it moves the card. */
  .line {
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .warn {
    color: var(--warn);
  }
</style>
