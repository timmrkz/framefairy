// The steps of the sequences about Report a Problem, the toasts and
// dictation. Each is said above it, a list with its name first.
// walkSequence in ../sequences.mjs walks it, and hands it the walk so far,
// the step's argument, what the caption box showed before it and the step's
// name. A sequence of any folder can take it. Below them the helpers only
// these steps use. See docs/TESTING.md.
import { ask, caughtUp, control, fromSidebar, letClockGo, letHeld, nextPaint, noDeadlines, pageUntil, runClock, seenUntil, settle, sidebarStill, slowDown } from "../bridge.mjs";
import { dropOn, standsAt, stillStands, toastSays, toastsSay, url } from "../common/steps.mjs";

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
      // Every note the window plays, with its pitch and whether the
      // microphone was open then, for the dictate step's sounds.
      await walk.reportPage.addInitScript(() => {
        window.__notes = [];
        const start = OscillatorNode.prototype.start;
        OscillatorNode.prototype.start = function (...a) {
          window.__notes.push({ f: this.frequency.value, open: document.querySelector("button.dictate")?.getAttribute("aria-label") === "Stop" });
          return start.apply(this, a);
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
  async attach(walk, arg) {
    const { page } = walk;
    const before = await page.locator("li.tile").count();
    walk.standing = await standsAt(page);
    for (let i = 0; i < arg; i++) await fetch(new URL("/pick-picture", url));
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
  // ["drop recording"] drops a screen recording of three seconds on the
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
  // ["report at end"]  the report's field is scrolled to its end
  async "report at end"(walk) {
    const { page } = walk;
    // The field shows the end of what is in it, down to its padding.
    const m = await page.evaluate(() => {
      const f = document.querySelector(".telling");
      return [f.scrollTop, f.scrollHeight - f.clientHeight];
    });
    if (m[1] <= 0) walk.wrong = "the report is not long enough to scroll";
    else if (Math.abs(m[0] - m[1]) > 1) walk.wrong = `the report is scrolled to ${m[0]} px, not to its end at ${m[1]} px`;
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
  // ["microphone moves"]
  //                    while it listens, the microphone grows with the hum
  //                    smoothly and in its place, falls back in its pause
  //                    like a level meter's needle, and is drawn white
  async "microphone moves"(walk) {
    const { page } = walk;
    // While it listens, the microphone moves with the voice it hears,
    // so a person knows they are heard. Watched here once rather than
    // in every dictate step, since it takes seconds of the hum, issue
    // #543.
    const now = await page.locator("button.dictate").getAttribute("aria-label");
    if (now !== "Stop") {
      walk.wrong = `the microphone button says ${now}, so there is nothing listening to watch`;
      return;
    }
    // It moves enough to be seen, smoothly, and in its place. From
    // the hum through its pause until the hum is heard again, every
    // frame, the size it is drawn at, its middle and the size of its
    // own box, so the walk sees it grow and fall back. A
    // microphone 16 px at rest grows by at least six tenths of that
    // for every step of loudness: by a third it grew 3 px for a voice
    // talking, and Tim could hardly see it. Its box keeps one size
    // and its middle keeps still: a size that changed snapped to
    // whole pixels and shook on Tim's Mac. It falls back no faster
    // than a level meter's needle, a third of a second for the whole
    // way: jumping to every reading shook too.
    // The recording goes on a tenth of a second a frame while the walk
    // watches, see /microphone, so the hum, its pause and the hum after
    // it each stand for frames on the page.
    let watched = false;
    const watching = page.evaluate(
      () =>
        new Promise((done) => {
          const b = document.querySelector("button.dictate");
          const xs = [];
          // Every size the app writes, at the time of the frame it
          // wrote it in, so each step is matched with the time it
          // went by. Read on the walk's own frames, a step showed one
          // frame late once the walk's frame came before the app's,
          // and was set against the gap after it: a step of 100 ms
          // after a long frame, seen across a short one, read six
          // times too fast on a busy machine, issue #354.
          const writes = [];
          const wrote = new MutationObserver(() => {
            const icon = b.querySelector("svg");
            writes.push({ t: document.timeline.currentTime, w: icon.getBoundingClientRect().width });
          });
          wrote.observe(b, { subtree: true, attributes: true, attributeFilter: ["style"] });
          // The pause is a loudness of nothing, and the hum after it
          // ends the watch, or the recording ending does.
          let paused = false;
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
            const loud = parseFloat(b.dataset.loud);
            paused ||= loud === 0;
            const over = (paused && loud > 0) || b.getAttribute("aria-label") !== "Stop";
            if (!over) requestAnimationFrame(step);
            else {
              wrote.disconnect();
              done({ xs, writes });
            }
          };
          requestAnimationFrame(step);
        }),
    );
    void watching.then(() => (watched = true));
    while (!watched) {
      await control(url, "/microphone?until=tenth");
      await nextPaint(page);
    }
    const { xs: track, writes } = await watching;
    const widest = Math.max(...track.map((f) => f.w));
    const loudest = Math.max(...track.map((f) => f.loud));
    const boxes = [...new Set(track.map((f) => f.box))];
    const drift = Math.max(...track.map((f) => Math.hypot(f.x - track[0].x, f.y - track[0].y)));
    // The sizes the app wrote are the sizes drawn, or the fall is
    // timed from nothing and passes whatever it does.
    const written = writes.map((f) => f.w);
    const narrowest = Math.min(...track.map((f) => f.w));
    if (writes.length < 2 || Math.abs(Math.max(...written) - widest) > 0.5 || Math.abs(Math.min(...written) - narrowest) > 0.5) {
      walk.wrong = `the walk saw ${writes.length} sizes written for a microphone drawn from ${narrowest.toFixed(1)} to ${widest.toFixed(1)} px, so it cannot time its fall`;
      return;
    }
    let fastest = 0;
    for (let k = 1; k < writes.length; k++) {
      const dt = writes[k].t - writes[k - 1].t;
      if (dt > 0) fastest = Math.max(fastest, (writes[k - 1].w - writes[k].w) / track[0].size / dt);
    }
    if (widest < 16 * (1 + 0.6 * loudest) - 0.5) {
      walk.wrong = `the microphone hardly grew with the voice, ${widest.toFixed(1)} px wide at most at a loudness of ${loudest}`;
      return;
    }
    if (boxes.length > 1) {
      walk.wrong = `the microphone changes the size of its box as it grows, ${boxes.join(", ")}, which snaps to whole pixels and shakes`;
      return;
    }
    if (drift > 0.5) {
      walk.wrong = `the middle of the microphone moved by ${drift.toFixed(2)} px as it grew`;
      return;
    }
    if (fastest > 0.003) {
      walk.wrong = `the microphone fell back by ${(fastest * 1000).toFixed(1)} of its size a second, faster than a level meter's needle`;
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
      const left = await page.evaluate(() => [document.querySelector(".telling").innerText.trim(), document.querySelectorAll("li.tile").length]);
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
    const written = await page.evaluate(() => document.querySelector(".telling").innerText.trim());
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
    const left = await page.evaluate(() => [document.querySelector(".telling").innerText.trim(), document.querySelectorAll("li.tile").length]);
    if (!walk.wrong && (left[0] || left[1])) walk.wrong = `after Report the field holds "${left[0]}" and ${left[1]} pictures`;
  },
  // ["report scroll", n]
  //                    turns the wheel over the report's field by n
  //                    pixels, down for more than 0
  async "report scroll"(walk, arg) {
    const { kept, page } = walk;
    const box = await page.locator(".telling").boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
    const was = await page.locator(".telling").evaluate((f) => f.scrollTop);
    await page.mouse.wheel(0, arg);
    // Read as soon as the wheel has turned the field, before the next
    // words come, or two frames on if it did not.
    kept.scrolledTo = await page.locator(".telling").evaluate(
      (f, was) =>
        new Promise((done) => {
          if (f.scrollTop !== was) return done(f.scrollTop);
          f.addEventListener("scroll", () => done(f.scrollTop), { once: true });
          requestAnimationFrame(() => requestAnimationFrame(() => done(f.scrollTop)));
        }),
      was,
    );
    if (arg < 0 && kept.scrolledTo >= was) walk.wrong = `the wheel did not scroll the field up, it is at ${kept.scrolledTo} px`;
  },
  // ["report follows"] more is said, and the field follows it: the words
  //                    being said are in view, at its end when they end
  //                    the report
  // ["report stays"]   more is said, and the field stays where the last
  //                    "report scroll" put it
  async "report stays"(walk, arg, s, what) {
    const { kept, page } = walk;
    const was = await page.locator(".telling").evaluate((f) => f.innerText);
    // The recording goes on until more words are heard, see /microphone.
    await control(url, "/microphone?until=hearing");
    await pageUntil(page, (was) => document.querySelector(".telling").innerText !== was, was);
    await settle(page);
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
      if (m.top !== kept.scrolledTo) walk.wrong = `the field moved to ${m.top} px while more was said, from ${kept.scrolledTo} px where it was scrolled to`;
    } else if (!m.seen) walk.wrong = "the words being said are out of view";
    else if (Math.abs(m.top - m.end) > 1) walk.wrong = `the field is at ${m.top} px, not at its end, ${m.end} px`;
  },
  // ["caret at words"] the cursor is drawn right after the words being
  //                    said, the field's own hidden
  async "caret at words"(walk) {
    const { page } = walk;
    const c = await page.locator(".telling").evaluate((f) => {
      const mark = f.querySelector(".hearing");
      return { ours: f.classList.contains("ours"), caret: getComputedStyle(f).caretColor, bar: mark ? getComputedStyle(mark).borderRightWidth : "" };
    });
    if (!c.ours || c.bar !== "1px" || c.caret !== "rgba(0, 0, 0, 0)") walk.wrong = `the cursor is not drawn after the words being said: ${JSON.stringify(c)}`;
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
    const c = await page.locator(".telling").evaluate((f) => ({ ours: f.classList.contains("ours"), caret: getComputedStyle(f).caretColor }));
    if (c.ours || c.caret === "rgba(0, 0, 0, 0)") walk.wrong = `the field hides its cursor after a click into its first line: ${JSON.stringify(c)}`;
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
      const at = document.createRange();
      if (n === "end") {
        at.selectNodeContents(field);
        at.collapse(false);
      } else at.setStart(field.firstChild, n);
      getSelection().removeAllRanges();
      getSelection().addRange(at);
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
  //                    "start" only starts it, "hearing" waits for more
  //                    to be said and settled, "settled" waits for words
  //                    before a pause to be settled, and a recording
  //                    already running is only ended
  async dictate(walk, arg, s) {
    const { kept, page, watch } = walk;
    const button = page.locator("button.dictate");
    if (arg === "hearing") {
      // More is said after what was there, and settled. The stand-in
      // speech model hears new words each time it is asked, so the
      // words shown as being heard when the recording stops can be
      // written down as none, and the walk of Shift+Enter then found
      // nothing said after the new line on a busy machine, issue #424.
      // What is settled is kept however the recording ends. The
      // recording goes on until the speech model has settled more of
      // what it heard, see /microphone, and the field shows it.
      const settled = () => {
        const copy = document.querySelector(".telling").cloneNode(true);
        copy.querySelector(".hearing")?.remove();
        return copy.textContent;
      };
      const was = await page.evaluate(settled);
      await control(url, "/microphone?until=said");
      await pageUntil(
        page,
        (was) => {
          const copy = document.querySelector(".telling").cloneNode(true);
          copy.querySelector(".hearing")?.remove();
          return copy.textContent !== was || document.querySelector("button.dictate").getAttribute("aria-label") === "Dictate";
        },
        was,
      );
      if ((await page.evaluate(settled)) === was) walk.wrong = "the recording stopped before anything more was said and settled";
      return;
    }
    if (arg === "settled") {
      // The field's text grows by what was said before the pause.
      await control(url, "/microphone?until=said");
      await pageUntil(page, (was) => document.querySelector(".telling").innerText !== was, kept.dictating);
      return;
    }
    const running = (await button.getAttribute("aria-label")) === "Stop";
    // The recording goes on in steps that take a moment all told, and
    // what each tells the window is sent once. A report window just
    // opened may not be listening yet, and missed every reading of how
    // loud the microphone was, so the walk waits until it is.
    if (!running) await caughtUp(page, url);
    if (!running) await button.click();
    // Stop the moment it is pressed, see the rules on a click.
    const said = await button.getAttribute("aria-label");
    if (!running && said !== "Stop") {
      walk.wrong = `Dictate says "${said}" the frame after it was pressed, not Stop`;
      return;
    }
    // The words appear while they are heard, before anything stops. The
    // recording goes on until the app has told words being heard, see
    // /microphone.
    await control(url, "/microphone?until=hearing");
    await pageUntil(page, () => document.querySelector(".telling .hearing")?.textContent.trim());
    if (!running) {
      // The microphone moves with the voice it hears, so a person knows
      // they are heard. The bridge's microphone plays a tone, and a new
      // recording starts with two seconds of it. One already running may
      // stand in its pause, and stays there between steps.
      await pageUntil(page, () => parseFloat(document.querySelector("button.dictate")?.dataset.loud) > 0.3);
    }
    // Two notes going up say the microphone is open, while it is.
    const up = await page.evaluate(() => window.__notes.slice(-2));
    if (up.length < 2 || !up.every((n) => n.open) || up[0].f >= up[1].f) {
      walk.wrong = `the microphone opened without the start sound, it played ${JSON.stringify(up)}`;
      return;
    }
    kept.notes = await page.evaluate(() => window.__notes.length);
    await watch.step("hearing", s);
    if (arg === "start") {
      kept.dictating = await page.locator(".telling").evaluate((f) => f.innerText);
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
        const c = await page.locator(".telling").evaluate((f) => ({ writing: document.querySelector("button.dictate")?.getAttribute("aria-label"), ours: f.classList.contains("ours") }));
        if (c.writing !== "Writing") walk.wrong = `the button says ${c.writing} right after Escape, not Writing, so there was nothing to look at`;
        else if (!c.ours) walk.wrong = "the cursor left the end of the words while they were written down";
      }
      if (late) await control(url, "/hold?call=StopListening&gate=open");
    }
    await letHeld(page);
    await pageUntil(page, () => document.querySelector("button.dictate")?.getAttribute("aria-label") === "Dictate");
    if (await page.locator(".telling .dictated").count()) walk.wrong = "what was being said is still grey in the report";
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
    // Two notes going down say it stopped, however it stopped, and
    // only those two.
    const down = await page.evaluate((from) => window.__notes.slice(from), kept.notes);
    if (down.length !== 2 || down[0].f <= down[1].f) walk.wrong = `it stopped with ${JSON.stringify(down)} played, not the two notes of the stop sound`;
    await watch.step("dictate", s);
  },
  // ["report is text"] the report's field holds text alone, its new lines
  //                    too, and no block or line break of the browser's
  async "report is text"(walk) {
    const { page } = walk;
    const inside = await page.locator(".telling").evaluate((f) => [...f.querySelectorAll("*")].map((e) => e.outerHTML));
    if (inside.length) walk.wrong = `the report holds ${inside.join(", ")}, not text alone`;
  },
  // ["report reads", text]
  //                    the report's field reads text, where * is
  //                    whatever the speech model heard, a word or more
  async "report reads"(walk, arg) {
    const { kept, page } = walk;
    const have = await page.locator(".telling").evaluate((f) => f.innerText.replace(/\n$/, ""));
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
    const have = await page.locator(".telling").evaluate((f) => f.innerText.replace(/\n$/, ""));
    const was = arg === "grown" ? kept.grown : kept.report;
    if (have !== was) walk.wrong = `the report reads ${JSON.stringify(have)}, not ${JSON.stringify(was)} as before`;
  },
  // ["report grew"]    the report's field holds more than it did at the
  //                    last "report reads", all of that and more words
  async "report grew"(walk) {
    const { kept, page } = walk;
    const have = await page.locator(".telling").evaluate((f) => f.innerText.replace(/\n$/, ""));
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
    // whole, with Report, and the row says only that it failed.
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
