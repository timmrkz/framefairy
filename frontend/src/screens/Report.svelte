<script lang="ts">
  import { onDestroy, onMount, tick } from "svelte";
  import {
    api,
    clock,
    onDictation,
    onLoudness,
    onReportAbout,
    onReportDraft,
    onUndo,
    type Dictation,
    type ReportDraft,
    type ReportPicture,
  } from "../lib/api";
  import { asking, keyFocus, typing } from "../lib/keys";
  import Removed from "../components/Removed.svelte";
  import { chime, wake } from "../lib/chime";
  import Busy from "../components/Busy.svelte";
  import Icon from "../components/Icon.svelte";
  import Info from "../components/Info.svelte";
  import Confirm from "../components/Confirm.svelte";
  import { toasts } from "../lib/toast.svelte";
  // A line in the app's log. said is the words being dictated here.
  import { said as log } from "../lib/said";
  import { changed, Dictating, edit, type Ask, type Span } from "../lib/dictating";
  import { Undo } from "../lib/undo";

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

  // Dictate, plan row 2.189: the words appear in the field while they
  // are said, where the cursor was, and the last of them change as more
  // is heard until a pause settles them. They can be corrected while the
  // person talks on. Enter, Escape or the button keeps all of it, written
  // in as one step Cmd+Z takes back. Escape removed it once, as on
  // claude.ai, and Tim lost a long report to an Escape meant for the app,
  // with no Cmd+Z to bring it back, issue #355. On a Mac, Escape ends
  // Dictation and keeps what was said. The Go side records and writes
  // down, see listen.go.
  //
  // The field is a textarea, and the browser draws its cursor, moves it
  // and scrolls it, as in every text field. What the dictation does to the
  // text is worked out in lib/dictating.ts, in letters and numbers, and
  // written into the field where it changed. The field was a page of its
  // own making, which drew the words being heard in grey and was rebuilt
  // under the cursor ten times a second, and on Tim's Mac the cursor
  // bounced, jumped and was left drawn on every line (29 October).
  let field: HTMLTextAreaElement;
  let report: HTMLButtonElement;
  let listening = $state(false);
  let hearing = $state(false);
  // The last words of a dictation being written down, which Report
  // waits for, so Tab and Enter right after Enter make a report with
  // them in it, issue #404.
  let writing: Promise<void> = Promise.resolve();
  // The recording in hand, and how many times the Go side was told to
  // settle or cut what it is hearing, see tell.
  let recording = 0;
  let asked = 0;
  // The asks to settle or cut go to the Go side one at a time, in the
  // order they were made, and stopping waits for them. Each call runs on
  // a goroutine of its own there, so two Shift+Enters in quick succession
  // could arrive the other way round, and the words would land under the
  // wrong new line.
  let told: Promise<unknown> = Promise.resolve();
  function ask(call: () => Promise<unknown>) {
    told = told.then(call).catch(() => {});
  }
  // The dictation in the field, while there is one, and the pieces the
  // Go side last said it has written down.
  let dictation: Dictating | null = null;
  let pieces: string[] = [];
  // A letter being put together from more than one key, like ä on a
  // German keyboard. What the Go side says meanwhile waits for it, since
  // a change to the text would end it.
  let composing = false;
  let waited: Dictation | null = null;

  const sentence = (e: unknown) => {
    const said = e instanceof Error ? e.message : String(e);
    return said.charAt(0).toUpperCase() + said.slice(1);
  };

  // The field's text, as it is sent. While somebody dictates it holds
  // what is still being heard too, and Report waits until that is
  // kept.
  const read = () => {
    what = field.value;
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
  // the field's undo, so the field's Cmd+Z brings them back, and the
  // pictures follow that step. Anything done after it, a letter typed or
  // a picture added or removed, ends it, as in any editor.
  let cleared: { text: boolean; pics: ReportPicture[]; undone: boolean } | null = null;
  const unclear = () => (cleared = null);
  // The field's undo, which the page keeps, see lib/undo.ts, and the text
  // as it was when the dictation in hand began.
  let steps = new Undo();
  let began = "";

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
    const text = !!field.value;
    if (text) {
      steps.did(0, field.value.length, field.value, "", true);
      field.value = "";
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

  // Undo of Clear puts its pictures back one call at a time, and Redo
  // clears with a call of its own. Each waits for the one before to be
  // answered, or a Redo pressed while Undo was still putting pictures
  // back cleared between them, and the pictures put back after it stayed
  // in the report with the words gone, issue #554.
  let turn: Promise<void> = Promise.resolve();

  // Undo and Redo in the Edit menu, and the field's own Cmd+Z. After
  // Clear they bring everything back and clear it again. Otherwise they go
  // to the field, as on every page with no workspace, or Cmd+Z in it
  // would do nothing.
  async function undo(how: "undo" | "redo") {
    if (asking()) return;
    const c = cleared;
    if (!c || c.undone !== (how === "redo")) {
      if (document.activeElement === field) step(how);
      else if (typing()) document.execCommand(how);
      return;
    }
    c.undone = how === "undo";
    log(`report: ${how === "undo" ? "Undo brings back" : "Redo clears again"} the words and ${c.pics.length} pictures`, { level: "info" });
    field.focus({ preventScroll: true });
    if (c.text) step(how);
    const before = turn;
    let done = () => {};
    turn = new Promise((r) => (done = r));
    try {
      if (how === "redo") {
        if (saving) clearTimeout(saving);
        saving = null;
        pictures = pictures.filter((p) => !c.pics.some((q) => q.id === p.id));
        await before;
        try {
          took(await api.clearReport());
        } catch (e) {
          oops(sentence(e));
        }
        return;
      }
      pictures = [...pictures, ...c.pics].sort((a, b) => order(a.id) - order(b.id));
      await before;
      let last: ReportDraft | null = null;
      for (const p of c.pics) {
        try {
          last = await api.putBackPicture(p.id);
        } catch (e) {
          oops(sentence(e));
        }
      }
      // A Redo pressed meanwhile has taken the pictures away again, and
      // shows what its clear leaves.
      if (last && c.undone) took(last);
    } finally {
      done();
    }
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

  // The selection in the field, from where it starts to where it ends.
  const selection = (): Span => [field.selectionStart, field.selectionEnd];

  // show writes the dictation's text into the field where it changed, and
  // puts the selection where it is now, which was the selection before.
  // A field scrolled to its end stays at its end as the words come at the
  // end of the report, the way a terminal follows its output, while the
  // cursor is at the end of what was said or the focus is somewhere else.
  // Scrolled away to read, it stays where it was put, as Tim asked, and
  // so it does while the person is typing somewhere else in it.
  function show(d: Dictating, sel: Span, before: Span, end: number) {
    const bottom = field.scrollTop + field.clientHeight >= field.scrollHeight - 1;
    const follow = bottom && (document.activeElement !== field || (before[0] === end && before[1] === end));
    if (field.value !== d.text) {
      const [a, b, put] = changed(field.value, d.text);
      field.setRangeText(put, a, b);
    }
    if (field.selectionStart !== sel[0] || field.selectionEnd !== sel[1]) field.setSelectionRange(sel[0], sel[1], field.selectionDirection);
    if (follow && d.end === d.text.length) field.scrollTop = field.scrollHeight;
  }

  // tell passes on to the Go side what the person did to the words still
  // being heard, and what it says until it has taken that in is passed
  // over, since it would show those words a second time.
  function tell(a: Ask | null) {
    if (!a || !recording) return;
    const r = recording;
    asked++;
    if ("cut" in a) ask(() => api.breakListening(r, a.cut));
    else ask(() => api.settleListening(r, a.settle, a.words));
  }

  // take shows what the Go side says: what it has written down and what it
  // is still hearing. A cursor put in the words being heard, by a click or
  // a key, settles them first, so they are not changed under it, and they
  // can be corrected while the person talks on. Tim could not click into
  // them once.
  function take(m: Dictation) {
    const d = dictation;
    if (!d) return;
    if (composing) return void (waited = m);
    pieces = m.said ?? [];
    const before = selection();
    if (d.inside(before)) return tell(d.settle());
    const end = d.end;
    const sel = d.hear(pieces, m.hearing, before);
    for (const [a, b, n] of d.moves.splice(0)) steps.moved(a, b, n);
    show(d, sel, before, end);
    read();
  }

  // composed is the end of a letter put together from more than one key,
  // and shows what the Go side said meanwhile.
  function composed() {
    composing = false;
    const m = waited;
    waited = null;
    if (m) take(m);
  }

  // editing hears a key in the field before the browser makes the edit.
  // Cmd+Z and Cmd+Shift+Z are the field's undo, see undo, and a word
  // typed right after a word that was said gets a space in front of it,
  // as the letter is typed, so the two are one step.
  function editing(e: InputEvent) {
    if (e.inputType === "historyUndo" || e.inputType === "historyRedo") {
      e.preventDefault();
      return void undo(e.inputType === "historyUndo" ? "undo" : "redo");
    }
    if (!dictation || e.isComposing || e.inputType !== "insertText" || !e.data) return;
    const put = dictation.typing(selection(), e.data);
    if (put === e.data) return;
    e.preventDefault();
    document.execCommand("insertText", false, put);
  }

  // typed follows an edit the person made, which the browser has made in
  // the field. It is a step of the field's undo, and the dictation in
  // hand is told of it.
  function typed() {
    unclear();
    const was = dictation?.text ?? what;
    const [a, b, put] = edit(was, field.value, field.selectionEnd);
    steps.did(a, b, was.slice(a, b), put);
    follow(a, b, put);
    read();
  }

  // follow tells the dictation in hand of a change in the field, made by
  // a key or by Undo, and the Go side what it means for the words still
  // being heard, see edited in lib/dictating.ts.
  function follow(a: number, b: number, put: string) {
    if (!dictation) return;
    for (const x of dictation.edited(a, b, put, listening)) tell(x);
    dictation.moves.length = 0;
  }

  // step takes the field's last step back, or does again the last one
  // taken back, with the cursor after it.
  function step(how: "undo" | "redo") {
    const c = how === "undo" ? steps.undo(field.value) : steps.redo(field.value);
    if (!c) return;
    field.setRangeText(c[2], c[0], c[1], "end");
    follow(...c);
    read();
  }

  // finish ends the dictation with every piece the speech model wrote
  // down, and the selection stays where the person has it. What was said,
  // with whatever was typed and corrected meanwhile, is one step Cmd+Z
  // takes back.
  function finish(said: string[]) {
    const d = dictation;
    dictation = null;
    waited = null;
    if (!d) return read();
    const before = selection();
    const end = d.end;
    show(d, d.hear(said, "", before, true), before, end);
    steps.end(began, d.text);
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
        finish(d.said ?? pieces);
        return;
      }
      unheard = d.unheard ? sentence(d.unheard) : "";
      if ((d.asked ?? 0) < asked) return;
      take(d);
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
    asked = 0;
    pieces = [];
    told = Promise.resolve();
    // What is said goes where the cursor is, or where it was when the
    // button was pressed, which a textarea keeps.
    const at = field.selectionEnd;
    dictation = new Dictating(field.value, at);
    began = field.value;
    steps.begin();
    field.focus({ preventScroll: true });
    field.setSelectionRange(at, at);
    wake();
    try {
      recording = await api.listen();
      // The microphone is open: what is said from here is written down.
      if (listening) chime("on");
    } catch (e) {
      listening = false;
      finish([]);
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
      await told;
      // It may have stopped by itself just before, and then says why.
      const d = await api.stopListening();
      if (d.recording === recording) {
        pieces = d.said ?? pieces;
        if (d.ended) note = sentence(d.ended) + ".";
      }
    } catch (e) {
      oops(sentence(e));
    } finally {
      finish(pieces);
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
    keyFocus(next);
    if (next !== here) log(`report: ${e.shiftKey ? "Shift+Tab" : "Tab"} moved the focus from ${named(here)} to ${named(next)}`);
  }

  // A recording is not left running behind another page.
  onDestroy(() => {
    if (listening) api.cancelListening().catch(() => {});
  });

  // The draft from before, the window's last time open or the app's last
  // run, and what a drop on the window attaches.
  onMount(() => {
    api
      .reportDraft()
      .then((d) => {
        if (d.text && !what) {
          field.value = d.text;
          what = d.text;
        }
        took({ ...d, error: "" });
      })
      .catch(() => {})
      .finally(() => (loaded = true));
    const noDraft = onReportDraft(dropped);
    // A toast's Report button, while this window is open: its words go
    // after what is written, a line apart, and are kept like typing.
    const noAbout = onReportAbout((text) => {
      if (!text) return;
      // Words written in end what Cmd+Z could bring back after Clear.
      unclear();
      const was = what.replace(/\s+$/, "");
      const [a, b, put] = changed(what, was ? `${was}\n\n${text}` : text);
      field.setRangeText(put, a, b);
      steps.did(a, b, what.slice(a, b), put, true);
      follow(a, b, put);
      read();
    });
    const leave = () => saveNow();
    window.addEventListener("pagehide", leave);
    return () => {
      noDraft();
      noAbout();
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
      field.value = "";
      what = "";
      steps = new Undo();
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
      <!-- A plain textarea: the browser draws the cursor, moves it and
           scrolls, and lib/dictating.ts says what a dictation does to the
           text. -->
      <div class="box">
        <textarea
          class="telling selectable"
          bind:this={field}
          readonly={making}
          aria-label="What happened"
          placeholder={listening ? "" : "What happened, and what were you doing when it did?"}
          spellcheck="true"
          oninput={typed}
          onbeforeinput={editing}
          oncompositionstart={() => (composing = true)}
          oncompositionend={composed}
          onpaste={pasted}
        ></textarea>
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
    margin: 0;
    resize: none;
  }

  /* What the field is for, before anything is in it, in the grey it had
     before the field was a textarea. */
  .telling::placeholder {
    color: var(--muted);
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
