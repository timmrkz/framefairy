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
export function chime(how: "on" | "off") {
  if (!context || (how === "on") === on) return;
  on = how === "on";
  const c = context;
  const at = c.currentTime + 0.01;
  const notes = how === "on" ? [LOW, HIGH] : [HIGH, LOW];
  notes.forEach((f, i) => note(c, f, at + i * NEXT));
  said(`dictation: the ${how === "on" ? "start" : "stop"} sound played`);
  const was = ++played;
  if (how === "off")
    setTimeout(
      () => was === played && c.suspend().catch(() => {}),
      (0.01 + NEXT + NOTE) * 1000 + 100,
    );
}

function note(c: AudioContext, f: number, at: number) {
  const tone = c.createOscillator();
  const gain = c.createGain();
  tone.type = "sine";
  tone.frequency.value = f;
  // A soft start and a long fall, so it rings rather than clicks.
  gain.gain.setValueAtTime(0, at);
  gain.gain.linearRampToValueAtTime(LOUD, at + 0.008);
  gain.gain.exponentialRampToValueAtTime(0.0001, at + NOTE);
  tone.connect(gain).connect(c.destination);
  tone.start(at);
  tone.stop(at + NOTE + 0.02);
}
