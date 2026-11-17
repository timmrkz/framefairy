// When the cards of what was said, the transcript search, are read again.

// The pieces a clip keeps, as the cards are read for them: the start and
// end of each piece, or of the clip where it has no cuts. A clip whose
// pieces change, an edge or a cut saved, has its cards read again, since
// its captions are cards of their own, see engine.SaidCards.
export function piecesOf(clip: { start: number; end: number; segments: { start: number; end: number }[] }): string {
  return (clip.segments.length ? clip.segments : [clip]).map((p) => `${p.start.toFixed(2)}-${p.end.toFixed(2)}`).join(" ");
}

// The captions of a clip as last read: what they say and where, and the
// pieces the clip had when they were.
export interface CaptionsRead {
  words: string;
  pieces: string;
}

// Whether the captions of a clip, read again, changed its cards in a way
// no read has caught yet: their words or places changed while the clip's
// pieces stayed, a word corrected or removed, a caption moved, the look
// of every caption changed. Captions that changed with the pieces were
// read with them already: an edge or a cut saved reads the cards for the
// pieces, and the captions of the same edit come 15 to 45 ms later.
// Reading the cards again for them was a second read of the whole
// transcript on every save while the search was open, issue #895.
export function captionsAlone(was: CaptionsRead | undefined, now: CaptionsRead): boolean {
  return was !== undefined && was.pieces === now.pieces && was.words !== now.words;
}
