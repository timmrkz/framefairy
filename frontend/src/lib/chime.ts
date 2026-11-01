// The two sounds of dictation: one when the microphone is open and what
// is said is written down, one when it stops. Tim asked for them, so a
// person who talks while looking at the workspace hears that they are
// heard without looking at the report window. Plan row 2.244.
//
// Every dictation people know does it this way. macOS Dictation, Siri and
// the Google Assistant play two short notes going up when they start to
// listen and the same two going down when they stop, so the pair is read
// as on and off without having been learnt. These are two soft sine notes
// a fifth apart, short and quiet, made here rather than played from a
// file, so nothing ships with the app for them.

import { said } from "./said";

// The notes, in Hz: E5 and B5.
const LOW = 659.26;
const HIGH = 987.77;
// How long each note sounds, and how long after the first the second
// starts, in seconds.
const NOTE = 0.11;
const NEXT = 0.075;
// How loud, as a gain: quiet beside a voice, and a tone the speech model
// writes nothing down for if the microphone hears it.
const LOUD = 0.12;
// How long a note takes to reach LOUD, in seconds, and the gain it falls
// to by its end.
const RISE = 0.008;
const QUIET = 0.0001;

let context: AudioContext | null = null;
// Counts every wake and every sound, so the rest after the stop sound is
// skipped when a dictation started meanwhile.
let played = 0;
// Whether the start sound played and the stop sound has not.
let on = false;

// wake readies the sound while the hand is still on the button: a webview
// lets sound start only from a click or a key, and the microphone opens
// after the click has passed.
export function wake() {
  played++;
  try {
    context ??= new AudioContext();
    if (context.state === "suspended") context.resume().catch(() => {});
  } catch (e) {
    said(`dictation: no sound, ${e}`, { level: "warn" });
  }
}

// chime plays the notes going up for on and going down for off, and off
// only after an on, so a microphone that never opened makes no sound. Off
// lets the sound rest again once the notes have sounded, so nothing holds
// the speakers between two dictations.
//
// The pair is one sound, made whole before it plays and started once, so
// the second note follows the first however late the window gets to it.
// Two notes each started at a moment of their own sounded together when
// the window was late to start them, and not at all when it was later
// than their end, which a walk heard on a busy machine, #654.
export function chime(how: "on" | "off") {
  if (!context || (how === "on") === on) return;
  on = how === "on";
  const c = context;
  const sound = c.createBufferSource();
  sound.buffer = pair(c, how === "on" ? [LOW, HIGH] : [HIGH, LOW]);
  sound.connect(c.destination);
  const was = ++played;
  if (how === "off") sound.onended = () => was === played && c.suspend().catch(() => {});
  sound.start();
  said(`dictation: the ${how === "on" ? "start" : "stop"} sound played`);
}

// pair writes the two notes into one sound, the second NEXT after the
// first, each with a soft start and a long fall, so it rings rather than
// clicks.
function pair(c: AudioContext, notes: number[]): AudioBuffer {
  const rate = c.sampleRate;
  const sound = c.createBuffer(1, Math.ceil((NEXT + NOTE) * rate), rate);
  const d = sound.getChannelData(0);
  notes.forEach((f, n) => {
    const from = Math.round(n * NEXT * rate);
    for (let i = 0; i < NOTE * rate && from + i < d.length; i++) {
      const t = i / rate;
      const loud = t < RISE ? (LOUD * t) / RISE : LOUD * (QUIET / LOUD) ** ((t - RISE) / (NOTE - RISE));
      d[from + i] += loud * Math.sin(2 * Math.PI * f * t);
    }
  });
  return sound;
}
