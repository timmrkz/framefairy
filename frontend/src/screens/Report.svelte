<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";
  import { api, clock, onDictation, onLoudness, onReportDraft, onUndo, type ReportDraft, type ReportPicture } from "../lib/api";
  import { asking, typing } from "../lib/keys";
  import Removed from "../components/Removed.svelte";
  import { chime, wake } from "../lib/chime";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import Confirm from "../components/Confirm.svelte";
  import { toasts } from "../lib/toast.svelte";
  // A line in the app's log. said is the words being dictated here.
  import { said as log } from "../lib/said";

  // Report a Problem, in the window Bugs in the sidebar opens: what the
  // person says happened, the pictures they attach, and one button that
  // puts it on the Desktop with the app's log, as the one file they send
  // us. The Go side writes it and shows it in Finder, see report.go,
  // bugs.go and docs/LOGGING.md. Plan rows 2.188 and 2.244. What is
  // written and attached is a draft the Go side keeps until Report, so
  // closing the window loses nothing.

  let what = $state("");
  let making = $state(false);
  // Why a report could not be made, or pictures attached. It is a toast
  // over the window, issue #327: beside the buttons it wrapped over
  // three lines and stood where nothing else was about it. Each time is
  // a toast of its own.
  const oops = (text: string) => toasts.say("report", text);

  // Dictate, plan row 2.189, the way dictation on claude.ai works: the
  // words appear in the field while they are said, where the cursor was,
  // grey until they are kept. What follows the last pause is still being
  // heard and changes as more of it is, so it is plain text in a mark
  // that cannot be typed in. What came before the pause is settled and is text
  // in the field, still grey, which can be corrected while the person
  // talks on. Enter, Escape or the button keeps all of it, written in as
  // one step Cmd+Z takes back. Escape removed it once, as on claude.ai,
  // and Tim lost a long report to an Escape meant for the app, with no
  // Cmd+Z to bring it back, issue #355. On a Mac, Escape ends Dictation
  // and keeps what was said. The Go side records and writes down, see
  // listen.go.
  let field: HTMLDivElement;
  let report: HTMLButtonElement;
  let listening = $state(false);
  let hearing = $state(false);
  // The last words of a dictation being written down, which Report
  // waits for, so Tab and Enter right after Enter make a report with
  // them in it, issue #404.
  let writing: Promise<void> = Promise.resolve();
  // The recording in hand, how many of its pieces are in the field, and
  // how many times words being heard were settled here by a click.
  let recording = 0;
  let placed = 0;
  let asked = 0;
  // What has been said, in the text where the cursor was, and the mark at
  // its end that draws what is still being heard. The settled words go
  // in by hand rather than as typing, so they are no steps of their own
  // for Cmd+Z, and Cmd+Z and Cmd+Y while talking undo and redo only what
  // was typed. What is still being heard is plain text in the mark, which
  // cannot be typed in. It was drawn by the mark's ::after from an
  // attribute, and on Tim's Mac the end of a long report was left painted
  // where it had been, grey under white, in lines that did not line up.
  let said: HTMLSpanElement | null = null;
  let mark: HTMLSpanElement | null = null;
  // Where the cursor was in the field, kept while the button has the
  // focus, since the field forgets it then on some systems.
  let cursor: Range | null = null;
  // Whether the field follows the words as they come, the way a terminal
  // follows its output: it does until the person scrolls away from them,
  // and again once they scroll back to them. lastTop is where the field
  // was last scrolled to follow them, so that scroll is not taken for the
  // person's.
  let following = true;
  let lastTop = -1;
  // Whether the cursor is right after the words being said, where what is
  // said next goes. It is drawn there by the mark, see the style, since
  // WebKit drew it at the front of the last line on Tim's Mac while the
  // words ran on to the right.
  let ours = $state(false);

  const sentence = (e: unknown) => {
    const said = e instanceof Error ? e.message : String(e);
    return said.charAt(0).toUpperCase() + said.slice(1);
  };

  // The field's text, as it is sent. While somebody dictates it holds
  // what is still being heard too, and Report waits until that is
  // kept.
  const read = () => {
    what = field.innerText.replace(/\n$/, "");
    keepText();
  };

  // The field is kept as it changes, a moment after the last change, and
  // at once when the window goes.
  let saving: ReturnType<typeof setTimeout> | null = null;
  let loaded = false;
  function keepText() {
    if (!loaded) return;
    if (saving) clearTimeout(saving);
    saving = setTimeout(saveNow, 300);
  }
  function saveNow() {
    if (saving) clearTimeout(saving);
    saving = null;
    if (loaded) api.saveReportText(what).catch(() => {});
  }

  // The pictures attached, in the order they came, and the ones removed
  // a moment ago, by id. A removed one keeps its place, says so
  // and offers itself back, and then the grid closes over it.
  let pictures = $state<ReportPicture[]>([]);
  let removed = $state<Record<string, { picture: ReportPicture }>>({});
  let attaching = $state(false);
  // The pictures on screen: the attached and the removed in their places.
  const tiles = $derived.by(() => {
    const all = [...pictures.map((p) => ({ p, gone: false })), ...Object.values(removed).map((r) => ({ p: r.picture, gone: true }))];
    return all.sort((a, b) => order(a.p.id) - order(b.p.id));
  });
  const order = (id: string) => Number(id.slice(1)) || 0;

  // Whether there is nothing to report: no words and no pictures. Report
  // and Clear are off then, issue #415. Words still being written down
  // after a dictation may be on their way, so Report waits for them
  // rather than being off.
  const nothing = $derived(!what.trim() && pictures.length === 0 && !hearing);

  // Clear empties the report, the words and every picture, once the box
  // over the page has asked, issue #415. A report that turned out to be
  // for nothing had to be emptied by hand, a picture at a time.
  let clearing = $state(false);
  // What the last Clear took, while Cmd+Z can still bring it back and
  // Cmd+Y take it again, as Tim asked: whether it took words, the
  // pictures, and whether it is undone now. The words go as one step of
  // the field's own undo, so the field's Cmd+Z brings them back, and the
  // pictures follow that step. Anything done after it, a letter typed or
  // a picture added or removed, ends it, as in any editor.
  let cleared: { text: boolean; pics: ReportPicture[]; undone: boolean } | null = null;
  // An edit of the field that Clear or its undo makes, which ends nothing.
  let quiet = false;
  const unclear = () => (cleared = null);

  async function clearAll() {
    clearing = false;
    log("report: cleared", { level: "info" });
    // The box lets go of the focus as it closes, and the field can only
    // take it, and its undo the words, once it has.
    await tick();
    const pics = [...pictures];
    removed = {};
    pictures = [];
    field.focus({ preventScroll: true });
    let text = false;
    if (field.textContent) {
      getSelection()?.selectAllChildren(field);
      quiet = true;
      text = document.execCommand("delete");
      quiet = false;
      // Where the field's own undo is not to be had, the words go anyway.
      if (field.textContent) field.textContent = "";
    }
    what = "";
    // A save still to come would bring the words back.
    if (saving) clearTimeout(saving);
    saving = null;
    cleared = { text, pics, undone: false };
    try {
      took(await api.clearReport());
    } catch (e) {
      oops(sentence(e));
    }
  }

  // Undo and Redo in the Edit menu. After Clear they bring everything
  // back and clear it again. Otherwise they go to the field, as on every
  // page with no workspace, or Cmd+Z in it would do nothing.
  async function undo(how: "undo" | "redo") {
    if (asking()) return;
    const c = cleared;
    if (!c || c.undone !== (how === "redo")) {
      if (typing()) document.execCommand(how);
      return;
    }
    c.undone = how === "undo";
    log(`report: ${how === "undo" ? "Undo brings back" : "Redo clears again"} the words and ${c.pics.length} pictures`, { level: "info" });
    field.focus({ preventScroll: true });
    if (c.text) {
      quiet = true;
      document.execCommand(how);
      quiet = false;
    }
    read();
    if (how === "redo") {
      if (saving) clearTimeout(saving);
      saving = null;
      pictures = pictures.filter((p) => !c.pics.some((q) => q.id === p.id));
      try {
        took(await api.clearReport());
      } catch (e) {
        oops(sentence(e));
      }
      return;
    }
    pictures = [...pictures, ...c.pics].sort((a, b) => order(a.id) - order(b.id));
    let last: ReportDraft | null = null;
    for (const p of c.pics) {
      try {
        last = await api.putBackPicture(p.id);
      } catch (e) {
        oops(sentence(e));
      }
    }
    if (last) took(last);
  }
  onMount(() => onUndo(undo));

  // took shows what the Go side says the draft holds now, with why a
  // picture was left out.
  function took(d: ReportDraft) {
    pictures = d.pictures ?? [];
    for (const id of Object.keys(removed)) if (pictures.some((p) => p.id === id)) delete removed[id];
    if (d.error) oops(d.error.endsWith(".") ? d.error : d.error + ".");
  }

  // A drop says what came of it on its own.
  function dropped(d: ReportDraft) {
    unclear();
    took(d);
  }

  async function attach() {
    if (attaching) return;
    unclear();
    attaching = true;
    try {
      took(await api.attachPictures());
    } catch (e) {
      oops(sentence(e));
    } finally {
      attaching = false;
    }
  }

  function removePicture(p: ReportPicture) {
    unclear();
    removed[p.id] = { picture: p };
    pictures = pictures.filter((q) => q.id !== p.id);
    api.removePicture(p.id).then(took, (e) => oops(sentence(e)));
  }

  // The picture is back in its place at once, the way a removed one goes
  // at once, so nothing closes over it while the Go side answers. Turned
  // away, at ten pictures, it stands as removed again.
  function putBack(id: string) {
    const r = removed[id];
    if (!r) return;
    unclear();
    pictures = [...pictures, r.picture];
    delete removed[id];
    const again = () => {
      if (!pictures.some((p) => p.id === id)) removed[id] = r;
    };
    api.putBackPicture(id).then(
      (d) => {
        took(d);
        again();
      },
      (e) => {
        pictures = pictures.filter((p) => p.id !== id);
        again();
        oops(sentence(e));
      },
    );
  }

  // A picture or a recording pasted into the field is attached, the way
  // Mail takes one. Text pastes as text.
  function pasted(e: ClipboardEvent) {
    const files = [...(e.clipboardData?.files ?? [])].filter((f) => f.type.startsWith("image/") || f.type.startsWith("video/"));
    if (files.length === 0) return;
    e.preventDefault();
    unclear();
    for (const f of files) {
      const reader = new FileReader();
      reader.onload = () => {
        const data = String(reader.result).replace(/^data:[^,]*,/, "");
        api.attachPasted(f.name || "Pasted picture.png", data).then(took, (err) => oops(sentence(err)));
      };
      reader.readAsDataURL(f);
    }
  }

  // Delete or Backspace on a tile removes its picture.
  function tileKey(e: KeyboardEvent, p: ReportPicture) {
    if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault();
      removePicture(p);
    }
  }

  function remember() {
    const sel = getSelection();
    if (sel?.rangeCount && field?.contains(sel.anchorNode)) cursor = sel.getRangeAt(0).cloneRange();
  }

  // How loud the microphone is, which the button moves with, and why the
  // last recording stopped by itself.
  let loud = $state(0);
  let note = $state("");
  // Why the microphone hears nothing, while it records.
  let unheard = $state("");

  // How far the microphone has grown, from 0 at rest to 1 for a raised
  // voice. It follows the loudness the way the needle of a level meter
  // does: up with the voice at once, in about a twentieth of a second,
  // and back down slowly, in about a third, so it swells with a word and
  // settles between words. Moving straight to every reading, ten a
  // second, it shook. It is worked out every frame, so it moves between
  // readings rather than jumping to them, and it is written straight to
  // the icon as a scale: no layout, no transition, nothing else moves.
  let mic = $state<HTMLSpanElement>();
  let grown = 0;
  let frame = 0;
  let then = 0;
  const still = matchMedia("(prefers-reduced-motion: reduce)");
  function grow(now: number) {
    const target = listening && !still.matches ? loud : 0;
    const dt = then ? Math.min(now - then, 100) : 16;
    grown += (target - grown) * (1 - Math.exp(-dt / (target > grown ? 50 : 300)));
    if (Math.abs(target - grown) < 0.002) grown = target;
    if (mic) mic.style.transform = `scale(${(16 + 12 * grown) / 28})`;
    then = now;
    frame = grown === target ? 0 : requestAnimationFrame(grow);
    if (!frame) then = 0;
  }
  $effect(() => {
    void [loud, listening, mic];
    if (!frame) frame = requestAnimationFrame(grow);
  });
  onDestroy(() => cancelAnimationFrame(frame));

  // The text of the field before and after a node in it.
  function around(node: Node): [string, string] {
    const before = document.createRange();
    before.selectNodeContents(field);
    before.setEndBefore(node);
    const after = document.createRange();
    after.selectNodeContents(field);
    after.setStartAfter(node);
    return [before.toString(), after.toString()];
  }

  // write types text where the selection is, as one step for Cmd+Z. A new
  // line is typed as one, since text with a new line in it was put in as
  // a block of its own, which WebKit on Tim's Mac showed as an empty line
  // at the end, and which the text of the field does not count, issue
  // #373.
  function write(text: string): boolean {
    let ok = true;
    for (const [i, line] of text.split("\n").entries()) {
      if (i) ok = document.execCommand("insertLineBreak") && ok;
      if (line) ok = document.execCommand("insertText", false, line) && ok;
    }
    return ok;
  }

  // count says how many letters of the field come before a place in it,
  // and point finds the place so many letters in. The cursor is kept by
  // these while the field's pieces are put together again, which moves
  // every place held by a node.
  function count(node: Node, offset: number): number {
    const r = document.createRange();
    r.selectNodeContents(field);
    r.setEnd(node, offset);
    return r.toString().length;
  }
  function point(n: number): [Node, number] {
    const walk = document.createTreeWalker(field, NodeFilter.SHOW_TEXT);
    for (let t = walk.nextNode() as Text | null; t; t = walk.nextNode() as Text | null) {
      if (n <= t.length) return [t, n];
      n -= t.length;
    }
    return [field, field.childNodes.length];
  }

  // begin puts what will be said where the cursor is, or at the end of
  // the text, and the cursor after it.
  function begin(from?: Range) {
    let at = from ?? (cursor && field.contains(cursor.startContainer) ? cursor.cloneRange() : null);
    if (!at) {
      at = document.createRange();
      at.selectNodeContents(field);
    }
    at.collapse(false);
    said = document.createElement("span");
    said.className = "dictated";
    mark = document.createElement("span");
    mark.className = "hearing";
    mark.contentEditable = "false";
    said.append(mark);
    at.insertNode(said);
    // A space from the word before, where it would run into it.
    if (/\S$/.test(around(said)[0])) said.prepend(" ");
    field.focus({ preventScroll: true });
    const after = document.createRange();
    after.setStartAfter(said);
    getSelection()?.removeAllRanges();
    getSelection()?.addRange(after);
    following = true;
    lastTop = -1;
    caret();
  }

  // atWords says whether the words being said are in view in the field.
  function atWords(): boolean {
    if (!mark) return false;
    const m = mark.getBoundingClientRect();
    const f = field.getBoundingClientRect();
    return m.bottom <= f.bottom + 1 && m.top >= f.top - 1;
  }

  // scrolled hears the field scroll. A scroll that was not the field
  // following the words is the person's, and the field follows the words
  // from then on only while they are in view.
  function scrolled() {
    if (!listening || !mark) return;
    following = field.scrollTop === lastTop || atWords();
  }

  // caret says whether the cursor is right after the words being said,
  // with nothing between, in the field that has the focus. It stays drawn
  // there while the last words are written down: WebKit drew its own at
  // the front of the line or past the words on Tim's Mac, and it jumped
  // back once they came, issue #373. A key typed or a click meanwhile
  // gives the field its own cursor where that put it.
  function caret() {
    const sel = getSelection();
    let at = false;
    if ((listening || hearing) && said && mark && field.contains(said) && document.activeElement === field && sel?.rangeCount && sel.isCollapsed) {
      const r = sel.getRangeAt(0);
      const gap = document.createRange();
      gap.setStartAfter(mark);
      if (gap.comparePoint(r.startContainer, r.startOffset) >= 0) {
        gap.setEnd(r.startContainer, r.startOffset);
        at = !gap.toString();
      }
    }
    ours = at;
  }

  // What has been settled so far, without what is still being heard.
  function settled(): string {
    const at = document.createRange();
    at.selectNodeContents(said!);
    at.setEndBefore(mark!);
    return at.toString();
  }

  // push keeps the cursor after the words being said when it is at their
  // end, so what is said pushes it along, the way it does before anything
  // is typed. Typed at the end of what was said, the cursor was left in
  // front of the words heard next, and the next letters went in before
  // them, issue #373.
  function push() {
    const sel = getSelection();
    if (!said || !mark || !sel?.rangeCount || !sel.isCollapsed || !said.contains(sel.anchorNode)) return;
    const gap = document.createRange();
    gap.setStart(sel.anchorNode!, sel.anchorOffset);
    gap.setEndBefore(mark);
    if (gap.toString()) return;
    const after = document.createRange();
    after.setStartAfter(said);
    sel.removeAllRanges();
    sel.addRange(after);
  }

  // add puts a settled piece in at the end of what has been said.
  function add(piece: string) {
    if (!said || !mark || !field.contains(said)) begin();
    if (!said || !mark) return;
    push();
    const before = settled();
    const text = (before.trim() && !/\s$/.test(before) ? " " : "") + piece;
    const last = mark.previousSibling;
    if (last?.nodeType === Node.TEXT_NODE) (last as Text).appendData(text);
    else mark.before(text);
  }

  // show draws what is still being heard after what was settled, and
  // keeps the end of it in view, however long the report has grown.
  function show(now: string) {
    if ((!said || !mark || !field.contains(said)) && now) begin();
    if (!said || !mark) return;
    const rest = space(now);
    if (now.trim()) push();
    // At the end of the report, down to its padding. Tim talked past the
    // field's last line and could not see what was written down. Not
    // while the person has scrolled away to read: then the field stays
    // where they put it, as Tim asked.
    if (following) {
      if (rest.trim()) mark.scrollIntoView({ block: "nearest", inline: "nearest" });
      else field.scrollTop = field.scrollHeight;
      lastTop = field.scrollTop;
    }
    caret();
    read();
  }

  // space puts now in the mark with the spaces it needs between what was
  // settled before it and what follows it, and gives what follows.
  function space(now: string): string {
    const rest = around(said!)[1];
    const before = settled();
    const text = (now && /\S$/.test(before) ? " " : "") + now + (/^\S/.test(rest) && (now || before.trim()) ? " " : "");
    if (mark!.textContent !== text) mark!.textContent = text;
    return rest;
  }

  // typed keeps a space between what was said and a letter typed right
  // after it, at once rather than when something more is heard, which
  // may be never once the person stops talking.
  function typed() {
    if (!quiet) unclear();
    if (listening && said && mark && field.contains(said)) space(mark.textContent!.trim());
    read();
  }

  // follow keeps what is said next after what is typed at the end of
  // what was said. Tim typed a few words there and talked on, and what he
  // said next went in front of them and pushed them along. Typed there,
  // the words being heard are settled and the typing goes in after them,
  // inside what was said, so what is said next comes after it. What was
  // said stays one piece of the field, wherever else the person types or
  // moves the cursor, and is written in as one when it is kept. It was
  // split into a piece for each time somebody typed at its end, and
  // WebKit, on Tim's Mac, typed the letters after that backwards, and
  // Enter lost what was said once the pieces were no longer in order.
  // Nothing in the field is moved while the browser types, only before,
  // with the browser's own typing called off and done again here, which
  // keeps it one more step for Cmd+Z.
  function follow(e: InputEvent) {
    if (!listening || e.isComposing || !said || !mark || !field.contains(said)) return;
    const sel = getSelection();
    if (!sel?.rangeCount || !sel.isCollapsed) return;
    const at = sel.getRangeAt(0);
    // Right after what was said and what is being heard, with nothing
    // between.
    const gap = document.createRange();
    gap.setStartAfter(mark);
    if (gap.comparePoint(at.startContainer, at.startOffset) < 0) return;
    gap.setEnd(at.startContainer, at.startOffset);
    if (gap.toString()) return;
    let data: string;
    if (e.inputType === "insertText") data = e.data ?? "";
    else if (e.inputType === "insertFromPaste") data = e.dataTransfer?.getData("text/plain") ?? "";
    else if (e.inputType === "insertLineBreak" || e.inputType === "insertParagraph") data = "\n";
    else if (e.inputType === "deleteContentBackward" && (settled() + mark.textContent).trim()) data = "";
    else return;
    e.preventDefault();
    settleHeard();
    const before = settled();
    // A space between a word that was said and a word typed after it,
    // not before a full stop or a comma.
    if (data && /\S$/.test(before) && /^[^\s.,;:!?)\]}]/.test(data)) data = " " + data;
    // A new line, typed or pasted, goes in by hand, the way the settled
    // words do. The browser's own put it after the mark on Tim's Mac, so
    // what was said next went on the line above the cursor, or as a block
    // of its own, which split what was said in two, issue #373.
    if (data.includes("\n")) {
      const last = mark.previousSibling;
      if (last?.nodeType === Node.TEXT_NODE) (last as Text).appendData(data);
      else mark.before(data);
      const after = document.createRange();
      after.setStartAfter(said);
      sel.removeAllRanges();
      sel.addRange(after);
      return typed();
    }
    const end = document.createRange();
    end.setStartBefore(mark);
    const last = mark.previousSibling;
    if (last?.nodeType === Node.TEXT_NODE) end.setStart(last, (last as Text).length);
    sel.removeAllRanges();
    sel.addRange(end);
    if (data) document.execCommand("insertText", false, data);
    else if (before) document.execCommand("delete");
  }

  // freeze settles the words still being heard where they are, when the
  // pointer goes down in them or just after them, before the browser puts
  // the cursor there. They become text like any settled piece, so the
  // cursor goes where it was put, and a word can be selected and
  // corrected while the person talks on. Tim could not click into them.
  // The Go side is told, and what it says until it has taken that in is
  // passed over, since it would show those words a second time.
  function freeze(e: PointerEvent) {
    if (!listening || !recording || !said || !mark || !field.contains(said)) return;
    if (!mark.textContent!.trim()) return;
    if (!mark.contains(e.target as Node)) {
      const at = document.caretRangeFromPoint?.(e.clientX, e.clientY);
      if (!at) return;
      const from = document.createRange();
      from.setStartBefore(mark);
      from.setEndAfter(said);
      if (from.comparePoint(at.startContainer, at.startOffset) !== 0) return;
    }
    settleHeard();
  }

  // settleHeard makes the words being heard a settled piece where they
  // are.
  function settleHeard() {
    const now = mark?.textContent!.trim();
    if (!now || !recording) return;
    add(now);
    api.settleListening(recording, placed, now).catch(() => {});
    placed++;
    asked++;
    show("");
  }

  // catchUp writes in the pieces not yet in the field. Every message has
  // all of them, so one that was missed costs nothing.
  function catchUp(pieces: string[]) {
    for (; placed < pieces.length; placed++) add(pieces[placed]);
  }

  // finish ends the dictation. Kept, what was said, with whatever was
  // corrected in it, goes into the text as if it had been typed, as one
  // step Cmd+Z takes back, and the cursor after it.
  // What was typed at the end of what was said is part of it, see
  // follow.
  // The cursor stays where the person has it. What is typed or clicked
  // while the last words are written down lands after them or before them,
  // and is not moved: Tim ended a dictation and pressed Enter at once for
  // a new line, and the cursor was put back in front of it, so what he
  // typed next went before the new line, issue #373.
  function finish(kept: boolean) {
    const was = said && field.contains(said) ? said : null;
    mark?.remove();
    said = mark = null;
    if (!was) return read();
    const top = field.scrollTop;
    const at = document.createRange();
    at.setStartBefore(was);
    at.setEndAfter(was);
    const whole = was.textContent!;
    // Only the spaces around it, not a new line typed at its end.
    const text = whole.replace(/^ +| +$/g, "");
    const cut = whole.length - whole.trimStart().length;
    const [before, after] = around(was);
    const sel = getSelection();
    const had = sel?.rangeCount && field.contains(sel.anchorNode) ? [count(sel.anchorNode!, sel.anchorOffset), count(sel.focusNode!, sel.focusOffset)] : null;
    was.remove();
    let words = "";
    if (kept && text) {
      words = (/\S$/.test(before) && /^\S/.test(text) ? " " : "") + text + (/\S$/.test(text) && /^\S/.test(after) ? " " : "");
      field.focus({ preventScroll: true });
      getSelection()?.removeAllRanges();
      getSelection()?.addRange(at);
      if (!write(words)) {
        at.deleteContents();
        at.insertNode(document.createTextNode(words));
      }
    }
    // Where the cursor was, counted in the text, moved by what the words
    // became: before them it stays, after them it moves with the text
    // after them, and in them it keeps its place among their letters.
    if (had) {
      const lead = words.length - words.trimStart().length;
      const moved = (n: number) =>
        n <= before.length
          ? n
          : n >= before.length + whole.length
            ? n - whole.length + words.length
            : before.length + Math.min(lead + Math.max(n - before.length - cut, 0), words.length);
      const [a, b] = [point(moved(had[0])), point(moved(had[1]))];
      getSelection()?.setBaseAndExtent(a[0], a[1], b[0], b[1]);
    }
    // The field stays as dictating left it, with the end of the report in
    // view, down to its padding, when what was said ends it, however the
    // dictation was stopped. Stop on Tim's Mac left it half a line up,
    // where Enter did not, since the browser put the cursor in view its
    // own way.
    field.scrollTop = after.trim() || !following ? top : field.scrollHeight;
    ours = false;
    read();
  }

  onMount(() =>
    onLoudness((n) => {
      if (listening && (!recording || n.recording === recording)) loud = n.level;
    }),
  );

  onMount(() =>
    onDictation((d) => {
      if (!listening || (recording && d.recording !== recording)) return;
      if (d.error || d.ended) {
        if (d.error) oops(sentence(d.error));
        if (d.ended) note = sentence(d.ended) + ".";
        listening = false;
        chime("off");
        loud = 0;
        catchUp(d.said ?? []);
        finish(true);
        return;
      }
      unheard = d.unheard ? sentence(d.unheard) : "";
      if ((d.asked ?? 0) < asked) return;
      catchUp(d.said ?? []);
      show(d.hearing);
    }),
  );

  async function dictate() {
    if (hearing) return;
    if (listening) return keep();
    unclear();
    note = "";
    unheard = "";
    listening = true;
    recording = 0;
    placed = 0;
    asked = 0;
    begin();
    wake();
    try {
      recording = await api.listen();
      // The microphone is open: what is said from here is written down.
      if (listening) chime("on");
    } catch (e) {
      listening = false;
      finish(false);
      oops(sentence(e));
    }
  }

  // keep stops listening and keeps everything that was said, the rest
  // written in once the speech model has it.
  async function keep() {
    if (!listening) return;
    listening = false;
    chime("off");
    loud = 0;
    hearing = true;
    let written = () => {};
    writing = new Promise((done) => (written = done));
    try {
      // It may have stopped by itself just before, and then says why.
      const d = await api.stopListening();
      if (d.recording === recording) {
        catchUp(d.said ?? []);
        if (d.ended) note = sentence(d.ended) + ".";
      }
    } catch (e) {
      oops(sentence(e));
    } finally {
      finish(true);
      hearing = false;
      written();
    }
  }

  // Enter and Escape keep what was said, the way the button does.
  // Removing it is Cmd+Z after, one step, and Cmd+Shift+Z brings it back,
  // so no key throws a report away for good.
  function key(e: KeyboardEvent) {
    if (e.key === "Tab" && !e.isComposing && !e.altKey && !e.metaKey && !e.ctrlKey) return void tab(e);
    if (!listening) return;
    if (e.key === "Enter" && !e.isComposing && !e.shiftKey && !e.altKey && !e.metaKey && !e.ctrlKey) {
      e.preventDefault();
      keep();
    } else if (e.key === "Escape" && !e.isComposing) {
      e.preventDefault();
      e.stopPropagation();
      keep();
    }
  }

  // Tab goes from the field to Report, then through the pictures and
  // back to the field, and Shift+Tab the other way, issue #404. Tim ends
  // a dictation with Enter and makes the report with Tab and Enter. macOS
  // only tabs between buttons when full keyboard access is on, so WebKit
  // passed over Report to the first picture, or with none to nothing on
  // the page. The page moves the focus itself, as Confirm.svelte does.
  // While dictating Tab does nothing: only Enter, Escape and the button
  // end a dictation, as Tim asked.
  const stops = () => [field, report, ...document.querySelectorAll<HTMLElement>(".pictures figure, .pictures button.back")];
  const named = (e: Element | null) => (e === field ? "the field" : e === report ? "Report" : (e?.getAttribute("aria-label") ?? e?.textContent?.trim() ?? "nothing"));
  function tab(e: KeyboardEvent) {
    const here = document.activeElement as HTMLElement | null;
    if (listening) {
      e.preventDefault();
      return;
    }
    if (!here || !stops().includes(here)) return;
    e.preventDefault();
    const open = stops().filter((s) => s === here || !(s as HTMLButtonElement).disabled);
    const next = open[(open.indexOf(here) + (e.shiftKey ? -1 : 1) + open.length) % open.length];
    next.focus();
    if (next !== here) log(`report: ${e.shiftKey ? "Shift+Tab" : "Tab"} moved the focus from ${named(here)} to ${named(next)}`);
  }

  // A recording is not left running behind another page.
  onDestroy(() => {
    if (listening) api.cancelListening().catch(() => {});
  });

  onMount(() => {
    document.addEventListener("selectionchange", caret);
    return () => document.removeEventListener("selectionchange", caret);
  });

  // The draft from before, the window's last time open or the app's last
  // run, and what a drop on the window attaches.
  onMount(() => {
    api
      .reportDraft()
      .then((d) => {
        if (d.text && !what) {
          field.textContent = d.text;
          what = d.text;
        }
        took({ ...d, error: "" });
      })
      .catch(() => {})
      .finally(() => (loaded = true));
    const noDraft = onReportDraft(dropped);
    const leave = () => saveNow();
    window.addEventListener("pagehide", leave);
    return () => {
      noDraft();
      window.removeEventListener("pagehide", leave);
      saveNow();
    };
  });

  async function make() {
    if (making) return;
    making = true;
    note = "";
    // Pressed while the last words of a dictation are written down, it
    // shows it is making the report at once and waits for them, so they
    // are in it, issue #404.
    if (hearing) log("report: Report waits for the last words of the dictation", { level: "info" });
    await writing;
    // Nothing was said after all, so there is nothing to report.
    if (!what.trim() && pictures.length === 0) {
      making = false;
      return;
    }
    // Nothing is kept from here: the Go side lets go of the draft and
    // closes the window, which would otherwise keep the text again as it
    // went.
    if (saving) clearTimeout(saving);
    saving = null;
    loaded = false;
    try {
      // Finder shows the report, and the Go side closes this window.
      await api.reportProblem(what, "");
      // The report holds it all, and the next one starts empty.
      field.textContent = "";
      what = "";
      pictures = [];
      removed = {};
    } catch (e) {
      oops(e instanceof Error ? e.message : String(e));
    } finally {
      making = false;
      loaded = true;
    }
  }
</script>

<svelte:window onkeydowncapture={key} />

<section class="scroll">
  <div class="card">
    <div class="item">
      <!-- A field of plain text that can hold the words still being heard,
           which a textarea cannot. The placeholder lies over it in a box
           of their own, so it lines up with the field's text. -->
      <div class="box">
        <div
          class="telling selectable"
          class:ours
          bind:this={field}
          contenteditable={making ? "false" : "plaintext-only"}
          aria-readonly={making}
          role="textbox"
          tabindex="0"
          aria-multiline="true"
          aria-label="What happened"
          spellcheck="true"
          oninput={typed}
          onpaste={pasted}
          onbeforeinput={follow}
          onpointerdown={freeze}
          onscroll={scrolled}
          onkeyup={remember}
          onmouseup={remember}
          onblur={remember}
        ></div>
        {#if !what && !listening}
          <span class="placeholder" aria-hidden="true">What happened, and what were you doing when it did?</span>
        {/if}
      </div>
    </div>
    <div class="item">
      <!-- Square, the microphone alone, the way a toolbar shows it: what
           it does is in its title. Its name says what a press does now,
           Dictate, Stop or Writing, for the keyboard and a screen reader. -->
      <button
        class="act square dictate"
        class:listening
        onclick={dictate}
        disabled={hearing || making}
        data-loud={loud}
        aria-label={listening ? "Stop" : hearing ? "Writing" : "Dictate"}
        title={listening
          ? "Stop. Keeps what you said, and Enter or Escape does the same. Cmd+Z undoes it"
          : hearing
            ? "Writing down what you said"
            : "Dictate. Say what happened. The speech model on this computer writes it into the field as you talk, and you can correct it while it does"}
      >
        {#if listening || hearing}<Busy />{/if}
        <span class="grows" bind:this={mic}><Icon name="mic" size={28} line={1.5 * (28 / 16)} /></span>
      </button>
      <button
        class="act square attach"
        onclick={attach}
        disabled={attaching || making}
        aria-label="Attach screenshots or recordings"
        title="Attach screenshots or screen recordings. You can also drop them on this window or paste them"
      >
        {#if attaching}<Busy />{/if}
        <Icon name="attach" />
      </button>
      <!-- Only what the microphone hears, and why a recording stopped by
           itself. What the buttons are doing shows on the buttons, how the
           page works is in the info mark, and what went wrong is a toast.
           Tim found standing lines there read as news that was not true
           yet. -->
      <div class="words">
        {#if listening && unheard}
          <span class="error selectable">{unheard}</span>
        {:else if !listening && !hearing && note}
          <span>{note}</span>
        {/if}
      </div>
      <!-- Beside the button rather than in the card's corner, where it
           pushed the field off the middle of its row, and always there,
           as Tim asked, since what goes in a report is worth seeing
           before it is made. -->
      <span class="ask always">
        <Info label="How a report works" side="right">
          Say what happened with the microphone, or type it. Enter or Escape keeps what you said, and Cmd+Z undoes it. Attach
          screenshots or screen recordings with the paperclip, by dropping them on this window or by pasting them, up
          to ten of 10 MB each.
          What you write and attach stays here when you close the window, and Clear empties it all after asking, which Cmd+Z undoes. Report saves one zip file on the Desktop,
          shows it in Finder and closes this window. It holds what you wrote, the pictures and recordings without what
          else their files hold, like a place or a device, the app's log, which build this is, what Mac it runs on and your settings. Never a video, its
          words or its captions, and your home folder is written as ~. You can look inside before you send it.
        </Info>
      </span>
      <!-- Clear and then Report, the buttons of words on the right as the
           buttons of icons are on the left, as Tim asked. Clear asks
           first and Cmd+Z undoes it, so a slip of the hand costs nothing. -->
      <button
        class="act danger clear"
        onclick={() => (clearing = true)}
        disabled={nothing || making || listening || hearing || attaching}
        title="Empties this report, what you wrote and everything attached, after asking"
      >
        Clear
      </button>
      <button bind:this={report} class="act primary report" onclick={make} disabled={nothing || making || listening || attaching} title="Saves the report on the Desktop, shows it in Finder and closes this window">
        {#if making}<Busy />{/if}
        {making ? "Making…" : "Report"}
      </button>
    </div>
  </div>
  {#if tiles.length > 0}
    <!-- The pictures attached, in the order they came, each with a bin
         on hover. A removed one keeps its place for a moment, says so and
         offers itself back, the way a removed clip's row does. -->
    <div class="shots">
    <ul class="pictures" aria-label="Pictures">
      {#each tiles as t (t.p.id)}
        <li class="tile" data-id={t.p.id}>
          {#if t.gone}
            <Removed
              tile
              onputback={() => putBack(t.p.id)}
              onend={() => delete removed[t.p.id]}
            />
          {:else}
            <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
            <figure
              tabindex="0"
              title={t.p.name}
              aria-label={t.p.recording ? `${t.p.name}, a recording of ${clock(t.p.seconds ?? 0)}` : `${t.p.name}, ${t.p.width} by ${t.p.height}`}
              onkeydown={(e) => tileKey(e, t.p)}
            >
              <img src="/report-picture?id={t.p.id}" alt={t.p.name} draggable="false" />
              {#if t.p.recording}
                <!-- How long a recording is, in the corner Photos and
                     Finder show it in. -->
                <span class="length">{clock(t.p.seconds ?? 0)}</span>
              {/if}
            </figure>
            <button class="glyph small quiet danger bin" title="Remove" aria-label="Remove {t.p.name}" onclick={() => removePicture(t.p)}>
              <Icon name="trash" />
            </button>
          {/if}
        </li>
      {/each}
    </ul>
    </div>
  {/if}
  {#if clearing}
    <Confirm title="Clear the report?" oncancel={() => (clearing = false)}>
      <p>Everything you wrote and attached will be deleted. You can undo it with Cmd+Z.</p>
      {#snippet actions()}
        <button onclick={() => (clearing = false)}>Cancel</button>
        <button class="danger" onclick={clearAll}>Clear</button>
      {/snippet}
    </Confirm>
  {/if}
</section>

<style>
  /* The same page as the settings and the updates, kept to a width that
     reads, in the middle of the app. */
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

  /* The card and the pictures keep their own height, and the page
     scrolls once the pictures reach past the window's foot. Left to
     shrink, the card gave way to the pictures and its field was
     squeezed to nothing behind the buttons. */
  section > :global(*) {
    flex: none;
  }

  .always {
    opacity: 1;
    pointer-events: auto;
  }

  /* A field of ten whole lines of the card's 19 px, with the padding of a
     control round them. */
  .box {
    position: relative;
    flex: 1;
    display: flex;
  }

  .telling {
    flex: 1;
    height: calc(10 * 19px + 2 * 8px + 2px);
    box-sizing: border-box;
    overflow-y: auto;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    font: inherit;
    line-height: 19px;
    color: inherit;
    background: var(--ink-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    padding: 8px 12px;
  }

  /* Where a textarea has its placeholder, in the field's own border and
     padding, 1 px and 12 px across, 1 px and 8 px down. */
  .placeholder {
    position: absolute;
    left: 13px;
    top: 9px;
    line-height: 19px;
    color: var(--muted);
    pointer-events: none;
  }

  /* What is being said: grey and slanted until it is kept, the way
     claude.ai shows it. What is still being heard is in a mark that
     cannot be typed in. */
  .telling :global(.dictated) {
    color: var(--muted);
    font-style: italic;
  }

  /* The cursor while it is right after the words being said, drawn by
     the end of the mark, which is always where those words end, however
     they wrap. The field's own is hidden then. It blinks the way the
     system's does, and takes no room, so nothing moves as it goes. */
  .telling.ours {
    caret-color: transparent;
  }

  .telling.ours :global(.hearing) {
    border-right: 1px solid var(--text);
    margin-right: -1px;
    animation: caret 1.06s steps(1) infinite;
  }

  @keyframes caret {
    50% {
      border-right-color: transparent;
    }
  }

  .act {
    flex: none;
  }

  /* The microphone and the paperclip are square, as tall as every control
     in the row and as wide. */
  .act.square {
    width: var(--control-h);
    padding: 0;
  }

  /* The microphone moves with the voice it hears, so the person sees they
     are heard. It grows from 16 px to 28 px about its middle, in a box of
     28 px that never changes, so nothing in the row moves and the icon
     does not shift by a pixel as it grows. It is drawn at its largest and
     scaled down to rest, with a line as much thicker, so at rest it is
     the icon every other button has, and grown it is drawn from a picture
     at least as large as it is shown, never one blown up: WebKit draws a
     scale of a small picture blurred, and a size that changed instead
     snapped to whole pixels as it grew and shook. It stays white, like
     every icon in the app. */
  .act.dictate .grows {
    display: block;
    flex: none;
    width: 28px;
    height: 28px;
    transform: scale(calc(16 / 28));
    transform-origin: 50% 50%;
  }

  .act.square {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
  }

  /* The pictures under the card, a space below it, in tiles of one size
     with a space between them: three across, four once the page is wide
     enough for four of 150 px. Every tile has the shape 16 to 10, the
     picture whole inside it. A tile's width and height are rounded down
     to whole pixels, so every tile and the bin in its corner stand on
     whole pixels, and what that leaves, less than a pixel a tile, stays
     at the end of the row. The width is worked out from the room the
     page has, 100cqw of .shots, in the stylesheet. */
  .shots {
    container: shots / inline-size;
  }

  .pictures {
    --cols: 3;
    --tile-w: round(down, calc((100cqw - (var(--cols) - 1) * var(--gap)) / var(--cols)), 1px);
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(var(--cols), var(--tile-w));
    grid-auto-rows: round(down, calc(var(--tile-w) * 10 / 16), 1px);
    gap: var(--gap);
  }

  @container shots (min-width: 636px) {
    .pictures {
      --cols: 4;
    }
  }

  .tile {
    position: relative;
    min-width: 0;
  }

  .tile figure {
    margin: 0;
    position: absolute;
    inset: 0;
    background: var(--ink-2);
    border: 1px solid var(--line);
    border-radius: var(--radius-s);
    overflow: hidden;
  }

  .tile figure:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 1px;
  }

  /* The picture fills the tile's inside and is fitted into it whole, in
     its middle, as large as the tile lets it be, small ones too. Sized by
     max-width and max-height in a grid, it was drawn at its own size and
     cut off, a strip of a tall one or a corner of a small one. */
  .tile img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: contain;
    display: block;
  }

  /* The bin in the tile's top right corner, only while the pointer is on
     the tile or the keyboard is in it. It is the quiet trash can of every
     list, with no box of its own: on a solid grey square it looked pasted
     on, Tim found, next to how it looks under the hand. A shade falls
     from that corner instead, so it reads on any picture, and the tile's
     edge lifts the way a row's does. It is the sidebar's bin, 24 px
     square and 6 px in from the corner, see .tools in App.svelte, and
     close to the clip list's. A space in, and as tall as a control, it
     stood too far into the picture, Tim found. */
  .tile .bin {
    position: absolute;
    top: 6px;
    right: 6px;
    width: 24px;
    height: 24px;
    opacity: 0;
    pointer-events: none;
    color: var(--text);
  }

  /* A recording's length, at the foot of the tile on the right, the way
     Photos and Finder show a video's, on a shade of its own so it reads
     on any frame. */
  .tile .length {
    position: absolute;
    right: 0;
    bottom: 0;
    padding: 12px var(--gap) 4px 16px;
    font-size: var(--size-s);
    line-height: 16px;
    font-variant-numeric: tabular-nums;
    color: var(--text);
    background: radial-gradient(circle at 100% 100%, var(--backdrop), transparent 100%);
    pointer-events: none;
  }

  .tile figure::after {
    content: "";
    position: absolute;
    inset: 0;
    background: radial-gradient(circle at 100% 0, var(--backdrop), transparent calc(3 * var(--control-h)));
    opacity: 0;
    pointer-events: none;
  }

  .tile:hover figure,
  .tile:focus-within figure {
    border-color: var(--faint);
  }

  .tile:hover figure::after,
  .tile:focus-within figure::after,
  .tile:hover .bin,
  .tile:focus-within .bin {
    opacity: 1;
  }

  .tile:hover .bin,
  .tile:focus-within .bin {
    pointer-events: auto;
  }

</style>
