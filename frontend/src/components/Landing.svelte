<!-- Where files dragged over a window of the app would land: all of it.
     The runtime puts file-drop-target-active on the page while a file is
     held over it, so this is lit for exactly as long as a drop would take
     something, and nothing here has to follow the drag. The app's window
     takes videos this way and the report window pictures. -->
<div class="landing frame" aria-hidden="true"></div>

<style>
  /* The frame of what is chosen, round the whole window with a gap to its
     edge, the way the crop sits in the video preview, and the wash of a
     chosen clip inside it. The gap keeps the frame's corners clear of the
     window's own rounded corners. Over the workspace and under the
     question about quitting, and never in the way of the pointer. */
  .landing {
    position: fixed;
    inset: var(--gap);
    z-index: 999;
    pointer-events: none;
    background: var(--accent-wash);
    opacity: 0;
    transition: opacity 120ms;
  }

  :global(:root.file-drop-target-active) .landing {
    opacity: 1;
  }
</style>
