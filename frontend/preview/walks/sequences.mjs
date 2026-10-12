// Named sequences of steps with what has to come of them: the cases found
// by hand, kept so they stay fixed. Every step is also checked against the
// rules every walk is, see rules.mjs. See docs/TESTING.md.
//
//   BRIDGE_URL=http://127.0.0.1:8123/ node sequences.mjs [part of a name]
//
// A step is a list, its name first:
//
//   ["frame", word]    walks the playhead with Shift and → until the word is
//                      in the caption box, clicks it and leaves it with
//                      Escape, so it wears the keyboard's frame
//   ["click", word]    clicks the word, which opens it
//   ["block", word]    clicks the block on the clip timeline of the caption
//                      the engine has the word in, which picks it
//   ["gone", word]     no caption the engine has holds the word
//   ["fewer", label, n] the engine's captions hold n words fewer than at
//                      the mark, and as many captions
//   ["blocks"]         the clip timeline draws a block for every caption the
//                      engine has, and for no other
//   ["press", key]     presses a key, as Playwright names it
//   ["button", label]  clicks the button that says label, and waits until
//                      it has done its work
//   ["type", text]     types
//   ["undo"], ["redo"] the menu bar's Undo and Redo
//   ["undo refused"]   the menu bar's Undo, which the Go side refuses
//                      because what it would put back has changed since
//   ["mark", label]    remembers the engine's captions and pieces
//   ["cut at", share]  double-clicks in the clip, this share of the way
//                      along it, which cuts a part out there
//   ["join", n]        double-clicks on the clip timeline's nth cut, from 1,
//                      which puts the part back
//   ["trim", edge, px] drags the clip's "start" or "end" edge by px
//   ["drag while playing", edge, px]
//                      plays the clip if it does not play, and drags its
//                      "end" edge, or the first edge of its first "cut", by
//                      px in twenty steps while it plays, reading the
//                      playhead and the frame on screen on every animation
//                      frame from before the hand takes hold to after it
//                      lets go
//   ["press while playing", px]
//                      plays the clip if it does not play, presses the
//                      clip timeline halfway along the clip and holds the
//                      button down still, then moves the hand by px and
//                      holds it still again, then lets go, reading the
//                      playhead, the frame on screen and the play button on
//                      every animation frame
//   ["loop off near the end", seconds]
//                      records every sample handed to the sound card,
//                      plays the clip from where the playhead is, presses
//                      L to loop it, and once the playhead is so many
//                      seconds before the clip's end presses L five times
//                      quickly, the loop off after the last, then reads
//                      the playhead, the frame on screen and the play
//                      button on every animation frame until the play
//                      stops, and a second after
//   ["play from", where, seconds]
//                      clicks the clip timeline "before" the clip, halfway
//                      between the track's left end and the clip's start,
//                      which puts the playhead on the video, or "in" it, a
//                      fifth of the way along, which puts it on the clip,
//                      presses the space bar, reads what the video preview
//                      lays over the picture on every animation frame for
//                      so many seconds of play, and pauses
//   ["play from", where, seconds, key]
//                      the same, pressing this key every 400 ms of the
//                      play, "Shift+ArrowRight" to skip ahead by words,
//                      and from "before" the playhead has to have gone a
//                      second or more further than the play takes it
//   ["make while playing", key]
//                      clicks the clip timeline before the clip, which
//                      puts the playhead on the video, presses the space
//                      bar, and a second and a half into the play presses
//                      "i" or "o", Mark In or Mark Out, which makes a clip
//                      there while the episode plays on. It reads what the
//                      clip timeline and the video preview show on every
//                      animation frame until the clip made is written and
//                      chosen, and two seconds after, then pauses
//   ["make while paused", key]
//                      the same with the video paused, and no play
//   ["reset", edge]    double-clicks the clip's edge, which puts it back
//                      where the clip was found
//   ["cut switch", n]  double-clicks on the clip timeline where the clip's
//                      nth shot meets the next, from 1, which cuts a part
//                      out across the camera switch
//   ["trim past", edge, n]
//                      drags the clip's "start" or "end" edge two seconds
//                      past the nth camera switch, so the shot beyond it is
//                      gone
//   ["model", how]     the language model "holds" its answers, "fails" or
//                      "answers"
//   ["add", seconds]   adds a new video of so many seconds with Add
//   ["add", seconds, rate]
//                      adds one filmed at a frame rate, "30000/1001" for
//                      29.97 fps, whose frames keep their numbers in a
//                      short
//   ["add", seconds, rate, timing]
//                      the same with its frames at a phone's uneven times,
//                      "uneven", or its picture starting after its sound,
//                      "late"
//   ["add", seconds, rate, timing, "mkv"]
//                      the same in Matroska, the way OBS records, with ""
//                      for frames at the rate
//   ["render"]         presses Render and waits until the short is written
//   ["captions like the short"]
//                      puts the playhead on the first frame of the clip's
//                      words and in their middle, and holds the lower half
//                      of the crop in the video preview to the frame of
//                      the short Render wrote that shows the same moment
//   ["add sound alone"] adds a file of sound with no picture, named like a
//                      video, with Add
//   ["add cameras", seconds, switch]
//                      adds one filmed by two cameras that switch so many
//                      seconds in, each looking at its own subject
//   ["look", label]    remembers, for every piece of the clip, the crop
//                      frame in the video preview with the playhead in the
//                      middle of the piece, put there with a click on the
//                      clip timeline: where it stands and what it shows
//   ["head"]           presses the clip list's head button, New, Cancel or
//                      Continue
//   ["hold", call, ms] the next call of this name waits so many
//                      milliseconds before it reaches the Go side, the way
//                      a busy machine delivers it late
//   ["new and cancel"] presses New, and Cancel in the first frame the head
//                      says it, and every frame from New to that press the
//                      head says Cancel and takes the click, never greyed
//                      out
//   ["restart"]        closes the app and opens it again on the episode
//   ["speech", ms]     the speech model takes so many milliseconds over
//                      each piece of audio, so a video added is heard
//                      slowly enough to do something while it is
//   ["settings"]       opens the settings from the sidebar, and remembers
//                      the models they name
//   ["menu", "report"] picks Help → Report a Problem… in the menu bar
//   ["write", text, n] types text into the report's field and puts the
//                      cursor n characters in
//   ["dictate", how]   presses Dictate, waits for the words to appear grey
//                      while they are heard, and ends the recording how
//                      says: "button" presses Stop, "enter" presses Enter
//                      and "escape" Escape, and "ends" presses nothing
//                      until it stops by itself and says why. Then waits
//                      for Dictate again.
//                      "start" only starts it, "hearing" waits for more
//                      to be heard, "settled" waits for words
//                      before a pause to be settled, and a recording
//                      already running is only ended
//   ["click heard"]    clicks into the first word still being heard and
//                      sees it settled there, with the cursor in it
//   ["field", selector, text]
//                      clicks the field selector names and types text
//   ["field reads", selector, text]
//                      the field selector names holds text
//   ["report reads", text]
//                      the report's field reads text, where * is
//                      whatever the speech model heard, a word or more
//   ["report as before"]
//                      the report's field reads what it did at the last
//                      "report reads"
//   ["episode"]        opens the episode on screen before again
//   ["episode watched"]
//                      the same, reading back from the video preview every
//                      frame it draws for the episode from the click on
//   ["pick model"]     opens the list of what finds clips and, if it
//                      opens, picks the last model here it does not have
//                      picked
//   ["remove model", which]
//                      presses the bin of the "language" model on this
//                      machine, under Downloaded models, or of the
//                      "speech" model
//   ["screen light"]   draws pixels of standard video, of PQ and of HLG with
//                      the video preview's GPU drawing, and holds what the
//                      GPU gives the canvas to light.ts worked out on the
//                      processor, standard video to QuickTime's look with
//                      every 10-bit step a step of its own, and HDR to
//                      BT.2100's own anchors: HDR's
//                      reference white is the app's white, its highlights
//                      are brighter, and a green of BT.2020 lies outside
//                      BT.709. Says so and checks nothing without WebGPU
//   ["play to the end"]
//                      drags the clip's end to the end of the episode,
//                      puts the playhead three seconds before it with a
//                      click on the clip timeline and plays until the play
//                      stops there, so the episode's decoder reads to the
//                      end of the file
//   ["no browser decoder"]
//                      takes the browser's picture and sound decoders away,
//                      every one, and opens the episode again
//   ["add hevc"]       adds a video of 40 seconds whose picture is HEVC with
//                      10-bit colour in open GOPs, as x265 writes it, and
//                      chooses the clip its first search finds
//   ["leading frames"] puts the playhead with a click on the clip timeline
//                      on three frames of the clip that are shown before
//                      the key frame they are decoded after, read from the
//                      file by ffprobe, waits for each in the video
//                      preview, and plays from the last
//   ["add namesake", seconds]
//                      adds a new video of so many seconds with the file
//                      name of the bridge's episode, from another folder
//   ["rotate at", share]
//                      puts the playhead in the clip, this share of the
//                      way along it, with a click on the clip timeline, and
//                      presses R, which rotates the part of the clip there,
//                      and the video preview has to show it rotated within
//                      300 ms, before a held answer comes
//   ["scrub", from, to] reads the frame on the video preview's canvas, by
//                      its bands, and how far the video preview turns it on
//                      every animation frame, while the hand presses the
//                      clip timeline this share of the way along the clip
//                      and drags the playhead quickly back and forth three
//                      times over the middle of the clip, where its cut is,
//                      and then on to that share
//   ["remove clip"]    presses the bin of the clip on screen in the clip
//                      list, and waits until the Go side has it removed
//   ["thumbnail"]      presses T, which makes the frame under the playhead
//                      a thumbnail of the clip, and waits for the clip to
//                      have one more
//   ["keep", label]    remembers the clip on screen, its short and its
//                      pictures on disk, and the shorts in its folder
//   ["updates", how]   opens the Updates page from the sidebar on a build
//                      as how says: from the channel how.from, or made on
//                      the Mac without it, with how.stored picked by a
//                      build before it, and the channels how.list on the
//                      list, each with a newer build, and those in
//                      how.conflict said to no longer merge into main
//   ["follow", name]   picks the channel of this name in the Updates
//                      page's list, and reads the page in the next frame
//   ["words again"]    the speech stand-in says its sentences again from
//                      the first word, so a video added next is heard as
//                      the bridge's episode was and its clip has the same
//                      name
//
// and what has to come of them:
//
//   ["opened on the clip"]
//                      since "episode watched", the video preview drew the
//                      first frame of the chosen clip and no other, and the
//                      playhead was on the clip before the workspace asked
//                      for the window
//   ["drop", seconds, where]
//                      makes a new video of so many seconds, opens the
//                      "settings", "updates" or stays on the "workspace",
//                      holds the file over the app the way the runtime
//                      reports a drag from Finder, and drops it there. The
//                      app has to wear its frame while the file is held
//                      over it and not after the drop
//   ["drop again", where]
//                      the same with the video dropped last
//   ["drop not a video", where]
//                      the same with a PDF
//   ["dropped in front"]
//                      the video dropped last is the one on screen, and in
//                      the library once
//   ["not a video"]    the PDF dropped last is not in the library, and the
//                      app says by its name that it is not a video
//   ["no reason"]      the app says nothing was left out: a reason from
//                      files given before is gone once the next are taken
//   ["left out", name] Add left out the file whose name starts so, says why
//                      where the app says what went wrong, and the library
//                      does not hold it
//   ["played through"] since "drag while playing", the play went on as if
//                      no hand were there: the playhead never went back nor
//                      stood still for longer than 150 ms, and no frame
//                      drawn was older than the one before it
//   ["stopped at its end"]
//                      since "loop off near the end", the presses of L
//                      started no play over, the sound did not click,
//                      nothing was heard after the clip's end, the play
//                      stopped within 400 ms of the playhead reaching it,
//                      and no frame drawn from then on is from before the
//                      last second of the clip, so nothing of the clip's
//                      start played again
//   ["nailed"]         since "press while playing", the playhead stood
//                      still under the hand while it held still, on the
//                      press and again where the hand moved to, the button
//                      said Pause the whole time, and the play went on
//                      from where the hand let go
//   ["only the video", "across cuts"], ["the clip shown", "across cuts"]
//                      the same, and the play crossed a cut: it stood in
//                      two pieces of the clip or more, and on the video
//                      went through the part cut out between them
//   ["only the video"] since "play from", the playhead was on the video
//                      on every frame and played two seconds or more of
//                      the clip's own moments, and the video preview laid
//                      nothing over the picture, no crop frame, no shade
//                      and no captions
//   ["the clip shown"] since "play from", the playhead was on the clip on
//                      every frame and played two seconds or more inside
//                      it, the crop frame stood over the picture on every
//                      one of them, and the captions on every one where the
//                      engine has a caption, a few at least
//   ["made on the clip"]
//                      since "make while playing", the clip made was the
//                      chosen card from the frame it came on, on its way
//                      and then written, and on every animation frame from
//                      then on, playing and paused, the playhead was on it:
//                      the clip timeline did not dim it
//   ["box", text]      the caption box reads this, word by word
//   ["open", word]     this word is open for typing
//   ["same", label]    the engine's captions and pieces are what they were
//                      at the mark
//   ["cuts", n]        the clip timeline shows n cuts
//   ["wait for", word] the clip list's head button comes to say this, in a
//                      minute at most
//   ["row", words]     a row of the clip list says this
//   ["no row", words] no row of the clip list says this
//   ["no put back"]    no row of the clip list offers a removed clip back
//   ["row closes", [words, s]]
//                      a row of the clip list says this, runs down, and
//                      closes by itself about s seconds later, not much
//                      sooner and not much later
//   ["cards", n]       the clip list holds n clips
//   ["whole clip", s]  the episode on screen, s seconds long, has one clip
//                      in the list, and it runs from its first second to
//                      its last
//   ["waveform"]       the clip timeline draws the waveform across the whole
//                      of its view within ten seconds, read from its canvas
//   ["rulers fit", s]  the range picker has lines and times on it, every
//                      one inside the episode of s seconds, and the clip
//                      timeline shows that episode and nothing past it
//   ["spans", label]   every caption appears and goes when it did at the mark
//   ["pieces", n]      the clip timeline draws the clip in n pieces
//   ["framed", label]  the clip timeline draws as many pieces as at the
//                      look, and at each moment looked at the crop frame
//                      stands where it stood and shows what it showed
//   ["shots", label]   the short, read back from disk: every frame shows
//                      what the crop frame showed on its shot at the look,
//                      and at every camera switch the sound runs straight
//                      on, no 10 ms of it quieter than 80% of the tone
//   ["from the app", n]
//                      the picture comes from the Go side, and n clicks
//                      along the clip timeline each show the frame that
//                      holds the playhead
//   ["app size", w, h] makes the app w by h pixels
//   ["square buttons"] every button in the row under the clip timeline
//                      that wears an icon or a key's letter is as wide as
//                      it is tall, and as tall as every control
//   ["sharp"]          the frame on the video preview, as the workspace
//                      opened and before anything played, was decoded at
//                      the size of the canvas, no larger than the file's
//                      own picture
//   ["sharp", "opened"] the same, and the first frame the workspace asked
//                      for since it opened was asked at that size
//   ["on frames"]      every edge of the clip the hand put, all but its
//                      end and any before the picture starts, lands on
//                      one of the picture's own frames, counted from where
//                      the picture starts, the frames the render cuts on
//   ["resize"]         drags the app's height and then its width a pixel at
//                      a time, down to the app's smallest, and on every step every part of the
//                      workspace moves only one way the whole drag, stands
//                      on a whole pixel, and meets the app's edges, and the
//                      two sides beside the picture, measured from the
//                      app's edges, are the same width, or a pixel apart,
//                      and stand still while only the height changes
//   ["crop to", side]  drags the crop frame as far as it goes to the
//                      "left" or the "right" of the picture
//   ["frame whole"]    over a white picture, the crop frame's line is the
//                      app's colour all the way along its four sides, the
//                      viewer's rounded corners cutting none of it, the
//                      shade reaches into the frame's rounded corners with
//                      nothing bright left there, the inside of the frame
//                      is not shaded and the picture beside it is, and the
//                      line stands where the crop the hand moves stands
//   ["on borders"]     the chosen clip's frame on the clip timeline and the
//                      bars of the window at rest on the range picker lie
//                      on their track's top and bottom border, read off a
//                      screenshot: the outermost rows of the track's box
//                      are the app's colour, as deep as the line is thick,
//                      and the row just outside it is not, so no grey of
//                      the border shows beside them, away from the
//                      playhead and the marks that stand on the lines
//   ["frame whole", "turned"]
//                      the same on a part rotated a quarter, whose picture
//                      stands inside the viewer rather than filling it
//   ["short", cuts]    the clip has at least so many cuts, none of them on
//                      a frame that starts on a whole millisecond, and the
//                      short Render wrote, read back from disk, holds
//                      exactly the frames of the clip's pieces, each the
//                      right frame of the episode, with its sound as long
//                      as its picture and quiet at a cut for no longer
//                      than the render's fade
//   ["apart", label]   the clip on screen, of another episode with a clip
//                      of the same name as the one kept, has a short of its
//                      own, the short kept is still the same file, and the
//                      clip kept still says it is its short
//   ["apart", label, number]
//                      the same, with the short on screen called as the
//                      one kept with this after it, " 2". With both, the
//                      pictures kept are all still there, untouched
//   ["in place", label]
//                      the clip's short is the one kept, under the same
//                      name with no number, written again since, and no
//                      other short appeared in its folder
//   ["by its words"]   the clip's short is named after the clip's words
//                      alone, <slug>.mp4, with nothing of its id
//   ["pictured"]       the short's pictures, one for each thumbnail of the
//                      clip, lie beside it in its episode's folder, and no
//                      picture more
//   ["as previewed", rate]
//                      the short Render wrote, read back from disk, has
//                      this frame rate and as many frames as the clip's
//                      pieces last, and at every cut, at the clip's two
//                      ends and on every tenth frame between, its frame is
//                      the frame the video preview shows at the moment that
//                      frame stands for, read off the canvas with the
//                      playhead put there by a click on the clip timeline
//   ["edges as rendered"]
//                      the short Render wrote, read back from disk, and the
//                      video preview agree on the clip's edges: with the
//                      playhead put on the clip's start and on where each
//                      cut ends by a click on it, the video preview shows
//                      the frame the short starts that piece with, and on
//                      the clip's end the short's last frame. The clip's
//                      end is where the search put it, and at least one
//                      edge is kept to the millisecond just before its
//                      frame's start
//   ["as previewed", rate, before]
//                      the same, for an episode whose picture starts after
//                      its sound: the clip's first piece starts before
//                      this moment, inside the time before the picture
//   ["refused", card]  the gesture before it shook the "finding" or the
//                      "speech" card and opened no list and no box, the
//                      settings name the models they named at "settings",
//                      and with "finding" the line under Find clips with
//                      says a search is finding clips with it
//   ["asks"]           the gesture before it opened the box asking whether
//                      to remove, which Escape closes again
//   ["changed"]        the settings name another model than at "settings"
//   ["hearing"]        a search still hears, so what came before was
//                      about a search that had not come to the model yet
//   ["rendered"]       the clip says its short is in its episode's folder
//                      inside the folder the settings name for shorts, the
//                      file is there and the app serves it, Render says
//                      Render again and Show in folder is there
//   ["channel", name]  the Updates page's list says this, in ten seconds
//                      at most
//   ["choices", names, picked]
//                      the list offers these channels and nothing else, in
//                      this order, with this one picked, or none with ""
//   ["status", head, more]
//                      the line under the build comes to say this head, in
//                      ten seconds at most, and more somewhere after it
//   ["no check"]       the line under the build offers no button
//   ["mark", what]     the line under the build starts with this mark,
//                      "check", "triangle" or "dot", and ", warning" when
//                      it wears the colour of a warning
//   ["warned"]         the list wears the colour of a warning
//   ["turned", quarters]
//                      the engine has the clip's pieces rotated so many
//                      degrees each, in order, and with the playhead put in
//                      the middle of each by a click on the clip timeline,
//                      the video preview shows the piece rotated that far,
//                      the other way round from the video where a quarter
//                      turn says so, with no black along the top or the
//                      bottom of the picture, where a frame squeezed into
//                      the wrong shape leaves it, and the clip timeline
//                      marks the start of every rotated part
//   ["turned with its frames"]
//                      since "scrub", of a video standing up by its display
//                      matrix: every frame read off the canvas was turned
//                      as far as the Go side has the piece it belongs to,
//                      the way the short turns it, and a frame of a cut as
//                      far as the clip's rotations have its start, a few
//                      hundred frames of the rotated part and of the rest
//   ["upright"]        the short Render wrote, read back from disk, holds
//                      as many frames as the clip's pieces, and in every
//                      piece rotated a quarter each frame is the right
//                      frame of the episode, read across its top, while in
//                      the pieces not rotated most are not, since there it
//                      lies on its side
//   ["keyframe at", share]
//                      puts the playhead in the clip, this share of the
//                      way along it, with a click on the clip timeline, and
//                      presses C, and the clip timeline has to show a crop
//                      mark lit on the playhead's frame within 300 ms,
//                      before a held answer comes
//   ["thumbnail at", share]
//                      puts the playhead in the clip, this share of the
//                      way along it, with a click on the clip timeline, and
//                      presses T, and the engine has a thumbnail more
//   ["under the hand", kind]
//                      moves the pointer onto the first "thumbnail" or
//                      "crop" mark on the clip timeline: the mark is what
//                      is under the pointer, it takes the hover, and it
//                      lights the way it does under the hand
//   ["open a word, then the mark", kind]
//                      clicks the first "thumbnail" or "crop" mark on the
//                      clip timeline and reads the caption there, clicks
//                      the clip timeline at the other end of the clip and
//                      opens the first word of the caption there, then
//                      clicks the mark again: the playhead is on the mark,
//                      no word is open, and the caption box holds the
//                      caption on the mark again, not the one the word was
//                      opened in
//   ["frames on the pills", n]
//                      puts the playhead in the middle of each of the first
//                      n words with a click on the clip timeline, and reads
//                      the pill libass drew behind the lit word off the
//                      canvas: the frame the caption box puts round that
//                      word stands on it, its middle within 1.5 px of the
//                      pill's and as wide within 2 px, and the pill is no
//                      lower than the frame
//   ["remove mark", kind]
//                      double-clicks the first "thumbnail" or "crop" mark
//                      on the clip timeline, and the engine has one fewer,
//                      and no cut more
//   ["keyframe on", "last"]
//                      a click on the clip's end edge, which leaves the
//                      playhead on the end with the last frame on the
//                      video preview, then C: a keyframe comes on the
//                      clip's last frame and its diamond lights, and C again
//                      removes it
//   ["keyframe on", "first"]
//                      a click on the clip's start edge and three steps
//                      on and three back with the arrow keys, which leave
//                      the playhead a hair off the first frame's start
//                      with that frame on the video preview, then the same
//   ["keyframe past", edge]
//                      drags the first keyframe's diamond well past the
//                      clip's "end" or "start" edge and lets go: the engine
//                      has it on the clip's last or first frame, and the
//                      diamond never stood further out while it was held
//                      than where it stays once it is let go
//   ["drag crop", px]  drags the crop frame in the video preview sideways
//                      by px, in ten steps, and lets go
//   ["keyframes", n]  the engine has n keyframes on the clip, at as many
//                      left edges, and the clip timeline draws n
//                      diamonds, each 16 px wide
//   ["glides"]         with the playhead put by a click on the clip
//                      timeline on the first keyframe, a quarter, half
//                      and three quarters of the way to the next and on
//                      it, the crop frame in the video preview stands on
//                      each frame read off the canvas where smoothstep from
//                      the one mark to the next puts it, as the engine's
//                      render does
//   ["glides while playing"]
//                      plays from two fifths of the way from the first
//                      keyframe to the next for two seconds and
//                      reads the frame on the canvas and the crop frame on
//                      every animation frame: on each, the crop frame
//                      stands where the glide puts it for that frame, and
//                      it moves on many frames, not once
//   ["fits"]           the list's trigger is as wide as the channel it
//                      names, with no room kept for a longer one
//   ["at once", head]  in the frame after the pick, the list said the
//                      channel picked and the line under it this head
//
// The episode is the bridge's, so the words are the sentences its speech
// stand-in says: "Ich war vielleicht sechs Jahre alt, als mich auf dem
// Schulhof irgendein Typ geschubst hat."
import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, extname, join } from "node:path";
import { open, chosen, shown, engineState, handles, timeline, middle, high, drag } from "./bridge.mjs";
import { clipList, control, pressHead, fromSidebar, settle, ask, episodeOn, shortFrames, shortSound } from "./bridge.mjs";
import { xOf, seekTo, cropFrame, sameLook, sayLook, readShort, loudness, filmedOnScreen, preview } from "./bridge.mjs";
import { recordFrames, addHEVC, overlay, cropPixels, shortFrame } from "./bridge.mjs";
import { Watch, same, describe, overlaid } from "./rules.mjs";

export const sequences = [
  {
    name: "delete removes the framed word and nothing else",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["box", "Ich war sechs Jahre alt,"],
      ["spans", "start"],
    ],
  },
  {
    name: "delete removes the caption block clicked on the clip timeline",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["blocks"],
      ["mark", "removed"],
      ["undo"],
      ["same", "start"],
      ["blocks"],
      ["redo"],
      ["same", "removed"],
    ],
  },
  {
    name: "a caption's right edge dragged in over its last word removes the word, and back out puts it back",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["gone last", ["start", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
      ["undo"],
      ["same", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["edge out", ["end", "Schulhof"]],
      ["fewer", "start", 0],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    name: "a caption's left edge dragged in over its first word removes the word and leaves the caption before it alone",
    steps: [
      ["mark", "start"],
      ["edge in", ["start", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
      ["gone first", ["start", "Schulhof"]],
      ["edge out", ["start", "Schulhof"]],
      ["fewer", "start", 0],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    name: "a caption's right edge dragged in and double-clicked, twice, is back where it was with its words",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["edge put back", ["end", "Schulhof"]],
      ["same", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["edge put back", ["end", "Schulhof"]],
      ["same", "start"],
    ],
  },
  {
    name: "a caption block pops for neither edge dragged in, with the playhead on it",
    steps: [
      ["block", "Schulhof"],
      ["count pops"],
      ["edge in", ["start", "Schulhof"]],
      ["pops", 0],
      ["undo"],
      ["block", "Schulhof"],
      ["count pops"],
      ["edge in", ["end", "Schulhof"]],
      ["pops", 0],
    ],
  },
  {
    name: "a caption's edges moved by hand leave the captions beside them alone",
    steps: [
      ["mark", "start"],
      ["edge nudge", ["start", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
      ["edge nudge", ["end", "Schulhof"]],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    name: "removing a caption's last word draws it in like dragging its right edge over the word",
    steps: [
      ["mark", "start"],
      ["edge in", ["end", "Schulhof"]],
      ["mark", "dragged"],
      ["undo"],
      ["same", "start"],
      ["remove edge word", ["last", "Schulhof"]],
      ["drawn in", ["dragged", "Schulhof", "last"]],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    name: "removing a caption's first word draws it in like dragging its left edge over the word",
    steps: [
      ["mark", "start"],
      ["edge in", ["start", "Schulhof"]],
      ["mark", "dragged"],
      ["undo"],
      ["same", "start"],
      ["remove edge word", ["first", "Schulhof"]],
      ["drawn in", ["dragged", "Schulhof", "first"]],
      ["others", ["start", "Schulhof"]],
    ],
  },
  {
    name: "a caption block clicked, then a word walked to, delete removes only the word",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "Shift+ArrowRight"],
      ["press", "Backspace"],
      ["fewer", "start", 1],
      ["blocks"],
    ],
  },
  {
    name: "a caption block clicked, then the playhead moved a frame, delete removes nothing",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "ArrowRight"],
      ["press", "Backspace"],
      ["same", "start"],
      ["blocks"],
    ],
  },
  {
    name: "a caption block clicked and then let go of is not removed by delete",
    steps: [
      ["mark", "start"],
      ["block", "Schulhof"],
      ["press", "Escape"],
      ["press", "Delete"],
      ["same", "start"],
      ["blocks"],
    ],
  },
  {
    name: "a word saved empty is removed",
    steps: [
      ["click", "Jahre"],
      ["press", "ControlOrMeta+A"],
      ["press", "Backspace"],
      ["press", "Enter"],
      ["box", "Ich war vielleicht sechs alt,"],
    ],
  },
  {
    name: "undo puts a removed word back and redo takes it again",
    steps: [
      ["mark", "start"],
      ["frame", "sechs"],
      ["press", "Delete"],
      ["mark", "removed"],
      ["undo"],
      ["same", "start"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["redo"],
      ["same", "removed"],
    ],
  },
  {
    name: "a removed word typed back after the word before it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "war"],
      ["press", "End"],
      ["type", " vielleicht"],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a removed word typed back before the word after it is the word put back",
    steps: [
      ["mark", "start"],
      ["frame", "vielleicht"],
      ["press", "Backspace"],
      ["click", "sechs"],
      ["press", "Home"],
      ["type", "vielleicht "],
      ["press", "Enter"],
      ["same", "start"],
    ],
  },
  {
    name: "a word typed in beside another is a word of its own, removed alone",
    steps: [
      ["mark", "start"],
      ["click", "war"],
      ["press", "End"],
      ["type", " noch"],
      ["press", "Enter"],
      // A word more takes room, so the last word goes on to the next
      // caption. Adding may move words, removing may not.
      ["box", "Ich war noch vielleicht sechs Jahre"],
      ["click", "noch"],
      ["open", "noch"],
      ["press", "Escape"],
      ["press", "Backspace"],
      ["box", "Ich war vielleicht sechs Jahre alt,"],
      ["same", "start"],
    ],
  },
  {
    name: "Enter opens the framed word, wherever the playhead is",
    steps: [
      ["click", "Jahre"],
      ["press", "Escape"],
      ["press", "Enter"],
      ["open", "Jahre"],
    ],
  },
  {
    name: "a double-click in the clip cuts a part out, and one on the cut puts it back",
    steps: [
      ["mark", "start"],
      ["cut at", 0.5],
      ["cuts", 1],
      ["join", 1],
      ["cuts", 0],
      ["same", "start"],
    ],
  },
  {
    name: "a double-click on a trimmed edge puts it back where the clip was found",
    steps: [
      ["mark", "start"],
      ["trim", "end", -120],
      ["reset", "end"],
      ["same", "start"],
      ["trim", "start", 90],
      ["reset", "start"],
      ["same", "start"],
    ],
  },
  {
    name: "undo takes back a trim and then a cut, and redo does them again",
    steps: [
      ["mark", "start"],
      ["cut at", 0.4],
      ["mark", "cut"],
      ["trim", "end", -100],
      ["mark", "trimmed"],
      ["undo"],
      ["same", "cut"],
      ["undo"],
      ["same", "start"],
      ["redo"],
      ["same", "cut"],
      ["redo"],
      ["same", "trimmed"],
    ],
  },
  {
    // Plan row 2.189. Tim asked to talk into Report a Problem rather than
    // type: what he says appears while he says it, where the cursor was,
    // and Cmd+Z takes it back like anything typed, all of it at once. The
    // microphone is the bridge's, which plays a hum with a pause in it as
    // fast as it is said, and the speech model its stand-in. What came
    // before the pause is settled while it listens, the rest when it
    // stops.
    name: "Dictate writes what is said while it is said, where the cursor was, and undo takes it back",
    steps: [
      ["menu", "report"],
      ["write", "It froze.", 2],
      ["dictate", "button"],
      ["report reads", "It * froze."],
      ["undo"],
      ["report reads", "It froze."],
    ],
  },
  {
    // Plan row 2.189. Tim undid what he typed while he talked, and Cmd+Y
    // did not bring it back: the words that arrived meanwhile were steps
    // of their own, which emptied what there was to redo. They are not
    // steps any more, so undo and redo while talking are about what was
    // typed alone.
    name: "While dictating, undo and redo are about what was typed",
    steps: [
      ["menu", "report"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["type", "X"],
      ["undo"],
      ["dictate", "settled"],
      ["redo"],
      // X was typed at the end of what was said, so what is said next
      // comes after it, with a space on either side.
      ["dictate", "hearing"],
      ["report reads", "It * X * froze."],
      ["dictate", "enter"],
      ["report reads", "It * X * froze."],
    ],
  },
  {
    // Plan row 2.189. Tim asked for dictation the way claude.ai does it:
    // Enter keeps what was said, as the microphone does, and Escape
    // removes it.
    name: "Enter keeps what was dictated and Escape removes it",
    steps: [
      ["menu", "report"],
      ["write", "It froze.", 2],
      ["dictate", "enter"],
      ["report reads", "It * froze."],
      ["dictate", "escape"],
      ["report as before"],
    ],
  },
  {
    // Plan row 2.189. Cmd+Z and Cmd+Y come from the app's menu, which
    // the workspace answers with its own undo. On every other page they go
    // to the field being typed in, which did nothing before Report a
    // Problem needed it, in the settings too.
    name: "Undo and redo in a settings field undo and redo the typing",
    steps: [
      ["settings"],
      ["field", "input.licence-key", "FF-12"],
      ["undo"],
      ["field reads", "input.licence-key", ""],
      ["redo"],
      ["field reads", "input.licence-key", "FF-12"],
    ],
  },
  {
    // Plan row 2.189. Tim could not click into the words still being
    // heard, nor at the end of their line, to put the cursor there or to
    // correct a word while he talked. A click in them settles them where
    // they are, and what is heard after comes after them.
    name: "A click in the words being heard puts the cursor there while dictating",
    steps: [
      ["menu", "report"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["click heard"],
      ["type", "X"],
      ["dictate", "hearing"],
      ["dictate", "enter"],
      ["report reads", "It *X* * froze."],
    ],
  },
  {
    // Plan row 2.189. Tim typed a few words after what he had said and
    // talked on, and what he said next went in front of what he typed,
    // pushing it along. What is typed at the end of the words being said
    // goes after them, and what is said next goes after what was typed.
    name: "What is said after typing at the end of a dictation comes after the typing",
    steps: [
      ["menu", "report"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["dictate", "settled"],
      ["type", "Y"],
      ["dictate", "hearing"],
      ["dictate", "enter"],
      ["report reads", "It * Y * froze."],
    ],
  },
  {
    // Plan row 2.189. Tim typed at the end of what he said, moved the
    // cursor with the arrow keys and typed there, and talked on. On his
    // Mac the letters came out backwards, and Enter lost everything said
    // before he moved. What was said stays one piece wherever the person
    // types, and Enter keeps all of it, in order.
    name: "Typing elsewhere while dictating keeps every word, in order",
    steps: [
      ["menu", "report"],
      ["write", "It froze.", 2],
      ["dictate", "start"],
      ["dictate", "settled"],
      ["type", "Y"],
      ["press", "Home"],
      ["press", "ArrowRight"],
      ["press", "ArrowRight"],
      ["type", "Z"],
      ["dictate", "hearing"],
      ["dictate", "enter"],
      ["report reads", "ItZ * Y * froze."],
    ],
  },
  {
    // Plan row 2.189. Tim talked past the longest a recording can be and
    // lost what he said after it, with nothing to say why. A recording
    // nobody talks into stops by itself, keeps every word as Stop would,
    // and says why under the field. The bridge's waits 8 s.
    name: "Dictation nobody talks into stops by itself and keeps what was said",
    steps: [
      ["menu", "report"],
      ["write", "It froze.", 2],
      ["dictate", "ends"],
      ["report reads", "It * froze."],
      ["undo"],
      ["report reads", "It froze."],
    ],
  },
  {
    name: "a new video gets its first clips with no click",
    steps: [
      ["add", 60],
      ["wait for", "New"],
      ["cards", 1],
    ],
  },
  {
    // Plan row 2.181. Tim added a video and its clip timeline stayed bare
    // until its first clip came, where the waveform used to come within
    // seconds, measured on its own. The episode is heard slowly, so the
    // search is still hearing it when the waveform has to be there.
    name: "a video just added shows its waveform before its first clip",
    steps: [
      ["speech", 3000],
      ["add", 120],
      ["waveform"],
      ["cards", 0],
      ["speech", 0],
      ["wait for", "New"],
      ["cards", 1],
    ],
  },
  {
    // Plan row 2.177. Tim added a video of 15 s with Shortest at 20 s:
    // its first search failed, the range picker had no line on it and the
    // clip timeline showed a minute.
    name: "a video shorter than a clip is its own clip, with rulers that fit it",
    steps: [
      // Heard slowly, so the rulers are looked at with no clip yet, the
      // way Tim saw them once the search had failed.
      ["speech", 3000],
      ["add", 15],
      ["rulers fit", 15],
      ["speech", 0],
      ["wait for", "New"],
      ["cards", 1],
      ["whole clip", 15],
      ["rulers fit", 15],
    ],
  },
  {
    // Tim removed the one clip of a video of 15 s and pressed New, and
    // every search failed with "there is nothing to make a clip of in
    // this video": the clip removed still counted as the one moment the
    // video has. New brings it back, and New with it in the list finds
    // nothing more, fails nothing either, and says so where its clip
    // would have appeared.
    name: "a video shorter than a clip gets its clip again once it was removed",
    steps: [
      ["add", 15],
      ["wait for", "New"],
      ["whole clip", 15],
      ["remove clip"],
      ["head"],
      ["wait for", "New"],
      ["whole clip", 15],
      // The clip made again is the clip that was removed, so the removed
      // one is not offered back, and undoing its removal does not bring
      // it back beside the new one. Both showed the video twice.
      ["no row", "Nothing new found"],
      ["no put back"],
      ["undo refused"],
      ["whole clip", 15],
      ["head"],
      ["wait for", "New"],
      ["whole clip", 15],
      // Tim saw the row of that search close again with nothing to say.
      // It says so now, as news: it runs down like a removed clip's row
      // and closes.
      ["row closes", ["Nothing new found", 10]],
      // New again is a new search, and it says so again.
      ["head"],
      ["wait for", "New"],
      ["row closes", ["Nothing new found", 10]],
    ],
  },
  {
    name: "Cancel stops a search, and Continue carries it on to its clips",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "New"],
      ["cards", 1],
    ],
  },
  {
    // Plan row 2.131. Cancel is pressed before the Go side has heard of
    // the search New asked for, which is held on its way there, so the
    // Cancel reaches the Go side first. The search never starts, and says
    // Stopped with Continue where its clips would have been.
    name: "Cancel pressed at once after New stops the search before it starts",
    steps: [
      ["hold", "Search", 1500],
      ["new and cancel"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
    ],
  },
  {
    name: "a search the app was closed on says Interrupted, after it opens again",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["restart"],
      ["wait for", "Continue"],
      ["row", "Interrupted. Click Continue"],
    ],
  },
  {
    name: "a search that failed says so, and Continue tries again",
    steps: [
      ["model", "fails"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Failed. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "New"],
    ],
  },
  {
    // Tim's steps for #116, on a clip a search parted at a camera switch
    // into two pieces, each with the crop of its own shot.
    name: "a cut or an edge put back keeps the camera switch, and the short runs straight through it",
    steps: [
      ["add cameras", 30, 12],
      ["wait for", "New"],
      ["pieces", 2],
      ["mark", "found"],
      ["look", "found"],
      ["cut switch", 1],
      ["cuts", 1],
      ["join", 1],
      ["cuts", 0],
      ["framed", "found"],
      ["same", "found"],
      ["trim past", "end", 1],
      ["pieces", 1],
      ["reset", "end"],
      ["framed", "found"],
      ["same", "found"],
      ["render"],
      ["shots", "found"],
    ],
  },
  {
    // Found while 2.124 was proven: from a 29.97 fps episode every piece
    // of a short had a frame too many and about 30 ms of padded silence at
    // each cut, because a piece went to ffmpeg rounded to the millisecond.
    // TestARenderCutsOnWholeFrames holds the engine to it, this the app.
    name: "a render of a 29.97 fps episode cuts on whole frames",
    steps: [
      ["add", 40, "30000/1001"],
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["short", 2],
    ],
  },
  {
    // Bug 1 of docs/DESIGN-REVIEW.md: at 29.97 fps an edge kept to the
    // millisecond lies just before its frame's start for 1400 of every
    // 3000 frames, and the video preview drew the frame before the one the
    // short starts on. And a search left its edges between frames, where
    // the render took the frame whose start was nearer and the video
    // preview the frame that held the edge. The clip's edges here are the
    // search's own, its cuts are put by the hand, and at least one edge is
    // kept just before its frame's start. The picture starts after the
    // sound, so the frames are on a grid of their own, and the search's end
    // lies where the two rules used to part.
    name: "the video preview on a clip's edges shows the frames its short starts and ends on",
    steps: [
      ["add", 40, "30000/1001", "late"],
      ["wait for", "New"],
      ["cards", 1],
      ["trim", "start", 20],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["edges as rendered"],
    ],
  },
  {
    // Plan row 2.198, found by Tim: a video OBS recorded in Matroska was
    // added, searched and found clips, and the video preview said "The
    // file has no moov box, so it is not an MP4 or MOV it can read". It
    // read the file's frames with a reader of its own, mp4.ts, where
    // everything else reads through ffmpeg. The picture starts after the
    // sound, by Matroska's first timestamp, so the frames are on a grid
    // of their own, as in the walk on a clip's edges above.
    name: "a video in Matroska shows in the video preview the frames its short has",
    steps: [
      ["add", 40, "30000/1001", "late", "mkv"],
      ["wait for", "New"],
      ["cards", 1],
      ["trim", "start", 20],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["edges as rendered"],
    ],
  },
  {
    // Plan row 2.130: a phone records its frames at uneven times, which
    // the render counted as if they came every 1/29.97 s, and made the
    // short at the 50 frames a second ffmpeg reports for them. Now every
    // frame of the short is the frame the video preview shows for its
    // moment, at the frames' average rate rounded, 30.
    // TestARenderShowsTheFrameThatHoldsEachMoment holds the engine to it,
    // this the app.
    name: "a short of a phone's uneven frames shows what the video preview shows",
    steps: [
      ["add", 40, "30000/1001", "uneven"],
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      ["render"],
      ["as previewed", "30/1"],
    ],
  },
  {
    // Plan row 3.25: a phone turned while it recorded leaves a part of the
    // video lying on its side. The video here stands up by its display
    // matrix, so its frame number runs down its right side, and the part
    // after the cut, rotated a quarter to the left with R, reads across
    // its top again, in the video preview and in its short.
    // TestRotateTurnsThePartBetweenCutsAndFourTurnsBringItBack and
    // TestARotatedPieceIsRotatedBeforeItsCrop hold the engine to it, this
    // the app.
    name: "a part of a phone's video rotated with R stands up in the video preview and in its short",
    steps: [
      ["add", 30, "25", "turned"],
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.5],
      ["cuts", 1],
      ["hold", "RotatePart", 1000],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["render"],
      ["upright"],
    ],
  },
  {
    // Found by Tim on #188: dragging the playhead back over the cut before
    // a rotated part, the video preview turned the picture as the playhead
    // crossed the cut, before the frame from the other side had come, so a
    // frame stood turned the wrong way for a moment and the picture flipped
    // back and forth. And dragging the clip's end out past a rotated part
    // brought its frames back as filmed, not rotated.
    name: "a rotated part turns with its own frames under a dragged playhead, and stays rotated as its end moves",
    steps: [
      ["add", 30, "25", "turned"],
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.5],
      ["cuts", 1],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["scrub", 0.95, 0.05],
      ["turned with its frames"],
      ["scrub", 0.05, 0.95],
      ["turned with its frames"],
      ["trim", "end", -60],
      ["trim", "end", 40],
      ["turned", [0, 90]],
    ],
  },
  {
    // A rotation belongs to the video's time, not to the cuts: putting back
    // the cut before a rotated part leaves the footage after the turn
    // rotated and the footage before it as filmed, and making the cut
    // again finds the part as it was. The review of #188 found the
    // rotation lost when it was kept on each piece.
    name: "a rotated part keeps its rotation when the cut before it is put back and made again",
    steps: [
      ["add", 30, "25", "turned"],
      ["wait for", "New"],
      ["cards", 1],
      ["cut at", 0.5],
      ["cuts", 1],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["join", 1],
      ["cuts", 0],
      ["turned", [0, 90]],
      ["cut at", 0.5],
      ["cuts", 1],
      ["turned", [0, 90]],
    ],
  },
  {
    // Plan row 3.26: keyframes put with C, the second dragged to another
    // place, glide the crop from the one to the other, in the video
    // preview frame by frame while it plays as the render crops it.
    // TestTheRenderGlidesTheCropOverTheMarks holds the render to the same
    // glide, and glide.cases.json the window and the engine to each other.
    name: "keyframes put with C glide the crop from one to the next in the video preview, frame by frame",
    steps: [
      ["add", 30, "25"],
      ["wait for", "New"],
      ["cards", 1],
      ["hold", "SetCropMark", 1000],
      ["keyframe at", 0.2],
      ["keyframe at", 0.8],
      ["drag crop", 400],
      ["keyframes", 2],
      ["glides"],
      ["glides while playing"],
      ["under the hand", "crop"],
      ["remove mark", "crop"],
      ["keyframes", 1],
    ],
  },
  {
    // Found by Tim on #192: C put no mark on a clip's first or last frame,
    // only one frame in.
    name: "a keyframe goes on a clip's first and last frame with C, and comes off with C",
    steps: [
      ["add", 30, "25"],
      ["wait for", "New"],
      ["cards", 1],
      ["keyframe on", "last"],
      ["keyframe on", "first"],
    ],
  },
  {
    // Found on #192: Automatic crop took the keyframes of the angle away
    // with the crop, and Put the crop back brought back the crop alone,
    // so the glide was lost to one click.
    name: "Put the crop back after Automatic crop brings back the keyframes and their glide",
    steps: [
      ["add", 30, "25"],
      ["wait for", "New"],
      ["cards", 1],
      ["keyframe at", 0.2],
      ["keyframe at", 0.8],
      ["drag crop", 400],
      ["keyframes", 2],
      ["button", "Automatic crop"],
      ["keyframes", 0],
      ["button", "Put the crop back"],
      ["keyframes", 2],
      ["glides"],
    ],
  },
  {
    // Found by Tim on #192: a keyframe dragged past the clip's end went a
    // pixel past the clip edge while held and jumped back to the last
    // frame when let go. It was drawn on the middle of the frame under the
    // hand while held and on the start of the frame once saved.
    name: "a keyframe dragged past the clip's edge stops on its first or last frame and stays where it was let go",
    steps: [
      ["add", 30, "25"],
      ["wait for", "New"],
      ["cards", 1],
      ["keyframe at", 0.5],
      ["keyframe past", "end"],
      ["keyframe past", "start"],
    ],
  },
  {
    // Plan row 2.190, found by Tim on #188: a thumbnail mark did not
    // react to the pointer, and was removed only by putting the playhead
    // on it and pressing T. It is the keyframe's kind of mark, lit under
    // the hand and removed with a double-click, the way a cut is.
    name: "a thumbnail mark lights under the hand and a double-click removes it",
    steps: [
      ["add", 30, "25"],
      ["wait for", "New"],
      ["cards", 1],
      ["thumbnail at", 0.5],
      ["thumbnail at", 0.2],
      ["under the hand", "thumbnail"],
      ["remove mark", "thumbnail"],
    ],
  },
  {
    // Found by Tim on #201: with a word open in the caption box, a click
    // on a thumbnail's camera or a keyframe's diamond moved the frame and
    // left the caption box on the word's moment, since the marks take no
    // focus and the word kept it.
    name: "a click on a thumbnail or a keyframe while a word is open ends the correction, and the captions follow",
    steps: [
      ["add", 30, "25"],
      ["wait for", "New"],
      ["cards", 1],
      ["thumbnail at", 0.1],
      ["keyframe at", 0.9],
      ["open a word, then the mark", "thumbnail"],
      ["open a word, then the mark", "crop"],
    ],
  },
  {
    // Found by Tim on #201: the frame round a word in the caption box
    // stood off the word, since the browser laid the words out in its own
    // font over the words libass drew.
    name: "the frame round a word in the caption box is the pill libass draws",
    steps: [["frames on the pills", 5]],
  },
  {
    // Plan row 2.130: a picture that starts after its sound, by an edit
    // list. The clip's start is dragged to before the picture starts,
    // where the render took the picture's frames from its first on and the
    // sound from where the picture starts.
    name: "a short of a picture that starts after its sound shows what the video preview shows",
    steps: [
      ["add", 40, "30000/1001", "late"],
      ["wait for", "New"],
      ["cards", 1],
      ["trim", "start", -60],
      ["cut at", 0.3],
      ["cut at", 0.7],
      ["cuts", 2],
      // Plan row 2.137: edges counted from the start of the file stood
      // half a frame from the picture's own, which starts 0.25 s in.
      ["on frames"],
      ["render"],
      ["as previewed", "30000/1001", 0.5],
    ],
  },
  {
    // Found while 2.125 was proven: with a folder for shorts named in the
    // settings, which the bridge does, a clip never learnt it was
    // rendered, because only the episode's own out folder was looked in.
    // Render went on saying Render and Show in folder never came. After
    // a restart too, because where the short went is kept on disk.
    name: "a short rendered into the folder for shorts is known as rendered",
    steps: [["render"], ["rendered"], ["by its words"], ["restart"], ["rendered"]],
  },
  {
    // Plan row 2.133, found while 2.127 was fixed: a short is named after
    // its clip, and two episodes render into the one folder for shorts the
    // bridge names, so the clip of a second episode with the same name
    // wrote over the first episode's short, and the first episode's clip
    // went on saying it was rendered, over the other episode's short.
    name: "two episodes never overwrite each other's shorts",
    steps: [
      ["thumbnail"],
      ["render"],
      ["pictured"],
      ["keep", "first"],
      ["words again"],
      ["add", 120],
      ["wait for", "New"],
      ["cards", 1],
      ["render"],
      ["apart", "first"],
      ["rendered"],
    ],
  },
  {
    // Plan row 2.133: two episodes of the same file name share their
    // folder in the folder for shorts, so the second short of the same
    // name gets a number, the way Finder gives one.
    // The first clip has two pictures and the second none, so a render of
    // the second that went by the clip's name would take them away.
    name: "two episodes of one file name number their shorts",
    steps: [
      ["thumbnail"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["thumbnail"],
      ["render"],
      ["pictured"],
      ["keep", "first"],
      ["words again"],
      ["add namesake", 120],
      ["wait for", "New"],
      ["cards", 1],
      ["render"],
      ["apart", "first", " 2"],
      ["rendered"],
    ],
  },
  {
    // Plan row 2.133: Render again writes over the clip's own short, in
    // its place and under its name, with its pictures beside it, and
    // nothing else in the folder.
    name: "render again writes the short over in place, with its pictures beside it",
    steps: [
      ["thumbnail"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["thumbnail"],
      ["render"],
      ["rendered"],
      ["pictured"],
      ["keep", "first"],
      ["render"],
      ["in place", "first"],
      ["rendered"],
      ["pictured"],
    ],
  },
  {
    // Tim chose another model in the settings while a search ran, and
    // nothing stopped him. What a search uses stays as it is until it is
    // done, and once it is, it can be changed again.
    name: "a model a search finds clips with is not changed or removed under it",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["settings"],
      ["pick model"],
      ["refused", "finding"],
      ["remove model", "language"],
      ["refused", "finding"],
      ["remove model", "speech"],
      ["refused", "speech"],
      ["episode"],
      ["head"],
      ["wait for", "Continue"],
      ["settings"],
      ["remove model", "language"],
      ["asks"],
      ["pick model"],
      ["changed"],
    ],
  },
  {
    // Step 3 of docs/VIDEO-PREVIEW.md: Add asks the episode's decoder for a
    // file's first frame, and leaves out a file with no picture, with the
    // reason, where before it was taken and the video preview failed on it.
    name: "a file with sound and no picture is left out by Add, with the reason",
    steps: [["add sound alone"], ["left out", "nur-ton-"]],
  },
  {
    // Plan row 2.192. Tim asked for a video dropped anywhere on the app,
    // whatever page it is on, to do what Add does: it is added, it opens,
    // and its first search finds its clips with no click.
    name: "a video dropped on any page is added and opens, the way Add adds one",
    steps: [
      ["drop", 30, "settings"],
      ["dropped in front"],
      ["wait for", "New"],
      ["cards", 1],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["dropped in front"],
      ["drop again", "updates"],
      ["dropped in front"],
      ["no reason"],
    ],
  },
  {
    // Tim saw the play stutter, the playhead with it, as he dragged the
    // edge of a cut while the clip played. Only the space bar and a click
    // on the clip timeline change a play: an edge dragged while it plays
    // is saved and drawn, and the play goes on as it was.
    name: "an edge dragged while the clip plays changes nothing that plays",
    steps: [
      ["cut at", 0.6],
      ["cuts", 1],
      ["drag while playing", "cut", 40],
      ["played through"],
      ["press", "Space"],
      ["drag while playing", "end", -60],
      ["played through"],
    ],
  },
  {
    // Tim saw the captions stay up in the video preview while the episode
    // played on through the chosen clip, with the clip dimmed on the clip
    // timeline. On the video the video preview shows the video alone, and
    // on the clip its crop frame and its captions. The clip starts with
    // the episode, so its start is trimmed first to leave room before it.
    name: "the episode played through a clip shows only the video, and the clip its frame and captions",
    steps: [
      ["trim", "start", 300],
      ["play from", "before", 6],
      ["only the video"],
      ["play from", "in", 4],
      ["the clip shown"],
    ],
  },
  {
    // The same with cuts in the clip, which Tim asked for: the episode
    // plays straight through the parts cut out and the pieces either side
    // of them, on the video the whole way, and the clip jumps its cuts,
    // on the clip the whole way. Neither may show the clip on the video,
    // in a piece or in a cut, nor lose it on the clip after a jump.
    name: "the episode played through a clip with cuts shows only the video, and the clip its frame and captions",
    steps: [
      ["trim", "start", 300],
      ["cut at", 0.25],
      ["cut at", 0.55],
      ["cuts", 2],
      ["play from", "before", 13],
      ["only the video", "across cuts"],
      ["play from", "in", 9],
      ["the clip shown", "across cuts"],
    ],
  },
  {
    // Tim played the episode and pressed I to make a clip of what he was
    // watching, and the clip came up dimmed: no gesture had put the
    // playhead on it, so it counted as on the video. A clip made with I
    // or O is made around the playhead, so the playhead is on it from the
    // frame it is there, on its way and once it is written, and the play
    // goes on as it was.
    name: "a clip made with I while the episode plays is not dimmed",
    steps: [
      ["trim", "start", 300],
      ["make while playing", "i"],
      ["made on the clip"],
    ],
  },
  {
    name: "a clip made with O while the episode plays is not dimmed",
    steps: [
      ["trim", "start", 300],
      ["make while playing", "o"],
      ["made on the clip"],
    ],
  },
  {
    // Paused, the playhead is put on the clip's start while it is on its
    // way, and it was dimmed there too until it was written.
    name: "a clip made with I while the video is paused is not dimmed",
    steps: [
      ["trim", "start", 300],
      ["make while paused", "i"],
      ["made on the clip"],
    ],
  },
  {
    // Tim played the episode from before the clip and pressed shift and
    // the right arrow to skip ahead, and the clip lit up the moment a step
    // landed in it. A step while the episode plays on the video skips
    // through the episode and stays on the video, cuts and all. On the
    // clip the same keys walk its words and the play stays on the clip.
    name: "shift and the arrows skip ahead through a clip on the video, and walk its words on the clip",
    steps: [
      ["trim", "start", 300],
      ["cut at", 0.4],
      ["cuts", 1],
      ["play from", "before", 8, "Shift+ArrowRight"],
      ["only the video"],
      ["play from", "in", 4, "Shift+ArrowRight"],
      ["the clip shown"],
    ],
  },
  {
    // Tim pressed L a few times while the clip played, and the play went
    // on to the clip's end and stood there, but the sound of the clip's
    // start played on for about three seconds. Loop switched off once the
    // play has worked out the next time through, in the last four seconds
    // of a time through, stops the play at the end, sound and picture.
    // Then Tim pressed L quickly near the end and heard clicks: that fix
    // started the play over on each press. A press of L touches nothing
    // that plays, so the sound runs on without a click.
    name: "loop switched off near the end stops the play at the clip's end",
    steps: [
      ["loop off near the end", 2],
      ["stopped at its end"],
    ],
  },
  {
    // Tim pressed the clip timeline while the clip played, to get a drag
    // ready, and the play ran on from the press at once, away from the
    // hand. A press holds the play under the hand, like a nail, until the
    // hand lets go, and the play goes on from there. Plan row 2.158.
    name: "a press on the clip timeline while it plays holds the playhead under the hand",
    steps: [["press while playing", -60], ["nailed"]],
  },
  {
    // Tim saw the episode's first frames flicker in the video preview as an
    // episode opened, and then the playhead jump to its clip. An episode
    // with a clip opens on the clip, and nothing before the clip is drawn
    // on the way. The clip starts later than the episode does, so a frame
    // of the episode's start is a frame that should not be there.
    name: "an episode opens on its clip, with no frame of its start on the way",
    steps: [
      ["trim", "start", 160],
      ["add", 30],
      ["episode watched"],
      ["opened on the clip"],
    ],
  },
  {
    // The same from the first moment of a search, while it still hears a
    // video just added and has not come to the model yet.
    name: "a model is not changed or removed while a search still hears",
    steps: [
      ["speech", 3000],
      ["add", 120],
      ["wait for", "Cancel"],
      ["settings"],
      ["pick model"],
      ["refused", "finding"],
      ["remove model", "speech"],
      ["refused", "speech"],
      ["hearing"],
    ],
  },
  {
    name: "removing a word in the middle of a caption leaves the caption whole",
    steps: [
      ["mark", "start"],
      ["frame", "auf"],
      ["press", "Backspace"],
      ["box", "als mich dem Schulhof irgendein"],
      ["spans", "start"],
    ],
  },
  {
    // Plan row 2.211: Tim saw the highlight in the video preview with
    // padding round the word, settled, where the short had it mid-bounce
    // and the thumbnail had it with no padding at all. A browser drew the
    // preview's captions and libass the short's. Now libass draws both,
    // from the one caption file, see lib/frames/captions.ts.
    name: "the video preview shows the short's captions, frame for frame",
    steps: [["render"], ["captions like the short"]],
  },
  {
    // The video preview draws on a WebGPU canvas in the extended mode,
    // so an HDR episode shines on an HDR screen. Step 4 of 2.156.
    name: "the GPU draws standard video with QuickTime's look, a step for every value, and HDR as BT.2100's light",
    steps: [["screen light"]],
  },
  {
    // One engine: every frame comes from ffmpeg on the Go side, so a
    // webview with no decoder of its own plays the episode the same. Tim's
    // start.mp4, HEVC in 10-bit colour, was where it began: WebKit said it
    // would decode it and then failed on the first frame.
    name: "the picture comes from the Go side with no decoder in the browser",
    steps: [["no browser decoder"], ["from the app", 4]],
  },
  {
    // The video preview stayed black once the playhead had been to the end
    // of the episode, until the app started again: the cursor of the
    // episode's decoder that read to the end ended every stream after it
    // at once. Plan row 2.166.
    name: "the picture comes back after the playhead reached the end of the episode",
    steps: [["play to the end"], ["from the app", 4]],
  },
  {
    // Tim saw the first frame of a workspace blurred until he played, and
    // sharp once he came back to it: it was decoded before the canvas was
    // told its size, at the 300 by 150 a canvas has before that, and kept
    // at it. Plan row 2.187.
    name: "a workspace opens on a sharp frame, and again after the app restarts",
    steps: [["sharp", "opened"], ["restart"], ["sharp", "opened"]],
  },
  {
    // The frame on screen while paused is asked for again when the app
    // grows. The bridge's picture is 320 by 180, smaller than the video
    // preview of the smallest app, so the app is made smaller than it can
    // be first, for a frame smaller than the picture to grow from.
    name: "a paused frame grows sharp with the app",
    steps: [["app size", 700, 500], ["restart"], ["sharp", "opened"], ["app size", 1500, 1000], ["sharp"]],
  },
  {
    // Tim's start.mp4 stood without a picture and would not play, on
    // Space or Play, while every other video was fine. It is HEVC in open
    // GOPs, and the playhead was on a frame shown before the key frame it
    // is decoded after: the episode's decoder started a key frame late,
    // and the frame the video preview waited for never came. Plan row
    // 2.186.
    name: "a frame shown before its key frame comes and plays",
    steps: [["add hevc"], ["leading frames"]],
  },
  {
    // Tim recorded the range picker sawing up and down while he dragged
    // the app's edge, then the settings fields shaking: a height rounded
    // in steps of three left the picture the rest, and the columns beside
    // it took what the picture left, on a half pixel every other step.
    name: "the app dragged a pixel at a time moves every part one way, the sides equal and set by the width",
    steps: [["resize"]],
  },
  {
    // Tim moved the crop to the side of the picture and saw the viewer's
    // rounded corner cut into the crop frame's line, and bright specks of
    // the picture in the frame's own rounded corners, where the shade
    // beside it stopped short of them. An app this size makes the picture
    // fill the viewer, so its corners are the viewer's. Plan row 2.195.
    // Found by Tim on #192: the clip's frame on the clip timeline stood
    // just inside the track's border, with the grey of the border showing
    // above and below it, and the window's bars on the range picker stood
    // just outside it, with the border showing under them.
    name: "the clip's frame and the window's bars lie on their track's borders",
    steps: [["on borders"]],
  },
  {
    // Found by Tim on #192: in a narrow app the buttons under the clip
    // timeline gave way first and stood taller than they were wide.
    name: "the buttons under the clip timeline stay square in a narrow app",
    steps: [["square buttons"], ["app size", 900, 800], ["square buttons"]],
  },
  {
    name: "the crop frame at either side of the picture is drawn whole, with the shade into its corners",
    steps: [
      ["app size", 1000, 1000],
      ["frame whole"],
      ["crop to", "left"],
      ["frame whole"],
      ["crop to", "right"],
      ["frame whole"],
      ["rotate at", 0.5],
      ["crop to", "left"],
      ["frame whole", "turned"],
    ],
  },
  {
    // Tim made a build with make install, and its Updates page followed
    // pull request 143, picked by a build before it, and said it was not
    // checked yet with a Check that would never look by itself.
    name: "a build made on the Mac follows nothing until a channel is chosen, and then follows it",
    steps: [
      ["updates", { stored: "pr-143", list: "main,pr-143" }],
      ["channel", "Choose a channel"],
      ["choices", ["Branch main", "PR #143"], ""],
      ["status", "Built on this Mac", "It stays as it is until you choose a channel to follow."],
      ["no check"],
      ["follow", "Branch main"],
      ["at once", "Looking for a newer build"],
      ["status", "A newer build is ready", "Relaunch to finish updating"],
      ["channel", "Branch main"],
      ["choices", ["Branch main", "PR #143"], "Branch main"],
    ],
  },
  {
    name: "a build from a channel lists its channels and no Nothing",
    steps: [
      ["updates", { from: "main", list: "main,pr-20" }],
      ["status", "A newer build is ready"],
      ["choices", ["Branch main", "PR #20"], "Branch main"],
    ],
  },
  {
    name: "a pull request followed that closed says closed in the list and in the line under it",
    steps: [
      ["updates", { from: "main", stored: "pr-143", list: "main" }],
      ["status", "PR #143 is closed", "Nothing downloads until you choose what to follow next."],
      ["mark", "triangle, warning"],
      ["warned"],
      ["channel", "PR #143, closed"],
      ["choices", ["Branch main", "PR #143, closed"], "PR #143, closed"],
    ],
  },
  {
    // Tim asked for a pull request in conflict with main to be in the list
    // and say so, the way a closed one does. Its build is of the branch as
    // it is, so it can still be followed and installed.
    name: "a pull request in conflict with main says so in the list and can still be followed",
    steps: [
      ["updates", { from: "main", list: "main,pr-20,pr-21", conflict: "pr-20" }],
      ["choices", ["Branch main", "PR #21", "PR #20, conflicts"], "Branch main"],
      ["fits"],
      ["follow", "PR #20, conflicts"],
      ["status", "A newer build is ready", "Relaunch to finish updating"],
      ["channel", "PR #20, conflicts"],
      ["mark", "dot, warning"],
      ["warned"],
    ],
  },
];

// The Updates page: what its list says, and the line under the build. It
// is read in the page, so it can be read in the frame after a pick, see
// "follow".
function readUpdates() {
  const text = (el) => (el?.textContent ?? "").replace(/\s+/g, " ").trim();
  const line = document.querySelectorAll(".card.asks > .item")[1];
  return {
    channel: text(document.querySelector("#channel .said > span:not(.room)")),
    head: text(line?.querySelector(".head")),
    more: text(line?.querySelector(".words .muted")),
    button: text(line?.querySelector("button")),
    mark: markOf(line?.querySelector(".mark")),
    warned: !!document.querySelector("#channel")?.classList.contains("warn"),
    // What the trigger has beside the name it says: its padding, the gap
    // and the mark, about 54 pixels, and more once room is kept for a
    // longer name.
    slack: (() => {
      const trigger = document.querySelector("#channel");
      const said = trigger?.querySelector(".said > span:not(.room)");
      return trigger && said ? trigger.getBoundingClientRect().width - said.getBoundingClientRect().width : -1;
    })(),
  };

  // The mark by its shape, and whether it is drawn in the colour of a
  // warning, read from what is painted rather than from a class.
  function markOf(mark) {
    if (!mark) return "";
    const probe = document.createElement("span");
    probe.style.color = "var(--warn)";
    document.body.append(probe);
    const warn = getComputedStyle(probe).color;
    probe.remove();
    const dot = mark.querySelector(".dot");
    const shape = dot ? "dot" : mark.querySelector('svg path[d$="Z"]') ? "triangle" : mark.querySelector("svg") ? "check" : "";
    const colour = dot ? getComputedStyle(dot).backgroundColor : getComputedStyle(mark).color;
    return colour === warn ? `${shape}, warning` : shape;
  }
}
const updatesPage = (page) => page.evaluate(readUpdates);

// Waits up to ten seconds for the Updates page to come to what ok says,
// and hands back what it said last.
async function updatesComeTo(page, ok) {
  let now = await updatesPage(page);
  for (let i = 0; i < 100 && !ok(now); i++) {
    await page.waitForTimeout(100);
    now = await updatesPage(page);
  }
  return now;
}

// The workspace's parts, where they stand and how big they are, as the
// stylesheet lays them out for the app's size.
const workspaceParts = (page) =>
  page.evaluate(() => {
    const box = (s) => document.querySelector(s)?.getBoundingClientRect();
    const parts = {
      viewer: box(".viewer"),
      picture: box(".screen"),
      "range picker": box(".track[aria-label='The range picker']"),
      "clip timeline": box(".track[aria-label='The clip timeline']"),
      settings: box(".stage > :first-child"),
      clips: box(".stage > :last-child"),
      "a settings field": box(".settings input"),
      "a clip card": box("aside ol li"),
    };
    const out = {};
    for (const [k, b] of Object.entries(parts)) if (b) out[k] = { x: b.left, y: b.top, w: b.width, h: b.height };
    const rows = [...document.querySelectorAll(".detail .row")].map((e) => e.getBoundingClientRect().bottom);
    out.app = { w: innerWidth, h: innerHeight, foot: Math.max(...rows) };
    return out;
  });

// What is wrong with the workspace as the app is dragged smaller a pixel at
// a time, by its height and then by its width, or null.
async function resizeProblem(page) {
  const sweeps = [
    // Down to the app's smallest, 960 by 640, see main.go.
    { by: "height", sizes: Array.from({ length: 361 }, (_, i) => [1500, 1000 - i]) },
    { by: "width", sizes: Array.from({ length: 741 }, (_, i) => [1700 - i, 1000]) },
  ];
  const sideColumns = await page.evaluate(() => {
    const s = document.querySelector(".stage");
    return s ? getComputedStyle(s).gridTemplateColumns : "";
  });
  if (!sideColumns) return "there is no workspace to resize";
  for (const { by, sizes } of sweeps) {
    const seen = [];
    for (const [width, height] of sizes) {
      await page.setViewportSize({ width, height });
      const parts = await workspaceParts(page);
      // The two sides measured from the app's own edges, the rail and a
      // space in front of the settings and the edge after the clips, so
      // the picture stands in the middle of the app.
      const [left, right] = await page.evaluate(() => {
        const [l, , r] = getComputedStyle(document.querySelector(".stage")).gridTemplateColumns.split(" ").map(parseFloat);
        const token = (n) => parseFloat(getComputedStyle(document.documentElement).getPropertyValue(n));
        return [token("--rail") + token("--gap") + l, r + token("--edge")];
      });
      if (Math.abs(left - right) > 1) return `at ${width} by ${height} the sides beside the picture are ${left} and ${right} wide from the app's edges`;
      for (const [k, b] of Object.entries(parts)) {
        if (k === "app") continue;
        for (const [m, v] of Object.entries(b)) {
          if (Math.abs(v - Math.round(v)) > 0.01) return `at ${width} by ${height} the ${k}'s ${m} is ${v}, between two pixels`;
        }
      }
      const edge = parts.app.w - (parts.clips.x + parts.clips.w);
      const foot = parts.app.h - parts.app.foot;
      seen.push({ parts, left, edge, foot, width, height });
    }
    const first = seen[0];
    for (const step of seen) {
      if (step.edge !== first.edge) return `at ${step.width} by ${step.height} the clips end ${step.edge} pixels from the app's right, not ${first.edge}`;
      if (step.foot !== first.foot) return `at ${step.width} by ${step.height} the workspace ends ${step.foot} pixels from the app's foot, not ${first.foot}`;
      if (by === "height" && step.left !== first.left) return `the sides beside the picture went from ${first.left} to ${step.left} wide while only the height changed`;
    }
    for (const k of Object.keys(first.parts)) {
      if (k === "app") continue;
      for (const m of ["x", "y", "w", "h"]) {
        let up = 0;
        let down = 0;
        for (let i = 1; i < seen.length; i++) {
          const d = seen[i].parts[k][m] - seen[i - 1].parts[k][m];
          if (d > 0.01) up++;
          if (d < -0.01) down++;
        }
        if (up && down) return `dragging the app's ${by}, the ${k}'s ${m} moved ${down} times one way and ${up} times the other`;
      }
    }
  }
  await page.setViewportSize({ width: 1500, height: 1000 });
  return null;
}

// The models the settings name: what finds clips, here or in the cloud,
// and what hears.
async function modelsNamed(page) {
  const s = await ask(page, "GetSettings");
  return { planner: s.planner, llmModel: s.llmModel, apiModel: s.apiModel, asrModel: s.asrModel };
}

// Where the clip's shots meet: every place one piece runs straight into
// the next, with nothing cut out between them.
const switches = (pieces) =>
  pieces.slice(1).filter((p, i) => Math.abs(p.start - pieces[i].end) < 0.0005).map((p) => p.start);

const pieceCount = (n) => `${n} piece${n === 1 ? "" : "s"}`;

// Frames a second of the episode on screen.
const rate = async (page) => (await ask(page, "Source", await episodeOn(page))).fps;

// How far the captions in the crop of the video preview are from a frame
// of the short at the same size, in brightness, on average over every
// pixel and at most, in steps of 8 bits. The captions are the rows of the
// lower half of the short that have white text in them, and a few pixels
// round them. The short is given the video preview's look first, see lift
// in lib/frames/light.ts. The two are laid on each other a pixel either
// way too and the closest is taken: libass puts a drawing on whole pixels,
// and the short, made smaller to compare, lands half a pixel from where
// the preview draws it at the size it shows it.
const look = (v) => {
  const c = Math.min(Math.max(v / 255, 0), 1);
  return 255 * Math.pow(c, Math.max(0.891, 0.98 - 0.31 * c));
};
function captionDiff(rgba, rgb, w, h, channels = 4) {
  const luma = (d, i, n, f) => 0.2126 * f(d[n * i]) + 0.7152 * f(d[n * i + 1]) + 0.0722 * f(d[n * i + 2]);
  const a = new Float32Array(w * h);
  const b = new Float32Array(w * h);
  for (let i = 0; i < w * h; i++) {
    a[i] = luma(rgba, i, channels, channels === 3 ? look : (v) => v);
    b[i] = luma(rgb, i, 3, look);
  }
  let top = h;
  let bottom = -1;
  for (let y = Math.floor(h / 2); y < h; y++) {
    for (let x = 0; x < w; x++) {
      if (luma(rgb, y * w + x, 3, (v) => v) > 200) {
        top = Math.min(top, y);
        bottom = Math.max(bottom, y);
      }
    }
  }
  if (bottom < 0) return { mean: Infinity, most: Infinity };
  let best = { mean: Infinity, most: Infinity };
  for (let dy = -1; dy <= 1; dy++) {
    for (let dx = -1; dx <= 1; dx++) {
      let sum = 0;
      let most = 0;
      let n = 0;
      for (let y = Math.max(1, top - 8); y < Math.min(h - 1, bottom + 8); y++) {
        for (let x = 1; x < w - 1; x++) {
          const d = Math.abs(a[y * w + x] - b[(y + dy) * w + x + dx]);
          sum += d;
          most = Math.max(most, Math.round(d));
          n++;
        }
      }
      if (sum / n < best.mean) best = { mean: sum / n, most };
    }
  }
  return best;
}

// What is wrong with the short Render wrote, against the video preview, or
// null. The frame of the short that stands for a moment is the frame of
// the episode the video preview shows there: the last one to begin at or
// before it. A short is made at one rate, and the episode's frames, on a
// phone, at uneven times, so a frame of the short stands for the moment
// it starts at, on the grid of the rate from where the picture starts, see
// OnFrames and cutOf in engine/render.go, and the playhead is put where
// the preview shows the frame that holds that moment: after it, and
// before the episode's next frame begins. Which frame begins where is read
// from the episode with ffprobe. The clip timeline is pinched in for it,
// and a frame whose time is still too short to click on is passed over,
// but every piece is looked at where it starts and where it ends.
// The render takes a frame that begins up to a millisecond after a moment
// for that moment, frameHair, and so does the video preview, so the
// playhead goes after that too and ends a millisecond and a half before
// the next frame begins.
async function asPreviewed(page, rate, before) {
  const on = await chosen(page);
  const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
  const { fps } = await ask(page, "Source", on.path);
  const short = await shortOf(page, on);
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const probe = (args) => execFileSync("ffprobe", ["-v", "error", ...args, "-of", "csv=p=0"]).toString().trim();
  const got = probe(["-select_streams", "v:0", "-show_entries", "stream=r_frame_rate", short]);
  if (got !== rate) return `the short has ${got} frames a second, not ${rate}`;
  const begins = probe(["-select_streams", "v:0", "-show_entries", "packet=pts_time", on.path])
    .split(/\s+/).map(Number).filter((t) => Number.isFinite(t)).sort((a, b) => a - b);
  const fileStart = Number(probe(["-show_entries", "format=start_time", on.path])) || 0;
  const v0 = Math.max(0, begins[0] - fileStart);
  const pieces = entry.segments;
  if (before !== undefined && !(pieces[0].start < before && pieces[0].start < v0)) {
    return `the clip starts at ${pieces[0].start.toFixed(3)}, not before the picture at ${v0.toFixed(3)} and ${before}`;
  }
  // The pieces as the render takes them, see OnFrames.
  const grid = (t) => v0 + Math.round((t - v0) * fps) / fps;
  const slots = [];
  pieces.forEach((p, i) => {
    let start = grid(p.start);
    if (start < 0) start += 1 / fps;
    const end = Math.max(grid(p.end), start + 1 / fps);
    const n = Math.round((end - start) * fps);
    for (let q = 0; q < n; q++) slots.push({ piece: i, q, n, at: start + q / fps });
  });
  const { frames } = shortFrames(short);
  if (frames.length !== slots.length) return `the short has ${frames.length} frames, its pieces last ${slots.length}`;
  const after = (t) => begins.find((b) => b > t + 0.001) ?? Infinity;
  // Up close, pinched in to a tenth, so a frame is some pixels wide and
  // the edges of the pieces, which a click takes hold of, cover less than
  // one.
  const track = (await handles(page)).track;
  await page.mouse.move(track.x + track.w / 2, high(track));
  await page.keyboard.down("Control");
  await page.mouse.wheel(0, -230);
  await page.keyboard.up("Control");
  await page.waitForTimeout(300);
  const problems = [];
  let looked = 0;
  let missed = 0;
  const seenAt = new Set();
  for (let j = 0; j < slots.length; j++) {
    const { q, n, at } = slots[j];
    if (!(q < 3 || q >= n - 3 || q % 5 === 0)) continue;
    // Where the frame that holds the moment is the one on screen: from the
    // moment until the episode's next frame begins.
    const from = at + 0.0015;
    const to = after(at) - 0.0015;
    if (to - from < 0.002) continue;
    // Moved along with two fingers until the moment is well inside.
    for (let tries = 0; tries < 6; tries++) {
      const track = (await handles(page)).track;
      const off = (await xOf(page, (from + to) / 2)) - (track.x + track.w / 2);
      if (Math.abs(off) < track.w / 4) break;
      await page.mouse.move(track.x + track.w / 2, high(track));
      await page.mouse.wheel(off, 0);
      await page.waitForTimeout(150);
    }
    const g = await handles(page);
    const x0 = await xOf(page, from);
    const x1 = await xOf(page, to);
    // Clear of the clip's edges and of the edges of its cuts, which a
    // click takes hold of instead.
    const grips = [g.start, g.end, ...g.cuts, ...g.cutEdges].map((r) => [r.x - 3, r.x + r.w + 3]);
    if (x1 - x0 < 0.2) continue;
    // A click lands where the clip timeline's own track says, which can be
    // a pixel or two from where xOf reckons, so it is put right from where
    // the playhead went, as a hand would.
    const target = (from + to) / 2;
    let x = (x0 + x1) / 2;
    let placed = NaN;
    for (let tries = 0; tries < 4; tries++) {
      if (grips.some(([a, b]) => x > a && x < b)) break;
      await page.mouse.click(x, high(g.track));
      await settle(page);
      placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
      if (placed >= from && placed <= to) break;
      x += ((target - placed) * (x1 - x0)) / Math.max(to - from, 1e-6);
    }
    if (!(placed >= from && placed <= to)) {
      if (!Number.isNaN(placed)) missed++;
      continue;
    }
    looked++;
    seenAt.add(`${slots[j].piece} ${q < 3 ? "first" : q >= n - 3 ? "last" : "inside"}`);
    await filmedOnScreen(page);
    // The frame the preview shows, once it has settled on it: the short's
    // frame as soon as it comes, else what it stays on.
    const came = await page
      .waitForFunction((want) => window.__filmed?.() === want, frames[j], { polling: 50, timeout: 3000 })
      .then(() => true, () => false);
    const seen = came ? frames[j] : await filmedOnScreen(page);
    if (!came || (await filmedOnScreen(page)) !== frames[j]) {
      problems.push(`frame ${j} of the short, for ${at.toFixed(4)} s, is frame ${frames[j]} of the episode, the video preview shows ${seen} at ${placed.toFixed(4)}`);
    }
  }
  // Every piece looked at where it starts and where it ends, so at both
  // ends of the clip and on both sides of every cut, and inside.
  const unseen = pieces.flatMap((_, i) => ["first", "last"].filter((w) => !seenAt.has(`${i} ${w}`)).map((w) => `the ${w} frames of piece ${i + 1}`));
  if (unseen.length) problems.push(`the playhead could not be put on ${unseen.join(", ")}`);
  if (looked < 20) problems.push(`only ${looked} frames could be looked at, the playhead missed ${missed}`);
  console.log(`      ${looked} frames of ${slots.length} looked at, the playhead missed ${missed} more`);
  return problems.length ? problems.slice(0, 8).join("\n") + (problems.length > 8 ? `\nand ${problems.length - 8} more` : "") : null;
}

// What is wrong with the frames the video preview draws on the clip's
// edges, against the short Render wrote, or null. The playhead is put on
// each edge the way a hand puts it there, a click on the clip's start, on
// where each cut ends and on the clip's end, and the frame the video
// preview draws, read off the canvas, is the frame the short starts that
// piece with, or the short's last at the clip's end. Bug 1 of
// docs/DESIGN-REVIEW.md. An edge is kept to the millisecond, and at 29.97
// fps that lies just before its frame's start for 1400 of every 3000
// frames, where the video preview drew the frame before the one the short
// starts on. And a search left its edges between frames. The clip's end
// has to be where the search put it, and at least one edge just before
// its frame's start, or the walk is not about the case.
async function edgesAsRendered(page) {
  const on = await chosen(page);
  const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
  const { fps, videoStart = 0 } = await ask(page, "Source", on.path);
  const short = await shortOf(page, on);
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const { frames } = shortFrames(short);
  const pieces = entry.segments;
  // The frame an edge means, the one whose start is nearest, and each
  // piece's frames from there, the way the render cuts them, see
  // nearestFrame and cutOf in engine/render.go.
  const nearest = (t) => Math.round((t - videoStart) * fps);
  const firsts = [];
  let held = 0;
  for (const p of pieces) {
    const first = nearest(p.start);
    firsts.push(held);
    held += Math.max(nearest(p.end), first + 1) - first;
  }
  if (frames.length !== held) return `the short has ${frames.length} frames, its pieces hold ${held}`;
  // The clip's start and where each cut ends. Two pieces that meet, at a
  // camera switch or where the picture starts, have no cut between them.
  const edges = [{ what: "the clip's start", label: "Clip start", at: pieces[0].start, want: frames[firsts[0]] }];
  for (let i = 1; i < pieces.length; i++) {
    if (pieces[i].start <= pieces[i - 1].end) continue;
    edges.push({ what: `where cut ${i} ends`, label: `Where cut ${i} ends`, at: pieces[i].start, want: frames[firsts[i]] });
  }
  edges.push({ what: "the clip's end", label: "Clip end", at: pieces[pieces.length - 1].end, want: frames[frames.length - 1] });
  // Kept just before the start of the frame it means, by the rounding to
  // the millisecond.
  const before = edges.filter((e) => {
    const begins = videoStart + nearest(e.at) / fps;
    return e.at < begins - 1e-9 && begins - e.at < 0.0006;
  });
  if (!before.length) {
    return `no edge of the clip, at ${edges.map((e) => e.at).join(", ")}, lies just before its frame's start, the case this is about`;
  }
  if (Math.abs(entry.found[1] - pieces[pieces.length - 1].end) > 1e-9) {
    return `the clip ends at ${pieces[pieces.length - 1].end}, not at ${entry.found[1]} where the search put it, the other case this is about`;
  }
  await filmedOnScreen(page);
  const problems = [];
  for (const e of edges) {
    // High on the track, where the captions' edges below are not.
    const grip = await page
      .locator(`.clip-timeline [aria-label="${e.label}"]`)
      .boundingBox({ timeout: 3000 })
      .catch(() => null);
    if (!grip) {
      problems.push(`there is no ${e.label} to click`);
      continue;
    }
    await page.mouse.click(grip.x + grip.width / 2, high((await handles(page)).track));
    await settle(page);
    const placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
    if (Math.abs(placed - e.at) > 1e-6) {
      problems.push(`a click on ${e.what} put the playhead at ${placed.toFixed(4)}, not ${e.at.toFixed(4)}`);
      continue;
    }
    const came = await page
      .waitForFunction((want) => window.__filmed?.() === want, e.want, { polling: 50, timeout: 3000 })
      .then(() => true, () => false);
    if (!came) {
      problems.push(`on ${e.what} at ${e.at.toFixed(3)} s the video preview shows frame ${await filmedOnScreen(page)}, the short frame ${e.want}`);
    }
  }
  console.log(`      ${edges.length} edges looked at, ${before.length} kept just before their frame`);
  return problems.length ? problems.join("\n") : null;
}

// The shorts in a folder, by name, finished ones only.
const shortsIn = (dir) => readdirSync(dir).filter((n) => n.endsWith(".mp4") && !n.endsWith(".part.mp4")).sort();

// The pictures beside a short, <name>-1.jpg and on, in their order.
function picturesOf(short) {
  const stem = basename(short, ".mp4");
  const mine = new RegExp(`^${stem.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}-(\\d+)\\.jpg$`);
  return readdirSync(dirname(short))
    .map((n) => [n, mine.exec(n)])
    .filter(([, m]) => m)
    .sort((a, b) => Number(a[1][1]) - Number(b[1][1]))
    .map(([n]) => join(dirname(short), n));
}

// Where the clip's short is, as the clip says, or empty when it has none.
async function shortOf(page, at) {
  const clip = (await ask(page, "Clips", at.path)).find((c) => c.plan === at.plan && c.id === at.clip);
  return clip?.rendered ?? "";
}

// What is wrong with the short Render wrote for a clip, read back from
// disk, or null. Its pieces are cut the way the render cuts them, see
// cutOf in engine/render.go: the frames from the one a piece's start is on
// up to the one before its end, at least one. Which frame of the episode a
// frame of the short is comes from its bands, see shortFrames, and is
// compared with the frame it should be, so a frame too many, one lost or
// one from the wrong place shows. The episode's sound is a steady tone, so
// padded silence at a cut, or sound that runs on after its picture, shows.
// The engine's own test of this is TestARenderCutsOnWholeFrames.
function shortProblem(entry, short, fps, cutsWanted) {
  if (!short || !existsSync(short)) return `the clip has no short${short ? ` at ${short}` : ""}`;
  const pieces = entry.segments;
  const problems = [];
  // The case is the one it says: a clip with cuts whose edges, on the
  // frames of the episode where the render puts them, fall between
  // milliseconds, where rounding to one moved a piece by a frame. The
  // engine keeps an edge where a word starts or ends, which here is a
  // whole millisecond, so it is the frame it lands on that counts.
  const edges = pieces.slice(1).flatMap((p, i) => [pieces[i].end, p.start]).map((t) => Math.round(t * fps) / fps);
  if (pieces.length - 1 < cutsWanted) problems.push(`the clip has ${pieces.length - 1} cuts, not ${cutsWanted}`);
  const whole = edges.filter((t) => Math.abs(t * 1000 - Math.round(t * 1000)) < 0.01);
  if (whole.length) problems.push(`cuts on frames that start on a whole millisecond, ${whole.join(" ")}, not the case this is about`);

  // Seen: the frames of each piece in order, each once, and nothing else.
  const want = [];
  const cuts = [];
  for (const p of pieces) {
    const first = Math.round(p.start * fps);
    const end = Math.max(Math.round(p.end * fps), first + 1);
    for (let k = first; k < end; k++) want.push(k);
    cuts.push(want.length / fps);
  }
  cuts.pop();
  const { frames, modulo } = shortFrames(short);
  if (frames.length !== want.length) problems.push(`the short has ${frames.length} frames, its pieces hold ${want.length}`);
  let wrongFrames = 0;
  for (let i = 0; i < Math.min(frames.length, want.length); i++) {
    if (frames[i] !== want[i] % modulo && ++wrongFrames <= 5) {
      problems.push(`frame ${i} of the short is frame ${frames[i]} of the episode, not ${want[i]}`);
    }
  }
  if (wrongFrames > 5) problems.push(`and ${wrongFrames - 5} frames more are the wrong frame`);

  // Heard: as long as the picture, the tone right up to each cut, and
  // quiet there only for the fade out and in, 15 ms, Fade in
  // engine/render.go, and that quiet where the picture cuts.
  const rate = 48000;
  const samples = shortSound(short);
  const heard = samples.length / rate;
  const pictured = want.length / fps;
  if (Math.abs(heard - pictured) > 0.025) {
    problems.push(`the sound is ${heard.toFixed(3)} s long, the picture ${pictured.toFixed(3)} s`);
  }
  // The loudest sample in the 2.5 ms around each one, more than one swing
  // of the tone, so how loud the tone is there.
  const loud = (k) => {
    let most = 0;
    for (let j = Math.max(0, k - 60); j < Math.min(samples.length, k + 60); j++) most = Math.max(most, Math.abs(samples[j]));
    return most;
  };
  const steady = loud(Math.round(0.5 * rate));
  cuts.forEach((cut, n) => {
    const from = Math.round((cut - 0.06) * rate);
    const to = Math.round((cut + 0.06) * rate);
    if (to > samples.length) {
      problems.push(`the sound ends before cut ${n + 1} at ${cut.toFixed(3)} s`);
      return;
    }
    let first = -1;
    let last = -1;
    for (let k = from; k < to; k++) {
      if (loud(k) < 0.25 * steady) {
        if (first < 0) first = k;
        last = k;
      }
    }
    if (first < 0) {
      problems.push(`cut ${n + 1} at ${cut.toFixed(3)} s has no fade in its sound`);
      return;
    }
    const quiet = (last - first) / rate;
    const mid = (first + last) / 2 / rate;
    if (quiet > 0.015) problems.push(`the sound is quiet for ${(1000 * quiet).toFixed(1)} ms at cut ${n + 1}, more than its fade`);
    if (Math.abs(mid - cut) > 0.003) {
      problems.push(`the sound cuts at ${mid.toFixed(4)} s, the picture at ${cut.toFixed(4)} s, cut ${n + 1}`);
    }
  });
  if (!problems.length) return null;
  const said = pieces.map((p) => `${p.start}-${p.end}`).join(" ");
  return `${problems.join("\n")}\npieces ${said} at ${fps} fps, the short ${short}`;
}

const url = process.env.BRIDGE_URL ?? "http://127.0.0.1:8123/";
const only = process.argv[2] ?? "";
let failures = 0;

// --names lists the sequences, one name a line, for make walks to hand
// out one at a time to as many bridges as it runs, see TestWalks. SEQUENCE
// walks the one of that name and no other, where a part of a name could
// be a part of two.
if (only === "--names") {
  const names = sequences.map((q) => q.name);
  const twice = names.filter((n, i) => names.indexOf(n) !== i);
  if (twice.length) {
    console.log(`two sequences are called ${JSON.stringify(twice[0])}`);
    process.exit(1);
  }
  console.log(names.join("\n"));
  process.exit(0);
}
const exact = process.env.SEQUENCE;
const toWalk = sequences.filter((q) => (exact ? q.name === exact : q.name.includes(only)));
if (exact && !toWalk.length) {
  console.log(`no sequence is called ${JSON.stringify(exact)}`);
  process.exit(1);
}

for (const seq of toWalk) {
  const began = performance.now();
  const { browser, page, errors } = await open(url);
  const at = await chosen(page);
  const watch = new Watch(page, at, errors);
  await watch.start();
  const marks = {};
  const looks = {};
  const kept = {};
  // The models the settings name, at "settings", and the episode that was
  // on screen before them.
  let named = null;
  let episodeBefore = "";
  // The Updates page in the frame after a channel was picked.
  let picked = null;
  // What the video preview laid over the picture on every animation frame
  // of the last "play from", and the clip the engine had then.
  let played = null;
  let made = null;
  let loopEnd = null;
  let wrong = null;
  // The video and the other file dropped last.
  let droppedVideo = "";
  let droppedOther = "";
  const done = [];
  const word = (text) => page.locator(".captions .word").filter({ hasText: new RegExp(`^${text}$`) }).first();

  for (const [what, ...args] of seq.steps) {
    const arg = args.length > 1 ? args : args[0];
    if (wrong || watch.failed) break;
    done.push(arg === undefined ? what : `${what} ${JSON.stringify(arg)}`);
    const s = await shown(page);
    switch (what) {
      case "frame": {
        for (let i = 0; i < 80 && !(await word(arg).count()); i++) {
          await page.keyboard.press("Shift+ArrowRight");
          await page.waitForTimeout(100);
        }
        await word(arg).click();
        await page.keyboard.press("Escape");
        await watch.step("frame", s);
        break;
      }
      case "block": {
        const now = await engineState(page, watch.at);
        const cue = now.captions.find((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(arg))));
        if (!cue) {
          wrong = `no caption has "${arg}"`;
          break;
        }
        await page.locator(`.clip-timeline .caption[data-first="${cue.first}"]`).first().click();
        await watch.step("block", s);
        break;
      }
      case "edge in":
      case "edge out":
      case "edge nudge": {
        // An edge of the caption holding a word dragged: in, just over
        // the middle of the word at that edge, out, back past where that
        // word was at "start", or a little in, short of any word's middle.
        const [edge, text] = arg;
        const now = await engineState(page, watch.at);
        const was = what === "edge out" ? marks.start : now;
        const i = was.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
        if (i < 0) {
          wrong = `no caption has "${text}"`;
          break;
        }
        const words = was.captions[i].lines.flatMap((l) => l.words);
        const w = edge === "end" ? words.at(-1) : words[0];
        const mid = (w.start + w.end) / 2;
        const into = (t) => {
          let sum = 0;
          for (const p of now.segments) {
            if (t <= sum + p.end - p.start) return p.start + t - sum;
            sum += p.end - p.start;
          }
          return now.segments.at(-1).end;
        };
        const c = now.captions[i];
        const from = edge === "end" ? c.end : c.start;
        let to;
        if (what === "edge in") to = edge === "end" ? (w.start + mid) / 2 : (mid + w.end) / 2;
        else if (what === "edge out") to = edge === "end" ? was.captions[i].end : was.captions[i].start;
        else to = edge === "end" ? Math.max(mid + 0.05, from - 0.2) : Math.min(mid - 0.05, from + 0.2);
        const label = `When caption ${i + 1} ${edge === "end" ? "goes" : "appears"}`;
        const box = await page.locator(`.clip-timeline [aria-label="${label}"]`).first().boundingBox();
        if (!box) {
          wrong = `there is no edge "${label}"`;
          break;
        }
        // From the middle of the edge's handle to the moment itself, since
        // the timeline reads where the pointer is, not how far it went.
        // Held there a moment before it is let go, so what the timeline
        // draws under the hand can be held against what it saves: the
        // edge is where the drag showed it, and does not jump when let go.
        const x = box.x + box.width / 2;
        const y = box.y + box.height / 2;
        const target = await xOf(page, into(to));
        await page.mouse.move(x, y);
        await page.mouse.down();
        for (let k = 1; k <= 6; k++) {
          await page.mouse.move(x + ((target - x) * k) / 6, y);
          await page.waitForTimeout(30);
        }
        await page.waitForTimeout(300);
        const handle = page.locator(`.clip-timeline [aria-label="${label}"]`).first();
        const held = await handle.boundingBox();
        await page.mouse.up();
        await watch.step(what, s);
        let left = null;
        for (let k = 0; k < 20; k++) {
          left = await handle.boundingBox();
          if (held && left && Math.abs(left.x - held.x) <= 1) break;
          await page.waitForTimeout(100);
        }
        if (!held || !left || Math.abs(left.x - held.x) > 1) {
          wrong = `the edge was drawn at ${held?.x.toFixed(1)} px while dragged and at ${left?.x.toFixed(1)} px once let go`;
        }
        break;
      }
      case "count pops":
      case "pops": {
        // How often a caption block on the clip timeline pops, counted from
        // "count pops" on. The playhead stands still, so any pop is one an
        // edit made.
        if (what === "count pops") {
          await page.waitForTimeout(400);
          await page.evaluate(() => {
            window.__pops = 0;
            if (!window.__popsHeard) {
              window.__popsHeard = true;
              document.addEventListener("animationstart", (e) => {
                if (e.animationName === "pop" && e.target.closest?.(".clip-timeline .caption")) window.__pops++;
              });
            }
          });
        } else {
          await page.waitForTimeout(400);
          const n = await page.evaluate(() => window.__pops);
          if (n !== arg) wrong = `a caption block popped ${n} times, ${arg} wanted`;
        }
        break;
      }
      case "edge put back": {
        // A double-click on an edge of the caption holding a word.
        const [edge, text] = arg;
        const now = await engineState(page, watch.at);
        const i = now.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
        const label = `When caption ${i + 1} ${edge === "end" ? "goes" : "appears"}`;
        const box = await page.locator(`.clip-timeline [aria-label="${label}"]`).first().boundingBox();
        if (i < 0 || !box) {
          wrong = `there is no edge "${label}"`;
          break;
        }
        await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
        await watch.step(what, s);
        for (let k = 0; k < 20 && same(await engineState(page, watch.at), now); k++) await page.waitForTimeout(100);
        break;
      }
      case "drawn in": {
        // The captions read as they did at a mark, and the caption holding
        // a word draws in at its first or last edge from where it was at
        // "start", never out, its other edge where it was.
        const [label, text, which] = arg;
        const read = (caps) => JSON.stringify(caps.captions.map((c) => c.lines.flatMap((l) => l.words.map((w) => w.text))));
        const now = await engineState(page, watch.at);
        const i = marks.start.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
        const was = marks.start.captions[i];
        const c = now.captions[i];
        const near = (a, b) => Math.abs(a - b) < 1e-3;
        const inward = which === "last" ? near(c.start, was.start) && c.end < was.end - 1e-3 : near(c.end, was.end) && c.start > was.start + 1e-3;
        if (read(now) !== read(marks[label]) || !inward) {
          wrong = `the caption with "${text}" did not draw in at its ${which} edge as it did at "${label}"\n${describe(marks[label], now)}`;
        }
        break;
      }
      case "gone last":
      case "gone first": {
        // The caption holding a word at a mark reads as it did with its
        // last or first word left out, and every other caption as it did.
        const [label, text] = arg;
        const read = (c) => c.lines.flatMap((l) => l.words.map((w) => w.text));
        const was = marks[label];
        let now = null;
        for (let k = 0; k < 20; k++) {
          now = await engineState(page, watch.at);
          if (now.captions.length === was.captions.length) {
            const i = was.captions.findIndex((c) => read(c).some((w) => w.includes(text)));
            const want = what === "gone last" ? read(was.captions[i]).slice(0, -1) : read(was.captions[i]).slice(1);
            if (read(now.captions[i]).join(" ") === want.join(" ")) break;
          }
          await page.waitForTimeout(100);
          now = null;
        }
        if (!now) wrong = `the caption with "${text}" did not lose its ${what.slice(5)} word\n${describe(was, await engineState(page, watch.at))}`;
        break;
      }
      case "remove edge word": {
        // The first or last word of the caption holding a word framed and
        // removed in the caption box.
        const [which, text] = arg;
        const now = await engineState(page, watch.at);
        const c = now.captions.find((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
        const all = c.lines.flatMap((l) => l.words);
        const edgeWord = (which === "last" ? all.at(-1) : all[0]).text;
        for (let i = 0; i < 80 && !(await word(edgeWord).count()); i++) {
          await page.keyboard.press("Shift+ArrowRight");
          await page.waitForTimeout(100);
        }
        await word(edgeWord).click();
        await page.keyboard.press("Escape");
        await watch.step("frame", s);
        await page.keyboard.press("Backspace");
        await watch.step("press", await shown(page));
        break;
      }
      case "others": {
        // Every caption but the one holding a word at a mark appears and
        // goes when it did then.
        const [label, text] = arg;
        const was = marks[label];
        const now = await engineState(page, watch.at);
        const mine = was.captions.find((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
        const span = (c) => `${c.start.toFixed(3)}-${c.end.toFixed(3)}`;
        const moved = was.captions
          .filter((c) => c !== mine)
          .filter((c) => {
            const n = now.captions.find((d) => Math.abs(d.first - c.first) < 1e-6);
            return !n || span(n) !== span(c);
          });
        if (moved.length) {
          wrong = `captions beside the one with "${text}" changed: ${moved.map(span).join(" ")}\nthen ${was.captions.map(span).join(" ")}\nnow  ${now.captions.map(span).join(" ")}`;
        }
        break;
      }
      case "gone": {
        const now = await engineState(page, watch.at);
        if (now.captions.some((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(arg))))) {
          wrong = `a caption still has "${arg}"\n${describe(marks.start ?? now, now)}`;
        }
        break;
      }
      case "fewer": {
        const [label, n] = arg;
        const count = (caps) => caps.captions.flatMap((c) => c.lines.flatMap((l) => l.words)).length;
        const now = await engineState(page, watch.at);
        const was = marks[label];
        if (count(was) - count(now) !== n || now.captions.length !== was.captions.length) {
          wrong = `${count(was) - count(now)} words and ${was.captions.length - now.captions.length} captions fewer than at "${label}", not ${n} and none\n${describe(was, now)}`;
        }
        break;
      }
      case "blocks": {
        const now = await engineState(page, watch.at);
        const want = [...new Set(now.captions.map((c) => String(c.first)))].join(" ");
        let drawn = "";
        for (let i = 0; i < 20; i++) {
          drawn = await page.evaluate(() =>
            [...new Set([...document.querySelectorAll(".clip-timeline .caption")].map((b) => b.dataset.first))].join(" "),
          );
          if (drawn === want) break;
          await page.waitForTimeout(100);
        }
        if (drawn !== want) wrong = `the clip timeline draws captions at ${drawn}, the engine has them at ${want}`;
        break;
      }
      case "click":
        await word(arg).click();
        await watch.step("click", s);
        break;
      case "button": {
        const button = page.getByRole("button", { name: arg, exact: true });
        await button.click();
        await page.waitForFunction(
          (label) => ![...document.querySelectorAll("button")].some((b) => b.textContent.trim() === label && b.disabled),
          arg,
        );
        break;
      }
      case "press":
        await page.keyboard.press(arg);
        await watch.step(arg === "Enter" && !s.some((w) => w.focused) ? "open" : "press", s);
        break;
      case "type":
        await page.keyboard.type(arg);
        await watch.step("type", s);
        break;
      case "click heard": {
        // Inside the first word still being heard that has three letters
        // or more, just after its second letter, so the X typed there has
        // letters of the word on either side. A click a fixed 12 px in
        // landed after a short word like "er" on CI, not in it.
        const at = await page.evaluate(() => {
          const mark = document.querySelector(".telling .hearing");
          const text = mark?.firstChild;
          if (!text) return null;
          const word = /\p{L}{3,}/u.exec(text.textContent);
          if (!word) return null;
          const third = document.createRange();
          third.setStart(text, word.index + 2);
          third.setEnd(text, word.index + 3);
          const r = third.getClientRects()[0];
          return r && { x: r.left + r.width * 0.25, y: r.top + r.height / 2 };
        });
        if (!at) {
          wrong = "nothing was being heard to click into";
          break;
        }
        await page.mouse.click(at.x, at.y);
        const where = await page.evaluate(() => {
          const sel = getSelection();
          const mark = document.querySelector(".telling .hearing");
          return {
            heard: mark?.textContent.trim() ?? "",
            inMark: !!mark?.contains(sel.anchorNode),
            inSaid: !!document.querySelector(".telling .dictated")?.contains(sel.anchorNode),
          };
        });
        if (where.heard) wrong = `the words clicked into are still being heard: "${where.heard}"`;
        else if (where.inMark || !where.inSaid) wrong = "the cursor is not in the words clicked into";
        break;
      }
      case "field": {
        const [selector, text] = arg;
        await page.locator(selector).click();
        await page.keyboard.type(text);
        break;
      }
      case "field reads": {
        const [selector, text] = arg;
        const have = await page.locator(selector).inputValue();
        if (have !== text) wrong = `${selector} reads ${JSON.stringify(have)}, not ${JSON.stringify(text)}`;
        break;
      }
      case "undo refused": {
        const from = await page.evaluate(() => window.__calls.length);
        await page.evaluate(() => window.__menu("undo"));
        let said = null;
        for (let tries = 0; tries < 40 && said === null; tries++) {
          await page.waitForTimeout(100);
          // The refusal is the answer here, so it is taken off the calls
          // that failed before the rules read them.
          said = await page.evaluate((from) => {
            const c = window.__calls.slice(from).find((c) => c.name === "Undo" && c.failed);
            if (!c) return null;
            const failed = c.failed;
            c.failed = null;
            return String(failed);
          }, from);
        }
        if (!said?.includes("changed since")) wrong = `Undo was not refused as changed since: ${said}`;
        await watch.step("undo", s);
        break;
      }
      case "undo":
      case "redo":
        await page.evaluate((m) => window.__menu(m), what);
        await watch.step(what, s);
        break;
      case "cut at": {
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        await page.mouse.dblclick(from + arg * (g.end.x - from), high(g.track));
        await watch.step("cut", s);
        break;
      }
      case "join": {
        const g = await handles(page);
        await page.mouse.dblclick(middle(g.cuts[arg - 1]), high(g.track));
        await watch.step("join", s);
        break;
      }
      case "trim": {
        const g = await handles(page);
        const [edge, px] = arg;
        await drag(page, middle(g[edge]), high(g.track), px, false);
        await watch.step("trim", s);
        break;
      }
      case "drag while playing": {
        const [edge, px] = arg;
        await preview(page, 0.2);
        if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
          await page.keyboard.press("Space");
        }
        await page.waitForTimeout(700);
        await page.evaluate(() => {
          window.__played = [];
          window.__playWatch = true;
          const tick = () => {
            if (!window.__playWatch) return;
            const at = Number(document.querySelector(".screen")?.dataset.playhead);
            window.__played.push([performance.now(), at, window.__pictured ? window.__pictured() : -1]);
            requestAnimationFrame(tick);
          };
          requestAnimationFrame(tick);
        });
        await page.waitForTimeout(300);
        const g = await handles(page);
        const box = edge === "cut" ? g.cutEdges[0] : g[edge];
        if (!box) {
          wrong = `there is no ${edge} edge to drag`;
          break;
        }
        await page.mouse.move(middle(box), box.y + box.h / 2);
        await page.mouse.down();
        for (let i = 1; i <= 20; i++) {
          await page.mouse.move(middle(box) + (px * i) / 20, box.y + box.h / 2);
          await page.waitForTimeout(40);
        }
        await page.mouse.up();
        await page.waitForTimeout(500);
        await page.evaluate(() => (window.__playWatch = false));
        await settle(page);
        break;
      }
      case "loop off near the end": {
        const end = (await engineState(page, watch.at)).segments.at(-1).end;
        await preview(page, 0.2);
        // Every sample the sound card is handed, with the sound card's own
        // frame it plays at, the way the frame queue's probe records it.
        await page.evaluate(async () => {
          const q = window.__frameQueue;
          const code = `registerProcessor("recorder", class extends AudioWorkletProcessor {
            process(inputs) {
              const ch = inputs[0][0];
              // Chromium now and then hands two render quanta the same
              // currentFrame and the one after that two on, see the frame
              // queue's probe, preview/frames/main.ts.
              const frame = Math.max(currentFrame, (this.last ?? -Infinity) + 128);
              this.last = frame;
              // Sent 32 quanta at a time: a message for each, 375 a second,
              // stopped coming after about 8000 of them.
              if (!ch) return true;
              if (!this.batch) this.batch = { frame, data: new Float32Array(4096), n: 0 };
              if (this.batch.frame + this.batch.n !== frame && this.batch.n) {
                this.port.postMessage({ frame: this.batch.frame, data: this.batch.data.slice(0, this.batch.n) });
                this.batch = { frame, data: new Float32Array(4096), n: 0 };
              }
              this.batch.data.set(ch, this.batch.n);
              this.batch.n += ch.length;
              if (this.batch.n + 128 > 4096) {
                this.port.postMessage({ frame: this.batch.frame, data: this.batch.data.slice(0, this.batch.n) });
                this.batch = null;
              }
              return true;
            }
          });`;
          await q.audio.audioWorklet.addModule(URL.createObjectURL(new Blob([code], { type: "text/javascript" })));
          const node = new AudioWorkletNode(q.audio, "recorder", { numberOfOutputs: 1 });
          window.__heard = [];
          node.port.onmessage = (e) => window.__heard.push(e.data);
          const hush = q.audio.createGain();
          hush.gain.value = 0;
          q.out.connect(node).connect(hush).connect(q.audio.destination);
          // Held, or the recorder is collected while it records.
          window.__recorder = [node, hush];
        });
        if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
          await page.keyboard.press("Space");
        }
        await page.waitForTimeout(300);
        const starts = await page.evaluate(() => window.__frameQueue.stats.starts);
        await page.keyboard.press("l");
        await page
          .waitForFunction((to) => Number(document.querySelector(".screen").dataset.playhead) > to, end - arg, { timeout: 60000, polling: "raf" })
          .catch(() => {});
        await page.evaluate(() => {
          window.__played = [];
          window.__playWatch = true;
          const tick = () => {
            if (!window.__playWatch) return;
            const at = Number(document.querySelector(".screen")?.dataset.playhead);
            const playing = !!document.querySelector('button[aria-label="Pause"]');
            window.__played.push([performance.now(), at, window.__pictured(), playing]);
            requestAnimationFrame(tick);
          };
          requestAnimationFrame(tick);
        });
        // L pressed quickly five times, the way Tim did, the loop off after
        // the last.
        for (let i = 0; i < 5; i++) {
          await page.keyboard.press("l");
          await page.waitForTimeout(120);
        }
        await page
          .waitForFunction(() => !!document.querySelector('button[aria-label="Play"]'), null, { timeout: (arg + 8) * 1000, polling: 100 })
          .catch(() => {});
        await page.waitForTimeout(1000);
        const q = await page.evaluate(() => {
          window.__playWatch = false;
          const q = window.__frameQueue;
          return {
            starts: q.stats.starts,
            c0: q.c0,
            m0: q.m0,
            rate: q.audio.sampleRate,
            programEnd: q.program.end,
            heard: window.__heard.map((h) => ({ frame: h.frame, data: Array.from(h.data) })),
          };
        });
        loopEnd = { end, fps: await rate(page), starts: q.starts - starts, ...q };
        await settle(page);
        break;
      }
      case "press while playing": {
        await preview(page, 0.2);
        if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
          await page.keyboard.press("Space");
        }
        await page.waitForTimeout(700);
        await page.evaluate(() => {
          window.__played = [];
          window.__hand = {};
          window.__playWatch = true;
          const tick = () => {
            if (!window.__playWatch) return;
            const at = Number(document.querySelector(".screen")?.dataset.playhead);
            const playing = !!document.querySelector('button[aria-label="Pause"]');
            window.__played.push([performance.now(), at, window.__pictured ? window.__pictured() : -1, playing]);
            requestAnimationFrame(tick);
          };
          requestAnimationFrame(tick);
        });
        await page.waitForTimeout(300);
        const g = await handles(page);
        const x = (g.start.x + g.start.w + g.end.x) / 2;
        const y = high(g.track);
        const hand = (what) => page.evaluate((w) => (window.__hand[w] = performance.now()), what);
        await page.mouse.move(x, y);
        await hand("down");
        await page.mouse.down();
        await page.waitForTimeout(700);
        await hand("moves");
        for (let i = 1; i <= 10; i++) {
          await page.mouse.move(x + (arg * i) / 10, y);
          await page.waitForTimeout(30);
        }
        await hand("still");
        await page.waitForTimeout(700);
        await hand("up");
        await page.mouse.up();
        await page.waitForTimeout(700);
        await page.evaluate(() => (window.__playWatch = false));
        await settle(page);
        break;
      }
      case "play from": {
        const [where, seconds, key] = arg;
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        const x = where === "before" ? (g.track.x + g.start.x) / 2 : from + 0.2 * (g.end.x - from);
        if (where === "before" && g.start.x - g.track.x < 40) {
          wrong = "there is no room on the clip timeline before the clip";
          break;
        }
        await page.mouse.click(x, high(g.track));
        await settle(page);
        const state = await engineState(page, watch.at);
        const stop = await recordFrames(page, 1 / (await rate(page)));
        await page.evaluate(() => document.body.focus());
        const began = (await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead))) || 0;
        await page.keyboard.press("Space");
        // With a key, it is pressed every 400 ms of the play, the way a
        // hand skips ahead while it watches.
        if (key) {
          for (let t = 0; t + 0.4 <= seconds; t += 0.4) {
            await page.waitForTimeout(400);
            await page.keyboard.press(key);
          }
        } else await page.waitForTimeout(seconds * 1000);
        await page.evaluate(() => {
          if (document.querySelector('button[aria-label="Play"]')) return;
          const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
          document.body.dispatchEvent(new KeyboardEvent("keydown", key));
          document.body.dispatchEvent(new KeyboardEvent("keyup", key));
        });
        played = { overlays: (await stop()).overlays, state, fps: await rate(page) };
        // The steps have to have skipped: on the video the playhead went
        // further than the play alone takes it.
        const ended = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
        if (key === "Shift+ArrowRight" && where === "before" && ended - began < seconds + 1) {
          wrong = `${key} while playing moved the playhead ${(ended - began).toFixed(2)} s in ${seconds} s of play, no skip ahead`;
        }
        await watch.step("play", s);
        break;
      }
      case "make while paused":
      case "make while playing": {
        const playing = what === "make while playing";
        const g = await handles(page);
        if (g.start.x - g.track.x < 40) {
          wrong = "there is no room on the clip timeline before the clip";
          break;
        }
        await page.mouse.click((g.track.x + g.start.x) / 2, high(g.track));
        await settle(page);
        const path = await episodeOn(page);
        const was = (await overlay(page)).card;
        const stop = await recordFrames(page, 1 / (await rate(page)));
        await page.evaluate(() => document.body.focus());
        if (playing) {
          await page.keyboard.press("Space");
          await page.waitForTimeout(1500);
        }
        await page.keyboard.press(arg);
        // Until the card chosen is a clip the engine has written, the one
        // made, which is not the one chosen before.
        let written = "";
        for (let t = 0; t < 30000 && !written; t += 250) {
          await page.waitForTimeout(250);
          const card = (await overlay(page)).card;
          if (card && card !== was && (await ask(page, "Clips", path)).some((c) => c.key === card)) written = card;
        }
        await page.waitForTimeout(2000);
        await page.evaluate(() => {
          if (document.querySelector('button[aria-label="Play"]')) return;
          const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
          document.body.dispatchEvent(new KeyboardEvent("keydown", key));
          document.body.dispatchEvent(new KeyboardEvent("keyup", key));
        });
        await page.waitForTimeout(300);
        made = { playing, was, written, overlays: (await stop()).overlays };
        await watch.step("play", s);
        break;
      }
      case "reset": {
        const g = await handles(page);
        await page.mouse.dblclick(middle(g[arg]), high(g.track));
        await watch.step("reset", s);
        break;
      }
      case "cut switch": {
        const at = switches((await engineState(page, watch.at)).segments)[arg - 1];
        const g = await handles(page);
        await page.mouse.dblclick(await xOf(page, at), high(g.track));
        await watch.step("cut", s);
        break;
      }
      case "trim past": {
        const [edge, n] = arg;
        const at = switches((await engineState(page, watch.at)).segments)[n - 1];
        const g = await handles(page);
        const to = await xOf(page, edge === "end" ? at - 2 : at + 2);
        await drag(page, middle(g[edge]), high(g.track), to - middle(g[edge]), false);
        await watch.step("trim", s);
        break;
      }
      case "look": {
        const fps = await rate(page);
        looks[arg] = [];
        for (const p of (await engineState(page, watch.at)).segments) {
          const at = (p.start + p.end) / 2;
          if (!(await seekTo(page, at, fps))) {
            wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
            break;
          }
          looks[arg].push({ ...p, at, frame: await cropFrame(page) });
        }
        break;
      }
      case "pieces": {
        const n = await page.locator(".clip-timeline .piece").count();
        if (n !== arg) wrong = `the clip timeline draws ${pieceCount(n)}, not ${arg}`;
        break;
      }
      case "framed": {
        // What a person sees first: the crop frame on every shot, where
        // it stood and with the same picture inside it. Then the pieces.
        const then = looks[arg];
        const fps = await rate(page);
        for (const was of then) {
          if (!(await seekTo(page, was.at, fps))) {
            wrong = `the video preview never showed the frame at ${was.at.toFixed(2)}`;
            break;
          }
          const now = await cropFrame(page);
          if (!now) {
            wrong = `at ${was.at.toFixed(2)} the video preview has no crop frame`;
          } else if (Math.abs(now.left - was.frame.left) > 0.005) {
            wrong = `at ${was.at.toFixed(2)} the crop frame stands ${(100 * now.left).toFixed(1)}% in, not ${(100 * was.frame.left).toFixed(1)}% as at "${arg}", and shows ${sayLook(now.look)}, not ${sayLook(was.frame.look)}`;
          } else if (!sameLook(now.look, was.frame.look)) {
            wrong = `at ${was.at.toFixed(2)} the crop frame shows ${sayLook(now.look)}, not ${sayLook(was.frame.look)} as at "${arg}"`;
          }
          if (wrong) break;
        }
        const n = await page.locator(".clip-timeline .piece").count();
        if (!wrong && n !== then.length) {
          wrong = `the clip timeline draws ${pieceCount(n)}, not ${then.length} as at "${arg}"`;
        }
        break;
      }
      case "shots": {
        const pieces = (await engineState(page, watch.at)).segments;
        const short = readShort(await shortOf(page, watch.at));
        // Each frame by the moment of the episode in its middle, through
        // the pieces, and what the crop frame showed there at the look.
        let into = 0;
        const placed = pieces.map((p) => {
          const q = { ...p, from: into };
          into += p.end - p.start;
          return q;
        });
        let compared = 0;
        for (let i = 0; i < short.frames.length && !wrong; i++) {
          const t = (i + 0.5) / short.fps;
          const p = placed.find((q) => t < q.from + q.end - q.start) ?? placed[placed.length - 1];
          const at = p.start + t - p.from;
          const was = looks[arg].find((l) => at >= l.start && at < l.end);
          if (was) compared++;
          if (was && !sameLook(short.frames[i], was.frame.look)) {
            wrong = `frame ${i} of the short, ${at.toFixed(2)} in the episode, shows ${sayLook(short.frames[i])}, not ${sayLook(was.frame.look)} as the crop frame did at "${arg}"`;
          }
        }
        // A short with no frame to compare proves nothing.
        if (!wrong && !compared) wrong = `no frame of the short was compared, of ${short.frames.length}`;
        if (process.env.VERBOSE) {
          console.log(`      ${compared} frames of ${short.frames.length} compared, pieces ${placed.map((p) => `${p.start.toFixed(2)}-${p.end.toFixed(2)}`).join(" ")}`);
        }
        // Heard: where one piece runs straight into the next, no 10 ms
        // within a tenth of a second either side is quieter than 80% of
        // the tone half a second before.
        for (const p of placed.slice(1)) {
          const before = placed[placed.indexOf(p) - 1];
          if (wrong || p.start - before.end > 0.0005) continue;
          const steady = loudness(short, p.from - 0.5);
          let least = { l: Infinity, t: 0 };
          for (let t = p.from - 0.1; t < p.from + 0.1; t += 0.0025) {
            const l = loudness(short, t);
            if (l < least.l) least = { l, t };
          }
          if (least.l < 0.8 * steady) {
            wrong = `at ${least.t.toFixed(4)} s into the short, by ${p.start.toFixed(2)} in the episode where two shots meet, the sound is down to ${((100 * least.l) / steady).toFixed(0)}% of the tone`;
          }
        }
        break;
      }
      case "cuts": {
        const t = await timeline(page);
        if (t?.cuts.length !== arg) wrong = `the clip timeline shows ${t?.cuts.length ?? "no"} cuts, not ${arg}`;
        break;
      }
      case "model": {
        const how = { holds: "hang=1&fail=0", fails: "hang=0&fail=1", answers: "hang=0&fail=0" }[arg];
        await control(url, `/model?${how}`);
        break;
      }
      case "add": {
        const [seconds, rate, timing, inside] = [arg].flat();
        await control(url, `/pick?seconds=${seconds}${rate ? `&rate=${rate}` : ""}${timing ? `&timing=${timing}` : ""}${inside ? `&in=${inside}` : ""}`);
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await watch.step("add", s);
        break;
      }
      case "drop":
      case "drop again":
      case "drop not a video": {
        const where = what === "drop" ? arg[1] : arg;
        let file;
        if (what === "drop") file = droppedVideo = await control(url, `/pick?seconds=${arg[0]}&box=0`);
        else if (what === "drop again") file = droppedVideo;
        else {
          file = droppedOther = join(tmpdir(), `notizen-${process.pid}.pdf`);
          writeFileSync(file, "%PDF-1.4\n");
        }
        if (where !== "workspace") {
          const name = where === "settings" ? "Settings" : "Updates";
          await fromSidebar(page, () => page.locator("aside").getByText(name, { exact: true }).first().click());
          await settle(page);
        }
        wrong = await dropOn(page, [file]);
        if (what === "drop not a video") rmSync(file, { force: true });
        await watch.step("add", s);
        break;
      }
      case "no reason": {
        await page.waitForTimeout(500);
        const said = await page.evaluate(() => document.querySelector(".error")?.textContent ?? "");
        if (said.trim()) wrong = `the app still says "${said.trim()}" after the next files were taken`;
        break;
      }
      case "dropped in front": {
        const name = basename(droppedVideo);
        let on = null;
        for (let i = 0; i < 100 && on !== droppedVideo; i++) {
          await page.waitForTimeout(100);
          on = await episodeOn(page);
        }
        const library = await ask(page, "Library");
        const times = library.filter((e) => e.source === droppedVideo).length;
        if (on !== droppedVideo) wrong = `${on ? basename(on) : "no video"} is on screen, not ${name}, which was dropped`;
        else if (times !== 1) wrong = `${name} is in the library ${times} times`;
        else if (!(await page.locator(".workspace.front").count())) wrong = `${name} is in the sidebar, and its workspace is not open`;
        await settle(page);
        break;
      }
      case "not a video": {
        const name = basename(droppedOther);
        let said = "";
        for (let i = 0; i < 50 && !said.includes(name); i++) {
          await page.waitForTimeout(100);
          said = await page.evaluate(() => document.querySelector(".error")?.textContent ?? "");
        }
        const library = await ask(page, "Library");
        if (!said.includes(`${name} is not a video`)) wrong = `the app says "${said.trim()}", not that ${name} is not a video`;
        else if (library.some((e) => e.name === name)) wrong = `${name} is in the library`;
        break;
      }
      case "add sound alone": {
        await control(url, "/pick-sound");
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await page.waitForTimeout(1500);
        await settle(page);
        break;
      }
      case "add cameras": {
        const [seconds, at] = arg;
        await control(url, `/pick?seconds=${seconds}&switch=${at}`);
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await watch.step("add", s);
        break;
      }
      case "render": {
        const on = await chosen(page);
        await page.waitForFunction(
          (r) => {
            const b = document.querySelector(r);
            return b && !b.disabled && b.textContent.trim().startsWith("Render");
          },
          "button.primary.render",
          { timeout: 60000, polling: 200 },
        );
        const renders = async () => (await ask(page, "Jobs")).filter((j) => j.kind === "render" && j.episode === on.path);
        const before = (await renders()).length;
        await page.locator("button.primary.render").click();
        let job = null;
        for (let i = 0; i < 600 && !job; i++) {
          await page.waitForTimeout(200);
          job = (await renders()).slice(before).find((j) => j.state === "done" || j.state === "failed") ?? null;
        }
        if (job?.state !== "done") wrong = `Render ${job ? `failed: ${job.error}` : "wrote no short in two minutes"}`;
        await watch.step("render", s);
        break;
      }
      case "captions like the short": {
        const fps = await rate(page);
        const state = await engineState(page, watch.at);
        const short = await shortOf(page, watch.at);
        if (!short || !existsSync(short)) {
          wrong = "the clip has no short to compare with";
          break;
        }
        const pieces = state.segments;
        const source = await ask(page, "Source", await episodeOn(page));
        // The video preview at the short's own size, so the two are drawn
        // at one size and compared pixel for pixel: on a page of its own,
        // made taller until the crop is as tall as the short.
        const tall = Number(execFileSync("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries",
          "stream=height", "-of", "csv=p=0", short]).toString().trim());
        const sized = (await open(url, { viewport: { width: 2400, height: 1000 }, again: page.context().browser() })).page;
        const canvasTall = () => sized.evaluate(() => document.querySelector(".screen canvas").height);
        for (let i = 0, high = 1000; i < 12 && (await canvasTall()) !== tall; i++) {
          high += Math.round((tall - (await canvasTall())) * 1.4) || Math.sign(tall - (await canvasTall()));
          await sized.setViewportSize({ width: 2400, height: high });
          await settle(sized);
        }
        if ((await canvasTall()) !== tall) {
          wrong = `the video preview could not be made ${tall} pixels tall, it is ${await canvasTall()}`;
          await sized.close();
          break;
        }
        // A moment of the clip's clock in the episode, and the frame of the
        // short that shows the frame of the episode that holds it: the
        // frames of the pieces before, and the frames into its own, each
        // piece the frames from its start up to the one before its end,
        // see pieceFrames in engine/render.go.
        const into = (t) => {
          let sum = 0;
          for (const p of pieces) {
            if (t < sum + p.end - p.start) return p.start + t - sum;
            sum += p.end - p.start;
          }
          return null;
        };
        // The frame that holds a moment, a frame that begins a millisecond
        // or less after it included, as seekTo waits for it.
        const shortIndex = (at) => {
          const k = Math.floor((at + 0.001) * fps);
          let n = 0;
          for (const p of pieces) {
            const [first, end] = [Math.round(p.start * fps), Math.round(p.end * fps)];
            if (k < end) return n + k - first;
            n += end - first;
          }
          return -1;
        };
        // The first frame of every word, where the highlight bounces, and
        // a frame in its middle, where it has settled.
        const words = (state.captions ?? []).flatMap((c) => c.lines.flatMap((l) => l.words));
        const moments = words
          .flatMap((w) => [w.start + 0.5 / fps, (w.start + w.end) / 2])
          .map(into)
          .filter((at) => at !== null);
        let compared = 0;
        let apart = 0;
        const report = [];
        for (const at of moments.slice(0, 12)) {
          if (!(await seekTo(sized, at, fps))) {
            wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
            break;
          }
          await sized.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
          await sized.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
          // Nothing but the canvas draws them: the caption box over it
          // shows no word and no pill unless a word is being typed.
          const painted = await sized.evaluate(() => {
            const alpha = (c) => {
              const m = c.match(/rgba?\(([^)]*)\)/);
              const parts = m ? m[1].split(/[\s,/]+/).filter(Boolean) : [];
              return parts.length > 3 ? Number(parts[3]) : 1;
            };
            return [...document.querySelectorAll(".captions .line, .captions .line *")]
              .filter((e) => {
                const s = getComputedStyle(e);
                return (e.textContent.trim() && alpha(s.color) > 0) || alpha(s.backgroundColor) > 0;
              })
              .map((e) => e.textContent.trim());
          });
          if (painted.length) {
            wrong = `the caption box draws "${painted[0]}" over the video preview, a second drawing of the captions`;
            break;
          }
          const got = await cropPixels(sized, source);
          // A click lands on a whole pixel of the clip timeline, so the
          // playhead is where it landed, near the moment asked for.
          const landed = Number(await sized.evaluate(() => document.querySelector(".screen").dataset.playhead));
          const n = shortIndex(landed);
          if (!got || n < 0) continue;
          const want = shortFrame(short, n, got.w, got.h);
          const d = captionDiff(got.pixels, want, got.w, got.h);
          compared++;
          report.push(`${landed.toFixed(2)} s, frame ${n} of the short: ${d.mean.toFixed(2)} on average`);
          // The short is made with H.264 and keeps its colour at a quarter
          // of its pixels, so it is never the same to the level: 3 on
          // average where the preview draws what the short does, against
          // 4.5 and more for the frame before or after with another word
          // lit, and 41 for the canvas before 2.211, which drew none and
          // left the captions to the browser.
          if (d.mean > 3.6) {
            wrong = `at ${landed.toFixed(2)} s the captions in the video preview differ from frame ${n} of the short by ${d.mean.toFixed(2)} on average and ${d.most} at most, where the captions are`;
            break;
          }
          // The frames of the short either side, where their captions are
          // drawn otherwise, are told apart from it, so a frame off in the
          // highlight would not pass.
          for (const m of [n - 1, n + 1]) {
            if (m < 0) continue;
            const other = shortFrame(short, m, got.w, got.h);
            if (other.length !== want.length) continue;
            const o = captionDiff(want, other, got.w, got.h, 3);
            if (o.mean < 1) continue;
            const off = captionDiff(got.pixels, other, got.w, got.h);
            report.push(`  frame ${m}, drawn otherwise by ${o.mean.toFixed(2)}: ${off.mean.toFixed(2)} on average`);
            if (off.mean > d.mean + 0.5) apart++;
          }
        }
        await sized.close();
        if (process.env.VERBOSE) console.log(`        ${report.join("\n        ")}`);
        if (!wrong && apart < 4) wrong = `only ${apart} frames of the short either side were told apart from the video preview, so a frame off would pass`;
        if (!wrong && compared < 6) wrong = `only ${compared} frames of the short were compared`;
        break;
      }
      case "head":
        await pressHead(page);
        await watch.step("head", s);
        break;
      case "speech":
        await control(url, `/speech?ms=${arg}`);
        break;
      case "hold":
        await control(url, `/hold?call=${arg[0]}&ms=${arg[1]}`);
        break;
      case "new and cancel": {
        const seen = await page.evaluate(
          () =>
            new Promise((done) => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              const head = () => pane.querySelector(".listhead button.new");
              const text = (el) => (el?.textContent ?? "").replace(/\s+/g, " ").trim();
              const frames = [];
              const pressed = text(head());
              head().click();
              const tick = () => {
                const b = head();
                frames.push({ says: text(b), off: !!b?.disabled });
                if (text(b) === "Cancel" && !b.disabled) {
                  b.click();
                  requestAnimationFrame(() => done({ pressed, frames, after: text(head()) }));
                } else if (frames.length < 300) requestAnimationFrame(tick);
                else done({ pressed, frames, after: text(head()) });
              };
              requestAnimationFrame(tick);
            }),
        );
        const greyed = seen.frames.filter((f) => f.off).length;
        if (seen.pressed !== "New") wrong = `the head said ${seen.pressed}, not New`;
        else if (seen.frames.some((f) => f.says !== "Cancel"))
          wrong = `after New the head said ${[...new Set(seen.frames.map((f) => f.says))].join(", ")}, not Cancel at once`;
        else if (greyed)
          wrong = `the head said Cancel greyed out for ${greyed} of ${seen.frames.length} frames after New, so a press there did nothing`;
        else if (seen.after !== "Cancelling") wrong = `Cancel pressed, the head said ${seen.after}, not Cancelling`;
        console.log(`      Cancel pressed ${seen.frames.length} frames after New, ${greyed} of them greyed out`);
        await watch.step("head", s);
        break;
      }
      case "settings": {
        episodeBefore = (await chosen(page))?.path ?? episodeBefore;
        await fromSidebar(page, () => page.locator("aside").getByText("Settings", { exact: true }).first().click());
        await page.locator(".card.finding").waitFor();
        await settle(page);
        named = await modelsNamed(page);
        // Every card that shakes from here on, by its classes.
        await page.evaluate(() => {
          window.__shook = [];
          new MutationObserver((changes) => {
            for (const c of changes) if (c.target.classList?.contains("shaking")) window.__shook.push(c.target.className);
          }).observe(document.body, { attributes: true, attributeFilter: ["class"], subtree: true });
        });
        break;
      }
      case "menu":
        await page.evaluate((m) => window.__menu(m), arg);
        await page.locator("button.dictate").waitFor();
        await settle(page);
        break;
      case "write": {
        const [text, at] = arg;
        await page.locator(".telling").click();
        await page.keyboard.type(text);
        await page.evaluate((n) => {
          const field = document.querySelector(".telling");
          const at = document.createRange();
          at.setStart(field.firstChild, n);
          getSelection().removeAllRanges();
          getSelection().addRange(at);
        }, at);
        await watch.step("type", s);
        break;
      }
      case "dictate": {
        const button = page.locator("button.dictate");
        if (arg === "hearing") {
          // More is heard after what was there.
          const more = await page
            .waitForFunction(() => document.querySelector(".telling .hearing")?.textContent.trim(), null, { timeout: 10000 })
            .then(() => true)
            .catch(() => false);
          if (!more) wrong = "nothing more was heard";
          break;
        }
        if (arg === "settled") {
          // The field's text grows by what was said before the pause.
          const settled = await page
            .waitForFunction((was) => document.querySelector(".telling").innerText !== was, kept.dictating, { timeout: 10000 })
            .then(() => true)
            .catch(() => false);
          if (!settled) wrong = "nothing said was settled";
          break;
        }
        const running = (await button.textContent()).trim() === "Stop";
        if (!running) await button.click();
        // Stop the moment it is pressed, see the rules on a click.
        const said = (await button.textContent()).trim();
        if (!running && said !== "Stop") {
          wrong = `Dictate says "${said}" the frame after it was pressed, not Stop`;
          break;
        }
        // The words appear while they are heard, before anything stops.
        const heard = await page
          .waitForFunction(() => document.querySelector(".telling .hearing")?.textContent.trim(), null, { timeout: 10000 })
          .then(() => true)
          .catch(() => false);
        if (!heard) {
          wrong = "nothing appeared in the report while it was listening";
          break;
        }
        // The microphone moves with the voice it hears, so a person knows
        // they are heard. The bridge's microphone plays a tone.
        const moved = await page
          .waitForFunction(() => parseFloat(document.querySelector("button.dictate")?.style.getPropertyValue("--loud")) > 0.3, null, { timeout: 10000 })
          .then(() => true)
          .catch(() => false);
        if (!moved) {
          wrong = "the microphone did not move while it heard a voice";
          break;
        }
        await watch.step("hearing", s);
        if (arg === "start") {
          kept.dictating = await page.locator(".telling").evaluate((f) => f.innerText);
          break;
        }
        if (arg === "button") await button.click();
        else if (arg === "ends") {
          // Nobody presses anything: it stops by itself and says why.
          const why = await page
            .waitForFunction(() => /without a word/.test(document.querySelector(".words")?.textContent ?? ""), null, { timeout: 30000 })
            .then(() => true)
            .catch(() => false);
          if (!why) {
            wrong = "the recording never said it stopped by itself";
            break;
          }
        } else {
          await page.locator(".telling").focus();
          await page.keyboard.press(arg === "enter" ? "Enter" : "Escape");
        }
        await page.waitForFunction(() => document.querySelector("button.dictate")?.textContent.trim() === "Dictate", null, {
          timeout: 20000,
        });
        if (await page.locator(".telling .dictated").count()) wrong = "what was being said is still grey in the report";
        await watch.step("dictate", s);
        break;
      }
      case "report reads": {
        const have = await page.locator(".telling").evaluate((f) => f.innerText.replace(/\n$/, ""));
        const want = new RegExp(
          `^${arg
            .split("*")
            .map((p) => p.replace(/[.?+^$()[\]{}|\\]/g, "\\$&"))
            .join("\\S.*\\S")}$`,
        );
        if (!want.test(have)) wrong = `the report reads ${JSON.stringify(have)}, not ${JSON.stringify(arg)}`;
        kept.report = have;
        break;
      }
      case "report as before": {
        const have = await page.locator(".telling").evaluate((f) => f.innerText.replace(/\n$/, ""));
        if (have !== kept.report) wrong = `the report reads ${JSON.stringify(have)}, not ${JSON.stringify(kept.report)} as before`;
        break;
      }
      case "episode":
        await fromSidebar(page, () => page.locator("aside li", { hasText: episodeBefore.split("/").pop() }).first().click());
        await page.locator(".listhead button.new").first().waitFor();
        await settle(page);
        break;
      case "episode watched": {
        // Every frame the new episode's queue draws, read back from the
        // canvas on every animation frame until well after it settled. The
        // canvas is the one the episode before drew on, so only frames
        // drawn once the new episode's frames are open count.
        await preview(page, 0.04);
        await page.evaluate(() => {
          const was = window.__appFrames;
          window.__drawn = [];
          window.__watching = true;
          // Every call to the Go side, with where the playhead stood as it
          // was made.
          window.__asked = [];
          if (!window.__askWatched) {
            window.__askWatched = true;
            const real = window.fetch;
            window.fetch = (input, init) => {
              if (window.__watching && String(input).endsWith("/call") && init?.body) {
                const playhead = Number(document.querySelector(".screen")?.dataset.playhead);
                window.__asked.push([String(JSON.parse(init.body).name).split(".").pop(), playhead]);
              }
              return real(input, init);
            };
          }
          const watch = () => {
            if (!window.__watching) return;
            if (window.__appFrames && window.__appFrames !== was) {
              const n = window.__pictured();
              if (n >= 0 && n !== window.__drawn.at(-1)) window.__drawn.push(n);
            }
            requestAnimationFrame(watch);
          };
          requestAnimationFrame(watch);
        });
        await fromSidebar(page, () => page.locator("aside li", { hasText: episodeBefore.split("/").pop() }).first().click());
        await page.locator(".listhead button.new").first().waitFor();
        await settle(page);
        await page.waitForTimeout(1000);
        await page.evaluate(() => (window.__watching = false));
        break;
      }
      case "pick model": {
        await page.evaluate(() => (window.__shook = []));
        await page.locator(".card.finding button.pick").first().click();
        await page.waitForTimeout(400);
        const other = page.locator('[role="option"]:not([data-selected])');
        if (await other.count()) {
          await other.last().click();
          await page.waitForTimeout(400);
        }
        await settle(page);
        break;
      }
      case "remove model": {
        await page.evaluate(() => (window.__shook = []));
        if (arg === "language") {
          const fold = page.locator(".card.finding button.disclose-row");
          if ((await fold.getAttribute("aria-expanded")) !== "true") await fold.click();
          await page.locator('.card.finding button.bin[aria-label^="Remove "]').first().click();
        } else {
          await page.locator('.speech button.bin[aria-label^="Remove "]').first().click();
        }
        await page.waitForTimeout(400);
        break;
      }
      case "refused": {
        await page.waitForTimeout(200);
        const seen = await page.evaluate(() => ({
          shook: window.__shook ?? [],
          box: !!document.querySelector("dialog[open]"),
          list: !!document.querySelector('[role="listbox"]'),
          line: document.querySelector(".card.finding .words .line")?.textContent.trim() ?? "",
        }));
        const now = await modelsNamed(page);
        if (!seen.shook.some((c) => c.split(" ").includes(arg))) wrong = `the ${arg} card did not shake, shaken: ${JSON.stringify(seen.shook)}`;
        else if (seen.box) wrong = "a box opened";
        else if (seen.list) wrong = "the list opened";
        else if (JSON.stringify(now) !== JSON.stringify(named)) wrong = `the settings name ${JSON.stringify(now)}, not ${JSON.stringify(named)}`;
        else if (arg === "finding" && seen.line !== "A search is finding clips with it now.") wrong = `the line under Find clips with says "${seen.line}"`;
        break;
      }
      case "asks": {
        const box = page.locator("dialog[open]");
        if (!(await box.count())) {
          wrong = "no box asked whether to remove";
          break;
        }
        await page.keyboard.press("Escape");
        await page.waitForTimeout(300);
        break;
      }
      case "hearing": {
        const jobs = await ask(page, "Jobs");
        if (!jobs.some((j) => j.kind === "search" && j.state === "running" && j.step === "hearing")) {
          wrong = `no search hears now: ${JSON.stringify(jobs.map((j) => [j.kind, j.state, j.step]))}`;
        }
        break;
      }
      case "changed": {
        const now = await modelsNamed(page);
        if (JSON.stringify(now) === JSON.stringify(named)) wrong = `the settings still name ${JSON.stringify(now)}`;
        break;
      }
      case "screen light": {
        // A pixel of 10 bits a colour, red lowest, as the Go side sends
        // every frame.
        const word = (r, g, b) => {
          const w = (r | (g << 10) | (b << 20)) >>> 0;
          return [w & 255, (w >>> 8) & 255, (w >>> 16) & 255, w >>> 24];
        };
        // PQ's signals for 203 and 1000 nits, and HLG's 75 percent.
        const pqWhite = Math.round(0.5806 * 1023);
        const pq1000 = Math.round(0.7518 * 1023);
        const hlgWhite = Math.round(0.75 * 1023);
        const pixels = [
          { light: "", bytes: word(514, 257, 803), name: "a colour of standard video" },
          // A grey of 22 in 255 is shown as 25, QuickTime's look.
          { light: "", bytes: word(88, 88, 88), name: "a dark grey of standard video", shown: 25 / 255 },
          // The next value in 10 bits: in floats it is a step of its own,
          // where 8 bits made a step of 3 in the dark and banded.
          { light: "", bytes: word(89, 89, 89), name: "the next dark grey", above: 1 },
          { light: "pq", bytes: word(pqWhite, pqWhite, pqWhite), name: "PQ's reference white", white: true },
          { light: "pq", bytes: word(pq1000, pq1000, pq1000), name: "PQ at 1000 nits", bright: true },
          { light: "hlg", bytes: word(hlgWhite, hlgWhite, hlgWhite), name: "HLG's reference white", white: true },
          { light: "hlg", bytes: word(1023, 1023, 1023), name: "HLG's peak", bright: true },
          { light: "pq", bytes: word(0, pqWhite, 0), name: "a green of BT.2020", outside: true },
          // Captions laid over the frame before its look, the way the
          // render burns them in, see lib/frames/captions.ts: the purple
          // of a highlight over grey, and a box of black at half that lets
          // half of the grey through.
          { light: "", bytes: word(512, 512, 512), name: "a middle grey" },
          { light: "", bytes: word(512, 512, 512), over: [148, 33, 146, 255], name: "a highlight over standard video" },
          { light: "", bytes: word(512, 512, 512), over: [0, 0, 0, 128], name: "a box at half over standard video", darker: 8 },
          { light: "pq", bytes: word(pqWhite, pqWhite, pqWhite), over: [255, 255, 255, 255], name: "a white caption over PQ" },
        ];
        const got = await page.evaluate((p) => window.__checkPixels?.(p) ?? null, pixels.map(({ light, bytes, over }) => ({ light, bytes, over })));
        if (!got) {
          console.log("        no WebGPU in this browser, so what the GPU draws is not checked");
          break;
        }
        const problems = [];
        pixels.forEach((p, i) => {
          const gpu = got.gpu[i];
          const cpu = got.cpu[i];
          // Half floats hold 3 figures, and 10 bits sampled on the GPU may
          // round a step apart.
          const off = Math.max(...gpu.map((v, c) => Math.abs(v - cpu[c]) / Math.max(1, Math.abs(cpu[c]))));
          if (!(off < 0.01)) problems.push(`${p.name}: the GPU gives ${gpu.map((v) => v.toFixed(4))}, light.ts ${cpu.map((v) => v.toFixed(4))}`);
          if (p.white && gpu.some((v) => Math.abs(v - 1) > 0.01)) problems.push(`${p.name} is ${gpu.map((v) => v.toFixed(4))}, not the app's white`);
          if (p.bright && gpu.some((v) => v < 1.5)) problems.push(`${p.name} is ${gpu.map((v) => v.toFixed(4))}, not brighter than white`);
          if (p.outside && !(gpu[0] < 0)) problems.push(`${p.name} has a red of ${gpu[0].toFixed(4)}, inside BT.709`);
          if (p.shown !== undefined && gpu.some((v) => Math.abs(v - p.shown) > 0.5 / 255)) problems.push(`${p.name} is ${gpu.map((v) => (v * 255).toFixed(2))} in 255, not ${(p.shown * 255).toFixed(0)}`);
          if (p.darker !== undefined && !(gpu[0] < got.gpu[p.darker][0] * 0.9)) problems.push(`${p.name} is ${gpu.map((v) => v.toFixed(4))}, no darker than the grey under it`);
          if (p.above !== undefined) {
            const step = (gpu[0] - got.gpu[p.above][0]) * 255;
            if (!(step > 0 && step < 1)) problems.push(`${p.name} is ${step.toFixed(2)} above the one before in 255, not a step of its own below 1`);
          }
        });
        if (problems.length) wrong = problems.join("\n");
        break;
      }
      case "play to the end": {
        // Far to the right first, which the clip timeline takes as far as
        // it shows, and then onto the end of the episode where the clip
        // timeline shows it now.
        const { duration, fps } = await ask(page, "Source", await episodeOn(page));
        let g = await handles(page);
        await drag(page, middle(g.end), high(g.track), 4000, false);
        await watch.step("trim", s);
        g = await handles(page);
        await drag(page, middle(g.end), high(g.track), (await xOf(page, duration)) - middle(g.end), false);
        await watch.step("trim", s);
        const { segments } = await engineState(page, watch.at);
        const end = segments[segments.length - 1].end;
        if (Math.abs(end - duration) > 0.05) {
          wrong = `the clip ends at ${end.toFixed(2)}, not at the end of the episode, ${duration.toFixed(2)}`;
          break;
        }
        // Three seconds before the end, so the click lands on the clip
        // timeline clear of the end's handle.
        if (!(await seekTo(page, end - 3, fps))) {
          wrong = `the video preview never showed the frame at ${(end - 3).toFixed(2)}`;
          break;
        }
        await page.keyboard.press("Space");
        const stopped = await page
          .waitForFunction(
            (end) =>
              document.querySelector('button[aria-label="Play"]') !== null &&
              Number(document.querySelector(".screen").dataset.playhead) >= end - 0.1,
            end,
            { polling: 50, timeout: 10000 },
          )
          .then(() => true, () => false);
        if (!stopped) {
          wrong = `the play did not stop at the end of the episode, ${end.toFixed(2)}`;
          break;
        }
        // The playhead stays at the end, drawn on the clip timeline and on
        // the range picker, where the hand can take hold of it again. A
        // playhead a hair past the end was drawn on neither, plan row 2.172.
        const lost = await page.evaluate((duration) => {
          const at = Number(document.querySelector(".screen").dataset.playhead);
          if (at > duration) return `the playhead stopped at ${at.toFixed(3)}, past the end of the episode, ${duration.toFixed(3)}`;
          const on = (line, track) => {
            if (!line || !track) return false;
            const l = line.getBoundingClientRect();
            const t = track.getBoundingClientRect();
            const x = l.left + l.width / 2;
            return l.width > 0 && l.height > 0 && x >= t.left - 1 && x <= t.right + 1;
          };
          const timeline = document.querySelector(".clip-timeline");
          if (!on(timeline?.querySelector(".at .playhead"), timeline?.querySelector(".track")))
            return "the clip timeline shows no playhead at the end of the episode";
          const picker = [...document.querySelectorAll(".playhead")].find((e) => !e.closest(".clip-timeline"));
          const track = [...document.querySelectorAll(".track")].find((e) => !e.closest(".clip-timeline"));
          if (!on(picker, track)) return "the range picker shows no playhead at the end of the episode";
          return "";
        }, duration);
        if (lost) wrong = lost;
        break;
      }
      case "add hevc":
        await addHEVC(url, page);
        await watch.step("wait", s);
        break;
      case "leading frames": {
        const path = await episodeOn(page);
        const fps = await rate(page);
        // In the order the frames are decoded: a frame shown before the
        // key frame decoded last leads up to it.
        const packets = execFileSync("ffprobe", ["-v", "error", "-select_streams", "v:0", "-show_entries", "packet=pts_time,flags", "-of", "csv=p=0", path])
          .toString()
          .trim()
          .split("\n");
        let key = -Infinity;
        const leading = [];
        for (const line of packets) {
          const [t, flags] = line.split(",");
          if (flags.startsWith("K")) key = Number(t);
          else if (Number(t) < key) leading.push(Number(t));
        }
        const { segments } = await engineState(page, watch.at);
        const first = segments[0].start;
        const last = segments[segments.length - 1].end;
        const on = leading.filter((t) => t > first + 1 && t < last - 3).slice(0, 3);
        if (on.length < 3) {
          wrong = `the clip from ${first.toFixed(2)} to ${last.toFixed(2)} holds ${on.length} frames shown before their key frame, not 3`;
          break;
        }
        // In the middle of the frame, so the click lands on it.
        for (const t of on) {
          if (!(await seekTo(page, t + 0.5 / fps, fps))) {
            const at = await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
            wrong = `the video preview never showed the frame at ${t.toFixed(3)}, shown before its key frame, with the playhead at ${at.toFixed(3)}`;
            break;
          }
        }
        if (wrong) break;
        const from = await page.evaluate(() => ({ at: Number(document.querySelector(".screen").dataset.playhead), frame: window.__pictured() }));
        await page.evaluate(() => document.body.focus());
        await page.keyboard.press("Space");
        const plays = await page
          .waitForFunction(
            (from) =>
              Number(document.querySelector(".screen").dataset.playhead) > from.at + 0.5 && window.__pictured() > from.frame + 1,
            from,
            { polling: 50, timeout: 5000 },
          )
          .then(() => true, () => false);
        await page.keyboard.press("Space");
        if (!plays) wrong = `the space bar did not play from the frame at ${from.at.toFixed(3)}, shown before its key frame`;
        break;
      }
      case "no browser decoder": {
        await page.addInitScript(() => {
          for (const name of ["VideoDecoder", "AudioDecoder", "EncodedVideoChunk", "EncodedAudioChunk"]) {
            Object.defineProperty(window, name, {
              get() {
                throw new Error(`${name} is not to be used: the Go side decodes`);
              },
            });
          }
        });
        const name = (await episodeOn(page)).split("/").pop();
        await page.reload();
        await page.waitForTimeout(1200);
        await fromSidebar(page, () => page.locator("aside li", { hasText: name }).first().click());
        await page.locator("aside ol li[data-key] button.pick").first().click();
        await settle(page);
        break;
      }
      case "on frames": {
        // Where the picture starts, read from the file by ffprobe rather
        // than from the app, which is what is being checked: the picture's
        // first timestamp after the file's own start.
        const path = await episodeOn(page);
        const { fps } = await ask(page, "Source", path);
        const read = (args) => Number(execFileSync("ffprobe", ["-v", "error", ...args, "-of", "csv=p=0", path]).toString().trim());
        const videoStart = read(["-select_streams", "v:0", "-show_entries", "stream=start_time"]) - read(["-show_entries", "format=start_time"]);
        const { segments } = await engineState(page, watch.at);
        // Before the picture starts there is no frame to land on: a start
        // dragged there takes the picture from its first frame.
        const edges = segments
          .flatMap((p) => [p.start, p.end])
          .slice(0, -1)
          .filter((t) => t >= videoStart - 1e-6);
        // Edges are kept to the millisecond, a thirtieth of a frame here.
        const off = edges
          .map((t) => ({ t, by: Math.abs((t - videoStart) * fps - Math.round((t - videoStart) * fps)) }))
          .filter((e) => e.by > 0.001 * fps + 1e-6);
        if (off.length) {
          wrong = `the picture starts at ${videoStart.toFixed(4)} s, and ${off
            .map((e) => `${e.t.toFixed(3)} is ${e.by.toFixed(2)} of a frame from its frames`)
            .join(", ")}`;
        }
        break;
      }
      case "resize":
        wrong = await resizeProblem(page);
        break;
      case "from the app": {
        const fps = await rate(page);
        const { segments } = await engineState(page, watch.at);
        const first = segments[0].start;
        const last = segments[segments.length - 1].end;
        for (let i = 1; i <= arg && !wrong; i++) {
          const at = first + ((last - first) * i) / (arg + 1);
          if (!(await seekTo(page, at, fps))) wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
        }
        const streams = await page.evaluate(() => window.__appFrames?.stats.streams ?? 0);
        if (!wrong && !streams) wrong = "the picture did not come from the Go side";
        const trouble = await page.evaluate(() => document.querySelector(".screen")?.textContent.trim() ?? "");
        if (!wrong && /decod/i.test(trouble)) wrong = `the video preview says "${trouble}"`;
        break;
      }
      case "app size":
        await page.setViewportSize({ width: arg[0], height: arg[1] });
        await page.waitForTimeout(600);
        await settle(page);
        break;
      case "square buttons": {
        const got = await page.evaluate(() => {
          const h = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--control-h"));
          return { h, sizes: [...document.querySelectorAll(".detail .row .glyph")].map((b) => [b.title || b.textContent.trim(), b.getBoundingClientRect().width, b.getBoundingClientRect().height]) };
        });
        const off = got.sizes.filter(([, w, h]) => Math.abs(w - got.h) > 0.01 || Math.abs(h - got.h) > 0.01);
        if (got.sizes.length < 5) wrong = `the row under the clip timeline has ${got.sizes.length} buttons with an icon, not the row`;
        else if (off.length) wrong = `${off.length} of ${got.sizes.length} buttons are not ${got.h} by ${got.h}, the first, "${off[0][0].slice(0, 40)}", is ${off[0][1].toFixed(2)} by ${off[0][2].toFixed(2)}`;
        break;
      }
      case "sharp": {
        // The playhead stands still and nothing plays, so the frame on the
        // canvas is the one asked for while paused.
        const got = await page.evaluate(() => {
          const q = window.__frameQueue;
          const c = document.querySelector(".screen canvas");
          const f = q?.picture();
          const v = q?.video;
          const first = window.__appFrames?.stats.first ?? "";
          return f && v && c ? { frame: [f.width, f.height], canvas: [c.width, c.height], file: [v.width, v.height], first } : null;
        });
        if (!got) {
          wrong = "the video preview has no frame on its canvas";
          break;
        }
        const [cw, ch] = got.canvas;
        const [vw, vh] = got.file;
        const scale = Math.min(1, cw / vw, ch / vh);
        const even = (n) => Math.max(2, Math.round(n / 2) * 2);
        const want = [even(vw * scale), even(vh * scale)];
        if (got.frame[0] !== want[0] || got.frame[1] !== want[1]) {
          wrong = `the frame on the video preview is ${got.frame.join(" by ")}, decoded for a canvas of another size: the canvas is ${got.canvas.join(" by ")} and the file ${got.file.join(" by ")}, so it should be ${want.join(" by ")}`;
        } else if (arg === "opened" && got.first !== want.join("x")) {
          // Nothing has resized the app since it opened, so the first
          // frame it asked for was asked at this size too, not at the size
          // of a canvas the stylesheet had not laid out yet and then again.
          wrong = `the first frame was asked for at ${got.first}, before the canvas had its size, and only then at ${want.join("x")}`;
        }
        break;
      }
      case "restart": {
        const path = await page.evaluate(() => [...window.__calls].reverse().find((c) => c.name === "Clips")?.args[0]);
        await control(url, "/reopen");
        await page.reload();
        await page.waitForTimeout(1200);
        await fromSidebar(page, () => page.locator("aside li", { hasText: path.split("/").pop() }).first().click());
        await settle(page);
        break;
      }
      case "wait for": {
        const came = await page
          .waitForFunction(
            (word) => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              return pane?.querySelector(".listhead button.new")?.textContent.replace(/\s+/g, " ").trim() === word;
            },
            arg,
            { timeout: 60000, polling: 200 },
          )
          .then(() => true, () => false);
        if (!came) wrong = `the clip list's head says ${(await clipList(page))?.head}, not ${arg}, after a minute`;
        // The clip on screen may be a new one now, the first of a video
        // just added, and the watch follows it.
        else await watch.step("wait", s);
        break;
      }
      case "row": {
        const list = await clipList(page);
        if (!list?.rows.some((r) => r.what === arg)) wrong = `no row says "${arg}": ${JSON.stringify(list?.rows)}`;
        break;
      }
      case "no row": {
        const list = await clipList(page);
        if (list?.rows.some((r) => r.what === arg)) wrong = `a row says "${arg}": ${JSON.stringify(list?.rows)}`;
        break;
      }
      case "no put back": {
        const offered = await page.evaluate(
          () => [...document.querySelectorAll("aside ol button.back")].filter((b) => b.textContent.includes("Put it back")).length,
        );
        if (offered) wrong = `${offered} removed clip(s) are offered back`;
        break;
      }
      case "row closes": {
        const [words, after] = arg;
        const says = async () => !!(await clipList(page))?.rows.some((r) => r.what === words);
        if (!(await says())) {
          wrong = `no row says "${words}": ${JSON.stringify((await clipList(page))?.rows)}`;
          break;
        }
        const drains = await page.evaluate(() => !!document.querySelector("aside ol li.told .fill .run"));
        if (!drains) {
          wrong = `the row that says "${words}" does not run down`;
          break;
        }
        const from = Date.now();
        while ((await says()) && Date.now() - from < (after + 5) * 1000) await page.waitForTimeout(250);
        const took = (Date.now() - from) / 1000;
        if (await says()) wrong = `the row that says "${words}" is still there after ${took.toFixed(1)} s`;
        else if (took < after - 3) wrong = `the row that says "${words}" closed after ${took.toFixed(1)} s`;
        break;
      }
      case "cards": {
        const list = await clipList(page);
        if (list?.cards.length !== arg) wrong = `the clip list holds ${list?.cards.length} clips, not ${arg}`;
        break;
      }
      case "whole clip": {
        const clips = (await ask(page, "Clips", await episodeOn(page)))?.filter((c) => !c.rejected);
        if (clips?.length !== 1) wrong = `the episode has ${clips?.length} clips, not its one`;
        else if (clips[0].start > 1 || clips[0].end < arg - 1.5) {
          wrong = `the clip runs ${clips[0].start} to ${clips[0].end}, not the episode of ${arg} s`;
        }
        break;
      }
      case "waveform": {
        // The canvas itself, not what the interface was told: a column of
        // it with more than a few pixels painted is the waveform there.
        // The bridge's episode has sound from its first second to its last.
        let read = null;
        for (let tries = 0; tries < 40; tries++) {
          read = await page.evaluate(() => {
            const c = document.querySelector(".clip-timeline .track canvas");
            if (!c?.width || !c.height) return null;
            const d = c.getContext("2d").getImageData(0, 0, c.width, c.height).data;
            let drawn = 0;
            for (let x = 0; x < c.width; x++) {
              let painted = 0;
              for (let y = 0; y < c.height; y++) if (d[(y * c.width + x) * 4 + 3] > 0) painted++;
              if (painted > 6) drawn++;
            }
            return { drawn, width: c.width };
          });
          if (read && read.drawn >= read.width * 0.9) break;
          await page.waitForTimeout(250);
        }
        if (!read) wrong = "the clip timeline has no canvas";
        else if (read.drawn < read.width * 0.9) {
          wrong = `the clip timeline draws the waveform in ${read.drawn} of its ${read.width} columns after ten seconds`;
        }
        break;
      }
      case "rulers fit": {
        // Waited for while the workspace of an episode just added opens,
        // and then looked at once: looked at until it held, it held as
        // soon as the clip came, and passed on the old clip timeline.
        const seconds = (time) => time.split(":").reduce((sum, part) => sum * 60 + Number(part), 0);
        for (let tries = 0; tries < 40; tries++) {
          wrong = "";
          const read = await page.evaluate(() => {
            const picker = document.querySelector('[aria-label="The range picker"]');
            const text = (e) => (e.textContent ?? "").trim();
            return {
              lines: picker?.querySelectorAll(".tick").length ?? 0,
              times: [...(picker?.querySelectorAll(".time:not(.start)") ?? [])].map(text),
              timeline: [...document.querySelectorAll(".clip-timeline .time")].map(text),
            };
          });
          const picked = read.times.map(seconds);
          const shown = read.timeline.map(seconds);
          if (read.lines < 2 || picked.length < 2) {
            wrong = `the range picker has ${read.lines} lines and the times ${JSON.stringify(read.times)}`;
          } else if (picked.some((t) => t <= 0 || t >= arg)) {
            wrong = `the range picker has times outside the episode: ${JSON.stringify(read.times)}`;
          } else if (!shown.length || Math.max(...shown) > arg || Math.max(...shown) < arg - 2) {
            wrong = `the clip timeline shows the times ${JSON.stringify(read.timeline)} for an episode of ${arg} s`;
          }
          if (!wrong || (read.lines > 0 && shown.length > 0)) break;
          await page.waitForTimeout(250);
        }
        break;
      }
      case "mark":
        marks[arg] = await engineState(page, watch.at);
        break;
      case "made on the clip": {
        if (!made) {
          wrong = "made on the clip with no clip made";
          break;
        }
        const { playing, was, written, overlays } = made;
        if (!written) {
          wrong = "the clip made was never written and chosen while it played";
          break;
        }
        // From the frame the clip made is the chosen card on.
        const from = overlays.findIndex((o) => o.card && o.card !== was);
        const since = overlays.slice(from);
        const dim = since.find((o) => o.video);
        if (playing && !since.some((o) => o.playing && o.card === written)) {
          wrong = "the play had stopped before the clip made was written";
        } else if (!since.some((o) => !o.playing)) {
          wrong = "the play was not paused at the end";
        } else if (dim) {
          const when = dim.card === written ? "written" : dim.card ? "on its way" : "with no card chosen";
          wrong = `the clip made was dimmed at ${dim.at.toFixed(3)}, ${dim.playing ? "playing" : "paused"}, ${when}`;
        }
        break;
      }
      case "only the video":
      case "the clip shown": {
        if (!played) {
          wrong = `${what} with nothing played`;
          break;
        }
        const { overlays, state, fps } = played;
        const video = what === "only the video";
        const frame = 1 / fps;
        // With "across cuts" the play has to have crossed a cut: the
        // episode through the part cut out, from a piece before it to a
        // piece after it, and the clip from one piece on into the next.
        const pieceOf = (at) => state.segments.findIndex((p) => at >= p.start + frame && at < p.end - frame);
        const played_ = overlays.filter((o) => o.playing);
        const pieces = new Set(played_.map((o) => pieceOf(o.at)).filter((i) => i >= 0));
        const inCut = played_.some((o) => state.segments.some((p, i) => i > 0 && o.at > state.segments[i - 1].end + frame && o.at < p.start - frame));
        if (arg === "across cuts" && state.segments.length < 2) {
          wrong = `the clip has ${state.segments.length} piece, so no cut to play across`;
          break;
        }
        if (arg === "across cuts" && pieces.size < 2) {
          wrong = `the play stood in ${pieces.size} piece of the clip, so it crossed no cut`;
          break;
        }
        if (arg === "across cuts" && video && !inCut) {
          wrong = "the episode played through no part cut out of the clip";
          break;
        }
        // The frames of the play that stood in one of the clip's pieces,
        // which is where a clip's frame and captions could show.
        const inside = overlays.filter((o) => o.playing && state.segments.some((p) => o.at >= p.start + frame && o.at < p.end - frame));
        const span = inside.length ? inside[inside.length - 1].at - inside[0].at : 0;
        const elsewhere = overlays.filter((o) => o.playing && o.video !== video);
        if (elsewhere.length) {
          wrong = `the playhead was on the ${video ? "clip" : "video"} at ${elsewhere[0].at.toFixed(3)} while it played`;
        } else if (span < 2) {
          wrong = `the play stood inside the clip for ${span.toFixed(2)} s of the episode, in ${inside.length} animation frames`;
        } else if (!video && !inside.some((o) => o.captions)) {
          wrong = "the clip played with no caption shown";
        } else {
          wrong = overlaid(overlays, state, frame);
        }
        break;
      }
      case "opened on the clip": {
        // The video preview of an episode that opens on a clip draws the
        // clip's first frame and nothing before it: no frame of the
        // episode's start on the way to the clip.
        const on = await chosen(page);
        const clip = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const fps = await rate(page);
        const first = Math.floor((clip.start + 0.001) * fps);
        const drawn = await page.evaluate(() => window.__drawn ?? []);
        if (!drawn.length) wrong = "the video preview drew no frame of the episode it opened";
        else if (drawn.some((n) => n !== first)) {
          wrong = `the episode opened on its clip at frame ${first} and the video preview drew frames ${drawn.join(", ")} on the way`;
        }
        // Where the playhead opens comes before anything else the
        // workspace reads as it opens: the window, which it reads last and
        // only as it opens, is asked with the playhead on the clip. The
        // coverage and the room are read again whenever work elsewhere
        // ends, a search of the episode added before, so they say nothing
        // about the order the workspace opens in. A workspace kept from
        // before, see App.svelte, is woken rather than built and asks
        // neither the clip it opens on nor the window: it has them.
        const asked = await page.evaluate(() => window.__asked ?? []);
        const after = ["ChosenWindow"];
        const built = asked.some(([name]) => name === "ChosenClip");
        const early = asked.filter(([name, at]) => after.includes(name) && Math.abs(at - clip.start) > 0.002);
        if (!wrong && built && !asked.some(([name]) => after.includes(name))) wrong = `the workspace opened asking none of ${after.join(", ")}`;
        else if (!wrong && early.length) {
          wrong = `the workspace asked ${early.map(([n, at]) => `${n} with the playhead at ${at}`).join(", ")}, before the playhead was on its clip at ${clip.start}`;
        }
        break;
      }
      case "left out": {
        const said = await page.evaluate(() => document.querySelector(".error")?.textContent ?? "");
        const library = await ask(page, "Library");
        if (!said.includes(`the picture of ${arg}`) || !said.includes("cannot be decoded")) {
          wrong = `the app says "${said.trim()}", not why ${arg} was left out`;
        } else if (library.some((e) => e.name.startsWith(arg))) {
          wrong = `${arg} is in the library`;
        }
        break;
      }
      case "played through": {
        const played = await page.evaluate(() => window.__played ?? []);
        let still = 0;
        let longest = 0;
        for (let i = 1; i < played.length && !wrong; i++) {
          const [t, at, n] = played[i];
          const [t0, at0, n0] = played[i - 1];
          if (at < at0 - 1e-6) wrong = `the playhead went back from ${at0.toFixed(3)} to ${at.toFixed(3)} while the hand was on the edge`;
          else if (n >= 0 && n0 >= 0 && n < n0) wrong = `frame ${n} was drawn after frame ${n0} while the hand was on the edge`;
          if (at === at0) still += t - t0;
          else still = 0;
          longest = Math.max(longest, still);
        }
        if (!wrong && played.length < 30) wrong = `only ${played.length} animation frames were read`;
        if (!wrong && longest > 150) wrong = `the playhead stood still for ${longest.toFixed(0)} ms while the hand was on the edge`;
        break;
      }
      case "stopped at its end": {
        if (!loopEnd) {
          wrong = "stopped at its end with nothing played";
          break;
        }
        const { end, fps } = loopEnd;
        const played = await page.evaluate(() => window.__played ?? []);
        // Reached once the playhead stands in the clip's last frame.
        const i = played.findIndex(([, at]) => at >= end - 1.5 / fps);
        if (i < 0) {
          wrong = `the playhead never reached the clip's end at ${end.toFixed(3)}, it got to ${Math.max(...played.map(([, at]) => at)).toFixed(3)}`;
          break;
        }
        const [reached] = played[i];
        const stopped = played.slice(i).find(([, , , playing]) => !playing);
        const old = played.slice(i).find(([, , n]) => n >= 0 && n / fps < end - 1);
        // The sound, sample by sample. The bridge's episode sounds a steady
        // tone, so a click is a step in it: from one sample to the next the
        // tone bends by its second difference, never more than a few
        // thousandths of its height, and a stop or a restart jumps by a
        // good part of it. The sound card's time of the program's end is
        // where the sound ends, and after it nothing.
        const { heard, c0, m0, rate: sr, programEnd } = loopEnd;
        const xs = [];
        for (const h of heard) for (let k = 0; k < h.data.length; k++) xs.push([h.frame + k, h.data[k]]);
        const endFrame = Math.round((c0 + (Math.round(programEnd * sr) - m0) / sr) * sr);
        let height = 0;
        for (const [f, x] of xs) if (f < endFrame) height = Math.max(height, Math.abs(x));
        let clicks = 0;
        let firstClick = null;
        let after = 0;
        for (let k = 2; k < xs.length; k++) {
          const [f, x] = xs[k];
          if (f >= endFrame + 2) {
            if (Math.abs(x) > 1e-3) after++;
            continue;
          }
          if (xs[k - 1][0] !== f - 1 || xs[k - 2][0] !== f - 2) continue;
          if (Math.abs(x - 2 * xs[k - 1][1] + xs[k - 2][1]) > 0.03 * height) {
            clicks++;
            firstClick ??= ((f - endFrame) / sr) * 1000;
          }
        }
        if (loopEnd.starts) wrong = `the play started over ${loopEnd.starts} times while L was pressed`;
        else if (!(height > 0.01)) wrong = `the sound card was handed nothing to hear, ${xs.length} samples`;
        else if (clicks) wrong = `the sound clicked ${clicks} times, the first ${(-firstClick).toFixed(0)} ms before the end`;
        else if (after) wrong = `${after} samples of sound were heard after the clip's end`;
        else if (!stopped) wrong = `the play went on for ${(played.at(-1)[0] - reached).toFixed(0)} ms with the playhead at the clip's end`;
        else if (stopped[0] - reached > 400) wrong = `the play stopped ${(stopped[0] - reached).toFixed(0)} ms after the playhead reached the clip's end`;
        else if (old) wrong = `frame ${(old[2] / fps).toFixed(3)} was drawn ${(old[0] - reached).toFixed(0)} ms after the playhead reached the clip's end at ${end.toFixed(3)}`;
        break;
      }
      case "nailed": {
        const { played, hand } = await page.evaluate(() => ({ played: window.__played ?? [], hand: window.__hand ?? {} }));
        // A seek lands within a few frames of the hand, so each still
        // stretch is read from 200 ms after it starts.
        const during = (from, to) => played.filter(([t]) => t >= from && t < to);
        const stood = (from, to, what) => {
          const got = during(from + 200, to);
          if (got.length < 10) return `only ${got.length} animation frames were read ${what}`;
          const ats = new Set(got.map(([, at]) => at));
          if (ats.size > 1) {
            const all = got.map(([, at]) => at);
            return `the playhead moved from ${Math.min(...all).toFixed(3)} to ${Math.max(...all).toFixed(3)} ${what}`;
          }
          const frames = new Set(got.map(([, , n]) => n));
          if (frames.size > 1) return `${frames.size} different frames were drawn ${what}`;
          return got[0][1];
        };
        const first = stood(hand.down, hand.moves, "while the hand held still on the press");
        if (typeof first === "string") {
          wrong = first;
          break;
        }
        const second = stood(hand.still, hand.up, "while the hand held still where it moved to");
        if (typeof second === "string") {
          wrong = second;
          break;
        }
        if (!(second < first)) {
          wrong = `the playhead did not follow the hand, it stood at ${first.toFixed(3)} and then at ${second.toFixed(3)}`;
          break;
        }
        if (during(hand.down, hand.up).some(([, , , playing]) => !playing)) {
          wrong = "the button said Play while the hand held the playhead";
          break;
        }
        const after = during(hand.up + 300, Infinity);
        if (after.length < 10) {
          wrong = `only ${after.length} animation frames were read after the hand let go`;
          break;
        }
        const ats = after.map(([, at]) => at);
        if (!after.every(([, , , playing]) => playing)) wrong = "the play did not go on after the hand let go";
        else if (ats[ats.length - 1] <= ats[0]) wrong = "the playhead stood still after the hand let go";
        else if (ats[0] < second - 1e-6 || ats[0] > second + 1) {
          wrong = `the play went on from ${ats[0].toFixed(3)}, not from ${second.toFixed(3)} where the hand let go`;
        }
        break;
      }
      case "box": {
        const box = s.map((w) => w.text).join(" ");
        if (box !== arg) wrong = `the caption box reads "${box}", not "${arg}"`;
        break;
      }
      case "open": {
        const o = s.find((w) => w.focused);
        if (o?.text !== arg) wrong = `"${o?.text ?? "nothing"}" is open, not "${arg}"`;
        break;
      }
      case "same": {
        const now = await engineState(page, watch.at);
        if (!same(now, marks[arg])) wrong = `the captions are not what they were at "${arg}"\n${describe(marks[arg], now)}`;
        break;
      }
      case "spans": {
        const spans = (caps) => caps.captions.map((c) => `${c.start.toFixed(3)}-${c.end.toFixed(3)}`).join(" ");
        const now = await engineState(page, watch.at);
        if (spans(now) !== spans(marks[arg])) {
          wrong = `the captions do not appear and go when they did at "${arg}"\nthen ${spans(marks[arg])}\nnow  ${spans(now)}\n${describe(marks[arg], now)}`;
        }
        break;
      }
      case "rendered": {
        const on = await chosen(page);
        const short = await shortOf(page, on);
        // The episode's own folder in the folder for shorts, named after
        // its file, see EpisodeName in engine/run.go.
        const shorts = realpathSync((await ask(page, "GetSettings")).outputDir);
        const folder = join(shorts, basename(on.path, extname(on.path)));
        const render = page.locator("button.primary.render");
        const again = await page
          .waitForFunction((r) => document.querySelector(r)?.textContent.trim() === "Render again", "button.primary.render", {
            timeout: 10000,
            polling: 200,
          })
          .then(() => true, () => false);
        if (!short) wrong = "the clip says it has no short";
        else if (dirname(short) !== folder) wrong = `the short is at ${short}, not in the folder for shorts, ${folder}`;
        else if (!existsSync(short)) wrong = `the clip says its short is at ${short}, and nothing is there`;
        else if (!again) wrong = `the Render button says ${(await render.textContent())?.trim()}, not Render again`;
        else if (!(await page.getByRole("button", { name: "Show in folder" }).count())) wrong = "Show in folder is not there";
        break;
      }
      case "remove clip": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        // The bin shows while the pointer is on the card, as it does for
        // a hand.
        const bin = page.locator(`button[aria-label="Remove ${entry.title || entry.slug}"]`);
        await page.hover(`li:has(> button[aria-label="Remove ${entry.title || entry.slug}"])`);
        await bin.click();
        let gone = false;
        for (let i = 0; i < 50 && !gone; i++) {
          await page.waitForTimeout(100);
          gone = (await ask(page, "Clips", on.path)).some((c) => c.plan === on.plan && c.id === on.clip && c.rejected);
        }
        if (!gone) wrong = "the bin left the clip in the list";
        await watch.step("remove clip", s);
        break;
      }
      case "thumbnail": {
        const on = await chosen(page);
        const count = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.thumbnails.length ?? 0;
        const before = await count();
        await page.keyboard.press("t");
        let now = before;
        for (let i = 0; i < 50 && now === before; i++) {
          await page.waitForTimeout(100);
          now = await count();
        }
        if (now !== before + 1) wrong = `T left the clip with ${now} thumbnails, not ${before + 1}`;
        await watch.step("press", s);
        break;
      }
      case "keep": {
        const on = await chosen(page);
        const short = await shortOf(page, on);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const file = short && existsSync(short) ? statSync(short) : null;
        kept[arg] = { ...on, name: entry?.slug, short, file };
        if (!file) {
          wrong = `the clip has no short to keep${short ? `, nothing is at ${short}` : ""}`;
          break;
        }
        kept[arg].shorts = shortsIn(dirname(short));
        kept[arg].pictures = picturesOf(short).map((p) => ({ path: p, file: statSync(p) }));
        break;
      }
      case "by its words": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const short = await shortOf(page, on);
        if (!short || !existsSync(short)) wrong = "the clip has no short in its folder";
        else if (basename(short) !== `${entry?.slug}.mp4`) wrong = `the short is called ${basename(short)}, not ${entry?.slug}.mp4`;
        break;
      }
      case "pictured": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const short = await shortOf(page, on);
        const want = entry?.thumbnails.length ?? 0;
        const got = short ? picturesOf(short) : [];
        const named = got.map((p) => basename(p)).join(", ");
        if (!short) wrong = "the clip says it has no short";
        else if (want === 0) wrong = "the clip has no thumbnails, so there is nothing to look for";
        else if (got.length !== want || got.some((p, i) => basename(p) !== `${basename(short, ".mp4")}-${i + 1}.jpg`)) {
          wrong = `beside ${short} lie ${named || "no pictures"}, not the clip's ${want}`;
        }
        break;
      }
      case "in place": {
        const was = kept[arg];
        const on = await chosen(page);
        const short = await shortOf(page, on);
        const now = short && existsSync(short) ? statSync(short) : null;
        const added = short ? shortsIn(dirname(short)).filter((n) => !was.shorts.includes(n)) : [];
        if (short !== was.short) wrong = `rendered again, the short is at ${short || "nowhere"}, not ${was.short}`;
        else if (!now || now.mtimeMs === was.file.mtimeMs) wrong = `rendered again, ${short} was not written again`;
        else if (added.length) wrong = `rendered again, ${added.join(", ")} appeared beside ${basename(short)}`;
        break;
      }
      case "add namesake":
        await control(url, `/pick?seconds=${arg}&namesake=1`);
        await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
        await watch.step("add", s);
        break;
      case "words again":
        await control(url, "/speech?ms=0&from=0");
        break;
      case "updates": {
        const q = new URLSearchParams({
          from: arg.from ?? "",
          stored: arg.stored ?? "",
          list: arg.list ?? "",
          conflict: arg.conflict ?? "",
        });
        await control(url, `/updates?${q}`);
        await fromSidebar(page, () => page.locator("aside").getByText("Updates", { exact: true }).first().click());
        await page.locator("#channel").waitFor();
        await settle(page);
        break;
      }
      case "follow": {
        await page.locator("#channel").click();
        await page.getByRole("option", { name: arg, exact: true }).click();
        const read = await page.evaluate(
          (src) => new Promise((done) => requestAnimationFrame(() => done(new Function(`return (${src})()`)()))),
          readUpdates.toString(),
        );
        picked = { name: arg, ...read };
        break;
      }
      case "channel": {
        const now = await updatesComeTo(page, (u) => u.channel === arg);
        if (now.channel !== arg) wrong = `the list says ${now.channel || "nothing"}, not ${arg}`;
        break;
      }
      case "choices": {
        const [names, want] = arg;
        await page.locator("#channel").click();
        await page.getByRole("option").first().waitFor();
        const rows = await page.evaluate(() =>
          [...document.querySelectorAll('[role="option"]')].map((o) => ({
            name: (o.textContent ?? "").replace(/\s+/g, " ").trim(),
            picked: o.hasAttribute("data-selected") || o.getAttribute("aria-selected") === "true",
          })),
        );
        await page.keyboard.press("Escape");
        await page.waitForTimeout(300);
        const got = rows.map((r) => r.name);
        const on = rows.filter((r) => r.picked).map((r) => r.name);
        if (JSON.stringify(got) !== JSON.stringify(names)) wrong = `the list offers ${JSON.stringify(got)}, not ${JSON.stringify(names)}`;
        else if (JSON.stringify(on) !== JSON.stringify(want ? [want] : [])) {
          wrong = `the list has ${on.length ? on.join(", ") : "nothing"} picked, not ${want || "nothing"}`;
        }
        break;
      }
      case "status": {
        const [head, more] = [arg].flat();
        const now = await updatesComeTo(page, (u) => u.head === head);
        if (now.head !== head) wrong = `the line under the build says ${now.head || "nothing"}, not ${head}`;
        else if (more && !now.more.includes(more)) wrong = `under ${head} it says ${JSON.stringify(now.more)}, not ${JSON.stringify(more)}`;
        break;
      }
      case "mark": {
        const now = await updatesComeTo(page, (u) => u.mark === arg);
        if (now.mark !== arg) wrong = `the line under the build starts with ${now.mark || "no mark"}, not ${arg}`;
        break;
      }
      case "warned": {
        const now = await updatesComeTo(page, (u) => u.warned);
        if (!now.warned) wrong = `the list does not wear the colour of a warning`;
        break;
      }
      case "fits": {
        const now = await updatesPage(page);
        if (now.slack < 0 || now.slack > 60) wrong = `the list's trigger keeps ${Math.round(now.slack)} px beside ${now.channel}, room for a longer name`;
        break;
      }
      case "no check": {
        const now = await updatesPage(page);
        if (now.button) wrong = `the line under the build offers ${now.button}`;
        break;
      }
      case "at once":
        if (picked?.channel !== picked?.name) wrong = `in the frame after picking ${picked?.name}, the list said ${picked?.channel || "nothing"}`;
        else if (picked.head !== arg) wrong = `in the frame after the pick, the line under the build said ${picked.head || "nothing"}, not ${arg}`;
        break;
      case "apart": {
        const [label, number] = [arg].flat();
        const was = kept[label];
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const short = await shortOf(page, on);
        const now = existsSync(was.short) ? statSync(was.short) : null;
        const theirs = await shortOf(page, was);
        if (on.path === was.path) wrong = `the clip on screen is of the episode kept at "${label}"`;
        else if (entry?.slug !== was.name) wrong = `the clips are named ${was.name} and ${entry?.slug}, which never meet in one folder`;
        else if (!short) wrong = "the clip on screen says it has no short";
        else if (short === was.short) wrong = `both episodes' shorts are ${short}`;
        else if (!now || now.ino !== was.file.ino || now.mtimeMs !== was.file.mtimeMs) {
          wrong = `the second render wrote over the first episode's short at ${was.short}`;
        } else if (theirs !== was.short) {
          wrong = `the first episode's clip says its short is ${theirs || "nowhere"}, not ${was.short}`;
        } else if (number && basename(short) !== `${was.name}${number}.mp4`) {
          wrong = `the second short is called ${basename(short)}, not ${was.name}${number}.mp4`;
        } else {
          const gone = was.pictures.filter((p) => {
            const now = existsSync(p.path) ? statSync(p.path) : null;
            return !now || now.ino !== p.file.ino || now.mtimeMs !== p.file.mtimeMs;
          });
          if (gone.length) wrong = `the second render took or wrote over the first episode's pictures ${gone.map((p) => basename(p.path)).join(", ")}`;
        }
        break;
      }
      case "as previewed": {
        const [rate, before] = [arg].flat();
        wrong = await asPreviewed(page, rate, before);
        break;
      }
      case "crop to": {
        // Taken a little below its top, away from the captions.
        const f = await page.locator(".player .frame").boundingBox();
        const w = await page.evaluate(() => document.querySelector(".screen").getBoundingClientRect().width);
        await drag(page, f.x + f.width / 2, f.y + 0.2 * f.height, arg === "left" ? -w : w);
        await settle(page);
        break;
      }
      case "frame whole":
        wrong = await frameWhole(page, arg === "turned");
        break;
      case "on borders":
        wrong = await onBorders(page);
        break;
      case "edges as rendered":
        wrong = await edgesAsRendered(page);
        break;
      case "rotate at": {
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        await page.mouse.click(from + arg * (g.end.x - from), high(g.track));
        await settle(page);
        const turn = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.turn));
        const before = await turn();
        const pressed = Date.now();
        await page.keyboard.press("r");
        const took = await page
          .waitForFunction((b) => Number(document.querySelector(".screen").dataset.turn) !== b, before, { polling: "raf", timeout: 3000 })
          .then(() => Date.now() - pressed, () => -1);
        if (took < 0) wrong = `R left the video preview rotated ${before}`;
        else if (took > 300) wrong = `the video preview rotated ${took} ms after R, not at once`;
        await watch.step("press", s);
        break;
      }
      case "keyframe at": {
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        await page.mouse.click(from + arg * (g.end.x - from), high(g.track));
        await settle(page);
        const count = () => page.evaluate(() => document.querySelectorAll(".clip-timeline .cropdot").length);
        const before = await count();
        const pressed = Date.now();
        await page.keyboard.press("c");
        const took = await page
          .waitForFunction(
            (b) => document.querySelectorAll(".clip-timeline .cropdot").length > b && !!document.querySelector(".clip-timeline .cropdot.here"),
            before,
            { polling: "raf", timeout: 3000 },
          )
          .then(() => Date.now() - pressed, () => -1);
        if (took < 0) wrong = "C put no keyframe on the playhead's frame";
        else if (took > 300) wrong = `the keyframe showed ${took} ms after C, not at once`;
        await watch.step("press", s);
        await settle(page);
        break;
      }
      case "keyframe on": {
        const on = await chosen(page);
        const entry = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const src = await ask(page, "Source", on.path);
        const pieces = (await entry()).segments;
        const want = arg === "last" ? pieces[pieces.length - 1].end - 1 / src.fps : pieces[0].start;
        const g = await handles(page);
        const edge = arg === "last" ? g.end : g.start;
        await page.mouse.click(edge.x + edge.w / 2, edge.y + edge.h / 2);
        await settle(page);
        if (arg === "first") {
          for (const key of ["ArrowRight", "ArrowRight", "ArrowRight", "ArrowLeft", "ArrowLeft", "ArrowLeft"]) {
            await page.keyboard.press(key);
            await settle(page);
          }
        }
        const marks = async () => (await entry()).cropMarks ?? [];
        const before = (await marks()).length;
        const lit = () => page.evaluate(() => !!document.querySelector(".clip-timeline .cropdot.here"));
        await page.keyboard.press("c");
        let got = [];
        for (let i = 0; i < 30 && got.length <= before; i++) {
          await page.waitForTimeout(100);
          got = await marks();
        }
        const near = got.find((m) => Math.abs(m.at - want) < 0.5 / src.fps);
        if (got.length <= before) {
          const at = await page.evaluate(() => document.querySelector(".screen").dataset.playhead);
          wrong = `C put no keyframe with the playhead at ${Number(at).toFixed(4)} s, on the clip's ${arg} frame at ${want.toFixed(4)} s`;
          break;
        }
        if (!near) {
          wrong = `C put a keyframe at ${got.map((m) => m.at).join(", ")}, not on the clip's ${arg} frame at ${want.toFixed(3)} s`;
          break;
        }
        await settle(page);
        if (!(await lit())) {
          wrong = `the keyframe on the clip's ${arg} frame does not light while that frame is on the video preview`;
          break;
        }
        await page.keyboard.press("c");
        for (let i = 0; i < 30 && (await marks()).length > before; i++) await page.waitForTimeout(100);
        if ((await marks()).length > before) wrong = `C again did not remove the keyframe on the clip's ${arg} frame`;
        await settle(page);
        break;
      }
      case "thumbnail at": {
        const on = await chosen(page);
        const count = async () => ((await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.thumbnails ?? []).length;
        const before = await count();
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        await page.mouse.click(from + arg * (g.end.x - from), high(g.track));
        await settle(page);
        await page.keyboard.press("t");
        await watch.step("press", s);
        await settle(page);
        if ((await count()) !== before + 1) wrong = "T made no thumbnail";
        break;
      }
      case "under the hand": {
        // A mark the playhead is not on, so only the hand can light it,
        // tagged so it is the same element all the way through.
        const kind = arg === "crop" ? ".cropdot" : ".thumb";
        const tagged = await page.evaluate((kind) => {
          const el = [...document.querySelectorAll(`.clip-timeline ${kind}`)].find((m) => !m.classList.contains("here"));
          el?.setAttribute("data-probe", "");
          return !!el;
        }, kind);
        const box = tagged ? await page.locator("[data-probe]").boundingBox() : null;
        if (!box) {
          wrong = `the clip timeline shows no ${arg} mark off the playhead`;
          break;
        }
        const paint = () =>
          page.evaluate(() => {
            const el = document.querySelector("[data-probe]");
            const look = getComputedStyle(el, el.classList.contains("cropdot") ? "::after" : null);
            return `${look.backgroundColor} ${look.color}`;
          });
        const away = await paint();
        const x = box.x + box.width / 2;
        const y = box.y + box.height / 2;
        await page.mouse.move(x, y);
        await page.waitForTimeout(100);
        const got = await page.evaluate(
          ({ x, y }) => {
            const el = document.querySelector("[data-probe]");
            return { top: document.elementFromPoint(x, y)?.closest("[data-probe]") === el, hover: el.matches(":hover") };
          },
          { x, y },
        );
        const lit = await paint();
        await page.evaluate(() => document.querySelector("[data-probe]")?.removeAttribute("data-probe"));
        if (!got.top) wrong = `something else lies over the ${arg} mark, so the pointer never reaches it`;
        else if (!got.hover) wrong = `the ${arg} mark does not take the hover`;
        else if (lit === away) wrong = `the ${arg} mark looks the same under the hand as away from it, ${away}`;
        break;
      }
      case "frames on the pills": {
        const fps = await rate(page);
        const state = await engineState(page, watch.at);
        const pieces = state.segments;
        // A moment of the clip's clock in the episode.
        const into = (t) => {
          let sum = 0;
          for (const p of pieces) {
            if (t < sum + p.end - p.start) return p.start + t - sum;
            sum += p.end - p.start;
          }
          return null;
        };
        // Away from the clip's edges, where a click takes the edge.
        const length = pieces.reduce((n, p) => n + p.end - p.start, 0);
        const words = (state.captions ?? [])
          .flatMap((c) => c.lines.flatMap((l) => l.words))
          .filter((w) => (w.start + w.end) / 2 > 0.6 && (w.start + w.end) / 2 < length - 0.6)
          .slice(0, arg);
        const report = [];
        for (const w of words) {
          const at = into((w.start + w.end) / 2);
          if (at === null) continue;
          if (!(await seekTo(page, at, fps))) {
            wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
            break;
          }
          await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
          await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
          const got = await page.evaluate(() => {
            const word = document.querySelector(".captions .word.now");
            const canvas = document.querySelector(".screen canvas");
            if (!word || !canvas?.width) return { none: !word ? "no word is lit" : "there is no canvas" };
            const c = canvas.getBoundingClientRect();
            const f = word.getBoundingClientRect();
            const sx = canvas.width / c.width;
            const sy = canvas.height / c.height;
            const pill = getComputedStyle(word).getPropertyValue("--pill").match(/[\d.]+/g).map(Number);
            // Round the frame by its own height, so a pill beside it is
            // found too, but no other word's.
            const x0 = Math.max(0, Math.floor((f.left - c.left - f.height / 2) * sx));
            const x1 = Math.min(canvas.width, Math.ceil((f.right - c.left + f.height / 2) * sx));
            const y0 = Math.max(0, Math.floor((f.top - c.top - f.height) * sy));
            const y1 = Math.min(canvas.height, Math.ceil((f.bottom - c.top + f.height) * sy));
            const s = document.createElement("canvas");
            s.width = x1 - x0;
            s.height = y1 - y0;
            const g = s.getContext("2d", { willReadFrequently: true });
            g.drawImage(canvas, x0, y0, s.width, s.height, 0, 0, s.width, s.height);
            const d = g.getImageData(0, 0, s.width, s.height).data;
            let [l, r, t, b] = [Infinity, -Infinity, Infinity, -Infinity];
            for (let y = 0; y < s.height; y++) {
              for (let x = 0; x < s.width; x++) {
                const i = (y * s.width + x) * 4;
                if ([0, 1, 2].every((k) => Math.abs(d[i + k] - pill[k]) < 40)) {
                  [l, r, t, b] = [Math.min(l, x), Math.max(r, x + 1), Math.min(t, y), Math.max(b, y + 1)];
                }
              }
            }
            if (l === Infinity) return { none: "no pill is drawn round the lit word" };
            // Both in CSS pixels from the canvas's corner.
            return {
              text: word.textContent,
              frame: { l: f.left - c.left, r: f.right - c.left, t: f.top - c.top, b: f.bottom - c.top },
              pill: { l: (x0 + l) / sx, r: (x0 + r) / sx, t: (y0 + t) / sy, b: (y0 + b) / sy },
            };
          });
          if (got.none) {
            wrong = `at ${at.toFixed(2)} s ${got.none}`;
            break;
          }
          const { frame, pill, text } = got;
          const mid = (a, b) => (a + b) / 2;
          const across = mid(frame.l, frame.r) - mid(pill.l, pill.r);
          const down = mid(frame.t, frame.b) - mid(pill.t, pill.b);
          const wider = frame.r - frame.l - (pill.r - pill.l);
          const lower = frame.b - frame.t - (pill.b - pill.t);
          report.push(`"${text}": ${across.toFixed(1)} px across, ${down.toFixed(1)} px down, ${wider.toFixed(1)} px wider, ${lower.toFixed(1)} px taller`);
          if (Math.abs(across) > 1.5 || Math.abs(down) > 1.5 || Math.abs(wider) > 2 || lower > 2) {
            wrong = `the frame round "${text}" stands ${across.toFixed(1)} px across and ${down.toFixed(1)} px down from the middle of its pill, and is ${(frame.r - frame.l).toFixed(1)}x${(frame.b - frame.t).toFixed(1)} px round a pill of ${(pill.r - pill.l).toFixed(1)}x${(pill.b - pill.t).toFixed(1)}`;
            break;
          }
        }
        if (!wrong && report.length < Math.min(arg, words.length)) wrong = `only ${report.length} words were measured`;
        console.log(`    ${report.join("\n    ")}`);
        break;
      }
      case "open a word, then the mark": {
        const kind = arg === "crop" ? ".cropdot" : ".thumb";
        const mark = await page.locator(`.clip-timeline ${kind}`).first().boundingBox();
        if (!mark) {
          wrong = `the clip timeline shows no ${arg} mark`;
          break;
        }
        const mx = mark.x + mark.width / 2;
        const my = mark.y + mark.height / 2;
        const playhead = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
        const words = async () => (await shown(page)).map((w) => w.at).join(" ");
        await page.mouse.click(mx, my);
        await settle(page);
        const onMark = await playhead();
        const there = await words();
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        const other = mx < (from + g.end.x) / 2 ? 0.75 : 0.25;
        await page.mouse.click(from + other * (g.end.x - from), high(g.track));
        await settle(page);
        const first = page.locator(".captions .word.correctable").first();
        if (!(await first.count())) {
          wrong = "no word in the caption box to open";
          break;
        }
        const before = await shown(page);
        await first.click();
        await watch.step("click", before);
        if (!(await shown(page)).some((w) => w.focused)) {
          wrong = "the word clicked did not open";
          break;
        }
        if ((await words()) === there) {
          wrong = "the caption on the mark is the caption elsewhere too, so this proves nothing";
          break;
        }
        await page.mouse.click(mx, my);
        await settle(page);
        // The caption box follows within a few frames, the way it does
        // after a click on the clip timeline.
        const followed = await page
          .waitForFunction(
            (there) => {
              const words = [...document.querySelectorAll(".captions .word")];
              return (
                !document.querySelector(".captions.typing") &&
                !words.includes(document.activeElement) &&
                words.map((w) => Number(w.dataset.at)).join(" ") === there
              );
            },
            there,
            { polling: 50, timeout: 2000 },
          )
          .then(
            () => true,
            () => false,
          );
        const now = await shown(page);
        const still = now.find((w) => w.focused);
        const at = await playhead();
        if (Math.abs(at - onMark) > 0.05) wrong = `a click on the ${arg} mark put the playhead at ${at}, not on the mark at ${onMark}`;
        else if (still) wrong = `after a click on the ${arg} mark, "${still.text}" is still open`;
        else if (!followed)
          wrong = `after a click on the ${arg} mark, the caption box holds "${now.map((w) => w.text).join(" ")}", not the caption on the mark`;
        break;
      }
      case "remove mark": {
        const on = await chosen(page);
        const entry = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const count = (c) => (arg === "crop" ? c?.cropMarks ?? [] : c?.thumbnails ?? []).length;
        const was = await entry();
        const box = await page.locator(arg === "crop" ? ".clip-timeline .cropdot" : ".clip-timeline .thumb").first().boundingBox();
        if (!box) {
          wrong = `the clip timeline shows no ${arg} mark`;
          break;
        }
        await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
        await watch.step("double-click", s);
        await settle(page);
        const now = await entry();
        if (count(now) !== count(was) - 1) wrong = `a double-click left the clip ${count(now)} ${arg} marks, not ${count(was) - 1}`;
        else if (now.segments.length !== was.segments.length) wrong = `a double-click on the ${arg} mark cut the clip`;
        break;
      }
      case "keyframe past": {
        const on = await chosen(page);
        const entry = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const src = await ask(page, "Source", on.path);
        const pieces = (await entry()).segments;
        const want = arg === "end" ? pieces[pieces.length - 1].end - 1 / src.fps : pieces[0].start;
        const centre = () =>
          page.evaluate(() => {
            const b = document.querySelector(".clip-timeline .cropdot").getBoundingClientRect();
            return b.left + b.width / 2;
          });
        const dot = await page.locator(".clip-timeline .cropdot").first().boundingBox();
        const g = await handles(page);
        const y = dot.y + dot.height / 2;
        const x = dot.x + dot.width / 2;
        const to = arg === "end" ? g.end.x + g.end.w + 60 : g.start.x - 60;
        await page.mouse.move(x, y);
        await page.mouse.down();
        const held = [];
        for (let i = 1; i <= 12; i++) {
          await page.mouse.move(x + ((to - x) * i) / 12, y);
          await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
          held.push(await centre());
        }
        await page.mouse.up();
        let got = [];
        for (let i = 0; i < 30; i++) {
          got = (await entry()).cropMarks ?? [];
          if (got.some((m) => Math.abs(m.at - want) < 0.5 / src.fps)) break;
          await page.waitForTimeout(100);
        }
        await settle(page);
        const left = await centre();
        const out = arg === "end" ? Math.max(...held) - left : left - Math.min(...held);
        if (!got.some((m) => Math.abs(m.at - want) < 0.5 / src.fps))
          wrong = `the keyframe dragged past the clip's ${arg} is at ${got.map((m) => m.at).join(", ")}, not on its ${arg === "end" ? "last" : "first"} frame at ${want.toFixed(3)} s`;
        else if (out > 0.25)
          wrong = `the keyframe stood ${out.toFixed(2)} px past where it stays while it was held past the clip's ${arg}, and jumped back when let go`;
        else if (Math.abs(held[held.length - 1] - left) > 0.25)
          wrong = `the keyframe was held at ${held[held.length - 1].toFixed(2)} px and stays at ${left.toFixed(2)} px once let go`;
        await watch.step("drag", s);
        break;
      }
      case "drag crop": {
        const box = await page.locator(".screen .crop").boundingBox();
        if (!box) {
          wrong = "the video preview shows no crop frame to drag";
          break;
        }
        const y = box.y + box.height / 2;
        const x = box.x + box.width / 2;
        await page.mouse.move(x, y);
        await page.mouse.down();
        for (let i = 1; i <= 10; i++) await page.mouse.move(x + (arg * i) / 10, y);
        await page.mouse.up();
        await watch.step("drag", s);
        await settle(page);
        break;
      }
      case "keyframes": {
        const on = await chosen(page);
        const marks = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.cropMarks ?? [];
        let got = [];
        for (let i = 0; i < 50; i++) {
          got = await marks();
          if (got.length === arg && new Set(got.map((m) => m.x)).size === arg) break;
          await page.waitForTimeout(100);
        }
        const dots = await page.evaluate(() => document.querySelectorAll(".clip-timeline .cropdot").length);
        // Drawn as a diamond, the shape editors draw a keyframe in, and
        // the keyframe button's own: Lucide's diamond, filled.
        const shapes = await page.evaluate(() =>
          [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => getComputedStyle(e, "::after").maskImage),
        );
        const sizes = await page.evaluate(() => [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => e.getBoundingClientRect().width));
        if (got.length !== arg) wrong = `the engine has ${got.length} keyframes, not ${arg}`;
        else if (new Set(got.map((m) => m.x)).size !== arg) wrong = `the keyframes stand at ${got.map((m) => m.x).join(", ")}, not each at its own place`;
        else if (dots !== arg) wrong = `the clip timeline draws ${dots} keyframes, not ${arg}`;
        else if (shapes.some((c) => !c.includes("M2.7 10.3a2.41 2.41 0 0 0 0 3.41l7.59 7.59")))
          wrong = `a keyframe is not drawn as a diamond: ${shapes.join(", ")}`;
        else if (sizes.some((w) => w !== 16)) wrong = `a keyframe's diamond is ${sizes.join(", ")} px wide, not 16 px, found too small by Tim on #192`;
        break;
      }
      case "glides":
      case "glides while playing": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const src = await ask(page, "Source", on.path);
        const piece = entry?.segments.find((p) => (p.marks ?? []).length >= 2);
        if (!piece) {
          wrong = "no piece of the clip has two keyframes to glide between";
          break;
        }
        const glide = glideOf(piece.marks, src);
        const [a, b] = piece.marks;
        const crop = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.crop));
        // The episode is a filmed one, at a camera's rate, whose frame
        // number is read off the canvas from its bands.
        await filmedOnScreen(page);
        if (what === "glides") {
          const problems = [];
          for (const share of [0, 0.25, 0.5, 0.75, 1]) {
            // Half a frame in, so the click lands in the frame meant.
            const at = a.at + share * (b.at - a.at) + (share < 1 ? 0.5 / src.fps : 0);
            const g = await handles(page);
            await page.mouse.click(await xOf(page, at), high(g.track));
            await settle(page);
            const came = await page
              .waitForFunction(
                (fps) => {
                  const at = Number(document.querySelector(".screen").dataset.playhead);
                  // The frame that holds the playhead, see frameAt in lib/flow.ts.
                  return window.__filmed() === Math.floor((at + 0.001) * fps);
                },
                src.fps,
                { polling: 50, timeout: 5000 },
              )
              .then(() => true, () => false);
            if (!came) {
              problems.push(`no frame came for ${at.toFixed(3)} s`);
              continue;
            }
            const k = await filmedOnScreen(page);
            const got = await crop();
            if (got !== glide(k)) problems.push(`on frame ${k} the crop frame stands at ${got}, the glide puts it at ${glide(k)}`);
          }
          const ends = [glide(Math.round(a.at * src.fps)), glide(Math.round(b.at * src.fps))];
          if (ends[0] === ends[1]) problems.push(`the two keyframes stand at ${ends[0]} both, so nothing glides`);
          if (problems.length) wrong = problems.join("; ");
          break;
        }
        const g = await handles(page);
        // Two fifths of the way, where smoothstep moves the crop fastest.
        await page.mouse.click(await xOf(page, a.at + 0.4 * (b.at - a.at) + 0.5 / src.fps), high(g.track));
        await settle(page);
        await page.evaluate(() => {
          window.__glide = [];
          const read = () => {
            const el = document.querySelector(".screen");
            window.__glide.push({ k: window.__filmed(), crop: Number(el.dataset.crop) });
            if (window.__glide.length < 2000) window.__glideRaf = requestAnimationFrame(read);
          };
          window.__glideRaf = requestAnimationFrame(read);
        });
        await page.keyboard.press(" ");
        await page.waitForTimeout(2000);
        await page.keyboard.press(" ");
        const seen = await page.evaluate(() => {
          cancelAnimationFrame(window.__glideRaf);
          return window.__glide;
        });
        await settle(page);
        const off = seen.filter((r) => r.k >= 0 && r.crop !== glide(r.k));
        const places = new Set(seen.filter((r) => r.k >= 0).map((r) => r.crop));
        if (off.length) {
          wrong = `while playing, ${off.length} of ${seen.length} frames had the crop frame off the glide, the first frame ${off[0].k} at ${off[0].crop} for ${glide(off[0].k)}`;
        } else if (places.size < 10) {
          wrong = `while playing, the crop frame stood in ${places.size} places, not following the glide`;
        }
        break;
      }
      case "turned": {
        const on = await chosen(page);
        const pieces = async () => (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip)?.segments ?? [];
        let got = [];
        for (let i = 0; i < 50; i++) {
          got = (await pieces()).map((p) => p.rotate ?? 0);
          if (JSON.stringify(got) === JSON.stringify(arg)) break;
          await page.waitForTimeout(100);
        }
        if (JSON.stringify(got) !== JSON.stringify(arg)) {
          wrong = `the engine has the pieces rotated ${got.join(" ")}, not ${arg.join(" ")}`;
          break;
        }
        const src = await ask(page, "Source", on.path);
        const problems = [];
        for (const [i, p] of (await pieces()).entries()) {
          const g = await handles(page);
          await page.mouse.click(await xOf(page, (p.start + p.end) / 2), high(g.track));
          await settle(page);
          await page
            .waitForFunction((want) => Number(document.querySelector(".screen").dataset.turn) === want, arg[i], { timeout: 2000 })
            .catch(() => {});
          const seen = await page.evaluate(() => {
            const el = document.querySelector(".screen");
            const b = el.getBoundingClientRect();
            // How bright the picture is along its top and its bottom, where
            // a frame squeezed into the wrong shape leaves black.
            const c = el.querySelector("canvas");
            const s = document.createElement("canvas");
            s.width = Math.max(1, Math.floor(c.width / 2));
            s.height = 1;
            const t = s.getContext("2d", { willReadFrequently: true });
            const row = (y) => {
              t.drawImage(c, Math.floor(c.width / 4), y, s.width, 1, 0, 0, s.width, 1);
              const d = t.getImageData(0, 0, s.width, 1).data;
              let sum = 0;
              for (let k = 0; k < d.length; k += 4) sum += d[k];
              return sum / (d.length / 4);
            };
            return {
              turn: Number(el.dataset.turn),
              wide: b.width > b.height,
              edges: [row(Math.floor(c.height * 0.03)), row(Math.floor(c.height * 0.97))],
            };
          });
          const wide = arg[i] % 180 === 0 ? src.width > src.height : src.height > src.width;
          if (seen.turn !== arg[i]) problems.push(`piece ${i + 1} shows rotated ${seen.turn}, not ${arg[i]}`);
          if (seen.wide !== wide) problems.push(`piece ${i + 1} shows a ${seen.wide ? "wide" : "tall"} picture, not a ${wide ? "wide" : "tall"} one`);
          if (Math.min(...seen.edges) < 15) {
            problems.push(`piece ${i + 1} shows black along the top or the bottom of its picture, ${seen.edges.map((e) => e.toFixed(0)).join(" and ")}, so its frame is squeezed`);
          }
        }
        const all = await pieces();
        // A mark where a rotated stretch starts: at the start of a part,
        // or inside one where a cut put back joined two turned apart.
        const parts = all.filter((p, i) => arg[i] !== 0 && (i === 0 || p.start - all[i - 1].end > 0.0005 || arg[i - 1] !== arg[i])).length;
        const marks = await page.evaluate(() => document.querySelectorAll(".clip-timeline .turnmark").length);
        if (marks !== parts) problems.push(`the clip timeline marks ${marks} rotated parts, not ${parts}`);
        if (problems.length) wrong = problems.join("\n");
        break;
      }
      case "scrub": {
        const [from, to] = arg;
        const g = await handles(page);
        const left = g.start.x + g.start.w;
        const span = g.end.x - left;
        const y = high(g.track);
        await page.evaluate(
          ([bits, band]) => {
            // The frame number of a filmed video standing up by its display
            // matrix: its bands run down the right side, the highest bit
            // furthest right, see makeTurnedEpisode in bridge_test.go.
            const c = document.querySelector(".screen canvas");
            const s = document.createElement("canvas");
            const t = s.getContext("2d", { willReadFrequently: true });
            const read = () => {
              if (!c || !c.width || !c.height) return -1;
              const h = Math.max(1, Math.floor(c.height / 2));
              s.width = 1;
              s.height = h;
              let n = 0;
              for (let b = 0; b < bits; b++) {
                const x = Math.floor(c.width * (1 - ((b + 0.5) * band) / 180));
                t.drawImage(c, x, Math.floor(c.height / 4), 1, h, 0, 0, 1, h);
                const col = t.getImageData(0, 0, 1, h).data;
                let sum = 0;
                for (let i = 0; i < col.length; i += 4) sum += col[i];
                n = n * 2 + (sum / (col.length / 4) > 110 ? 1 : 0);
              }
              return n;
            };
            window.__scrubbed = [];
            window.__scrubbing = true;
            const tick = () => {
              if (!window.__scrubbing) return;
              const el = document.querySelector(".screen");
              window.__scrubbed.push([Number(el.dataset.turn), read(), Number(el.dataset.playhead)]);
              requestAnimationFrame(tick);
            };
            requestAnimationFrame(tick);
          },
          [11, 4],
        );
        // Over the cut and back, quickly, the way a hand looks for a frame,
        // so the playhead crosses it more often than its frames keep up.
        const path = [from, 0.42, 0.62, 0.42, 0.62, 0.42, 0.62, to];
        await page.mouse.move(left + from * span, y);
        await page.mouse.down();
        for (let leg = 1; leg < path.length; leg++) {
          const [a, b] = [path[leg - 1], path[leg]];
          for (let i = 1; i <= 12; i++) {
            await page.mouse.move(left + (a + ((b - a) * i) / 12) * span, y);
            await page.waitForTimeout(16);
          }
        }
        await page.mouse.up();
        await page.waitForTimeout(1000);
        await page.evaluate(() => (window.__scrubbing = false));
        await watch.step("scrub", s);
        break;
      }
      case "turned with its frames": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const { fps } = await ask(page, "Source", on.path);
        const seen = await page.evaluate(() => window.__scrubbed ?? []);
        // The turn the short gives frame n: its piece's, as the Go side
        // reads it, and for a frame of a cut, which no short holds, the
        // stretch of the clip's rotations its start lies in.
        const turnOf = (n) => {
          const at = n / fps;
          const p = entry.segments.find((q) => at >= q.start - 0.001 && at < q.end - 0.001);
          if (p) return p.rotate ?? 0;
          const r = (entry.rotations ?? []).find((q) => at >= (q.from ?? -Infinity) - 0.001 && at < (q.to ?? Infinity) - 0.001);
          return r ? r.deg : 0;
        };
        const read = seen.filter(([, n]) => n >= 0 && n < 30 * fps);
        const bad = read.filter(([turn, n]) => turn !== turnOf(n));
        const rotated = read.filter(([, n]) => turnOf(n) !== 0).length;
        const said = [];
        if (read.length < 0.8 * seen.length || !seen.length) {
          said.push(`only ${read.length} of ${seen.length} frames on the canvas could be read`);
        } else if (!rotated || rotated === read.length) {
          said.push(`the playhead showed ${rotated} frames of the rotated part out of ${read.length}, not both kinds`);
        }
        if (bad.length) {
          said.push(`${bad.length} of ${read.length} animation frames were turned wrong:`);
          for (const [turn, n, at] of bad.slice(0, 6)) {
            said.push(`frame ${n}, of a piece rotated ${turnOf(n)}, was turned ${turn} with the playhead at ${at.toFixed(3)}`);
          }
        }
        if (said.length) wrong = said.join("\n");
        break;
      }
      case "upright": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const { fps } = await ask(page, "Source", on.path);
        const short = await shortOf(page, on);
        if (!short || !existsSync(short)) {
          wrong = `the clip has no short${short ? ` at ${short}` : ""}`;
          break;
        }
        const { frames, modulo } = shortFrames(short);
        const want = [];
        for (const p of entry.segments) {
          const first = Math.round(p.start * fps);
          const end = Math.max(Math.round(p.end * fps), first + 1);
          for (let k = first; k < end; k++) want.push({ k, rotate: p.rotate ?? 0 });
        }
        if (frames.length !== want.length) {
          wrong = `the short has ${frames.length} frames, its pieces hold ${want.length}`;
          break;
        }
        const read = { rotated: [0, 0], lying: [0, 0] };
        want.forEach(({ k, rotate }, i) => {
          const r = read[rotate % 180 ? "rotated" : "lying"];
          r[1]++;
          if (frames[i] === k % modulo) r[0]++;
        });
        if (!read.rotated[1]) wrong = "the clip has no piece rotated a quarter";
        else if (read.rotated[0] !== read.rotated[1]) {
          wrong = `${read.rotated[1] - read.rotated[0]} of the ${read.rotated[1]} frames rotated a quarter are not the right frame read across their top`;
        } else if (read.lying[0] * 2 > read.lying[1]) {
          wrong = `${read.lying[0]} of the ${read.lying[1]} frames not rotated read across their top, so the video did not lie on its side`;
        }
        break;
      }
      case "short": {
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const { fps } = await ask(page, "Source", on.path);
        const short = await shortOf(page, on);
        wrong = shortProblem(entry, short, fps, arg);
        break;
      }
      default:
        throw new Error(`no step called ${what}`);
    }
  }
  const failed = wrong ?? (watch.failed && `broke: ${watch.failed.rule}\n${watch.failed.detail}`);
  const took = ((performance.now() - began) / 1000).toFixed(1).padStart(5);
  console.log(`${failed ? "FAIL" : "ok  "}  ${took} s  ${seq.name}`);
  if (failed) {
    failures++;
    console.log(`      after ${done.join(", ")}\n      ${failed.replaceAll("\n", "\n      ")}`);
  }
  await browser.close();
}
process.exit(failures ? 1 : 0);

// The left edge of the crop on each frame of the video, gliding over a
// piece's keyframes by smoothstep, the way the engine's glideX and
// lib/glide.ts work it out, see glide.cases.json, for frame numbers
// counted from the picture's first frame.
function glideOf(marks, src) {
  const v0 = src.videoStart ?? 0;
  const most = src.width - src.cropWidth;
  const on = marks.map((m) => {
    const x = Math.max(0, Math.min(m.x, most));
    return [Math.floor((m.at - v0) * src.fps + 0.5), x - (x % 2)];
  });
  return (k) => {
    if (k <= on[0][0]) return on[0][1];
    for (let i = 1; i < on.length; i++) {
      const [kb, xb] = on[i];
      if (k >= kb) continue;
      const [ka, xa] = on[i - 1];
      const u = (k - ka) / (kb - ka);
      return xa + (xb - xa) * (u * u * (3 - 2 * u));
    }
    return on[on.length - 1][1];
  };
}

// Holds files over the app and drops them, the way the runtime reports a
// drag from Finder, its start and where it is, and a drop, see wails-bridge.ts, somewhere left of the
// middle and below it, over whatever the page has there. Says what was
// wrong, or null: the app has to wear its frame while the files are held
// over it, take the drop, and wear no frame after it.
async function dropOn(page, files) {
  const { width, height } = page.viewportSize();
  const [x, y] = [Math.round(width * 0.4), Math.round(height * 0.6)];
  const landing = () => page.evaluate(() => {
      const frame = document.querySelector(".landing");
      return frame ? getComputedStyle(frame).opacity : "nothing, there is no frame";
    });
  await page.evaluate(([x, y]) => {
    window._wails.handleDragEnter();
    window._wails.handleDragOver(x, y);
  }, [x, y]);
  await page.waitForTimeout(300);
  const held = await landing();
  if (held !== "1") return `the app wore no frame while a file was held over it, its opacity was ${held}`;
  const took = await page.evaluate(([f, x, y]) => window._wails.handlePlatformFileDrop(f, x, y), [files, x, y]);
  if (!took) return `the drop at ${x}, ${y} landed on no drop target`;
  await page.waitForTimeout(300);
  const after = await landing();
  if (after !== "0") return `the app still wore its frame after the drop, its opacity was ${after}`;
  return null;
}

// What is wrong with how the crop frame is drawn over the video preview,
// or null, read off a screenshot. The picture under it is made white for
// the look, so the shade over it is a grey of its own and a speck of the
// picture it missed is white, and a note over the picture, like the one
// the bridge's sound gives, is put away: the frame, the shade and the
// viewer are drawn as they are. See ["frame whole"].
async function frameWhole(page, turned) {
  const at = await page.evaluate(() => {
    const box = (e) => e?.getBoundingClientRect().toJSON();
    const frame = document.querySelector(".player .frame");
    if (!frame) return null;
    const line = getComputedStyle(frame);
    const crop = document.querySelector(".screen .crop") ?? frame;
    return {
      viewer: box(document.querySelector(".viewer")),
      screen: box(document.querySelector(".screen")),
      frame: box(frame),
      crop: box(crop),
      accent: line.borderTopColor.match(/\d+/g).slice(0, 3).map(Number),
      line: parseFloat(line.borderTopWidth),
      radius: parseFloat(line.borderTopLeftRadius),
      dpr: devicePixelRatio,
    };
  });
  if (!at) return "the video preview has no crop frame";
  const { viewer, screen, frame, crop } = at;
  const near = (a, b) => Math.abs(a - b) < 0.5;
  if (!near(frame.left, crop.left) || !near(frame.width, crop.width) || !near(frame.top, crop.top) || !near(frame.height, crop.height)) {
    return `the crop frame's line stands at ${frame.left.toFixed(1)} to ${frame.right.toFixed(1)}, the crop the hand moves at ${crop.left.toFixed(1)} to ${crop.right.toFixed(1)}`;
  }
  // A crop frame away from the viewer's corners has nothing there to cut
  // it, and the case would pass without being the case. A part rotated a
  // quarter is narrower than the viewer, and is there for the lid, which
  // has to turn with the picture.
  const corners = [screen.left - viewer.left, viewer.right - screen.right, screen.top - viewer.top, viewer.bottom - screen.bottom];
  if (turned ? corners[0] < 8 : corners.some((d) => d > 1)) {
    return `the picture stands ${corners.map((d) => d.toFixed(1)).join(", ")} px in from the viewer's sides, so its corners are not the viewer's`;
  }
  const look = await page.addStyleTag({ content: ".screen canvas, .screen .trouble, .screen .failed { visibility: hidden !important } .screen { background: #fff !important }" });
  await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
  const clip = { x: viewer.left - 4, y: viewer.top - 4, width: viewer.width + 8, height: viewer.height + 8 };
  const shot = (await page.screenshot({ clip })).toString("base64");
  await look.evaluate((e) => e.remove());
  return page.evaluate(
    async ({ shot, at, clip }) => {
      const img = new Image();
      img.src = `data:image/png;base64,${shot}`;
      await img.decode();
      const c = document.createElement("canvas");
      c.width = img.naturalWidth;
      c.height = img.naturalHeight;
      const g = c.getContext("2d", { willReadFrequently: true });
      g.drawImage(img, 0, 0);
      const data = g.getImageData(0, 0, c.width, c.height).data;
      const d = at.dpr;
      const px = (x, y) => {
        const i = (Math.floor(y) * c.width + Math.floor(x)) * 4;
        return [data[i], data[i + 1], data[i + 2]];
      };
      const say = (p) => `rgb(${p.join(", ")})`;
      const light = (p) => Math.max(...p);
      const accent = (p) => p.every((v, k) => Math.abs(v - at.accent[k]) <= 48);
      // The frame in the screenshot's own pixels.
      const l = (at.frame.left - clip.x) * d;
      const t = (at.frame.top - clip.y) * d;
      const r = (at.frame.right - clip.x) * d;
      const b = (at.frame.bottom - clip.y) * d;
      const w = at.line * d;
      const round = at.radius * d;
      const problems = [];

      // The line, its outermost row of pixels and its innermost, from
      // where one rounded corner ends to where the next begins.
      const sides = [
        ["left", (k, e) => [l + e, k], t, b],
        ["right", (k, e) => [r - e, k], t, b],
        ["top", (k, e) => [k, t + e], l, r],
        ["bottom", (k, e) => [k, b - e], l, r],
      ];
      for (const [side, point, from, to] of sides) {
        const off = [];
        for (let k = Math.ceil(from + round + 1); k < to - round - 1; k++) {
          for (const e of [0.5, w - 0.5]) {
            const p = px(...point(k + 0.5, e));
            if (!accent(p)) off.push({ k: k - from, p });
          }
        }
        if (off.length) {
          problems.push(
            `the crop frame's line is not the app's colour on its ${side} side for ${off.length} pixels, the first ${(off[0].k / d).toFixed(0)} px along it, which is ${say(off[0].p)}`,
          );
        }
      }

      // Each rounded corner, outside its curve: the shade, never the white
      // picture. A pixel the curve passes through is the line's and the
      // shade's both, so only those whose middle is outside it count.
      const corners = [
        ["top left", l, t, 1, 1],
        ["top right", r, t, -1, 1],
        ["bottom left", l, b, 1, -1],
        ["bottom right", r, b, -1, -1],
      ];
      for (const [name, x0, y0, sx, sy] of corners) {
        const cx = x0 + sx * round;
        const cy = y0 + sy * round;
        let worst = null;
        for (let i = 0; i < round; i++) {
          for (let j = 0; j < round; j++) {
            const x = x0 + sx * (i + 0.5);
            const y = y0 + sy * (j + 0.5);
            if (Math.hypot(x - cx, y - cy) < round + 0.25) continue;
            const p = px(x, y);
            if (light(p) > 170 && (!worst || light(p) > light(worst))) worst = p;
          }
        }
        if (worst) problems.push(`the crop frame's ${name} corner has the picture bright outside its curve, ${say(worst)}, where the shade should be`);
      }

      // Inside the frame the picture as it is, beside it under the shade.
      const inside = px((l + r) / 2, t + w + 4 * d);
      if (light(inside) < 230) problems.push(`inside the crop frame the white picture is ${say(inside)}, shaded`);
      // A crop as wide as the picture, on a rotated part, has nothing
      // beside it.
      const right = at.screen.right - at.frame.right;
      const left = at.frame.left - at.screen.left;
      const beside = px(right > left ? r + 3 * d : l - 3 * d, (t + b) / 2);
      if (Math.max(left, right) > 4 && !(light(beside) > 60 && light(beside) < 170)) {
        problems.push(`beside the crop frame the white picture is ${say(beside)}, not shaded`);
      }
      return problems.length ? problems.join("\n") : null;
    },
    { shot, at, clip },
  );
}

// What is wrong with where the frames lie on the two tracks, or null, read
// off a screenshot of each: the chosen clip's frame on the clip timeline
// and the bars of the window at rest on the range picker. Each lies on its
// track's top and bottom border, so the outermost rows of the track's box
// are the app's colour as deep as the line is thick, and the row just
// outside the box is not. Columns near the playhead and the marks standing
// on a line are left out, and so are the frame's rounded ends. See
// ["on borders"].
async function onBorders(page) {
  const at = await page.evaluate(() => {
    const box = (e) => e?.getBoundingClientRect().toJSON();
    const span = document.querySelector(".clip-timeline .span");
    const aim = document.querySelector(".aim");
    const bar = aim?.querySelector(".bar.top");
    // The app's colour and its lighter shade, the frame under the hand,
    // as numbers, whether the browser says them as rgb() or color(srgb).
    const colour = (css) => {
      const e = document.createElement("i");
      e.style.color = css;
      document.body.append(e);
      const said = getComputedStyle(e).color;
      e.remove();
      const n = said.match(/[\d.]+/g).map(Number);
      return said.startsWith("color(") ? n.slice(0, 3).map((v) => Math.round(v * 255)) : n.slice(0, 3);
    };
    const away = [...document.querySelectorAll(".playhead, .head, .cropdot, .thumb, .tip")].map((e) => box(e));
    return {
      dpr: devicePixelRatio,
      accent: [colour("var(--accent)"), colour("var(--accent-hi)")],
      clip: span && { track: box(document.querySelector(".clip-timeline .track")), from: box(span).left, to: box(span).right, deep: parseFloat(getComputedStyle(span).borderTopWidth) },
      range: aim && { track: box(aim.parentElement.querySelector(".track")), from: box(aim).left, to: box(aim).right, deep: parseFloat(getComputedStyle(bar, null).backgroundSize.split(" ")[1]) },
      away,
    };
  });
  if (!at.clip) return "the clip timeline shows no frame for the chosen clip";
  if (!at.range) return "the range picker shows no window at rest";
  const problems = [];
  for (const [name, t] of [["the clip's frame on the clip timeline", at.clip], ["the window's bars on the range picker", at.range]]) {
    const clip = { x: Math.floor(t.track.left) - 2, y: Math.floor(t.track.top) - 4, width: Math.ceil(t.track.width) + 4, height: Math.ceil(t.track.height) + 8 };
    const shot = (await page.screenshot({ clip })).toString("base64");
    const found = await page.evaluate(
      async ({ shot, at, t, clip, name }) => {
        const img = new Image();
        img.src = `data:image/png;base64,${shot}`;
        await img.decode();
        const c = document.createElement("canvas");
        c.width = img.naturalWidth;
        c.height = img.naturalHeight;
        const g = c.getContext("2d", { willReadFrequently: true });
        g.drawImage(img, 0, 0);
        const data = g.getImageData(0, 0, c.width, c.height).data;
        const d = at.dpr;
        // The pixel in the middle of a CSS pixel of the page.
        const px = (x, y) => {
          const i = (Math.floor((y - clip.y + 0.5) * d) * c.width + Math.floor((x - clip.x + 0.5) * d)) * 4;
          return [data[i], data[i + 1], data[i + 2]];
        };
        const accent = (p) => at.accent.some((a) => p.every((v, k) => Math.abs(v - a[k]) <= 24));
        const say = (p) => `rgb(${p.join(", ")})`;
        const near = (x) => at.away.some((b) => b && x > b.left - 4 && x < b.right + 4);
        const problems = [];
        let looked = 0;
        for (let x = Math.ceil(t.from) + 12; x < Math.floor(t.to) - 12; x += 3) {
          if (near(x)) continue;
          looked++;
          for (const [side, edge, inward] of [["top", t.track.top, 1], ["bottom", t.track.bottom - 1, -1]]) {
            for (let k = 0; k < t.deep; k++) {
              const p = px(x, edge + inward * k);
              if (!accent(p)) {
                problems.push(`${name} does not cover the ${side} border at ${(x - t.from).toFixed(0)} px along it: ${k} px in from the track's edge is ${say(p)}`);
                return problems;
              }
            }
            const out = px(x, edge - inward);
            if (accent(out)) {
              problems.push(`${name} stands outside the ${side} border at ${(x - t.from).toFixed(0)} px along it: the row just past the track's edge is the app's colour`);
              return problems;
            }
          }
        }
        if (looked < 5) problems.push(`${name} had only ${looked} places to look at away from the playhead and the marks`);
        return problems;
      },
      { shot, at, t, clip, name },
    );
    problems.push(...found);
  }
  return problems.length ? problems.join("; ") : null;
}
