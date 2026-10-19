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
//   ["open in place", word]
//                      clicks the word and leaves it with Escape, and sees
//                      no word of the caption box move, libass draw the
//                      caption without that word's letters while it is
//                      open and with the rest where they were, its letters
//                      on the rows libass drew them on, and nothing pop
//                      when it is left
//   ["grow", word]     walks the playhead with Shift and → until the word
//                      is lit and a frame on with →, into its pop, clicks
//                      the word, types after it and leaves it with Escape,
//                      and sees it open where it stood, at the scale it
//                      was drawn at, and its box and pill grow round their
//                      middle by what was typed
//   ["block", word]    clicks the block on the clip timeline of the caption
//                      the engine has the word in, which picks it
//   ["trim into", s]   drags the clip's start to s seconds before the end of
//                      the first word of its first caption
//   ["every block"]    clicks every caption block on the clip timeline in
//                      turn, and each lights a word of its caption
//   ["gone", word]     no caption the engine has holds the word
//   ["holds", word]    a caption the engine has holds the word
//   ["clips with", [word, n]]
//                      n clips of the episode on screen, of those not
//                      removed, have a caption with this very word
//   ["fewer", label, n] the engine's captions hold n words fewer than at
//                      the mark, and as many captions
//   ["blocks"]         the clip timeline draws a block for every caption the
//                      engine has, and for no other
//   ["press", key]     presses a key, as Playwright names it
//   ["read ahead", key] rests a second and a half, presses the key and
//                      checks that the frame it lands on was there already,
//                      read while the hand rested: it came from the frames
//                      kept, see AppFrames.ahead
//   ["heard"]          rests a second and a half, plays with the space bar
//                      and asks that the sound card was handed sound for
//                      most of the next second and a half, then pauses
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
//   ["drag edge", which, px]
//                      drags the clip's "start" or "end" edge, or the
//                      "cut from" or "cut to" edge of its first cut, by px
//                      in twenty steps while it is paused, reading the
//                      playhead on every animation frame from the press to
//                      after it lets go
//   ["drag edge", which, px, "shift"]
//                      the same with shift held, so the edge lands on words
//   ["drag edge", which, px, "at the playhead"]
//                      the same, pressed where the playhead's grip and the
//                      edge's handle overlap rather than in its middle
//   ["hold on", which] presses the track near the end away from it, then the
//                      clip's "start" or "end" edge, the "cut from" or "cut
//                      to" edge of its first cut, its first "thumbnail" or
//                      "crop" mark, or the "caption start" or "caption end"
//                      edge of its second caption, reading the playhead on
//                      every animation frame while the button is down, and
//                      lets go
//   ["drag edge twice", which, px, again, late]
//                      drags an edge by px, lets go, and at once, with
//                      nothing waited for, drags it again by again. With
//                      late, the last place of the first drag is a frame
//                      further on and answered so many milliseconds late,
//                      so the edge is taken again before that answer is in
//   ["edge then track", which]
//                      clicks an edge and at once presses the track near
//                      its right end: the playhead stands where the track
//                      was pressed
//   ["cut on caption", n]
//                      double-clicks the middle of the caption block n,
//                      from 0
//   ["block after", n] clicks the caption block of the first caption that
//                      appears after the clip's nth cut, from 1, which puts
//                      the playhead on its first word
//   ["drag while playing", edge, px]
//                      plays the clip from a moment into it, and drags its
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
//   ["swipe after click", share, px]
//                      clicks the clip timeline this share of the way
//                      along the clip, which puts the playhead under the
//                      hand, and then swipes with two fingers from there
//                      three times by px without moving the hand, later in
//                      the episode for more than nought. The clip has to
//                      move along the clip timeline the other way on every
//                      swipe
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
//   ["move window", handle, share]
//                      drags the window's mark on the range picker, its
//                      "to" or "from" triangle or its "bar", by this share
//                      of the track's width, and remembers the episode's
//                      last search and where the window was left
//   ["searched moved"] the episode's newest search is a new one, not the
//                      search there was at "move window", and it searched
//                      the window where "move window" left it
//   ["head"]           presses the clip list's head button, Search, Cancel or
//                      Continue
//   ["hold", call, ms] the next call of this name waits so many
//                      milliseconds before it reaches the Go side, the way
//                      a busy machine delivers it late
//   ["search and cancel"] presses Search, and Cancel in the first frame the head
//                      says it, and every frame from Search to that press the
//                      head says Cancel and takes the click, never greyed
//                      out
//   ["restart"]        closes the app and opens it again on the episode
//   ["speech", ms]     the speech model takes so many milliseconds over
//                      each piece of audio, so a video added is heard
//                      slowly enough to do something while it is
//   ["settings"]       opens the settings from the sidebar, and remembers
//                      the models they name
//   ["app log", "open"]
//                      the app starts its log, with an older file beside
//                      the one in use, see /app-log in bridge_test.go
//   ["app log", n]     Settings, Advanced, opened if it is not, says the
//                      app's log has n files, and the Go side has as many
//   ["bugs, the pointer gone"]
//                      clicks Bugs with the pointer on the sidebar, and
//                      the app's window loses the focus to the report
//                      window with no word of the pointer leaving, as on
//                      a Mac. The sidebar closes then, stays closed when
//                      the focus comes back to the button, and the next
//                      move of the pointer outside leaves it closed
//   ["bugs"]           clicks Bugs in the sidebar, which has the app open
//                      the report window or bring it forward, and opens
//                      the report window as a second page if none is open,
//                      where the steps after it act. See bugs.go
//   ["windows", made, fronted]
//                      the app has made so many report windows and
//                      brought one forward so many times, and Bugs is lit
//                      while one is open
//   ["close report"]   closes the report window, the way Cmd+W does, and
//                      goes back to the app's window
//   ["attach", n]      presses the paperclip with n pictures waiting in
//                      the open box
//   ["drop pictures", n, other]
//                      drops n pictures on the report window, and a file of
//                      text named other with them when there is one
//   ["paste picture"]  pastes a picture into the report's field
//   ["drop recording"] drops a screen recording of three seconds on the
//                      report window, a MOV the way a Mac makes one
//   ["recording tile", length]
//                      the last tile is a recording, its still loaded and
//                      its length written in its corner as length
//   ["pictures", n]    the report window shows n pictures
//   ["toast", text]    a toast over the window says text, and with "" no
//                      toast is there. Nothing under it moved since the
//                      step before it, and the line beside the report's
//                      buttons says nothing (#327)
//   ["toast goes"]     with the pointer away, the toast is still there
//                      after nine seconds and gone by itself within fourteen
//   ["toast held"]     the pointer stays on the toast longer than its time
//                      and it stays, then the pointer goes and so does the
//                      toast, within fourteen seconds
//   ["toast closed"]   presses the cross on the top toast, and it goes
//   ["remove picture", i]
//                      points at picture i, from 1, sees its bin and no
//                      other, and presses it. The picture keeps its place
//                      and says Removed, its time standing full while the
//                      pointer is on it, through the made-up leave and
//                      over a browser may send while the node under a
//                      still pointer goes. [i, "key"] presses Delete on
//                      the tile instead, the pointer standing on it.
//                      [i, "tile"] presses the bin, and the browser says
//                      the pointer is over the tile round the picture and
//                      never over the picture itself, the way WebKit may
//   ["put back picture", i]
//                      presses Put it back on picture i, which is back in
//                      the same frame
//   ["removed goes"]   the pointer leaves, and the time to put the picture
//                      back runs out and the grid closes over it
//   ["report", n]      presses Report, which closes the report window and
//                      says nothing beside the buttons, and reads the zip
//                      it made: what was written and n pictures, or with
//                      [n, m] n pictures and m recordings, and the field
//                      empty after
//   ["pictures fit"]   every picture is drawn whole in its tile, in its
//                      middle, reaching both sides or top and bottom
//   ["report at end"]  the report's field is scrolled to its end
//   ["report scroll", n]
//                      turns the wheel over the report's field by n
//                      pixels, down for more than 0
//   ["report follows"] more is said, and the field follows it: the words
//                      being said are in view, at its end when they end
//                      the report
//   ["report stays"]   more is said, and the field stays where the last
//                      "report scroll" put it
//   ["caret at words"] the cursor is drawn right after the words being
//                      said, the field's own hidden
//   ["caret elsewhere"]
//                      clicks into the report's first line, and the field
//                      shows its own cursor there again
//   ["report lines up"]
//                      the microphone, the paperclip and Report are one
//                      height on a whole pixel, the two are square, the
//                      tiles are one size
//                      on whole pixels with a space between them, each
//                      with the sidebar's bin in its corner
//   ["buttons fit", words]
//                      opens Advanced where there is one, and every button
//                      on screen that holds words alone is as wide as the
//                      words it shows and its padding, to a pixel. A
//                      button made to fill its row, like a caption
//                      setting's name, is not about its words. Buttons
//                      saying each of words are among them (#362)
//   ["write", text, n] types text into the report's field and puts the
//                      cursor n characters in, or at its end for "end"
//   ["dictate", how]   presses Dictate, waits for the words to appear grey
//                      while they are heard, and ends the recording how
//                      says: "button" presses Stop, "enter" presses Enter
//                      and "escape" Escape, and "ends" presses nothing
//                      until it stops by itself and says why. Then waits
//                      for Dictate again.
//                      It hears two notes going up once the microphone is
//                      open, and two going down when it stops, however
//                      it stops.
//                      "start" only starts it, "hearing" waits for more
//                      to be heard, "settled" waits for words
//                      before a pause to be settled, and a recording
//                      already running is only ended
//   ["click heard"]    clicks into the first word still being heard and
//                      sees it settled there, with the cursor in it
//   ["field", selector, text]
//                      clicks the field selector names and types text
//   ["escape lets go", selector]
//                      clicks the control or field, presses Escape, and
//                      sees no list or pop-up open, the keyboard let go
//                      of and no ring on anything
//   ["field reads", selector, text]
//                      the field selector names holds text
//   ["report reads", text]
//                      the report's field reads text, where * is
//                      whatever the speech model heard, a word or more
//   ["report as before"]
//                      the report's field reads what it did at the last
//                      "report reads", or at the last "report grew" with
//                      "grown"
//   ["report grew"]    the report's field holds more than it did at the
//                      last "report reads", all of that and more words
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
//                      BT.709. A word lifted out and grown by 1.15 and 2
//                      is held to laidPart's on every pixel, so the GPU
//                      grows it round its middle. Says so and checks
//                      nothing without WebGPU
//   ["play to the end"]
//                      drags the clip's end to the end of the episode,
//                      puts the playhead three seconds before it with a
//                      click on the clip timeline and plays until the play
//                      stops there, so the episode's decoder reads to the
//                      end of the file
//   ["past the end"]
//                      drags the clip's end far to the right three times,
//                      each from where the end is drawn then, and checks
//                      after each that the clip ends where the video does
//                      at the latest, in the engine's plan
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
//   ["folder removed", when]
//                      deletes the folder beside the video on screen,
//                      <name>.framefairy, from outside the app, the way
//                      Finder does: "away" while the app is behind
//                      another, which then comes back to the front, and
//                      "closed" while the app is closed, which is then
//                      opened again, and "with the video" deletes the
//                      video too, while the app is behind another.
//                      "written back" deletes both while the app is
//                      closed and writes the chosen clip back into a new
//                      folder, the way the app did when it was asked
//                      about a video whose folder was gone, before 2.247
//   ["let go"]         the video whose folder was removed has left the
//                      sidebar and the library, its workspace is closed,
//                      nothing writes its folder back and the video file
//                      itself is still there, unless it was deleted too
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
//   ["put back"]       presses Put it back on the row of the clip removed
//                      last, and waits until the Go side has it back
//   ["put back another while playing"]
//                      plays the clip on screen with the space bar,
//                      removes another clip with its bin and puts it back,
//                      and sees the clip chosen stay chosen, the play go
//                      on, and the playhead never go back while it plays
//   ["remove the chosen", end]
//                      clicks the "first" or "last" clip in the list, which
//                      chooses it, removes it with its bin, and sees the
//                      clip beside it chosen: the next, or for the last
//                      the one before it
//   ["chosen again"]   the clip the last "remove the chosen" removed is
//                      chosen again
//   ["put back the chosen"]
//                      removes the clip chosen with its bin, paused, puts
//                      it back, and sees it chosen again with the
//                      playhead where it was
//   ["make here", key] presses "i" or "o", Mark In or Mark Out, with the
//                      playhead where it is, and waits until the clip made
//                      is written and chosen
//   ["asks none", name, seconds]
//                      the interface asks the Go side nothing of this name
//                      for so many seconds
//   ["swipe keeps the canvas", n]
//                      swipes along the clip timeline n times with two
//                      fingers, and the waveform's canvas is never made new
//                      on the way, only drawn again
//   ["stands still", away]
//                      holds the window up for 900 ms, after it was told the
//                      app went behind another when away is true, and reads
//                      whether the log was told the window stood still: it
//                      must be, unless the app was away
//   ["remove two, hand still"]
//                      presses the bins of the first two clips in the clip
//                      list, one after the other, and leaves the hand where
//                      it pressed the second while the first one's row runs
//                      out and closes, which moves the second row out from
//                      under the hand. The page hears nothing of the
//                      pointer meanwhile, the way WebKit tells it nothing
//                      of a row moving from under a hand that stands
//                      still, where Chromium makes up a move for it. Then
//                      the hand goes to the clip timeline, and the second
//                      row's time has to run down and close it
//   ["row folds"]      presses the bin of a clip with another clip's card
//                      under it, takes the hand to the clip timeline, and
//                      reads where the card under it stands on every
//                      animation frame while the removed row runs down and
//                      closes. The card has to rise by the row and the
//                      space after it, never go back down, and move no more
//                      than a pixel in the frame the row leaves the list
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
//   ["drop not a video", where] or ["drop not a video", [where, stem]]
//                      the same with a PDF, named stem if given
//   ["dropped in front"]
//                      the video dropped last is the one on screen, and in
//                      the library once
//   ["not a video"]    the PDF dropped last is not in the library, and a
//                      toast says by its name that it cannot be added
//                      because it is not a video, with the workspace where
//                      it stood before the drop
//   ["no reason"]      no toast names the video dropped last: a video
//                      that is added says nothing
//   ["stacked"]        the two PDFs dropped last each have a toast, the
//                      newer on top, the way macOS stacks its notes
//   ["left out", name] Add left out the file whose name starts so, says why
//                      in a toast, and the library does not hold it
//   ["followed", which]
//                      since "drag edge", the playhead went with the edge:
//                      on frames, the edge stood in two places or more,
//                      and on each the playhead came onto the edge's frame
//                      within three animation frames of the edge leaving
//                      it, and once the hand let go it stands on the
//                      frame the edge leaves on screen, the clip's or the
//                      piece's first frame for "start" and "cut to", its
//                      last for "end" and "cut from", with shift too
//   ["went the hand's way", which]
//                      since "drag edge", the edge never stood on the far
//                      side of where it was pressed from the way the hand
//                      went
//   ["moved", which]   since "drag edge", the edge moved by a frame or more
//   ["kept up", which] since "drag edge twice", the edge moved while the
//                      hand took it again and still held it, so it went
//                      with the hand while the drag before it saved, and
//                      the edge was saved where the hand left it last
//   ["on press", which]
//                      since "hold on", the playhead had moved while the
//                      button was still down, onto the frame "followed"
//                      names for an edge, the mark's frame, or the frame
//                      the caption appears on or the last it is up on, in
//                      no more animation frames than the press on the
//                      track took
//   ["pill pops", n]   the pill round the lit word in the video preview
//                      popped n times since "count pops"
//   ["played through"] since "drag while playing", the play went on as if
//                      no hand were there: the playhead never went back nor
//                      stood still for longer than 150 ms, and no frame
//                      drawn was older than the one before it
//   ["loop through the end", seconds]
//                      puts the playhead so many seconds before the clip's
//                      end, plays, switches the loop on with L at once and
//                      lets it run round the end and three seconds into
//                      the next time through, then asks that from half a
//                      second after it came round the video preview shows
//                      the frame of the playhead, give or take two
//   ["loop off as the sound crosses the end"]
//                      puts the playhead near the clip's end, plays it with
//                      the loop on, holds the page 250 ms as the playhead
//                      comes to the end and presses L in that moment, and
//                      asks that the sound had crossed the end, the loop is
//                      off and the play ends ahead of the sound card
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
//                      press and again where the hand moved to, the frame
//                      that holds it came and nothing was drawn after it,
//                      the button said Pause the whole time, and the play
//                      went on from where the hand let go
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
//   ["captions drawn"] once the layers have come, the picture shows the
//                      letters of a caption
//   ["drawn", word]    the video preview shows the caption holding the
//                      word where the playhead stands, letters drawn and the
//                      caption box holding the word
//   ["caption edge", [edge, word]]
//                      clicks the "start" or "end" edge of the caption
//                      holding the word on the clip timeline, without moving
//   ["one pill", word] the word opened while another is lit sits on the
//                      colour of the pill libass draws round it lit, with no
//                      ring of another shade round it and no focus ring
//   ["box", text]      the caption box reads this, word by word
//   ["open", word]     this word is open for typing
//   ["font", name]     picks this family in the captions column, and waits
//                      for the captions drawn in it
//   ["search font", "letters|name"]
//                      types the letters in the Font list's search field,
//                      finds the family first, picks it with Enter and
//                      waits for the captions drawn in it
//   ["search said", word]
//   ["search said", [word, n]]
//                      presses the transcript button in the head of the clip column
//                      and types the word, and sees the field take the
//                      head's place at its height with the keyboard in it,
//                      a card for every sentence the engine has that holds
//                      the word and for no sentence it does not have, the
//                      word bold in each, the clip a sentence is in named
//                      on its card, a dashed card and a dashed part of the
//                      range picker for every part not transcribed yet, at
//                      least n of them, sentences in the first frame after
//                      the press, a card's edge and its clip's mark lit
//                      under the hand, a part not transcribed lit on the
//                      range picker under the hand, a clip chosen there
//                      bringing its sentence into view, a click on a card
//                      putting the playhead on its sentence, and Escape,
//                      with the keyboard on that card, giving the head and
//                      the clips back. A word of "*" is the longest word
//                      of the first sentence
//   ["said fails", "on"|"check"]
//   ["font then transcript"]
//                      "on" makes every read of the cards of what was said
//                      fail. "check" opens the search and sees the list say
//                      the transcript could not be read, with the reason
//                      under the pointer, then lets the reads work, opens it
//                      again and sees the cards with nothing said of it
//   ["heard some"]     waits until the engine has a sentence of the video
//                      on screen, and for the piece of audio being heard
//                      then to land
//   ["font list"]      the Font list hangs on the field's left edge with
//                      its corners, every row is as tall as the field and
//                      square, nothing stands between the search field and
//                      the rows or under them, each family is drawn in its
//                      own face, ours by name first, the field is drawn
//                      in the face picked, the search field is a row tall
//                      above its line, the line over Installed sits on the
//                      last of ours, and every name drawn in its face has
//                      its capitals in the middle of its box
//   ["style list"]     the Style list of one style ends on the bottom of
//                      the field under the Style field
//   ["nothing found", letters]
//                      typing the letters in the Font list finds nothing,
//                      said in the middle of a list as wide as its field,
//                      or as the words where they need more
//   ["search order", letters]
//                      what typing the letters in the Font list finds is
//                      in the order of the whole list, under its own head
//   ["style is", name] the Style field says this
//   ["style", name]    picks this style of the family, and waits for the
//                      captions drawn in it
//   ["thickened as libass", word]
//                      the word opened is drawn at its face's own weight
//                      with no bold the browser fakes, and its letters
//                      grown by an em over 64 where libass thickens the
//                      face, see engine.Thickening
//   ["selection in the pill"]
//                      the word open, all its letters selected paint
//                      nothing outside its pill, above it or under it
//   ["no caret"]       no word of the caption box holds the keyboard or the
//                      selection, where WebKit would draw a caret
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
//   ["time field"]     the row under the clip timeline does not say the
//                      chosen clip's title, and its time field, as tall as
//                      every control, says where the frame that holds the
//                      playhead starts, to the millisecond: on every step
//                      of a drag along the clip timeline with the button
//                      held, standing as wide as it stood, and after each
//                      of three presses of →, one frame on each time
//   ["card says"]      the chosen clip's card says where the clip starts,
//                      how long it comes out and in how many pieces, the
//                      engine's numbers and as many pieces as the clip
//                      timeline draws
//   ["card follows", px]
//                      drags the clip's end by px with the button held,
//                      and the chosen clip's card says the clip's new
//                      length before the hand lets go
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
//                      playhead put on the frame of the clip's start and of
//                      where each cut ends by a click on it, the video
//                      preview shows the frame the short starts that piece
//                      with, and on the clip's last frame, put there by a
//                      click on its end, the short's last frame. The clip's
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
//   ["build mark", what]
//                      the line under the build starts with this mark,
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
//                      draws the rotate icon on every rotated piece
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
//                      way along it, with a click on the clip timeline,
//                      waits for the video preview to show that frame, and
//                      presses C, and the clip timeline has to show a crop
//                      mark lit on the playhead's frame within 300 ms,
//                      before a held answer comes
//   ["thumbnail at", share]
//                      puts the playhead in the clip, this share of the
//                      way along it, with a click on the clip timeline,
//                      waits for the video preview to show that frame, and
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
//   ["frame on the pill while it plays", seconds]
//                      puts the playhead a second before the end of the
//                      first caption that ends well inside the clip and
//                      plays for so many seconds, and on every frame the
//                      browser paints reads the pill libass drew off the
//                      canvas: where a pill is drawn, the caption box
//                      lights a word and its frame stands on the pill,
//                      across within 3 px of the pill's middle and down
//                      inside it, and where no pill is drawn no word is
//                      lit. At most one paint in twenty shows the layer of
//                      a frame before, and never for two frames on end
//   ["walk the words with the frame on the pill", n]
//                      from the clip's first frame, Shift and the right
//                      arrow n times, and on every frame the browser paints
//                      after each press reads the pill libass drew off the
//                      canvas: the keyboard's frame stands on the pill,
//                      within 3 px, on every frame it shows, never on a
//                      word the picture has not lit yet, and is there once
//                      the captions of the frame have come
//   ["layers late", ms]
//                      every caption layer the video preview asks the Go
//                      side for comes ms late, the way it does on a busy
//                      machine
//   ["answers late", ms]
//                      every answer of the engine to a gesture on the clip
//                      timeline, while the hand moves, comes ms late, the
//                      way it does on a busy machine
//   ["playhead on", edge]
//                      the playhead is on the frame the clip's "start" or
//                      "end" edge is on
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
//   ["on the playhead", n]
//                      n marks are lit by the playhead, a thumbnail or a
//                      keyframe on its frame, and each stands centred on
//                      the playhead's line
//   ["pinch in"]       pinches the clip timeline in to a tenth on the
//                      playhead, where a frame is many pixels wide, and
//                      takes the hand off it
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
//   ["glides across the cut"]
//                      with the playhead put on a frame between the clip's
//                      first keyframe and its first cut, and on one between
//                      the cut and its last keyframe, after it, the crop
//                      frame stands where the glide puts it, on its way
//                      from the one keyframe to the other
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
//                      in a place of its own on every frame shown, not once
//   ["fits"]           the list's trigger is as wide as the channel it
//                      names, with no room kept for a longer one
//   ["at once", head]  in the frame after the pick, the list said the
//                      channel picked and the line under it this head
//
// The episode is the bridge's, so the words are the sentences its speech
// stand-in says: "Ich war vielleicht sechs Jahre alt, als mich auf dem
// Schulhof irgendein Typ geschubst hat."
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, realpathSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, extname, join } from "node:path";
import { open, chosen, shown, engineState, handles, timeline, inside, middle, high, drag } from "./bridge.mjs";
import { clipList, control, pressHead, fromSidebar, settle, ask, episodeOn, shortFrames, shortSound } from "./bridge.mjs";
import { xOf, seekTo, cropFrame, sameLook, sayLook, readShort, loudness, filmedOnScreen, preview } from "./bridge.mjs";
import { recordFrames, addHEVC, overlay, cropPixels, shortFramesFrom, slowDown, BITS, nextPaint } from "./bridge.mjs";
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
    // Found by Tim on #201: a caption whose first word the picture lights
    // for a single frame showed nothing when its block was clicked, and
    // its block did not pop, while shift and an arrow went to that frame.
    // The clip's start dragged near the end of the first word lands it on
    // the frame before "war", which starts in the middle of the clip's
    // first frame, so the first frame shows its caption with no word lit
    // and a click a frame into "war" by its own time landed there.
    name: "every caption block clicked lights a word of its caption",
    steps: [["trim into", 0.04], ["every block"]],
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
    // Tim removed a caption block, removed its clip with the bin, and
    // while its row still offered it back pressed I where the playhead
    // was. The clip came back, and the caption he had removed was still
    // gone from it: a removed word was kept for the whole video, so every
    // clip over it lost it. Now it is the clip's, and goes with it, and the
    // clip put back has it removed as it was. Plan row 2.249.
    name: "a caption removed, then its clip removed and made again with I, has the caption again",
    steps: [
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["blocks"],
      ["remove clip"],
      ["make here", "i"],
      ["holds", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    // Putting a clip back takes back its remove and nothing more. Tim
    // played a clip, removed another and put it back, and the one put
    // back was chosen and played from its start. Issue #323.
    name: "a clip put back while another plays leaves the play alone",
    steps: [["remove clip"], ["make here", "i"], ["put back"], ["put back another while playing"]],
  },
  {
    // A clip is always chosen while there is one: removing the chosen clip
    // chooses the next, or the one before it at the end of the list, and
    // putting it back chooses it again. Tim found nothing chosen. Issue
    // #331.
    name: "removing the chosen clip chooses the one beside it",
    steps: [
      ["remove clip"],
      ["make here", "i"],
      ["put back"],
      ["remove the chosen", "first"],
      ["put back"],
      ["chosen again"],
      ["remove the chosen", "last"],
    ],
  },
  {
    // The clip chosen when it was removed is the one exception: putting it
    // back takes the remove back as it was, chosen, and the playhead stays.
    name: "the clip chosen, removed and put back, is chosen again where the playhead was",
    steps: [["put back the chosen"]],
  },
  {
    name: "a caption removed, then its clip removed and put back, is still removed",
    steps: [
      ["block", "Schulhof"],
      ["press", "Backspace"],
      ["mark", "removed"],
      ["remove clip"],
      ["put back"],
      ["same", "removed"],
      ["gone", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    // A word removed in the caption box is the clip's too, and goes with
    // it.
    name: "a word removed, then its clip removed and made again with I, has the word again",
    steps: [
      ["frame", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["remove clip"],
      ["make here", "i"],
      ["holds", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    // Plan row 2.249, Tim chose: a spelling fix is the clip's, and goes
    // with it.
    name: "a word corrected, then its clip removed and made again with I, reads as it was heard",
    steps: [
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["remove clip"],
      ["make here", "i"],
      ["gone", "Schulhoff"],
      ["holds", "Schulhof"],
      ["blocks"],
    ],
  },
  {
    name: "a word corrected in one clip reads as it was heard in another clip over it",
    steps: [
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["remove clip"],
      ["make here", "i"],
      ["put back"],
      ["clips with", ["Schulhoff", 1]],
      ["clips with", ["Schulhof", 1]],
    ],
  },
  {
    name: "a word removed in one clip is still shown in another clip over it",
    steps: [
      ["frame", "Schulhof"],
      ["press", "Backspace"],
      ["gone", "Schulhof"],
      ["remove clip"],
      ["make here", "i"],
      ["put back"],
      ["clips with", ["Schulhof", 1]],
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
    // A click on the edge where a caption appears put the playhead on the
    // frame that holds its first word's start, which begins before the
    // word, where the caption is not up yet. Tim saw it on #294.
    // Its caption starts at 10.15 s, between two frames of the test video.
    name: "a click on either edge of a caption shows the caption",
    steps: [
      ["caption edge", ["start", "jedenfalls"]],
      ["drawn", "jedenfalls"],
      ["caption edge", ["end", "jedenfalls"]],
      ["drawn", "jedenfalls"],
    ],
  },
  {
    name: "a caption's edges dragged with shift land on words",
    steps: [
      ["mark", "start"],
      ["edge to word", ["start", "Schulhof"]],
      ["drawn", "Schulhof"],
      ["others", ["start", "Schulhof"]],
      ["edge to word", ["end", "Schulhof"]],
      ["drawn", "Schulhof"],
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
    // An edge pressed takes the playhead with it, so the edge's move is
    // taken back with Undo while the playhead stands on the caption, the
    // same change with no hand on the edge.
    name: "a caption block pops for neither edge dragged in, with the playhead on it",
    steps: [
      ["edge in", ["start", "Schulhof"]],
      ["block", "Schulhof"],
      ["count pops"],
      ["undo"],
      ["pops", 0],
      ["edge in", ["end", "Schulhof"]],
      ["block", "Schulhof"],
      ["count pops"],
      ["undo"],
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
    // Found by Tim on #201: a word opened in the caption box made the
    // whole caption grow, drawn by the browser, and shrink again when it
    // was left, and leaving it popped the word lit.
    name: "a word opened in the caption box stays where libass draws it",
    steps: [["open in place", "Jahre"]],
  },
  {
    // Found by Tim on #201: a word typed longer ran out of its pill,
    // because libass drew the pill at the word's old size, and opening a
    // word on a frame of its pop is to move nothing.
    name: "a word typed longer grows its pill round its middle",
    steps: [["frame", "Jahre"], ["grow", "Jahre"]],
  },
  {
    // Found by Tim: in Archivo Black, a face of weight 400 that libass
    // thickens for bold captions, the word typed was narrower and lighter
    // than libass draws it.
    name: "a word typed in a face libass thickens is as heavy as libass draws it",
    steps: [["font", "Archivo Black"], ["frame", "Jahre"], ["grow", "Jahre"], ["thickened as libass", "Jahre"]],
  },
  {
    // Tim asked for the fonts installed on his Mac (#347). The bridge's
    // computer has Anton installed as Bridge Narrow, which the Font list
    // offers under Installed, and libass and the word typed are both drawn
    // from its copy.
    name: "a font installed on the computer draws the captions and the word typed",
    steps: [
      ["listed under", "Installed|Bridge Narrow"],
      ["search font", "brnar|Bridge Narrow"],
      ["frame", "Jahre"],
      ["grow", "Jahre"],
      ["thickened as libass", "Jahre"],
    ],
  },
  {
    // Found by Tim on #349: the Font list lined up with neither edge of
    // its field, its rows were squeezed to the height of their text, and
    // every name was in the app's face, so nothing showed what a font
    // looks like.
    name: "the font list hangs on the field's edge with rows as tall as it, each in its own face",
    steps: [["font list"]],
  },
  {
    // Found by Tim on #349: a search sorted what it found by how well it
    // answered, so Archivo Black stood under the head of the installed
    // fonts.
    name: "a search of the fonts keeps the order and the heads of the list",
    steps: [["search order", "b"]],
  },
  {
    // Tim asked for a search through what was said, in the clip column
    // (#366): a card for each sentence, the clip it is in named on it,
    // and the parts not heard yet drawn where the search cannot look.
    name: "a search of what was said finds the sentences, names their clips and puts the playhead on one",
    // A second clip, made with I, so choosing one on the range picker has
    // somewhere to take the list.
    steps: [["trim", "start", 300], ["make while paused", "i"], ["search said", "Jahre"]],
  },
  {
    // Found by Tim: a word corrected in the caption box read corrected in
    // the video preview and as it was heard in the search of what was
    // said. A clip's cards are its captions, so they read what it shows.
    name: "a word corrected in a clip reads corrected in the search of what was said",
    steps: [
      ["frame", "Schulhof"],
      ["click", "Schulhof"],
      ["press", "End"],
      ["type", "f"],
      ["press", "Enter"],
      ["holds", "Schulhoff"],
      ["search said", "Schulhoff"],
    ],
  },
  {
    // Tim, 19 October: the font list open and the transcript button
    // pressed left both open, the keyboard in the font list's field, and
    // Escape closed the font list first, though it was opened first.
    name: "the transcript button pressed with the font list open closes the list and takes the keyboard",
    steps: [["font then transcript"]],
  },
  {
    // Found in the review of #367: the cards that could not be read left
    // the column empty, with nothing said. A clip trimmed so they are read
    // again as the search opens.
    name: "cards of what was said that cannot be read say so in the list",
    steps: [["said fails", "on"], ["trim", "start", 300], ["said fails", "check"]],
  },
  {
    // The same on a video heard only as far as its first search reached,
    // so part of it is not heard yet and says so, in the list and on the
    // range picker.
    name: "a search of what was said shows the parts not transcribed yet",
    steps: [
      // Heard slowly, and held once some of it is, so part of it is still
      // to come while the search looks.
      ["speech", 1500],
      ["add", 600],
      ["heard some"],
      ["speech", 20000],
      ["search said", ["*", 1]],
      ["speech", 0],
    ],
  },
  {
    // Found by Tim on #349: the Style list of one style lay over the Size
    // field two pixels short of its bottom, so its border showed twice.
    name: "a list of one style covers the field under it",
    steps: [["style list"]],
  },
  {
    // Found by Tim on #349: Nothing matches stood where the names start,
    // after the room of a tick that is never there, and widened the list.
    name: "a search that finds nothing says so in the middle of a narrow list",
    steps: [["nothing found", "zzqx"]],
  },
  {
    // Found by Tim on #349: a family is picked first and then its style,
    // the way Pages does, so the list is as wide as a family's name. A
    // family picked comes in the style nearest the face it replaces.
    name: "a family picked keeps the look of the face it replaces",
    steps: [
      ["font", "Bridge Sans"],
      ["style is", "Heavy"],
      ["style", "Regular"],
    ],
  },
  {
    // Found by Tim on #201: a word opened while another was lit was typed
    // on the dark box, and the lit one on its pill inside a lighter ring.
    // Every word typed sits on the one pill libass draws, lit or not.
    name: "a word typed sits on the pill libass draws, lit or not",
    steps: [["one pill", "Jahre"]],
  },
  {
    // Found by Tim: the selection's blue reached over the pill, since a
    // selection is as tall as the face's line and the pill is not.
    name: "the letters selected in a word typed stay in its pill",
    steps: [["click", "Jahre"], ["selection in the pill"]],
  },
  {
    // Found by Tim: a word opened with Enter and left for Report a problem
    // kept its caret, and keys went on typing into it.
    name: "a word left for a button lets go of its caret",
    steps: [["click", "Jahre"], ["button", "Report a problem"], ["no caret"]],
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
    // Found by Tim on #201: several words typed into one and saved with
    // Enter left the picture with no captions, and corrections made
    // before were gone when he went back. A word typed into a short word
    // gets less than a frame of its time, and the engine left such a
    // word out of the captions. "Q" gets 12 ms of "war".
    name: "several words typed into one keep the captions in the picture and the corrections before",
    steps: [
      ["click", "sechs"],
      ["press", "End"],
      ["type", " eins zwei drei"],
      ["press", "Enter"],
      ["captions drawn"],
      ["holds", "drei"],
      ["click", "war"],
      ["press", "End"],
      ["type", " Q Schulhofgeschichten"],
      ["press", "Enter"],
      ["captions drawn"],
      ["holds", "Q"],
      ["holds", "Schulhofgeschichten"],
      ["holds", "drei"],
      ["press", "Shift+ArrowRight"],
      ["press", "Shift+ArrowRight"],
      ["press", "Enter"],
      ["type", "sieben acht neun"],
      ["press", "Enter"],
      ["captions drawn"],
      ["press", "Shift+ArrowRight"],
      ["captions drawn"],
      ["press", "Shift+ArrowLeft"],
      ["press", "Shift+ArrowLeft"],
      ["captions drawn"],
      ["holds", "neun"],
      ["holds", "Q"],
      ["holds", "drei"],
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
    // Tim's idea (2.269): paused, the frames of the word after the playhead
    // and of the word before are read while the hand rests, so Shift and an
    // arrow find their frame there. Before, a step to the next word read on
    // in the open stream, or opened one, while the hand waited.
    name: "a step to the word beside finds its frame read while the hand rested",
    steps: [
      ["add", 40, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["heard"],
      ["read ahead", "Shift+ArrowRight"],
      ["read ahead", "Shift+ArrowRight"],
      ["read ahead", "Shift+ArrowRight"],
      ["read ahead", "Shift+ArrowLeft"],
      // Tim heard nothing when clips played on #229, so a play after the
      // frames were read ahead is listened to.
      ["heard"],
    ],
  },
  {
    // Plan row 2.235, found by Tim: the clip's edges took the playhead
    // along and a cut's edges left it where it was, so the frame beside
    // the cut could not be seen while it was placed.
    name: "a cut's edges dragged take the playhead along, the way the clip's edges do",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", -40],
      ["followed", "cut from"],
      ["drag edge", "cut to", 40],
      ["followed", "cut to"],
      ["drag edge", "end", -40],
      ["followed", "end"],
      ["drag edge", "start", 40],
      ["followed", "start"],
    ],
  },
  {
    // Found by Tim on youtube.mp4 (2.270): dragging a cut's left edge
    // into the clip, the video preview flickered between the edge's frame
    // and the frames after the cut. Each step of the drag cues the play
    // from the edge, which reads the next piece ahead, and the step after
    // withdrew those frames: their coming late was taken for a place the
    // hand had passed, and they were put up. And a playhead a hair before
    // the end of the piece was cued as inside it, while the cue's plan
    // started after the cut. At 25 frames a second, as Tim's video is, and
    // back and forth, because one drag catches it about half the time.
    name: "a cut's left edge dragged shows the frames before the cut, never one after it",
    steps: [
      ["add", 40, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["cut at", 0.5],
      ["drag edge", "cut from", -60],
      ["nothing past the cut"],
      ["drag edge", "cut from", 40],
      ["nothing past the cut"],
      ["drag edge", "cut from", -60],
      ["nothing past the cut"],
      ["drag edge", "cut from", 40],
      ["nothing past the cut"],
      ["drag edge", "cut from", -60],
      ["nothing past the cut"],
      ["drag edge", "cut from", 40],
      ["nothing past the cut"],
    ],
  },
  {
    // Found by Tim on this: with shift, the playhead went inside the word
    // the edge landed on, a pause away from the edge, and dragging a cut's
    // left edge on past the last word took it over the cut into the
    // caption before. It stays in line with the edge.
    name: "a cut's edges dragged with shift keep the playhead in line with them",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", 40, "shift"],
      ["followed", "cut from"],
      ["drag edge", "cut from", -60, "shift"],
      ["followed", "cut from"],
      ["drag edge", "cut to", -40, "shift"],
      ["followed", "cut to"],
      ["drag edge", "cut to", 60, "shift"],
      ["followed", "cut to"],
      ["drag edge", "end", -60, "shift"],
      ["followed", "end"],
      ["drag edge", "start", 60, "shift"],
      ["followed", "start"],
    ],
  },
  {
    // Found by Tim: a press anywhere on the clip timeline moved the
    // playhead at once, and a press on an edge or a mark only when the
    // hand let go or began to move, so it caught up with a drag late.
    name: "a press on an edge or a mark puts the playhead on it at once",
    steps: [
      ["cut at", 0.5],
      ["thumbnail at", 0.25],
      ["keyframe at", 0.75],
      ["hold on", "end"],
      ["on press", "end"],
      ["hold on", "start"],
      ["on press", "start"],
      ["hold on", "cut from"],
      ["on press", "cut from"],
      ["hold on", "cut to"],
      ["on press", "cut to"],
      ["hold on", "thumbnail"],
      ["on press", "thumbnail"],
      ["hold on", "crop"],
      ["on press", "crop"],
      ["hold on", "caption start"],
      ["on press", "caption start"],
      ["hold on", "caption end"],
      ["on press", "caption end"],
    ],
  },
  {
    // Found by Tim: an edge let go of and taken again at once went to the
    // playhead standing on it. While the first drag was being saved the
    // edges were off the page.
    name: "an edge taken again at once after a drag is the edge's",
    steps: [
      ["cut at", 0.5],
      ["drag edge twice", "cut from", -30, -30],
      ["moved", "cut from"],
      ["followed", "cut from"],
      ["drag edge twice", "end", -30, -30],
      ["moved", "end"],
      ["followed", "end"],
    ],
  },
  {
    // An edge taken again before the last place the drag before it left
    // was answered starts from where that answer puts it, and the playhead
    // goes there with it. It started from the place answered before, a
    // frame behind, and the answer that came after the press put the edge
    // on its frame with the playhead left a frame away (#284).
    name: "an edge taken again before the drag before it is answered goes on from its answer",
    steps: [
      ["cut at", 0.5],
      ["drag edge twice", "cut from", -30, -30, 600],
      ["moved", "cut from"],
      ["followed", "cut from"],
      ["drag edge twice", "end", -30, -30, 600],
      ["moved", "end"],
      ["followed", "end"],
    ],
  },
  {
    // An edge taken again while the drag before it is still being saved
    // goes with the hand. It stood still until the save came back, which a
    // busy machine made long enough to see (2.259).
    name: "an edge taken again while the drag before it saves goes with the hand",
    steps: [
      ["cut at", 0.5],
      ["hold", "Reshape", 3000],
      ["drag edge twice", "cut from", -30, -30],
      ["kept up", "cut from"],
      ["followed", "cut from"],
      ["hold", "Reshape", 3000],
      ["drag edge twice", "end", -30, -30],
      ["kept up", "end"],
      ["followed", "end"],
    ],
  },
  {
    // A click on an edge and at once a press on the track: an answer for
    // the edge that came late put the playhead back on the edge.
    name: "a press on the track straight after a click on an edge keeps the playhead there",
    steps: [
      // The engine is slow to answer, the way it can be on a long
      // episode, so an answer to the click comes after the press.
      ["cut at", 0.5],
      ["hold", "Shape", 300],
      ["edge then track", "cut from"],
      ["hold", "Shape", 300],
      ["edge then track", "end"],
      ["hold", "Shape", 300],
      ["edge then track", "start"],
    ],
  },
  {
    // Found by Tim: a cut's left edge pressed with shift and dragged right
    // first jumped left, to the nearest word, and only then went right.
    name: "an edge dragged with shift goes the way of the hand",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", 8, "shift"],
      ["went the hand's way", "cut from"],
      ["drag edge", "cut to", -8, "shift"],
      ["went the hand's way", "cut to"],
      ["drag edge", "end", -8, "shift"],
      ["went the hand's way", "end"],
      ["drag edge", "start", 8, "shift"],
      ["went the hand's way", "start"],
    ],
  },
  {
    // Found by Tim: a cut's edge dragged takes the playhead along, so it
    // stands on the edge, and the next press there took hold of the
    // playhead and not the edge.
    name: "a cut's edge with the playhead on it is the edge's to take",
    steps: [
      ["cut at", 0.5],
      ["drag edge", "cut from", -40],
      ["drag edge", "cut from", -30, "at the playhead"],
      ["moved", "cut from"],
      ["followed", "cut from"],
    ],
  },
  {
    // Plan row 2.236, found by Tim: with the playhead on a caption after a
    // cut, the cut's edges moved made the caption block and the pill in
    // the video preview pop again and again. A word was known by when it
    // starts on the clip's clock, which the cut moves. Undo takes the cut
    // back with the playhead standing on the caption, which is the same
    // change with no hand on an edge.
    name: "a cut changed before the caption under the playhead pops nothing",
    steps: [
      ["cut at", 0.4],
      ["block after", 1],
      ["drag edge", "cut from", -40],
      ["block after", 1],
      ["count pops"],
      ["undo"],
      ["pops", 0],
      ["pill pops", 0],
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
    // Found in a review: a caption block kept the press to itself, so a
    // double-click on one, across the middle of the clip, cut nothing.
    name: "a double-click on a caption cuts a part out there",
    steps: [
      ["cut on caption", 1],
      ["cuts", 1],
    ],
  },
  {
    // Tim swiped along the clip timeline with two fingers, clicked to put
    // the playhead where he was, and could not swipe on from there. The
    // playhead was then under the hand, and it was the one thing the
    // swipe did not go through.
    name: "a swipe goes on after a click put the playhead under the hand",
    steps: [
      ["swipe after click", 0.3, 60],
      ["swipe after click", 0.5, -60],
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
      ["bugs"],
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
      ["bugs"],
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
    // Enter keeps what was said, as the microphone does. Escape removed
    // it then, and Tim lost a long report to an Escape meant for the app,
    // with nothing Cmd+Z could bring back, issue #355. Escape keeps it
    // too now, as it does on a Mac, and Cmd+Z takes it back as one step
    // and Cmd+Shift+Z brings it back again.
    name: "Enter and Escape keep what was dictated, and undo takes it back",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["dictate", "enter"],
      ["report reads", "It * froze."],
      ["dictate", "escape"],
      ["report grew"],
      ["undo"],
      ["report as before"],
      ["redo"],
      ["report as before", "grown"],
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
    // Plan row 2.245. Settings, Advanced shows the app's log, and its bin
    // removes every file of it, the older one too, after asking. Cancel
    // leaves it as it was, and the log that follows is the one new file.
    name: "The app's log is removed whole from Settings, after asking",
    steps: [
      ["app log", "open"],
      ["settings"],
      ["app log", 2],
      ["button", "Remove the app's log"],
      ["button", "Cancel"],
      ["app log", 2],
      ["button", "Remove the app's log"],
      ["button", "Remove them"],
      ["app log", 1],
    ],
  },
  {
    // Plan row 2.189. Tim could not click into the words still being
    // heard, nor at the end of their line, to put the cursor there or to
    // correct a word while he talked. A click in them settles them where
    // they are, and what is heard after comes after them.
    name: "A click in the words being heard puts the cursor there while dictating",
    steps: [
      ["bugs"],
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
      ["bugs"],
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
      ["bugs"],
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
    // Plan row 2.244. Tim asked for one report window at a time: Bugs
    // opens it, Bugs again brings it forward, and once it is closed Bugs
    // opens a new one. Bugs is lit while it is open.
    name: "Bugs opens one report window and brings it back",
    steps: [
      ["windows", 0, 0],
      ["bugs"],
      ["windows", 1, 0],
      ["bugs"],
      ["windows", 1, 1],
      ["close report"],
      ["windows", 1, 1],
      ["bugs"],
      ["windows", 2, 1],
    ],
  },
  {
    // Plan row 2.244. Closing the report window loses nothing: what was
    // written and the pictures are there when Bugs opens it again.
    name: "What is written and attached stays when the report window closes",
    steps: [
      ["bugs"],
      ["write", "It froze.", 2],
      ["attach", 1],
      ["pictures", 1],
      ["close report"],
      ["bugs"],
      ["report reads", "It froze."],
      ["pictures", 1],
    ],
  },
  {
    // Plan row 2.244. Pictures come from the paperclip's box, a drop on
    // the window and a paste into the field, and a file that is not a
    // picture is left out with the reason. Everything lines up.
    name: "Pictures come from the paperclip, a drop and a paste, and what is not one is left out",
    steps: [
      ["bugs"],
      ["attach", 1],
      ["drop pictures", 2, "notes.txt"],
      ["toast", "notes.txt cannot be attached because it is not a picture or a screen recording"],
      // A good drop after it takes the old reason away. Found in the
      // review of #217.
      ["drop pictures", 1],
      ["toast", ""],
      ["paste picture"],
      ["pictures", 5],
      ["report lines up"],
    ],
  },
  {
    // Issue #362. Tim saw the buttons a size of their own, mostly much
    // wider than their words, and some of them growing past it on his
    // Mac. A button is as wide as the words it shows, whatever they are.
    name: "every button is as wide as its words and no wider",
    steps: [
      ["buttons fit", ["Search", "Render"]],
      ["settings"],
      ["buttons fit", ["Choose…", "Check again"]],
      ["updates", { from: "main", list: "main,pr-20" }],
      ["status", "A newer build is ready"],
      ["buttons fit", ["Relaunch"]],
      ["bugs"],
      ["buttons fit", ["Report"]],
    ],
  },
  {
    // Issue #370. Tim pressed Escape on the Font list and the list let
    // go with no ring, on the Style list and its trigger kept the ring,
    // and in the Size field and nothing happened. Escape lets go of
    // every list, pop-up and field the same way, everywhere in the app.
    name: "Escape lets go of every list, pop-up and field with no ring left",
    steps: [
      ["escape lets go", "#caption-font"],
      ["escape lets go", "#caption-style"],
      ["escape lets go", "label.setting:has-text('Size') input"],
      ["escape lets go", "button.colour >> nth=0"],
      ["settings"],
      ["escape lets go", "input.licence-key"],
    ],
  },
  {
    // Issue #326. Tim attached a screen recording made on his Mac and
    // it was turned away as not a PNG or JPEG picture. A recording is
    // attached beside the pictures, shows a still of its first frame and
    // its length, and goes in the report as a recording.
    name: "A screen recording is attached, shown by its first frame and its length, and goes in the report",
    steps: [
      ["bugs"],
      ["drop pictures", 1],
      ["drop recording"],
      ["toast", ""],
      ["pictures", 2],
      ["recording tile", "0:03"],
      ["report lines up"],
      ["report", [1, 1]],
    ],
  },
  {
    // Plan row 2.244. Tim attached screenshots of every shape: tall ones
    // showed only a strip of their middle, and a small tall one showed
    // nothing at all. Every picture is drawn whole, in the middle of its
    // tile and as large as it lets it be.
    name: "Pictures of every shape are drawn whole in their tiles",
    steps: [
      ["bugs"],
      ["drop pictures", "446x738 152x338 1492x190 142x104 786x504 380x1766"],
      ["pictures", 6],
      ["pictures fit"],
    ],
  },
  {
    // Plan row 2.244. Tim ended a dictation at the end of a long report
    // with Enter and with the microphone, and the microphone left the
    // field scrolled half a line up where Enter kept its end in view.
    // Both keep the end of what was said in view.
    name: "Stop and Enter both leave the end of a long report in view",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve\nThirteen\nFourteen", "end"],
      ["dictate", "button"],
      ["report at end"],
      ["dictate", "enter"],
      ["report at end"],
    ],
  },
  {
    // Plan row 2.244. Tim scrolled up to read while he talked, and the
    // field pulled him back down with every word. It follows the words
    // the way a terminal follows its output, until scrolled away from
    // them and again once back at them. And the cursor stood at the front
    // of the last line while the words ran on.
    name: "While dictating the field follows the words until scrolled away, like a terminal",
    steps: [
      ["bugs"],
      ["write", "One\nTwo\nThree\nFour\nFive\nSix\nSeven\nEight\nNine\nTen\nEleven\nTwelve\nThirteen\nFourteen\nFifteen\nSixteen", "end"],
      ["dictate", "start"],
      ["report follows"],
      ["caret at words"],
      ["report scroll", -120],
      ["report stays"],
      ["report scroll", 5000],
      ["report follows"],
      ["caret elsewhere"],
      ["dictate", "button"],
    ],
  },
  {
    // Plan row 2.244. A report takes ten pictures, and the eleventh is
    // left out with the reason.
    name: "A report takes ten pictures and says why it leaves out the eleventh",
    steps: [
      ["bugs"],
      ["drop pictures", 10],
      ["pictures", 10],
      ["attach", 1],
      ["toast", "would be more than the 10 pictures and recordings a report takes"],
      ["pictures", 10],
    ],
  },
  {
    // Plan row 2.244. The bin on a picture removes it, the picture keeps
    // its place and offers itself back, and once its time runs out the
    // grid closes over it.
    name: "A removed picture can be put back, and then the grid closes over it",
    steps: [
      ["bugs"],
      ["attach", 3],
      ["remove picture", 2],
      ["put back picture", 2],
      ["remove picture", [2, "tile"]],
      ["put back picture", 2],
      ["pictures", 3],
      ["remove picture", [2, "key"]],
      ["removed goes"],
      ["pictures", 2],
    ],
  },
  {
    // Plan row 2.244. Tim clicked Bugs, and the sidebar stayed open behind
    // the report window until he clicked back into the app and moved the
    // pointer.
    name: "Bugs leaves the sidebar closed once the report window has the focus",
    steps: [["bugs, the pointer gone"]],
  },
  {
    // Plan row 2.244. Report makes one zip with what was written and the
    // pictures, and the next report starts empty.
    name: "Report makes one zip with the pictures, and the next report starts empty",
    steps: [
      ["bugs"],
      ["write", "It froze.", 0],
      ["attach", 2],
      ["report lines up"],
      ["report", 2],
      ["close report"],
      ["bugs"],
      ["pictures", 0],
    ],
  },
  {
    // Plan row 2.189. Tim talked past the longest a recording can be and
    // lost what he said after it, with nothing to say why. A recording
    // nobody talks into stops by itself, keeps every word as Stop would,
    // and says why under the field. The bridge's waits 8 s.
    name: "Dictation nobody talks into stops by itself and keeps what was said",
    steps: [
      ["bugs"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
      ["cards", 1],
      ["whole clip", 15],
      ["rulers fit", 15],
    ],
  },
  {
    // Tim removed the one clip of a video of 15 s and pressed Search, and
    // every search failed with "there is nothing to make a clip of in
    // this video": the clip removed still counted as the one moment the
    // video has. Search brings it back, and Search with it in the list finds
    // nothing more, fails nothing either, and says so where its clip
    // would have appeared.
    name: "a video shorter than a clip gets its clip again once it was removed",
    steps: [
      ["add", 15],
      ["wait for", "Search"],
      ["whole clip", 15],
      ["remove clip"],
      ["head"],
      ["wait for", "Search"],
      ["whole clip", 15],
      // The clip made again is the clip that was removed, so the removed
      // one is not offered back, and undoing its removal does not bring
      // it back beside the new one. Both showed the video twice.
      ["no row", "Nothing new found"],
      ["no put back"],
      ["undo refused"],
      ["whole clip", 15],
      ["head"],
      ["wait for", "Search"],
      ["whole clip", 15],
      // Tim saw the row of that search close again with nothing to say.
      // It says so now, as news: it runs down like a removed clip's row
      // and closes.
      ["row closes", ["Nothing new found", 10]],
      // Search again is a new search, and it says so again.
      ["head"],
      ["wait for", "Search"],
      ["row closes", ["Nothing new found", 10]],
    ],
  },
  {
    // Tim removed a clip, worked on in the workspace, and the time to put
    // it back stood still, the fill and all, until his hand passed over
    // the row again. On his Mac a row moved out from under a hand that
    // stood still is never told the hand is off it, and the time waited
    // for that word. Plan row 2.240.
    name: "a row to put back runs down once the hand is elsewhere, though it moved from under a still hand",
    steps: [["trim", "start", 300], ["make while paused", "i"], ["remove two, hand still"]],
  },
  {
    // Tim removed clips with the bin, and once a row's time ran out it
    // closed to a thin strip and then the list jumped up the rest in one
    // frame: the space between the rows stayed until the row was gone.
    // Plan row 2.275.
    name: "a removed row closes with the space after it, in one movement",
    steps: [["trim", "start", 300], ["make while paused", "i"], ["row folds"]],
  },
  {
    // Tim felt the app lag while it worked. A running search reads the
    // clip list again every two seconds, and every read made every clip a
    // new one, the clip on screen too, so its captions were asked for
    // again and the clip timeline and the caption box drew them again,
    // with nothing changed. Plan row 2.242.
    name: "a running search leaves the clip on screen alone",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["asks none", "Captions", 7],
    ],
  },
  {
    // A swipe along the clip timeline told the waveform's canvas its size
    // on every step, which throws its pixels away and makes it new, twice
    // the clip timeline's pixels on a Mac, with nothing changed in size.
    // Plan row 2.242.
    name: "a swipe along the clip timeline draws the waveform again on the same canvas",
    steps: [["swipe keeps the canvas", 12]],
  },
  {
    // WebKit lets the timers of an app that is not in front fire once a
    // second at most, and the log said the window stood still every time
    // the app came back to the front: 262 lines in two days of Tim's
    // report, none while he worked. A window held up in front is still
    // said. Plan row 2.242.
    name: "the log says the window stood still only while the app is in front",
    steps: [
      ["stands still", true],
      ["stands still", false],
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
      ["wait for", "Search"],
      ["cards", 1],
    ],
  },
  {
    // Tim cancelled a search to look somewhere else, and the window stood
    // locked on the range picker until the search was carried on. Moved
    // off it, the search is set aside: Search, and a fresh search where the
    // window is. Undo puts it back on the search, and Continue with it.
    name: "after Cancel the window moves, and Search looks where it was moved",
    steps: [
      ["model", "holds"],
      ["head"],
      ["wait for", "Cancel"],
      ["head"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["move window", "to", -0.4],
      ["wait for", "Search"],
      ["no row", "Stopped. Click Continue"],
      ["undo"],
      ["wait for", "Continue"],
      ["row", "Stopped. Click Continue"],
      ["redo"],
      ["wait for", "Search"],
      ["restart"],
      ["wait for", "Search"],
      ["no row", "Stopped. Click Continue"],
      ["model", "answers"],
      ["head"],
      ["wait for", "Search"],
      ["searched moved"],
    ],
  },
  {
    // Plan row 2.131. Cancel is pressed before the Go side has heard of
    // the search that Search asked for, which is held on its way there, so the
    // Cancel reaches the Go side first. The search never starts, and says
    // Stopped with Continue where its clips would have been.
    name: "Cancel pressed at once after Search stops the search before it starts",
    steps: [
      ["hold", "Search", 1500],
      ["search and cancel"],
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
      ["wait for", "Search"],
    ],
  },
  {
    // Tim's steps for #116, on a clip a search parted at a camera switch
    // into two pieces, each with the crop of its own shot.
    name: "a cut or an edge put back keeps the camera switch, and the short runs straight through it",
    steps: [
      ["add cameras", 30, 12],
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
    // Found by Tim testing #340, issue #348: rotating a part with R took
    // its keyframe diamond off the clip timeline, and the rotate mark
    // stood under the diamond. The keyframe stays through four turns, and
    // the rotate icon is stamped in the part's wash in the dark of the
    // empty track, large, in its middle, clear of the row the keyframes are in and of the row the
    // thumbnails are in. TestRotatingAPartKeepsItsKeyframes holds the
    // engine to it.
    name: "a rotated part keeps its keyframe, and has the rotate icon drawn in its middle",
    steps: [
      ["add", 30, "25", "turned"],
      ["wait for", "Search"],
      ["cards", 1],
      ["cut at", 0.5],
      ["cuts", 1],
      ["keyframe at", 0.75],
      ["keyframes", 1],
      ["rotate at", 0.75],
      ["turned", [0, 90]],
      ["keyframes", 1],
      ["stamp"],
      ["rotate at", 0.75],
      ["keyframes", 1],
      ["rotate at", 0.75],
      ["keyframes", 1],
      ["rotate at", 0.75],
      ["turned", [0, 0]],
      ["keyframes", 1],
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
      ["wait for", "Search"],
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
    // Found by Tim: a keyframe before a cut and the next one after it did
    // not glide, the crop stood still on both sides. Plan row 2.238.
    name: "a keyframe before a cut glides to one after it",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["keyframe at", 0.2],
      ["keyframe at", 0.8],
      ["drag crop", 400],
      ["keyframes", 2],
      ["cut at", 0.5],
      ["cuts", 1],
      ["glides across the cut"],
    ],
  },
  {
    // Found by Tim: a thumbnail and a keyframe lit by the playhead stood
    // half a pixel and a pixel to the right of its line. Plan row 2.234.
    name: "a thumbnail and a keyframe on the playhead's frame stand centred on its line",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["thumbnail at", 0.1],
      ["keyframe at", 0.1],
      ["on the playhead", 2],
      ["thumbnail at", 0.9],
      ["keyframe at", 0.9],
      ["on the playhead", 2],
      ["pinch in"],
      ["on the playhead", 2],
    ],
  },
  {
    // Found by Tim on #192: C put no mark on a clip's first or last frame,
    // only one frame in.
    name: "a keyframe goes on a clip's first and last frame with C, and comes off with C",
    steps: [
      ["add", 30, "25"],
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
      ["cards", 1],
      ["thumbnail at", 0.1],
      ["keyframe at", 0.9],
      ["open a word, then the mark", "thumbnail"],
      ["open a word, then the mark", "crop"],
    ],
  },
  {
    // Found on #201: on a busy machine the walk "an episode opens on its
    // clip" broke, because the hand let go of the clip's start before the
    // engine's answer to where it was let go had come, and the playhead
    // stayed on a frame the trim had cut away.
    name: "an edge let go before the engine answers leaves the playhead on the edge",
    steps: [
      ["answers late", 500],
      ["trim", "start", 160],
      ["playhead on", "start"],
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
    // Found by Tim on #201: while a clip played, the frame round a word
    // went on to the first word of the next caption a few frames before
    // the pill did, since the caption box followed the playhead and the
    // pill the frame drawn.
    name: "the frame round a word stays on the pill while the clip plays",
    steps: [["frame on the pill while it plays", 3]],
  },
  {
    // Found by Tim on #201: Shift and an arrow put the keyboard's frame
    // on the next word a frame before the pill came, while the picture's
    // captions were on their way.
    name: "the keyboard's frame goes from word to word with the pill",
    steps: [
      ["layers late", 120],
      ["walk the words with the frame on the pill", 8],
    ],
  },
  {
    // Found by Tim on #201: a step onto a word left it standing still,
    // where the browser's captions before had popped it, and the word and
    // its pill did not grow together.
    // Filmed at 29.97 fps, as a camera records: at the 5 fps of the
    // other walks' videos the pop is over before the next frame.
    // At 25 fps, the rate of Tim's videos, forward and back again to the
    // clip's first word: Tim saw some words pop fully, some barely, some
    // rise and snap back, and the first word not at all.
    // Then Tim saw the pop the short burns in played again over every
    // frame he stepped to with an arrow alone, two animations mixed into
    // one: a step from frame to frame shows the short's frames and nothing
    // over them, and only a step from word to word bounces.
    name: "a step onto a word pops it over the paused frame",
    steps: [
      ["add", 40, "25"],
      ["wait for", "Search"],
      ["cards", 1],
      ["step onto words and see them pop", 6],
      ["step frame by frame and see nothing pop", 40],
    ],
  },
  {
    // Plan row 2.130: a picture that starts after its sound, by an edit
    // list. The clip's start is dragged to before the picture starts,
    // where the render took the picture's frames from its first on and the
    // sound from where the picture starts.
    name: "a short of a picture that starts after its sound shows what the video preview shows",
    steps: [
      ["add", 40, "30000/1001", "late"],
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
      ["wait for", "Search"],
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
    // Issue #327. Tim dropped a picture on the app and a red line
    // over the workspace pushed it all down. What went wrong with nothing
    // on screen about it is a toast that moves nothing, goes by itself
    // after a while the way toasts do, stays while the pointer is on it,
    // and goes at once by its cross.
    name: "a file that is not a video is said in a toast that moves nothing and goes by itself unless held",
    steps: [
      ["drop not a video", "workspace"],
      ["not a video"],
      ["toast goes"],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["toast held"],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["toast closed"],
      ["drop not a video", "workspace"],
      ["not a video"],
      ["drop not a video", ["workspace", "zweite"]],
      ["not a video"],
      ["stacked"],
      ["toast closed"],
      ["toast closed"],
      ["no reason"],
    ],
  },
  {
    // Plan row 2.247. Tim deleted a video's folder in Finder to remove the
    // video, and it stayed in the sidebar with no clips and no lamp, and
    // opening it wrote a folder back. Its folder is everything the app
    // has of a video, so a video whose folder is gone has left the list,
    // the way Remove with Delete takes it.
    name: "a video whose folder is deleted outside the app leaves the list",
    steps: [
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "away"],
      ["let go"],
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "closed"],
      ["let go"],
      // Tim then deleted two videos from Documents with their folders,
      // and they stayed with a red lamp, File not found.
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "with the video"],
      ["let go"],
      // One of them had been opened after it was deleted, which wrote its
      // chosen clip back, and the app opened on it after the update and
      // said it was not in the library.
      ["add", 20],
      ["wait for", "Search"],
      ["folder removed", "written back"],
      ["let go"],
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
    // Tim switched the loop on early in a clip with a cut, let it run, and
    // at the end the sound went round while the video preview stood on the
    // clip's last frame (2.264).
    name: "a clip looped from early on goes round its end, the picture with the sound",
    steps: [
      ["cut at", 0.85],
      ["loop through the end", 8],
    ],
  },
  {
    // The loop switched off with L as the sound crosses the clip's end,
    // while the page is held and the playhead has not caught up, ends the
    // play where the sound is, after one more time through. It ended the
    // play at the end the sound had just crossed, and the clip's start was
    // heard for a moment after it, on a busy machine (2.253).
    name: "loop switched off as the sound crosses the clip's end ends the play ahead of the sound",
    steps: [["loop off as the sound crosses the end"]],
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
    // A clip's end dragged past the end of the video was saved there, and
    // its short then came out shorter than the clip and failed its check.
    // On the bridge's video of 120 s the second drag saved 456.4 s. Plan
    // row 2.170.
    name: "a clip's end dragged past the end of the video stops where the video ends",
    steps: [["past the end"], ["from the app", 4]],
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
    // Issue #361, asked for by Tim: the row under the clip timeline said
    // the chosen clip's title and numbers, which its card says, and
    // nothing said where the playhead was.
    name: "the playhead's time is in a field beside the buttons, and the clip's numbers on its card",
    steps: [["time field"], ["card says"], ["cut at", 0.5], ["card says"], ["card follows", -60], ["card says"]],
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
      ["build mark", "triangle, warning"],
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
      ["build mark", "dot, warning"],
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
    // The icon by its name, which Lucide puts on the svg. Its outline is
    // no guide: the triangle's path once ended in Z and no longer does.
    const shape = dot
      ? "dot"
      : mark.querySelector("svg.lucide-triangle-alert")
        ? "triangle"
        : mark.querySelector("svg.lucide-check")
          ? "check"
          : "";
    const colour = dot ? getComputedStyle(dot).backgroundColor : getComputedStyle(mark).color;
    return colour === warn ? `${shape}, warning` : shape;
  }
}
const updatesPage = (page) => page.evaluate(readUpdates);

// How long the page took from a key going down to the first animation
// frame on which shows holds, in the page's own time. Timed from Node, a
// press and the polling that waits for its answer are calls to the
// browser, and on a busy runner each can take a frame or more to arrive:
// C was once 538 ms "after C" in CI where the page showed the keyframe in
// a frame or two. The time starts when the page handles the keydown, so
// it is how long the app took to answer the key, not how long a busy
// machine took to hand the key over. -1 when it never holds within the
// timeout.
async function keyShows(page, key, shows, arg) {
  await page.evaluate(
    ([source, arg]) => {
      const shows = new Function(`return (${source})`)();
      window.__keyShows = { down: -1, took: -1 };
      addEventListener("keydown", () => (window.__keyShows.down = performance.now()), { capture: true, once: true });
      const tick = () => {
        const k = window.__keyShows;
        if (k.down < 0) return requestAnimationFrame(tick);
        if (shows(arg)) k.took = performance.now() - k.down;
        else requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
    },
    [shows.toString(), arg],
  );
  await page.keyboard.press(key);
  return page
    .waitForFunction(() => window.__keyShows.took >= 0, null, { timeout: 3000 })
    .then(() => page.evaluate(() => Math.round(window.__keyShows.took)), () => -1);
}

// Reads the edge of that label, the playhead and the frame the video
// preview shows on every animation frame, all in the same frame, until
// __followWatch is set false: the edge into __edgeSeen, and the three into
// __edgeFrames, the edge as NaN on a frame it is not drawn.
function watchEdge(page, label) {
  return page.evaluate((label) => {
    window.__edgeSeen = [];
    window.__edgeFrames = [];
    window.__followWatch = true;
    // Its own, so a watch that was stopped stops even when the next has
    // begun by its next frame, as watchPills does. On a machine too busy
    // to draw a frame between the two, it read the new watch's
    // __followWatch and went on reading its own edge into the new watch.
    const id = (window.__followId ?? 0) + 1;
    window.__followId = id;
    const tick = () => {
      if (!window.__followWatch || window.__followId !== id) return;
      const screen = document.querySelector(".screen");
      const playhead = Number(screen?.dataset.playhead);
      const shown = Number(screen?.dataset.shown);
      const edge = document.querySelector(`.clip-timeline [aria-label="${label}"]`);
      const at = edge ? Number(edge.getAttribute("aria-valuenow")) : NaN;
      if (edge) window.__edgeSeen.push(at);
      window.__edgeFrames.push([at, playhead, shown]);
      requestAnimationFrame(tick);
    };
    tick();
  }, label);
}

// Waits up to five seconds for the video preview to show the frame that
// holds the playhead, by where the app says the frame shown begins, and
// says whether it did. T and C put a mark on the frame shown, the frame a
// person sees, and on a busy machine the picture comes a moment after the
// playhead.
async function shownOnPlayhead(page) {
  const { fps, videoStart = 0 } = await ask(page, "Source", await episodeOn(page));
  return page
    .waitForFunction(
      ([fps, start]) => {
        const screen = document.querySelector(".screen");
        const at = Number(screen?.dataset.playhead);
        const shown = Number(screen?.dataset.shown);
        // The frame that holds the playhead, one that begins a millisecond
        // or less after it included, see frameAt in lib/flow.ts.
        const begins = start + Math.floor((at - start + 0.001) * fps) / fps;
        return Number.isFinite(shown) && Math.abs(shown - begins) < 0.5 / fps;
      },
      [fps, videoStart],
      { polling: 50, timeout: 5000 },
    )
    .then(() => true, () => false);
}

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
// A moment the way the clip list says it, clock in lib/api.ts.
const clockOf = (t) => {
  const h = Math.floor(t / 3600);
  const m = Math.floor((t % 3600) / 60);
  const ss = String(Math.floor(t % 60)).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
};

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
  // A wheel moves the view at once, see wheel in ClipTimeline.svelte, so
  // two frames have drawn it.
  await nextPaint(page, 2);
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
      await nextPaint(page, 2);
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
    // Where each click put the playhead, which a failure says.
    const tried = [];
    for (let tries = 0; tries < 4; tries++) {
      if (grips.some(([a, b]) => x > a && x < b)) break;
      await page.mouse.click(x, high(g.track));
      await settle(page);
      placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
      tried.push(placed.toFixed(4));
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
      // With the clicks that put the playhead there, and where the frame
      // shown begins by the app's count, since this failed only on a busy
      // machine, issue #325, and said nothing of how it came about.
      const shown = await page.evaluate(() => document.querySelector(".screen").dataset.shown);
      problems.push(
        `frame ${j} of the short, for ${at.toFixed(4)} s, is frame ${frames[j]} of the episode, the video preview shows ${seen} at ${placed.toFixed(4)}` +
          `${came ? ", after it showed the right one" : ""}, with the playhead put at ${tried.join(", then ")} and the frame shown beginning at ${shown}`,
      );
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
    // On the edge's frame, where the engine puts the playhead for an
    // edge, see edgePlayhead in engine/shape.go: the first frame of the
    // piece an edge starts, and the clip's last frame for its end.
    const placed = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
    // A millisecond early is on the frame, the rounding the edges are kept
    // to, which is the case this is about.
    const frameOn = nearest(e.at) - (e.label === "Clip end" ? 1 : 0);
    if (Math.floor((placed - videoStart + 0.001) * fps) !== frameOn) {
      problems.push(`a click on ${e.what} put the playhead at ${placed.toFixed(4)}, not on its frame ${frameOn}`);
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
  // Two steps of one name are the same mistake, and a quieter one: the
  // switch below takes the first case of a name and never reaches the
  // second, so the walks that meant the second check nothing. Two pull
  // requests that each add a step can pick the same name, and the steps
  // are read from this file's own source, as the one switch has them.
  const steps = [...readFileSync(new URL(import.meta.url), "utf8").matchAll(/^\s*case ("[^"]*"):/gm)].map((m) => JSON.parse(m[1]));
  const stepTwice = steps.filter((n, i) => steps.indexOf(n) !== i);
  if (stepTwice.length) {
    console.log(`two steps are called ${JSON.stringify(stepTwice[0])}`);
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
  const opened = await open(url);
  const { browser, errors } = opened;
  // The page the steps act on: the app's window, or the report window
  // once Bugs opened it. The watch keeps to the app's window.
  let page = opened.page;
  const main = page;
  let reportPage = null;
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
  // The episode's last search and the window "move window" left, for
  // "searched moved".
  let movedOff = null;
  // The clip "remove clip" removed last, for "put back".
  let removedLast = null;
  let removedChosen = "";
  let loopEnd = null;
  // Whether the last "drag edge" held shift.
  let dragShift = false;
  const edgeLabel = { start: "Clip start", end: "Clip end", "cut from": "Where cut 1 starts", "cut to": "Where cut 1 ends" };
  let dragPx = 0;
  // Whether the edge "drag edge twice" took again moved while the hand
  // still held it.
  let movedHeld = false;
  // What the press that took the edge again landed on, the grips under
  // it, said if the playhead did not go with the edge (2.271).
  let pressedOn = "";
  let held = null;
  let wrong = null;
  // The video and the other file dropped last.
  let droppedVideo = "";
  // The video whose folder a walk deleted from outside the app.
  let folderGone = "";
  // Whether the walk deleted that video too.
  let videoGone = false;
  let droppedOther = "";
  let droppedBefore = "";
  // Where the workspace and the report's field stood before a toast came.
  let standing = null;
  const done = [];
  const word = (text) => page.locator(".captions .word").filter({ hasText: new RegExp(`^${text}$`) }).first();
  // The letters on screen in a box of the page, libass's on the canvas or
  // the caption box's own, as the box of their near white pixels, in
  // pixels of the page from the box's corner. Read off a screenshot, since
  // the page's own text is on no canvas.
  const letters = async (box) => {
    const clip = { x: Math.floor(box.left), y: Math.floor(box.top), width: Math.ceil(box.width), height: Math.ceil(box.height) };
    const shot = (await page.screenshot({ clip })).toString("base64");
    return page.evaluate(async (data) => {
      const img = new Image();
      img.src = `data:image/png;base64,${data}`;
      await img.decode();
      const c = document.createElement("canvas");
      c.width = img.width;
      c.height = img.height;
      const g = c.getContext("2d", { willReadFrequently: true });
      g.drawImage(img, 0, 0);
      const d = g.getImageData(0, 0, c.width, c.height).data;
      let left = Infinity, right = -Infinity, top = Infinity, bottom = -Infinity, n = 0;
      for (let y = 0; y < c.height; y++)
        for (let x = 0; x < c.width; x++) {
          const i = (y * c.width + x) * 4;
          if (d[i] > 200 && d[i + 1] > 200 && d[i + 2] > 200) {
            n++;
            left = Math.min(left, x);
            right = Math.max(right, x + 1);
            top = Math.min(top, y);
            bottom = Math.max(bottom, y + 1);
          }
        }
      const most = (keep) => {
        const seen = new Map();
        for (let i = 0; i < d.length; i += 4)
          if (keep(d[i], d[i + 1], d[i + 2])) {
            const k = `${d[i]},${d[i + 1]},${d[i + 2]}`;
            seen.set(k, (seen.get(k) ?? 0) + 1);
          }
        return [...seen].sort((a, b) => b[1] - a[1])[0]?.[0] ?? "";
      };
      const ink = most((r, g, b) => r > 200 && g > 200 && b > 200);
      const pill = most((r, g, b) => !(r > 200 && g > 200 && b > 200) && Math.max(r, g, b) - Math.min(r, g, b) > 40);
      return n ? { left, right, top, bottom, width: right - left, height: bottom - top, n, ink, pill } : null;
    }, shot);
  };
  // Until the captions over the frame are drawn, twice over, so a layer
  // asked for by the last is drawn too.
  const settled = async () => {
    for (let i = 0; i < 2; i++) {
      await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
      await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
    }
  };

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
      case "trim into": {
        // The clip's start dragged to a moment arg seconds before the end
        // of its first word, so what is left of that word is a sliver.
        const now = await engineState(page, watch.at);
        const w = now.captions[0].lines[0].words[0];
        const t = await timeline(page);
        const g = await handles(page);
        const to = now.segments[0].start + w.end - arg;
        // Until the frames each word of the new captions is lit on have
        // come, as they have long before a hand gets to a block. Until
        // then a click goes by the words' own times, as it did before.
        // The drag asks for them for every track it shows on the way, and
        // the first answer was taken for the saved track's: on a busy
        // runner a click then landed on a frame lighting no word. So
        // until every ask is answered and none follows for a moment.
        const words = (r) => r.url().includes("/frames/captions/words");
        let asking = 0;
        let answered = Date.now();
        const asked = (r) => words(r) && asking++;
        const came = (r) => {
          if (!words(r)) return;
          asking--;
          answered = Date.now();
        };
        page.on("request", asked);
        page.on("requestfinished", came);
        page.on("requestfailed", came);
        try {
          const framed = page.waitForResponse(words, { timeout: 15000 });
          await drag(page, middle(g.start), high(g.track), (await xOf(page, to)) - (await xOf(page, t.start)), false);
          await watch.step("trim", s);
          await framed;
          const until = Date.now() + 10000;
          while ((asking > 0 || Date.now() - answered < 400) && Date.now() < until) await page.waitForTimeout(50);
        } finally {
          page.off("request", asked);
          page.off("requestfinished", came);
          page.off("requestfailed", came);
        }
        break;
      }
      case "every block": {
        // Each caption block clicked in turn, and the caption box read for
        // the word it lights.
        const now = await engineState(page, watch.at);
        const firsts = [...new Set(await page.locator(".clip-timeline .caption").evaluateAll((b) => b.map((x) => x.dataset.first)))];
        for (const first of firsts) {
          const cue = now.captions.find((c) => String(c.first) === first);
          const words = cue?.lines.flatMap((l) => l.words) ?? [];
          await page.locator(`.clip-timeline .caption[data-first="${first}"]`).first().click();
          // Until it lights a word of this caption, since the word the last
          // click lit stays until the frame of this one is drawn. A word is
          // known by where it starts as well as by what it says: two
          // captions can share a word, and the word the click before lit
          // then passed for this caption's.
          const ours = words.map((w) => [w.text, w.start]);
          const ofOurs = (ours) => {
            const el = document.querySelector(".captions .word.now");
            const at = Number(el?.dataset.at);
            return !!el && ours.some(([text, start]) => text === el.textContent && Math.abs(start - at) < 0.0005);
          };
          await page.waitForFunction(ofOurs, ours, { polling: "raf", timeout: 3000 }).catch(() => {});
          if (!(await page.evaluate(ofOurs, ours))) {
            const lit = await page.evaluate(() => {
              const el = document.querySelector(".captions .word.now");
              return el ? `"${el.textContent}" at ${el.dataset.at}` : null;
            });
            wrong = `a click on the caption block of "${words.map((w) => `${w.text} at ${w.start}`).join(" ")}" lit ${lit ?? "no word"}`;
            break;
          }
        }
        console.log(`    ${firsts.length} blocks clicked`);
        break;
      }
      case "edge in":
      case "edge out":
      case "edge nudge":
      case "edge to word": {
        // An edge of the caption holding a word dragged: in, just over
        // the middle of the word at that edge, out, back past where that
        // word was at "start", a little in, short of any word's middle, or
        // with shift to a little past where the word after the first
        // starts, or before where the word before the last ends, nearer
        // that than any other, where it has to land.
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
        let landing = null;
        if (what === "edge in") to = edge === "end" ? (w.start + mid) / 2 : (mid + w.end) / 2;
        else if (what === "edge out") to = edge === "end" ? was.captions[i].end : was.captions[i].start;
        else if (what === "edge to word") {
          const next = edge === "end" ? words.at(-2) : words[1];
          if (!next) {
            wrong = `the caption with "${text}" has one word`;
            break;
          }
          landing = edge === "end" ? next.end : next.start;
          to = landing + (edge === "end" ? -0.04 : 0.04);
        } else to = edge === "end" ? Math.max(mid + 0.05, from - 0.2) : Math.min(mid - 0.05, from + 0.2);
        const label = `When caption ${i + 1} ${edge === "end" ? "goes" : "appears"}`;
        const box = await page.locator(`.clip-timeline [aria-label="${label}"]`).first().boundingBox();
        if (!box) {
          wrong = `there is no edge "${label}"`;
          break;
        }
        // From the middle of the edge's handle, as far as from the edge to
        // the moment, since the timeline reads how far the hand went from
        // the press. Held there a moment before it is let go, so what the
        // timeline draws under the hand can be held against what it saves:
        // the edge is where the drag showed it, and does not jump when let
        // go.
        const x = box.x + box.width / 2;
        const y = box.y + box.height / 2;
        const target = x + (await xOf(page, into(to))) - (await xOf(page, into(from)));
        if (landing !== null) await page.keyboard.down("Shift");
        await page.mouse.move(x, y);
        await page.mouse.down();
        for (let k = 1; k <= 6; k++) {
          await page.mouse.move(x + ((target - x) * k) / 6, y);
          await nextPaint(page);
        }
        await page.waitForTimeout(300);
        const handle = page.locator(`.clip-timeline [aria-label="${label}"]`).first();
        const held = await handle.boundingBox();
        await page.mouse.up();
        if (landing !== null) await page.keyboard.up("Shift");
        await watch.step(what, s);
        let left = null;
        for (let k = 0; k < 20; k++) {
          left = await handle.boundingBox();
          if (held && left && Math.abs(left.x - held.x) <= 1) break;
          await page.waitForTimeout(100);
        }
        if (!held || !left || Math.abs(left.x - held.x) > 1) {
          wrong = `the edge was drawn at ${held?.x.toFixed(1)} px while dragged and at ${left?.x.toFixed(1)} px once let go`;
          break;
        }
        if (landing !== null) {
          const after = await engineState(page, watch.at);
          const c = after.captions[i];
          const landed = edge === "end" ? c?.end : c?.start;
          if (landed === undefined || Math.abs(landed - landing) > 0.002) {
            wrong = `dragged with shift to ${to.toFixed(3)}, the ${edge} of the caption is at ${landed?.toFixed(3)}, not on the word at ${landing.toFixed(3)}`;
          }
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
            window.__pillPops = 0;
            if (!window.__popsHeard) {
              window.__popsHeard = true;
              document.addEventListener("animationstart", (e) => {
                if (e.animationName !== "pop") return;
                if (e.target.closest?.(".clip-timeline .caption")) window.__pops++;
                if (e.target.closest?.(".screen .captions")) window.__pillPops++;
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
      case "pill pops": {
        await page.waitForTimeout(400);
        const n = await page.evaluate(() => window.__pillPops);
        if (n !== arg) wrong = `the pill in the video preview popped ${n} times, ${arg} wanted`;
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
      case "holds": {
        const now = await engineState(page, watch.at);
        if (!now.captions.some((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(arg))))) {
          wrong = `no caption has "${arg}"\n${describe(marks.start ?? now, now)}`;
        }
        break;
      }
      case "clips with": {
        const [word, n] = arg;
        const path = await episodeOn(page);
        const bare = (text) => text.replace(/[^\p{L}\p{N}]/gu, "");
        let have = 0;
        for (const c of (await ask(page, "Clips", path)).filter((c) => !c.rejected)) {
          const view = await ask(page, "Captions", path, c.plan, c.id);
          if (view.captions.some((cap) => cap.lines.some((l) => l.words.some((w) => bare(w.text) === word)))) have++;
        }
        if (have !== n) wrong = `${have} clips have "${word}", not ${n}`;
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
      case "open in place": {
        // The words where they stand in the caption box, and the white of
        // the letters libass draws on the canvas inside the word and round
        // it, with the word shut, open, and shut again.
        const look = () =>
          page.evaluate((text) => {
            const words = [...document.querySelectorAll(".captions .word")];
            const canvas = document.querySelector(".screen canvas");
            const it = words.find((w) => w.textContent === text);
            if (!it || !canvas?.width) return null;
            const c = canvas.getBoundingClientRect();
            const sx = canvas.width / c.width;
            const sy = canvas.height / c.height;
            const s = document.createElement("canvas");
            s.width = canvas.width;
            s.height = canvas.height;
            const g = s.getContext("2d", { willReadFrequently: true });
            g.drawImage(canvas, 0, 0);
            const f = it.getBoundingClientRect();
            const x0 = Math.floor((f.left - c.left) * sx);
            const x1 = Math.ceil((f.right - c.left) * sx);
            const y0 = Math.floor((f.top - c.top) * sy);
            const y1 = Math.ceil((f.bottom - c.top) * sy);
            // Near white, the letters, inside the word and in a band as
            // wide again on each side of it.
            const white = (a, b) => {
              const d = g.getImageData(a, y0, Math.max(1, b - a), Math.max(1, y1 - y0)).data;
              let n = 0;
              for (let i = 0; i < d.length; i += 4) if (d[i] > 220 && d[i + 1] > 220 && d[i + 2] > 220) n++;
              return n;
            };
            const w = x1 - x0;
            return {
              rects: words.map((e) => {
                const r = e.getBoundingClientRect();
                return [r.left, r.top, r.width, r.height].map((v) => Math.round(v * 10) / 10);
              }),
              inside: white(x0, x1),
              around: white(Math.max(0, x0 - w), x0) + white(x1, Math.min(canvas.width, x1 + w)),
              popped: window.__captionLayers?.stats.popped ?? 0,
            };
          }, arg);
        await settled();
        const shut = await look();
        if (!shut) {
          wrong = `"${arg}" is not in the caption box`;
          break;
        }
        // The rows of the word's letters on screen, libass's while it is
        // shut and the browser's while it is open, without the caret.
        const b = await word(arg).boundingBox();
        const place = b && { left: b.x, top: b.y, width: b.width, height: b.height };
        const inked = async () => {
          await word(arg).evaluate((e) => (e.style.caretColor = "transparent"));
          const got = await letters(place);
          await word(arg).evaluate((e) => (e.style.caretColor = ""));
          return got;
        };
        const drawn = await inked();
        await word(arg).click();
        await watch.step("click", s);
        await settled();
        const open = await look();
        const typed = await inked();
        console.log(`    letters from row ${drawn?.top} to ${drawn?.bottom} shut, ${typed?.top} to ${typed?.bottom} open`);
        await page.keyboard.press("Escape");
        await settled();
        const again = await look();
        const moved = (a, b) => a.rects.findIndex((r, i) => r.some((v, k) => Math.abs(v - (b.rects[i]?.[k] ?? NaN)) > 0.6));
        console.log(`    shut ${shut.inside} white inside, ${shut.around} round, open ${open?.inside} and ${open?.around}`);
        if (!open || !again) wrong = `"${arg}" left the caption box`;
        else if (moved(shut, open) >= 0) wrong = `opening "${arg}" moved word ${moved(shut, open) + 1} of the caption box from ${shut.rects[moved(shut, open)]} to ${open.rects[moved(shut, open)]}`;
        else if (moved(shut, again) >= 0) wrong = `leaving "${arg}" moved word ${moved(shut, again) + 1} of the caption box`;
        else if (shut.inside < 20 || open.inside > shut.inside / 10)
          wrong = `libass drew ${shut.inside} white pixels of "${arg}" shut and ${open.inside} open, where the caption box draws it`;
        else if (Math.abs(open.around - shut.around) > shut.around / 20 + 5)
          wrong = `round "${arg}" libass drew ${shut.around} white pixels shut and ${open.around} open: the caption moved`;
        else if (again.popped !== shut.popped) wrong = `leaving "${arg}" popped the word lit`;
        // Found by Tim on #201: the word opened dropped a few pixels,
        // set in the middle of its pill rather than on libass's baseline.
        else if (!drawn || !typed) wrong = `the letters of "${arg}" were not found on screen`;
        else if (Math.abs(typed.top - drawn.top) > 1 || Math.abs(typed.bottom - drawn.bottom) > 1)
          wrong = `"${arg}" opened stands from row ${typed.top} to ${typed.bottom}, where libass drew it from ${drawn.top} to ${drawn.bottom}`;
        break;
      }
      case "one pill": {
        // The colour of the pill libass draws round the word lit, and the
        // colour behind its letters opened while the next word is lit.
        const it = word(arg);
        for (let i = 0; i < 20 && !(await it.evaluate((e) => e.classList.contains("now"))); i++) {
          await page.keyboard.press("Shift+ArrowRight");
          await settled();
        }
        await watch.step("press", s);
        await settled();
        const box = async () => {
          const r = await it.boundingBox();
          return r && { left: r.x + 1, top: r.y + 1, width: r.width - 2, height: r.height - 2 };
        };
        const shut = await box();
        const lit = shut && (await letters(shut));
        await page.keyboard.press("Shift+ArrowRight");
        await watch.step("press", s);
        await settled();
        await it.click();
        await watch.step("click", s);
        await settled();
        await it.evaluate((e) => (e.style.caretColor = "transparent"));
        const open = await box();
        // The whole box, its edge where a ring would be drawn included.
        const typed = open && (await letters({ ...open, left: open.left - 1, top: open.top - 1, width: open.width + 2, height: open.height + 2 }));
        const state = await it.evaluate((e) => ({
          open: e.classList.contains("fixing"),
          lit: e.classList.contains("now"),
          shadow: getComputedStyle(e).boxShadow,
          outline: `${getComputedStyle(e).outlineStyle} ${getComputedStyle(e).outlineWidth}`,
        }));
        await it.evaluate((e) => (e.style.caretColor = ""));
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        console.log(`    lit on ${lit?.pill}, typed unlit on ${typed?.pill}, ring ${state.shadow}, outline ${state.outline}`);
        const rgb = (c) => (c ?? "").split(",").map(Number);
        const unlike = (a, b) => rgb(a).length !== 3 || rgb(a).some((v, i) => Math.abs(v - rgb(b)[i]) > 4);
        if (!lit?.pill) wrong = `libass drew no pill round "${arg}" lit`;
        else if (!state.open || state.lit) wrong = `"${arg}" is ${state.open ? "" : "not "}open and ${state.lit ? "" : "not "}lit`;
        else if (!typed) wrong = `the letters typed into "${arg}" were not found on screen`;
        else if (unlike(lit.pill, typed.pill))
          wrong = `"${arg}" typed unlit sits on ${typed.pill || "no colour"}, and libass's pill is ${lit.pill}`;
        else if (/rgb|color\(/.test(state.shadow.replaceAll("rgba(0, 0, 0, 0)", "")))
          wrong = `"${arg}" typed has a ring round its pill, ${state.shadow}`;
        // The focus ring the app gives a control, a pixel off the pill,
        // which left a dark line between the two.
        else if (!state.outline.startsWith("none")) wrong = `"${arg}" typed has a focus ring round its pill, ${state.outline}`;
        break;
      }
      case "grow": {
        // The word as the screen shows it: its box, the scale it is
        // drawn at, whether it is open and lit, the width of its letters
        // and the colour behind them.
        const look = () =>
          page.evaluate(() => {
            const it = document.querySelector(".captions .word.fixing") ?? window.__growing;
            if (!it?.isConnected) return null;
            const r = it.getBoundingClientRect();
            const letters = document.createRange();
            letters.selectNodeContents(it);
            const style = getComputedStyle(it);
            return {
              left: r.left,
              width: r.width,
              top: r.top,
              height: r.height,
              letters: letters.getBoundingClientRect().width,
              pop: style.getPropertyValue("--pop") || getComputedStyle(it.closest(".captions")).getPropertyValue("--pop"),
              open: it.classList.contains("fixing"),
              lit: it.classList.contains("now"),
              behind: style.backgroundColor,
            };
          });
        const it = word(arg);
        for (let i = 0; i < 20 && !(await it.evaluate((e) => e.classList.contains("now"))); i++) {
          await page.keyboard.press("Shift+ArrowRight");
          await settled();
        }
        await page.keyboard.press("ArrowRight");
        await watch.step("press", s);
        await it.evaluate((e) => (window.__growing = e));
        await settled();
        const shut = await look();
        // Inside the pill, which lies over the words beside it.
        const around = shut && { left: shut.left + 1, top: shut.top + 1, width: shut.width - 2, height: shut.height - 2 };
        const drawn = around && (await letters(around));
        await it.click();
        await watch.step("click", s);
        await settled();
        const open = await look();
        // Without the caret, which is the colour of the letters and would
        // count as one.
        await it.evaluate((e) => (e.style.caretColor = "transparent"));
        const typing = around && (await letters(around));
        await it.evaluate((e) => (e.style.caretColor = ""));
        console.log(`    libass's letters ${JSON.stringify(drawn)}, the caption box's ${JSON.stringify(typing)}`);
        const rgb = (c) => (c ?? "").split(",").map(Number);
        const unlike = (a, b) => rgb(a).length !== 3 || rgb(a).some((v, i) => Math.abs(v - rgb(b)[i]) > 4);
        await page.keyboard.press("End");
        await page.keyboard.type("ungen");
        await watch.step("type", s);
        await settled();
        const typed = await look();
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        const mid = (l) => l.left + l.width / 2;
        const off = (a, b) => ["left", "width", "top", "height"].find((k) => Math.abs(a[k] - b[k]) > 0.6);
        console.log(
          `    scale ${shut?.pop}, ${shut?.lit ? "lit" : "not lit"}, ${shut?.width.toFixed(1)} px shut, ${open?.width.toFixed(1)} open, ${typed?.width.toFixed(1)} typed round ${typed?.letters.toFixed(1)} px of letters`,
        );
        if (!shut || !open || !typed) wrong = `"${arg}" left the caption box`;
        else if (!shut.lit || !(Number(shut.pop) > 1)) wrong = `"${arg}" is not lit on a frame of its pop, scale ${shut.pop}`;
        else if (!open.open) wrong = `"${arg}" did not open`;
        else if (!drawn || !typing) wrong = `"${arg}" shows no letters ${drawn ? "open" : "shut"}`
        // The Mac thickened the face in the caption box where libass did not,
        // and the box was drawn without the look the frame has.
        else if (Math.abs(drawn.width - typing.width) > 2 || Math.abs(drawn.height - typing.height) > 1)
          wrong = `"${arg}" opened is ${typing.width} by ${typing.height} px of letters and ${drawn.width} by ${drawn.height} shut`;
        else if (unlike(drawn.pill, typing.pill))
          wrong = `"${arg}" opened sits on a pill of ${typing.pill} and the pill shut is ${drawn.pill}`;
        else if (unlike(drawn.ink, typing.ink))
          wrong = `"${arg}" opened is written in ${typing.ink} and shut in ${drawn.ink}`;
        else if (off(shut, open))
          wrong = `opening "${arg}" moved its ${off(shut, open)} from ${shut[off(shut, open)].toFixed(1)} to ${open[off(shut, open)].toFixed(1)} px`;
        else if (typed.width < open.width + (typed.letters - open.letters) - 1.5)
          wrong = `"${arg}" typed longer by ${(typed.letters - open.letters).toFixed(1)} px of letters grew its box by ${(typed.width - open.width).toFixed(1)} px`;
        else if (typed.letters > typed.width) wrong = `the letters typed run out of their box, ${typed.letters.toFixed(1)} px in ${typed.width.toFixed(1)}`;
        else if (Math.abs(mid(typed) - mid(open)) > 0.6)
          wrong = `"${arg}" grew round ${mid(typed).toFixed(1)} px and not round the middle of its pill at ${mid(open).toFixed(1)}`;
        else if (Math.abs(typed.top - open.top) > 0.6 || Math.abs(typed.height - open.height) > 0.6)
          wrong = `typing into "${arg}" moved it up or down`;
        else if (typed.lit && /rgba\(0, 0, 0, 0\)|transparent/.test(typed.behind))
          wrong = `"${arg}" is the word lit and has no pill behind what is typed`;
        break;
      }
      case "button": {
        const button = page.getByRole("button", { name: arg, exact: true });
        await button.click();
        await page.waitForFunction(
          (label) => ![...document.querySelectorAll("button")].some((b) => b.textContent.trim() === label && b.disabled),
          arg,
        );
        break;
      }
      case "read ahead": {
        await page.waitForTimeout(1500);
        const came = await page.evaluate(async (key) => {
          const frames = window.__appFrames;
          const before = Number(document.querySelector(".screen")?.dataset.playhead);
          window.dispatchEvent(new KeyboardEvent("keydown", { key: key.replace("Shift+", ""), shiftKey: key.startsWith("Shift+"), bubbles: true }));
          // How the frames came by the time the video preview shows the
          // frame that holds the playhead, its first frame of the word.
          return new Promise((done) => {
            const t0 = performance.now();
            const check = () => {
              const el = document.querySelector(".screen");
              const at = Number(el?.dataset.playhead);
              const shown = Number(el?.dataset.shown);
              const holds = at !== before && shown <= at + 1e-6 && at - shown < frames.track.frame - 1e-6;
              // The rank of the frame on screen, and whether it was one
              // handed over from the frames kept.
              let rank = 0;
              while (rank < frames.track.samples.count - 1 && frames.ptsOf(rank + 1) <= shown + 1e-6) rank++;
              const fromKept = frames.stats.keptRanks.includes(rank);
              if (holds) done({ at, fromKept, ms: Math.round(performance.now() - t0) });
              else if (performance.now() - t0 > 5000) done({ at, fromKept, ms: -1 });
              else requestAnimationFrame(check);
            };
            check();
          });
        }, arg);
        // The frames after it, which the step's cue reads ahead, and the
        // frame of the word after the one it lands on, which is read ahead
        // now, may come from a stream. The frame it lands on may not.
        if (came.ms < 0) wrong = `${arg} to ${came.at.toFixed(3)} s showed no frame of it in five seconds`;
        else if (!came.fromKept)
          wrong = `${arg} to ${came.at.toFixed(3)} s had to fetch its frame, ${came.ms} ms, though the hand rested a second and a half before it`;
        await watch.step("press", s);
        break;
      }
      case "heard": {
        await page.waitForTimeout(1500);
        // What the sound card is handed, a render quantum at a time, the
        // way "loop off near the end" records it.
        await page.evaluate(async () => {
          const q = window.__frameQueue;
          if (!window.__loudness) {
            const code = `registerProcessor("loudness", class extends AudioWorkletProcessor {
              process(inputs) {
                const ch = inputs[0][0];
                if (ch) { let sum = 0; for (const v of ch) sum += v * v; this.port.postMessage(sum / ch.length); }
                return true;
              }
            });`;
            await q.audio.audioWorklet.addModule(URL.createObjectURL(new Blob([code], { type: "text/javascript" })));
            const node = new AudioWorkletNode(q.audio, "loudness", { numberOfOutputs: 1 });
            node.port.onmessage = (e) => window.__loudness.push(e.data);
            const hush = q.audio.createGain();
            hush.gain.value = 0;
            q.out.connect(node).connect(hush).connect(q.audio.destination);
            // Held, or the recorder is collected while it records.
            window.__loudnessNodes = [node, hush];
          }
          window.__loudness = [];
        });
        await page.keyboard.press("Space");
        await page.waitForFunction(() => window.__frameQueue.state === "playing", null, { timeout: 5000 }).catch(() => {});
        await page.evaluate(() => (window.__loudness = []));
        await page.waitForTimeout(1500);
        const got = await page.evaluate(() => {
          const xs = window.__loudness;
          return { quanta: xs.length, loud: xs.filter((x) => x > 1e-6).length, state: window.__frameQueue.state };
        });
        await page.keyboard.press("Space");
        await page.waitForTimeout(300);
        // A second and a half is about 560 quanta at 48 kHz. The test
        // video's sound is a tone from its first moment to its last.
        if (got.state !== "playing") wrong = `the space bar left the play ${got.state}`;
        else if (got.loud < got.quanta * 0.8 || got.quanta < 200)
          wrong = `the sound card was handed sound in ${got.loud} of ${got.quanta} render quanta of a second and a half of play`;
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
        // What is heard is held back from the window from the reading of
        // the word to the click, see __pauseEvents in wails-bridge.ts. The
        // stand-in speech model hears new words each time, and on CI
        // "erzählt," was read and "er wäre" clicked, which put the X
        // after "er", in one run of three on a busy machine.
        await page.evaluate(() => window.__pauseEvents(true));
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
          await page.evaluate(() => window.__pauseEvents(false));
          wrong = "nothing was being heard to click into";
          break;
        }
        await page.mouse.click(at.x, at.y);
        // Read before what waited is let through, which may be heard
        // words that are new.
        const where = await page.evaluate(() => {
          const sel = getSelection();
          const mark = document.querySelector(".telling .hearing");
          const where = {
            heard: mark?.textContent.trim() ?? "",
            inMark: !!mark?.contains(sel.anchorNode),
            inSaid: !!document.querySelector(".telling .dictated")?.contains(sel.anchorNode),
          };
          window.__pauseEvents(false);
          return where;
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
      case "escape lets go": {
        const control = page.locator(arg).first();
        await control.click();
        await settle(page);
        await page.keyboard.press("Escape");
        await settle(page);
        const after = await control.evaluate((el) => ({
          open: document.querySelectorAll(".pick-list, .colour-pop").length,
          kept: document.activeElement === el,
          lit: [...document.querySelectorAll(":focus-visible")].map((x) => x.id || x.getAttribute("aria-label") || x.className || x.tagName),
        }));
        if (after.open) wrong = `${arg}: a list or pop-up is still open after Escape`;
        else if (after.kept) wrong = `${arg} still has the keyboard after Escape`;
        else if (after.lit.length) wrong = `${arg}: a ring is left on ${after.lit.join(", ")} after Escape`;
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
      case "cut on caption": {
        const box = await page.locator(".clip-timeline .caption").nth(arg).boundingBox();
        if (!box) {
          wrong = `there is no caption ${arg + 1} to double-click`;
          break;
        }
        await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
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
      case "drag edge": {
        const [which, px, how] = arg;
        dragShift = how === "shift";
        dragPx = px;
        const g = await handles(page);
        const box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[which];
        if (!box) {
          wrong = `there is no ${which} edge to drag`;
          break;
        }
        let x = middle(box);
        if (how === "at the playhead") {
          // The playhead's grip is its line and four pixels either side.
          const line = await page.evaluate(() => {
            const b = document.querySelector(".clip-timeline .playhead").getBoundingClientRect();
            return b.left + b.width / 2;
          });
          const from = Math.max(line - 4, box.x + 1);
          const to = Math.min(line + 4, box.x + box.w - 1);
          if (!(to > from)) {
            wrong = `the playhead's line at ${line.toFixed(1)} is not on the ${which} edge's handle, ${box.x.toFixed(1)} to ${(box.x + box.w).toFixed(1)}`;
            break;
          }
          x = (from + to) / 2;
        }
        await watchEdge(page, edgeLabel[which]);
        const y = box.y + box.h / 2;
        await page.mouse.move(x, y);
        if (dragShift) await page.keyboard.down("Shift");
        await page.mouse.down();
        for (let i = 1; i <= 20; i++) {
          await page.mouse.move(x + (px * i) / 20, y);
          await nextPaint(page);
        }
        await page.mouse.up();
        if (dragShift) await page.keyboard.up("Shift");
        await watch.step("drag edge", s);
        await page.evaluate(() => (window.__followWatch = false));
        break;
      }
      case "hold on": {
        // The playhead read on every animation frame from the press, for a
        // press on the track first and then for the thing held, so "at
        // once" is a number: no more frames than the track takes.
        //
        // The frames are counted from the press as the page has it, not
        // from when the watch began. Moving the pointer and pressing it are
        // two calls to the browser, and on a busy runner either can take a
        // frame or two to arrive, so counted from the watch the same app
        // took 2 frames for the track and 4 for the mark, at random.
        const at = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
        const watchFrames = () =>
          page.evaluate(() => {
            window.__pressed = [];
            window.__pressedAt = -1;
            window.__pressWatch = true;
            addEventListener("pointerdown", () => (window.__pressedAt = window.__pressed.length), {
              capture: true,
              once: true,
            });
            const tick = () => {
              if (!window.__pressWatch) return;
              window.__pressed.push(Number(document.querySelector(".screen")?.dataset.playhead));
              requestAnimationFrame(tick);
            };
            requestAnimationFrame(tick);
          });
        // The frames from the press on, the first of them the first painted
        // after it.
        const seen = () =>
          page.evaluate(() => {
            window.__pressWatch = false;
            return window.__pressedAt < 0 ? [] : window.__pressed.slice(window.__pressedAt);
          });
        const g = await handles(page);
        const caption = (edge) =>
          page.locator(`.clip-timeline [aria-label="When caption 2 ${edge === "end" ? "goes" : "appears"}"]`).first().boundingBox();
        let box;
        if (arg === "thumbnail" || arg === "crop") {
          box = await page.locator(`.clip-timeline .mark${arg === "crop" ? ".cropdot" : ".thumb"}`).first().boundingBox();
        } else if (arg === "caption start" || arg === "caption end") {
          box = await caption(arg === "caption end" ? "end" : "start");
        } else {
          box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[arg];
        }
        if (!box) {
          wrong = `there is no ${arg} to press`;
          break;
        }
        // The track, near the end away from what is held, so the press on
        // it moves the playhead off the frame it is then to come back to.
        const left = box.x + (box.width ?? box.w) / 2 > g.track.x + g.track.w / 2;
        const trackBefore = await at();
        await watchFrames();
        await page.mouse.move(left ? g.track.x + 8 : g.track.x + g.track.w - 8, high(g.track));
        await page.mouse.down();
        await page.waitForTimeout(150);
        const track = await seen();
        await page.mouse.up();
        await watch.step("hold on", s);
        const trackFrames = track.findIndex((t) => Math.abs(t - trackBefore) > 1e-6);
        const before = await at();
        await watchFrames();
        await page.mouse.move(box.x + box.width / 2 || middle(box), box.y + (box.height ?? box.h) / 2);
        await page.mouse.down();
        await page.waitForTimeout(150);
        held = { which: arg, before, frames: await seen(), trackFrames, at: await at() };
        await page.mouse.up();
        await watch.step("hold on", s);
        break;
      }
      case "on press": {
        const frame = 1 / (await rate(page));
        const now = await engineState(page, watch.at);
        const segs = now.segments;
        // A moment on the clip's clock in the episode, the way the clip
        // timeline reads it, a start where two pieces meet.
        const into = (t, closes) => {
          let sum = 0;
          for (const p of segs) {
            const span = p.end - p.start;
            if (closes ? t <= sum + span + 1e-6 : t < sum + span - 1e-6) return p.start + t - sum;
            sum += span;
          }
          return segs.at(-1).end;
        };
        let want;
        if (arg === "thumbnail" || arg === "crop") {
          want = await page.evaluate(
            (kind) => Number(document.querySelector(`.clip-timeline .mark${kind}`)?.getAttribute("aria-valuenow")),
            arg === "crop" ? ".cropdot" : ".thumb",
          );
        } else if (arg === "caption start") {
          want = into(now.captions[1].start, false);
        } else if (arg === "caption end") {
          want = into(now.captions[1].end, true) - frame;
        } else {
          want = { start: segs[0].start, end: segs.at(-1).end - frame, "cut from": segs[0].end - frame, "cut to": segs[1]?.start }[arg];
        }
        const same = (a, b) => Math.floor(a / frame + 0.001 / frame) === Math.floor(b / frame + 0.001 / frame);
        const on = held?.frames?.findIndex((t) => same(t, want)) ?? -1;
        if (held?.which !== arg) wrong = `${arg} was not held`;
        else if (same(held.before, want)) wrong = `the playhead stood on the ${arg}'s frame before the press, which proves nothing`;
        else if (!same(held.at, want)) {
          wrong = `the playhead stood at ${held.at.toFixed(3)} with the button down on the ${arg}, not on its frame at ${want.toFixed(3)}`;
        } else if (held.trackFrames < 0) wrong = "a press on the track did not move the playhead";
        else if (on < 0) wrong = `the playhead came onto the ${arg} only after 150 ms`;
        else if (on > held.trackFrames) {
          wrong = `the playhead came onto the ${arg} ${on} frames after the press, a press on the track takes ${held.trackFrames}`;
        }
        break;
      }
      case "went the hand's way": {
        const seen = await page.evaluate(() => window.__edgeSeen ?? []);
        const back = seen.find((v) => (v - seen[0]) * dragPx < -0.0005);
        if (!seen.length) wrong = `the ${arg} edge was not seen`;
        else if (back !== undefined) {
          wrong = `the ${arg} edge went from ${seen[0].toFixed(3)} to ${back.toFixed(3)}, against the hand, which went ${dragPx > 0 ? "right" : "left"}`;
        }
        break;
      }
      case "moved": {
        const frame = 1 / (await rate(page));
        const seen = await page.evaluate(() => window.__edgeSeen ?? []);
        if (!(seen.length && Math.abs(seen.at(-1) - seen[0]) >= frame - 0.001)) {
          wrong = `the ${arg} edge stayed at ${seen[0]?.toFixed(3)} while the hand dragged it`;
        }
        break;
      }
      case "kept up": {
        const seen = await page.evaluate(() => window.__edgeSeen ?? []);
        // The two drags are saved in the order they were let go, so the
        // engine has the edge where the hand left it the second time, the
        // farthest it went the hand's way, once both saves are done.
        await settle(page);
        const segs = (await engineState(page, watch.at)).segments;
        const saved = { start: segs[0].start, end: segs.at(-1).end, "cut from": segs[0].end, "cut to": segs[1]?.start }[arg];
        const left = dragPx < 0 ? Math.min(...seen) : Math.max(...seen);
        if (!movedHeld) wrong = `the ${arg} edge stood where the drag before let it go while the hand took it again, it did not go with the hand`;
        else if (!(Math.abs(saved - left) < 0.002)) {
          wrong = `the hand left the ${arg} edge at ${left.toFixed(3)} and it was saved at ${saved?.toFixed(3)}`;
        }
        break;
      }
      case "drag edge twice": {
        // Dragged, let go, and taken again at once where it was let go, with
        // nothing waited for in between, while the first is being saved.
        const [which, px, again, late] = arg;
        dragShift = false;
        dragPx = again;
        pressedOn = "";
        const g = await handles(page);
        const box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[which];
        if (!box) {
          wrong = `there is no ${which} edge to drag`;
          break;
        }
        const y = box.y + box.h / 2;
        const x = middle(box);
        await page.mouse.move(x, y);
        await page.mouse.down();
        for (let i = 1; i <= 10; i++) {
          await page.mouse.move(x + (px * i) / 10, y);
          await nextPaint(page);
        }
        // Every place so far answered, and the last one, ten pixels on,
        // which is more than a frame at this zoom, held on its way.
        const took = late ? x + px + Math.sign(px) * 10 : x + px;
        if (late) {
          await page.waitForFunction(() => window.__pending === 0, null, { polling: "raf" });
          await control(url, `/hold?call=Shape&ms=${late}`);
          await page.mouse.move(took, y);
        }
        await page.mouse.up();
        await watchEdge(page, edgeLabel[which]);
        pressedOn = await page.evaluate(
          ([x, y]) =>
            [...new Set(document.elementsFromPoint(x, y).map((el) => el.closest("[data-grip]")?.dataset.grip).filter(Boolean))].join(", ") || "no grip",
          [took, y],
        );
        await page.mouse.down();
        for (let i = 1; i <= 10; i++) {
          await page.mouse.move(took + (again * i) / 10, y);
          await nextPaint(page);
        }
        // The hand stays down until the edge has left where the first drag
        // let it go, for up to two seconds, so a busy machine slow to
        // answer the drag is waited for and an edge that waits for the
        // save before it is seen standing.
        movedHeld = await page
          .waitForFunction(() => new Set(window.__edgeSeen.map((t) => t.toFixed(3))).size > 1, null, { timeout: 2000, polling: "raf" })
          .then(() => true, () => false);
        await page.mouse.up();
        await watch.step("drag edge twice", s);
        await page.evaluate(() => (window.__followWatch = false));
        break;
      }
      case "edge then track": {
        // A click on an edge, and at once a press on the track: the
        // playhead stays where the track was pressed.
        const g = await handles(page);
        const box = { start: g.start, end: g.end, "cut from": g.cutEdges[0], "cut to": g.cutEdges[1] }[arg];
        if (!box) {
          wrong = `there is no ${arg} edge to click`;
          break;
        }
        const y = box.y + box.h / 2;
        await page.mouse.click(middle(box), y);
        const x = g.track.x + g.track.w * 0.9;
        await page.mouse.move(x, high(g.track));
        await page.mouse.down();
        await page.waitForTimeout(400);
        const at = await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
        const want = await page.evaluate((x) => {
          const b = document.querySelector(".clip-timeline .track").getBoundingClientRect();
          return x - b.left;
        }, x);
        await page.mouse.up();
        await watch.step("edge then track", s);
        const placed = await xOf(page, at);
        if (Math.abs(placed - x) > 3) {
          wrong = `the playhead stands ${(placed - x).toFixed(1)} px from where the track was pressed after a click on the ${arg}, ${want.toFixed(0)} px in`;
        }
        break;
      }
      case "block after": {
        const now = await engineState(page, watch.at);
        // Where the cut ends, on the clip's clock, which the captions are on.
        const back = now.segments.slice(0, arg).reduce((sum, p) => sum + p.end - p.start, 0);
        const cue = now.captions.find((c) => c.start >= back + 0.01);
        if (!cue) {
          wrong = `no caption appears after cut ${arg}`;
          break;
        }
        await page.locator(`.clip-timeline .caption[data-first="${cue.first}"]`).first().click();
        await watch.step("block after", s);
        break;
      }
      case "followed": {
        const segs = (await engineState(page, watch.at)).segments;
        const frame = 1 / (await rate(page));
        const want = {
          start: segs[0].start,
          end: segs.at(-1).end - frame,
          "cut from": segs[0].end - frame,
          "cut to": segs[1]?.start,
        }[arg];
        const at = await page.evaluate(() => Number(document.querySelector(".screen").dataset.playhead));
        // Every place the edge stood while it was watched, the playhead
        // stood on that place's frame too, the edge's own frame for "start"
        // and "cut to", the one before for "end" and "cut from", within
        // three animation frames of the edge leaving it. The two are read in
        // the same frames, so this holds on a machine of any speed. A count
        // of places did not: an edge taken again at once stands still until
        // the drag before it is saved (2.259), so on a busy runner it stood
        // in 2 or 3 places, and a playhead left behind stands in 2 as well.
        // On words the engine puts the playhead by the words, so this is
        // asked of a drag on frames.
        const frames = (await page.evaluate(() => window.__edgeFrames ?? [])).filter(([e]) => Number.isFinite(e));
        const onFrame = (a, b) => Math.floor(a / frame + 0.001 / frame) === Math.floor(b / frame + 0.001 / frame);
        const late = 3;
        const runs = [];
        frames.forEach(([e], i) => {
          if (runs.at(-1)?.edge.toFixed(3) === e.toFixed(3)) runs.at(-1).to = i;
          else runs.push({ edge: e, from: i, to: i });
        });
        const opens = arg === "start" || arg === "cut to";
        const missed = runs.find(
          (r) =>
            r.to + late < frames.length &&
            !frames.slice(r.from, r.to + late + 1).some(([, p]) => onFrame(p, opens ? r.edge : r.edge - frame)),
        );
        if (dragShift) {
          // Nothing more than where it lands, below.
        } else if (runs.length < 2) {
          wrong = `the ${arg} edge stood in ${runs.length ? "one place" : "no place"} while the hand dragged it, which proves nothing`;
        } else if (missed) {
          const went = frames.slice(missed.from, missed.to + late + 1).map(([, p]) => p.toFixed(3));
          // Every edge and playhead the watch saw, and what the press
          // landed on, since a runner saw this and this machine does not.
          const seen = frames.map(([e, p]) => `${e.toFixed(3)}/${p.toFixed(3)}`).filter((v, k, all) => v !== all[k - 1]);
          wrong = `the ${arg} edge stood at ${missed.edge.toFixed(3)} and the playhead did not come onto its frame, it stood at ${[...new Set(went)].join(", ")}.${pressedOn ? ` The press that took it again landed on ${pressedOn}.` : ""} Edge/playhead went ${seen.join(" ")}`;
        } else if (!(Math.abs(at - want) < frame / 2 + 0.001)) {
          wrong = `the playhead stands at ${at.toFixed(3)} after the ${arg} edge was dragged, not on its frame at ${want?.toFixed(3)}`;
        }
        break;
      }
      case "nothing past the cut": {
        // While the cut's left edge was dragged, the video preview showed
        // the frames before the cut, the edge's, and never one after it:
        // the playhead stood before the cut all along.
        const segs = (await engineState(page, watch.at)).segments;
        const after = segs[1].start;
        const frames = await page.evaluate(() => window.__edgeFrames ?? []);
        const past = frames.filter(([, p, f]) => p < after && f >= after - 0.001);
        if (frames.length < 10) wrong = `the drag was watched for ${frames.length} frames, which proves nothing`;
        else if (past.length) {
          const seen = [...new Set(past.map(([, p, f]) => `${f.toFixed(3)} with the playhead at ${p.toFixed(3)}`))];
          wrong = `while the cut's left edge was dragged, the video preview showed frames from after the cut, which starts again at ${after.toFixed(3)}: ${seen.slice(0, 6).join(", ")}`;
        }
        break;
      }
      case "drag while playing": {
        const [edge, px] = arg;
        // The play starts a moment into the clip, so however slowly a busy
        // machine moves the hand, the edge it drags stays far ahead of the
        // playhead. Played on from wherever the step before left it, the
        // play came within a frame of the clip's end while the hand was
        // still on its way, the end was dragged back over the playhead, and
        // the playhead went back with it, which is right and not what this
        // step is about.
        const g0 = await handles(page);
        const into = g0.start.x + g0.start.w;
        await page.mouse.click(into + 0.05 * (g0.end.x - into), high(g0.track));
        await settle(page);
        await page.evaluate(() => document.body.focus());
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
          await nextPaint(page);
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
      case "loop through the end": {
        // Played with the loop switched on early, before the play works
        // out the next time through, and left to run on through the
        // clip's end into its start. Tim saw the sound go round while the
        // video preview stood on the clip's last frame (2.264).
        const segs = (await engineState(page, watch.at)).segments;
        const start = segs[0].start;
        const end = segs.at(-1).end;
        const fps = await rate(page);
        await seekTo(page, end - arg, fps);
        await preview(page, 1 / fps);
        if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
          await page.keyboard.press("Space");
        }
        await page.waitForTimeout(300);
        await page.keyboard.press("l");
        await page.evaluate(() => {
          window.__played = [];
          window.__playWatch = true;
          const tick = () => {
            if (!window.__playWatch) return;
            const at = Number(document.querySelector(".screen")?.dataset.playhead);
            window.__played.push([performance.now(), at, window.__pictured()]);
            requestAnimationFrame(tick);
          };
          requestAnimationFrame(tick);
        });
        // Round the end and three seconds into the next time through.
        const wrapped = await page
          .waitForFunction(
            ([start, end]) => {
              const p = window.__played;
              const i = p.findIndex(([, at]) => at > end - 1);
              return i >= 0 && p.slice(i).some(([, at]) => at > start + 3 && at < end - 2);
            },
            [start, end],
            { timeout: (arg + 10) * 1000, polling: 100 },
          )
          .then(() => true, () => false);
        const played = await page.evaluate(() => {
          window.__playWatch = false;
          return window.__played;
        });
        await page.keyboard.press("Space");
        await page.keyboard.press("l");
        await settle(page);
        if (!wrapped) {
          wrong = `the playhead did not go round the clip's end, it got to ${Math.max(...played.map(([, at]) => at)).toFixed(3)}`;
          break;
        }
        // From half a second after the playhead came round, the picture
        // shows the frame that holds the playhead, give or take two.
        const i = played.findIndex(([, at]) => at > end - 1);
        const round = i + played.slice(i).findIndex(([, at]) => at < end - 2);
        const after = played.slice(round).filter(([t]) => t > played[round][0] + 500);
        const off = after.find(([, at, n]) => !(n >= 0) || Math.abs(n / fps - at) > 2.5 / fps);
        if (off) {
          wrong = `${(off[0] - played[round][0]).toFixed(0)} ms after the playhead came round to ${played[round][1].toFixed(3)}, it stood at ${off[1].toFixed(3)} and the video preview showed ${off[2] >= 0 ? (off[2] / fps).toFixed(3) : "nothing it could read"}`;
        }
        break;
      }
      case "loop off as the sound crosses the end": {
        // The playhead moves on with the page's frames, and the sound card
        // plays on its own clock. A busy machine holds the page for a
        // moment as the sound crosses the clip's end, and L comes in that
        // moment, before the playhead has caught up. The loop off ends the
        // play at the end of the time through the sound is in, never at an
        // end the sound card has already played past.
        const end = (await engineState(page, watch.at)).segments.at(-1).end;
        const g = await handles(page);
        await page.mouse.click(middle(g.end) - 80, high(g.track));
        await settle(page);
        if (!(await page.evaluate(() => !!document.querySelector('button[aria-label="Pause"]')))) {
          await page.keyboard.press("Space");
        }
        await page.waitForTimeout(300);
        await page.keyboard.press("l");
        await page
          .waitForFunction((to) => Number(document.querySelector(".screen").dataset.playhead) > to, end - 0.03, { timeout: 30000, polling: "raf" })
          .catch(() => {});
        const r = await page.evaluate(async () => {
          const q = window.__frameQueue;
          const sounded = () => q.m0 / q.sampleRate + (q.audio.currentTime - q.c0);
          const t = performance.now();
          while (performance.now() - t < 250) {}
          const crossed = q.c0 !== null && sounded() >= q.program.length;
          document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "l", bubbles: true }));
          // The loop is switched by the page's next microtasks, not its
          // next frame.
          await Promise.resolve();
          await new Promise((done) => setTimeout(done, 0));
          return { crossed, loop: q.program.loop, ends: q.program.end, sounded: sounded() };
        });
        await page.keyboard.press("Space");
        await settle(page);
        if (!r.crossed) wrong = "the sound had not crossed the clip's end when L came";
        else if (r.loop) wrong = "L did not switch the loop off";
        else if (!(r.ends > r.sounded)) {
          wrong = `the play was ended ${((r.sounded - r.ends) * 1000).toFixed(0)} ms behind the sound, which had played on into the clip's start`;
        }
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
        } else {
          // So many seconds of the play, read off the playhead, not off the
          // wall: a busy machine takes a moment longer to start the play,
          // and a play timed by the wall then stood inside the clip for
          // 1.83 s where the rules ask 2. The play may end sooner, at the
          // clip's end.
          await page
            .waitForFunction(
              (to) => {
                const playing = !!document.querySelector('button[aria-label="Pause"]');
                window.__wasPlaying ||= playing;
                return (window.__wasPlaying && !playing) || Number(document.querySelector(".screen").dataset.playhead) >= to;
              },
              began + seconds,
              { timeout: seconds * 1000 + 30000, polling: "raf" },
            )
            .catch(() => (wrong = `the play from ${began.toFixed(3)} neither went on ${seconds} s nor ended`));
          await page.evaluate(() => delete window.__wasPlaying);
        }
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
      case "swipe after click": {
        const [share, px] = arg;
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        const x = from + share * (g.end.x - from);
        if (!inside(g.track, x)) {
          wrong = `${share} of the way along the clip is not on the clip timeline`;
          break;
        }
        await page.mouse.click(x, high(g.track));
        await settle(page);
        // The hand stays where it clicked, on the playhead now, and swipes
        // from there, the way a hand on a trackpad does after it clicks.
        for (let i = 1; i <= 3 && !wrong; i++) {
          const was = (await handles(page)).start.x;
          await page.mouse.wheel(px, 0);
          await page.waitForTimeout(100);
          const moved = (await handles(page)).start.x - was;
          if (!(-moved * Math.sign(px) > Math.abs(px) / 3)) {
            wrong = `swipe ${i} by ${px} px after the click moved the clip ${moved.toFixed(1)} px`;
          }
        }
        await settle(page);
        await watch.step("swipe", s);
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
        const [where, stem] = what === "drop" ? [arg[1]] : [arg].flat();
        let file;
        if (what === "drop") file = droppedVideo = await control(url, `/pick?seconds=${arg[0]}&box=0`);
        else if (what === "drop again") file = droppedVideo;
        else {
          if (droppedOther) droppedBefore = droppedOther;
          file = droppedOther = join(tmpdir(), `${stem ?? "notizen"}-${process.pid}.pdf`);
          writeFileSync(file, "%PDF-1.4\n");
        }
        if (where !== "workspace") {
          const name = where === "settings" ? "Settings" : "Updates";
          await fromSidebar(page, () => page.locator("aside").getByText(name, { exact: true }).first().click());
          await settle(page);
        }
        if (what === "drop not a video") standing = await standsAt(page);
        wrong = await dropOn(page, [file]);
        if (what === "drop not a video") rmSync(file, { force: true });
        await watch.step("add", s);
        break;
      }
      case "no reason": {
        await page.waitForTimeout(500);
        const said = await toastSays(page);
        const name = droppedVideo ? basename(droppedVideo) : "";
        if (name && said.includes(name)) wrong = `the app says "${said}" about ${name}, which it added`;
        else if (!name && said) wrong = `the app still says "${said}"`;
        break;
      }
      case "stacked": {
        // Tim dropped one picture after another and saw only the last:
        // a newer reason replaced the older.
        const said = await page.evaluate(() => [...document.querySelectorAll(".toast .text")].map((t) => t.textContent.trim()));
        const [newer, older] = [basename(droppedOther), basename(droppedBefore)];
        const tops = await page.evaluate(() => [...document.querySelectorAll(".toast")].map((t) => Math.round(t.getBoundingClientRect().top)));
        if (said.length !== 2) wrong = `${said.length} toasts show, not one for each of ${older} and ${newer}: ${JSON.stringify(said)}`;
        else if (!said[0].includes(newer) || !said[1].includes(older)) wrong = `the toasts read ${JSON.stringify(said)}, not ${newer} on top of ${older}`;
        else if (!(tops[0] < tops[1])) wrong = `the newer toast stands at ${tops[0]} px and the older at ${tops[1]} px`;
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
          said = await toastSays(page);
        }
        const library = await ask(page, "Library");
        if (!said.includes(`${name} cannot be added because it is not a video`)) wrong = `the app says "${said}", not that ${name} cannot be added because it is not a video`;
        else if (library.some((e) => e.name === name)) wrong = `${name} is in the library`;
        else wrong = await stillStands(page, standing);
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
          // Twice: the frame's own layer, then the pop it may start over
          // the frame once it is drawn, see "the pop over a paused frame".
          for (let i = 0; i < 2; i++) {
            await sized.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
            await sized.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
          }
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
          // The frame and the two either side of it, in one read.
          const near = shortFramesFrom(short, Math.max(0, n - 1), n + 1, got.w, got.h);
          const ofShort = (m) => near[m - Math.max(0, n - 1)] ?? Buffer.alloc(0);
          const want = ofShort(n);
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
            const other = ofShort(m);
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
      case "search and cancel": {
        const seen = await page.evaluate(
          () =>
            new Promise((done) => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              const head = () => pane.querySelector(".listhead button.search");
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
        if (seen.pressed !== "Search") wrong = `the head said ${seen.pressed}, not Search`;
        else if (seen.frames.some((f) => f.says !== "Cancel"))
          wrong = `after Search the head said ${[...new Set(seen.frames.map((f) => f.says))].join(", ")}, not Cancel at once`;
        else if (greyed)
          wrong = `the head said Cancel greyed out for ${greyed} of ${seen.frames.length} frames after Search, so a press there did nothing`;
        else if (seen.after !== "Cancelling") wrong = `Cancel pressed, the head said ${seen.after}, not Cancelling`;
        console.log(`      Cancel pressed ${seen.frames.length} frames after Search, ${greyed} of them greyed out`);
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
      case "bugs": {
        await fromSidebar(main, () => main.locator("aside button.bugs").click());
        if (!reportPage) {
          reportPage = await browser.newPage({ viewport: { width: 560, height: 640 } });
          await slowDown(reportPage);
          reportPage.on("pageerror", (e) => errors.push(String(e)));
          // Every note the window plays, with its pitch and whether the
          // microphone was open then, for the dictate step's sounds.
          await reportPage.addInitScript(() => {
            window.__notes = [];
            const start = OscillatorNode.prototype.start;
            OscillatorNode.prototype.start = function (...a) {
              window.__notes.push({ f: this.frequency.value, open: document.querySelector("button.dictate")?.getAttribute("aria-label") === "Stop" });
              return start.apply(this, a);
            };
          });
          await reportPage.goto(new URL("?window=report", url).toString());
        } else {
          await reportPage.bringToFront();
        }
        page = reportPage;
        await page.locator("button.dictate").waitFor();
        await settle(page);
        break;
      }
      case "bugs, the pointer gone": {
        page = main;
        await main.mouse.move(20, 500);
        await main.waitForTimeout(400);
        const shut = () => main.evaluate(() => !document.querySelector("aside")?.classList.contains("open"));
        if (await shut()) {
          wrong = "the sidebar did not open under the pointer";
          break;
        }
        // Clicked from the page, so the pointer stays where it is: on a Mac
        // the report window comes up and the app's window hears nothing
        // more of the pointer.
        await main.evaluate(() => {
          document.querySelector("aside button.bugs").click();
          window.dispatchEvent(new FocusEvent("blur"));
        });
        await main.waitForTimeout(400);
        if (!(await shut())) {
          wrong = "the sidebar stayed open once the report window had the focus";
          break;
        }
        // The focus comes back to the button that was clicked.
        await main.evaluate(() => {
          window.dispatchEvent(new FocusEvent("focus"));
          document.querySelector("aside button.bugs").focus();
        });
        await main.waitForTimeout(400);
        if (!(await shut())) {
          wrong = "the sidebar opened when the app's window had the focus again";
          break;
        }
        await main.mouse.move(750, 400);
        await main.waitForTimeout(400);
        if (!(await shut())) wrong = "the sidebar opened on the next move of the pointer outside it";
        break;
      }
      case "app log": {
        if (arg === "open") {
          await fetch(new URL("/app-log?open=1", url));
          break;
        }
        // Advanced reads the log again whenever it opens.
        const disclose = page.locator("button.disclose");
        if ((await disclose.getAttribute("aria-expanded")) !== "true") await disclose.click();
        const said = arg === 1 ? "1 file," : `${arg} files,`;
        const card = page.locator(".card", { hasText: "The app's log" });
        await card.locator(".num", { hasText: said }).waitFor({ timeout: 5000 }).catch(() => {});
        const shown = ((await card.locator(".num").textContent().catch(() => "")) ?? "").trim();
        const kept = await (await fetch(new URL("/app-log", url))).json();
        if (!shown.startsWith(said)) wrong = `Settings says the app's log is "${shown}", not ${arg} files`;
        else if (kept.files !== arg) wrong = `the app's log has ${kept.files} files on the disk, not ${arg}`;
        break;
      }
      case "windows": {
        const [made, fronted] = arg;
        const w = await (await fetch(new URL("/report-window", url))).json();
        if (w.made !== made || w.fronted !== fronted) {
          wrong = `the app made ${w.made} report windows and brought one forward ${w.fronted} times, not ${made} and ${fronted}`;
          break;
        }
        const lit = await main.evaluate(() => document.querySelector("aside button.bugs")?.classList.contains("current"));
        if (lit !== w.open) wrong = `Bugs is ${lit ? "" : "not "}lit while the report window is ${w.open ? "open" : "closed"}`;
        break;
      }
      case "close report": {
        // What was typed is kept a moment after the last key.
        await page.waitForTimeout(600);
        await fetch(new URL("/report-window?close=1", url));
        await reportPage?.close();
        reportPage = null;
        page = main;
        await main.waitForFunction(() => !document.querySelector("aside button.bugs")?.classList.contains("current"), null, { timeout: 5000 }).catch(() => {});
        break;
      }
      case "attach": {
        const before = await page.locator("li.tile").count();
        standing = await standsAt(page);
        for (let i = 0; i < arg; i++) await fetch(new URL("/pick-picture", url));
        await page.locator("button.attach").click();
        const grew = await page
          .waitForFunction((n) => document.querySelectorAll("li.tile").length >= n || document.querySelector(".toast"), before + arg, { timeout: 10000 })
          .then(() => true)
          .catch(() => false);
        if (!grew) wrong = `the paperclip attached nothing, ${before} pictures before and after`;
        await settle(page);
        break;
      }
      case "drop pictures": {
        const [n, other] = Array.isArray(arg) ? arg : [arg];
        // The pictures that come take room under the field, so it may move.
        standing = null;
        // A number of pictures, or their shapes, "446x738".
        const shapes = typeof n === "string" ? n.split(" ") : Array(n).fill("");
        const files = [];
        for (const shape of shapes) {
          const [w, h] = shape.split("x");
          files.push(await (await fetch(new URL(`/pick-picture?box=0${w ? `&w=${w}&h=${h}` : ""}`, url))).json());
        }
        if (other) files.push(await (await fetch(new URL(`/pick-picture?box=0&kind=text&name=${encodeURIComponent(other)}`, url))).json());
        const before = await page.locator("li.tile").count();
        wrong = await dropOn(page, files);
        if (wrong) break;
        await page
          .waitForFunction((n) => document.querySelectorAll("li.tile").length >= n, before + shapes.length, { timeout: 10000 })
          .catch(() => (wrong = `a drop of ${shapes.length} pictures left ${before} on the report window`));
        await settle(page);
        break;
      }
      case "drop recording": {
        standing = null;
        const file = await (await fetch(new URL("/pick-picture?box=0&kind=recording", url))).json();
        const before = await page.locator("li.tile").count();
        wrong = await dropOn(page, [file]);
        if (wrong) break;
        await page
          .waitForFunction((n) => document.querySelectorAll("li.tile").length >= n, before + 1, { timeout: 15000 })
          .catch(async () => (wrong = `a dropped recording was not attached, the toast says "${await toastSays(page)}"`));
        await settle(page);
        break;
      }
      case "recording tile": {
        const tile = await page.evaluate(() => {
          const all = document.querySelectorAll("li.tile figure");
          const f = all[all.length - 1];
          const img = f?.querySelector("img");
          return f && { length: f.querySelector(".length")?.textContent.trim() ?? "", loaded: !!img && img.complete && img.naturalWidth > 0, label: f.getAttribute("aria-label") };
        });
        if (!tile) wrong = "there is no tile";
        else if (!tile.loaded) wrong = "the still of the recording did not load";
        else if (tile.length !== arg) wrong = `the recording's tile says "${tile.length}", not ${arg}`;
        else if (!tile.label.includes(`a recording of ${arg}`)) wrong = `the recording's tile is read out as "${tile.label}"`;
        break;
      }
      case "paste picture": {
        const before = await page.locator("li.tile").count();
        await page.locator(".telling").click();
        await page.evaluate(async () => {
          const c = document.createElement("canvas");
          c.width = 64;
          c.height = 40;
          const g = c.getContext("2d");
          g.fillStyle = "#942192";
          g.fillRect(0, 0, 64, 40);
          const blob = await new Promise((done) => c.toBlob(done, "image/png"));
          const data = new DataTransfer();
          data.items.add(new File([blob], "image.png", { type: "image/png" }));
          document.querySelector(".telling").dispatchEvent(new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }));
        });
        await page
          .waitForFunction((n) => document.querySelectorAll("li.tile").length >= n, before + 1, { timeout: 10000 })
          .catch(() => (wrong = "a pasted picture was not attached"));
        await settle(page);
        break;
      }
      case "pictures": {
        const n = await page.evaluate(() => document.querySelectorAll("li.tile figure img").length);
        if (n !== arg) wrong = `the report window shows ${n} pictures, not ${arg}`;
        // Each one is the picture the Go side kept, loaded and shown.
        const unseen = await page.evaluate(() => [...document.querySelectorAll("li.tile figure img")].filter((i) => !i.complete || i.naturalWidth === 0).length);
        if (!wrong && unseen) wrong = `${unseen} of the pictures did not load`;
        break;
      }
      case "pictures fit": {
        // Every picture is drawn whole in its tile, in its middle, and as
        // large as the tile lets it be: it reaches both sides or the top
        // and the bottom. Tim saw tall pictures cut off and small ones
        // standing in a corner.
        await page.waitForFunction(() => [...document.querySelectorAll("li.tile figure img")].every((i) => i.complete && i.naturalWidth), null, { timeout: 10000 }).catch(() => {});
        const bad = await page.evaluate(() =>
          [...document.querySelectorAll("li.tile figure")].flatMap((fig) => {
            const img = fig.querySelector("img");
            const f = fig.getBoundingClientRect();
            const inner = { l: f.left + fig.clientLeft, t: f.top + fig.clientTop, w: fig.clientWidth, h: fig.clientHeight };
            const b = img.getBoundingClientRect();
            // A picture that never loaded has no size, and every sum below
            // with it is NaN, which no comparison is true of.
            if (!img.complete || !img.naturalWidth) return [`${img.alt || "a picture"} did not load`];
            const fit = getComputedStyle(img).objectFit;
            const s = fit === "contain" ? Math.min(b.width / img.naturalWidth, b.height / img.naturalHeight) : b.width / img.naturalWidth;
            const w = img.naturalWidth * s;
            const h = img.naturalHeight * s;
            const l = b.left + (b.width - w) / 2;
            const t = b.top + (b.height - h) / 2;
            const name = `${img.naturalWidth}x${img.naturalHeight}`;
            const out = [];
            if (l < inner.l - 0.5 || t < inner.t - 0.5 || l + w > inner.l + inner.w + 0.5 || t + h > inner.t + inner.h + 0.5) out.push(`${name} is cut off, drawn ${Math.round(w)}x${Math.round(h)} in a tile of ${inner.w}x${inner.h}`);
            else if (Math.abs(w - inner.w) > 1 && Math.abs(h - inner.h) > 1) out.push(`${name} is drawn ${Math.round(w)}x${Math.round(h)}, short of its tile of ${inner.w}x${inner.h} both ways`);
            else if (Math.abs(l + w / 2 - (inner.l + inner.w / 2)) > 1 || Math.abs(t + h / 2 - (inner.t + inner.h / 2)) > 1) out.push(`${name} is not in the middle of its tile`);
            return out;
          }),
        );
        if (bad.length) wrong = bad.join(", ");
        else if (!(await page.locator("li.tile figure img").count())) wrong = "there is no picture to fit";
        break;
      }
      case "report at end": {
        // The field shows the end of what is in it, down to its padding.
        const m = await page.evaluate(() => {
          const f = document.querySelector(".telling");
          return [f.scrollTop, f.scrollHeight - f.clientHeight];
        });
        if (m[1] <= 0) wrong = "the report is not long enough to scroll";
        else if (Math.abs(m[0] - m[1]) > 1) wrong = `the report is scrolled to ${m[0]} px, not to its end at ${m[1]} px`;
        break;
      }
      case "toast": {
        let said = "";
        for (let i = 0; i < 50 && (arg === "" ? said !== "" : !said.includes(arg)); i++) {
          await page.waitForTimeout(100);
          said = await toastSays(page);
        }
        const line = await page.evaluate(() => document.querySelector(".words")?.textContent.trim() ?? "");
        if (arg === "" ? said !== "" : !said.includes(arg)) wrong = `the toast says "${said}", not "${arg}"`;
        else if (line) wrong = `the line beside the buttons says "${line}", where only the toast should`;
        else wrong = await stillStands(page, standing);
        break;
      }
      case "toast goes": {
        // Its time runs from the moment it comes, with no hand on it. Tim
        // found a toast that waited for the pointer before its time began.
        // Its time is the ten seconds a removed row has, so it is still
        // there after nine. It once ran eight of its own.
        await page.mouse.move(2, 400);
        await page.waitForTimeout(9000);
        if (!(await toastSays(page))) {
          wrong = "the toast went within nine seconds, sooner than the ten a removed row has";
          break;
        }
        if (!(await gone(page, ".toast", 5000))) wrong = "the toast was still there fourteen seconds after it came, with the pointer away";
        break;
      }
      case "toast held": {
        const box = await page.locator(".toast .text").first().boundingBox();
        await page.mouse.move(box.x + 4, box.y + box.height / 2);
        await page.waitForTimeout(11500);
        if (!(await toastSays(page))) {
          wrong = "the toast went while the pointer was on it";
          break;
        }
        await page.mouse.move(2, 400);
        if (!(await gone(page, ".toast", 14000))) wrong = "the toast was still there fourteen seconds after the pointer left it";
        break;
      }
      case "toast closed": {
        const before = await page.locator(".toast").count();
        await page.locator(".toast button.close").first().click();
        // Within 400 ms, and on as soon as it is gone.
        await page.waitForFunction((n) => document.querySelectorAll(".toast").length === n, before - 1, { polling: "raf", timeout: 400 }).catch(() => {});
        if ((await page.locator(".toast").count()) !== before - 1) wrong = "the toast is still there after its cross was pressed";
        break;
      }
      case "remove picture": {
        // By Delete on the tile under a pointer that stands still, with
        // "key", the way a Mac draws a row that comes up under it: not
        // under the pointer until the pointer moves.
        const [i, how] = [arg].flat();
        const tile = page.locator("li.tile").nth(i - 1);
        await tile.hover();
        await page.waitForTimeout(150);
        const shown = await page.evaluate(() => [...document.querySelectorAll("li.tile .bin")].map((b) => getComputedStyle(b).opacity));
        if (shown[i - 1] !== "1" || shown.filter((o) => o === "1").length !== 1) {
          wrong = `pointing at picture ${i} the bins stand at ${shown.join(", ")}`;
          break;
        }
        const id = await tile.getAttribute("data-id");
        // Where the pointer stands while the picture goes: on the bin it
        // presses, or in the middle of the tile hovered for Delete.
        const spot = await (how === "key" ? tile : tile.locator(".bin")).boundingBox();
        const point = [spot.x + spot.width / 2, spot.y + spot.height / 2];
        // What the page hears of the pointer and the focus from the press
        // on, said if the time runs, since a runner's Chromium has made it
        // run where this machine's does not (2.263).
        await page.evaluate(() => {
          const name = (n) => (n instanceof Element ? `${n.tagName.toLowerCase()}.${[...n.classList].join(".")}${n.isConnected ? "" : " gone"}` : String(n));
          window.__heardPointer = [];
          const t0 = performance.now();
          for (const type of ["pointerover", "pointerout", "pointermove", "pointerdown", "pointerup", "focusin", "focusout"]) {
            document.addEventListener(
              type,
              (e) => window.__heardPointer.push(`${Math.round(performance.now() - t0)} ms ${type} ${name(e.target)}${e.relatedTarget ? ` from ${name(e.relatedTarget)}` : ""}${"clientX" in e ? ` at ${e.clientX},${e.clientY}` : ""}`),
              true,
            );
          }
        });
        if (how === "key") {
          await tile.locator("figure").focus();
          await page.keyboard.press("Delete");
        } else await tile.locator(".bin").click();
        // The tile says Removed in the frame of the click.
        const gone = await page.evaluate((id) => {
          const g = document.querySelector(`li.tile[data-id="${id}"] .gone`);
          return g ? getComputedStyle(g).animationName : null;
        }, id);
        if (gone === null) wrong = `picture ${i} did not say Removed in its place`;
        // It says so where the picture was, with nothing sliding in. Tim
        // saw a swipe across the tile.
        else if (gone !== "none") wrong = `picture ${i} says Removed with the animation ${gone}`;
        if (wrong) break;
        // What a browser may tell a pointer that stands still while the
        // node under it goes: that it went off the app, and over the tile
        // round what took its place, and only a frame later over that.
        // The time ran for that frame on CI's Chromium (2.263). With
        // "tile" that last over never comes, which WebKit, the Mac's
        // browser, may well do, since it counts nothing that comes up
        // under a still pointer as under it until the pointer moves.
        await page.evaluate(
          async ({ id, point: [x, y], onTile }) => {
            const tile = document.querySelector(`li.tile[data-id="${id}"]`);
            const at = { bubbles: true, clientX: x, clientY: y, pointerType: "mouse" };
            tile.dispatchEvent(new PointerEvent("pointerout", at));
            tile.dispatchEvent(new PointerEvent("pointerover", at));
            await new Promise(requestAnimationFrame);
            if (!onTile) document.elementFromPoint(x, y).dispatchEvent(new PointerEvent("pointerover", at));
          },
          { id, point, onTile: how === "tile" },
        );
        // The pointer is still on it, so its time has not started: the
        // fill stands full until the pointer leaves. Tim saw it run for a
        // moment and stop a little short of full.
        await page.waitForTimeout(500);
        const fill = await page.evaluate((id) => {
          const run = document.querySelector(`li.tile[data-id="${id}"] .gone .run`);
          const box = run?.parentElement?.getBoundingClientRect();
          return run && box ? -new DOMMatrix(getComputedStyle(run).transform).m41 / box.width : null;
        }, id);
        if (fill === null || fill > 0.001) {
          const heard = await page.evaluate(() => window.__heardPointer.slice(0, 12).join(", "));
          wrong = `picture ${i}'s time ran out by ${Math.round((fill ?? 1) * 1000) / 10} % with the pointer still on it, the page heard ${heard}`;
        }
        await settle(page);
        break;
      }
      case "put back picture": {
        // The picture is back in the same frame as the click, before the
        // Go side answers, or the grid closes over it for a moment and
        // the tiles after it jump. Found in the review of #217.
        const counts = await page.evaluate(async (i) => {
          const before = document.querySelectorAll("li.tile").length;
          const button = [...document.querySelectorAll("li.tile")[i - 1].querySelectorAll("button")].find((b) => b.textContent.includes("Put it back"));
          button.click();
          await Promise.resolve();
          await Promise.resolve();
          return [before, document.querySelectorAll("li.tile").length];
        }, arg);
        if (counts[0] !== counts[1]) {
          wrong = `the grid went from ${counts[0]} tiles to ${counts[1]} on Put it back, before the picture came back`;
          break;
        }
        await settle(page);
        break;
      }
      case "removed goes": {
        await page.mouse.move(5, 5);
        const went = await page
          .waitForFunction(() => !document.querySelector("li.tile .gone"), null, { timeout: 15000 })
          .then(() => true)
          .catch(() => false);
        if (!went) wrong = "a removed picture still stands in its place after its time";
        break;
      }
      case "report": {
        const written = await page.evaluate(() => document.querySelector(".telling").innerText.trim());
        await page.locator("button.report").click();
        // Report closes the window once Finder has the zip, and says
        // nothing beside the buttons on the way. Tim had a line there.
        const closed = await (async () => {
          for (const end = Date.now() + 15000; Date.now() < end; await new Promise((r) => setTimeout(r, 100))) {
            const w = await (await fetch(new URL("/report-window", url))).json();
            if (!w.open) return true;
          }
          return false;
        })();
        if (!closed) {
          wrong = `Report left the window open, saying "${await page.evaluate(() => document.querySelector(".words")?.textContent.trim())}"`;
          break;
        }
        const said = await page.evaluate(() => document.querySelector(".words")?.textContent.trim());
        if (said) {
          wrong = `Report said "${said}" beside the buttons`;
          break;
        }
        const names = await (await fetch(new URL("/report-made", url))).json();
        const [want, films = 0] = [arg].flat();
        const pictures = names.filter((n) => /\/pictures\/\d+\.png$/.test(n)).length;
        const recordings = names.filter((n) => /\/recordings\/\d+\.(mov|mp4|m4v|webm|mkv)$/.test(n)).length;
        if (pictures !== want) wrong = `the report holds ${pictures} pictures, not ${want}: ${names.join(", ")}`;
        else if (recordings !== films) wrong = `the report holds ${recordings} recordings, not ${films}: ${names.join(", ")}`;
        else if (written && !names.some((n) => n.endsWith("what-happened.txt"))) wrong = "the report holds nothing of what was written";
        const left = await page.evaluate(() => [document.querySelector(".telling").innerText.trim(), document.querySelectorAll("li.tile").length]);
        if (!wrong && (left[0] || left[1])) wrong = `after Report the field holds "${left[0]}" and ${left[1]} pictures`;
        break;
      }
      case "report scroll": {
        const box = await page.locator(".telling").boundingBox();
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
        const was = await page.locator(".telling").evaluate((f) => f.scrollTop);
        await page.mouse.wheel(0, arg);
        // Read as soon as the wheel has turned, before the next words come.
        kept.scrolledTo = await page.locator(".telling").evaluate(
          (f, was) => new Promise((done) => {
            const read = () => (f.scrollTop !== was ? done(f.scrollTop) : requestAnimationFrame(read));
            requestAnimationFrame(read);
            setTimeout(() => done(f.scrollTop), 1000);
          }),
          was,
        );
        if (arg < 0 && kept.scrolledTo >= was) wrong = `the wheel did not scroll the field up, it is at ${kept.scrolledTo} px`;
        break;
      }
      case "report follows":
      case "report stays": {
        const was = await page.locator(".telling").evaluate((f) => f.innerText);
        const more = await page
          .waitForFunction((was) => document.querySelector(".telling").innerText !== was, was, { timeout: 10000 })
          .then(() => true)
          .catch(() => false);
        if (!more) {
          wrong = "nothing more was said";
          break;
        }
        await page.waitForTimeout(100);
        const m = await page.locator(".telling").evaluate((f) => {
          const mark = f.querySelector(".hearing")?.getBoundingClientRect();
          const box = f.getBoundingClientRect();
          return {
            top: f.scrollTop,
            end: f.scrollHeight - f.clientHeight,
            seen: !!mark && mark.bottom <= box.bottom + 1 && mark.top >= box.top - 1,
          };
        });
        if (what === "report stays") {
          if (m.top !== kept.scrolledTo) wrong = `the field moved to ${m.top} px while more was said, from ${kept.scrolledTo} px where it was scrolled to`;
        } else if (!m.seen) wrong = "the words being said are out of view";
        else if (Math.abs(m.top - m.end) > 1) wrong = `the field is at ${m.top} px, not at its end, ${m.end} px`;
        break;
      }
      case "caret at words": {
        const c = await page.locator(".telling").evaluate((f) => {
          const mark = f.querySelector(".hearing");
          return { ours: f.classList.contains("ours"), caret: getComputedStyle(f).caretColor, bar: mark ? getComputedStyle(mark).borderRightWidth : "" };
        });
        if (!c.ours || c.bar !== "1px" || c.caret !== "rgba(0, 0, 0, 0)") wrong = `the cursor is not drawn after the words being said: ${JSON.stringify(c)}`;
        break;
      }
      case "caret elsewhere": {
        const box = await page.locator(".telling").boundingBox();
        await page.locator(".telling").evaluate((f) => (f.scrollTop = 0));
        await page.mouse.click(box.x + 20, box.y + 12);
        await page.waitForTimeout(200);
        const c = await page.locator(".telling").evaluate((f) => ({ ours: f.classList.contains("ours"), caret: getComputedStyle(f).caretColor }));
        if (c.ours || c.caret === "rgba(0, 0, 0, 0)") wrong = `the field hides its cursor after a click into its first line: ${JSON.stringify(c)}`;
        break;
      }
      case "buttons fit": {
        const disclose = page.locator("button.disclose");
        if ((await disclose.count()) && (await disclose.getAttribute("aria-expanded")) !== "true") {
          await disclose.click();
          await settle(page);
        }
        const seen = await page.evaluate(() => {
          const out = [];
          const ruler = document.createElement("span");
          ruler.style.cssText = "position: absolute; visibility: hidden; white-space: pre";
          document.body.append(ruler);
          for (const b of document.querySelectorAll("button")) {
            const r = b.getBoundingClientRect();
            const cs = getComputedStyle(b);
            if (!r.width || cs.visibility === "hidden" || parseFloat(cs.flexGrow) > 0) continue;
            if (b.matches(".link") || [...b.children].some((c) => !c.classList.contains("beam"))) continue;
            const own = b.innerText.trim();
            if (!own) continue;
            ruler.style.font = cs.font;
            ruler.style.letterSpacing = cs.letterSpacing;
            ruler.textContent = own;
            const words = ruler.getBoundingClientRect().width;
            const round = ["paddingLeft", "paddingRight", "borderLeftWidth", "borderRightWidth"].reduce((n, k) => n + parseFloat(cs[k]), 0);
            out.push({ own, width: r.width, want: words + round });
          }
          ruler.remove();
          return out;
        });
        const problems = seen
          .filter((b) => Math.abs(b.width - b.want) > 1)
          .map((b) => `${b.own} is ${b.width.toFixed(1)} px wide, its words and padding ${b.want.toFixed(1)}`);
        for (const w of arg ?? []) if (!seen.some((b) => b.own === w)) problems.push(`no button says ${w}`);
        if (problems.length) wrong = problems.join("\n");
        break;
      }
      case "report lines up": {
        const m = await page.evaluate(() => {
          const r = (s) => document.querySelector(s)?.getBoundingClientRect();
          const tiles = [...document.querySelectorAll("li.tile")].map((t) => t.getBoundingClientRect());
          const bins = [...document.querySelectorAll("li.tile .bin")].map((b) => b.getBoundingClientRect());
          const gap = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--gap"));
          const control = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--control-h"));
          return { mic: r("button.dictate"), clip: r("button.attach"), report: r("button.report"), tiles, bins, gap, control };
        });
        const problems = [];
        for (const [name, b] of [["the microphone", m.mic], ["the paperclip", m.clip], ["Report", m.report]]) {
          if (b.height !== m.control) problems.push(`${name} is ${b.height} px high, not ${m.control}`);
          // Issue #279: the row under the field stood half a pixel down,
          // its top at 309.5, so it stepped as it faded or moved.
          if (b.top !== Math.round(b.top)) problems.push(`${name} stands between two pixels, its top at ${b.top}`);
        }
        if (m.mic.width !== m.mic.height || m.clip.width !== m.clip.height) problems.push("the microphone or the paperclip is not square");
        const t0 = m.tiles[0];
        for (const [i, t] of m.tiles.entries()) {
          if (t.width !== t0.width || t.height !== t0.height) problems.push(`picture ${i + 1} is ${t.width} by ${t.height}, the first ${t0.width} by ${t0.height}`);
          for (const v of [t.x, t.y, t.width, t.height]) if (v !== Math.round(v)) problems.push(`picture ${i + 1} stands between pixels, at ${v}`);
          const next = m.tiles[i + 1];
          if (next && next.y === t.y && next.x - (t.x + t.width) !== m.gap) problems.push(`the space after picture ${i + 1} is ${next.x - (t.x + t.width)} px, not ${m.gap}`);
          // The sidebar's bin in its row's corner: 24 px square, 6 px in.
          // A space in and as tall as a control, it stood too far into
          // the picture, Tim found.
          const b = m.bins[i];
          const bin = [b.width, b.height, b.y - t.y, t.x + t.width - (b.x + b.width)];
          if (bin.join() !== "24,24,6,6") problems.push(`the bin of picture ${i + 1} is ${bin[0]} by ${bin[1]} px, ${bin[2]} px from its top and ${bin[3]} px from its right, not 24 by 24 and 6 from both`);
        }
        if (problems.length) wrong = problems.join("\n");
        break;
      }
      case "write": {
        const [text, at] = arg;
        await page.locator(".telling").click();
        await page.keyboard.type(text);
        await page.evaluate((n) => {
          const field = document.querySelector(".telling");
          const at = document.createRange();
          if (n === "end") {
            at.selectNodeContents(field);
            at.collapse(false);
          } else at.setStart(field.firstChild, n);
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
        const running = (await button.getAttribute("aria-label")) === "Stop";
        if (!running) await button.click();
        // Stop the moment it is pressed, see the rules on a click.
        const said = await button.getAttribute("aria-label");
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
          .waitForFunction(() => parseFloat(document.querySelector("button.dictate")?.dataset.loud) > 0.3, null, { timeout: 10000 })
          .then(() => true)
          .catch(() => false);
        if (!moved) {
          wrong = "the microphone did not move while it heard a voice";
          break;
        }
        // And it moves enough to be seen, smoothly, and in its place. For
        // three seconds of the hum and its pauses, every frame, the size
        // it is drawn at, its middle and the size of its own box. A
        // microphone 16 px at rest grows by at least six tenths of that
        // for every step of loudness: by a third it grew 3 px for a voice
        // talking, and Tim could hardly see it. Its box keeps one size
        // and its middle keeps still: a size that changed snapped to
        // whole pixels and shook on Tim's Mac. It falls back no faster
        // than a level meter's needle, a third of a second for the whole
        // way: jumping to every reading shook too.
        const track = await page.evaluate(
          () =>
            new Promise((done) => {
              const b = document.querySelector("button.dictate");
              const xs = [];
              const start = performance.now();
              const step = (now) => {
                const icon = b.querySelector("svg");
                const r = icon.getBoundingClientRect();
                xs.push({
                  t: now,
                  w: r.width,
                  x: r.left + r.width / 2,
                  y: r.top + r.height / 2,
                  box: getComputedStyle(icon).width,
                  loud: parseFloat(b.dataset.loud),
                  size: Number(icon.getAttribute("width")),
                });
                if (now - start < 3000) requestAnimationFrame(step);
                else done(xs);
              };
              requestAnimationFrame(step);
            }),
        );
        const widest = Math.max(...track.map((f) => f.w));
        const loudest = Math.max(...track.map((f) => f.loud));
        const boxes = [...new Set(track.map((f) => f.box))];
        const drift = Math.max(...track.map((f) => Math.hypot(f.x - track[0].x, f.y - track[0].y)));
        let fastest = 0;
        for (let k = 1; k < track.length; k++) {
          const dt = track[k].t - track[k - 1].t;
          if (dt > 0) fastest = Math.max(fastest, (track[k - 1].w - track[k].w) / track[0].size / dt);
        }
        if (widest < 16 * (1 + 0.6 * loudest) - 0.5) {
          wrong = `the microphone hardly grew with the voice, ${widest.toFixed(1)} px wide at most at a loudness of ${loudest}`;
          break;
        }
        if (boxes.length > 1) {
          wrong = `the microphone changes the size of its box as it grows, ${boxes.join(", ")}, which snaps to whole pixels and shakes`;
          break;
        }
        if (drift > 0.5) {
          wrong = `the middle of the microphone moved by ${drift.toFixed(2)} px as it grew`;
          break;
        }
        if (fastest > 0.003) {
          wrong = `the microphone fell back by ${(fastest * 1000).toFixed(1)} of its size a second, faster than a level meter's needle`;
          break;
        }
        // It is drawn white, like every icon in the app, as the paperclip
        // beside it is, and so stands out from its button by at least 3
        // to 1. Tim saw the accent dark on dark, and the lit shade of it
        // matched nothing in the app.
        const look = await page.evaluate(() => {
          const b = document.querySelector("button.dictate");
          const icon = b.querySelector("svg");
          const clip = document.querySelector("button.attach svg");
          // A colour mixed from the accent comes back as color(srgb ...),
          // its channels from 0 to 1.
          const rgb = (c) => {
            const v = c.match(/[\d.]+/g).map(Number);
            return c.startsWith("color(") ? [...v.slice(0, 3).map((x) => x * 255), ...v.slice(3, 4)] : v.slice(0, 4);
          };
          const luminance = ([r, g, b]) => {
            const l = (v) => ((v /= 255) <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
            return 0.2126 * l(r) + 0.7152 * l(g) + 0.0722 * l(b);
          };
          let back = null;
          for (let e = b; e && !back; e = e.parentElement) {
            const c = rgb(getComputedStyle(e).backgroundColor);
            if (c.length < 4 || c[3] > 0) back = c;
          }
          const [hi, lo] = [luminance(rgb(getComputedStyle(icon).color)), luminance(back ?? [0, 0, 0])].sort((x, y) => y - x);
          return { colour: getComputedStyle(icon).color, clip: getComputedStyle(clip).color, contrast: (hi + 0.05) / (lo + 0.05) };
        });
        if (look.colour !== look.clip) {
          wrong = `the microphone is drawn ${look.colour} while it listens, not white like the paperclip, ${look.clip}`;
          break;
        }
        if (look.contrast < 3) {
          wrong = `the microphone stands out from its button by only ${look.contrast.toFixed(2)} to 1 while it listens`;
          break;
        }
        // Two notes going up say the microphone is open, while it is.
        const up = await page.evaluate(() => window.__notes.slice(-2));
        if (up.length < 2 || !up.every((n) => n.open) || up[0].f >= up[1].f) {
          wrong = `the microphone opened without the start sound, it played ${JSON.stringify(up)}`;
          break;
        }
        kept.notes = await page.evaluate(() => window.__notes.length);
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
        await page.waitForFunction(() => document.querySelector("button.dictate")?.getAttribute("aria-label") === "Dictate", null, {
          timeout: 20000,
        });
        if (await page.locator(".telling .dictated").count()) wrong = "what was being said is still grey in the report";
        // Two notes going down say it stopped, however it stopped, and
        // only those two.
        const down = await page.evaluate((from) => window.__notes.slice(from), kept.notes);
        if (down.length !== 2 || down[0].f <= down[1].f) wrong = `it stopped with ${JSON.stringify(down)} played, not the two notes of the stop sound`;
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
        const was = arg === "grown" ? kept.grown : kept.report;
        if (have !== was) wrong = `the report reads ${JSON.stringify(have)}, not ${JSON.stringify(was)} as before`;
        break;
      }
      case "report grew": {
        const have = await page.locator(".telling").evaluate((f) => f.innerText.replace(/\n$/, ""));
        // Every word there was, in order, and words between them.
        const words = kept.report.split(/\s+/);
        let at = 0;
        for (const w of have.split(/\s+/)) if (w === words[at]) at++;
        if (at < words.length || have.split(/\s+/).length <= words.length)
          wrong = `the report reads ${JSON.stringify(have)}, not more than ${JSON.stringify(kept.report)}`;
        kept.grown = have;
        break;
      }
      case "episode":
        await fromSidebar(page, () => page.locator("aside li", { hasText: episodeBefore.split("/").pop() }).first().click());
        await page.locator(".listhead button.search").first().waitFor();
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
        await page.locator(".listhead button.search").first().waitFor();
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
          // A word lifted out of its caption after a word step, laid over
          // the rest grown to its peak.
          { light: "", bytes: word(512, 512, 512), over: [0, 0, 0, 128], lift: [148, 33, 146, 255], name: "a word bouncing over a box at half" },
        ];
        const got = await page.evaluate((p) => window.__checkPixels?.(p) ?? null, pixels.map(({ light, bytes, over, lift }) => ({ light, bytes, over, lift })));
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
        // A word lifted out and grown, every pixel of it, on the GPU and
        // by laidPart alike, so the shader grows it round its middle as
        // the 2D canvas does. Both sample the word between its four
        // nearest pixels, the GPU's filter in steps of its own.
        const LIFT_OFF = 0.01;
        const lifted = await page.evaluate(() => window.__checkLift?.([1.15, 2]) ?? null);
        let most = 0;
        lifted?.gpu.forEach((pixels, k) =>
          pixels.forEach((gpu, i) => {
            const off = Math.max(...gpu.map((v, c) => Math.abs(v - lifted.cpu[k][i][c])));
            most = Math.max(most, off);
            if (!(off < LIFT_OFF))
              problems.push(`the word grown by ${[1.15, 2][k]} at ${i % 8},${i >> 3}: the GPU gives ${gpu.map((v) => v.toFixed(4))}, laidPart ${lifted.cpu[k][i].map((v) => v.toFixed(4))}`);
          }),
        );
        if (lifted) console.log(`        a word grown on the GPU is at most ${most.toFixed(4)} from laidPart's, over every pixel`);
        // The canvas of standard video is in the standard mode, the one
        // the rest of the app is drawn in. In the extended mode it turned
        // the Mac's HDR on, and Tim's pictures on #201 showed the captions
        // on the canvas darker than the box a word is typed in.
        const mode = await page.evaluate(
          () => document.querySelector(".screen canvas")?.getContext("webgpu")?.getConfiguration?.()?.toneMapping?.mode ?? null,
        );
        console.log(`        the canvas of the bridge's standard video is in the ${mode} mode`);
        if (mode && mode !== "standard") problems.push(`the canvas of standard video is in the ${mode} mode`);
        if (problems.length) wrong = problems.join("\n");
        break;
      }
      case "past the end": {
        const { duration, fps } = await ask(page, "Source", await episodeOn(page));
        // The end of the video is the frame after its last, which a length
        // that falls inside a frame puts up to a frame later.
        const last = duration + 1 / fps + 0.001;
        for (let i = 1; i <= 3 && !wrong; i++) {
          const g = await handles(page);
          await drag(page, middle(g.end), high(g.track), 4000, false);
          await watch.step("trim", s);
          const { segments } = await engineState(page, watch.at);
          const end = segments[segments.length - 1].end;
          if (end > last) {
            wrong = `dragged past the end ${i} times, the clip ends at ${end.toFixed(2)}, after the video, which ends at ${duration.toFixed(2)}`;
          }
        }
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
        await page.locator("aside li", { hasText: name }).first().waitFor();
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
      case "time field": {
        const { fps, videoStart = 0, duration = 0 } = await ask(page, "Source", await episodeOn(page));
        // Said here the way the field must say it, from the frame, so the
        // walk takes nobody's word for what the frame is: hours for a
        // video of an hour or more, and the minutes always two figures.
        const said = (at) => {
          const k = Math.floor((Math.max(at - videoStart, 0) + 0.001) * fps);
          const ms = Math.round((videoStart + k / fps) * 1000);
          const two = (n) => String(n).padStart(2, "0");
          const h = Math.floor(ms / 3600000);
          const m = Math.floor(ms / 60000) % 60;
          const rest = `${two(Math.floor(ms / 1000) % 60)}.${String(ms % 1000).padStart(3, "0")}`;
          return duration >= 3600 ? `${h}:${two(m)}:${rest}` : `${two(m + h * 60)}:${rest}`;
        };
        const read = () =>
          page.evaluate(() => {
            const f = document.querySelector(".detail .row .timecode");
            const b = f?.getBoundingClientRect();
            return {
              text: f?.textContent.trim() ?? null,
              w: b?.width ?? 0,
              h: b?.height ?? 0,
              control: parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--control-h")),
              at: Number(document.querySelector(".screen")?.dataset.playhead),
              row: document.querySelector(".detail .row")?.textContent ?? "",
              title: document.querySelector(".pick.current .title")?.textContent.trim() ?? "",
            };
          });
        const first = await read();
        if (first.text === null) {
          wrong = "the row under the clip timeline has no time field";
          break;
        }
        if (first.title && first.row.includes(first.title)) {
          wrong = `the row under the clip timeline still says the clip's title, "${first.title}"`;
          break;
        }
        if (Math.abs(first.h - first.control) > 0.01) {
          wrong = `the time field is ${first.h.toFixed(2)} px tall, not ${first.control} as every control`;
          break;
        }
        // Down on the clip, a third of the way along, and along it with
        // the button held, each step a frame of its own.
        const g = await handles(page);
        const from = g.start.x + g.start.w;
        const x = from + (g.end.x - from) / 3;
        const y = high(g.track);
        await page.mouse.move(x, y);
        await page.mouse.down();
        const seen = new Set();
        for (let i = 0; i <= 12 && !wrong; i++) {
          if (i) await page.mouse.move(x + i * 8, y);
          await nextPaint(page, 2);
          const r = await read();
          seen.add(r.at);
          if (r.text !== said(r.at)) {
            wrong = `${i ? `${i * 8} px into the drag` : "pressed on the clip"}, with the button held, the playhead is at ${r.at.toFixed(4)} s and the time field says ${r.text}, not ${said(r.at)}`;
          } else if (Math.abs(r.w - first.w) > 0.01) {
            wrong = `the time field was ${first.w.toFixed(2)} px wide and is ${r.w.toFixed(2)} px at ${r.text}`;
          }
        }
        await page.mouse.up();
        await settle(page);
        if (wrong) break;
        if (seen.size < 4) {
          wrong = `the drag along the clip timeline moved the playhead to ${seen.size} places, so it proves nothing`;
          break;
        }
        // One frame on with each press of the arrow.
        await page.evaluate(() => document.body.focus());
        let was = await read();
        for (let i = 1; i <= 3 && !wrong; i++) {
          await page.keyboard.press("ArrowRight");
          await nextPaint(page, 2);
          const r = await read();
          const step = Math.round((r.at - was.at) * fps);
          if (step !== 1) wrong = `→ ${i} moved the playhead ${step} frames, from ${was.at.toFixed(4)} to ${r.at.toFixed(4)} s`;
          else if (r.text !== said(r.at)) wrong = `after → ${i} the playhead is at ${r.at.toFixed(4)} s and the time field says ${r.text}, not ${said(r.at)}`;
          was = r;
        }
        break;
      }
      case "card says": {
        const { segments } = await engineState(page, watch.at);
        const seconds = Math.round(segments.reduce((sum, p) => sum + p.end - p.start, 0));
        const n = await page.locator(".clip-timeline .piece").count();
        const want = `${clockOf(segments[0].start)}, ${seconds} s, ${n} ${n === 1 ? "piece" : "pieces"}`;
        const ok = await page
          .waitForFunction(
            (want) => document.querySelector(".pick.current .meta")?.textContent.replace(/\s+/g, " ").trim() === want,
            want,
            { polling: 50, timeout: 3000 },
          )
          .then(() => true, () => false);
        if (!ok) {
          const got = await page.evaluate(() => document.querySelector(".pick.current .meta")?.textContent.replace(/\s+/g, " ").trim() ?? "nothing");
          wrong = `the chosen clip's card says "${got}", not "${want}"`;
        }
        if (!wrong && n !== segments.length) wrong = `the clip timeline draws ${pieceCount(n)} and the engine keeps ${segments.length}`;
        break;
      }
      case "card follows": {
        const card = () => page.evaluate(() => document.querySelector(".pick.current .meta")?.textContent.replace(/\s+/g, " ").trim() ?? "");
        const before = await card();
        const g = await handles(page);
        const x = middle(g.end);
        const y = high(g.track);
        await page.mouse.move(x, y);
        await page.mouse.down();
        for (let i = 1; i <= 6; i++) {
          await page.mouse.move(x + (arg * i) / 6, y);
          await nextPaint(page);
        }
        await nextPaint(page, 2);
        const held = await card();
        await page.mouse.up();
        if (held === before) wrong = `with the clip's end dragged ${arg} px and the button still held, the card still says "${before}"`;
        await watch.step("trim", s);
        break;
      }
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
        await page.locator("aside li", { hasText: path.split("/").pop() }).first().waitFor();
        await fromSidebar(page, () => page.locator("aside li", { hasText: path.split("/").pop() }).first().click());
        await settle(page);
        break;
      }
      case "wait for": {
        const came = await page
          .waitForFunction(
            (word) => {
              const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
              return pane?.querySelector(".listhead button.search")?.textContent.replace(/\s+/g, " ").trim() === word;
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
      case "move window": {
        const [handle, share] = arg;
        const mark = () =>
          page.evaluate(() => {
            const aim = document.querySelector(".aim");
            const n = (sel) => Number(aim?.querySelector(sel)?.getAttribute("aria-valuenow"));
            return aim && { from: n(".tip.top.from"), to: n(".tip.top.to") };
          });
        const before = await mark();
        const grip = page.locator(handle === "bar" ? ".aim .bar.top" : `.aim .tip.top.${handle}`);
        const box = await grip.boundingBox().catch(() => null);
        const track = await page.getByLabel("The range picker", { exact: true }).boundingBox();
        if (!before || !box) {
          wrong = "the range picker shows no window to take hold of";
          break;
        }
        const path = await episodeOn(page);
        const searches = (await ask(page, "Jobs")).filter((j) => j.episode === path && j.kind === "search");
        await drag(page, box.x + box.width / 2, box.y + box.height / 2, share * track.width);
        const after = await mark();
        if (!after || (Math.abs(after.from - before.from) < 0.5 && Math.abs(after.to - before.to) < 0.5)) {
          wrong = `the window did not move: ${JSON.stringify(before)} before, ${JSON.stringify(after)} after`;
          break;
        }
        movedOff = { search: searches[searches.length - 1]?.id ?? "", window: after };
        await watch.step("move window", s);
        break;
      }
      case "searched moved": {
        const path = await episodeOn(page);
        const last = (await ask(page, "Jobs")).filter((j) => j.episode === path && j.kind === "search").pop();
        const w = movedOff?.window;
        const to = last && last.to > 0 ? last.to : Infinity;
        if (!last || !w) wrong = "there is no search, or no window was moved";
        else if (last.id === movedOff.search) wrong = `the search there was before the window moved was carried on, ${last.from}–${last.to} s`;
        else if (Math.abs((last.from ?? 0) - w.from) > 0.5 || Math.abs(to - w.to) > 0.5) {
          wrong = `the search looked at ${last.from}–${last.to} s, the window was moved to ${w.from}–${w.to} s`;
        }
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
        const said = await toastSays(page);
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
        const fps = await rate(page);
        const during = (from, to) => played.filter(([t]) => t >= from && t < to);
        // The frame that holds the playhead, a frame that begins a
        // millisecond or less after it included, see frameAt in
        // lib/flow.ts, as the bars carry it, its number modulo 2 ** BITS.
        const holds = (at) => Math.floor((at + 0.001) * fps) % 2 ** BITS;
        // Each still stretch is read from 200 ms after the hand stopped:
        // the press puts the playhead under the hand at once, and from
        // there it stands. The picture takes as long as the machine needs
        // to decode the frame under the hand, and the frames passing on
        // the way are drawn, so on a busy machine a frame was still on its
        // way 200 ms in. So the picture is read from where its frame
        // landed, which has to be within the stretch, and from there
        // nothing else is drawn.
        const stood = (from, to, what) => {
          const got = during(from + 200, to);
          if (got.length < 10) return `only ${got.length} animation frames were read ${what}`;
          const ats = new Set(got.map(([, at]) => at));
          if (ats.size > 1) {
            const all = got.map(([, at]) => at);
            return `the playhead moved from ${Math.min(...all).toFixed(3)} to ${Math.max(...all).toFixed(3)} ${what}`;
          }
          const at = got[0][1];
          const landed = got.findIndex(([, , n]) => n === holds(at));
          if (landed < 0 || got.length - landed < 3) {
            return `frame ${holds(at)}, which holds the playhead at ${at.toFixed(3)}, was not drawn ${what}, frames ${[...new Set(got.map(([, , n]) => n))].join(", ")} were`;
          }
          const other = got.slice(landed).find(([, , n]) => n !== holds(at));
          if (other) return `frame ${other[2]} was drawn after frame ${holds(at)} had landed ${what}`;
          return at;
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
      case "caption edge": {
        const [edge, text] = arg;
        const now = await engineState(page, watch.at);
        const i = now.captions.findIndex((c) => c.lines.some((l) => l.words.some((w) => w.text.includes(text))));
        if (i < 0) {
          wrong = `no caption has "${text}"`;
          break;
        }
        const label = `When caption ${i + 1} ${edge === "end" ? "goes" : "appears"}`;
        await page.locator(`.clip-timeline [aria-label="${label}"]`).first().click();
        await watch.step(what, s);
        break;
      }
      case "drawn": {
        // The video preview shows the caption holding the word where the
        // playhead stands: libass has drawn letters on the frame, and the
        // caption box over them holds the word. An edge dragged with shift
        // took the playhead to a moment between two frames, and the frame
        // shown was the one before the caption appears. Tim saw nothing to
        // snap to on #294.
        await settled();
        await page.waitForTimeout(300);
        await settled();
        const white = await page.evaluate(() => {
          const canvas = document.querySelector(".screen canvas");
          if (!canvas?.width) return 0;
          const c = document.createElement("canvas");
          c.width = canvas.width;
          c.height = canvas.height;
          const g = c.getContext("2d", { willReadFrequently: true });
          g.drawImage(canvas, 0, 0);
          const d = g.getImageData(0, 0, c.width, c.height).data;
          let n = 0;
          for (let i = 0; i < d.length; i += 4) if (d[i] > 220 && d[i + 1] > 220 && d[i + 2] > 220) n++;
          return n;
        });
        const box = (await shown(page)).map((w) => w.text).join(" ");
        const at = await page.evaluate(() => Number(document.querySelector(".screen")?.dataset.playhead));
        if (white < 100 || !box.includes(arg)) {
          wrong = `with the playhead at ${at.toFixed(3)} the video preview shows ${white} white pixels under the caption box "${box}", not the caption with "${arg}"`;
        }
        break;
      }
      case "captions drawn": {
        // The letters libass draws on the canvas, under the caption box,
        // once the layers asked for have come.
        await settled();
        await page.waitForTimeout(300);
        await settled();
        const white = await page.evaluate(() => {
          const canvas = document.querySelector(".screen canvas");
          if (!canvas?.width) return 0;
          const c = document.createElement("canvas");
          c.width = canvas.width;
          c.height = canvas.height;
          const g = c.getContext("2d", { willReadFrequently: true });
          g.drawImage(canvas, 0, 0);
          const d = g.getImageData(0, 0, c.width, c.height).data;
          let n = 0;
          for (let i = 0; i < d.length; i += 4) if (d[i] > 220 && d[i + 1] > 220 && d[i + 2] > 220) n++;
          return n;
        });
        const box = (await shown(page)).map((w) => w.text).join(" ");
        console.log(`    ${white} white pixels on the canvas under "${box}"`);
        if (white < 100) wrong = `the picture shows no captions, ${white} white pixels, under the caption box "${box}"`;
        break;
      }
      case "box": {
        const box = s.map((w) => w.text).join(" ");
        if (box !== arg) wrong = `the caption box reads "${box}", not "${arg}"`;
        break;
      }
      case "font":
      case "style": {
        const field = what === "style" ? "Style" : "Font";
        await page.locator(`button.pick[aria-label="${field}"]`).click();
        await page.locator(".pick-row", { hasText: new RegExp(`^\\s*${arg}\\s*$`) }).first().click();
        await watch.step("click", s);
        await page.waitForFunction((name) => document.querySelector(".captions")?.style.fontFamily.includes(name), arg);
        await settled();
        break;
      }
      case "search font": {
        const [letters, name] = arg.split("|");
        await page.locator('button.pick[aria-label="Font"]').click();
        await watch.step("click", s);
        await page.keyboard.type(letters);
        await watch.step("type", s);
        const first = await page.evaluate(() => document.querySelector(".pick-list .pick-row")?.textContent.trim());
        if (first !== name) {
          wrong = `${letters} found ${first ?? "nothing"} first, not ${name}`;
          await page.keyboard.press("Escape");
          break;
        }
        await page.keyboard.press("Enter");
        await watch.step("press", s);
        await page.waitForFunction((name) => document.querySelector(".captions")?.style.fontFamily.includes(name), name);
        await settled();
        break;
      }
      case "heard some": {
        const path = await episodeOn(page);
        const until = Date.now() + 60000;
        while (Date.now() < until && !(await ask(page, "Sentences", path))?.length) await page.waitForTimeout(250);
        // The piece of audio being heard as the speech model is held lands
        // before the search looks.
        await page.waitForTimeout(2500);
        await settled();
        break;
      }
      // The cards of what was said cannot be read while "on", and are
      // read again after. With "check", the search opens on cards that
      // cannot be read, says so in the list, and says nothing of it once
      // they can be read again.
      case "font then transcript": {
        // The font list open, then the transcript button pressed, found
        // by Tim: the list stayed open over the workspace with the
        // keyboard still in its field, and Escape closed the list first.
        await page.locator('button.pick[aria-label="Font"]').click();
        await watch.step("click", s);
        await page.waitForSelector(".pick-list .pick-row");
        await page.locator('.listhead button[aria-label="Search the transcript"]').click();
        await watch.step("click", s);
        await page.waitForTimeout(300);
        const seen = await page.evaluate(() => ({
          list: !!document.querySelector(".pick-list"),
          focused: document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.placeholder ?? document.activeElement?.tagName ?? null,
          field: !!document.querySelector(".listhead .search input"),
        }));
        console.log(JSON.stringify(seen));
        if (seen.list) wrong = "the font list stayed open after the transcript button was pressed";
        else if (!seen.field) wrong = "the transcript button opened no search";
        else if (seen.focused !== "Search transcript") wrong = `the keyboard is in ${seen.focused}, not the transcript's field`;
        if (!wrong) {
          await page.keyboard.press("Escape");
          await watch.step("press", s);
          const after = await page.evaluate(() => !!document.querySelector(".listhead .search input"));
          if (after) wrong = "Escape did not close the transcript search";
        }
        break;
      }
      case "said fails": {
        if (arg === "on") {
          await page.route("**/call", (route) => {
            const body = route.request().postDataJSON?.() ?? {};
            if (String(body.name ?? "").endsWith("Sentences")) return route.fulfill({ status: 500, body: "the disk went away" });
            return route.fallback();
          });
          break;
        }
        await page.locator('.listhead button[aria-label="Search the transcript"]').click();
        await page.waitForTimeout(500);
        const failing = await page.evaluate(() => {
          // The failure is the answer here, so it is taken off the calls
          // that failed before the rules read them.
          for (const c of window.__calls) if (c.name === "Sentences" && String(c.failed).includes("the disk went away")) c.failed = null;
          const li = document.querySelector("ol.said li.failed");
          return li ? { says: li.textContent.trim(), why: li.title } : null;
        });
        await watch.step("click", s);
        await page.unroute("**/call");
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        await page.locator('.listhead button[aria-label="Search the transcript"]').click();
        await watch.step("click", s);
        await page.waitForTimeout(500);
        const again = await page.evaluate(() => ({
          failed: !!document.querySelector("ol.said li.failed"),
          cards: document.querySelectorAll("ol.said li[data-at]").length,
        }));
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        if (!failing) wrong = "the cards could not be read and the list said nothing of it";
        else if (failing.says !== "The transcript could not be read" || !failing.why.includes("the disk went away"))
          wrong = `the list said "${failing.says}", with "${failing.why}" under the pointer`;
        else if (again.failed || !again.cards) wrong = `read again, the list still says it could not be read, over ${again.cards} cards`;
        break;
      }
      case "search said": {
        const path = await episodeOn(page);
        const headHigh = () => page.evaluate(() => document.querySelector(".listhead").getBoundingClientRect().height);
        // The clips are on their cards first, so a sentence can be told
        // which clip it is in.
        const listed = (await ask(page, "Clips", path)).filter((c) => !c.rejected).length;
        await page.waitForFunction((n) => document.querySelectorAll("aside ol li[data-key] button.pick").length >= n, listed, { timeout: 10000 });
        await settled();
        let [word, unheardLeast = 0] = [arg].flat();
        if (word === "*") {
          const first = (await ask(page, "Sentences", path))[0]?.text ?? "";
          word = first.split(/[^\p{L}]+/u).sort((a, b) => b.length - a.length)[0] ?? "";
        }
        const was = await headHigh();
        // The playhead far into the video, so the cards opening at it can
        // be told from the cards opening at their top.
        const track = await page.evaluate(() => {
          const r = document.querySelector(".track").getBoundingClientRect();
          return { x: r.left + r.width * 0.85, y: r.top + r.height / 2 };
        });
        await page.mouse.click(track.x, track.y);
        await watch.step("click", s);
        await page.mouse.move(0, 0);
        const icon = await page.evaluate(
          () => document.querySelector('.listhead button[aria-label="Search the transcript"] svg')?.getAttribute("class") ?? "",
        );
        // The sentences are on screen from the frame the transcript button is
        // pressed in, never an empty column first, found by Tim.
        const blank = await page.evaluate(
          () =>
            new Promise((done) => {
              document.querySelector('.listhead button[aria-label="Search the transcript"]').click();
              let n = 0;
              let empty = 0;
              const tick = () => {
                if (!document.querySelector("ol.said li")) empty++;
                if (++n < 20) requestAnimationFrame(tick);
                else done(empty);
              };
              requestAnimationFrame(tick);
            }),
        );
        await watch.step("click", s);
        if (blank) {
          wrong = `the column stood empty for ${blank} of 20 frames after the transcript button`;
          await page.keyboard.press("Escape");
          break;
        }
        // The cards open at the playhead, not where the clip list was
        // scrolled to, found in the review of #367. Once the cards read
        // as it opens have come.
        await page.waitForFunction(() => window.__pending === 0, null, { timeout: 10000 }).catch(() => {});
        await page.waitForTimeout(100);
        const opened = await page.evaluate(() => {
          const list = document.querySelector(".scroll.list");
          const ol = document.querySelector("ol.said");
          const card = [...ol.querySelectorAll("li[data-row]")].find((li) => li.dataset.row === ol.dataset.followed);
          if (!card) return null;
          const box = list.getBoundingClientRect();
          const end = list.scrollTop >= list.scrollHeight - list.clientHeight - 1;
          const r = card.getBoundingClientRect();
          return { top: r.top - box.top, should: parseFloat(getComputedStyle(ol).paddingTop), end, seen: r.top >= box.top && r.bottom <= box.bottom };
        });
        if (!opened) {
          wrong = `the cards opened at no card of the playhead, ${await page.evaluate(() => JSON.stringify({ f: document.querySelector("ol.said").dataset.followed ?? null, at: document.querySelector(".screen")?.dataset.playhead, n: document.querySelectorAll("ol.said li").length }))}`;
          await page.keyboard.press("Escape");
          break;
        }
        if (!(Math.abs(opened.top - opened.should) <= 1 || (opened.end && opened.seen))) {
          wrong = `the cards opened with the card of the playhead ${opened.top.toFixed(0)} px down, not at the top, ${opened.should} px`;
          await page.keyboard.press("Escape");
          break;
        }
        // What is typed is read from the top of the list, though the hand
        // scrolled the sentences before typing, found by Tim.
        await page.evaluate(() => {
          const list = document.querySelector(".scroll.list");
          list.scrollTop = list.scrollHeight;
        });
        const scrolledTo = await page.evaluate(() => document.querySelector(".scroll.list").scrollTop);
        // A letter most sentences hold, so the answers are still more than
        // the list shows and the list could have stayed where it was.
        await page.keyboard.type("e");
        await watch.step("type", s);
        await page.waitForTimeout(100);
        const typedTop = await page.evaluate(() => {
          const list = document.querySelector(".scroll.list");
          return { top: list.scrollTop, more: list.scrollHeight - list.clientHeight };
        });
        await page.keyboard.press("Backspace");
        await page.keyboard.type(word);
        await watch.step("type", s);
        if (scrolledTo > 0 && typedTop.more > 0 && typedTop.top !== 0) {
          wrong = `typing in the search left the list ${typedTop.top} px down, where the hand had scrolled it to ${scrolledTo} px`;
          await page.keyboard.press("Escape");
          await page.keyboard.press("Escape");
          break;
        }
        const [sentences, clips, library] = await Promise.all([
          ask(page, "Sentences", path),
          ask(page, "Clips", path),
          ask(page, "Library"),
        ]);
        const heard = library.find((e) => e.source === path)?.heard ?? [];
        // The parts not heard yet, the way the workspace works them out
        // from what the transcript on disk has heard.
        const status = library.find((e) => e.source === path);
        const end = (await ask(page, "Source", path))?.duration;
        await page.waitForFunction(
          (n) => document.querySelectorAll("ol.said li:not(.hole):not(.none)").length >= n,
          sentences.filter((x) => x.text.toLowerCase().includes(word.toLowerCase())).length,
          { timeout: 10000 },
        ).catch(() => {});
        const seen = await page.evaluate(() => ({
          focused: document.activeElement?.getAttribute("aria-label"),
          head: document.querySelector(".listhead").getBoundingClientRect().height,
          cards: [...document.querySelectorAll("ol.said li:not(.hole):not(.none)")].map((li) => ({
            at: Number(li.dataset.at),
            text: li.querySelector(".text").textContent.trim(),
            bold: [...li.querySelectorAll(".text b")].map((b) => b.textContent),
            clip: li.querySelector(".clipname")?.textContent.trim() ?? "",
          })),
          holes: document.querySelectorAll("ol.said li.hole").length,
          edges: document.querySelectorAll(".unheard-edge").length,
          count: document.querySelector(".listhead .search .saycount")?.textContent.trim() ?? null,
          // The greys, as the browser has them: what no clip keeps, a
          // clip's words, a word that answers, the field's placeholder,
          // and a label's grey to measure them by.
          grey: (() => {
            const of = (sel, pseudo) => {
              const el = document.querySelector(sel);
              return el ? getComputedStyle(el, pseudo).color : null;
            };
            const probe = document.createElement("span");
            probe.style.color = "var(--muted)";
            document.body.append(probe);
            const muted = getComputedStyle(probe).color;
            probe.style.color = "var(--text)";
            const white = getComputedStyle(probe).color;
            // The grey the app gives what a field is for, which the browser
            // has a grey of its own for that, a lighter one in WebKit.
            probe.style.color = "color-mix(in srgb, var(--muted) 45%, var(--faint))";
            const unwritten = getComputedStyle(probe).color;
            probe.remove();
            return {
              out: of("ol.said li:not([data-clip]):not(.hole):not(.none) .text"),
              clip: of("ol.said li[data-clip] .text"),
              bold: of("ol.said li .text b"),
              placeholder: of('input[aria-label="Search transcript"]', "::placeholder"),
              muted,
              white,
              unwritten,
            };
          })(),
          says: document.querySelector('input[aria-label="Search transcript"]')?.placeholder ?? null,
        }));
        // How bright a colour of the browser's is, 0 to 255.
        const bright = (c) => {
          // A colour mixed in the stylesheet comes back as color(srgb ...),
          // from 0 to 1.
          const scale = c?.startsWith("color(") ? 255 : 1;
          const [r, g, b] = (c?.replace(/^color\(srgb/, "").match(/[\d.]+/g) ?? []).map((v) => Number(v) * scale);
          return c ? 0.2126 * r + 0.7152 * g + 0.0722 * b : NaN;
        };
        const holds = sentences.filter((x) => x.text.toLowerCase().includes(word.toLowerCase()));
        const texts = new Set(sentences.map((x) => x.text));
        const gaps = (() => {
          if (status?.transcribed) return 0;
          let at = 0;
          let n = 0;
          for (const [a, b] of [...heard].sort((x, y) => x[0] - y[0])) {
            if (a - at >= 1) n++;
            at = Math.max(at, b);
          }
          if (Number.isFinite(end) && end - at >= 1) n++;
          return n;
        })();
        // The clips the engine has showing a card at a moment, two where
        // clips are over each other.
        const namesAt = (c) =>
          sentences
            .filter((y) => Math.abs(y.start - c.at) < 0.0005)
            .map((y) => {
              const k = y.clip ? clips.find((q) => q.key === y.clip) : null;
              return k ? k.title || k.slug : "";
            });
        // A card named for a clip holds only what the clip keeps, to
        // within the word an edge falls in, found by Tim.
        const spills = (c) => {
          const x = sentences.find((y) => Math.abs(y.start - c.at) < 0.0005);
          const k = c.clip && clips.find((q) => !q.rejected && (q.title || q.slug) === c.clip);
          if (!x || !k) return false;
          const parts = k.segments.length ? k.segments : [k];
          const kept = (t) => parts.some((p) => t >= p.start - 0.5 && t <= p.end + 0.5);
          return !kept(x.start) || !kept(x.end);
        };
        const misnamed = (c) => !namesAt(c).includes(c.clip);
        // A clip's cards are its captions, one card a caption, reading
        // what the caption shows, found by Tim, a clip over another too.
        const flat = (t) => t.replace(/[\s\u00ad-]/g, "");
        let held = 0;
        const unlike = (
          await Promise.all(
            clips.filter((k) => !k.rejected).map(async (k) => {
              const view = await ask(page, "Captions", path, k.plan, k.id);
              const blocks = (view?.captions ?? []).map((c) => c.lines.flatMap((l) => l.words.map((w) => w.text)).join(" ")).filter((t) => t).map(flat);
              const cards = sentences.filter((x) => x.clip === k.key).map((x) => x.text);
              held += cards.length;
              if (cards.length === blocks.length && cards.every((t, i) => flat(t) === blocks[i])) return null;
              return `${k.title || k.slug} shows ${blocks.length} captions and has ${cards.length} cards, ${JSON.stringify(blocks.slice(0, 2))} against ${JSON.stringify(cards.slice(0, 2).map(flat))}`;
            }),
          )
        ).find(Boolean);
        if (seen.focused !== "Search transcript") wrong = `the keyboard is in ${seen.focused ?? "nothing"}, not the search field`;
        else if (seen.head !== was) wrong = `the head is ${seen.head} px high with the field, ${was} px without it`;
        else if (!holds.length) wrong = `the engine has no sentence with ${word}, so the step proves nothing`;
        else if (holds.some((x) => !seen.cards.some((c) => Math.abs(c.at - x.start) < 0.0005)))
          wrong = `no card for "${holds.find((x) => !seen.cards.some((c) => Math.abs(c.at - x.start) < 0.0005)).text}"`;
        // Only the cards that hold the word, where any does: three letters
        // answer a lot of words the way a font answers its name, found in
        // the review of #367.
        else if (seen.cards.length !== holds.length) wrong = `${seen.cards.length} cards answer ${word}, and ${holds.length} hold it`;
        else if (seen.cards.some((c) => !texts.has(c.text))) wrong = `a card reads "${seen.cards.find((c) => !texts.has(c.text)).text}", which the engine never said`;
        else if (seen.cards.some((c) => !c.bold.length)) wrong = `"${seen.cards.find((c) => !c.bold.length).text}" has no word in bold`;
        else if (seen.cards.some((c) => c.text.toLowerCase().includes(word.toLowerCase()) && !c.bold.some((b) => b.toLowerCase().includes(word.toLowerCase()))))
          wrong = `${word} is not bold in a card that holds it`;
        else if (seen.cards.some((c) => !sentences.some((y) => Math.abs(y.start - c.at) < 0.0005 && y.text === c.text)))
          wrong = `the card at ${seen.cards.find((c) => !sentences.some((y) => Math.abs(y.start - c.at) < 0.0005)).at} is of no sentence the engine has there`;
        else if (sentences.some((x) => x.end - x.start > 12))
          wrong = `a card of what was said runs ${(() => { const x = sentences.find((y) => y.end - y.start > 12); return `${(x.end - x.start).toFixed(1)} s from ${x.start.toFixed(1)}`; })()}, more than the 12 s a card may, found by Tim`;
        else if (seen.cards.some(spills))
          wrong = `the card at ${seen.cards.find(spills).at.toFixed(2)} is named for its clip and holds what the clip leaves out`;
        else if (seen.cards.some(misnamed)) {
          const c = seen.cards.find(misnamed);
          wrong = `the card at ${c.at.toFixed(2)} names ${c.clip || "no clip"}, and the engine has it in ${namesAt(c).join(", ") || "no clip"}`;
        }
        else if (unlike) wrong = unlike;
        // What no clip keeps reads in a label's grey, a clip's words a step
        // above it and below white, and a word that answers in white, so it
        // stands out, found by Tim.
        else if (seen.grey.out && seen.grey.out !== seen.grey.muted) wrong = `what no clip keeps reads in ${seen.grey.out}, not the grey of a label, ${seen.grey.muted}`;
        else if (seen.grey.clip && !(bright(seen.grey.clip) > bright(seen.grey.muted) && bright(seen.grey.clip) < bright(seen.grey.white)))
          wrong = `a clip's words read in ${seen.grey.clip}, not between ${seen.grey.muted} and ${seen.grey.white}`;
        else if (seen.grey.bold !== seen.grey.white) wrong = `a word that answers reads in ${seen.grey.bold}, not ${seen.grey.white}`;
        // The field says what it is in two words, darker than what is typed
        // and than a label, so it does not read as typed, and the button
        // that opens it wears the transcript, found by Tim.
        else if (seen.says !== "Search transcript") wrong = `the field says "${seen.says}"`;
        else if (seen.grey.placeholder !== seen.grey.unwritten || !(bright(seen.grey.placeholder) < bright(seen.grey.muted)))
          wrong = `the field's placeholder is ${seen.grey.placeholder}, not ${seen.grey.unwritten}, darker than ${seen.grey.muted}`;
        else if (!icon.includes("lucide-message-square-quote")) wrong = `the button that opens the search wears ${icon || "nothing"}`;
        else if (!held && clips.some((k) => !k.rejected)) wrong = "no clip has a card, so a clip's cards could not be held to its captions";
        else if (gaps < unheardLeast)
          wrong = `${gaps} parts are not transcribed yet, fewer than the ${unheardLeast} this step is about: heard ${JSON.stringify(heard)} of ${end}, transcribed ${status?.transcribed}`;
        // A search answers with sentences only. The parts not transcribed
        // yet stay on the range picker, found by Tim.
        else if (seen.holes || seen.edges !== gaps)
          wrong = `${gaps} parts are not transcribed yet, and with ${word} typed the list shows ${seen.holes} of them and the range picker ${seen.edges}`;
        else if (seen.count !== String(seen.cards.length)) wrong = `the field says ${seen.count ?? "nothing"} match, over ${seen.cards.length} cards`;
        else if (!scrolledTo || typedTop.more <= 0)
          wrong = `the sentences with an e scroll ${typedTop.more} px and the hand scrolled to ${scrolledTo} px, so typing could not be seen to go back to the top`;
        if (wrong) {
          await page.keyboard.press("Escape");
          await page.keyboard.press("Escape");
          break;
        }
        // What nothing said matches says so in two words, with no parts
        // not transcribed yet around them, found by Tim.
        const field = page.locator('input[aria-label="Search transcript"]');
        await field.fill("qxzvqxzv");
        await watch.step("type", s);
        await page.waitForTimeout(200);
        const none = await page.evaluate(() => ({
          says: document.querySelector("ol.said li.none")?.textContent.trim() ?? null,
          rows: document.querySelectorAll("ol.said li:not(.none)").length,
          count: document.querySelector(".listhead .search .saycount")?.textContent.trim() ?? null,
        }));
        if (none.says !== "No matches" || none.rows || none.count !== "0")
          wrong = `nonsense typed shows ${none.rows} cards over "${none.says}", and the field says ${none.count}`;
        // Three letters a few words hold, and more words have in that order
        // the way a font has its name, sechs and verstaucht for sch: only
        // the cards that hold them, found in the review of #367.
        await field.fill("sch");
        await watch.step("type", s);
        await page.waitForTimeout(200);
        const short = await page.evaluate(() => document.querySelectorAll("ol.said li[data-at]").length);
        const holdShort = sentences.filter((x) => x.text.toLowerCase().includes("sch")).length;
        if (!wrong && holdShort && short !== holdShort) wrong = `${short} cards answer sch, and ${holdShort} hold it`;
        // The word typed with a letter twice, the way Tim typed arrm for
        // arm, still finds every card that holds the word.
        const slip = word.slice(0, 2) + word.slice(1);
        await field.fill(slip);
        await watch.step("type", s);
        await page.waitForTimeout(200);
        const slipped = await page.evaluate(() => [...document.querySelectorAll("ol.said li[data-at]")].map((li) => Number(li.dataset.at)));
        const missed = holds.find((x) => !slipped.some((at) => Math.abs(at - x.start) < 0.0005));
        if (!wrong && missed) wrong = `${slip} typed for ${word} finds ${slipped.length} cards and not "${missed.text}"`;
        // With nothing typed, the whole of what was said, and a dashed card
        // for every part not transcribed yet.
        await field.fill("");
        await watch.step("type", s);
        await page.waitForTimeout(200);
        const whole = await page.evaluate(() => ({
          holes: document.querySelectorAll("ol.said li.hole").length,
          count: document.querySelector(".listhead .search .saycount")?.textContent.trim() ?? null,
        }));
        if (!wrong && whole.holes !== gaps) wrong = `${gaps} parts are not transcribed yet, and with nothing typed the list shows ${whole.holes}`;
        else if (!wrong && whole.count !== null) wrong = `with nothing typed the field still says ${whole.count} match`;
        if (wrong) {
          await page.keyboard.press("Escape");
          break;
        }
        // The edge of a sentence a clip keeps stays under the hand, and the
        // hand on it lights the clip's mark on the range picker, found by
        // Tim.
        const inClip = seen.cards.find((c) => c.clip);
        if (inClip) {
          const card = page.locator(`ol.said li[data-at="${inClip.at}"]`);
          await card.hover();
          await watch.step("hover", s);
          const lit = await page.evaluate((at) => {
            const li = document.querySelector(`ol.said li[data-at="${at}"]`);
            const key = li.dataset.clip;
            const grey = getComputedStyle(li.querySelector("button.pick")).backgroundColor;
            // Only the card under the hand is lit, not every sentence of
            // its clip with it, found by Tim.
            const others = [...document.querySelectorAll(`ol.said li[data-clip="${key}"]`)].filter((x) => x !== li);
            return {
              edge: getComputedStyle(li.querySelector("button.pick")).boxShadow,
              mark: !!document.querySelector(".track .clipmark.lit"),
              others: others.length,
              alsoLit: others.filter((x) => getComputedStyle(x.querySelector("button.pick")).backgroundColor === grey).length,
              key,
            };
          }, inClip.at);
          if (!/3px 0px 0px 0px/.test(lit.edge)) wrong = `the edge of the card at ${inClip.at.toFixed(2)} is gone under the hand: ${lit.edge}`;
          else if (!lit.mark) wrong = `the hand on a sentence of ${inClip.clip} lit no mark on the range picker`;
          else if (lit.alsoLit) wrong = `the hand on one sentence of ${inClip.clip} lit ${lit.alsoLit} of its ${lit.others} other sentences too`;
        }
        // The hand going from one card of a clip to the next is on a card
        // all the way, so the clip stays lit everywhere else and does not
        // flicker, found by Tim.
        if (!wrong && clips.some((k) => !k.rejected)) {
          await field.fill("");
          await watch.step("type", s);
          await page.waitForTimeout(200);
          const pair = await page.evaluate(() => {
            const cards = [...document.querySelectorAll("ol.said li[data-clip]")];
            const i = cards.findIndex((li, i) => li.nextElementSibling === cards[i + 1] && cards[i + 1]?.dataset.clip === li.dataset.clip);
            if (i < 0) return null;
            cards[i].scrollIntoView({ block: "center" });
            const [a, b] = [cards[i].getBoundingClientRect(), cards[i + 1].getBoundingClientRect()];
            return { x: a.left + a.width / 2, from: a.top + a.height / 2, to: b.top + b.height / 2 };
          });
          if (!pair) wrong = "no clip has two cards one after the other, so going from one to the next could not be seen";
          else {
            let off = 0;
            let dark = 0;
            for (let y = pair.from; y <= pair.to; y += 1) {
              await page.mouse.move(pair.x, y);
              const on = await page.evaluate(() => ({
                card: !!document.querySelector("ol.said li:hover"),
                mark: !!document.querySelector(".track .clipmark.lit"),
              }));
              if (!on.card) off++;
              if (!on.mark) dark++;
            }
            await watch.step("hover", s);
            if (off || dark)
              wrong = `the hand going from one card of a clip to the next was on no card for ${off} px and the clip's mark went dark for ${dark} px`;
          }
        }
        // The hand on a part not transcribed yet lights it on the range
        // picker too.
        if (!wrong && gaps) {
          await page.locator("ol.said li.hole").first().hover();
          await watch.step("hover", s);
          // Lit the way every card is under the hand, the dashes gone into
          // the grey they fill with, in the list and on the range picker,
          // and dashed in the grey of a card's edge until then, found by
          // Tim.
          const look = await page.evaluate(() => {
            const hole = getComputedStyle(document.querySelector("ol.said li.hole"));
            const fill = document.querySelector(".track .unheard.lit");
            const edge = document.querySelector(".unheard-edge.lit");
            const ring = getComputedStyle(document.querySelector("ol.said li:not(.hole):not(.none)")).boxShadow;
            const dots = document.querySelector("ol.said li.hole .dots");
            return {
              lit: !!fill && !!edge,
              dots: [dots && getComputedStyle(dots), fill && getComputedStyle(fill, "::before")].map((d) => (d ? [d.backgroundImage, d.maskImage] : null)),
              card: [hole.borderTopColor, hole.backgroundColor],
              picker: fill && edge
                ? [getComputedStyle(edge, "::before").borderTopColor, getComputedStyle(edge, "::after").borderTopColor, getComputedStyle(document.querySelector(".track")).borderTopColor]
                : [],
              ring,
            };
          });
          await page.mouse.move(0, 0);
          const rest = await page.evaluate(() => [
            getComputedStyle(document.querySelector("ol.said li.hole")).borderTopColor,
            getComputedStyle(document.querySelector(".unheard-edge"), "::after").borderTopColor,
          ]);
          // Halftone dots fill both, faded in at rest, from the left on the
          // card and over 24 px at each end on the range picker, and whole
          // under the hand. The dashes stay, so a part a few pixels wide
          // still shows, found by Tim.
          const still = await page.evaluate(() =>
            [document.querySelector("ol.said li.hole .dots"), document.querySelector(".track .unheard")].map((d, i) => {
              if (!d) return null;
              const st = i ? getComputedStyle(d, "::before") : getComputedStyle(d);
              return [st.backgroundImage, st.maskImage];
            }),
          );
          const dotted = (d) => !!d && d[0].includes("radial-gradient");
          if (!look.lit) wrong = "the hand on a part not transcribed yet lit nothing on the range picker";
          else if (!still.every(dotted) || !look.dots.every(dotted))
            wrong = `a part not transcribed yet is not dotted: at rest ${JSON.stringify(still)}, under the hand ${JSON.stringify(look.dots)}`;
          else if (!still[0][1].includes("linear-gradient") || !still[1][1].includes("24px"))
            wrong = `the dots of a part not transcribed yet do not fade in at rest: ${still[0][1]} on the card, ${still[1][1]} on the range picker`;
          else if (look.dots.some((d) => d[1] !== "none")) wrong = `the dots under the hand are still faded: ${JSON.stringify(look.dots)}`;
          else if (look.card[0] !== look.card[1]) wrong = `the dashed card under the hand has its edge in ${look.card[0]} over ${look.card[1]}, not gone into it`;
          // On the range picker the border is the range picker's, and it
          // stays over the part lit, found by Tim.
          else if (look.picker[0] !== "rgba(0, 0, 0, 0)" || look.picker[1] !== "rgba(0, 0, 0, 0)" || look.picker[2] === "rgba(0, 0, 0, 0)")
            wrong = `the dashed part on the range picker, lit, covers the border in ${look.picker[0]} and ${look.picker[1]}, over a border in ${look.picker[2]}`;
          else if (!look.ring.includes(rest[0]) || !look.ring.includes(rest[1]))
            wrong = `the dashes are ${rest[0]} on the card and ${rest[1]} on the range picker, and a card's edge is ${look.ring}`;
        }
        // The playhead put in a part not transcribed yet, on the range
        // picker, puts that part's card at the top of the list, not the
        // next words heard after it, found by Tim.
        if (!wrong && gaps) {
          await page.evaluate(() => (document.querySelector(".scroll.list").style.overflowAnchor = "none"));
          const part = await page.evaluate(() => {
            const r = [...document.querySelectorAll(".track .unheard")].at(-1).getBoundingClientRect();
            return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
          });
          await page.mouse.click(part.x, part.y);
          await watch.step("click", s);
          await page.waitForTimeout(300);
          const there = await page.evaluate(() => {
            const list = document.querySelector(".scroll.list");
            const box = list.getBoundingClientRect();
            const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
            // At the top, or as near it as the end of the list lets it.
            const end = list.scrollTop >= list.scrollHeight - list.clientHeight - 1;
            const top = [...document.querySelectorAll("ol.said li[data-at]")].find((li) => {
              const r = li.getBoundingClientRect();
              return end ? r.top >= box.top + pad - 1 && r.bottom <= box.bottom : Math.abs(r.top - box.top - pad) <= 1;
            });
            const hole = end ? [...document.querySelectorAll("ol.said li.hole")].at(-1) : top;
            const r = hole?.getBoundingClientRect();
            return {
              at: Number(document.querySelector(".screen")?.dataset.playhead),
              hole:
                !!hole?.classList.contains("hole") &&
                !!r &&
                r.top >= box.top &&
                r.bottom <= box.bottom &&
                (end || hole === top) &&
                hole.dataset.row === document.querySelector("ol.said").dataset.followed,
              top: top ? top.querySelector(".meta").textContent.trim() : null,
            };
          });
          const hole = (() => {
            let at = 0;
            for (const [a, b] of [...heard].sort((x, y) => x[0] - y[0])) {
              if (a - at >= 1 && there.at >= at && there.at < a) return true;
              at = Math.max(at, b);
            }
            return there.at >= at;
          })();
          if (!hole) wrong = `a press in the last part not transcribed yet put the playhead at ${there.at}, which is heard`;
          else if (!there.hole) wrong = `the playhead in a part not transcribed yet, at ${there.at.toFixed(1)}, left ${there.top ?? "no card"} at the top of the list, not that part's card`;
        }
        // A clip chosen on the range picker brings its first sentence into
        // view, found by Tim.
        const others = clips.filter((c) => !c.rejected);
        if (!wrong && others.length > 1) {
          // With nothing typed, so every sentence of the clip has a card.
          await page.locator('input[aria-label="Search transcript"]').fill("");
          await watch.step("type", s);
          await page.evaluate(() => {
            const list = document.querySelector(".scroll.list");
            list.scrollTop = list.scrollHeight;
          });
          const mark = page.locator(".track .clipmark:not(.selected)").last();
          const label = await mark.getAttribute("aria-label");
          const clockOf = (t) => `${Math.floor(t / 60)}:${String(Math.floor(t % 60)).padStart(2, "0")}`;
          const target = others.find((c) => `Clip at ${clockOf(c.start)}` === label);
          const parts = target ? (target.segments.length ? target.segments : [target]) : [];
          const firstAt = sentences.find((x) => x.clip === target?.key)?.start;
          // Where the card stands once the list stops moving, against where
          // the first card of the list stands, which is the top.
          const standing = () =>
            page.evaluate(
              ([at, key]) =>
                new Promise((done) => {
                  const list = document.querySelector(".scroll.list");
                  let last = -1;
                  let still = 0;
                  const tick = () => {
                    if (list.scrollTop === last) still++;
                    else still = 0;
                    last = list.scrollTop;
                    if (still < 10) return requestAnimationFrame(tick);
                    const card = document.querySelector(`ol.said li[data-at="${at}"][data-clip="${key}"]`);
                    const head = document.querySelector("ol.said");
                    const box = list.getBoundingClientRect();
                    const r = card?.getBoundingClientRect();
                    done(r ? { top: r.top - box.top, should: parseFloat(getComputedStyle(head).paddingTop), room: box.height } : null);
                  };
                  requestAnimationFrame(tick);
                }),
              [firstAt, target?.key],
            );
          // WebKit, which the app is drawn by on the Mac, keeps no anchor
          // when what is above the place in view changes size, and
          // Chromium does, which hid cards laid out late moving the place
          // a clip went to, found by Tim.
          await page.evaluate(() => (document.querySelector(".scroll.list").style.overflowAnchor = "none"));
          const chosen = [];
          for (const again of [false, true]) {
            // Chosen again after the hand scrolled away, it is gone to again.
            if (again)
              await page.evaluate(() => {
                const list = document.querySelector(".scroll.list");
                list.scrollTop = list.scrollHeight;
              });
            await (again ? page.locator(".track .clipmark.selected") : mark).click();
            await watch.step("click", s);
            chosen.push(await standing());
          }
          const off = chosen.find((v) => v && Math.abs(v.top - v.should) > 1);
          if (!target || chosen.some((v) => !v))
            wrong = `the mark ${label} is of no clip with sentences on screen`;
          else if (off)
            wrong = `choosing ${target.title} on the range picker${off === chosen[1] ? " a second time" : ""} left its first sentence ${off.top.toFixed(1)} px down a list ${off.room.toFixed(0)} px high, not at its top, ${off.should} px`;
        }
        // The playhead put on a caption on the clip timeline puts the card
        // of that caption at the top of the list, the way a clip chosen
        // does, found by Tim.
        if (!wrong && clips.some((k) => !k.rejected)) {
          await field.fill("");
          await watch.step("type", s);
          // The chosen clip's mark, so the clip timeline is on the clip and
          // not where the playhead was put before the search opened.
          await page.evaluate(() => document.querySelector(".track .clipmark.selected").click());
          await watch.step("click", s);
          await page.waitForTimeout(200);
          await page.evaluate(() => {
            const list = document.querySelector(".scroll.list");
            list.style.overflowAnchor = "none";
            list.scrollTop = list.scrollHeight;
          });
          const blocks = page.locator(".captions .caption");
          const n = await blocks.count();
          // The last block the clip timeline shows. It is zoomed in on the
          // clip, so after a caption was corrected its last blocks can be
          // past the edge of the app.
          const shown = await page.evaluate(() =>
            [...document.querySelectorAll(".captions .caption")].findLastIndex((el) => {
              const box = el.getBoundingClientRect();
              return box.width > 0 && box.left >= 0 && box.right <= innerWidth;
            }),
          );
          if (n < 2 || shown < 1) wrong = `the chosen clip has ${n} captions on the clip timeline, ${shown + 1} of them in view, so going to one could not be seen}`;
          else {
            await blocks.nth(shown).click();
            await watch.step("click", s);
            const stood = await page.evaluate(
              () =>
                new Promise((done) => {
                  const list = document.querySelector(".scroll.list");
                  let last = -1;
                  let still = 0;
                  const tick = () => {
                    if (list.scrollTop === last) still++;
                    else still = 0;
                    last = list.scrollTop;
                    if (still < 10) return requestAnimationFrame(tick);
                    const box = list.getBoundingClientRect();
                    const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
                    const top = [...document.querySelectorAll("ol.said li[data-at]")].find((li) => Math.abs(li.getBoundingClientRect().top - box.top - pad) <= 1);
                    done({ at: Number(document.querySelector(".screen")?.dataset.playhead), top: top ? Number(top.dataset.at) : null, scrolled: list.scrollTop });
                  };
                  requestAnimationFrame(tick);
                }),
            );
            const card = sentences.find((x) => x.end > stood.at);
            if (!card) wrong = `no card after the playhead at ${stood.at}`;
            else if (stood.top === null || Math.abs(stood.top - card.start) > 0.0005)
              wrong = `the playhead put on a caption at ${stood.at.toFixed(2)} left ${stood.top === null ? "no card" : `the card at ${stood.top.toFixed(2)}`} at the top of the list, not the card at ${card.start.toFixed(2)}, "${card.text}"`;
            else if (!card.clip) wrong = `the caption at ${stood.at.toFixed(2)} on the clip timeline is in a card of no clip`;
          }
        }
        // The clip played from its mark: its first caption stands at the
        // top, and as the playhead comes into the next caption, that card
        // stands at the top, found by Tim.
        if (!wrong && clips.some((k) => !k.rejected)) {
          await page.locator(".track .clipmark.selected").click();
          await watch.step("click", s);
          const chosenLabel = await page.locator(".track .clipmark.selected").getAttribute("aria-label");
          const chosenKey = clips.find((k) => `Clip at ${Math.floor(k.start / 60)}:${String(Math.floor(k.start % 60)).padStart(2, "0")}` === chosenLabel)?.key;
          const own = sentences.filter((x) => x.clip === chosenKey);
          // The card at the top of the list, and the one the list follows.
          const topRow = () =>
            page.evaluate(() => {
              const list = document.querySelector(".scroll.list");
              const box = list.getBoundingClientRect();
              const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
              const top = [...document.querySelectorAll("ol.said li[data-row]")].find((li) => Math.abs(li.getBoundingClientRect().top - box.top - pad) <= 1);
              const followed = document.querySelector("ol.said").dataset.followed ?? null;
              const li = followed && document.querySelector(`ol.said li[data-row="${CSS.escape(followed)}"]`);
              const end = list.scrollTop + list.clientHeight >= list.scrollHeight - 1;
              const shown = li && li.getBoundingClientRect().top >= box.top && li.getBoundingClientRect().bottom <= box.bottom;
              return { top: top?.dataset.row ?? null, followed, clip: li?.dataset.clip ?? null, standing: !!followed && (top?.dataset.row === followed || (end && shown)) };
            });
          // A search let go of without moving the playhead goes back to the
          // card of the playhead, by Escape or by taking the words away,
          // not to the top of the whole transcript, found by Tim.
          if (own.length >= 2) {
            for (const how of ["Escape", "erase"]) {
              const before = await topRow();
              await field.fill(word);
              await watch.step("type", s);
              await page.waitForTimeout(200);
              if (how === "Escape") await field.press("Escape");
              else await field.fill("");
              await watch.step(how === "Escape" ? "press" : "type", s);
              await page.waitForTimeout(200);
              const after = await topRow();
              if (!wrong && (!before.standing || before.clip !== chosenKey))
                wrong = `the clip chosen on the range picker left ${before.top} at the top, not its card ${before.followed}`;
              else if (!wrong && (!after.standing || after.followed !== before.followed))
                wrong = `a search let go of by ${how} left ${after.top} at the top of the list, not ${before.followed}, the card of the playhead it stood at before`;
            }
          }
          if (own.length < 2) wrong = `the chosen clip has ${own.length} cards, so playing into its next one could not be seen`;
          else if (!wrong) {
            // The hand scrolls away, and the space bar is pressed in the
            // empty search field: it plays, the field lets go of the
            // keyboard, and the card of the playhead comes to the top at
            // once, not at the next caption, found by Tim.
            await page.evaluate(() => {
              const list = document.querySelector(".scroll.list");
              list.scrollTop = list.scrollHeight;
            });
            await field.focus();
            await page.keyboard.press("Space");
            await watch.step("press", s);
            await page.waitForTimeout(250);
            const started = await page.evaluate(() => ({
              typed: document.querySelector(".listhead .search input")?.value ?? null,
              focused: document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.tagName ?? null,
              playing: document.querySelector('button[aria-label="Pause"]') !== null,
              at: Number(document.querySelector(".screen")?.dataset.playhead),
            }));
            const first = await topRow();
            if (started.typed !== "") wrong = `the space bar in the empty search field typed "${started.typed}"`;
            else if (started.focused === "Search transcript") wrong = "the space bar in the empty search field left the keyboard in it";
            else if (!started.playing) wrong = "the space bar in the empty search field did not play";
            else if (started.at < own[1].start && !first.standing)
              wrong = `playing from ${started.at.toFixed(2)} left ${first.top} at the top of a list scrolled away, not the card of the playhead ${first.followed}`;
            const until = Date.now() + 20000;
            let seen = null;
            while (Date.now() < until) {
              seen = await page.evaluate(() => {
                const list = document.querySelector(".scroll.list");
                const box = list.getBoundingClientRect();
                const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
                const top = [...document.querySelectorAll("ol.said li[data-at]")].find((li) => Math.abs(li.getBoundingClientRect().top - box.top - pad) <= 1);
                return { at: Number(document.querySelector(".screen")?.dataset.playhead), top: top ? Number(top.dataset.at) : null, clip: top?.dataset.clip ?? null };
              });
              if (seen.at >= own[1].start + 0.3) break;
              await page.waitForTimeout(100);
            }
            await page.keyboard.press("Space");
            await watch.step("press", s);
            // Stopping gives the search field the keyboard back, the one
            // the space bar took it from, found by Tim.
            await page.waitForTimeout(200);
            const stopped = await page.evaluate(() => document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.tagName ?? null);
            // The card the playhead is in, or the one before it in a pause.
            const now = own.filter((x) => x.start <= seen.at).at(-1);
            if (wrong);
            else if (stopped !== "Search transcript") wrong = `stopping with the space bar left the keyboard on ${stopped}, not back in the search field`;
            else if (!(seen.at >= own[1].start + 0.3)) wrong = `the clip played to ${seen.at.toFixed(2)} and never reached its second caption at ${own[1].start.toFixed(2)}`;
            else if (seen.top === null || Math.abs(seen.top - now.start) > 0.0005 || seen.clip !== chosenKey)
              wrong = `the clip played to ${seen.at.toFixed(2)} with ${seen.top === null ? "no card" : `the card at ${seen.top.toFixed(2)}`} at the top of the list, not its caption at ${now.start.toFixed(2)}, "${now.text}"`;
          }
        }
        // A card puts the playhead on its sentence, and a sentence of a
        // clip makes the clip the chosen one, found by Tim, with the list
        // left where it is. With nothing typed, so the list can move, and
        // a card of a clip not chosen yet that is not its first, so going
        // to the clip's first card would move it.
        await field.fill("");
        await watch.step("type", s);
        await page.waitForTimeout(200);
        const clockAt = (t) => `${Math.floor(t / 60)}:${String(Math.floor(t % 60)).padStart(2, "0")}`;
        const pickAt = await page.evaluate(
          (labels) => {
            const chosen = document.querySelector(".track .clipmark.selected")?.getAttribute("aria-label");
            const byClip = Map.groupBy([...document.querySelectorAll("ol.said li[data-clip]")], (li) => li.dataset.clip);
            const other = [...byClip].find(([key, list]) => labels[key] !== chosen && list.length > 1);
            return other ? { at: Number(other[1][1].dataset.at), row: other[1][1].dataset.row } : null;
          },
          Object.fromEntries(clips.map((k) => [k.key, `Clip at ${clockAt(k.start)}`])),
        );
        const pick = pickAt && sentences.find((x) => Math.abs(x.start - pickAt.at) < 0.0005);
        if (!pick && others.length > 1) wrong = "no clip but the chosen one has two cards, so choosing one by its sentence could not be seen";
        if (pick) {
          const row = `ol.said li[data-row="${pickAt.row}"]`;
          const button = page.locator(`${row} button.pick`);
          await button.scrollIntoViewIfNeeded();
          const stood = await page.evaluate((row) => document.querySelector(row).getBoundingClientRect().top, row);
          await button.click();
          await watch.step("click", s);
          await page.waitForTimeout(300);
          const after = await page.evaluate((row) => {
            const li = document.querySelector(row);
            return {
              at: Number(document.querySelector(".screen")?.dataset.playhead),
              top: li.getBoundingClientRect().top,
              key: li.dataset.clip ?? null,
              chosen: document.querySelector(".track .clipmark.selected")?.getAttribute("aria-label") ?? null,
            };
          }, row);
          const keyed = after.key ? clips.find((k) => k.key === after.key) : null;
          if (!wrong && !(after.at >= pick.start - 0.001 && after.at <= Math.min(pick.end, pick.start + 0.2)))
            wrong = `the card at ${pick.start.toFixed(3)} put the playhead at ${after.at}`;
          else if (!wrong && keyed && after.chosen !== `Clip at ${clockAt(keyed.start)}`)
            wrong = `a sentence of ${keyed.title || keyed.slug} clicked left ${after.chosen ?? "no clip"} chosen`;
          else if (!wrong && Math.abs(after.top - stood) > 1)
            wrong = `a sentence clicked moved the list ${(after.top - stood).toFixed(0)} px`;
        }
        // Shift and the arrows up and down walk the answers to a search,
        // the playhead onto each and the list after it, and letting go of
        // the search goes back to where the playhead and the chosen clip
        // stood, unless Enter said to stay, found by Tim.
        if (!wrong && holds.length >= 2) {
          const where = () =>
            page.evaluate(() => ({
              at: Number(document.querySelector(".screen")?.dataset.playhead),
              chosen: document.querySelector(".track .clipmark.selected")?.getAttribute("aria-label") ?? null,
              followed: document.querySelector("ol.said")?.dataset.followed ?? null,
              cards: [...document.querySelectorAll("ol.said li[data-row]")].map((li) => ({ row: li.dataset.row, at: Number(li.dataset.at) })),
            }));
          for (const stay of [false, true]) {
            if (wrong) break;
            await field.fill(word);
            await watch.step("type", s);
            await page.waitForTimeout(200);
            const from = await where();
            const went = [];
            for (let i = 0; i < 2; i++) {
              await field.press("Shift+ArrowDown");
              await watch.step("press", s);
              await page.waitForTimeout(250);
              went.push(await where());
            }
            for (const [i, w] of went.entries()) {
              const card = sentences.find((x) => Math.abs(x.start - from.cards[i].at) < 0.0005);
              if (wrong) break;
              if (!card || !(w.at >= card.start - 0.001 && w.at < card.end))
                wrong = `Shift and the down arrow ${i + 1} times put the playhead at ${w.at.toFixed(2)}, not on answer ${i + 1} at ${from.cards[i].at.toFixed(2)}`;
              else if (w.followed !== from.cards[i].row) wrong = `walked to ${from.cards[i].row}, the list follows ${w.followed}`;
            }
            if (wrong) break;
            if (stay) {
              await field.press("Enter");
              await watch.step("press", s);
            }
            await field.press("Escape");
            await watch.step("press", s);
            await page.waitForTimeout(250);
            const back = await where();
            if (!stay && (Math.abs(back.at - from.at) > 0.05 || back.chosen !== from.chosen))
              wrong = `a search walked and let go of by Escape left the playhead at ${back.at.toFixed(2)} in ${back.chosen}, not back at ${from.at.toFixed(2)} in ${from.chosen}`;
            else if (stay && Math.abs(back.at - went[1].at) > 0.05)
              wrong = `Enter at the answer walked to, then Escape, moved the playhead from ${went[1].at.toFixed(2)} to ${back.at.toFixed(2)}`;
          }
        }
        // The keyboard on a card of what was typed, for Escape.
        await field.fill(word);
        await watch.step("type", s);
        await page.locator(`ol.said li[data-at="${holds[0].start}"] button.pick`).first().click();
        await watch.step("click", s);
        // Escape with the keyboard on the card just clicked, not in the
        // field, which did nothing for Tim. The first takes back what was
        // typed, so every sentence is there again, and the second the
        // search, found by Tim.
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        const cleared = await page.evaluate(() => ({
          typed: document.querySelector('input[aria-label="Search transcript"]')?.value ?? null,
          cards: document.querySelectorAll("ol.said li:not(.hole):not(.none)").length,
        }));
        if (!wrong && cleared.typed === null) wrong = "the first Escape after typing closed the search, not what was typed";
        else if (!wrong && (cleared.typed !== "" || cleared.cards < sentences.length))
          wrong = `after the first Escape the field holds "${cleared.typed}" over ${cleared.cards} of ${sentences.length} sentences`;
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        const back = await page.evaluate(() => ({
          search: !!document.querySelector(".listhead button.search"),
          said: !!document.querySelector("ol.said"),
          edges: document.querySelectorAll(".unheard-edge").length,
        }));
        if (!wrong && (!back.search || back.said || back.edges))
          wrong = `Escape left ${back.said ? "the sentences" : back.edges ? "the dashed parts on the range picker" : "no Search button"}`;
        if (!wrong && (await headHigh()) !== was) wrong = "the head changed height after the search";
        break;
      }
      case "font list": {
        await page.locator('button.pick[aria-label="Font"]').click();
        await watch.step("click", s);
        await page.waitForSelector(".pick-list .pick-row");
        // Each family's face is fetched as its row is drawn, so the rows
        // are read once the faces have come.
        await page.evaluate(() => document.fonts.ready);
        const seen = await page.evaluate(() => {
          const trigger = document.querySelector('button.pick[aria-label="Font"]');
          const field = trigger.getBoundingClientRect();
          const list = document.querySelector(".pick-list").getBoundingClientRect();
          const rowsBox = document.querySelector(".pick-list .pick-rows");
          // Where the capitals of a name stand against the middle of the
          // box it is in: the baseline, read from a mark of no size sat on
          // it, less half the height of a capital in that face.
          const offCentre = (text, box) => {
            const mark = document.createElement("span");
            mark.style.cssText = "display:inline-block;width:0;height:0;vertical-align:baseline";
            text.append(mark);
            const baseline = mark.getBoundingClientRect().bottom;
            // And the height of a capital, from the face's own figures.
            mark.style.height = "1cap";
            const known = mark.getBoundingClientRect().height;
            mark.remove();
            const look = getComputedStyle(text);
            const pen = document.createElement("canvas").getContext("2d");
            pen.font = `${look.fontStyle} ${look.fontWeight} ${look.fontSize} ${look.fontFamily}`;
            const drawn = pen.measureText("H").actualBoundingBoxAscent;
            const r = box.getBoundingClientRect();
            // The capital the face says it has. The H drawn on a canvas is
            // rounded to whole pixels, and differently by each version of
            // Chromium, so measured by it Inter stood 0.3 px high here and
            // 0.8 px in CI, with the same layout.
            const by = baseline - known / 2 - (r.top + r.height / 2);
            return [by, `${Math.abs(by).toFixed(1)} px ${by < 0 ? "above" : "below"}, with capitals ${known.toFixed(2)} high, drawn ${drawn.toFixed(2)}`];
          };
          const said = trigger.querySelector(".said > span:last-child");
          const centres = [[said.textContent.trim() + " in the field", ...offCentre(said, trigger)]];
          for (const row of document.querySelectorAll(".pick-list .pick-row")) {
            const what = row.querySelector(".what.shown");
            if (what) centres.push([row.textContent.trim(), ...offCentre(what, row)]);
          }
          const search = document.querySelector(".pick-list .search");
          const head = [...document.querySelectorAll(".pick-list .pick-group")].find((it) => it.textContent.trim() === "Installed");
          const ourLast = [...document.querySelectorAll(".pick-list .pick-row")]
            .filter((row) => row.compareDocumentPosition(head) & Node.DOCUMENT_POSITION_FOLLOWING)
            .at(-1);
          const first = document.querySelector(".pick-list .pick-rows .pick-row, .pick-list .pick-rows .pick-group");
          const searchBox = search.getBoundingClientRect();
          const corner = (el) => getComputedStyle(el).borderTopLeftRadius;
          const above = first.getBoundingClientRect().top - searchBox.bottom;
          // The last row, with the rows scrolled to their end, to within
          // the part of a pixel a scroll is rounded to.
          rowsBox.scrollTop = rowsBox.scrollHeight;
          const all = [...document.querySelectorAll(".pick-list .pick-row")];
          const look = {
            corners: [corner(trigger), corner(document.querySelector(".pick-list"))],
            rowCorner: corner(all[0]),
            above,
            below: rowsBox.getBoundingClientRect().bottom - all.at(-1).getBoundingClientRect().bottom,
            face: getComputedStyle(trigger.querySelector(".said > span:last-child")).fontFamily,
            // The search field above its line, and the line over the head
            // of the installed fonts against the last of ours.
            searchHigh: searchBox.height - parseFloat(getComputedStyle(search).borderBottomWidth),
            headGap: head.getBoundingClientRect().top - ourLast.getBoundingClientRect().bottom,
            ourLast: ourLast.textContent.trim(),
            centres,
          };
          const rows = [...document.querySelectorAll(".pick-list .pick-row")].map((row) => {
            const what = row.querySelector(".what");
            const family = getComputedStyle(what).fontFamily.split(",")[0].replace(/"/g, "").trim();
            return {
              name: row.textContent.trim(),
              height: row.getBoundingClientRect().height,
              family,
              drawn: document.fonts.check(`13px "${family}"`, what.textContent),
            };
          });
          return { field: { left: field.left, height: field.height }, list: { left: list.left, height: list.height }, rows, look };
        });
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        const narrow = seen.rows.find((row) => row.name === "Bridge Narrow");
        const squeezed = seen.rows.find((row) => Math.abs(row.height - seen.field.height) > 0.5);
        if (seen.rows.length < 20) wrong = `the Font list has ${seen.rows.length} rows, too few to be longer than it may be tall`;
        else if (Math.abs(seen.list.left - seen.field.left) > 0.5)
          wrong = `the Font list starts at ${seen.list.left.toFixed(1)}, its field at ${seen.field.left.toFixed(1)}`;
        else if (squeezed) wrong = `${squeezed.name} is ${squeezed.height.toFixed(1)} high, its field ${seen.field.height.toFixed(1)}`;
        else if (!narrow) wrong = "the Font list has no Bridge Narrow";
        else if (narrow.family !== "Bridge Narrow" || !narrow.drawn)
          wrong = `Bridge Narrow is drawn in ${narrow.family}${narrow.drawn ? "" : ", which has not loaded"}`;
        else if (seen.rows.slice(0, 3).map((row) => row.name).join(", ") !== "Anton, Archivo Black, Inter")
          wrong = `the app's fonts stand as ${seen.rows.slice(0, 3).map((row) => row.name).join(", ")}, not by name`;
        else if (seen.look.corners[0] !== seen.look.corners[1])
          wrong = `the list's corners are ${seen.look.corners[1]}, its field's ${seen.look.corners[0]}`;
        else if (seen.look.rowCorner !== "0px") wrong = `a row has corners of ${seen.look.rowCorner}`;
        else if (Math.abs(seen.look.above) > 0.5 || Math.abs(seen.look.below) >= 1)
          wrong = `${seen.look.above.toFixed(1)} px between the search field and the first row, ${seen.look.below.toFixed(1)} under the last`;
        else if (!seen.look.face.startsWith('"Inter Black"'))
          wrong = `the Font field is drawn in ${seen.look.face}, not in Inter Black`;
        else if (Math.abs(seen.look.searchHigh - seen.field.height) > 0.5)
          wrong = `the search field is ${seen.look.searchHigh.toFixed(1)} high above its line, a row ${seen.field.height.toFixed(1)}`;
        else if (Math.abs(seen.look.headGap) > 0.5)
          wrong = `${seen.look.headGap.toFixed(1)} px between ${seen.look.ourLast} and the line over Installed`;
        else {
          const off = seen.look.centres.filter(([, by]) => Math.abs(by) > 0.5);
          if (off.length)
            wrong = `the capitals stand off the middle: ${off
              .map(([name, , said]) => `${name} ${said}`)
              .join(", ")}`;
        }
        break;
      }
      case "style list": {
        // A list of one style hung under its field covers the field under
        // that exactly, its bottom on that field's bottom, so no second
        // border shows under its own.
        await page.locator('button.pick[aria-label="Style"]').click();
        await watch.step("click", s);
        await page.waitForSelector(".pick-list .pick-row");
        const seen = await page.evaluate(() => {
          const setting = document.getElementById("caption-style").closest(".setting");
          const under = setting.nextElementSibling.querySelector("input, button");
          return {
            rows: document.querySelectorAll(".pick-list .pick-row").length,
            list: document.querySelector(".pick-list").getBoundingClientRect().bottom,
            under: under.getBoundingClientRect().bottom,
          };
        });
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        if (seen.rows !== 1) wrong = `the Style list has ${seen.rows} rows, not one`;
        else if (Math.abs(seen.list - seen.under) > 0.5)
          wrong = `the Style list ends ${Math.abs(seen.list - seen.under).toFixed(1)} px ${seen.list < seen.under ? "above" : "below"} the bottom of the field under it`;
        break;
      }
      case "nothing found": {
        // Letters that find nothing leave the list as wide as its field,
        // or as the words that say so where they need more, said in its
        // middle.
        await page.locator('button.pick[aria-label="Font"]').click();
        await watch.step("click", s);
        await page.keyboard.type(arg);
        await watch.step("type", s);
        const seen = await page.evaluate(() => {
          const none = document.querySelector(".pick-list .pick-none");
          if (!none) return null;
          const text = document.createRange();
          text.selectNodeContents(none);
          const t = text.getBoundingClientRect();
          const list = document.querySelector(".pick-list").getBoundingClientRect();
          const field = document.querySelector('button.pick[aria-label="Font"]').getBoundingClientRect();
          return {
            side: t.left - list.left - (list.right - t.right),
            list: list.width,
            field: field.width,
            needs: Math.max(field.width, t.width + 16 + 2),
            high: none.getBoundingClientRect().height,
          };
        });
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        if (!seen) wrong = `${arg} found something`;
        else if (Math.abs(seen.side) > 1) wrong = `Nothing matches stands ${seen.side.toFixed(1)} px off the middle of the list`;
        else if (seen.list - seen.needs > 1)
          wrong = `the list saying Nothing matches is ${seen.list.toFixed(1)} wide, its field ${seen.field.toFixed(1)}`;
        else if (Math.abs(seen.high - seen.field) > 0.5 && Math.abs(seen.high - 32) > 0.5)
          wrong = `Nothing matches is ${seen.high.toFixed(1)} high, not a row`;
        break;
      }
      case "search order": {
        // What a search finds stands in the order of the whole list, each
        // row under the head of its own group.
        const order = () =>
          page.evaluate(() =>
            [...document.querySelectorAll(".pick-list .pick-group, .pick-list .pick-row")].map((it) =>
              (it.classList.contains("pick-group") ? "# " : "") + it.textContent.trim(),
            ),
          );
        await page.locator('button.pick[aria-label="Font"]').click();
        await watch.step("click", s);
        const whole = await order();
        await page.keyboard.type(arg);
        await watch.step("type", s);
        const found = await order();
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        const head = (list, name) => list.slice(0, list.indexOf(name)).filter((it) => it.startsWith("# ")).at(-1) ?? "";
        const rows = found.filter((it) => !it.startsWith("# "));
        const places = rows.map((name) => whole.indexOf(name));
        if (rows.length < 2) wrong = `${arg} found ${rows.join(", ") || "nothing"}`;
        else if (places.some((at, i) => at < 0 || (i > 0 && at < places[i - 1])))
          wrong = `${arg} found ${rows.join(", ")}, not in the order of the list`;
        else {
          const moved = rows.find((name) => head(found, name) !== head(whole, name));
          if (moved) wrong = `${moved} stands under ${head(found, moved) || "no head"} after ${arg}`;
        }
        break;
      }
      case "style is": {
        const said = await page.locator('button.pick[aria-label="Style"]').textContent();
        // The trigger keeps room for its longest style, drawn and hidden.
        const shown = await page.locator('button.pick[aria-label="Style"] .said > span:last-child').textContent();
        if (shown.trim() !== arg) wrong = `the Style field says ${shown.trim() || said.trim()}, not ${arg}`;
        break;
      }
      case "listed under": {
        // The Font list puts a face under a head of its group.
        const [head, name] = arg.split("|");
        await page.locator('button.pick[aria-label="Font"]').click();
        await watch.step("click", s);
        const order = await page.evaluate(() =>
          [...document.querySelectorAll(".pick-list .pick-group, .pick-list .pick-row")].map((it) =>
            (it.classList.contains("pick-group") ? "# " : "") + it.textContent.trim(),
          ),
        );
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        const at = order.indexOf(name);
        const heads = order.slice(0, at).filter((it) => it.startsWith("# "));
        if (at < 0) wrong = `the Font list has no ${name}: ${order.join(", ")}`;
        else if (heads.at(-1) !== `# ${head}`) wrong = `${name} is under ${heads.at(-1) ?? "no head"}, not ${head}`;
        break;
      }
      case "thickened as libass": {
        // Read off the word opened, since the pixels of letters differ from
        // one machine's rendering to another's more than the bold does:
        // Chromium faked the bold of Archivo Black 18 % heavier than
        // libass, the Mac 16 % lighter, and CI draws Inter Black, which
        // nothing thickens, 10 % heavier than here.
        await word(arg).click();
        await watch.step("click", s);
        const look = await page.evaluate(async () => {
          const it = document.querySelector(".captions .word.fixing");
          if (!it) return null;
          const style = getComputedStyle(it);
          const face = style.fontFamily.split(",")[0].replace(/["']/g, "").trim();
          const fonts = await fetch("/call", { method: "POST", body: JSON.stringify({ name: "Fonts", args: [] }) })
            .then((r) => r.json())
            .catch(() => null);
          return {
            face,
            weight: Number(style.fontWeight),
            synthesis: style.fontSynthesis || style.getPropertyValue("font-synthesis"),
            stroke: parseFloat(style.webkitTextStrokeWidth),
            size: parseFloat(style.fontSize),
            declared: (fonts ?? []).find((f) => f.name === face)?.weight,
          };
        });
        await page.keyboard.press("Escape");
        await watch.step("press", s);
        console.log(`    ${JSON.stringify(look)}`);
        if (!look) wrong = `"${arg}" did not open`;
        else if (!/none/.test(look.synthesis)) wrong = `the browser may fake a bold of its own, font-synthesis is "${look.synthesis}"`;
        else if (look.declared !== undefined && look.weight !== look.declared)
          wrong = `"${arg}" is drawn at weight ${look.weight} and ${look.face} is a face of ${look.declared}`;
        else if (Math.abs(look.stroke - look.size / 64) > 0.01)
          wrong = `"${arg}" is grown by ${look.stroke} px and libass grows ${look.face} at ${look.size} px by ${(look.size / 64).toFixed(3)}`;
        break;
      }
      case "selection in the pill": {
        // The strips over and under the pill as far as the letters' line
        // reaches, shot with the caret alone and again with every letter
        // selected: what the selection paints there is outside the pill.
        const open = page.locator(".captions .word.fixing");
        const box = await open.evaluate((e) => {
          const pill = e.getBoundingClientRect();
          const r = document.createRange();
          r.selectNodeContents(e);
          const line = r.getBoundingClientRect();
          return { left: pill.left, width: pill.width, top: pill.top, bottom: pill.bottom, over: line.top, under: line.bottom };
        });
        const strips = [
          { x: box.left, y: Math.min(box.over, box.top - 6), width: box.width, height: box.top - 1 - Math.min(box.over, box.top - 6) },
          { x: box.left, y: box.bottom + 1, width: box.width, height: Math.max(box.under, box.bottom + 6) - box.bottom - 1 },
        ].filter((c) => c.height >= 1);
        const shoot = async () => Promise.all(strips.map(async (clip) => (await page.screenshot({ clip })).toString("base64")));
        await open.evaluate((e) => (e.style.caretColor = "transparent"));
        const before = await shoot();
        await page.keyboard.press("ControlOrMeta+A");
        await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
        const after = await shoot();
        await open.evaluate((e) => (e.style.caretColor = ""));
        const changed = await page.evaluate(async ([before, after]) => {
          const read = async (shot) => {
            const img = new Image();
            img.src = `data:image/png;base64,${shot}`;
            await img.decode();
            const c = document.createElement("canvas");
            c.width = img.naturalWidth;
            c.height = img.naturalHeight;
            const g = c.getContext("2d", { willReadFrequently: true });
            g.drawImage(img, 0, 0);
            return g.getImageData(0, 0, c.width, c.height).data;
          };
          let n = 0;
          for (let k = 0; k < before.length; k++) {
            const [a, b] = [await read(before[k]), await read(after[k])];
            for (let i = 0; i < a.length; i += 4) {
              if (Math.max(Math.abs(a[i] - b[i]), Math.abs(a[i + 1] - b[i + 1]), Math.abs(a[i + 2] - b[i + 2])) > 24) n++;
            }
          }
          return n;
        }, [before, after]);
        console.log(`        the letters' line reaches ${(box.top - box.over).toFixed(1)} px over the pill and ${(box.under - box.bottom).toFixed(1)} px under it`);
        if (changed) wrong = `the selection painted ${changed} pixels outside the pill`;
        break;
      }
      case "no caret": {
        const held = await page.evaluate(() => {
          const box = document.querySelector(".captions");
          const sel = getSelection();
          return {
            selection: !!(box && sel?.rangeCount && box.contains(sel.anchorNode)),
            focus: !!(box && box.contains(document.activeElement)),
          };
        });
        if (held.selection) wrong = "the selection stayed in the caption box, where WebKit draws the caret";
        else if (held.focus) wrong = "a word of the caption box still holds the keyboard";
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
        removedLast = on;
        await watch.step("remove clip", s);
        break;
      }
      case "put back": {
        const on = removedLast;
        const back = page.locator("aside ol button.back", { hasText: "Put it back" }).first();
        await back.click();
        let kept = false;
        for (let i = 0; i < 50 && !kept; i++) {
          await page.waitForTimeout(100);
          kept = (await ask(page, "Clips", on.path)).some((c) => c.plan === on.plan && c.id === on.clip && !c.rejected);
        }
        if (!kept) wrong = "Put it back left the clip removed";
        await watch.step("put back", s);
        break;
      }
      case "put back another while playing": {
        const path = await episodeOn(page);
        const was = await page.evaluate(() => document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "");
        const other = await page.evaluate(
          (was) =>
            [...document.querySelectorAll("aside ol li[data-key]")].find((li) => li.dataset.key !== was && li.querySelector("button.drop"))?.dataset.key ?? "",
          was,
        );
        if (!was || !other) {
          wrong = `there is no other clip to remove beside the one chosen, ${was || "none"}`;
          break;
        }
        await page.evaluate(() => document.body.focus());
        await page.keyboard.press("Space");
        await page.waitForSelector('button[aria-label="Pause"]', { timeout: 10000 }).catch(() => (wrong = "the space bar did not start the play"));
        if (wrong) break;
        // Every playhead and every clip chosen from here on, on every
        // animation frame, so a jump back is seen however short it was.
        await page.evaluate(() => {
          window.__heads = [];
          const read = () => {
            if (!window.__heads) return;
            const at = Number(document.querySelector(".screen")?.dataset.playhead);
            const on = document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "";
            const playing = !!document.querySelector('button[aria-label="Pause"]');
            window.__heads.push([at, on, playing]);
            requestAnimationFrame(read);
          };
          requestAnimationFrame(read);
        });
        await page.waitForTimeout(500);
        await page.hover(`aside ol li[data-key="${other}"]`);
        await page.locator(`aside ol li[data-key="${other}"] button.drop`).click();
        let gone = false;
        for (let i = 0; i < 50 && !gone; i++) {
          await page.waitForTimeout(100);
          gone = (await ask(page, "Clips", path)).some((c) => c.key === other && c.rejected);
        }
        if (!gone) {
          wrong = "the bin left the other clip in the list";
          break;
        }
        await page.locator(`aside ol li[data-key="${other}"] button.back`, { hasText: "Put it back" }).click();
        let kept = false;
        for (let i = 0; i < 50 && !kept; i++) {
          await page.waitForTimeout(100);
          kept = (await ask(page, "Clips", path)).some((c) => c.key === other && !c.rejected);
        }
        if (!kept) wrong = "Put it back left the other clip removed";
        await page.waitForTimeout(1000);
        const heads = await page.evaluate(() => {
          const h = window.__heads;
          delete window.__heads;
          return h;
        });
        await page.evaluate(() => {
          if (document.querySelector('button[aria-label="Play"]')) return;
          const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
          document.body.dispatchEvent(new KeyboardEvent("keydown", key));
          document.body.dispatchEvent(new KeyboardEvent("keyup", key));
        });
        for (let i = 1; i < heads.length && !wrong; i++) {
          const [at, on, playing] = heads[i];
          const [at0] = heads[i - 1];
          if (on !== was) wrong = `the clip chosen went from ${was} to ${on || "none"} when the other clip, ${other}, was put back`;
          else if (!playing) wrong = `the play stopped at ${at.toFixed(3)} when the other clip was removed or put back`;
          else if (at < at0 - 1e-6) wrong = `the playhead went back from ${at0.toFixed(3)} to ${at.toFixed(3)} when the other clip was removed or put back`;
        }
        if (!wrong && heads.length < 30) wrong = `only ${heads.length} animation frames were read`;
        await watch.step("put back", s);
        break;
      }
      case "remove the chosen": {
        const path = await episodeOn(page);
        const order = () =>
          page.evaluate(() =>
            [...document.querySelectorAll("aside ol li[data-key]")].filter((li) => li.querySelector("button.pick")).map((li) => li.dataset.key),
          );
        const chosenKey = () =>
          page.evaluate(() => document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "");
        const list = await order();
        if (list.length < 2) {
          wrong = `the list has ${list.length} clips, the step needs two`;
          break;
        }
        const key = arg === "last" ? list[list.length - 1] : list[0];
        const beside = arg === "last" ? list[list.length - 2] : list[1];
        await page.locator(`aside ol li[data-key="${key}"] button.pick`).click();
        for (let i = 0; i < 50 && (await chosenKey()) !== key; i++) await page.waitForTimeout(100);
        if ((await chosenKey()) !== key) {
          wrong = `clicking ${key} did not choose it`;
          break;
        }
        await page.hover(`aside ol li[data-key="${key}"]`);
        await page.locator(`aside ol li[data-key="${key}"] button.drop`).click();
        let gone = false;
        for (let i = 0; i < 50 && !gone; i++) {
          await page.waitForTimeout(100);
          gone = (await ask(page, "Clips", path)).some((c) => c.key === key && c.rejected);
        }
        if (!gone) {
          wrong = "the bin left the clip in the list";
          break;
        }
        let on = "";
        for (let i = 0; i < 30 && on !== beside; i++) {
          on = await chosenKey();
          if (on !== beside) await page.waitForTimeout(100);
        }
        if (on !== beside) wrong = `removing the chosen ${arg} clip, ${key}, left ${on || "nothing"} chosen, not ${beside}`;
        removedChosen = key;
        const entry = (await ask(page, "Clips", path)).find((c) => c.key === key);
        removedLast = { path, plan: entry.plan, clip: entry.id };
        await watch.step("remove clip", s);
        break;
      }
      case "chosen again": {
        let on = "";
        for (let i = 0; i < 30 && on !== removedChosen; i++) {
          on = await page.evaluate(() => document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "");
          if (on !== removedChosen) await page.waitForTimeout(100);
        }
        if (on !== removedChosen) wrong = `${on || "nothing"} is chosen after ${removedChosen} was put back, not ${removedChosen} again`;
        break;
      }
      case "put back the chosen": {
        const path = await episodeOn(page);
        const read = () =>
          page.evaluate(() => [
            document.querySelector("aside ol li[data-key] button.pick.current")?.closest("li")?.dataset.key ?? "",
            Number(document.querySelector(".screen")?.dataset.playhead),
          ]);
        const [was, at] = await read();
        if (!was) {
          wrong = "no clip is chosen";
          break;
        }
        await page.hover(`aside ol li[data-key="${was}"]`);
        await page.locator(`aside ol li[data-key="${was}"] button.drop`).click();
        let gone = false;
        for (let i = 0; i < 50 && !gone; i++) {
          await page.waitForTimeout(100);
          gone = (await ask(page, "Clips", path)).some((c) => c.key === was && c.rejected);
        }
        if (!gone) {
          wrong = "the bin left the clip in the list";
          break;
        }
        await page.locator(`aside ol li[data-key="${was}"] button.back`, { hasText: "Put it back" }).click();
        let back = false;
        for (let i = 0; i < 50 && !back; i++) {
          await page.waitForTimeout(100);
          const [on] = await read();
          back = on === was && (await ask(page, "Clips", path)).some((c) => c.key === was && !c.rejected);
        }
        const [on, now] = await read();
        if (!back) wrong = `the clip chosen is ${on || "none"} after ${was} was put back, not ${was} again`;
        else if (Math.abs(now - at) > 1e-3) wrong = `the playhead went from ${at.toFixed(3)} to ${now.toFixed(3)} when the clip was put back`;
        await watch.step("put back", s);
        break;
      }
      case "make here": {
        const path = await episodeOn(page);
        const was = (await ask(page, "Clips", path)).map((c) => c.key);
        await page.evaluate(() => document.body.focus());
        await page.keyboard.press(arg);
        let found = null;
        for (let t = 0; t < 30000 && !found; t += 250) {
          await page.waitForTimeout(250);
          const on = await chosen(page);
          const key = on && (await ask(page, "Clips", path)).find((c) => c.plan === on.plan && c.id === on.clip)?.key;
          if (key && !was.includes(key)) found = on;
        }
        if (!found) wrong = `${arg} made no clip that was chosen`;
        await watch.step("wait", s);
        break;
      }
      case "asks none": {
        const [name, seconds] = arg;
        const before = await page.evaluate(() => window.__calls.length);
        await page.waitForTimeout(seconds * 1000);
        const asked = await page.evaluate((n) => window.__calls.slice(n).map((c) => c.name), before);
        const these = asked.filter((c) => c === name).length;
        if (these) wrong = `${name} was asked ${these} times in ${seconds} s, with nothing changed: ${asked.join(", ")}`;
        break;
      }
      case "stands still": {
        const before = await page.evaluate(() => window.__calls.length);
        await page.evaluate((away) => {
          if (away) window.dispatchEvent(new Event("blur"));
          const until = performance.now() + 900;
          while (performance.now() < until);
        }, arg);
        await page.waitForTimeout(400);
        if (arg) await page.evaluate(() => window.dispatchEvent(new Event("focus")));
        await page.waitForTimeout(400);
        const lines = await page.evaluate(
          (n) =>
            window.__calls
              .slice(n)
              .filter((c) => c.name === "Said")
              .flatMap((c) => c.args[0].map((l) => l.text)),
          before,
        );
        const stood = lines.some((l) => l.startsWith("window: stood still"));
        if (arg && stood) wrong = `the log said the window stood still while the app was behind another: ${lines.join(" | ")}`;
        if (!arg && !stood) wrong = `the window stood still for 900 ms in front and the log did not say so: ${lines.join(" | ")}`;
        break;
      }
      case "swipe keeps the canvas": {
        await page.evaluate(() => {
          const canvas = document.querySelector(".clip-timeline canvas");
          window.__madeNew = 0;
          for (const side of ["width", "height"]) {
            const own = Object.getOwnPropertyDescriptor(HTMLCanvasElement.prototype, side);
            Object.defineProperty(canvas, side, {
              configurable: true,
              get() {
                return own.get.call(this);
              },
              set(v) {
                window.__madeNew++;
                own.set.call(this, v);
              },
            });
          }
        });
        const line = await page.locator(".clip-timeline").boundingBox();
        await page.mouse.move(line.x + line.width / 2, line.y + line.height * 0.3);
        for (let i = 0; i < arg; i++) await page.mouse.wheel(i % 2 ? -40 : 40, 0);
        await page.waitForTimeout(300);
        const made = await page.evaluate(() => window.__madeNew);
        if (made) wrong = `the waveform's canvas was made new ${made} times in ${arg} swipes`;
        break;
      }
      case "remove two, hand still": {
        const bins = page.locator("aside ol li button.drop");
        if ((await bins.count()) < 2) {
          wrong = `the clip list has ${await bins.count()} clips to remove, not two`;
          break;
        }
        const press = async (bin) => {
          const box = await bin.boundingBox();
          await page.mouse.move(box.x - 40, box.y + box.height / 2, { steps: 4 });
          await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 4 });
          await page.mouse.down();
          await page.mouse.up();
        };
        await press(bins.nth(0));
        await page.waitForSelector("aside ol .gone");
        // The second bin is where it was: the first clip's row stays open
        // while it can be put back.
        await press(bins.nth(0));
        const rows = () =>
          page.evaluate(() =>
            [...document.querySelectorAll("aside ol .gone")].map((g) => {
              const run = g.querySelector(".fill .run");
              return { top: Math.round(g.getBoundingClientRect().top), running: run.getAnimations().length > 0, at: getComputedStyle(run).transform };
            }),
          );
        let now = [];
        for (let i = 0; i < 30 && now.length < 2; i++) {
          await page.waitForTimeout(100);
          now = await rows();
        }
        if (now.length < 2) {
          wrong = `${now.length} rows can be put back after two clips were removed`;
          break;
        }
        if (now[1].running) {
          wrong = "the second row's time runs down under the hand that removed it";
          break;
        }
        const second = now[1].top;
        // What WebKit says while the hand stands still: nothing.
        await page.evaluate(() => {
          const names = ["pointerover", "pointerout", "pointerenter", "pointerleave", "pointermove"];
          const still = (e) => e.stopImmediatePropagation();
          window.__stillHand = () => names.forEach((n) => window.removeEventListener(n, still, true));
          names.forEach((n) => window.addEventListener(n, still, true));
        });
        for (let i = 0; i < 150 && now.length > 1; i++) {
          await page.waitForTimeout(100);
          now = await rows();
        }
        if (now.length !== 1 || now[0].top >= second) {
          wrong = `the first row did not close and move the second up: ${JSON.stringify(now)}`;
          break;
        }
        await page.waitForTimeout(400);
        await page.evaluate(() => window.__stillHand());
        const line = await page.locator(".clip-timeline").boundingBox();
        await page.mouse.move(line.x + line.width / 2, line.y + line.height / 2, { steps: 8 });
        await page.waitForTimeout(600);
        const a = await rows();
        await page.waitForTimeout(500);
        const b = await rows();
        if (!a[0]?.running || a[0].at === b[0]?.at) {
          wrong = `the row left to put back stands still with the hand on the clip timeline: ${JSON.stringify(b)}`;
          break;
        }
        for (let i = 0; i < 120 && (await rows()).length; i++) await page.waitForTimeout(100);
        if ((await rows()).length) wrong = "the row left to put back is still there after its time";
        await watch.step("remove two, hand still", s);
        break;
      }
      case "row folds": {
        const bins = page.locator("aside ol li:has(+ li) button.drop");
        if (!(await bins.count())) {
          wrong = "no clip in the clip list has a card under it";
          break;
        }
        const bin = bins.first();
        const box = await bin.boundingBox();
        await page.mouse.move(box.x - 40, box.y + box.height / 2, { steps: 4 });
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 4 });
        await page.mouse.down();
        await page.mouse.up();
        await page.waitForSelector("aside ol .gone");
        // Every frame from here, until the row has been gone a while.
        await page.evaluate(() => {
          const row = document.querySelector("aside ol .gone").closest("li");
          const under = row.nextElementSibling;
          const gap = parseFloat(getComputedStyle(row.parentElement).rowGap);
          const seen = [];
          window.__fold = { seen, height: row.getBoundingClientRect().height, gap };
          const look = () => {
            seen.push({ there: row.isConnected, top: under.getBoundingClientRect().top });
            if (row.isConnected || seen.filter((f) => !f.there).length < 30) requestAnimationFrame(look);
            else window.__fold.done = true;
          };
          requestAnimationFrame(look);
        });
        const line = await page.locator(".clip-timeline").boundingBox();
        await page.mouse.move(line.x + line.width / 2, line.y + line.height / 2, { steps: 8 });
        for (let i = 0; i < 200 && !(await page.evaluate(() => window.__fold.done)); i++) await page.waitForTimeout(100);
        const { seen, height, gap, done } = await page.evaluate(() => window.__fold);
        if (!done) {
          wrong = "the removed row was still in the list after its time";
          break;
        }
        const tops = seen.map((f) => f.top);
        const left = seen.findIndex((f) => !f.there);
        const rose = tops[0] - tops[tops.length - 1];
        const back = tops.findIndex((t, i) => i && t > tops[i - 1] + 0.5);
        const last = tops[left - 1] - tops[left];
        if (Math.abs(rose - (height + gap)) > 1) wrong = `the card under the removed row rose ${rose.toFixed(1)} px, not the row's ${height} and the ${gap} after it`;
        else if (back >= 0) wrong = `the card under the removed row went back down ${(tops[back] - tops[back - 1]).toFixed(1)} px on frame ${back}`;
        else if (last > 1) wrong = `the card under the removed row jumped ${last.toFixed(1)} px in the frame the row left the list, of the ${rose.toFixed(1)} it rose`;
        await watch.step("row folds", s);
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
      case "folder removed": {
        folderGone = await episodeOn(page);
        const folder = join(dirname(folderGone), `${basename(folderGone, extname(folderGone))}.framefairy`);
        if (!existsSync(folder)) {
          wrong = `${basename(folder)} is not there to delete`;
          break;
        }
        // Again while the app writes into it, as a person deletes it again
        // when Finder says the folder is not empty: a file the app writes
        // while the folder is emptied leaves it not empty, which is not the
        // folder coming back. On CI's busy runners the app was still
        // writing after five tries in a second and a half, and the walk
        // failed on its own delete. So it deletes until the folder is gone,
        // for up to ten seconds. What the app must not do, make the folder
        // again once it is gone, is what the walk then looks for.
        const by = Date.now() + 10000;
        for (;;) {
          try {
            rmSync(folder, { recursive: true, force: true });
            break;
          } catch (err) {
            if (err.code !== "ENOTEMPTY" || Date.now() > by) throw err;
            await page.waitForTimeout(50);
          }
        }
        videoGone = arg === "with the video" || arg === "written back";
        if (videoGone) rmSync(folderGone, { force: true });
        if (arg === "written back") {
          mkdirSync(folder);
          writeFileSync(join(folder, "chosen.json"), JSON.stringify({ clip: "", opened: Date.now() }));
        }
        if (arg === "closed" || arg === "written back") {
          await control(url, "/reopen");
          await page.reload();
        } else {
          // The person went to Finder and came back: the app lost the
          // keyboard and has it again.
          await page.evaluate(() => {
            window.dispatchEvent(new FocusEvent("blur"));
            window.dispatchEvent(new FocusEvent("focus"));
          });
        }
        break;
      }
      case "let go": {
        const name = basename(folderGone);
        const folder = join(dirname(folderGone), `${basename(folderGone, extname(folderGone))}.framefairy`);
        // The sidebar first, before the walk asks the Go side anything,
        // so it is the app that read the library again and not the walk.
        const left = await page
          .waitForFunction(
            (n) => ![...document.querySelectorAll("aside button.episode .name")].some((e) => e.textContent === n),
            name,
            { timeout: 5000, polling: 100 },
          )
          .then(() => true, () => false);
        if (!left) {
          wrong = `${name} is still in the sidebar 5 s after its folder was deleted`;
          break;
        }
        await page.waitForTimeout(1500);
        const library = await ask(page, "Library");
        if (library.some((e) => e.source === folderGone)) wrong = `${name} is still in the library`;
        else if ((await episodeOn(page)) === folderGone) wrong = `${name} is still open`;
        // With what was written into it, which says what wrote it.
        else if (existsSync(folder))
          wrong = `${basename(folder)} was written back, with ${readdirSync(folder, { recursive: true }).join(", ") || "nothing in it"}`;
        else if (!videoGone && !existsSync(folderGone)) wrong = `${name} itself was deleted`;
        else if ((await page.locator(".workspace.front").count()) && !(await episodeOn(page))) {
          wrong = `a workspace is still open with no video picked in the sidebar`;
        }
        await settle(page);
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
        await page.locator('[role="listbox"]').waitFor({ state: "detached" });
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
      case "build mark": {
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
        // R works on the frame the video preview shows, inShort in
        // Episode.svelte, and the turn read is that frame's, so R is
        // pressed once it is the playhead's frame, as C is, see keyframe
        // at. On a busy machine R was pressed while the video preview
        // still showed the frame from before the click, issue #320.
        if (!(await shownOnPlayhead(page))) {
          wrong = "the video preview did not come to the playhead's frame";
          break;
        }
        const turn = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.turn));
        const before = await turn();
        const took = await keyShows(page, "r", (b) => Number(document.querySelector(".screen").dataset.turn) !== b, before);
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
        // C puts the keyframe on the frame the video preview shows, so it
        // is pressed once that is the playhead's frame, see thumbnail at.
        if (!(await shownOnPlayhead(page))) {
          wrong = "the video preview did not come to the playhead's frame";
          break;
        }
        const count = () => page.evaluate(() => document.querySelectorAll(".clip-timeline .cropdot").length);
        const before = await count();
        const took = await keyShows(
          page,
          "c",
          (b) => document.querySelectorAll(".clip-timeline .cropdot").length > b && !!document.querySelector(".clip-timeline .cropdot.here"),
          before,
        );
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
        // T makes a thumbnail of the frame the video preview shows, the
        // frame a person sees, so it is pressed once that is the playhead's
        // frame. Pressed before, on a loaded machine, T took the thumbnail
        // off the frame still shown from the step before (2.261).
        if (!(await shownOnPlayhead(page))) {
          wrong = "the video preview did not come to the playhead's frame";
          break;
        }
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
            const look = getComputedStyle(el);
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
      case "layers late": {
        await page.evaluate((ms) => {
          const real = window.fetch;
          window.fetch = async (input, init) => {
            const answer = real(input, init);
            if (String(input).startsWith("/frames/captions?")) await new Promise((r) => setTimeout(r, ms));
            return answer;
          };
        }, arg);
        break;
      }
      case "answers late": {
        await page.evaluate((ms) => {
          const real = window.fetch;
          window.fetch = async (input, init) => {
            if (String(input).endsWith("/call") && init?.body && String(JSON.parse(init.body).name).split(".").pop() === "Shape") {
              await new Promise((r) => setTimeout(r, ms));
            }
            return real(input, init);
          };
        }, arg);
        break;
      }
      case "playhead on": {
        const on = await chosen(page);
        const clip = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const fps = await rate(page);
        const want = arg === "start" ? Math.floor((clip.start + 0.001) * fps) : Math.floor((clip.end + 0.001) * fps) - 1;
        const at = Number(await page.evaluate(() => document.querySelector(".screen").dataset.playhead));
        const got = Math.floor((at + 0.001) * fps);
        if (got !== want) wrong = `the playhead is at ${at} s, frame ${got}, not on the clip's ${arg} at frame ${want}, ${clip[arg]} s`;
        break;
      }
      case "frame on the pill while it plays": {
        const fps = await rate(page);
        const state = await engineState(page, watch.at);
        const pieces = state.segments;
        const into = (t) => {
          let sum = 0;
          for (const p of pieces) {
            if (t < sum + p.end - p.start) return p.start + t - sum;
            sum += p.end - p.start;
          }
          return null;
        };
        const length = pieces.reduce((n, p) => n + p.end - p.start, 0);
        // The first caption that ends with room to play on both sides of
        // its end, and room after it for the next one.
        const end = (state.captions ?? []).map((c) => c.end).find((e) => e > 1.6 && e < length - arg);
        const from = end === undefined ? null : into(end - 1);
        if (from === null) {
          wrong = "no caption ends far enough inside the clip to play over its end";
          break;
        }
        if (!(await seekTo(page, from, fps))) {
          wrong = `the video preview never showed the frame at ${from.toFixed(2)}`;
          break;
        }
        await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
        const stop = await watchPills(page);
        await page.evaluate(() => document.body.focus());
        await page.keyboard.press("Space");
        await page
          .waitForFunction((to) => Number(document.querySelector(".screen").dataset.playhead) >= to, from + arg, {
            timeout: arg * 1000 + 30000,
            polling: "raf",
          })
          .catch(() => (wrong = `the play from ${from.toFixed(2)} never went on ${arg} s`));
        await page.evaluate(() => {
          if (document.querySelector('button[aria-label="Play"]')) return;
          const key = { key: " ", code: "Space", bubbles: true, cancelable: true };
          document.body.dispatchEvent(new KeyboardEvent("keydown", key));
          document.body.dispatchEvent(new KeyboardEvent("keyup", key));
        });
        const frames = await stop();
        if (wrong) break;
        const off = frames.find((f) => framesOffPill(f, "now"));
        const lit = new Set(frames.filter((f) => f.pill).map((f) => `${Math.round(f.pill.y)}:${f.now?.text}`));
        const behind = laggingFrames(frames);
        console.log(`    ${frames.length} frames painted, ${frames.filter((f) => f.pill).length} with a pill, ${lit.size} words lit`);
        console.log(`    ${behind.late} of ${behind.painted} painted with captions not their own, at most ${behind.ms.toFixed(0)} ms on end`);
        if (off) wrong = framesOffPill(off, "now");
        else if (frames.filter((f) => f.pill).length < fps) wrong = `only ${frames.length} frames were read while it played`;
        else if (lit.size < 3) wrong = `the play lit only ${lit.size} words, it never went from one caption to the next`;
        // The caption box follows the layer on screen, so a layer that
        // comes late takes the pill and the box behind the sound together,
        // and nothing else here sees it.
        else if (behind.late > behind.painted / 20) wrong = `${behind.late} of ${behind.painted} paints showed captions not their own, more than one in twenty`;
        else if (behind.ms >= 2000 / fps) wrong = `the captions were not the frame's own for ${behind.ms.toFixed(0)} ms on end, two frames or more`;
        break;
      }
      case "walk the words with the frame on the pill": {
        const fps = await rate(page);
        const state = await engineState(page, watch.at);
        const first = state.segments[0];
        // From the clip's first frame, so the walk goes over the words of
        // the first captions and from one caption to the next.
        if (!(await seekTo(page, first.start + 0.5 / fps, fps))) {
          wrong = "the video preview never showed the clip's first frame";
          break;
        }
        await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
        await page.evaluate(() => document.body.focus());
        const report = [];
        for (let k = 0; k < arg && !wrong; k++) {
          const stop = await watchPills(page);
          await page.keyboard.press("Shift+ArrowRight");
          await page.waitForTimeout(150);
          await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
          await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
          const frames = await stop();
          const off = frames.find((f) => framesOffPill(f, "keyed"));
          const last = frames[frames.length - 1];
          if (off) wrong = `after Shift and the right arrow ${k + 1} times, ${framesOffPill(off, "keyed")}`;
          else if (!last?.keyed) wrong = `after Shift and the right arrow ${k + 1} times, no word wears the keyboard's frame`;
          else report.push(`"${last.keyed.text}" ${frames.filter((f) => !f.keyed).length} frames without the frame`);
        }
        console.log(`    ${report.join(", ")}`);
        break;
      }
      case "step onto words and see them pop": {
        const fps = await rate(page);
        const state = await engineState(page, watch.at);
        const first = state.segments[0];
        if (!(await seekTo(page, first.start + 0.5 / fps, fps))) {
          wrong = "the video preview never showed the clip's first frame";
          break;
        }
        await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
        await page.evaluate(() => document.body.focus());
        const report = [];
        let popped = 0;
        // The most a pop grew.
        let grew = 1;
        // One step with key, watched: whether a pop started, the frames
        // read, and how many frames of the screen the bounce was drawn on.
        const stepWatched = async (key) => {
          const count = await page.evaluate(() => window.__captionLayers?.stats.popped ?? 0);
          const drawn = await page.evaluate(() => window.__captionLayers?.stats.bounced ?? 0);
          const stop = await watchPills(page);
          await page.keyboard.press(key);
          // The pop, if one starts, and its end.
          const started = await page
            .waitForFunction((n) => (window.__captionLayers?.stats.popped ?? 0) > n, count, { timeout: 1500, polling: 10 })
            .then(() => true, () => false);
          await page.waitForFunction(() => !window.__captionLayers?.popping && !window.__captionLayers?.busy, null, {
            timeout: 5000,
            polling: 10,
          });
          await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r))));
          const frames = await stop();
          const bounced = (await page.evaluate(() => window.__captionLayers?.stats.bounced ?? 0)) - drawn;
          // The longest the screen went without a frame while the bounce
          // was drawn, from the first frame it was drawn on to the last.
          const on = frames.filter((f, i) => i > 0 && f.bounced > frames[i - 1].bounced);
          const gap = Math.max(0, ...on.slice(1).map((f, i) => f.t - on[i].t));
          return { started, frames, bounced, gap };
        };
        // arg steps to the right, then as many back to the first word.
        for (let k = 0; k < 2 * arg && !wrong; k++) {
          const key = k < arg ? "Shift+ArrowRight" : "Shift+ArrowLeft";
          let { started, frames, bounced, gap } = await stepWatched(key);
          // The bounce lasts 220 ms and is drawn on every frame the screen
          // draws while it lasts, so on a machine so busy that the screen
          // drew fewer than three frames in that time, or none for 90 ms,
          // there is no bounce to see, only the machine. Issues #328 and
          // #329: a busy runner drew two, and one drew none round the
          // peak. Then the step is taken again, off the word and back onto
          // it, up to three times, and a bounce that is not there is not
          // there any of the four times.
          for (let again = 0; started && (bounced < 3 || gap > 90) && again < 3; again++) {
            await stepWatched(key === "Shift+ArrowRight" ? "Shift+ArrowLeft" : "Shift+ArrowRight");
            ({ started, frames, bounced, gap } = await stepWatched(key));
          }
          const off = frames.find((f) => framesOffPill(f, "keyed"));
          const last = frames[frames.length - 1];
          if (off) {
            wrong = `after ${key} on step ${k + 1}, ${framesOffPill(off, "keyed")}`;
            break;
          }
          if (!last?.keyed || !last.pill) {
            wrong = `after ${key} on step ${k + 1}, no word wears the keyboard's frame on a pill`;
            break;
          }
          // The paints of the word stepped onto, its pill's width on each.
          const on = frames.filter((f) => f.pill && f.keyed?.text === last.keyed.text);
          const widths = on.map((f) => f.pill.w);
          const least = Math.min(...widths);
          const most = Math.max(...widths);
          report.push(
            `"${last.keyed.text}" ${started ? `popped from ${least.toFixed(0)} to ${most.toFixed(0)} px over ${bounced} frames` : "did not pop"}`,
          );
          if (!started) continue;
          popped++;
          grew = Math.max(grew, most / least);
          // The frames of the screen from the first the bounce was drawn
          // on to the last, read off the count of bounce paints on each,
          // and those of them it was not drawn on.
          const paints = frames.map((f, i) => (i > 0 ? f.bounced - frames[i - 1].bounced : 0));
          const span = paints.slice(paints.findIndex((d) => d > 0), paints.findLastIndex((d) => d > 0) + 1);
          const skipped = span.filter((d) => d === 0).length;
          // The word and its pill grow together, to 115 % of rest, from
          // the word's first frame, where the step lands, at rest, and
          // back to rest, however briefly the short lights it, every word
          // alike. Tim saw it stop grown, a frame into the word, with the
          // frame round it at rest, and some words grow by a few pixels on
          // one frame, and then a word of one frame stop at its peak.
          // On the screen's frames, one scale each: Tim found the bounce
          // laggy when it was the short's frames of the pop, 25 a second.
          // How many frames the screen draws in a fifth of a second is the
          // machine's, 13 on a quiet one and 2 on a busy runner, so a word
          // holds that the bounce was drawn on every frame the screen drew
          // while it lasted, which no machine changes, and on three at the
          // least, on the way up, its peak and on the way down. Before, the
          // whole walk held that a bounce was drawn over 7 frames, which
          // a busy machine broke, issues #328 and #329. That every frame
          // gets its own scale is held by "the bounce after a word step"
          // in captions.test.ts. So too the peak: with fewer frames one may
          // land a little off it, 12.7 % of 15 on a machine running six
          // walks at once, and 8 % of "er" when a busy runner drew no frame
          // for over 100 ms round its peak, so a word holds that it popped
          // by 5 %, against the 1 % Tim saw, and the whole walk that some
          // pop reached the peak.
          if (bounced < 3) {
            wrong = `"${last.keyed.text}" bounced over only ${bounced} frames of the screen, four times`;
          } else if (skipped > 0) {
            wrong = `"${last.keyed.text}" bounced on ${span.length - skipped} of the ${span.length} frames the screen drew while it lasted`;
          } else if (most < least * 1.05) {
            wrong = `"${last.keyed.text}" popped, but its pill grew only from ${least.toFixed(1)} to ${most.toFixed(1)} px`;
          } else if (Math.abs(widths[0] - least) > 1.5 || Math.abs(last.pill.w - least) > 1.5) {
            wrong = `"${last.keyed.text}" popped from a pill ${widths[0].toFixed(1)} px wide to one ${last.pill.w.toFixed(1)} px wide, not from rest, ${least.toFixed(1)} px, back to rest`;
          } else {
            // Round its middle, in every direction alike, as Tim asked: the
            // pill at its largest grew as far up as down and as far left
            // as right, and in height by the scale it grew in width. A
            // bounce that grew one way only passed this walk before, found
            // by the second review. Each edge is read to a whole pixel, so
            // the two sides of a pill grown round its middle differ by a
            // pixel or two, 2 up and 1 down here, and a pill grown upward
            // only by 4, 3 up and 1 in.
            const rest = on[0].pill;
            const peak = on[widths.indexOf(most)].pill;
            const up = rest.t - peak.t;
            const down = peak.b - rest.b;
            const left = rest.x - rest.w / 2 - (peak.x - peak.w / 2);
            const right = peak.x + peak.w / 2 - (rest.x + rest.w / 2);
            const tall = (rest.b - rest.t) * (peak.w / rest.w - 1);
            if (Math.abs(up - down) > 2 || Math.abs(left - right) > 2) {
              wrong = `"${last.keyed.text}" grew ${up.toFixed(1)} px up and ${down.toFixed(1)} down, ${left.toFixed(1)} left and ${right.toFixed(1)} right, not round its middle`;
            } else if (Math.abs(up + down - tall) > 2) {
              wrong = `"${last.keyed.text}" grew ${(up + down).toFixed(1)} px in height where its width's scale makes ${tall.toFixed(1)}`;
            }
          }
        }
        console.log(`    ${report.join(", ")}`);
        if (!wrong && popped < 2 * arg) wrong = `only ${popped} of ${2 * arg} steps onto a word popped it`;
        else if (!wrong && grew < 1.14) wrong = `no pop grew by more than ${((grew - 1) * 100).toFixed(0)} %`;
        break;
      }
      case "step frame by frame and see nothing pop": {
        // arg steps of a frame from the clip's first frame, each onto the
        // frame as the short shows it, and nothing played over it, a step
        // onto a new word too.
        const fps = await rate(page);
        const state = await engineState(page, watch.at);
        if (!(await seekTo(page, state.segments[0].start + 0.5 / fps, fps))) {
          wrong = "the video preview never showed the clip's first frame";
          break;
        }
        await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 20 });
        await page.evaluate(() => document.body.focus());
        const count = await page.evaluate(() => window.__captionLayers?.stats.popped ?? 0);
        const words = new Set();
        for (let k = 0; k < arg && !wrong; k++) {
          await page.keyboard.press("ArrowRight");
          await page.waitForTimeout(80);
          await page.waitForFunction(() => !window.__captionLayers?.busy, null, { timeout: 5000, polling: 10 });
          const now = await page.evaluate(() => ({
            popped: window.__captionLayers?.stats.popped ?? 0,
            word: document.querySelector(".captions .word.now")?.textContent ?? "",
          }));
          if (now.word) words.add(now.word);
          if (now.popped !== count) wrong = `step ${k + 1} of a frame, onto "${now.word}", played a bounce over the frame`;
        }
        console.log(`    ${arg} frames over ${words.size} words, nothing played over them`);
        if (!wrong && words.size < 2) wrong = `${arg} frames lit only ${words.size} word`;
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
        // Away from the clip's edges, where a click takes the edge, and
        // once the word's pop has settled, 220 ms after it lights: the
        // frame is where the pill rests. In its pop the pill grows round
        // the same middle by up to 15 %.
        // A word stays lit until the next one starts.
        const settled = (w) => w.start + 0.22 + 2 / fps;
        const length = pieces.reduce((n, p) => n + p.end - p.start, 0);
        const words = (state.captions ?? [])
          .flatMap((c) => {
            const ws = c.lines.flatMap((l) => l.words);
            return ws.map((w, i) => ({ ...w, lit: i + 1 < ws.length ? ws[i + 1].start : c.end }));
          })
          .filter((w) => w.start > 0.6 && w.lit < length - 0.6)
          .map((w) => ({ ...w, rests: settled(w) < w.lit - 1 / fps }))
          // The words whose pop settles first, then words in their pop.
          .sort((a, b) => Number(b.rests) - Number(a.rests))
          .slice(0, arg);
        if (words.filter((w) => w.rests).length < 2) {
          wrong = `only ${words.filter((w) => w.rests).length} words are lit long enough for their pop to settle`;
          break;
        }
        const report = [];
        for (const w of words) {
          const at = into(w.rests ? Math.max((w.start + w.lit) / 2, settled(w)) : (w.start + w.lit) / 2);
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
          report.push(`"${text}"${w.rests ? "" : " in its pop"}: ${across.toFixed(1)} px across, ${down.toFixed(1)} px down, ${wider.toFixed(1)} px wider, ${lower.toFixed(1)} px taller`);
          // A word lit too briefly for its pop to settle is read in its
          // pop, where the frame grows with the pill round its middle.
          if (Math.abs(across) > 1.5 || Math.abs(down) > 1.5 || Math.abs(wider) > 2 || Math.abs(lower) > 2) {
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
      case "on the playhead": {
        // Found by Tim: a keyframe lit by the playhead stood a pixel to the
        // right of it and a thumbnail half a pixel, as he saw it on his Mac.
        // The playhead was placed on the track's whole box and the marks on
        // its inside, a pixel in from either side, and the playhead on its
        // moment where a thumbnail stood on the middle of its frame and a
        // keyframe on its start. So up close, where a frame is many pixels
        // wide, they stood apart by as much. Plan row 2.234.
        const m = await page.evaluate(() => {
          const mid = (e) => {
            const b = e.getBoundingClientRect();
            return b.left + b.width / 2;
          };
          const line = document.querySelector(".clip-timeline .playhead");
          const head = document.querySelector(".clip-timeline .head") ?? line;
          return {
            line: line ? mid(line) : NaN,
            head: mid(head),
            lit: [...document.querySelectorAll(".clip-timeline .thumb.here, .clip-timeline .cropdot.here")].map((e) => ({
              kind: e.classList.contains("thumb") ? "thumbnail" : "keyframe",
              at: mid(e),
            })),
          };
        });
        const off = m.lit.find((l) => Math.abs(l.at - m.line) > 0.01);
        if (!Number.isFinite(m.line)) wrong = "the clip timeline shows no playhead";
        else if (m.lit.length !== arg) wrong = `${m.lit.length} marks light on the playhead's frame, not ${arg}`;
        else if (off) wrong = `the ${off.kind} lit by the playhead stands ${(off.at - m.line).toFixed(2)} px from its line`;
        else if (Math.abs(m.head - m.line) > 0.01) wrong = `the playhead's head stands ${(m.head - m.line).toFixed(2)} px from its line`;

        break;
      }
      case "pinch in": {
        // Up close, pinched in on the playhead to a tenth, where a frame is
        // many pixels wide, and the hand taken off the clip timeline.
        const g = await handles(page);
        const line = await page.evaluate(() => {
          const b = document.querySelector(".clip-timeline .playhead")?.getBoundingClientRect();
          return b ? b.left + b.width / 2 : NaN;
        });
        await page.mouse.move(Number.isFinite(line) ? line : g.track.x + g.track.w / 2, high(g.track));
        await page.keyboard.down("Control");
        await page.mouse.wheel(0, -230);
        await page.keyboard.up("Control");
        await page.waitForTimeout(300);
        await page.mouse.move(g.track.x + g.track.w / 2, g.track.y + g.track.h + 80);
        await settle(page);
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
      case "stamp": {
        // The rotate icon over the rotated part, worked out from the two
        // layers the stylesheet lays over each piece of it, the icon and
        // the glow round it: how much of what is under the icon shows
        // through, the size it is drawn at and where, against the part's middle,
        // the band the keyframes stand in along the top of the clip
        // timeline and the band the thumbnails stand in along its foot.
        const got = await page.evaluate(() => {
          const track = document.querySelector(".clip-timeline .track").getBoundingClientRect();
          const band = (sel, sample) => {
            const e = document.querySelector(sel);
            if (!e) return sample;
            const r = e.getBoundingClientRect();
            return [r.top, r.bottom];
          };
          const pieces = [...document.querySelectorAll(".clip-timeline .piece.turned")].map((e) => {
            const r = e.getBoundingClientRect();
            const css = getComputedStyle(e, "::after");
            const glow = getComputedStyle(e, "::before");
            const size = parseFloat(css.maskSize);
            const left = parseFloat(css.maskPosition);
            return {
              left: r.left,
              right: r.right,
              top: r.top,
              height: r.height,
              size,
              at: r.left + left,
              image: css.maskImage,
              colour: css.backgroundColor,
              glow: glow.maskImage,
              glowColour: glow.backgroundColor,
              glowCut: glow.maskComposite,
              cut: getComputedStyle(e).maskImage,
            };
          });
          return {
            keys: band(".clip-timeline .cropdot", null),
            thumbs: band(".clip-timeline .thumb", [track.bottom - 10, track.bottom + 9]),
            height: track.height,
            pieces,
          };
        });
        const p = got.pieces[0];
        if (!p) {
          wrong = "the rotated part has no rotate icon on it";
          break;
        }
        if (!/svg/.test(p.image)) {
          wrong = `the rotated part's icon is drawn with ${p.image.slice(0, 40)}, not the rotate icon`;
          break;
        }
        // The accent at half strength, so the waveform shows through it,
        // which a solid dark did not, and was too strong. Its alpha is the
        // last number of the colour, rgba() or color() alike.
        const alpha = (c) => {
          const n = c.match(/[\d.]+/g)?.map(Number) ?? [];
          return /\/|rgba/.test(c) ? n[n.length - 1] : 1;
        };
        if (Math.abs(alpha(p.colour) - 0.5) > 0.02) {
          wrong = `the rotate icon is ${p.colour}, not the accent at half strength`;
          break;
        }
        // A dark glow round it, the icon's lines drawn wider and softened
        // with the icon itself taken out, so it parts the icon from the
        // waveform and the empty track alike and lies only outside it.
        if (!/svg[\s\S]*svg/.test(p.glow) || !/^exclude/.test(p.glowCut)) {
          wrong = `the rotate icon has no glow round it, its mask is ${p.glow.slice(0, 40)} with ${p.glowCut}`;
          break;
        }
        if (alpha(p.glowColour) < 0.5) {
          wrong = `the glow round the rotate icon is ${p.glowColour}, too faint to part it from the waveform`;
          break;
        }
        // Nothing really cut out of the piece, where the track's black and
        // the waveform's grey showed through by turns.
        if (p.cut !== "none") {
          wrong = `the rotated part's wash has ${p.cut.slice(0, 40)} cut out of it`;
          break;
        }
        const top = p.top + (p.height - p.size) / 2;
        const bottom = top + p.size;
        const middle = p.at + p.size / 2;
        if (p.size < got.height / 2) wrong = `the rotate icon is ${p.size.toFixed(1)} px, less than half the clip timeline's ${got.height.toFixed(0)} px`;
        else if (Math.abs(middle - (p.left + p.right) / 2) > 1)
          wrong = `the rotate icon's middle is at ${middle.toFixed(1)} px, not the part's, ${((p.left + p.right) / 2).toFixed(1)} px`;
        else if (got.keys && top < got.keys[1])
          wrong = `the rotate icon, ${top.toFixed(1)} to ${bottom.toFixed(1)} px, reaches into the keyframes' row, to ${got.keys[1].toFixed(1)} px`;
        else if (bottom > got.thumbs[0])
          wrong = `the rotate icon, ${top.toFixed(1)} to ${bottom.toFixed(1)} px, reaches into the thumbnails' row, from ${got.thumbs[0].toFixed(1)} px`;
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
        // the keyframe button's own: Lucide's diamond, filled, as a
        // thumbnail is Lucide's camera filled. Plan row 2.234.
        const shapes = await page.evaluate(() =>
          [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => e.querySelector("svg path")?.getAttribute("d") ?? "nothing"),
        );
        const sizes = await page.evaluate(() => [...document.querySelectorAll(".clip-timeline .cropdot")].map((e) => e.getBoundingClientRect().width));
        if (got.length !== arg) wrong = `the engine has ${got.length} keyframes, not ${arg}`;
        else if (new Set(got.map((m) => m.x)).size !== arg) wrong = `the keyframes stand at ${got.map((m) => m.x).join(", ")}, not each at its own place`;
        else if (dots !== arg) wrong = `the clip timeline draws ${dots} keyframes, not ${arg}`;
        else if (shapes.some((c) => !c.startsWith("M2.7 10.3a2.41 2.41 0 0 0 0 3.41l7.59 7.59")))
          wrong = `a keyframe is not drawn as a diamond: ${shapes.join(", ")}`;
        else if (sizes.some((w) => w !== 18)) wrong = `a keyframe is ${sizes.join(", ")} px wide, not 18 px like a thumbnail, found too small by Tim on #192`;
        break;
      }
      case "glides across the cut": {
        // The first keyframe before the clip's first cut and the last after
        // it: on a frame between the first and the cut, and on one between
        // the cut and the last, the crop frame stands where the glide puts
        // it, on its way from the one to the other.
        const on = await chosen(page);
        const entry = (await ask(page, "Clips", on.path)).find((c) => c.plan === on.plan && c.id === on.clip);
        const src = await ask(page, "Source", on.path);
        const [p, q] = entry?.segments ?? [];
        const marks = entry?.cropMarks ?? [];
        const a = marks.find((m) => p && m.at >= p.start && m.at < p.end);
        const b = marks.findLast((m) => q && m.at >= q.start && m.at < q.end);
        if (!a || !b) {
          wrong = "the clip has no keyframe either side of its cut";
          break;
        }
        await filmedOnScreen(page);
        const crop = () => page.evaluate(() => Number(document.querySelector(".screen").dataset.crop));
        const problems = [];
        const seen = [];
        for (const [piece, at] of [
          [p, a.at + 0.7 * (p.end - a.at)],
          [q, q.start + 0.3 * (b.at - q.start)],
        ]) {
          // Half a frame in, so the click lands in the frame meant.
          const t = Math.floor(at * src.fps) / src.fps + 0.5 / src.fps;
          const g = await handles(page);
          await page.mouse.click(await xOf(page, t), high(g.track));
          await settle(page);
          const came = await page
            .waitForFunction(
              (fps) => window.__filmed() === Math.floor((Number(document.querySelector(".screen").dataset.playhead) + 0.001) * fps),
              src.fps,
              { polling: 50, timeout: 5000 },
            )
            .then(() => true, () => false);
          if (!came) {
            problems.push(`no frame came for ${t.toFixed(3)} s`);
            continue;
          }
          const k = await filmedOnScreen(page);
          const got = await crop();
          seen.push(got);
          const want = (piece.marks ?? []).length ? glideOf(piece.marks, src)(k) : NaN;
          if (got !== want) problems.push(`on frame ${k} the crop frame stands at ${got}, the glide puts it at ${want}`);
        }
        const [ax, bx] = [a.x - (a.x % 2), b.x - (b.x % 2)];
        const between = (v) => (v - ax) * (v - bx) < 0;
        if (!problems.length && !seen.every(between)) {
          problems.push(`either side of the cut the crop frame stands at ${seen.join(" and ")}, not on its way from ${ax} to ${bx}`);
        }
        if (problems.length) wrong = problems.join("; ");
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
        // The play starts from the frame on the video preview, so it is
        // started once that is the playhead's frame.
        if (!(await shownOnPlayhead(page))) {
          wrong = "the video preview did not come to the playhead's frame";
          break;
        }
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
        // The frames of the episode the video preview showed, each with a
        // place of its own on the glide. How many it shows in two seconds
        // is the machine's: a busy one showed six, issue #320. So the crop
        // frame stands in as many places as frames were shown, and the
        // play showed at least three, where it stood in ten or more before.
        const shown = new Set(seen.filter((r) => r.k >= 0).map((r) => r.k));
        const apart = new Set([...shown].map((k) => glide(k)));
        if (off.length) {
          wrong = `while playing, ${off.length} of ${seen.length} frames had the crop frame off the glide, the first frame ${off[0].k} at ${off[0].crop} for ${glide(off[0].k)}`;
        } else if (shown.size < 3) {
          wrong = `while playing for two seconds, the video preview showed ${shown.size} frames`;
        } else if (places.size < apart.size) {
          wrong = `while playing, the crop frame stood in ${places.size} places over ${shown.size} frames, where the glide has ${apart.size}`;
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
        // Every rotated piece has the rotate icon drawn on its wash, and
        // no other.
        const turned = arg.filter((t) => t !== 0).length;
        const marked = await page.evaluate(() => document.querySelectorAll(".clip-timeline .piece.turned").length);
        if (marked !== turned) problems.push(`the clip timeline marks ${marked} pieces as rotated, not ${turned}`);
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
            await nextPaint(page);
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

// Reads, on every frame the browser paints, the pill libass drew off the
// canvas and the frames the caption box puts round words: the lit word's,
// now, and the keyboard's, keyed. The pill is looked for in a band round
// the caption box, so a pill of a caption the box does not hold is found
// too. Says how to stop, which hands back what was read.
async function watchPills(page) {
  await page.evaluate(() => {
    // Its own, so a watch that was stopped stops at its next frame even
    // when the next watch has begun by then. It read the new watch's on
    // before and went on, so every step of a walk added a reading of the
    // canvas to every frame of the screen, and on a busy machine the
    // screen drew a bounce on fewer frames each step, issues #328 and #329.
    const watch = { frames: [], on: true };
    window.__pillWatch = watch;
    const s = document.createElement("canvas");
    const g = s.getContext("2d", { willReadFrequently: true });
    const look = () => {
      if (!watch.on) return;
      requestAnimationFrame(look);
      const box = document.querySelector(".captions");
      const canvas = document.querySelector(".screen canvas");
      if (!box || !canvas?.width) return;
      const pill = (getComputedStyle(box.querySelector(".word") ?? box).getPropertyValue("--pill").match(/[\d.]+/g) ?? []).map(Number);
      if (pill.length < 3) return;
      const c = canvas.getBoundingClientRect();
      const b = box.getBoundingClientRect();
      const sx = canvas.width / c.width;
      const sy = canvas.height / c.height;
      const y0 = Math.max(0, Math.floor((b.top - c.top - b.height) * sy));
      const y1 = Math.min(canvas.height, Math.ceil((b.bottom - c.top + b.height) * sy));
      if (y1 <= y0) return;
      s.width = canvas.width;
      s.height = y1 - y0;
      g.drawImage(canvas, 0, y0, s.width, s.height, 0, 0, s.width, s.height);
      const d = g.getImageData(0, 0, s.width, s.height).data;
      let [l, r, t, bt] = [Infinity, -Infinity, Infinity, -Infinity];
      for (let y = 0; y < s.height; y++) {
        for (let x = 0; x < s.width; x++) {
          const i = (y * s.width + x) * 4;
          if ([0, 1, 2].every((k) => Math.abs(d[i + k] - pill[k]) < 40)) {
            [l, r, t, bt] = [Math.min(l, x), Math.max(r, x + 1), Math.min(t, y), Math.max(bt, y + 1)];
          }
        }
      }
      const frame = (word) => {
        if (!word) return null;
        const f = word.getBoundingClientRect();
        return {
          text: word.textContent,
          x: (f.left + f.right) / 2 - c.left,
          y: (f.top + f.bottom) / 2 - c.top,
          t: f.top - c.top,
          b: f.bottom - c.top,
          w: f.right - f.left,
        };
      };
      watch.frames.push({
        at: Number(document.querySelector(".screen").dataset.playhead),
        pictured: window.__pictured?.(),
        t: performance.now(),
        late: window.__captionLayers?.late ?? false,
        bounced: window.__captionLayers?.stats.bounced ?? 0,
        now: frame(box.querySelector(".word.now")),
        keyed: frame(box.querySelector(".word.keyed")),
        pill:
          l === Infinity
            ? null
            : { x: (l + r) / 2 / sx, y: (y0 + (t + bt) / 2) / sy, t: (y0 + t) / sy, b: (y0 + bt) / sy, w: (r - l) / sx },
      });
    };
    requestAnimationFrame(look);
  });
  return () =>
    page.evaluate(() => {
      window.__pillWatch.on = false;
      return window.__pillWatch.frames;
    });
}

// What is wrong on a frame read by watchPills with the frame round a word,
// the lit word's, "now", or the keyboard's, "keyed", or null. A lit word
// stands on the pill, and where a pill is drawn a word is lit. The
// keyboard's frame, where there is one, stands on the pill too.
// How long the video preview showed captions not their own, read off
// what watchPills saw: the paints made with the layer of a frame before,
// the frame's own still on its way, out of all, and the longest time they
// went on one after another, in milliseconds.
function laggingFrames(frames) {
  let late = 0;
  let since = null;
  let most = 0;
  for (const f of frames) {
    if (f.late) {
      late++;
      since ??= f.t;
      most = Math.max(most, f.t - since);
    } else since = null;
  }
  return { painted: frames.length, late, ms: most };
}

function framesOffPill(f, which) {
  const word = f[which];
  const where = `at ${f.at.toFixed(3)} s, frame ${f.pictured} on the video preview`;
  if (which === "now" && f.pill && !word) return `${where}, a pill is drawn and no word is lit in the caption box`;
  if (!word) return null;
  if (!f.pill) return `${where}, no pill is drawn and the frame stands round "${word.text}"`;
  // Across, the frame stands on the middle of the pill. Down, it stands
  // inside it, and it is as wide as the pill: the pill pops round the
  // frame's middle and the frame grows with it. Tim saw the frame stay at
  // rest round a pill still grown.
  const across = word.x - f.pill.x;
  const above = f.pill.t - word.t;
  const below = word.b - f.pill.b;
  const narrower = f.pill.w - word.w;
  if (Math.abs(across) > 3 || above > 3 || below > 3 || Math.abs(narrower) > 3) {
    return `${where}, the frame round "${word.text}" stands ${across.toFixed(1)} px across from the pill, ${above.toFixed(1)} px over its top and ${below.toFixed(1)} px under its bottom, and is ${narrower.toFixed(1)} px narrower`;
  }
  return null;
}


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
// What the toasts over the window say, one after the other.
async function toastSays(page) {
  return page.evaluate(() => [...document.querySelectorAll(".toast .text")].map((t) => t.textContent.trim()).join(" "));
}

// Where what a toast must not move stands: the workspace in front, the
// report's field, and the main column under the bar.
async function standsAt(page) {
  return page.evaluate(() =>
    [".workspace.front .stage", "main", ".telling"].map((q) => {
      const r = document.querySelector(q)?.getBoundingClientRect();
      return r ? [q, r.x, r.y, r.width, r.height] : [q];
    }),
  );
}

// Whether nothing matches selector within ms.
async function gone(page, selector, ms) {
  for (const end = Date.now() + ms; Date.now() < end; await page.waitForTimeout(100)) {
    if ((await page.locator(selector).count()) === 0) return true;
  }
  return (await page.locator(selector).count()) === 0;
}

// Says what moved since standsAt was read, or null.
async function stillStands(page, before) {
  if (!before) return null;
  const now = await standsAt(page);
  for (let i = 0; i < before.length; i++) {
    if (JSON.stringify(before[i]) !== JSON.stringify(now[i])) return `${before[i][0]} moved from ${before[i].slice(1).join(", ")} to ${now[i].slice(1).join(", ")} when the toast came`;
  }
  return null;
}

async function dropOn(page, files) {
  const { width, height } = page.viewportSize();
  const [x, y] = [Math.round(width * 0.4), Math.round(height * 0.6)];
  const landing = () => page.evaluate(() => {
      const frame = document.querySelector(".landing");
      return frame ? getComputedStyle(frame).opacity : "nothing, there is no frame";
    });
  // Within 300 ms each way, and on as soon as the frame stands or has gone.
  const comes = (want) =>
    page
      .waitForFunction((want) => getComputedStyle(document.querySelector(".landing") ?? document.body).opacity === want && !!document.querySelector(".landing"), want, { polling: "raf", timeout: 300 })
      .catch(() => {});
  await page.evaluate(([x, y]) => {
    window._wails.handleDragEnter();
    window._wails.handleDragOver(x, y);
  }, [x, y]);
  await comes("1");
  const held = await landing();
  if (held !== "1") return `the app wore no frame while a file was held over it, its opacity was ${held}`;
  const took = await page.evaluate(([f, x, y]) => window._wails.handlePlatformFileDrop(f, x, y), [files, x, y]);
  if (!took) return `the drop at ${x}, ${y} landed on no drop target`;
  await comes("0");
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
  // Issue #264: a part rotated a quarter stood a pixel above the viewer,
  // its top row cut and a black line under it. It stands inside the
  // viewer, in its middle, on a whole pixel.
  if (turned && (corners[2] < 0 || corners[3] < 0 || Math.abs(corners[2] - corners[3]) > 1 || Math.abs(corners[2] - Math.round(corners[2])) > 0.01)) {
    return `the rotated picture stands ${corners[2].toFixed(2)} px down from the viewer's top and ${corners[3].toFixed(2)} px up from its foot`;
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
