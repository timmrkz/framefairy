<script lang="ts">
  // The Updates page in made-up states, one under the other, so a pull
  // request in conflict with main can be seen in the real page before
  // 2.176 is merged. Each is the page itself, given a state instead of
  // asking the Go side. Its list opens and picks, and nothing else does
  // anything. It is in the Help menu while 2.176 is tried, and removed
  // before it is merged.
  import type { UpdateChannel, UpdateState } from "../lib/api";
  import Updates from "./Updates.svelte";

  const ch = (n: string, conflict = false): UpdateChannel => ({ id: n, name: n, version: `0.3.0-${n}`, conflict });
  const list = [ch("main"), ch("pr-23"), ch("pr-21", true), ch("pr-20", true)];
  const now = new Date().toISOString();

  const base: UpdateState = {
    version: "0.3.0-pr21.7",
    commit: "6ceea6d1f2a3",
    channel: "pr-21",
    off: "",
    channels: list,
    picked: "",
    follows: "",
    gone: "",
    building: "",
    phase: "current",
    next: "",
    nextName: "",
    nextCommit: "",
    checked: now,
    written: 0,
    total: 0,
    problem: "",
  };

  const samples: { what: string; state: UpdateState }[] = [
    {
      what: "Following main. Two PRs in the list conflict.",
      state: { ...base, version: "0.3.0-main.12", channel: "main", follows: "main" },
    },
    {
      what: "Following a PR in conflict, up to date.",
      state: { ...base, follows: "pr-21" },
    },
    {
      what: "Following a PR in conflict, a newer build ready.",
      state: { ...base, follows: "pr-21", phase: "ready", next: "0.3.0-pr21.8" },
    },
    {
      what: "Following a PR in conflict, a newer commit being built.",
      state: { ...base, follows: "pr-21", building: "9f8e7d6c5b4a" },
    },
    {
      what: "Following a PR that was closed, others in conflict in the list.",
      state: { ...base, channel: "pr-18", gone: "pr-18", phase: "gone" },
    },
    {
      what: "Built on this Mac, no channel chosen yet.",
      state: { ...base, version: "0.3.0-dev", commit: "", channel: "", phase: "" },
    },
  ];
</script>

<section class="scroll">
  {#each samples as s (s.what)}
    <div class="sample">
      <p class="muted small">{s.what}</p>
      <Updates sample={s.state} />
    </div>
  {/each}
</section>

<style>
  section {
    flex: 1;
    min-height: 0;
    padding-bottom: var(--edge);
  }

  .sample {
    display: flex;
    flex-direction: column;
  }

  .sample p {
    width: 100%;
    max-width: 880px;
    margin: var(--gap) auto 0;
    padding: 0 var(--edge);
  }
</style>
