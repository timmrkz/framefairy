// The steps of the sequences about the videos in the sidebar: added,
// dropped, labelled and gone from the disk. Each is said above it, a list
// with its name first. walkSequence in ../sequences.mjs walks it, and hands
// it the walk so far, the step's argument, what the caption box showed
// before it and the step's name. A sequence of any folder can take it.
// Below them the helpers only these steps use. See docs/TESTING.md.
import { existsSync, mkdirSync, readdirSync, renameSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, extname, join } from "node:path";
import { ask, caughtUp, clipList, control, episodeOn, fromSidebar, hold, letHeld, news, nextNews, pageUntil, seenUntil, settle, sidebarStill } from "../bridge.mjs";
import { dropOn, halfOf, standsAt, stillStands, toastSays, url } from "../common/steps.mjs";

export const steps = {
  // ["add", seconds]   adds a new video of so many seconds with Add
  // ["add", seconds, rate]
  //                    adds one filmed at a frame rate, "30000/1001" for
  //                    29.97 fps, whose frames keep their numbers in a
  //                    short
  // ["add", seconds, rate, timing]
  //                    the same with its frames at a phone's uneven times,
  //                    "uneven", or its picture starting after its sound,
  //                    "late"
  // ["add", seconds, rate, timing, "mkv"]
  //                    the same in Matroska, the way OBS records, with ""
  //                    for frames at the rate
  async add(walk, arg, s) {
    const { page, watch } = walk;
    const [seconds, rate, timing, inside] = [arg].flat();
    await control(url, `/pick?seconds=${seconds}${rate ? `&rate=${rate}` : ""}${timing ? `&timing=${timing}` : ""}${inside ? `&in=${inside}` : ""}`);
    await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
    await watch.step("add", s);
  },
  // ["label", [words, how]]
  //                    walks the pointer down the column of the three dots,
  //                    where every row it is on has to be lit with its dots
  //                    lit, then opens the three dots of the video on
  //                    screen in the sidebar, types the words in its label,
  //                    presses Tab for "Tab", or nothing for null, and
  //                    leaves the menu by how: a colour's name clicks that
  //                    colour and then Escape closes the menu, the same
  //                    with " after a press" after it presses beside the
  //                    field first and holds the first save, the same with
  //                    ", then Return" closes the menu with Return from
  //                    where WebKit leaves the focus after the click,
  //                    which has to keep the colour and show no ring
  //                    round the menu, "Remove label"
  //                    clicks the cross in the field and then Escape, "a
  //                    colour of its own, Return" or ", Escape" clicks a
  //                    shade in the colour picker and leaves it with that
  //                    key, which keeps the shade or puts back the colour
  //                    before, and "the colour picker, Escape" opens the
  //                    picker and leaves it with Escape without a click.
  //                    Leaving the picker with Escape has to leave the
  //                    field and the colour as they were before it opened.
  //                    "Return" presses Return, "Return, the save failing"
  //                    the same with the Go side refusing the save, which
  //                    has to say so in a toast and keep the label it had,
  //                    "Escape" puts back what was there. The menu has to
  //                    open on the label
  //                    chosen, or empty with the folder's name as its hint
  //                    and no colour, hang inside the sidebar and keep it
  //                    open, and the sidebar casts no shadow on the title
  //                    bar. Words in the field and a chosen colour go
  //                    together. The label the Go side keeps has to be the
  //                    one the row and the title bar show, and the video
  //                    has to keep its name and its bytes
  async label(walk, arg, s) {
    const { page, watch } = walk;
    const [typed, how] = arg;
    const colours = { Purple: "#942192", Blue: "#0a64d6", Pink: "#c2307a", Red: "#d0342c", Green: "#38923f" };
    const source = await episodeOn(page);
    const was = (await ask(page, "Library")).find((e) => e.source === source)?.label ?? null;
    const size = statSync(source).size;
    const row = page.locator("aside li:has(button.episode.current)");
    await page.mouse.move(20, 500);
    await sidebarStill(page);
    // Down the column of the three dots a pixel at a time, from the
    // sort mark to under the last row: wherever the pointer is on a
    // row, that row is lit and its dots are shown and lit, so going
    // from one mark to the next lights nothing twice. Tim saw the row
    // go dark the moment the pointer reached its dots, and every row
    // go dark in the gap to the next. Once, on the first step, since a
    // walk a pixel at a time takes its while.
    const column = await page.locator("aside .order button").boundingBox();
    const rowsAt = await page.evaluate(() => [...document.querySelectorAll("aside li")].map((li) => { const r = li.getBoundingClientRect(); return [r.top, r.bottom]; }));
    const x = column.x + column.width / 2;
    const once = typed === null && how === "Escape" && !was;
    for (let y = Math.round(rowsAt[0][0]); once && !walk.wrong && y < Math.round(rowsAt.at(-1)[1]); y++) {
      await page.mouse.move(x, y);
      const at = await page.evaluate(({ x, y }) => {
        const li = document.elementFromPoint(x, y)?.closest("aside li");
        if (!li) return "on no row";
        const lit = (el, pseudo) => getComputedStyle(el, pseudo).backgroundColor !== "rgba(0, 0, 0, 0)";
        if (!lit(li.querySelector(".episode"))) return "on a row that is not lit";
        if (getComputedStyle(li.querySelector(".tools")).opacity !== "1") return "on a row whose dots are hidden";
        const dots = li.querySelector("button.more");
        if (!lit(dots) && !lit(dots, "::before")) return "on dots that are not lit";
        return "";
      }, { x, y });
      if (at) walk.wrong = `the pointer ${Math.round(y - rowsAt[0][0])} px down the list, in the column of the dots, is ${at}`;
    }
    await row.hover();
    await row.locator("button.more").click();
    const menu = page.locator(".video-menu");
    await menu.waitFor();
    // The menu hangs inside the sidebar, so it reaches no further over
    // the workspace than the sidebar does, and the sidebar stays open
    // while the pointer is on the menu.
    // It opens on the label as it is, its words chosen so typing
    // replaces them. A video with none has an empty field with the name
    // of its folder as a hint, no colour chosen and no cross.
    const reach = await page.evaluate(() => {
      const m = document.querySelector(".video-menu").getBoundingClientRect();
      const a = document.querySelector("aside").getBoundingClientRect();
      const field = document.querySelector(".video-menu input");
      return {
        m: [m.left, m.right],
        a: [a.left, a.right],
        focused: document.activeElement?.getAttribute("aria-label"),
        words: field.value,
        hint: field.placeholder,
        cross: !!document.querySelector('.video-menu button[aria-label="Remove label"]'),
        chosen: field.selectionStart === 0 && field.selectionEnd === field.value.length,
        ringed:
          (document.querySelector(".video-menu .swatch.on")?.style.getPropertyValue("--swatch") ??
            document.querySelector(".video-menu .colour.well.on")?.style.getPropertyValue("--colour"))?.trim() ?? null,
        // The three dots and the sort mark are one column of marks of
        // one size, and the menu's icons stand in the sidebar's column
        // of icons, with their words where the sidebar's start.
        // The dots take the pointer over the row's whole height and
        // light a control's height of it, so what is lit is measured.
        marks: [
          ["aside li:has(button.episode.current) button.more", "::before"],
          ["aside .order button", null],
        ].map(([q, part]) => {
          const el = document.querySelector(q);
          const r = el?.getBoundingClientRect();
          return r && [r.left, r.right, part ? parseFloat(getComputedStyle(el, part).height) : r.height].join(" ");
        }),
        icons: [".video-menu .item svg", "aside .foot .nav svg"].map((q) => document.querySelector(q)?.getBoundingClientRect().left),
        // The lit part of the dots is in the middle of the row's button,
        // as far from its top as from its foot, and so are the dots.
        middles: (() => {
          const dots = document.querySelector("aside li:has(button.episode.current) button.more");
          const b = dots.getBoundingClientRect();
          const lit = getComputedStyle(dots, "::before");
          const top = b.top + parseFloat(getComputedStyle(dots).borderTopWidth) + parseFloat(lit.top);
          const row = document.querySelector("aside li:has(button.episode.current) .episode").getBoundingClientRect();
          const glyph = dots.querySelector("svg").getBoundingClientRect();
          return [top + parseFloat(lit.height) / 2, (glyph.top + glyph.bottom) / 2, (row.top + row.bottom) / 2];
        })(),
        // The menu's rows are as tall as the rows of the list it hangs
        // under.
        heights: [".video-menu .item", "aside .episode"].map((q) => document.querySelector(q)?.getBoundingClientRect().height),
      };
    });
    // The sidebar's shadow falls on the workspace and not on the title
    // bar above it: the bar over the sidebar is the colour it is far
    // to the right of it.
    const shade = await page.evaluate(async ({ shot, dpr }) => {
      const img = new Image();
      img.src = `data:image/png;base64,${shot}`;
      await img.decode();
      const c = document.createElement("canvas");
      c.width = img.naturalWidth;
      c.height = img.naturalHeight;
      const g = c.getContext("2d", { willReadFrequently: true });
      g.drawImage(img, 0, 0);
      const a = document.querySelector("aside").getBoundingClientRect();
      const px = (x) => [...g.getImageData(Math.floor(x * dpr), 0, 1, 1).data.slice(0, 3)];
      return { over: px(a.left + 40), beside: px(a.right + 300) };
    }, { shot: (await page.screenshot({ clip: await page.evaluate(() => { const a = document.querySelector("aside").getBoundingClientRect(); return { x: 0, y: a.top - 3, width: a.right + 320, height: 1 }; }) })).toString("base64"), dpr: await page.evaluate(() => devicePixelRatio) });
    const folder = basename(dirname(source));
    const words = was?.text ?? "";
    if (reach.m[0] < reach.a[0] || reach.m[1] > reach.a[1])
      walk.wrong ||= `the menu reaches ${reach.m.join(" to ")}, out of the sidebar at ${reach.a.join(" to ")}`;
    else if (shade.over.some((v, k) => Math.abs(v - shade.beside[k]) > 2))
      walk.wrong ||= `the title bar over the open sidebar is rgb(${shade.over}), beside it rgb(${shade.beside}): the sidebar casts its shadow on it`;
    else if (reach.focused !== "Label") walk.wrong ||= `the menu opened with the focus on ${reach.focused ?? "nothing"}, not on the label`;
    else if (reach.words !== words) walk.wrong ||= `the menu opened on ${JSON.stringify(reach.words)}, not ${JSON.stringify(words)}`;
    else if (!was && reach.hint !== folder) walk.wrong ||= `the field's hint is ${JSON.stringify(reach.hint)}, not the folder's name ${JSON.stringify(folder)}`;
    else if (reach.cross !== !!was) walk.wrong ||= `the menu opened ${reach.cross ? "with" : "without"} the cross, on ${was ? "a label" : "no label"}`;
    else if (!reach.chosen) walk.wrong ||= "the menu opened with the words in the field not chosen";
    else if (!reach.marks[1]) walk.wrong = "there is no sort mark to line the three dots up with, with two videos in the list";
    else if (reach.marks[0] !== reach.marks[1])
      walk.wrong = `the three dots stand at ${reach.marks[0]} (left, right, height), the sort mark at ${reach.marks[1]}`;
    else if (reach.icons[0] !== reach.icons[1]) walk.wrong = `the menu's icons start at ${reach.icons[0]}, the sidebar's at ${reach.icons[1]}`;
    else if (reach.middles.some((m) => m !== reach.middles[2]))
      walk.wrong = `the middle of the lit dots is at ${reach.middles[0]} px and of the dots at ${reach.middles[1]} px, the row's at ${reach.middles[2]} px`;
    else if (reach.heights[0] !== reach.heights[1]) walk.wrong = `the menu's rows are ${reach.heights[0]} px high, the list's ${reach.heights[1]} px`;
    else if (reach.ringed !== (was?.colour ?? null)) walk.wrong = `the menu opened on ${reach.ringed ?? "no colour"}, not ${was?.colour ?? "no colour"}`;
    // Tab takes the folder's name as a hint is taken, and the first
    // word chooses a colour.
    const field = menu.locator('input[aria-label="Label"]');
    if (typed === "Tab") await page.keyboard.press("Tab");
    else if (typed !== null) await field.fill(typed);
    const after = await page.evaluate(() => ({
      words: document.querySelector(".video-menu input").value,
      focused: document.activeElement?.getAttribute("aria-label"),
      ringed: document.querySelector(".video-menu .swatch.on")?.style.getPropertyValue("--swatch") ?? null,
    }));
    if (walk.wrong) {
      // Said already.
    } else if (typed === "Tab" && (after.words !== folder || after.focused !== "Label"))
      walk.wrong = `Tab left ${JSON.stringify(after.words)} in the field with the focus on ${after.focused ?? "nothing"}, not the folder's name in the field`;
    else if (after.words.trim() && !after.ringed && !was?.colour) walk.wrong = `the field says ${JSON.stringify(after.words)} and no colour is chosen`;
    else if (!after.words.trim() && after.ringed) walk.wrong = `the field is empty and ${after.ringed} is chosen`;
    await menu.hover();
    if (!walk.wrong && !(await page.evaluate(() => document.querySelector("aside").classList.contains("open"))))
      walk.wrong = "the sidebar closed under the menu";
    // The pill follows the words while they are typed.
    const pill = await page.evaluate(() => document.querySelector("aside li:has(button.episode.current) .tag")?.textContent ?? "");
    if (!walk.wrong && pill !== after.words.trim())
      walk.wrong = `while typing, the pill says ${JSON.stringify(pill)}, not ${JSON.stringify(after.words.trim())}`;
    const ringed = after.ringed ?? reach.ringed;
    const failing = how === "Return, the save failing";
    if (failing) await control(url, `/hold?call=SetLabel&fail=${encodeURIComponent("the disk is full")}`);
    if (how === "Return" || how === "Escape" || failing) await page.keyboard.press(how === "Escape" ? how : "Enter");
    else if (how.startsWith("a colour of its own") || how.startsWith("the colour picker")) {
      // A shade clicked in the colour picker at the end of the
      // colours, or none, and the picker left with Return or Escape,
      // then the menu with Escape.
      // The picker lies over the menu from the moment it is there. For
      // a frame it lay under it, so a click on its shade made that soon
      // pressed Remove, which CI's quick machines did now and then,
      // issue #401: its box is read the moment it is put in the page.
      await page.evaluate(() => {
        window.__pickerOver = new Promise((said) => {
          const seen = new MutationObserver(() => {
            const pop = document.querySelector(".colour-pop");
            if (!pop) return;
            seen.disconnect();
            const box = pop.closest("[data-bits-floating-content-wrapper]");
            const menu = document.querySelector(".video-menu")?.closest("[data-bits-floating-content-wrapper]");
            if (!box || !menu) return said(`the colour picker or the menu has no box of its own to lie in`);
            const z = [getComputedStyle(box).zIndex, getComputedStyle(menu).zIndex];
            // Equal z-indexes put the one later in the page on top.
            if (Number(z[0]) > Number(z[1]) || (z[0] === z[1] && menu.compareDocumentPosition(box) & Node.DOCUMENT_POSITION_FOLLOWING)) return said(null);
            said(`the colour picker opened at z-index ${z[0]}, under the menu at ${z[1]}, so a click on it lands on the menu`);
          });
          seen.observe(document.body, { childList: true, subtree: true });
        });
      });
      await menu.locator('button[aria-label="Another colour"]').click();
      await page.locator(".colour-pop").waitFor();
      const under = await page.evaluate(() => window.__pickerOver);
      if (under) walk.wrong ||= under;
      // A click Playwright makes on the shade itself, so it says so if
      // anything else would take it.
      const shade = page.locator('.colour-pop [aria-label="Shade"]');
      const box = await shade.boundingBox();
      if (!box) walk.wrong ||= "the colour picker opened with no shade to click";
      else if (how.startsWith("a colour of its own")) await shade.click({ position: { x: box.width * 0.8, y: box.height * 0.3 } });
      await page.keyboard.press(how.endsWith("Return") ? "Enter" : "Escape");
      await page.locator(".colour-pop").waitFor({ state: "detached" });
      if (!(await menu.isVisible())) walk.wrong ||= `${how.split(", ").pop()} in the colour picker closed the menu too`;
      // Escape put back what was there before the picker opened: the
      // words in the field and the colour chosen. Tim's review found a
      // video with no label given its folder's name in white.
      const back = await page.evaluate(() => ({
        words: document.querySelector(".video-menu input")?.value,
        ringed:
          (document.querySelector(".video-menu .swatch.on")?.style.getPropertyValue("--swatch") ??
            document.querySelector(".video-menu .colour.well.on")?.style.getPropertyValue("--colour"))?.trim() ?? null,
      }));
      if (!walk.wrong && how.endsWith("Escape") && (back.words !== after.words || back.ringed !== ringed))
        walk.wrong = `Escape in the colour picker left ${JSON.stringify(back.words)} in ${back.ringed ?? "no colour"}, not ${JSON.stringify(after.words)} in ${ringed ?? "no colour"} as before it opened`;
      await page.keyboard.press("Escape");
    } else if (how === "Remove label") {
      await menu.locator('button[aria-label="Remove label"]').click();
      await page.keyboard.press("Escape");
    } else {
      const [colour, pressFirst] = how.replace(", then Return", "").split(" after a press");
      if (pressFirst !== undefined) {
        // A press in the menu beside the field takes the focus from it
        // and gives it to nothing, which is what WebKit does with a
        // button pressed. The first save the Go side hears of is held
        // until the colour is picked, so a later one is heard first.
        await hold(page, "SetLabel");
        const box = await menu.locator(".label").boundingBox();
        await page.mouse.click(box.x + 3, box.y + 3);
      }
      await menu.locator(`button[aria-label="${colour}"]`).click();
      if (pressFirst !== undefined) {
        await settle(page);
        await letHeld(page);
      }
      if (how.endsWith(", then Return")) {
        // WebKit gives a pressed button no focus, so the focus is on
        // the menu itself. A key pressed there shows the keyboard's
        // ring wherever the focus is.
        await menu.evaluate((m) => m.focus());
        await page.keyboard.press("Shift");
        const ring = await menu.evaluate((m) => {
          const st = getComputedStyle(m);
          return st.outlineStyle !== "none" && parseFloat(st.outlineWidth) > 0 ? `${st.outlineWidth} ${st.outlineStyle} ${st.outlineColor}` : "";
        });
        if (ring) walk.wrong = `with the focus on the menu, the menu wears a ring, ${ring}`;
        await page.keyboard.press("Enter");
        if (!walk.wrong) {
          await settle(page);
          if (await menu.isVisible()) walk.wrong = "Return after a colour was clicked left the menu open";
        }
        if (walk.wrong) await page.keyboard.press("Escape");
      } else await page.keyboard.press("Escape");
    }
    await menu.waitFor({ state: "detached" });
    await page.mouse.move(750, 400);
    await sidebarStill(page);
    await settle(page);
    const named = colours[how.replace(", then Return", "").split(" after a press")[0]];
    // A colour clicked in an empty field takes the folder's name.
    const text = after.words.trim() || (named ? folder : "");
    let want = how === "Escape" || failing ? was : how === "Remove label" || !text ? null : { text, colour: named ?? ringed };
    // A shade taken with Return is a colour of the picker's own, which
    // is any but the one there was. Escape put back the one there was.
    const own = how === "a colour of its own, Return";
    const ep = (await ask(page, "Library")).find((e) => e.source === source);
    const kept = ep?.label ?? null;
    if (own && kept && kept.colour !== ringed) want = { ...want, colour: kept.colour };
    const drawn = await page.evaluate(() => {
      const pill = (el) => (el ? { text: el.textContent, colour: el.style.getPropertyValue("--tag") } : null);
      return {
        row: pill(document.querySelector("aside li:has(button.episode.current) .tag")),
        bar: pill(document.querySelector("header.bar .tag")),
      };
    });
    const says = (l) => (l ? `${l.text} in ${l.colour}` : "no label");
    const alike = (a, b) => JSON.stringify(a) === JSON.stringify(b);
    if (walk.wrong) {
      // Said already, while the menu was open.
    } else if (failing && !(await toastSays(page)).includes("the disk is full"))
      walk.wrong = `the save the Go side refused says ${JSON.stringify(await toastSays(page))}, not why`;
    else if (!alike(kept, want)) walk.wrong = `the Go side keeps ${says(kept)}, not ${says(want)}`;
    else if (!alike(drawn.row, want)) walk.wrong = `the row shows ${says(drawn.row)}, the Go side keeps ${says(kept)}`;
    else if (!alike(drawn.bar, want)) walk.wrong = `the title bar shows ${says(drawn.bar)}, the Go side keeps ${says(kept)}`;
    else if (ep.name !== basename(source)) walk.wrong = `the sidebar names the video ${ep.name}, not ${basename(source)}`;
    else if (!existsSync(source) || statSync(source).size !== size) walk.wrong = `the video at ${source} is gone or changed`;
    else if (existsSync(join(workDir(source), "label.json")) !== !!want)
      walk.wrong = `the work folder ${want ? "has no" : "still has a"} label.json`;
    // The refusal is the answer here, so it is taken off the calls that
    // failed before the rules read them, and the toast goes, so the
    // next step finds the app as it was.
    if (failing) {
      await page.evaluate(() => {
        for (const c of window.__calls) if (c.name === "SetLabel" && String(c.failed).includes("the disk is full")) c.failed = null;
      });
      await page.locator(".toast button.close").click();
    }
    await watch.step("label", s);
  },
  // ["drop", seconds, where]
  //                    makes a new video of so many seconds, opens the
  //                    "settings", "updates" or stays on the "workspace",
  //                    holds the file over the app the way the runtime
  //                    reports a drag from Finder, and drops it there. The
  //                    app has to wear its frame while the file is held
  //                    over it and not after the drop
  // ["drop again", where]
  //                    the same with the video dropped last
  // ["drop not a video", where] or ["drop not a video", [where, stem]]
  //                    the same with a PDF, named stem if given
  async "drop not a video"(walk, arg, s, what) {
    const { page, watch } = walk;
    const [where, stem] = what === "drop" ? [arg[1]] : [arg].flat();
    let file;
    if (what === "drop") file = walk.droppedVideo = await control(url, `/pick?seconds=${arg[0]}&box=0`);
    else if (what === "drop again") file = walk.droppedVideo;
    else {
      if (walk.droppedOther) walk.droppedBefore = walk.droppedOther;
      file = walk.droppedOther = join(tmpdir(), `${stem ?? "notizen"}-${process.pid}.pdf`);
      writeFileSync(file, "%PDF-1.4\n");
    }
    if (where !== "workspace") {
      const name = where === "settings" ? "Settings" : "Updates";
      await fromSidebar(page, () => page.locator("aside").getByText(name, { exact: true }).first().click());
      await settle(page);
    }
    if (what === "drop not a video") walk.standing = await standsAt(page);
    walk.wrong = await dropOn(page, [file]);
    if (what === "drop not a video") rmSync(file, { force: true });
    await watch.step("add", s);
  },
  // ["no reason"]      no toast names the video dropped last: a video
  //                    that is added says nothing
  async "no reason"(walk) {
    const { page } = walk;
    // Once the interface has heard all the Go side said, a toast it
    // would show is there.
    await caughtUp(page, url);
    const said = await toastSays(page);
    const name = walk.droppedVideo ? basename(walk.droppedVideo) : "";
    if (name && said.includes(name)) walk.wrong = `the app says "${said}" about ${name}, which it added`;
    else if (!name && said) walk.wrong = `the app still says "${said}"`;
  },
  // ["stacked"]        the two PDFs dropped last each have a toast, the
  //                    newer on top, the way macOS stacks its notes
  async stacked(walk) {
    const { page } = walk;
    // Tim dropped one picture after another and saw only the last:
    // a newer reason replaced the older.
    const said = await page.evaluate(() => [...document.querySelectorAll(".toast .text")].map((t) => t.textContent.trim()));
    const [newer, older] = [basename(walk.droppedOther), basename(walk.droppedBefore)];
    const tops = await page.evaluate(() => [...document.querySelectorAll(".toast")].map((t) => Math.round(t.getBoundingClientRect().top)));
    if (said.length !== 2) walk.wrong = `${said.length} toasts show, not one for each of ${older} and ${newer}: ${JSON.stringify(said)}`;
    else if (!said[0].includes(newer) || !said[1].includes(older)) walk.wrong = `the toasts read ${JSON.stringify(said)}, not ${newer} on top of ${older}`;
    else if (!(tops[0] < tops[1])) walk.wrong = `the newer toast stands at ${tops[0]} px and the older at ${tops[1]} px`;
  },
  // ["dropped in front"]
  //                    the video dropped last is the one on screen, and in
  //                    the library once
  async "dropped in front"(walk) {
    const { page } = walk;
    const name = basename(walk.droppedVideo);
    const on = await seenUntil(page, url, () => episodeOn(page), (on) => on === walk.droppedVideo);
    const library = await ask(page, "Library");
    const times = library.filter((e) => e.source === walk.droppedVideo).length;
    if (on !== walk.droppedVideo) walk.wrong = `${on ? basename(on) : "no video"} is on screen, not ${name}, which was dropped`;
    else if (times !== 1) walk.wrong = `${name} is in the library ${times} times`;
    else if (!(await page.locator(".workspace.front").count())) walk.wrong = `${name} is in the sidebar, and its workspace is not open`;
    await settle(page);
  },
  // ["not a video"]    the PDF dropped last is not in the library, and a
  //                    toast says by its name that it cannot be added
  //                    because it is not a video, with the workspace where
  //                    it stood before the drop
  async "not a video"(walk) {
    const { page } = walk;
    const name = basename(walk.droppedOther);
    const said = await seenUntil(page, url, () => toastSays(page), (said) => said.includes(name));
    const library = await ask(page, "Library");
    if (!said.includes(`${name} cannot be added because it is not a video`)) walk.wrong = `the app says "${said}", not that ${name} cannot be added because it is not a video`;
    else if (library.some((e) => e.name === name)) walk.wrong = `${name} is in the library`;
    else walk.wrong = await stillStands(page, walk.standing);
  },
  // ["add sound alone"] adds a file of sound with no picture, named like a
  //                    video, with Add
  async "add sound alone"(walk) {
    const { page } = walk;
    await control(url, "/pick-sound");
    await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
    await caughtUp(page, url);
  },
  // ["add cameras", seconds, switch]
  //                    adds one filmed by two cameras that switch so many
  //                    seconds in, each looking at its own subject
  // ["add cameras", seconds, switch, "hevc"]
  //                    the same with its picture in HEVC, 10-bit, which
  //                    the Go side decodes, the way Tim's camera files are
  async "add cameras"(walk, arg, s) {
    const { page, watch } = walk;
    const [seconds, at, codec] = arg;
    walk.cameraSwitch = at;
    await control(url, `/pick?seconds=${seconds}&switch=${at}${codec ? `&codec=${codec}` : ""}`);
    await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
    await watch.step("add", s);
  },
  // ["episode"]        opens the episode on screen before again
  async episode(walk) {
    const { page } = walk;
    await fromSidebar(page, () => page.locator("aside li", { hasText: walk.episodeBefore.split("/").pop() }).first().click());
    await page.locator(".listhead button.search").first().waitFor();
    await settle(page);
  },
  // ["left out", name] Add left out the file whose name starts so, says why
  //                    in a toast, and the library does not hold it
  async "left out"(walk, arg) {
    const { page } = walk;
    const said = await toastSays(page);
    const library = await ask(page, "Library");
    if (!said.includes(`the picture of ${arg}`) || !said.includes("cannot be decoded")) {
      walk.wrong = `the app says "${said.trim()}", not why ${arg} was left out`;
    } else if (library.some((e) => e.name.startsWith(arg))) {
      walk.wrong = `${arg} is in the library`;
    }
  },
  // ["dot", kind]      the video's dot in the sidebar is this kind, "err",
  //                    "warn" or "" for none of the two
  async dot(walk, arg) {
    const { page } = walk;
    const kind = await page.evaluate(() => {
      const dot = document.querySelector("aside button.episode.current .dot");
      return dot ? (["err", "warn"].find((k) => dot.classList.contains(k)) ?? "") : null;
    });
    if (kind !== arg) walk.wrong = `the video's dot in the sidebar is "${kind}", not "${arg}"`;
  },
  // ["folder removed", when]
  //                    deletes the folder beside the video on screen,
  //                    <name>.framefairy, from outside the app, the way
  //                    Finder does: "away" while the app is behind
  //                    another, which then comes back to the front, and
  //                    "closed" while the app is closed, which is then
  //                    opened again, and "with the video" deletes the
  //                    video too, while the app is behind another.
  //                    "written back" deletes both while the app is
  //                    closed and writes the chosen clip back into a new
  //                    folder, the way the app did when it was asked
  //                    about a video whose folder was gone, before 2.247
  async "folder removed"(walk, arg) {
    const { page } = walk;
    walk.folderGone = await episodeOn(page);
    const folder = join(dirname(walk.folderGone), `${basename(walk.folderGone, extname(walk.folderGone))}.framefairy`);
    if (!existsSync(folder)) {
      walk.wrong = `${basename(folder)} is not there to delete`;
      return;
    }
    // Again while the app writes into it, as a person deletes it again
    // when Finder says the folder is not empty: a file the app writes
    // while the folder is emptied leaves it not empty, which is not the
    // folder coming back. On CI's busy runners the app was still
    // writing after five tries in a second and a half, and the walk
    // failed on its own delete. So it deletes until the folder is gone,
    // trying again each time the interface has caught up with what the
    // Go side told it, however often that takes on a busy machine. What
    // the app must not do, make the folder again once it is gone, is
    // what the walk then looks for.
    for (;;) {
      try {
        rmSync(folder, { recursive: true, force: true });
        break;
      } catch (err) {
        if (err.code !== "ENOTEMPTY") throw err;
        await caughtUp(page, url);
      }
    }
    walk.videoGone = arg === "with the video" || arg === "written back";
    if (walk.videoGone) rmSync(walk.folderGone, { force: true });
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
  },
  // ["let go"]         the video whose folder was removed has left the
  //                    sidebar and the library, its workspace is closed,
  //                    nothing writes its folder back and the video file
  //                    itself is still there, unless it was deleted too
  async "let go"(walk) {
    const { page } = walk;
    const name = basename(walk.folderGone);
    const folder = join(dirname(walk.folderGone), `${basename(walk.folderGone, extname(walk.folderGone))}.framefairy`);
    // The sidebar first, before the walk asks the Go side anything,
    // so it is the app that read the library again and not the walk.
    // Until the sidebar has let the video go, read each time the page
    // changes, and then once the interface has heard all the Go side
    // said after that.
    const named = () => page.evaluate((n) => [...document.querySelectorAll("aside button.episode .name")].some((e) => e.textContent === n), name);
    await seenUntil(page, url, named, (there) => !there);
    // And until no job of the video still runs, which is what would
    // write its folder back late, read each time a job has news.
    let heard = await news(page);
    while ((await ask(page, "Jobs")).some((j) => j.episode === walk.folderGone && (j.state === "running" || j.state === "queued"))) {
      heard = await nextNews(page, heard);
    }
    await caughtUp(page, url);
    const library = await ask(page, "Library");
    if (library.some((e) => e.source === walk.folderGone)) walk.wrong = `${name} is still in the library`;
    else if ((await episodeOn(page)) === walk.folderGone) walk.wrong = `${name} is still open`;
    // With what was written into it, which says what wrote it.
    else if (existsSync(folder))
      walk.wrong = `${basename(folder)} was written back, with ${readdirSync(folder, { recursive: true }).join(", ") || "nothing in it"}`;
    else if (!walk.videoGone && !existsSync(walk.folderGone)) walk.wrong = `${name} itself was deleted`;
    else if ((await page.locator(".workspace.front").count()) && !(await episodeOn(page))) {
      walk.wrong = `a workspace is still open with no video picked in the sidebar`;
    }
    await settle(page);
  },
  // ["file moved away"]
  //                    moves the file of the video on screen away from
  //                    outside the app, the way Finder does, while
  //                    another video is on screen, and opens it again
  //                    from the sidebar: one toast says the file is no
  //                    longer there and nothing else does, the video
  //                    preview shows nothing and has not moved, and the
  //                    clip list is as it was. Then it puts the file back
  //                    and brings the app to the front, and the video
  //                    preview shows the video again
  async "file moved away"(walk) {
    const { page } = walk;
    const source = await episodeOn(page);
    const front = ".workspace.front";
    const picture = () =>
      page.evaluate((f) => {
        const r = document.querySelector(`${f} .screen`)?.getBoundingClientRect();
        return r ? { top: r.top, left: r.left, width: r.width, height: r.height } : null;
      }, front);
    const was = await picture();
    const listed = (await clipList(page))?.cards ?? [];
    const buttons = page.locator("aside button.episode");
    const mine = await buttons.evaluateAll((all) => all.findIndex((b) => b.classList.contains("current")));
    await fromSidebar(page, () => buttons.nth(mine === 0 ? 1 : 0).click());
    await settle(page);
    renameSync(source, `${source}.away`);
    await fromSidebar(page, () => buttons.nth(mine).click());
    // The video preview lets go of the picture it showed. Then every
    // call the workspace made on waking has been answered, so any
    // toast one of them would bring is there by now.
    await page.locator(`${front} .stack.missing`).waitFor();
    await settle(page);
    const want = `The video file is no longer at ${source}. Put it back there, or remove the video from the list.`;
    const toasts = await page.evaluate(() => [...document.querySelectorAll(".toast .text")].map((t) => t.textContent.trim()));
    const inside = await page.evaluate((f) => {
      const at = (q) => document.querySelector(`${f} ${q}`)?.textContent.trim() ?? "";
      return at(".screen .failed") || at(".error");
    }, front);
    const now = await picture();
    const cards = (await clipList(page))?.cards ?? [];
    const moved = (a, b) => !a || !b || ["top", "left", "width", "height"].some((k) => Math.abs(a[k] - b[k]) > 0.5);
    if (toasts.length !== 1 || toasts[0] !== want) walk.wrong = `the toasts say ${JSON.stringify(toasts)}, not once "${want}"`;
    else if (inside) walk.wrong = `the workspace says it as well: "${inside}"`;
    else if (moved(was, now)) walk.wrong = `the video preview moved from ${JSON.stringify(was)} to ${JSON.stringify(now)}`;
    else if (JSON.stringify(cards) !== JSON.stringify(listed)) {
      walk.wrong = `the clip list held ${listed.join(", ") || "nothing"} and holds ${cards.join(", ") || "nothing"}`;
    }
    renameSync(`${source}.away`, source);
    if (walk.wrong) return;
    // Put back in Finder, and the app comes to the front: the sidebar
    // sees it, and so does the video preview, which shows its picture
    // again. Tim saw the sidebar know and the video preview not.
    await page.evaluate(() => {
      window.dispatchEvent(new FocusEvent("blur"));
      window.dispatchEvent(new FocusEvent("focus"));
    });
    await caughtUp(page, url);
    const back = (await page.locator(`${front} .stack:not(.missing)`).count()) > 0;
    if (!back) walk.wrong = "the file is back and the sidebar says so, but the video preview still shows nothing";
    else if (await page.evaluate((f) => !!document.querySelector(`${f} .screen .failed`), front)) {
      walk.wrong = "the file is back, and the video preview says it could not be shown";
    }
  },
  // ["add namesake", seconds]
  //                    adds a new video of so many seconds with the file
  //                    name of the bridge's episode, from another folder
  async "add namesake"(walk, arg, s) {
    const { page, watch } = walk;
    await control(url, `/pick?seconds=${arg}&namesake=1`);
    await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
    await watch.step("add", s);
  },
  // ["words again"]    the speech stand-in says its sentences again from
  //                    the first word, so a video added next is heard as
  //                    the bridge's episode was and its clip has the same
  //                    name
  async "words again"(walk) {
    await control(url, "/speech?go&from=0");
    walk.speechHeld = false;
  },
  // ["transcript", how] opens the three dots of the video on screen in the
  //                    sidebar, whose row about the words says the one
  //                    thing that can be done with them now, issue #432.
  //                    "transcribe" waits until nothing runs for the
  //                    video and holds the speech, needs the hand on the
  //                    last part not transcribed yet to light it in half
  //                    the grey of a lit card with the cards of what was
  //                    said closed, then needs the menu to say
  //                    Transcribe video and clicks it: in the next
  //                    frame the menu has closed and the video's dot
  //                    pulses, with no number in its row. Once the work
  //                    runs, the range picker frames the part it works
  //                    on, breathing with the motes, over that part's
  //                    dots. "search stops it" clicks Search in the head
  //                    with the model holding its answer: the whole
  //                    video's transcription stops, its frame goes, and
  //                    the menu says Transcribe video and cannot be
  //                    clicked while the search runs. Once the search is
  //                    cancelled the transcription stays stopped and the
  //                    menu can be clicked again. "stop" needs Stop
  //                    transcribing and clicks it while a piece is being
  //                    heard: in the next frame the
  //                    frame is fading, standing still, the menu has
  //                    closed, and once the speech hears on and the job
  //                    ends the dots are where they were at the click.
  //                    Opened again the menu says Transcribe video at
  //                    once. "waits for the search", while the video's
  //                    first search transcribes its window with the
  //                    speech held, needs Transcribe video that cannot be
  //                    clicked, says to wait for the search, then lets
  //                    the speech go and needs it to be clickable once
  //                    the search is done.
  //                    "continue" waits the same way for the stopped
  //                    work to end, needs the frame gone and the dots
  //                    back, and clicks Transcribe video again, which
  //                    pulses the dot in the next frame again. "Text" and
  //                    "Subtitles" wait, asking again each time the work
  //                    says how far it heard, until the Go side says the
  //                    video is transcribed to its end, then need the row
  //                    to say Export transcript, hover it, which
  //                    hangs the formats on the right of the menu, and
  //                    click that format: the file saved has to be named
  //                    for the video with the format's ending and hold
  //                    the words of the video, and the menu has closed.
  //                    "Escape" hangs the formats the same way and needs
  //                    Escape to close them and the menu with them.
  //                    "click" hangs them and clicks Export transcript,
  //                    which has to keep them out. "keys" does it with the
  //                    keyboard alone: Return on Export transcript has to
  //                    put the keyboard on Text, the down arrow on
  //                    Subtitles, and Return saves the subtitles
  async transcript(walk, arg, s) {
    const { page } = walk;
    const source = await episodeOn(page);
    const row = page.locator("aside li:has(button.episode.current)");
    await page.mouse.move(20, 500);
    await sidebarStill(page);
    await row.hover();
    await row.locator("button.more").click();
    const menu = page.locator(".video-menu");
    await menu.waitFor();
    // The row about the words is the first row of the menu, under the
    // label.
    const says = () => page.evaluate(() => (document.querySelector(".video-menu .item")?.textContent ?? "").replace(/\s+/g, " ").trim());
    const first = await says();
    // The row about the words clicked, and what the next frame shows:
    // the menu closed, the way a menu item closes it, and whether the
    // video's dot in the sidebar pulses, before the Go side has said
    // anything. How far it has come is the dots on the range picker's
    // and nothing else's, Tim's call on #458.
    // Where the dots of what is not transcribed are, part by part,
    // from where to where each is drawn.
    // And how far the columns of dots stand in from the part's right
    // edge, which moves every dot sideways when it changes.
    const dotsNow = () =>
      [...document.querySelectorAll(".unheard")]
        .map((el) => `${el.style.getPropertyValue("--at")} to ${el.style.getPropertyValue("--to")} laid ${getComputedStyle(el, "::before").right} in`)
        .join(", ");
    await page.evaluate(`window.dotsNow = ${dotsNow}`);
    const pick = (want) =>
      page.evaluate(
        (want) =>
          new Promise((done) => {
            const item = document.querySelector(".video-menu .item");
            const said = (item?.textContent ?? "").replace(/\s+/g, " ").trim();
            if (said !== want) return done({ said });
            item.click();
            requestAnimationFrame(() =>
              done({
                said,
                open: !!document.querySelector(".video-menu"),
                busy: !!document.querySelector("aside li:has(button.episode.current) .dot.busy"),
                number: !!document.querySelector("aside li:has(button.episode.current) .share"),
                // The whole video's frame on the range picker, still
                // there as work in hand or going.
                framed: !!document.querySelector(".part:not(.going) .window.frame.locked"),
                going: !!document.querySelector(".part.going .window.frame.locked"),
                dots: window.dotsNow(),
              }),
            );
          }),
        want,
      );
    // The row about the words as it is now: what it says, whether it
    // can be clicked, and what its title says.
    const item = () =>
      page.evaluate(() => {
        const b = document.querySelector(".video-menu .item");
        return { said: (b?.textContent ?? "").replace(/\s+/g, " ").trim(), off: !!b?.disabled, title: b?.title ?? "" };
      });
    const reopen = async () => {
      await row.hover();
      await row.locator("button.more").click();
      await menu.waitFor();
    };
    const searching = async () => (await ask(page, "Jobs")).some((j) => j.episode === source && j.kind === "search" && (j.state === "running" || j.state === "queued"));
    if (arg === "waits for the search") {
      // The whole video's transcription and the work on its clips go one
      // at a time: while its first search runs, Transcribe video cannot
      // be clicked, and once the search is done it can. Tim's report of
      // 26 October, 12.27: Transcribe the rest and Continue next, which
      // waited for the search, were a menu nobody could read.
      const searchHears = async () => {
        const j = (await ask(page, "Jobs")).findLast((j) => j.episode === source && j.kind === "search");
        return j?.state === "running" && (j.step === "hearing" || j.lane === "hearing");
      };
      if (!(await searchHears())) {
        walk.wrong = "the first search is not transcribing its window, and this step is about the menu while it does";
        return;
      }
      const during = await item();
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      if (during.said !== "Transcribe video" || !during.off || !during.title.includes("once the search is done")) {
        walk.wrong = `while the search runs, the menu says ${JSON.stringify(during)}, not Transcribe video that cannot be clicked`;
        return;
      }
      await control(url, "/speech?go");
      for (let seen = await news(page); await searching(); ) seen = await nextNews(page, seen);
      await settle(page);
      await reopen();
      const after = await item();
      await page.keyboard.press("Escape");
      if (after.said !== "Transcribe video" || after.off) walk.wrong = `the search done, the menu says ${JSON.stringify(after)}, not Transcribe video to click`;
      return;
    }
    if (arg === "transcribe" || arg === "continue") {
      const want = "Transcribe video";
      // Nothing runs for the video first, its first search and the
      // work that was stopped included, so a dot that pulses in the
      // next frame pulses for the click and for nothing else. Found by
      // the review of #458: the first search still heard, and the dot
      // pulsed for it. The speech is held from then on, so the work
      // the click starts is still running when the next step looks.
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      const runs = async () => (await ask(page, "Jobs")).some((j) => j.episode === source && (j.state === "running" || j.state === "queued"));
      for (let seen = await news(page); await runs(); ) seen = await nextNews(page, seen);
      await control(url, "/speech?hold");
      await settle(page);
      if (await page.locator("aside li:has(button.episode.current) .dot.busy").count()) {
        walk.wrong = "the video's dot pulses with nothing running";
        return;
      }
      // Nothing running, the range picker frames nothing, and what is
      // not transcribed yet still shows its dots.
      if (await page.locator(".window.frame.locked").count()) {
        walk.wrong = "with nothing running, the range picker frames a part as work in hand";
        return;
      }
      if (arg === "continue" && !(await page.locator(".unheard").count())) {
        walk.wrong = "the whole video stopped, and the range picker shows no dots on what is left";
        return;
      }
      // With the cards of what was said closed, the hand on a part not
      // transcribed yet lights it all the same, in half the grey of a
      // lit card. It lit only with the cards open, and in the whole
      // grey, which was far too strong, Tim's report of 24 October.
      if (arg === "transcribe") {
        const lit = async (x, y) => {
          await page.mouse.move(x, y);
          await settle(page);
          return page.evaluate(() => {
            const probe = document.createElement("div");
            probe.style.background = "var(--ink-3)";
            document.body.append(probe);
            const grey = getComputedStyle(probe).backgroundColor;
            probe.remove();
            const parts = [...document.querySelectorAll(".track .unheard")];
            const on = parts.find((u) => u.classList.contains("lit"));
            return { cards: !!document.querySelector("ol.said"), lit: parts.map((u) => u.classList.contains("lit")), fill: on ? getComputedStyle(on).backgroundColor : "", grey };
          });
        };
        const part = await page.evaluate(() => {
          const r = [...document.querySelectorAll(".track .unheard")].at(-1)?.getBoundingClientRect();
          return r ? { x: r.left + r.width / 2, y: r.top + r.height * 0.75 } : null;
        });
        if (!part) {
          walk.wrong = "before the whole video is transcribed, the range picker shows no part not transcribed yet";
          return;
        }
        const under = await lit(part.x, part.y);
        const away = await lit(0, 0);
        if (under.cards) walk.wrong = "the cards of what was said are open, and this step is about the range picker without them";
        else if (!under.lit.at(-1) || under.lit.filter(Boolean).length !== 1)
          walk.wrong = `with the cards of what was said closed, the hand on the last part not transcribed yet lit parts ${JSON.stringify(under.lit)}`;
        else if (!halfOf(under.fill, under.grey)) walk.wrong = `a part not transcribed yet under the hand is lit in ${under.fill}, not half the grey of a lit card, ${under.grey}`;
        else if (away.lit.some(Boolean)) walk.wrong = `the hand gone from the range picker, parts ${JSON.stringify(away.lit)} stay lit`;
        if (walk.wrong) return;
      }
      await row.hover();
      await row.locator("button.more").click();
      await menu.waitFor();
      // The call reaches the Go side only once the frame of the click
      // is read, the way a busy machine answers late, so the dot pulses
      // for the click and not for the job's first news. Held on the
      // page, so settle does not wait for it.
      await hold(page, "Transcribe");
      const after = await pick(want);
      await settle(page);
      if (after.said !== want) walk.wrong = `the menu says ${JSON.stringify(after.said)}, not ${want}`;
      else if (after.open) walk.wrong = `the menu stayed open after ${want} was clicked`;
      else if (!after.busy) walk.wrong = `the frame after ${want} was clicked, the video's dot does not say that work runs`;
      else if (after.number) walk.wrong = `after ${want} was clicked, the video's row shows a number`;
      if (after.said === want) await letHeld(page);
      if (walk.wrong) {
        await page.keyboard.press("Escape");
        return;
      }
      // Once the work has said it runs, the range picker frames the part
      // it works on the way a search's window is framed while it hears,
      // breathing and shedding the motes, over that part's dots, Tim's
      // pick on #458.
      const framed = () =>
        page.evaluate(() => {
          const frame = document.querySelector(".window.frame.locked.waiting");
          return {
            frame: !!frame,
            motes: frame?.querySelectorAll(".mote").length ?? 0,
            dots: (() => {
              if (!frame) return 0;
              const f = frame.getBoundingClientRect();
              return [...document.querySelectorAll(".unheard")].filter((el) => {
                const r = el.getBoundingClientRect();
                return r.left < f.right && f.left < r.right && getComputedStyle(el, "::before").display !== "none";
              }).length;
            })(),
          };
        });
      // It hears once the Go side says so. Queued first, for as long as
      // a busy machine takes, its frame is on hold and sheds nothing.
      const hears = async () =>
        (await ask(page, "Jobs")).some((j) => j.episode === source && j.kind === "transcribe" && j.state === "running" && j.step === "hearing");
      for (let seen = await news(page); !(await hears()); ) seen = await nextNews(page, seen);
      await settle(page);
      const look = await framed();
      if (!look.frame) walk.wrong = `while the whole video is transcribed, the range picker frames no part of it`;
      else if (!look.motes) walk.wrong = `while the whole video is transcribed, its frame on the range picker sheds no motes`;
      else if (!look.dots) walk.wrong = `while the whole video is transcribed, the part it works on shows no dots under its frame`;
      return;
    }
    const whole = async () => (await ask(page, "Library")).find((e) => e.source === source)?.transcribed;
    // The whole video's transcription as the Go side has it, and a
    // wait on its news until its step is one of these.
    const transcription = async () => (await ask(page, "Jobs")).findLast((j) => j.episode === source && j.kind === "transcribe");
    const stepIs = async (...steps) => {
      for (let seen = await news(page); !steps.includes((await transcription())?.step); ) seen = await nextNews(page, seen);
    };
    const partNow = () =>
      page.evaluate(() => {
        const part = document.querySelector(".part .window.frame.locked");
        return {
          part: !!part,
          behind: !!part?.classList.contains("behind"),
          // A walk that asks for less motion sees the breath left out,
          // as the app leaves it out on a Mac set that way, see
          // .waiting in app.css, so there the frame only has to wear it.
          breathes:
            !!part &&
            part.classList.contains("waiting") &&
            getComputedStyle(part).animationName === (matchMedia("(prefers-reduced-motion: reduce)").matches ? "none" : "breath"),
          motes: part?.querySelectorAll(".mote").length ?? 0,
          // What it wore, said when it does not breathe.
          wore: part
            ? `${part.className}, animation ${getComputedStyle(part).animationName} ${getComputedStyle(part).animationPlayState}, in ${document.querySelectorAll(".part").length} parts, ${[...document.querySelectorAll(".part")].map((p) => p.className).join(" and ")}`
            : "",
        };
      });
    if (arg === "search stops it") {
      // A search asked for while the whole video is transcribed stops
      // the transcription, and it does not start again by itself once
      // the search is done: Transcribe video does, clicked again. While
      // the search runs it cannot be clicked. Tim's report of 26
      // October, 12.27. The model holds its answer, so the search keeps
      // running until it is cancelled.
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      await control(url, "/model?hang=1");
      const going = async () => ["running", "queued"].includes((await transcription())?.state);
      await page.locator(".listhead button.search").click();
      // The speech is held, so the piece being heard is let on, one at
      // a time, until the transcription has stopped between two.
      for (let seen = await news(page), n = 0; (await going()) && n < 20; n++) {
        await control(url, "/speech?step=1");
        seen = await nextNews(page, seen);
      }
      await settle(page);
      if ((await transcription())?.state !== "interrupted") {
        walk.wrong = `a search asked for while the whole video is transcribed left the transcription ${(await transcription())?.state}, not stopped`;
        return;
      }
      if (await page.locator(".part:not(.going) .window.frame.locked").count()) {
        walk.wrong = "a search stopped the whole video, and the range picker still frames its part as work in hand";
        return;
      }
      await reopen();
      const during = await item();
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      if (!(await searching())) {
        walk.wrong = "the search ended before the menu was read, and this step is about the menu while it runs";
        return;
      }
      if (during.said !== "Transcribe video" || !during.off) {
        walk.wrong = `while the search runs, the menu says ${JSON.stringify(during)}, not Transcribe video that cannot be clicked`;
        return;
      }
      // Cancelled, the search ends, and the transcription stays stopped.
      await page.locator(".listhead button.search").click();
      await control(url, "/model");
      for (let seen = await news(page), n = 0; (await searching()) && n < 20; n++) {
        await control(url, "/speech?step=1");
        seen = await nextNews(page, seen);
      }
      await settle(page);
      if (await going()) {
        walk.wrong = "the search ended, and the whole video's transcription started again by itself";
        return;
      }
      await reopen();
      const after = await item();
      await page.keyboard.press("Escape");
      if (after.said !== "Transcribe video" || after.off) walk.wrong = `the search ended, the menu says ${JSON.stringify(after)}, not Transcribe video to click`;
      return;
    }
    if (arg === "stop at the end") {
      // Stopped while its last piece is being heard, the work still
      // hears to the end, and once it has ended nothing on the range
      // picker says a part is left: the dots stayed for good while the
      // menu offered Export, found by the second review of #458. The
      // speech is held and let on a piece at a time until what is left
      // is shorter than a piece, so the piece waiting is the last.
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      const left = () =>
        page.evaluate(() => {
          const parts = [...document.querySelectorAll(".unheard")].map((el) => Number(/round\(([\d.]+) \*/.exec(el.style.getPropertyValue("--at"))?.[1] ?? 0));
          return parts.length === 1 ? 1 - parts[0] : 1;
        });
      const { duration } = await ask(page, "Source", source);
      for (let n = 0; (await left()) * duration >= 25; n++) {
        if (n >= 200) {
          walk.wrong = "two hundred pieces heard, and the whole video still has more than a piece left";
          break;
        }
        const seen = await news(page);
        await control(url, "/speech?step=1");
        await nextNews(page, seen);
      }
      if (walk.wrong) return;
      await control(url, "/speech?hold=one");
      await row.hover();
      await row.locator("button.more").click();
      await menu.waitFor();
      const after = await pick("Stop transcribing");
      if (after.said !== "Stop transcribing") {
        walk.wrong = `the menu of a video being transcribed says ${JSON.stringify(after.said)}, not Stop transcribing`;
        await page.keyboard.press("Escape");
        return;
      }
      const read = await news(page, "episode");
      await control(url, "/speech?go");
      await nextNews(page, read, "episode");
      const runs = async () => (await ask(page, "Jobs")).some((j) => j.episode === source && j.kind === "transcribe" && (j.state === "running" || j.state === "queued"));
      for (let seen = await news(page); await runs(); ) seen = await nextNews(page, seen);
      await settle(page);
      if (!(await whole())) walk.wrong = "stopped on its last piece, the video was not heard to its end";
      else if (await page.locator(".unheard").count()) walk.wrong = "the video is transcribed to its end, and the range picker still shows dots on a part";
      return;
    }
    if (arg === "stop") {
      // The search before was cancelled, and the dots follow the whole
      // video again: they stood still under it until the next search,
      // found on #458. Twenty reports of the work, counted, are many
      // pieces of the audio, let on one at a time.
      const before = await page.evaluate(() => window.dotsNow());
      let moved = false;
      for (let seen = await news(page), n = 0; !moved && n < 20; n++) {
        await control(url, "/speech?step=1");
        seen = await nextNews(page, seen);
        moved = (await page.evaluate(() => window.dotsNow())) !== before;
      }
      if (!moved) {
        walk.wrong = "a search cancelled while the whole video is transcribed, the dots no longer follow the transcription";
        await page.keyboard.press("Escape");
        return;
      }
      // Then held with a piece of the audio in it, so the press comes
      // while that piece is being heard, the way Tim pressed it.
      await control(url, "/speech?hold=one");
      await settle(page);
      const held = await page.evaluate(() => window.dotsNow());
      const after = await pick("Stop transcribing");
      if (after.said !== "Stop transcribing") walk.wrong = `the menu of a video being transcribed says ${JSON.stringify(after.said)}, not Stop transcribing`;
      else if (after.open) walk.wrong = "the menu stayed open after Stop transcribing was clicked";
      // In the frame of the click the whole video's frame is going,
      // standing still while it fades, and not cut away, Tim's report
      // of 23 October.
      else if (after.framed) walk.wrong = "the frame after Stop transcribing was clicked, the range picker still frames the part as work in hand";
      else if (!after.going) walk.wrong = "the frame after Stop transcribing was clicked, the whole video's frame was cut away and does not fade";
      // The dot is the video's, and its first search may still hear, so
      // it is not looked at: the menu says what became of the click.
      if (walk.wrong) {
        await page.keyboard.press("Escape");
        return;
      }
      // At once, the menu offers it again, whether the work has ended
      // yet or not.
      await row.hover();
      await row.locator("button.more").click();
      await menu.waitFor();
      if ((await says()) !== "Transcribe video") walk.wrong = `the menu of a video told to stop says ${JSON.stringify(await says())}, not Transcribe video`;
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      if (walk.wrong) return;
      // Nothing moves on after the press: the piece heard to its end,
      // the job ended and the library read again as it ends, the dots
      // are where they were in the frame of the click. They went on
      // shrinking after the frame had gone, Tim's report of 23
      // October.
      const read = await news(page, "episode");
      await control(url, "/speech?go");
      await nextNews(page, read, "episode");
      const runs = async () => (await ask(page, "Jobs")).some((j) => j.episode === source && j.kind === "transcribe" && (j.state === "running" || j.state === "queued"));
      for (let seen = await news(page); await runs(); ) seen = await nextNews(page, seen);
      await settle(page);
      // Not one dot moves sideways either: centred again as the work
      // stopped, every dot jumped, Tim's report of 24 October.
      const dots = await page.evaluate(() => window.dotsNow());
      if (dots !== held) {
        walk.wrong = `after Stop transcribing the dots moved, from ${held} to ${dots}`;
      }
      return;
    }
    // Asked again each time the work says how far it heard, until the
    // Go side says the video is transcribed to its end, and the menu
    // opened again on it.
    if (!(await whole())) {
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      for (let seen = await news(page); !(await whole()); ) seen = await nextNews(page, seen);
      await settle(page);
      await row.hover();
      await row.locator("button.more").click();
      await menu.waitFor();
    }
    if (!(await says()).startsWith("Export transcript")) {
      walk.wrong = `the menu of a video transcribed to its end says ${JSON.stringify(await says())}, not Export transcript`;
      return;
    }
    const formats = page.locator(".formats");
    if (arg === "keys") {
      // With the keyboard alone, the way it works in a menu on the
      // Mac: Return on Export transcript hangs the formats out with
      // the keyboard on the first, the down arrow goes to the next and
      // Return picks it. Found by the review of #458, no key reached
      // them.
      await menu.locator("[data-export]").focus();
      await page.keyboard.press("Enter");
      await formats.waitFor();
      const focused = () => page.evaluate(() => (document.activeElement?.closest(".formats .item")?.textContent ?? "").replace(/\s+/g, " ").trim());
      if (!(await focused()).startsWith("Text")) {
        walk.wrong = `Return on Export transcript left the keyboard on ${JSON.stringify(await focused())}, not on Text`;
        await page.keyboard.press("Escape");
        return;
      }
      await page.keyboard.press("ArrowDown");
      if (!(await focused()).startsWith("Subtitles")) {
        walk.wrong = `the down arrow in the formats went to ${JSON.stringify(await focused())}, not to Subtitles`;
        await page.keyboard.press("Escape");
        return;
      }
      await page.keyboard.press("Enter");
      await menu.waitFor({ state: "detached" });
      await settle(page);
      const file = await control(url, "/exported");
      if (!file?.name.endsWith(" transcript.srt")) walk.wrong = `the formats picked with the keyboard saved ${JSON.stringify(file?.name)}`;
      return;
    }
    // On the words of the row, the way a hand comes to it, so the way
    // on to a format runs down across the row under it.
    await menu.locator(".item", { hasText: "Export transcript" }).hover({ position: { x: 40, y: 10 } });
    await formats.waitFor();
    const hangs = await page.evaluate(() => {
      const m = document.querySelector(".video-menu").getBoundingClientRect();
      const f = document.querySelector(".formats").getBoundingClientRect();
      const r = [...document.querySelectorAll(".video-menu .item")].find((el) => el.textContent.includes("Export transcript")).getBoundingClientRect();
      return { right: f.left >= m.right, top: Math.abs(f.top - r.top) <= 2 };
    });
    if (!hangs.right || !hangs.top) {
      walk.wrong = "the formats do not hang on the right of the menu, beside Export transcript";
      return;
    }
    if (arg === "click") {
      // The row the pointer hung the formats out from, clicked, keeps
      // them out, the way a submenu stays on the Mac. Found by the
      // review of #458, the click put them away.
      await menu.locator("[data-export]").click();
      const still = await page.evaluate(() => new Promise((done) => requestAnimationFrame(() => done(!!document.querySelector(".formats")))));
      if (!still) walk.wrong = "Export transcript clicked put its formats away";
      await page.keyboard.press("Escape");
      await menu.waitFor({ state: "detached" });
      return;
    }
    if (arg === "Escape") {
      // Escape closes the formats and the menu with them, the way it
      // closes a submenu on the Mac.
      await page.keyboard.press("Escape");
      await formats.waitFor({ state: "detached" });
      if (await menu.isVisible()) {
        walk.wrong = "Escape in the formats left the menu open";
        await page.keyboard.press("Escape");
      }
      await menu.waitFor({ state: "detached" });
      return;
    }
    // The hand goes to the format in a straight line, crossing the row
    // under Export transcript on its way, and the formats stay. They
    // closed under it, found by the second review of #458.
    const format = formats.locator(".item", { hasText: arg });
    const box = await format.boundingBox();
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2, { steps: 12 });
    if (!(await formats.count())) {
      walk.wrong = `the formats closed on the way from Export transcript to ${arg}`;
      await page.keyboard.press("Escape");
      return;
    }
    await format.click();
    await menu.waitFor({ state: "detached" });
    // The export has been answered, so the file is there. A long
    // video's took longer than the walk took to look.
    await settle(page);
    const file = await control(url, "/exported");
    const ending = { Text: ".txt", Subtitles: ".srt" }[arg];
    const name = `${basename(source, extname(source))} transcript${ending}`;
    if (file?.name !== name) walk.wrong = `the transcript was saved as ${JSON.stringify(file?.name)}, not ${JSON.stringify(name)}`;
    else if (!file.text.trim()) walk.wrong = "the transcript saved is empty";
    else if (arg === "Subtitles" && !file.text.startsWith("1\n00:00:")) walk.wrong = `the subtitles begin ${JSON.stringify(file.text.slice(0, 40))}`;
    else if (arg === "Text" && !/^\d+:\d\d\n/.test(file.text)) walk.wrong = `the text begins ${JSON.stringify(file.text.slice(0, 40))}, not with the time of its first paragraph`;
    return;
  },
  // ["exported", word] the file exported last holds the word
  async exported(walk, arg, s) {
    const file = await control(url, "/exported");
    if (!file?.text.split(/\s+/).some((w) => w.replace(/[.,!?]$/, "") === arg))
      walk.wrong = `the transcript exported does not hold ${JSON.stringify(arg)}`;
    return;
  },
  // ["add, the window read late", seconds]
  //                    adds a new video of so many seconds with Add, and
  //                    its workspace hears that no window was kept for it
  //                    only once its first search is seen, the order Tim's
  //                    Mac took on 24 October
  async "add, the window read late"(walk, arg, s) {
    const { page, watch } = walk;
    // A video added whose workspace hears that no window was kept for
    // it only once its first search is seen, the order Tim's Mac took
    // on 24 October. The call is held and then fails, which the
    // workspace takes as nothing kept: let through, it would find the
    // window the search had already kept. Nothing settles while the
    // call is held, so this step waits on the head of the clip list
    // instead, and settles once the call has answered.
    await control(url, "/hold?call=ChosenWindow&gate=shut&fail=nothing%20kept");
    await control(url, `/pick?seconds=${arg}`);
    await fromSidebar(page, () => page.locator("aside").getByText("Add", { exact: true }).first().click());
    await pageUntil(page, () => {
      const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
      return pane?.querySelector(".listhead button.search")?.textContent.replace(/\s+/g, " ").trim() === "Cancel";
    });
    await control(url, "/hold?call=ChosenWindow&gate=open");
    await settle(page);
    // The call failed on purpose, so the watch reads on from here.
    await watch.start();
    return;
  },
};

// Steps that take the same body as the step they are walked by.
steps["drop"] = steps["drop not a video"];
steps["drop again"] = steps["drop not a video"];

// The video's work folder, see WorkDir in engine/run.go.
const workDir = (source) => join(dirname(source), basename(source, extname(source)) + ".framefairy");
