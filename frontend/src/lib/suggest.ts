// What a search asks for when nobody has said: how long each window of an
// episode is, and how many clips a window looks for. The engine decides
// the same, in engine/suggest.go, for the first search of a new episode
// and for the command line. Here it is worked out again while a window is
// dragged, and both are held to the table in suggest.cases.json, so the
// number in the Target field is the number the engine would take.

// The window grows with the square root of the episode: four hours make
// windows of half an hour, one hour of 15 minutes, half an hour of about 10.
const windowFactor = 3.75;
// No window is cut shorter than this, and an episode this short or shorter
// is one window.
const leastWindow = 10 * 60;
// One clip is asked for every twelve clip lengths of window.
const clipsApart = 12;

// How long each window of an episode is, in seconds: equal windows as close
// to the square root rule as divide the episode evenly, none shorter than
// ten minutes.
export function suggestedWindow(duration: number): number {
  if (duration <= leastWindow) return Math.max(duration, 0);
  const ideal = Math.max(Math.sqrt((windowFactor * duration) / 60) * 60, leastWindow);
  let n = Math.max(roundHalfAway(duration / ideal), 1);
  if (duration / n < leastWindow) n = Math.max(Math.floor(duration / leastWindow), 1);
  return duration / n;
}

// How many clips a window of the given length looks for, with clips from
// least to most seconds long. Never fewer than one.
export function suggestedCount(window: number, least: number, most: number): number {
  const clip = (least + most) / 2;
  if (clip <= 0 || window <= 0) return 1;
  return Math.max(roundHalfAway(window / (clipsApart * clip)), 1);
}

// Go's math.Round rounds a half away from zero, and Math.round rounds it
// up. For what is never negative they agree, but they are kept the same on
// purpose, so the two sides cannot differ on a half.
function roundHalfAway(x: number): number {
  return Math.sign(x) * Math.round(Math.abs(x));
}
