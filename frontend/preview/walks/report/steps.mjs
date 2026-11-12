// The steps of the sequences about Report a Problem, the toasts and
// dictation. Each is said above it, a list with its name first.
// walkSequence in ../sequences.mjs walks it, and hands it the walk so far,
// the step's argument, what the caption box showed before it and the step's
// name. A sequence of any folder can take it. Below them the helpers only
// these steps use. See docs/TESTING.md.
import { ask, caughtUp, control, fromSidebar, hold, holdClock, letClockGo, letHeld, letPageTimeGo, nextPaint, noDeadlines, pageUntil, runClock, seenUntil, settle, sidebarStill, slowDown } from "../bridge.mjs";
import { dropOn, standsAt, stillStands, toastSays, toastsSay, url } from "../common/steps.mjs";

// The two notes of dictation, E5 and B5 in Hz, going up as it starts and
// down as it stops, see frontend/src/lib/chime.ts.
const LOW = 659.26;
const HIGH = 987.77;
const near = (n, pitch) => Math.abs(n.f - pitch) < 40;
// sounds waits until the window has started a note near first and one
// near second since note from, and gives every note since, however late
// they come. The walk's own timeout ends it if they never do.
async function sounds(page, from, first, second) {
  await page.evaluate(([from, a, b]) => window.__sound(from, a, b), [from, first, second]);
  return page.evaluate((from) => window.__notes.slice(from), from);
}
// theTwo says whether the notes are exactly two that play, the one near
// first and then the one near second.
function theTwo(notes, first, second) {
  return notes.length === 2 && !notes.some((n) => n.silent) && near(notes[0], first) && near(notes[1], second) && notes[0].at < notes[1].at;
}

export const steps = {
  // ["bugs"]           clicks Bugs in the sidebar, which has the app open
  //                    the report window or bring it forward, and opens
  //                    the report window as a second page if none is open,
  //                    where the steps after it act. See bugs.go
  async bugs(walk) {
    const { browser, errors, main } = walk;
    await fromSidebar(main, () => main.locator("aside button.bugs").click());
    if (!walk.reportPage) {
      walk.reportPage = await browser.newPage({ viewport: { width: 560, height: 640 } });
      noDeadlines(walk.reportPage);
      await slowDown(walk.reportPage);
      walk.reportPage.on("pageerror", (e) => errors.push(String(e)));
      // Every note the window plays, read from the sound itself as the
      // window starts it, with its pitch and whether the microphone had
      // said it was on by then, its answer to the latest Listen, for the
      // dictate step's sounds. The walk does not listen to the speakers:
      // an ear on the audio's output heard a start sound late after the
      // audio had rested, in Chromium, or never, in WebKit, while the
      // window had played it, #672 and #680. What the window starts is
      // what a person hears, as long as its audio runs or was asked to,
      // so a sound started on audio that was last asked to rest is
      // marked silent, and so is one the walk cannot read. A note's at counts the seconds of every sound started
      // before it, so notes come in the order they play.
      // window.__sound(from, a, b) settles once, from note from on, a
      // note near pitch a and one near pitch b have been started, and
      // window.__before is the loudest sample started since the latest
      // Listen was asked and before it answered.
      //
      // What the window plays is also written to the page's console,
      // which the walk's trace keeps: every note, every change of the
      // audio's state, every sound started and every wait for a sound.
      await walk.reportPage.addInitScript(() => {
        const told = (...a) => console.log("ear:", ...a);
        window.__notes = [];
        window.__before = 0;
        let waiting = [];
        // A wait settles once both pitches were started, or once a sound
        // was started that plays nothing, for the step to say so rather
        // than wait for good.
        const has = ({ from, a, b }) => {
          const since = window.__notes.slice(from);
          return since.some((n) => n.silent) || [a, b].every((p) => since.some((n) => Math.abs(n.f - p) < 40));
        };
        window.__sound = (from, a, b) =>
          new Promise((done) => {
            told(`waits for ${a} and ${b} from note ${from}, ${window.__notes.length} played`);
            const settled = () => {
              told(`played ${a} and ${b} from note ${from}`);
              done();
            };
            if (has({ from, a, b })) settled();
            else waiting.push({ from, a, b, done: settled });
          });
        // A sample louder than this is sound, and a pitch whose strength
        // is over it sounds. The app's notes are about ten times louder.
        const loud = 0.01;
        // The pitches a note is looked for at, in Hz.
        const pitches = [];
        for (let f = 200; f <= 2000; f += 10) pitches.push(f);
        const strength = (d, from, to, f, rate) => {
          const k = (2 * Math.PI * f) / rate;
          const c = 2 * Math.cos(k);
          let a = 0, b = 0;
          for (let i = from; i < to; i++) [a, b] = [d[i] + c * a - b, a];
          return (2 * Math.sqrt(Math.max(0, a * a + b * b - c * a * b))) / (to - from);
        };
        // notesIn reads the notes of a sound, 1024 samples at a time, each
        // at the moment of the sound it starts at, in seconds. A note
        // sounds on for as long as its own pitch is there, even where a
        // newer one is stronger, so it is read once.
        const notesIn = (d, rate) => {
          const notes = [];
          let sounding = [];
          for (let at = 0; at < d.length; at += 1024) {
            const to = Math.min(d.length, at + 1024);
            let first = at;
            while (first < to && Math.abs(d[first]) <= loud) first++;
            if (to - first < 256) {
              if (first === to) sounding = [];
              continue;
            }
            const s = pitches.map((f) => strength(d, first, to, f, rate));
            // A pure tone also shows weaker beside itself, so only a peak
            // near the strongest is a pitch that sounds.
            const most = Math.max(...s);
            const peaks = pitches.filter((f, i) => s[i] > loud && s[i] > most / 3 && s[i] >= (s[i - 1] ?? 0) && s[i] >= (s[i + 1] ?? 0));
            sounding = sounding.filter((f) => strength(d, first, to, f, rate) > loud / 2);
            for (const f of peaks) {
              if (sounding.some((g) => Math.abs(g - f) < 60)) continue;
              notes.push({ f, at: first / rate });
              sounding.push(f);
            }
          }
          return notes;
        };
        // Whether the microphone has said it is on, by its answer to the
        // latest Listen, and how many seconds of sound were started
        // before, which a note's at counts from.
        let micOn = false;
        let playedFor = 0;
        // What each audio was last asked, to run or to rest, which is what
        // it does once it can.
        const meant = new WeakMap();
        const wants = (c) => meant.get(c) ?? c.state;
        const Real = window.AudioContext;
        window.AudioContext = class extends Real {
          constructor(...a) {
            super(...a);
            told(`audio made, ${this.state}`);
            this.addEventListener("statechange", () => told(`audio ${this.state} at ${this.currentTime.toFixed(3)} s`));
            // Settles once the audio's clock runs, which it does only
            // after the click that woke it.
            window.__running = new Promise((done) => {
              const runs = () => this.state === "running" && done();
              this.addEventListener("statechange", runs);
              runs();
            });
          }
          resume() {
            meant.set(this, "running");
            // A resume that fails leaves the audio as it was.
            const p = super.resume();
            p.catch(() => meant.set(this, this.state));
            return p;
          }
          suspend() {
            meant.set(this, "suspended");
            return super.suspend();
          }
        };
        // The bridge says when a call is asked and when it is answered,
        // see wails-bridge.ts, before the window that asked hears the
        // answer. A sound the window starts after the answer to Listen
        // comes after the microphone said it was on, and one started
        // before it does not.
        window.addEventListener("answered", (e) => {
          if (e.detail !== "Listen") return;
          micOn = true;
          told("Listen answered");
        });
        window.addEventListener("asked", (e) => {
          if (e.detail !== "Listen") return;
          micOn = false;
          window.__before = 0;
          told("Listen asked");
        });
        // played reads a sound the window starts.
        // A sound started at once and played whole is what the walk
        // reads. One started later, from inside, for part of its length
        // or stopped is a sound the walk cannot read, and the order of the
        // notes fails on it rather than reading what was not played.
        const played = (node, how = []) => {
          const c = node.context;
          const at = how[0] ?? 0;
          const asked = at > c.currentTime || how.length > 1 ? `start(${how.join(", ")})` : "";
          if (!(node instanceof AudioBufferSourceNode) || !node.buffer || asked) {
            const why = asked || node.constructor.name;
            told(`a sound the walk cannot read, ${why}`);
            window.__notes.push({ f: 0, at: playedFor, open: micOn, silent: true, unread: why });
            for (const w of waiting.filter(has)) w.done();
            waiting = waiting.filter((w) => !has(w));
            return;
          }
          node.__read = true;
          const d = node.buffer.getChannelData(0);
          const rate = node.buffer.sampleRate;
          told(`a sound started, ${(d.length / rate).toFixed(3)} s, while the audio is ${c.state} and was asked to be ${wants(c)}${micOn ? "" : ", before the microphone was on"}`);
          const silent = wants(c) !== "running";
          if (!micOn && !silent) for (const x of d) window.__before = Math.max(window.__before, Math.abs(x));
          for (const n of notesIn(d, rate)) {
            const note = { f: n.f, at: playedFor + n.at, open: micOn, ...(silent && { silent }) };
            told(`note ${window.__notes.length}, ${note.f} Hz at ${note.at.toFixed(3)} s${note.open ? "" : ", before the microphone was on"}${silent ? ", on audio asked to rest, so it plays nothing" : ""}`);
            window.__notes.push(note);
          }
          playedFor += d.length / rate;
          for (const w of waiting.filter(has)) w.done();
          waiting = waiting.filter((w) => !has(w));
        };
        // A window that is late to start a sound, for the dictate step's
        // "sounded late": with window.__late set, every sound is started
        // only once that many seconds of silence have played, past the
        // moment it was meant to start. What it plays is read when it
        // really starts. A sound of samples has a start of its own in
        // WebKit, and a tone has the one every sound has, so both are
        // taken.
        const connect = AudioNode.prototype.connect;
        const real = new Map();
        for (const Node of [AudioScheduledSourceNode, AudioBufferSourceNode]) if (Object.hasOwn(Node.prototype, "start")) real.set(Node, Node.prototype.start);
        const startOf = (node) => real.get(node instanceof AudioBufferSourceNode && real.has(AudioBufferSourceNode) ? AudioBufferSourceNode : AudioScheduledSourceNode);
        const late = function (when = 0, ...a) {
          const start = startOf(this);
          if (!window.__late) {
            played(this, [when, ...a]);
            return start.call(this, when, ...a);
          }
          const c = this.context;
          const rest = c.createBufferSource();
          rest.buffer = c.createBuffer(1, Math.round(window.__late * c.sampleRate), c.sampleRate);
          connect.call(rest, c.destination);
          rest.onended = () => {
            played(this, [when, ...a]);
            start.call(this, when, ...a);
          };
          startOf(rest).call(rest, Math.max(when, c.currentTime));
        };
        for (const Node of real.keys()) Node.prototype.start = late;
        const stop = AudioScheduledSourceNode.prototype.stop;
        AudioScheduledSourceNode.prototype.stop = function (...a) {
          if (this.__read) {
            told("a sound read whole was stopped");
            window.__notes.push({ f: 0, at: playedFor, open: micOn, silent: true, unread: "stop()" });
            for (const w of waiting.filter(has)) w.done();
            waiting = waiting.filter((w) => !has(w));
          }
          return stop.apply(this, a);
        };
      });
      // Whether the page asked for the ring when it moved the focus
      // last, for the tab step: WebKit rings a focus moved by a script
      // after a click only when asked, and Chromium rings it anyway.
      await walk.reportPage.addInitScript(() => {
        const focus = HTMLElement.prototype.focus;
        HTMLElement.prototype.focus = function (options) {
          window.__focusAsked = { on: this, ring: options?.focusVisible === true };
          return focus.call(this, options);
        };
      });
      await walk.reportPage.goto(new URL("?window=report", url).toString());
    } else {
      await walk.reportPage.bringToFront();
    }
    walk.page = walk.reportPage;
    await walk.page.locator("button.dictate").waitFor();
    await settle(walk.page);
  },
  // ["mission control"]
  //                    hovers the sidebar open, then plays what macOS hands
  //                    the app with the pointer held still, as in Mission
  //                    Control: the pointer leaving the sidebar and coming
  //                    back to where it is, with no movement, again and
  //                    again. The sidebar closes and opens no more. A real
  //                    move away and back opens it again, and none while
  //                    the app is not in front
  async "mission control"(walk) {
    const { main } = walk;
    walk.page = main;
    const shut = () => main.evaluate(() => !document.querySelector("aside")?.classList.contains("open"));
    // Every time the sidebar opens from here on.
    await main.evaluate(() => {
      window.__sidebarOpened = 0;
      new MutationObserver(() => {
        const now = document.querySelector("aside").classList.contains("open");
        if (now && !window.__sidebarWas) window.__sidebarOpened++;
        window.__sidebarWas = now;
      }).observe(document.querySelector("aside"), { attributes: true, attributeFilter: ["class"] });
      window.__sidebarWas = document.querySelector("aside").classList.contains("open");
    });
    await main.mouse.move(750, 400);
    await sidebarStill(main);
    await main.mouse.move(20, 500);
    await sidebarStill(main);
    if (await shut()) {
      walk.wrong = "the sidebar did not open under the pointer";
      return;
    }
    // What WebKit makes of the mouse exited and entered macOS sends
    // while the pointer stands still: the pointer leaving, and moves
    // to where it is with no movement. Each time after the sidebar has
    // done whatever it does about the one before.
    const still = (leave) =>
      main.evaluate((leave) => {
        const aside = document.querySelector("aside");
        const at = { clientX: 20, clientY: 500, pointerType: "mouse", isPrimary: true, movementX: 0, movementY: 0 };
        if (leave) {
          aside.dispatchEvent(new PointerEvent("pointerout", { ...at, bubbles: true }));
          aside.dispatchEvent(new PointerEvent("pointerleave", at));
        } else {
          aside.dispatchEvent(new PointerEvent("pointerover", { ...at, bubbles: true }));
          aside.dispatchEvent(new PointerEvent("pointerenter", at));
          aside.dispatchEvent(new PointerEvent("pointermove", { ...at, bubbles: true }));
        }
      }, leave);
    const before = await main.evaluate(() => window.__sidebarOpened);
    for (let i = 0; i < 4; i++) {
      await still(true);
      await sidebarStill(main);
      await still(false);
      await sidebarStill(main);
    }
    const again = (await main.evaluate(() => window.__sidebarOpened)) - before;
    if (again) {
      walk.wrong = `the sidebar opened ${again} times more under a pointer that did not move`;
      return;
    }
    if (!(await shut())) {
      walk.wrong = "the sidebar is open after the pointer left it and came back without moving";
      return;
    }
    // The app goes behind another: the sidebar answers no pointer.
    await main.mouse.move(750, 400);
    await sidebarStill(main);
    await main.evaluate(() => {
      document.hasFocus = () => false;
      window.dispatchEvent(new FocusEvent("blur"));
    });
    await main.mouse.move(20, 500);
    await sidebarStill(main);
    if (!(await shut())) {
      walk.wrong = "the sidebar opened under the pointer while the app was not in front";
      return;
    }
    // In front again with the pointer on it, the next move opens it.
    // What the pointer did while another app was in front is no
    // move onto it the app has already seen.
    await main.evaluate(() => {
      delete document.hasFocus;
      window.dispatchEvent(new FocusEvent("focus"));
    });
    await main.mouse.move(24, 500);
    await sidebarStill(main);
    if (await shut()) {
      walk.wrong = "the sidebar stayed shut under a pointer that moved on it once the app was in front again";
      return;
    }
    await main.mouse.move(750, 400);
    await sidebarStill(main);
    await main.mouse.move(20, 500);
    await sidebarStill(main);
    if (await shut()) walk.wrong = "the sidebar did not open under the pointer once the app was in front again";
    await main.mouse.move(750, 400);
    await sidebarStill(main);
  },
  // ["leave with no move"]
  //                    hovers the sidebar open and, while it slides open
  //                    and after, plays what WebKit hands the app when what
  //                    lies under a still pointer changes: the pointer
  //                    leaving the sidebar for the workspace with no
  //                    movement. The sidebar stays open. A real move off it
  //                    closes it at once, and so does the pointer leaving
  //                    the app, to nothing or at a point outside it, #540
  async "leave with no move"(walk) {
    const { main } = walk;
    walk.page = main;
    const open = () => main.evaluate(() => document.querySelector("aside")?.classList.contains("open"));
    await main.mouse.move(750, 400);
    await sidebarStill(main);
    await main.evaluate(() => {
      window.__sidebarClosed = 0;
      let was = document.querySelector("aside").classList.contains("open");
      new MutationObserver(() => {
        const now = document.querySelector("aside").classList.contains("open");
        if (was && !now) window.__sidebarClosed++;
        was = now;
      }).observe(document.querySelector("aside"), { attributes: true, attributeFilter: ["class"] });
    });
    // The leave and what follows it, as WebKit sends them: out of the
    // sidebar into what is beside the rail, and a move with no movement
    // there.
    const leave = () =>
      main.evaluate(() => {
        const aside = document.querySelector("aside");
        const at = { clientX: 20, clientY: 500, pointerType: "mouse", isPrimary: true, movementX: 0, movementY: 0 };
        const beside = document.elementsFromPoint(200, 500).find((el) => !aside.contains(el)) ?? document.body;
        aside.dispatchEvent(new PointerEvent("pointerout", { ...at, bubbles: true, relatedTarget: beside }));
        aside.dispatchEvent(new PointerEvent("pointerleave", { ...at, relatedTarget: beside }));
        beside.dispatchEvent(new PointerEvent("pointerover", { ...at, bubbles: true, relatedTarget: aside }));
        beside.dispatchEvent(new PointerEvent("pointermove", { ...at, bubbles: true }));
      });
    await main.mouse.move(20, 500);
    await leave();
    await nextPaint(main);
    await leave();
    await sidebarStill(main);
    await leave();
    await sidebarStill(main);
    const closed = await main.evaluate(() => window.__sidebarClosed);
    if (closed || !(await open())) {
      walk.wrong = `the sidebar opened by the pointer closed ${closed} time${closed === 1 ? "" : "s"} on a leave with no move of the pointer`;
      return;
    }
    await main.mouse.move(750, 400);
    if (await open()) {
      walk.wrong = "the sidebar stayed open after the pointer moved off it";
      return;
    }
    await sidebarStill(main);
    await main.mouse.move(20, 500);
    await sidebarStill(main);
    await main.evaluate(() => {
      const at = { clientX: 20, clientY: 500, pointerType: "mouse", isPrimary: true };
      document.querySelector("aside").dispatchEvent(new PointerEvent("pointerleave", at));
    });
    if (await open()) {
      walk.wrong = "the sidebar stayed open after the pointer left the app";
      return;
    }
    // The same where WebKit names the page itself as where the pointer
    // went, at a point left of the app.
    await sidebarStill(main);
    await main.mouse.move(750, 400);
    await sidebarStill(main);
    await main.mouse.move(20, 500);
    await sidebarStill(main);
    await main.evaluate(() => {
      const at = { clientX: -1, clientY: 500, pointerType: "mouse", isPrimary: true, relatedTarget: document.documentElement };
      document.querySelector("aside").dispatchEvent(new PointerEvent("pointerleave", at));
    });
    if (await open()) walk.wrong = "the sidebar stayed open after the pointer left the app to its left";
    await sidebarStill(main);
    await main.mouse.move(750, 400);
    await sidebarStill(main);
  },
  // ["bugs, the pointer gone"]
  //                    clicks Bugs with the pointer on the sidebar, and
  //                    the app's window loses the focus to the report
  //                    window with no word of the pointer leaving, as on
  //                    a Mac. The sidebar closes then, stays closed when
  //                    the focus comes back to the button, and the next
  //                    move of the pointer outside leaves it closed
  async "bugs, the pointer gone"(walk) {
    const { main } = walk;
    walk.page = main;
    await main.mouse.move(20, 500);
    await settle(main);
    await sidebarStill(main);
    const shut = () => main.evaluate(() => !document.querySelector("aside")?.classList.contains("open"));
    if (await shut()) {
      walk.wrong = "the sidebar did not open under the pointer";
      return;
    }
    // Clicked from the page, so the pointer stays where it is: on a Mac
    // the report window comes up and the app's window hears nothing
    // more of the pointer.
    await main.evaluate(() => {
      document.querySelector("aside button.bugs").click();
      window.dispatchEvent(new FocusEvent("blur"));
    });
    await settle(main);
    await sidebarStill(main);
    if (!(await shut())) {
      walk.wrong = "the sidebar stayed open once the report window had the focus";
      return;
    }
    // The focus comes back to the button that was clicked.
    await main.evaluate(() => {
      window.dispatchEvent(new FocusEvent("focus"));
      document.querySelector("aside button.bugs").focus();
    });
    await settle(main);
    await sidebarStill(main);
    if (!(await shut())) {
      walk.wrong = "the sidebar opened when the app's window had the focus again";
      return;
    }
    await main.mouse.move(750, 400);
    await settle(main);
    await sidebarStill(main);
    if (!(await shut())) walk.wrong = "the sidebar opened on the next move of the pointer outside it";
  },
  // ["app log", "open"]
  //                    the app starts its log, with an older file beside
  //                    the one in use, see /app-log in bridge_test.go
  // ["app log", n]     Settings, Advanced, opened if it is not, says the
  //                    app's log has n files, and the Go side has as many
  async "app log"(walk, arg) {
    const { page } = walk;
    if (arg === "open") {
      await fetch(new URL("/app-log?open=1", url));
      return;
    }
    // Advanced reads the log again whenever it opens.
    const disclose = page.locator("button.disclose");
    if ((await disclose.getAttribute("aria-expanded")) !== "true") await disclose.click();
    const said = arg === 1 ? "1 file," : `${arg} files,`;
    const card = page.locator(".card", { hasText: "The app's log" });
    await settle(page);
    const shown = ((await card.locator(".num").textContent().catch(() => "")) ?? "").trim();
    const kept = await (await fetch(new URL("/app-log", url))).json();
    if (!shown.startsWith(said)) walk.wrong = `Settings says the app's log is "${shown}", not ${arg} files`;
    else if (kept.files !== arg) walk.wrong = `the app's log has ${kept.files} files on the disk, not ${arg}`;
  },
  // ["windows", made, fronted]
  //                    the app has made so many report windows and
  //                    brought one forward so many times, and Bugs is lit
  //                    while one is open
  async windows(walk, arg) {
    const { main } = walk;
    const [made, fronted] = arg;
    const w = await (await fetch(new URL("/report-window", url))).json();
    if (w.made !== made || w.fronted !== fronted) {
      walk.wrong = `the app made ${w.made} report windows and brought one forward ${w.fronted} times, not ${made} and ${fronted}`;
      return;
    }
    const lit = await main.evaluate(() => document.querySelector("aside button.bugs")?.classList.contains("current"));
    if (lit !== w.open) walk.wrong = `Bugs is ${lit ? "" : "not "}lit while the report window is ${w.open ? "open" : "closed"}`;
  },
  // ["close report"]   closes the report window, the way Cmd+W does, and
  //                    goes back to the app's window
  async "close report"(walk) {
    const { main } = walk;
    // What was typed is kept a moment after the last key, see saving
    // in Report.svelte.
    await settle(walk.page);
    await fetch(new URL("/report-window?close=1", url));
    await walk.reportPage?.close();
    walk.reportPage = null;
    walk.page = main;
    await pageUntil(main, () => !document.querySelector("aside button.bugs")?.classList.contains("current"));
  },
  // ["attach", n]      presses the paperclip with n pictures waiting in
  //                    the open box
  // ["attach, as on a Mac", n]
  //                    the same, with a click that leaves the focus on
  //                    nothing, as WebKit on a Mac does: a button takes no
  //                    focus from the mouse there, and Chromium gives it
  //                    one
  async attach(walk, arg, s, what) {
    const { page } = walk;
    const before = await page.locator("li.tile").count();
    walk.standing = await standsAt(page);
    for (let i = 0; i < arg; i++) await fetch(new URL("/pick-picture", url));
    // After the page has heard the press, the way the browser's own
    // handling comes after it, so a press the page keeps from taking the
    // focus takes none here either.
    if (what === "attach, as on a Mac")
      await page.evaluate(() =>
        window.addEventListener(
          "mousedown",
          (e) => {
            if (e.defaultPrevented) return;
            e.preventDefault();
            document.activeElement?.blur();
          },
          { once: true },
        ),
      );
    await page.locator("button.attach").click();
    await pageUntil(page, (n) => document.querySelectorAll("li.tile").length >= n || !!document.querySelector(".toast"), before + arg);
    await settle(page);
  },
  // ["drop pictures", n, other]
  //                    drops n pictures on the report window, and a file of
  //                    text named other with them when there is one
  async "drop pictures"(walk, arg) {
    const { page } = walk;
    const [n, other] = Array.isArray(arg) ? arg : [arg];
    // The pictures that come take room under the field, so it may move.
    walk.standing = null;
    // A number of pictures, or their shapes, "446x738".
    const shapes = typeof n === "string" ? n.split(" ") : Array(n).fill("");
    const files = [];
    for (const shape of shapes) {
      const [w, h] = shape.split("x");
      files.push(await (await fetch(new URL(`/pick-picture?box=0${w ? `&w=${w}&h=${h}` : ""}`, url))).json());
    }
    if (other) files.push(await (await fetch(new URL(`/pick-picture?box=0&kind=text&name=${encodeURIComponent(other)}`, url))).json());
    const before = await page.locator("li.tile").count();
    walk.wrong = await dropOn(page, files);
    if (walk.wrong) return;
    await pageUntil(page, (n) => document.querySelectorAll("li.tile").length >= n, before + shapes.length);
    await settle(page);
  },
  // ["drop recording"] drops a screen recording of one second on the
  //                    report window, a MOV the way a Mac makes one
  async "drop recording"(walk) {
    const { page } = walk;
    walk.standing = null;
    const file = await (await fetch(new URL("/pick-picture?box=0&kind=recording", url))).json();
    const before = await page.locator("li.tile").count();
    walk.wrong = await dropOn(page, [file]);
    if (walk.wrong) return;
    await pageUntil(page, (n) => document.querySelectorAll("li.tile").length >= n, before + 1);
    await settle(page);
  },
  // ["recording tile", length]
  //                    the last tile is a recording, its still loaded and
  //                    its length written in its corner as length
  async "recording tile"(walk, arg) {
    const { page } = walk;
    const tile = await page.evaluate(() => {
      const all = document.querySelectorAll("li.tile figure");
      const f = all[all.length - 1];
      const img = f?.querySelector("img");
      return f && { length: f.querySelector(".length")?.textContent.trim() ?? "", loaded: !!img && img.complete && img.naturalWidth > 0, label: f.getAttribute("aria-label") };
    });
    if (!tile) walk.wrong = "there is no tile";
    else if (!tile.loaded) walk.wrong = "the still of the recording did not load";
    else if (tile.length !== arg) walk.wrong = `the recording's tile says "${tile.length}", not ${arg}`;
    else if (!tile.label.includes(`a recording of ${arg}`)) walk.wrong = `the recording's tile is read out as "${tile.label}"`;
  },
  // ["paste picture"]  pastes a picture into the report's field
  async "paste picture"(walk) {
    const { page } = walk;
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
    await pageUntil(page, (n) => document.querySelectorAll("li.tile").length >= n, before + 1);
    await settle(page);
  },
  // ["pictures", n]    the report window shows n pictures
  async pictures(walk, arg) {
    const { page } = walk;
    const n = await page.evaluate(() => document.querySelectorAll("li.tile figure img").length);
    if (n !== arg) walk.wrong = `the report window shows ${n} pictures, not ${arg}`;
    // Each one is the picture the Go side kept, loaded and shown. One
    // just put back is told when it has loaded, or failed to.
    const unseen = await page.evaluate(async () => {
      const all = [...document.querySelectorAll("li.tile figure img")];
      await Promise.all(
        all.filter((i) => !i.complete).map((i) => new Promise((done) => (i.addEventListener("load", done, { once: true }), i.addEventListener("error", done, { once: true })))),
      );
      return all.filter((i) => i.naturalWidth === 0).length;
    });
    if (!walk.wrong && unseen) walk.wrong = `${unseen} of the pictures did not load`;
  },
  // ["pictures fit"]   every picture is drawn whole in its tile, in its
  //                    middle, reaching both sides or top and bottom
  async "pictures fit"(walk) {
    const { page } = walk;
    // Every picture is drawn whole in its tile, in its middle, and as
    // large as the tile lets it be: it reaches both sides or the top
    // and the bottom. Tim saw tall pictures cut off and small ones
    // standing in a corner.
    // Once every picture is decoded, which each says by itself.
    await page.evaluate(() => Promise.all([...document.querySelectorAll("li.tile figure img")].map((i) => i.decode().catch(() => {}))));
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
    if (bad.length) walk.wrong = bad.join(", ");
    else if (!(await page.locator("li.tile figure img").count())) walk.wrong = "there is no picture to fit";
  },
  // ["report at end"]  the report's field is scrolled to its end, the
  //                    padding under its last line in view as the padding
  //                    over its first is at the top. Tim saw none under
  //                    it (30 October)
  async "report at end"(walk) {
    const { page } = walk;
    const scrolls = await page.locator(".telling").evaluate((f) => f.scrollHeight > f.clientHeight);
    if (!scrolls) walk.wrong = "the report is not long enough to scroll";
    else walk.wrong = await page.locator(".telling").evaluate(atEnd);
  },
  // ["toast", text]    a toast over the window says text, and with "" no
  //                    toast is there. With a list of texts, the toasts
  //                    say each and nothing else, newest first. Nothing
  //                    under it moved since the step before it, and the
  //                    line beside the report's buttons says nothing (#327)
  async toast(walk, arg) {
    const { page } = walk;
    // No toast, or the whole list of them, is read once the app has
    // heard all it was told, since a toast waited for would go by
    // itself after ten seconds and pass, and so would one the step
    // before should not have added. One that is wanted is read until
    // it comes.
    const all = arg === "" ? [] : Array.isArray(arg) ? arg : null;
    const said = all
      ? (await caughtUp(page, url), await toastsSay(page))
      : await seenUntil(page, url, () => toastSays(page), (said) => said.includes(arg));
    const line = await page.evaluate(() => document.querySelector(".words")?.textContent.trim() ?? "");
    if (all ? said.length !== all.length || said.some((t, i) => !t.includes(all[i])) : !said.includes(arg))
      walk.wrong = `the toasts say ${JSON.stringify(said)}, not ${JSON.stringify(all ?? arg)}`;
    else if (line) walk.wrong = `the line beside the buttons says "${line}", where only the toast should`;
    else walk.wrong = await stillStands(page, walk.standing);
  },
  // ["toast goes"]     on a held clock, with the pointer away, the toast is
  //                    still there after nine seconds and gone by itself
  //                    after ten and a half, then the clock goes on
  async "toast goes"(walk) {
    const { page } = walk;
    // Its time runs from the moment it comes, with no hand on it. Tim
    // found a toast that waited for the pointer before its time began.
    // Its time is the ten seconds a removed row has, so it is still
    // there after nine. It once ran eight of its own. The clock was
    // held before the toast came, so the nine seconds are the page's
    // own and not what a busy machine spent on the steps before.
    await page.mouse.move(2, 400);
    await runClock(page, 9000);
    if (!(await toastSays(page))) {
      walk.wrong = "the toast went within nine seconds, sooner than the ten a removed row has";
      return;
    }
    await runClock(page, 1500);
    if (await toastSays(page)) walk.wrong = "the toast was still there ten and a half seconds after it came, with the pointer away";
    await letClockGo(page);
  },
  // ["toast held"]     on a held clock, the pointer stays on the toast
  //                    longer than its time and it stays, then the pointer
  //                    goes and so does the toast, after ten and a half
  //                    seconds, then the clock goes on
  async "toast held"(walk) {
    const { page } = walk;
    const box = await page.locator(".toast .text").first().boundingBox();
    await page.mouse.move(box.x + 4, box.y + box.height / 2);
    await runClock(page, 11500);
    if (!(await toastSays(page))) {
      walk.wrong = "the toast went while the pointer was on it";
      return;
    }
    await page.mouse.move(2, 400);
    await runClock(page, 10500);
    if (await toastSays(page)) walk.wrong = "the toast was still there ten and a half seconds after the pointer left it";
    await letClockGo(page);
  },
  // ["toast closed"]   presses the cross on the top toast, and it goes
  async "toast closed"(walk) {
    const { page } = walk;
    const before = await page.locator(".toast").count();
    await page.locator(".toast button.close").first().hover();
    // At once: by the next frame after the click the toast is leaving,
    // which Svelte marks inert as its fade starts, see foldsDone in
    // bridge.mjs. Then gone once the app has settled.
    const leaving = await page.evaluate(() => {
      document.querySelector(".toast button.close").click();
      return new Promise((done) => requestAnimationFrame(() => done(document.querySelectorAll(".toast:not([inert])").length)));
    });
    await settle(page);
    if (leaving !== before - 1) walk.wrong = "the toast did not start to go in the frame its cross was pressed";
    else if ((await page.locator(".toast").count()) !== before - 1) walk.wrong = "the toast is still there after its cross was pressed";
  },
  // ["remove picture", i]
  //                    points at picture i, from 1, sees its bin and no
  //                    other, and presses it. The picture keeps its place
  //                    and says Removed, its time standing full while the
  //                    pointer is on it, through the made-up leave and
  //                    over a browser may send while the node under a
  //                    still pointer goes. [i, "key"] presses Delete on
  //                    the tile instead, the pointer standing on it.
  //                    [i, "tile"] presses the bin, and the browser says
  //                    the pointer is over the tile round the picture and
  //                    never over the picture itself, the way WebKit may
  async "remove picture"(walk, arg) {
    const { page } = walk;
    // By Delete on the tile under a pointer that stands still, with
    // "key", the way a Mac draws a row that comes up under it: not
    // under the pointer until the pointer moves.
    const [i, how] = [arg].flat();
    const tile = page.locator("li.tile").nth(i - 1);
    await tile.hover();
    await settle(page);
    const shown = await page.evaluate(() => [...document.querySelectorAll("li.tile .bin")].map((b) => getComputedStyle(b).opacity));
    if (shown[i - 1] !== "1" || shown.filter((o) => o === "1").length !== 1) {
      walk.wrong = `pointing at picture ${i} the bins stand at ${shown.join(", ")}`;
      return;
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
    if (gone === null) walk.wrong = `picture ${i} did not say Removed in its place`;
    // It says so where the picture was, with nothing sliding in. Tim
    // saw a swipe across the tile.
    else if (gone !== "none") walk.wrong = `picture ${i} says Removed with the animation ${gone}`;
    if (walk.wrong) return;
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
    // moment and stop a little short of full. Once the app has heard
    // all of it, the fill is held, with no animation of it running,
    // and stands full. A frame of running, of a time of seconds, is
    // more than the tenth of a percent allowed, so it is seen then
    // with no wait for it to move further.
    await settle(page);
    const { fill, running } = await page.evaluate((id) => {
      const run = document.querySelector(`li.tile[data-id="${id}"] .gone .run`);
      const box = run?.parentElement?.getBoundingClientRect();
      return {
        fill: run && box ? -new DOMMatrix(getComputedStyle(run).transform).m41 / box.width : null,
        running: !!run?.getAnimations().some((a) => a.playState === "running"),
      };
    }, id);
    if (running) {
      const heard = await page.evaluate(() => window.__heardPointer.slice(0, 12).join(", "));
      walk.wrong = `picture ${i}'s time runs with the pointer still on it, the page heard ${heard}`;
    } else if (fill === null || fill > 0.001) {
      const heard = await page.evaluate(() => window.__heardPointer.slice(0, 12).join(", "));
      walk.wrong = `picture ${i}'s time ran out by ${Math.round((fill ?? 1) * 1000) / 10} % with the pointer still on it, the page heard ${heard}`;
    }
    await settle(page);
  },
  // ["put back picture", i]
  //                    presses Put it back on picture i, which is back in
  //                    the same frame
  async "put back picture"(walk, arg) {
    const { page } = walk;
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
      walk.wrong = `the grid went from ${counts[0]} tiles to ${counts[1]} on Put it back, before the picture came back`;
      return;
    }
    await settle(page);
  },
  // ["removed goes"]   on a held clock, the pointer leaves, the picture is
  //                    still there half a second before the time to put it
  //                    back runs out, and the grid closes over it after,
  //                    then the clock goes on
  async "removed goes"(walk) {
    const { page, removeTime } = walk;
    // Its time starts once the hand leaves it, and the clock was held
    // before, so its ten seconds are the page's own, run on here.
    await page.mouse.move(5, 5);
    await runClock(page, (removeTime - 0.5) * 1000);
    if (!(await page.locator("li.tile .gone").count())) {
      walk.wrong = "a removed picture went before its time to put it back ran out";
      return;
    }
    await runClock(page, 1000);
    await settle(page);
    const went = await page.evaluate(() => !document.querySelector("li.tile .gone"));
    if (!went) walk.wrong = "a removed picture still stands in its place after its time";
    await letClockGo(page);
  },
  // ["microphone answered"] the Mac's question about the microphone is
  //                    answered, and the microphone that starts then is
  //                    stopped at once, since its recording has ended
  async "microphone answered"() {
    await control(url, "/microphone?answer=1");
    await control(url, "/hold?call=Listen&gate=open");
  },
  // ["microphone moves"]
  //                    while it listens, the microphone grows with the hum
  //                    on a spring, in its place: it goes a little past the
  //                    size of the voice and settles, falls back in the
  //                    pause a little under its rest and settles, never
  //                    jumps and never grows past 26 px, and is drawn white
  async "microphone moves"(walk) {
    const { page } = walk;
    // While it listens, the microphone moves with the voice it hears,
    // so a person knows they are heard. Watched here once rather than
    // in every dictate step, issue #543.
    const now = await page.locator("button.dictate").getAttribute("aria-label");
    if (now !== "Stop") {
      walk.wrong = `the microphone button says ${now}, so there is nothing listening to watch`;
      return;
    }
    // It hangs on a spring the loudness pulls, issue #739: Tim asked for
    // a bounce. The app's time is held, so it moves only as the walk runs
    // the clock, a frame of 16 ms at a time, and what it does is the same
    // on every machine. Every size it is given is kept, from the style
    // the app writes, in order: each change says the style before it, so
    // a run of frames that ends before the page reads them loses none.
    await holdClock(page);
    await page.evaluate(() => {
      const grows = document.querySelector("button.dictate .grows");
      const changes = [];
      const o = new MutationObserver((rs) => changes.push(...rs));
      o.observe(grows, { attributes: true, attributeFilter: ["style"], attributeOldValue: true });
      const size = (style) => 28 * Number(/scale\(([\d.]+)\)/.exec(style ?? "")?.[1] ?? 16 / 28);
      // The sizes it was given since the last time, and how it stands now.
      window.__mic = () => {
        const all = [...changes.splice(0), ...o.takeRecords()];
        const icon = grows.querySelector("svg");
        const r = icon.getBoundingClientRect();
        return {
          sizes: [...all.slice(1).map((c) => size(c.oldValue)), ...(all.length ? [size(grows.getAttribute("style"))] : [])],
          shown: { w: r.width, x: r.left + r.width / 2, y: r.top + r.height / 2, box: getComputedStyle(icon).width },
          loud: parseFloat(document.querySelector("button.dictate").dataset.loud),
        };
      };
    });
    // tenths hands the app tenths of a second of the hum until it is in
    // the pause, or out of it, and says how loud the microphone is then.
    const tenths = async (quiet) => {
      for (;;) {
        await control(url, "/microphone?until=tenth");
        await caughtUp(page, url);
        const loud = await page.evaluate(() => parseFloat(document.querySelector("button.dictate").dataset.loud));
        if ((loud === 0) === quiet) return loud;
      }
    };
    const run = async (ms) => {
      await page.clock.runFor(ms);
      await settle(page);
      return page.evaluate(() => window.__mic());
    };
    await tenths(true);
    const rest = await run(1500);
    const loud = await tenths(false);
    const rise = await run(1000);
    await tenths(true);
    const fall = await run(1500);
    await letPageTimeGo(page);
    const steps = (from, sizes) => Math.max(...sizes.map((w, k) => Math.abs(w - (k ? sizes[k - 1] : from))));
    const voice = 16 + 8 * loud;
    const grown = rise.sizes.at(-1);
    const widest = Math.max(...rise.sizes);
    const narrowest = Math.min(...fall.sizes);
    if (loud < 0.3 || !rise.sizes.length || !fall.sizes.length) {
      walk.wrong = `the walk saw the hum at a loudness of ${loud}, ${rise.sizes.length} sizes as it rose and ${fall.sizes.length} as it fell, so there was nothing to watch`;
      return;
    }
    if (Math.abs(rest.shown.w - 16) > 0.5 || Math.abs(rise.shown.w - grown) > 0.5) {
      walk.wrong = `the microphone is drawn ${rest.shown.w.toFixed(1)} px wide at rest and ${rise.shown.w.toFixed(1)} px for a voice it was given ${grown.toFixed(1)} px for, so the sizes the walk read are not the ones drawn`;
      return;
    }
    // It settles at the size of the voice: from 16 px at rest by 8 px for
    // a raised voice, with room in its button of 32 px to bounce. It went
    // up to 28 px before, and Tim wondered whether that was too large.
    if (Math.abs(grown - voice) > 0.1) {
      walk.wrong = `the microphone settled at ${grown.toFixed(2)} px for a voice at a loudness of ${loud}, not at ${voice.toFixed(2)} px`;
      return;
    }
    if (widest > 26) {
      walk.wrong = `the microphone grew to ${widest.toFixed(2)} px, larger than the 26 px it may reach as it bounces`;
      return;
    }
    // It bounces: past the size of the voice on the way up, and under its
    // rest on the way down, by a few hundredths of how far it went, and it
    // comes to rest there. Following like a meter's needle it never went
    // past, and Tim found it stiff.
    if (widest - grown < 0.04 * (grown - 16)) {
      walk.wrong = `the microphone rose to ${widest.toFixed(2)} px and settled at ${grown.toFixed(2)} px, so it did not bounce past the voice`;
      return;
    }
    if (narrowest > 16 - 0.04 * (grown - 16) || narrowest < 14) {
      walk.wrong = `the microphone fell back to ${narrowest.toFixed(2)} px at its least, not a little under its rest of 16 px`;
      return;
    }
    if (Math.abs(fall.sizes.at(-1) - 16) > 0.01) {
      walk.wrong = `the microphone came to rest at ${fall.sizes.at(-1).toFixed(2)} px after the voice, not at 16 px`;
      return;
    }
    // It never jumps: a frame moves it less than half the way to the size
    // of the voice, where going straight to a reading moved it all the
    // way at once.
    const jump = Math.max(steps(16, rise.sizes), steps(grown, fall.sizes));
    if (jump >= 0.5 * (grown - 16)) {
      walk.wrong = `the microphone moved ${jump.toFixed(2)} px in one frame, so it jumped rather than sprang`;
      return;
    }
    // Its box keeps one size and its middle keeps still: a size that
    // changed snapped to whole pixels and shook on Tim's Mac.
    if (rest.shown.box !== rise.shown.box) {
      walk.wrong = `the microphone changes the size of its box as it grows, ${rest.shown.box} and ${rise.shown.box}, which snaps to whole pixels and shakes`;
      return;
    }
    const drift = Math.hypot(rise.shown.x - rest.shown.x, rise.shown.y - rest.shown.y);
    if (drift > 0.5) {
      walk.wrong = `the middle of the microphone moved by ${drift.toFixed(2)} px as it grew`;
      return;
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
      walk.wrong = `the microphone is drawn ${look.colour} while it listens, not white like the paperclip, ${look.clip}`;
      return;
    }
    if (look.contrast < 3) {
      walk.wrong = `the microphone stands out from its button by only ${look.contrast.toFixed(2)} to 1 while it listens`;
      return;
    }
  },
  // ["dictating"]      the microphone still listens, its button says Stop
  async dictating(walk) {
    const { page } = walk;
    const now = await page.locator("button.dictate").getAttribute("aria-label");
    if (now !== "Stop") walk.wrong = `the microphone button says ${now}, so the dictation stopped`;
  },
  // ["report on", on]  Report and Clear can be pressed, or with false are
  //                    off, as they are with nothing to report
  async "report on"(walk, arg) {
    const { page } = walk;
    const off = await page.evaluate(() => [document.querySelector("button.report").disabled, document.querySelector("button.clear").disabled]);
    if (off[0] === arg) walk.wrong = `Report is ${arg ? "off" : "on"} with ${arg ? "something" : "nothing"} to report`;
    else if (off[1] === arg) walk.wrong = `Clear is ${arg ? "off" : "on"} with ${arg ? "something" : "nothing"} to report`;
  },
  // ["clear", how]     presses Clear, and the box over the page asks
  //                    first, with the keyboard on Cancel and saying
  //                    that everything goes and Cmd+Z undoes it. "cancel"
  //                    presses Escape, "clear" presses Clear in the box.
  //                    The box is gone after
  async clear(walk, arg) {
    const { page } = walk;
    await page.locator("button.clear").click();
    await page.locator("dialog[open]").waitFor();
    const box = await page.evaluate(() => ({
      title: document.querySelector("dialog[open] h2")?.textContent.trim(),
      says: document.querySelector("dialog[open] .body")?.textContent.trim(),
      focused: document.activeElement?.textContent.trim(),
    }));
    if (box.title !== "Clear the report?") walk.wrong = `Clear asked ${JSON.stringify(box.title)}`;
    else if (box.focused !== "Cancel") walk.wrong = `the box opened with the keyboard on ${box.focused}, not Cancel`;
    else if (box.says !== "Everything you wrote and attached will be deleted. You can undo it with Cmd+Z.") walk.wrong = `the box says ${JSON.stringify(box.says)}`;
    if (walk.wrong) return;
    if (arg === "cancel") await page.keyboard.press("Escape");
    else await page.locator("dialog[open] button.danger").click();
    await page.locator("dialog[open]").waitFor({ state: "detached" });
    if (arg === "clear") {
      // The pictures go at once, without the moment a removed one stays.
      const left = await page.evaluate(() => [document.querySelector(".telling").value.trim(), document.querySelectorAll("li.tile").length]);
      if (left[0] || left[1]) walk.wrong = `after Clear the field holds "${left[0]}" and ${left[1]} pictures`;
    }
  },
  // ["report", n]      presses Report, which closes the report window and
  //                    says nothing beside the buttons, and reads the zip
  //                    it made: what was written and n pictures, or with
  //                    [n, m] n pictures and m recordings, and the field
  //                    empty after
  // ["report made", n] the report was made without a press on Report here,
  //                    and is checked the way "report" checks it. With
  //                    [n, text] what it holds as written reads text,
  //                    where * is whatever the speech model heard
  async "report made"(walk, arg, s, what) {
    const { page } = walk;
    const [want, also] = what === "report" ? [arg] : [arg].flat();
    const written = await page.evaluate(() => document.querySelector(".telling").value.trim());
    if (what === "report") await page.locator("button.report").click();
    // Report closes the window once Finder has the zip, and says
    // nothing beside the buttons on the way. Tim had a line there.
    // Asked once the call that makes the report has come back.
    await settle(page);
    const closed = !(await (await fetch(new URL("/report-window", url))).json()).open;
    if (!closed) {
      walk.wrong = `Report left the window open, saying "${await page.evaluate(() => document.querySelector(".words")?.textContent.trim())}"`;
      return;
    }
    const said = await page.evaluate(() => document.querySelector(".words")?.textContent.trim());
    if (said) {
      walk.wrong = `Report said "${said}" beside the buttons`;
      return;
    }
    const names = await (await fetch(new URL("/report-made", url))).json();
    const [pics, films = 0] = [want].flat();
    const pictures = names.filter((n) => /\/pictures\/\d+\.png$/.test(n)).length;
    const recordings = names.filter((n) => /\/recordings\/\d+\.(mov|mp4|m4v|webm|mkv)$/.test(n)).length;
    if (pictures !== pics) walk.wrong = `the report holds ${pictures} pictures, not ${pics}: ${names.join(", ")}`;
    else if (recordings !== films) walk.wrong = `the report holds ${recordings} recordings, not ${films}: ${names.join(", ")}`;
    else if (written && !names.some((n) => n.endsWith("what-happened.txt"))) walk.wrong = "the report holds nothing of what was written";
    if (!walk.wrong && typeof also === "string") {
      const have = (await (await fetch(new URL("/report-made?file=what-happened.txt", url))).json()).trim();
      const like = new RegExp(
        `^${also
          .split("*")
          .map((p) => p.replace(/[.?+^$()[\]{}|\\]/g, "\\$&"))
          .join("\\S.*\\S")}$`,
      );
      if (!like.test(have)) walk.wrong = `the report holds ${JSON.stringify(have)} as written, not ${JSON.stringify(also)}`;
    }
    // The window closes while the call is still on its way back, and the
    // page empties the field once it is, so it is read once the app has
    // settled on the answer.
    await settle(page);
    const left = await page.evaluate(() => [document.querySelector(".telling").value.trim(), document.querySelectorAll("li.tile").length]);
    if (!walk.wrong && (left[0] || left[1])) walk.wrong = `after Report the field holds "${left[0]}" and ${left[1]} pictures`;
  },
  // ["report scroll", n]
  //                    turns the wheel over the report's field by n
  //                    pixels, down for more than 0
  async "report scroll"(walk, arg) {
    const { kept, page } = walk;
    const box = await page.locator(".telling").boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    // The field's own word on the scroll is heard from before the wheel
    // turns, so a scroll that ends at once is heard too: scroll when it
    // starts, scrollend when it has come to rest. In WebKit the step read
    // the field at 99 px, at its first scroll event, and it stood at 0 by
    // the next step (#380).
    const was = await page.locator(".telling").evaluate((f) => {
      f.wheeled = { scrolled: false, ended: new Promise((done) => f.addEventListener("scrollend", () => done(f.scrollTop), { once: true })) };
      f.addEventListener("scroll", () => (f.wheeled.scrolled = true), { once: true });
      return f.scrollTop;
    });
    await page.mouse.wheel(0, arg);
    // Where the wheel has left the field: where its scroll came to rest,
    // or where it was, two frames on, when the wheel turned nothing.
    kept.scrolledTo = await page.locator(".telling").evaluate(
      (f, was) =>
        new Promise((done) => {
          f.wheeled.ended.then(done);
          requestAnimationFrame(() => requestAnimationFrame(() => f.wheeled.scrolled || done(was)));
        }),
      was,
    );
    if (arg < 0 && kept.scrolledTo >= was) walk.wrong = `the wheel did not scroll the field up, it is at ${kept.scrolledTo} px`;
  },
  // ["report follows"] more is said, and the field follows it: the words
  //                    being said are in view, at its end when they end
  //                    the report
  // ["report follows, a line further"]
  //                    the same, with so much more said that the report
  //                    is a line longer
  // ["report stays"]   more is said, and the field stays where the last
  //                    "report scroll" put it
  async "report stays"(walk, arg, s, what) {
    const { kept, page } = walk;
    await watchRedraw(page);
    if (what === "report follows, a line further") {
      // A few words more may still fit on the last line, which the field
      // shows whether it follows or not. Thirty rounds of words make
      // more than a line on any width.
      const tall = await page.locator(".telling").evaluate((f) => f.scrollHeight);
      for (let i = 0; i < 30 && (await page.locator(".telling").evaluate((f) => f.scrollHeight)) <= tall; i++) {
        const was = await page.locator(".telling").evaluate((f) => f.value);
        await control(url, "/microphone?until=hearing");
        await pageUntil(page, (was) => document.querySelector(".telling").value !== was, was);
      }
      await settle(page);
      if ((await page.locator(".telling").evaluate((f) => f.scrollHeight)) <= tall) walk.wrong = "the report grew no longer while more was said";
      else walk.wrong = (await page.locator(".telling").evaluate(atEnd)) || (await redrawMissed(page));
      return;
    }
    const was = await page.locator(".telling").evaluate((f) => f.value);
    // The recording goes on until more words are heard, see /microphone.
    await control(url, "/microphone?until=hearing");
    await pageUntil(page, (was) => document.querySelector(".telling").value !== was, was);
    await settle(page);
    const top = await page.locator(".telling").evaluate((f) => f.scrollTop);
    if (what === "report stays") {
      if (top !== kept.scrolledTo) walk.wrong = `the field moved to ${top} px while more was said, from ${kept.scrolledTo} px where it was scrolled to`;
    } else walk.wrong = (await page.locator(".telling").evaluate(atEnd)) || (await redrawMissed(page));
  },
  // ["talk, the cursor in view", n]
  //                    more is said until the cursor that goes with the
  //                    words has gone n lines down, and after each word
  //                    that comes the cursor is in view. The field had to
  //                    scroll for it, or the step proves nothing. Words
  //                    said in the middle of a long report went out of
  //                    view under the foot of the field (third review of
  //                    #574)
  async "talk, the cursor in view"(walk, arg) {
    const { page } = walk;
    const at = () => page.locator(".telling").evaluate((f) => [window.__caretBox(f, f.selectionEnd)[0], f.scrollTop, parseFloat(getComputedStyle(f).lineHeight)]);
    const [from, top, line] = await at();
    // A round of the stand-in speech model's words is a few words, so a
    // line takes a few rounds, and thirty rounds a line is more than any
    // width needs.
    for (let i = 0; i < 30 * arg && !walk.wrong; i++) {
      if ((await at())[0] >= from + arg * line) break;
      const was = await page.locator(".telling").evaluate((f) => f.value);
      await control(url, "/microphone?until=hearing");
      await pageUntil(page, (was) => document.querySelector(".telling").value !== was, was);
      await settleButTheSave(page);
      const c = await page.locator(".telling").evaluate(cursorInView);
      if (c) walk.wrong = `after ${i + 1} rounds of words ${c}`;
    }
    if (walk.wrong) return;
    const [to, now] = await at();
    if (to < from + arg * line) walk.wrong = `the cursor went down ${to - from} px while more was said, not ${arg} lines`;
    else if (now <= top) walk.wrong = `the field stayed at ${now} px, so the cursor never came near its foot`;
  },
  // ["caret at words"] the cursor is right after the words being said,
  //                    at the end of the report, which they end
  async "caret at words"(walk) {
    const { page } = walk;
    const c = await page.locator(".telling").evaluate(cursorAtWords);
    if (c) walk.wrong = c;
  },
  // ["talk ahead", n]  the person dictating says n tenths of a second
  //                    more than the speech model has written down yet,
  //                    and the walk keeps how many words the field shows
  //                    after what was written before, for "said
  //                    before" (#573)
  async "talk ahead"(walk, arg) {
    const { kept, page } = walk;
    const handed = await control(url, `/microphone?ahead=${arg}`);
    if (handed !== arg) walk.wrong = `the microphone handed over ${handed} tenths of a second, not ${arg}`;
    // Everything after "It froze." is what was said.
    kept.shown = await page.locator(".telling").evaluate((f) => f.value.replace(/^It froze\.\s*/, "").split(/\s+/).filter(Boolean).length);
  },
  // ["said before", typed]
  //                    what the report holds between "It froze." and
  //                    typed, a new line or words typed at the end of what
  //                    was said, is more words than the field showed at
  //                    "talk ahead", since everything said before the
  //                    typing goes in front of it, and what follows it
  //                    holds what was said after (#573, #584)
  async "said before"(walk, arg) {
    const { kept, page } = walk;
    const text = await page.locator(".telling").evaluate((f) => f.value);
    const at = text.indexOf(arg);
    const words = (t) => t.split(/\s+/).filter(Boolean).length;
    const before = words(text.slice(0, Math.max(at, 0)).replace(/^It froze\.\s*/, ""));
    const name = arg === "\n" ? "the new line" : `"${arg}"`;
    if (at < 0) walk.wrong = `the report holds no ${name}: ${JSON.stringify(text)}`;
    else if (arg !== "\n" && !/\s/.test(text[at - 1])) walk.wrong = `no space in front of ${name}: ${JSON.stringify(text)}`;
    else if (before <= kept.shown) walk.wrong = `${before} words said are in front of ${name}, no more than the ${kept.shown} shown when it was typed, so what was said before it went after it`;
    else if (!text.slice(at + arg.length).trim()) walk.wrong = `nothing of what was said after ${name} follows it`;
  },
  // ["words kept"]     the report holds at least the words the field
  //                    showed at "talk ahead", after "It froze.", so no
  //                    key took away words that were still to be written
  //                    down (#584)
  async "words kept"(walk) {
    const { kept, page } = walk;
    const text = await page.locator(".telling").evaluate((f) => f.value);
    const words = text.replace(/^It froze\.\s*/, "").split(/\s+/).filter(Boolean).length;
    if (!kept.shown) walk.wrong = "the field showed no words at talk ahead, so there was nothing to keep";
    else if (words < kept.shown) walk.wrong = `${words} words are in the report, fewer than the ${kept.shown} shown: ${JSON.stringify(text)}`;
  },
  // ["press at the end", key, n]
  //                    presses key n times with the cursor right after the
  //                    words being said, with nothing heard let in
  //                    between, and after each press the field has not
  //                    moved and the cursor is still there. Tim pressed
  //                    ArrowDown there and the field bounced (29 October)
  async "press at the end"(walk, arg, s) {
    const { page, watch } = walk;
    const [key, n] = arg;
    await page.evaluate(() => window.__pauseEvents(true));
    const was = await page.locator(".telling").evaluate((f) => f.scrollTop);
    for (let i = 1; i <= n && !walk.wrong; i++) {
      const by = await pressed(page, key);
      const c = [await page.locator(".telling").evaluate((f) => f.scrollTop), await page.locator(".telling").evaluate(cursorAtWords)];
      if (by) walk.wrong = `after ${key} ${i} of ${n} times ${by}`;
      else if (c[0] !== was) walk.wrong = `after ${key} ${i} of ${n} times the field moved from ${was} px to ${c[0]} px`;
      else if (c[1]) walk.wrong = `after ${key} ${i} of ${n} times ${c[1]}`;
    }
    await page.evaluate(() => window.__pauseEvents(false));
    await watch.step("press", s, page);
  },
  // ["press, a line at a time", key, n]
  //                    presses key n times, with nothing heard let in
  //                    between, and after each press the report's field
  //                    stayed or scrolled by a line at most, the page
  //                    scrolled it and not the browser, and the cursor is
  //                    in view. On Tim's Mac the field jumped several
  //                    lines at a time and went past its last line and
  //                    back (29 and 30 October)
  async "press, a line at a time"(walk, arg, s) {
    const { page, watch } = walk;
    const [key, n] = arg;
    await page.evaluate(() => window.__pauseEvents(true));
    const line = await page.locator(".telling").evaluate((f) => parseFloat(getComputedStyle(f).lineHeight));
    for (let i = 1; i <= n && !walk.wrong; i++) {
      const was = await page.locator(".telling").evaluate((f) => f.scrollTop);
      const by = await pressed(page, key);
      const now = await page.locator(".telling").evaluate((f) => f.scrollTop);
      const c = await page.locator(".telling").evaluate(cursorInView);
      if (by) walk.wrong = `after ${key} ${i} of ${n} times ${by}`;
      else if (c) walk.wrong = `after ${key} ${i} of ${n} times ${c}`;
      else if (Math.abs(now - was) > line) walk.wrong = `after ${key} ${i} of ${n} times the field went from ${was} px to ${now} px, more than a line of ${line} px`;
    }
    await page.evaluate(() => window.__pauseEvents(false));
    await watch.step("press", s, page);
  },
  // ["field kept"]     notes where the report's field is scrolled to, for
  //                    "report stays"
  async "field kept"(walk) {
    walk.kept.scrolledTo = await walk.page.locator(".telling").evaluate((f) => f.scrollTop);
  },
  // ["watch the focus"]
  //                    counts each time the focus leaves the report's
  //                    field from here, for "focus stayed"
  async "watch the focus"(walk) {
    await walk.page.locator(".telling").evaluate((f) => {
      window.__left = 0;
      f.addEventListener("focusout", () => window.__left++);
    });
  },
  // ["focus stayed"]   the focus never left the report's field since
  //                    "watch the focus", so its ring did not go out and
  //                    come back. Tim saw it flash at each press of the
  //                    microphone (30 October)
  async "focus stayed"(walk) {
    const left = await walk.page.evaluate(() => window.__left);
    if (left) walk.wrong = `the focus left the report's field ${left} times`;
  },
  // ["cursor in view"] the cursor is in view in the report's field
  async "cursor in view"(walk) {
    const c = await walk.page.locator(".telling").evaluate(cursorInView);
    if (c) walk.wrong = c;
  },
  // ["press, the cursor in view", key, n]
  //                    presses key n times, with nothing heard by
  //                    dictation let in between, and after each press
  //                    the cursor is in view in the report's field, the
  //                    way a text field keeps it, and the page scrolled
  //                    it there and not the browser (#573)
  async "press, the cursor in view"(walk, arg, s) {
    const { page, watch } = walk;
    const [key, n] = arg;
    // Nothing heard comes in between, however busy the machine, so the
    // field following the words cannot bring the cursor back by chance.
    await page.evaluate(() => window.__pauseEvents(true));
    for (let i = 1; i <= n && !walk.wrong; i++) {
      const by = await pressed(page, key);
      const c = await page.locator(".telling").evaluate(cursorInView);
      if (by) walk.wrong = `after ${key} ${i} of ${n} times ${by}`;
      else if (c) walk.wrong = `after ${key} ${i} of ${n} times ${c}`;
    }
    await page.evaluate(() => window.__pauseEvents(false));
    await watch.step("press", s, page);
  },
  // ["caret elsewhere"]
  //                    clicks into the report's first line, and the field
  //                    shows its own cursor there again
  async "caret elsewhere"(walk) {
    const { page } = walk;
    const box = await page.locator(".telling").boundingBox();
    await page.locator(".telling").evaluate((f) => (f.scrollTop = 0));
    await page.mouse.click(box.x + 20, box.y + 12);
    await settle(page);
    const c = await page.locator(".telling").evaluate((f) => getComputedStyle(f).caretColor);
    if (c === "rgba(0, 0, 0, 0)") walk.wrong = "the field hides its cursor after a click into its first line";
  },
  // ["buttons fit", words]
  //                    opens Advanced where there is one, and every button
  //                    on screen that holds words alone is as wide as the
  //                    words it shows and its padding, to a pixel. A
  //                    button made to fill its row, like a caption
  //                    setting's name, is not about its words. Buttons
  //                    saying each of words are among them (#362)
  async "buttons fit"(walk, arg) {
    const { page } = walk;
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
    if (problems.length) walk.wrong = problems.join("\n");
  },
  // ["report lines up"]
  //                    the microphone, the paperclip and Report are one
  //                    height on a whole pixel, the two are square, the
  //                    tiles are one size
  //                    on whole pixels with a space between them, each
  //                    with the sidebar's bin in its corner
  async "report lines up"(walk) {
    const { page } = walk;
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
    if (problems.length) walk.wrong = problems.join("\n");
  },
  // ["write", text, n] types text into the report's field and puts the
  //                    cursor n characters in, or at its end for "end"
  async write(walk, arg, s) {
    const { page, watch } = walk;
    const [text, at] = arg;
    await page.locator(".telling").click();
    await page.keyboard.type(text);
    await page.evaluate((n) => {
      const field = document.querySelector(".telling");
      const at = n === "end" ? field.value.length : n;
      field.setSelectionRange(at, at);
    }, at);
    await watch.step("type", s);
  },
  // ["dictate", how]   presses Dictate, waits for the words to appear grey
  //                    while they are heard, and ends the recording how
  //                    says: "button" presses Stop, "enter" presses Enter
  //                    and "escape" Escape, "escape, then enter" presses
  //                    Escape and Enter right after it, while what was
  //                    said is still being written down, "enter, then
  //                    tab and enter" presses Enter, Tab and Enter, the
  //                    two after it while it is written down, "enter, then
  //                    tab" the same without the Enter, and sees the focus
  //                    still on Report once the words are written, "escape, the
  //                    cursor stays" sees the cursor still drawn after the
  //                    words while they are written down, and "ends"
  //                    presses nothing
  //                    until it stops by itself and says why. Then waits
  //                    for Dictate again.
  //                    It hears two notes going up once the microphone is
  //                    open, and two going down when it stops, however
  //                    it stops.
  //                    "start" only starts it, "start, answered late" the
  //                    same with the answer to Listen held until the
  //                    start sound is checked, the way a busy machine
  //                    hands it back after the microphone's first news,
  //                    "start, sounded late" the same with the window
  //                    starting the start sound half a second late,
  //                    "hearing" waits for more
  //                    to be said and settled, "settled" waits for words
  //                    before a pause to be settled, and a recording
  //                    already running is only ended. "asked" presses
  //                    Dictate while the Mac asks whether the app may use
  //                    the microphone, and Stop while it still asks, and
  //                    sees Dictate again at once, with the question left
  //                    open for "microphone answered". "asked, then
  //                    again" then presses Dictate once more while the
  //                    Mac still asks, answers the question, and hears the
  //                    start sound once, for the second press only, with
  //                    the recording left running
  async dictate(walk, arg, s) {
    const { kept, page, watch } = walk;
    const button = page.locator("button.dictate");
    if (arg === "asked" || arg === "asked, then again") {
      // Tim pressed Dictate, the Mac asked about the microphone, and
      // nothing he pressed did anything until the question went 45 s
      // later, #792. Stop ends a recording whose microphone has not
      // started, and Report then makes the report. Listen waits on the
      // Go side for the microphone, so the page holds its answer, and
      // the walk settles on everything else while it waits, see hold.
      await control(url, "/microphone?ask=1");
      await hold(page, "Listen", true);
      await button.click();
      const said = await button.getAttribute("aria-label");
      if (said !== "Stop") {
        walk.wrong = `Dictate says "${said}" the frame after it was pressed, not Stop`;
        return;
      }
      await button.click();
      await pageUntil(page, () => document.querySelector("button.dictate")?.getAttribute("aria-label") === "Dictate");
      if (arg === "asked") return;
      // Dictate again while the Mac still asks: two Listens wait for the
      // one answer. The first one's answer is held until the second's is
      // in, the order that left the page on the recording that had ended,
      // passing over every word of the new one. Only the latest press
      // takes its answer.
      await settle(page);
      const before = await page.evaluate(() => window.__notes.length);
      const ended = await page.evaluate(() => window.__last.dictation?.recording ?? 0);
      await button.click();
      const again = await button.getAttribute("aria-label");
      if (again !== "Stop") {
        walk.wrong = `Dictate pressed again says "${again}" the frame after, not Stop`;
        return;
      }
      await control(url, "/microphone?answer=1");
      await settle(page);
      await letHeld(page);
      await settle(page);
      const up = await sounds(page, before, LOW, HIGH);
      if (!theTwo(up, LOW, HIGH)) {
        walk.wrong = `the second press of Dictate sounded ${JSON.stringify(up)}, not the two notes of the start sound going up once`;
        return;
      }
      await control(url, "/microphone?until=hearing");
      await pageUntil(page, (ended) => window.__last.dictation?.recording > ended && window.__last.dictation.hearing?.trim(), ended);
      await settle(page);
      const shown = await page.evaluate(() => document.querySelector(".telling").value.includes(window.__last.dictation.hearing.trim()));
      if (!shown) walk.wrong = "the words heard after the second press of Dictate were passed over, as if they were the recording's that had ended";
      return;
    }
    if (arg === "hearing") {
      // More is said after what was there, and settled. The stand-in
      // speech model hears new words each time it is asked, so the
      // words shown as being heard when the recording stops can be
      // written down as none, and the walk of Shift+Enter then found
      // nothing said after the new line on a busy machine, issue #424.
      // What is settled is kept however the recording ends. The
      // recording goes on until the speech model has settled more of
      // what it heard, see /microphone, and the field shows it.
      // What the speech model has settled, as it told the window, and
      // that the field shows the last of it.
      const settled = () => {
        const said = window.__last.dictation?.said ?? [];
        return said.length && document.querySelector(".telling").value.includes(said.at(-1)) ? said.join(" ") : "";
      };
      const was = await page.evaluate(settled);
      await control(url, "/microphone?until=said");
      await pageUntil(
        page,
        (was) => {
          const said = window.__last.dictation?.said ?? [];
          const shown = said.length && document.querySelector(".telling").value.includes(said.at(-1)) ? said.join(" ") : "";
          return shown !== was || document.querySelector("button.dictate").getAttribute("aria-label") === "Dictate";
        },
        was,
      );
      if ((await page.evaluate(settled)) === was) walk.wrong = "the recording stopped before anything more was said and settled";
      return;
    }
    if (arg === "settled") {
      // The field's text grows by what was said before the pause.
      await control(url, "/microphone?until=said");
      await pageUntil(page, (was) => document.querySelector(".telling").value !== was, kept.dictating);
      return;
    }
    const running = (await button.getAttribute("aria-label")) === "Stop";
    // The recording goes on in steps that take a moment all told, and
    // what each tells the window is sent once. A report window just
    // opened may not be listening yet, and missed every reading of how
    // loud the microphone was, so the walk waits until it is.
    if (!running) await caughtUp(page, url);
    // The recording starts and tells the window what it hears. "start,
    // answered late" holds the answer that it started, see /hold, until
    // the window has told what it heard.
    const late = arg === "start, answered late";
    // The window starts the start sound later than it means to, by more
    // than the sound is long, counted on the audio's clock once it runs.
    const slow = arg === "start, sounded late";
    if (late) await control(url, "/hold?call=Listen&gate=shut&answer=1&times=1");
    const before = await page.evaluate(() => window.__notes.length);
    if (!running) await button.click();
    // Stop the moment it is pressed, see the rules on a click.
    const said = await button.getAttribute("aria-label");
    if (!running && said !== "Stop") {
      walk.wrong = `Dictate says "${said}" the frame after it was pressed, not Stop`;
      return;
    }
    if (slow) await page.evaluate(() => window.__running.then(() => (window.__late = 0.5)));
    // The words appear while they are heard, before anything stops. The
    // recording goes on until the app has told words being heard, see
    // /microphone.
    await control(url, "/microphone?until=hearing");
    await pageUntil(page, () => {
      const hearing = window.__last.dictation?.hearing?.trim();
      return hearing && document.querySelector(".telling").value.includes(hearing);
    });
    if (!running) {
      // The microphone moves with the voice it hears, so a person knows
      // they are heard. The bridge's microphone plays a tone, and a new
      // recording starts with two seconds of it. One already running may
      // stand in its pause, and stays there between steps.
      await pageUntil(page, () => parseFloat(document.querySelector("button.dictate")?.dataset.loud) > 0.3);
    }
    // From the press on, nothing sounds until the microphone says it is
    // on, not a sample, and then exactly the two notes going up, #654. A
    // busy machine hands that answer back after the microphone's first
    // news, so the walk waits until the window has started the two
    // notes, however late, and reads only what was started since the
    // press. The walk's own timeout ends it if they never come.
    if (late) await control(url, "/hold?call=Listen&gate=open");
    if (!running) {
      const up = await sounds(page, before, LOW, HIGH);
      const quiet = await page.evaluate(() => window.__before);
      if (quiet > 0.001 || up.some((n) => !n.open)) {
        walk.wrong = `the window played sound before the microphone said it was on, as loud as ${quiet.toFixed(3)}, and it sounded ${JSON.stringify(up)}`;
        return;
      }
      if (!theTwo(up, LOW, HIGH)) {
        walk.wrong = `the microphone opened without the two notes of the start sound going up, it sounded ${JSON.stringify(up)}`;
        return;
      }
    }
    kept.notes = await page.evaluate(() => window.__notes.length);
    await watch.step("hearing", s);
    if (slow) await page.evaluate(() => (window.__late = 0));
    if (arg === "start" || late || slow) {
      kept.dictating = await page.locator(".telling").evaluate((f) => f.value);
      return;
    }
    // Something said is settled before the recording is ended, so what
    // it ends with is more than the words last heard. The stand-in speech
    // model hears new words each time it is asked, and words that fit
    // what was heard so far can be followed by a word that does not, so
    // the end could write down nothing, issue #424.
    if (arg !== "ends") await control(url, "/microphone?until=settled");
    if (arg === "button") await button.click();
    else if (arg === "ends") {
      // Nobody presses anything, and nobody talks any more: it stops
      // by itself and says why.
      await control(url, "/microphone?silent=1");
      await control(url, "/microphone?until=ended");
      await pageUntil(page, () => /without a word/.test(document.querySelector(".words")?.textContent ?? ""));
    } else {
      await page.locator(".telling").focus();
      // The last words are held back until the walk has looked, so
      // Tab, or the look at the cursor, comes while they are written
      // down on any machine.
      const tab = arg === "enter, then tab and enter" || arg === "enter, then tab";
      const late = tab || arg === "escape, the cursor stays";
      if (late) await control(url, "/hold?call=StopListening&gate=shut");
      await page.keyboard.press(arg === "enter" ? "Enter" : "Escape");
      if (arg === "escape, then enter") await page.keyboard.press("Enter");
      if (tab) {
        await page.keyboard.press("Tab");
        if (arg === "enter, then tab and enter") await page.keyboard.press("Enter");
        const writing = await button.getAttribute("aria-label");
        if (writing !== "Writing") walk.wrong = `the button says ${writing} after Tab, not Writing, so Tab came after the words were written`;
      }
      // While the last words are written down, the cursor is still
      // drawn after them, where they end.
      if (arg === "escape, the cursor stays") {
        const c = { writing: await page.locator("button.dictate").getAttribute("aria-label"), at: await page.locator(".telling").evaluate(cursorAtWords) };
        if (c.writing !== "Writing") walk.wrong = `the button says ${c.writing} right after Escape, not Writing, so there was nothing to look at`;
        else if (c.at) walk.wrong = `while the words were written down ${c.at}`;
      }
      if (late) await control(url, "/hold?call=StopListening&gate=open");
    }
    await letHeld(page);
    await pageUntil(page, () => document.querySelector("button.dictate")?.getAttribute("aria-label") === "Dictate");
    // Tab put the focus on Report while the last words were written
    // down, and it is still there, ringed, once they are.
    if (arg === "enter, then tab") {
      const on = await page.evaluate(() => ({
        name: document.activeElement?.classList.contains("report") ? "Report" : document.activeElement?.classList.contains("telling") ? "the field" : (document.activeElement?.tagName ?? "nothing"),
        ring: !!document.activeElement?.matches(":focus-visible") && window.__focusAsked?.on === document.activeElement && window.__focusAsked.ring,
      }));
      if (on.name !== "Report") walk.wrong ||= `once the last words were written the focus was on ${on.name}, not on Report where Tab put it`;
      else if (!on.ring) walk.wrong ||= "once the last words were written Report had the focus with no ring asked for";
    }
    // The two notes going down say it stopped, however it stopped. They
    // start a moment after Dictate shows again, so the walk waits for
    // them.
    const down = await sounds(page, kept.notes, HIGH, LOW);
    if (!theTwo(down, HIGH, LOW)) walk.wrong = `it stopped with ${JSON.stringify(down)} sounded, not the two notes of the stop sound going down`;
    await watch.step("dictate", s);
  },
  // ["report reads", text]
  //                    the report's field reads text, where * is
  //                    whatever the speech model heard, a word or more
  async "report reads"(walk, arg) {
    const { kept, page } = walk;
    const have = await page.locator(".telling").evaluate((f) => f.value);
    const want = new RegExp(
      `^${arg
        .split("*")
        .map((p) => p.replace(/[.?+^$()[\]{}|\\]/g, "\\$&"))
        .join("\\S.*\\S")}$`,
    );
    if (!want.test(have)) walk.wrong = `the report reads ${JSON.stringify(have)}, not ${JSON.stringify(arg)}`;
    kept.report = have;
  },
  // ["report as before"]
  //                    the report's field reads what it did at the last
  //                    "report reads", or at the last "report grew" with
  //                    "grown"
  async "report as before"(walk, arg) {
    const { kept, page } = walk;
    const have = await page.locator(".telling").evaluate((f) => f.value);
    const was = arg === "grown" ? kept.grown : kept.report;
    if (have !== was) walk.wrong = `the report reads ${JSON.stringify(have)}, not ${JSON.stringify(was)} as before`;
  },
  // ["report grew"]    the report's field holds more than it did at the
  //                    last "report reads", all of that and more words
  async "report grew"(walk) {
    const { kept, page } = walk;
    const have = await page.locator(".telling").evaluate((f) => f.value);
    // Every word there was, in order, and words between them.
    const words = kept.report.split(/\s+/);
    let at = 0;
    for (const w of have.split(/\s+/)) if (w === words[at]) at++;
    if (at < words.length || have.split(/\s+/).length <= words.length)
      walk.wrong = `the report reads ${JSON.stringify(have)}, not more than ${JSON.stringify(kept.report)}`;
    kept.grown = have;
  },
  async "failure toast"(walk) {
    const { page } = walk;
    // Issue #311: Tim read "Anthropic is busy and did not a…" in the
    // row, cut where it said what to do. Why is said in a toast,
    // whole, with Report, and the row says only that it failed. The Go
    // side tells the failure on an event of its own, after the job's
    // news, see notice.go. Once the interface has heard all the Go side
    // said, the toast is there.
    await caughtUp(page, url);
    const said = await page.evaluate(() => {
      const t = document.querySelector(".toast");
      const li = document.querySelector("li.next.stopped");
      return {
        text: t?.querySelector(".text")?.textContent.trim() ?? "",
        report: !!t?.querySelector("button.report"),
        row: li?.querySelector(".meta")?.textContent.trim() ?? "",
      };
    });
    if (!said.text.includes("local model")) walk.wrong = `the toast says "${said.text}", not why the search failed`;
    else if (!said.report) walk.wrong = "the toast offers no Report";
    else if (said.row.includes("local model")) walk.wrong = `the row carries the reason as well: "${said.row}"`;
    if (walk.wrong) return;
    const before = (await control(url, "/report-window")).made;
    await page.locator(".toast button.report").click();
    await settle(page);
    const draft = await ask(page, "ReportDraft");
    const after = (await control(url, "/report-window")).made;
    if (!draft.text?.includes(said.text)) walk.wrong = `Report wrote "${draft.text}" in the report, not the toast's words`;
    else if (after !== before + 1) walk.wrong = `Report opened ${after - before} report windows`;
    else if (await toastSays(page)) walk.wrong = "the toast stayed after Report";
    await control(url, "/report-window?close=1");
  },
};

// Steps that take the same body as the step they are walked by.
steps["report"] = steps["report made"];
steps["report follows"] = steps["report stays"];
steps["report follows, a line further"] = steps["report stays"];
steps["attach, as on a Mac"] = steps.attach;

// cursorAtWords runs in the page on the report's field f, and says what
// is wrong when the cursor is not right after the words being said, at
// the end of the report, or nothing when it is.
function cursorAtWords(f) {
  const hearing = window.__last.dictation?.hearing?.trim() ?? "";
  if (document.activeElement !== f) return "the cursor is not in the field";
  if (f.selectionStart !== f.selectionEnd) return "the selection is not a cursor";
  if (f.selectionEnd !== f.value.length) return `the cursor is ${f.selectionEnd} letters in, not after the words being said, ${f.value.length}`;
  if (hearing && !f.value.trimEnd().endsWith(hearing)) return `the report ends ${JSON.stringify(f.value.slice(-40))}, not with the words being heard, ${JSON.stringify(hearing)}`;
  return "";
}

// atEnd runs in the page on the report's field f, and says where it is
// scrolled to when it is not at its end, the padding under its last line
// included, or nothing when it is. A pixel short is the end on a screen
// whose pixels are not whole ones.
function atEnd(f) {
  const end = f.scrollHeight - f.clientHeight;
  if (Math.abs(f.scrollTop - end) > 1) return `the report is scrolled to ${f.scrollTop} px, not to its end at ${end} px with the padding under its last line`;
  return "";
}

// pressed presses key in page and says what is wrong when the browser
// scrolled the report's field itself after the page had put the cursor's
// line in view, or nothing when it did not. The page scrolls the field
// as the key comes and after the edit it makes, and the browser then
// finds the cursor in view and has nothing to do. Where it does scroll,
// WebKit on a Mac puts the cursor in the middle of the field, which is
// the jump of several lines Tim saw, and Chromium at its very edge.
async function pressed(page, key) {
  await page.evaluate(() => {
    if (window.__pageScrolled !== undefined) return;
    const f = document.querySelector(".telling");
    const note = () => (window.__pageScrolled = f.scrollTop);
    note();
    window.addEventListener("keydown", note);
    window.addEventListener("input", note);
  });
  await watchRedraw(page);
  await page.keyboard.press(key);
  await nextPaint(page);
  const [left, now] = await page.locator(".telling").evaluate((f) => [window.__pageScrolled, f.scrollTop]);
  if (left !== now) return `the browser scrolled the field from ${left} px, where the page left it, to ${now} px`;
  return await redrawMissed(page);
}

// watchRedraw notes, from here, each frame the report's field scrolled in
// without the page having WebKit draw it again, for redrawMissed. WebKit
// draws the cursor again only where its place on the screen changed, so
// a cursor that moves a line while the field scrolls that line stays
// drawn on the line it left. Tim saw such cursors on his Mac, which a
// walk in Chromium cannot see, so the walk checks the page asked: the
// field's caret-color changes in every frame the field scrolls in.
async function watchRedraw(page) {
  await page.locator(".telling").evaluate((f) => {
    window.__redraw ??= (() => {
      const w = { asked: false, missed: [] };
      new MutationObserver(() => (w.asked = true)).observe(f, { attributes: true, attributeFilter: ["style"] });
      f.addEventListener("scroll", () => {
        if (w.on && !w.asked) w.missed.push(f.scrollTop);
        w.asked = false;
      });
      return w;
    })();
    Object.assign(window.__redraw, { on: true, asked: false, missed: [] });
  });
}

// redrawMissed says in which frames since watchRedraw the report's field
// scrolled with its cursor left as it was drawn, or nothing.
async function redrawMissed(page) {
  const missed = await page.evaluate(() => {
    window.__redraw.on = false;
    return window.__redraw.missed;
  });
  return missed.length ? `the page scrolled the field to ${missed.join(", ")} px and left the cursor drawn where it was, so on a Mac it stays on the line it left` : "";
}

// cursorInView runs in the page on the report's field f, and says where
// the cursor is when it is not in view, or nothing when it is. A
// textarea does not say where its cursor is, so it is measured the one
// way the page measures it, caretBox in lib/caret.ts, which the page
// scrolls by too. The check that the browser scrolled nothing after the
// page, in pressed, does not rest on it.
// Waits as settle does, but for the timers the app has set: the window
// keeps the report's text 300 ms after its last change, see keepText in
// Report.svelte, which settle waits for, and which moves nothing on
// screen. A step that looks at the field after each round of words would
// wait the 300 ms each round, 11 s of the 13 s of a walk that talks two
// lines into a long report. What is kept is still waited for by the next
// settle, the way a step that reads it back does.
function settleButTheSave(page) {
  return page.evaluate(() => window.__settled(true));
}

function cursorInView(f) {
  if (document.activeElement !== f) return "the cursor is not in the field";
  if (f.selectionStart !== f.selectionEnd) return "the selection is not a cursor";
  const [top, bottom] = window.__caretBox(f, f.selectionEnd);
  if (top < f.scrollTop - 1) return `the cursor is ${Math.round(f.scrollTop - top)} px above the top of the field, scrolled to ${f.scrollTop} px`;
  if (bottom > f.scrollTop + f.clientHeight + 1) return `the cursor is ${Math.round(bottom - f.scrollTop - f.clientHeight)} px below the foot of the field, scrolled to ${f.scrollTop} px`;
  return "";
}
