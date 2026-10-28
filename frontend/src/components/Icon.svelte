<script lang="ts" module>
  // Every glyph in the app is a Lucide icon, the open set much of the web
  // and many desktop apps draw with, so none of them is one we made up
  // and they all share one hand. The app knows them by what they are for,
  // here, and nowhere else names a Lucide icon. Plan row 2.199.
  import PanelLeft from "@lucide/svelte/icons/panel-left";
  import SquarePlus from "@lucide/svelte/icons/square-plus";
  import Folder from "@lucide/svelte/icons/folder";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Info from "@lucide/svelte/icons/info";
  import X from "@lucide/svelte/icons/x";
  import Pause from "@lucide/svelte/icons/pause";
  import Play from "@lucide/svelte/icons/play";
  import Repeat from "@lucide/svelte/icons/repeat";
  import RepeatOff from "@lucide/svelte/icons/repeat-off";
  import Camera from "@lucide/svelte/icons/camera";
  import CameraOff from "@lucide/svelte/icons/camera-off";
  import Diamond from "@lucide/svelte/icons/diamond";
  import DiamondPlus from "@lucide/svelte/icons/diamond-plus";
  import DiamondMinus from "@lucide/svelte/icons/diamond-minus";
  import Locate from "@lucide/svelte/icons/locate";
  import RotateCcw from "@lucide/svelte/icons/rotate-ccw";
  import Undo2 from "@lucide/svelte/icons/undo-2";
  import Redo2 from "@lucide/svelte/icons/redo-2";
  import ChevronsUpDown from "@lucide/svelte/icons/chevrons-up-down";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import Check from "@lucide/svelte/icons/check";
  import Copy from "@lucide/svelte/icons/copy";
  import CircleArrowDown from "@lucide/svelte/icons/circle-arrow-down";
  import ArrowUpDown from "@lucide/svelte/icons/arrow-up-down";
  import Clock from "@lucide/svelte/icons/clock";
  import ArrowDownAZ from "@lucide/svelte/icons/arrow-down-a-z";
  import SlidersHorizontal from "@lucide/svelte/icons/sliders-horizontal";
  import Pipette from "@lucide/svelte/icons/pipette";
  import Mic from "@lucide/svelte/icons/mic";
  import Bug from "@lucide/svelte/icons/bug";
  import Paperclip from "@lucide/svelte/icons/paperclip";
  import Search from "@lucide/svelte/icons/search";
  import MessageSquareQuote from "@lucide/svelte/icons/message-square-quote";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import Share from "@lucide/svelte/icons/share";
  import type { Component } from "svelte";

  // Mark In and Mark Out are the square brackets every editor draws on
  // the two buttons. Lucide has them only as a pair, its brackets icon, so
  // each is one half of that, moved to the middle of the square and drawn
  // the way Lucide draws its own.
  const brackets: Record<string, string> = {
    "mark-in": "M14 21h-3a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h3",
    "mark-out": "M10 3h3a1 1 0 0 1 1 1v16a1 1 0 0 1-1 1h-3",
  };

  const glyphs: Record<string, Component<any>> = {
    sidebar: PanelLeft,
    plus: SquarePlus,
    folder: Folder,
    trash: Trash2,
    info: Info,
    close: X,
    pause: Pause,
    play: Play,
    // The loop, and with it on, the loop crossed out: what a click does,
    // the way pause shows while playing.
    loop: Repeat,
    "loop-off": RepeatOff,
    // A thumbnail is a picture taken of a frame, so a camera, and crossed
    // out where a click removes the one at the playhead.
    thumbnail: Camera,
    "thumbnail-add": Camera,
    "thumbnail-remove": CameraOff,
    // A keyframe of the crop, drawn as the diamond editors draw one.
    // Plan row 3.26. On the clip timeline the keyframe itself, the
    // diamond alone, the way the camera alone is a thumbnail there.
    keyframe: Diamond,
    "keyframe-add": DiamondPlus,
    "keyframe-remove": DiamondMinus,
    // Rotate Left: an arrow turning anticlockwise, and nothing else, so
    // it still reads at the size of a button.
    rotate: RotateCcw,
    locate: Locate,
    // Put back as it started, and put back as you had it: undo and redo,
    // so neither looks like Rotate.
    back: Undo2,
    forward: Redo2,
    pick: ChevronsUpDown,
    chevron: ChevronRight,
    warn: TriangleAlert,
    check: Check,
    copy: Copy,
    // macOS brings updates in with an arrow pointing down.
    update: CircleArrowDown,
    sort: ArrowUpDown,
    "sort-added": Clock,
    "sort-name": ArrowDownAZ,
    sliders: SlidersHorizontal,
    pipette: Pipette,
    // Dictate on Report a Problem.
    mic: Mic,
    // Bugs in the sidebar, the mark issue trackers draw for a bug report,
    // and the paperclip that attaches pictures to it, as in Mail.
    report: Bug,
    attach: Paperclip,
    // The field over a list long enough to search, the magnifier of
    // Spotlight and of every search field on the Mac.
    search: Search,
    // The transcript of the video, searched in the clip column: a speech
    // bubble with quotes in it, the mark Apple Podcasts puts on the button
    // that opens an episode's transcript.
    transcript: MessageSquareQuote,
    // What can be done with a video in the sidebar, behind the three dots
    // Finder and Mail put on a row for its menu.
    more: Ellipsis,
    // Saving something out of the app in another format, the box with an
    // arrow up the Mac puts on Share and Export.
    export: Share,
  };

  // The marks that are solid wherever people see them, play and pause on
  // every transport.
  const solid = new Set(["play", "pause"]);
</script>

<script lang="ts">
  // They take the colour of the text around them, and a line one and a
  // half pixels wide at every size, the weight the workspace's text has.
  // An icon drawn large to be shown smaller, by a scale, takes a line as
  // much thicker, so it is that weight where it rests.
  let { name, size = 16, line = 1.5 }: { name: string; size?: number; line?: number } = $props();

  const Glyph = $derived(glyphs[name]);
  const fill = $derived(solid.has(name) ? "currentColor" : "none");
</script>

{#if brackets[name]}
  <svg
    class="lucide-icon"
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width={(line * 24) / size}
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path d={brackets[name]} />
  </svg>
{:else if Glyph}
  <Glyph {size} {fill} strokeWidth={line} absoluteStrokeWidth aria-hidden="true" />
{/if}

<style>
  :global(svg.lucide-icon) {
    flex: none;
    display: block;
  }
</style>
