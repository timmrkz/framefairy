<script lang="ts">
  import { onMount } from "svelte";

  // The colour test of step 4 of the video preview on one engine, see
  // docs/VIDEO-PREVIEW.md, opened from the Help menu. It lives only in the
  // pull request of step 4 and is removed before it is merged, once Tim
  // has looked and the choices are made.
  //
  // HDR: whether the webview lets a canvas be brighter than white. A
  // WebGPU canvas is asked for the extended mode, half floats in which 1.0
  // is white, and is painted with patches of 1, 2, 4 and 8 times white. Beside them is
  // the white of the interface. If the patches above 1 shine brighter than
  // it, WebKit gives a canvas HDR on this Mac.

  const steps = [1, 2, 4, 8];
  const side = 160;

  let canvas = $state<HTMLCanvasElement>();
  let gpu = $state("asking");
  let mode = $state("asking");
  let screenHDR = $state("asking");
  let problem = $state("");

  async function draw() {
    const c = canvas!;
    const nav = navigator as any;
    if (!nav.gpu) {
      gpu = "no, the webview has no WebGPU";
      mode = "not asked";
      return;
    }
    const adapter = await nav.gpu.requestAdapter();
    if (!adapter) {
      gpu = "no, no adapter";
      mode = "not asked";
      return;
    }
    const device = await adapter.requestDevice();
    gpu = "yes";
    const scale = Math.max(1, Math.round(devicePixelRatio || 1));
    const w = side * steps.length * scale;
    const h = side * scale;
    c.width = w;
    c.height = h;
    const ctx = c.getContext("webgpu") as any;
    if (!ctx) {
      mode = "no WebGPU canvas";
      return;
    }
    ctx.configure({
      device,
      format: "rgba16float",
      colorSpace: "srgb",
      alphaMode: "opaque",
      toneMapping: { mode: "extended" },
    });
    const conf = ctx.getConfiguration?.();
    mode = conf ? (conf.toneMapping?.mode ?? "not said, so standard") : "not said";
    // Each patch is painted by a shader with its own multiple of white,
    // by where it is across the canvas.
    const shader = device.createShaderModule({
      code: `
        @vertex fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4f {
          let p = array(vec2f(-1, -1), vec2f(3, -1), vec2f(-1, 3));
          return vec4f(p[i], 0, 1);
        }
        @fragment fn fs(@builtin(position) at: vec4f) -> @location(0) vec4f {
          let steps = array(${steps.join(".0, ")}.0);
          let i = min(u32(at.x / ${side * scale}.0), ${steps.length - 1}u);
          let v = steps[i];
          return vec4f(v, v, v, 1);
        }`,
    });
    const pipeline = device.createRenderPipeline({
      layout: "auto",
      vertex: { module: shader, entryPoint: "vs" },
      fragment: { module: shader, entryPoint: "fs", targets: [{ format: "rgba16float" }] },
    });
    const encoder = device.createCommandEncoder();
    const pass = encoder.beginRenderPass({
      colorAttachments: [{ view: ctx.getCurrentTexture().createView(), loadOp: "clear", storeOp: "store", clearValue: [0, 0, 0, 1] }],
    });
    pass.setPipeline(pipeline);
    pass.draw(3);
    pass.end();
    device.queue.submit([encoder.finish()]);
  }

  onMount(() => {
    const hdr = matchMedia("(dynamic-range: high)").matches;
    const video = matchMedia("(video-dynamic-range: high)").matches;
    screenHDR = `${hdr ? "yes" : "no"}, for video ${video ? "yes" : "no"}`;
    draw().catch((e) => (problem = String(e)));
  });
</script>

<section class="scroll">
  <div class="panel">
    <h2>HDR on a canvas</h2>
    <p class="muted">
      The square on the left is the white of the app. The four on the right are drawn on the kind
      of canvas the video preview would use for HDR, at 1, 2, 4 and 8 times white. If 2, 4 and 8
      shine brighter than the white on the left, the webview gives a canvas HDR.
    </p>
    <div class="patches">
      <div class="white" style:width="{side}px" style:height="{side}px"></div>
      <canvas bind:this={canvas} style:width="{side * steps.length}px" style:height="{side}px"></canvas>
    </div>
    <div class="labels" style:grid-template-columns="repeat({steps.length + 1}, {side}px)">
      <span class="muted">the app's white</span>
      {#each steps as s (s)}<span class="muted num">{s} × white</span>{/each}
    </div>
    <dl class="selectable">
      <dt>WebGPU</dt>
      <dd>{gpu}</dd>
      <dt>Extended mode</dt>
      <dd>{mode}</dd>
      <dt>Screen says HDR</dt>
      <dd>{screenHDR}</dd>
      {#if problem}<dt>Problem</dt><dd class="error">{problem}</dd>{/if}
    </dl>
  </div>
</section>

<style>
  section {
    padding: var(--gap) var(--edge) var(--edge);
    display: flex;
    flex-direction: column;
    gap: var(--gap);
    flex: 1;
    min-height: 0;
    width: 100%;
    max-width: 880px;
    margin: 0 auto;
  }

  h2 {
    font-size: var(--size-l);
    font-weight: 600;
  }

  .panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .patches {
    display: flex;
  }

  .white {
    background: #fff;
  }

  .labels {
    display: grid;
  }

  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 4px 16px;
    margin: 0;
  }

  dd {
    margin: 0;
  }
</style>
