<script lang="ts">
  import { onMount } from "svelte";
  import Bar from "../components/Bar.svelte";
  import Landing from "../components/Landing.svelte";
  import Report from "./Report.svelte";
  import { api, onUndo } from "../lib/api";
  import { typing } from "../lib/keys";
  import { titleBar } from "../lib/chrome.svelte";
  import { wearColour } from "../lib/colour";

  // The report window, which Bugs in the sidebar opens, one at a time:
  // Report a Problem alone, under the same bar as the app's window. A
  // picture dropped anywhere on it is attached, see bugs.go. Plan row
  // 2.244.
  onMount(() => {
    api
      .getSettings()
      .then((s) => wearColour(s.appColour))
      .catch(() => {});
    document.documentElement.toggleAttribute("data-file-drop-target", true);
    // Undo and Redo in the Edit menu go to the field being typed in, as
    // on every page with no workspace, or Cmd+Z in it would do nothing.
    const noUndo = onUndo((what) => {
      if (typing()) document.execCommand(what);
    });
    const noChrome = titleBar.follow();
    return () => {
      noUndo();
      noChrome();
    };
  });
</script>

<div class="shell" style={titleBar.style}>
  <Bar title="Report a Problem" />
  <Report />
  <!-- Where a picture dragged over the window would land: all of it. -->
  <Landing />
</div>

<style>
  .shell {
    position: relative;
    display: flex;
    flex-direction: column;
    height: 100%;
  }
</style>
