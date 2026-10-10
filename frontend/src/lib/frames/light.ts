// How a frame of HDR turns into light on the screen. The Go side sends HDR
// as it is in the file, red, green and blue with 10 bits each in the
// file's own curve, PQ or HLG, with BT.2020's colours, see
// framewire.Picture. Here that becomes light, in units of the reference
// white of BT.2408, 203 nits, the white of the interface, in BT.709's
// colours, the canvas's. Above 1 is brighter than white, which the WebGPU
// canvas in the extended mode shows on an HDR screen, see screen.ts. Where
// there is no such canvas, or for a colour taken from the picture,
// everything above white is cut at white, as an SDR screen shows it. The
// formulas are BT.2100's, written once here for the GPU, wgsl, and once
// for the processor, the functions below.

// "" is standard video, whose frames are colours ready to draw.
export type Light = "" | "pq" | "hlg";

export function lightOf(said: string | null): Light {
  return said === "pq" || said === "hlg" ? said : "";
}

// BT.2408's reference white for graphics in HDR, in nits.
const WHITE = 203;
// The display HLG is shown on, in nits: BT.2100's reference, at which
// HLG's reference white, 75 percent, is 203 nits.
const HLG_PEAK = 1000;
// BT.2020's colours in BT.709's, in linear light, row by row. A colour
// outside BT.709 comes out below 0, which the extended canvas keeps.
const TO_709 = [
  [1.6605, -0.5876, -0.0728],
  [-0.1246, 1.1329, -0.0083],
  [-0.0182, -0.1006, 1.1187],
];
// BT.2020's luminance.
const Y2020 = [0.2627, 0.678, 0.0593];

// PQ's curve, a signal of 0 to 1 to nits.
const M1 = 0.1593017578125;
const M2 = 78.84375;
const C1 = 0.8359375;
const C2 = 18.8515625;
const C3 = 18.6875;
// HLG's curve.
const A = 0.17883277;
const B = 0.28466892;
const C = 0.55991073;

export function pqNits(e: number): number {
  const p = Math.pow(Math.max(e, 0), 1 / M2);
  return 10000 * Math.pow(Math.max(p - C1, 0) / (C2 - C3 * p), 1 / M1);
}

// HLG's signal to scene light, 0 to 1.
export function hlgScene(e: number): number {
  return e <= 0.5 ? (e * e) / 3 : (Math.exp((e - C) / A) + B) / 12;
}

// A pixel in light, red, green and blue in units of the reference white
// and in BT.709's colours, from its three signals of 0 to 1.
export function lightOfPixel(light: Light, r: number, g: number, b: number): [number, number, number] {
  let rgb: number[];
  if (light === "pq") rgb = [pqNits(r), pqNits(g), pqNits(b)];
  else {
    const s = [hlgScene(r), hlgScene(g), hlgScene(b)];
    const y = Y2020[0] * s[0] + Y2020[1] * s[1] + Y2020[2] * s[2];
    const gain = HLG_PEAK * Math.pow(Math.max(y, 1e-6), 0.2);
    rgb = s.map((v) => v * gain);
  }
  const l = rgb.map((v) => v / WHITE);
  return TO_709.map((row) => row[0] * l[0] + row[1] * l[1] + row[2] * l[2]) as [number, number, number];
}

// sRGB's curve, light of 0 to 1 to a signal of 0 to 1.
export function srgb(l: number): number {
  return l <= 0.0031308 ? 12.92 * l : 1.055 * Math.pow(l, 1 / 2.4) - 0.055;
}

// A frame of HDR as 8-bit colours with an opaque alpha, everything above
// white cut at white. The signals of each colour are looked up rather
// than worked out, 1024 of them, so a frame of 1280 by 720 costs one
// power a pixel, HLG's, and none for PQ.
const tables = new Map<Light, Float32Array>();
const out8 = new Uint8Array(4096);
for (let i = 0; i < 4096; i++) out8[i] = Math.round(255 * srgb(i / 4095));

export function toSDR(light: Light, data: Uint8Array, into: Uint8ClampedArray) {
  let table = tables.get(light);
  if (!table) {
    table = new Float32Array(1024);
    for (let i = 0; i < 1024; i++) table[i] = light === "pq" ? pqNits(i / 1023) / WHITE : hlgScene(i / 1023);
    tables.set(light, table);
  }
  const words = new Uint32Array(data.buffer, data.byteOffset, data.byteLength / 4);
  const to = (v: number) => out8[Math.round(Math.min(Math.max(v, 0), 1) * 4095)];
  for (let p = 0; p < words.length; p++) {
    const w = words[p];
    let r = table[w & 1023];
    let g = table[(w >>> 10) & 1023];
    let b = table[(w >>> 20) & 1023];
    if (light === "hlg") {
      const gain = (HLG_PEAK * Math.pow(Math.max(Y2020[0] * r + Y2020[1] * g + Y2020[2] * b, 1e-6), 0.2)) / WHITE;
      r *= gain;
      g *= gain;
      b *= gain;
    }
    const i = p * 4;
    into[i] = to(TO_709[0][0] * r + TO_709[0][1] * g + TO_709[0][2] * b);
    into[i + 1] = to(TO_709[1][0] * r + TO_709[1][1] * g + TO_709[1][2] * b);
    into[i + 2] = to(TO_709[2][0] * r + TO_709[2][1] * g + TO_709[2][2] * b);
    into[i + 3] = 255;
  }
}

const f = (n: number) => n.toFixed(10);
const row = (r: number[]) => r.map(f).join(", ");

// The same for the GPU: a function from a frame's sampled colour to what
// the canvas is given, sRGB's curve carried on past 0 and 1, as the
// extended canvas takes it. kind is 0 for standard video, 1 for PQ, 2 for
// HLG.
export const wgsl = `
fn pqNits(e: vec3f) -> vec3f {
  let p = pow(max(e, vec3f(0)), vec3f(1.0 / ${f(M2)}));
  return 10000.0 * pow(max(p - ${f(C1)}, vec3f(0)) / (${f(C2)} - ${f(C3)} * p), vec3f(1.0 / ${f(M1)}));
}

fn hlgNits(e: vec3f) -> vec3f {
  let s = select((exp((e - ${f(C)}) / ${f(A)}) + ${f(B)}) / 12.0, e * e / 3.0, e <= vec3f(0.5));
  let y = dot(s, vec3f(${row(Y2020)}));
  return ${f(HLG_PEAK)} * pow(max(y, 1e-6), 0.2) * s;
}

fn srgb(l: vec3f) -> vec3f {
  let a = abs(l);
  return sign(l) * select(1.055 * pow(a, vec3f(1.0 / 2.4)) - 0.055, 12.92 * a, a <= vec3f(0.0031308));
}

fn shown(c: vec3f, kind: u32) -> vec3f {
  if (kind == 0u) {
    return c;
  }
  let nits = select(hlgNits(c), pqNits(c), kind == 1u);
  let to709 = transpose(mat3x3f(
    ${row(TO_709[0])},
    ${row(TO_709[1])},
    ${row(TO_709[2])},
  ));
  return srgb(to709 * (nits / ${f(WHITE)}));
}
`;
