// The steps of the sequences about searching: the clip list's rows of a
// search, the window, the language model and the speech model. Each is said
// above it, a list with its name first. walkSequence in ../sequences.mjs
// walks it, and hands it the walk so far, the step's argument, what the
// caption box showed before it and the step's name. A sequence of any
// folder can take it. Below them the helpers only these steps use. See
// docs/TESTING.md.
import { ask, caughtUp, clipList, control, drag, episodeOn, fakeClock, fromSidebar, news, nextNews, nextPaint, pageUntil, pressHead, realFrames, runClock, seenUntil, settle } from "../bridge.mjs";
import { clockOf, nextCall, onePiece, toastSays, url } from "../common/steps.mjs";

export const steps = {
  // ["model", how]     the language model "holds" its answers, "fails" or
  //                    "answers"
  async model(walk, arg) {
    const how = { holds: "hang=1&fail=0", fails: "hang=0&fail=1", answers: "hang=0&fail=0" }[arg];
    await control(url, `/model?${how}`);
  },
  // ["speech", "hold"] stops the speech model hearing, so a video added
  //                    is heard only as far as the walk lets it, and the
  //                    steps that say so let it on one piece at a time.
  //                    ["speech", "go"] lets it hear on by itself.
  //                    ["speech", "wordless"] hears no word at all, the way
  //                    a video with no speech is heard
  async speech(walk, arg) {
    await control(url, { wordless: "/speech?wordless=1", hold: "/speech?hold" }[arg] ?? "/speech?go");
    if (arg !== "wordless") walk.speechHeld = arg === "hold";
  },
  // ["search and cancel"] presses Search, and Cancel in the first frame the head
  //                    says it, and every frame from Search to that press the
  //                    head says Cancel and takes the click, never greyed
  //                    out
  async "search and cancel"(walk, arg, s) {
    const { page, watch } = walk;
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
    if (seen.pressed !== "Search") walk.wrong = `the head said ${seen.pressed}, not Search`;
    else if (seen.frames.some((f) => f.says !== "Cancel"))
      walk.wrong = `after Search the head said ${[...new Set(seen.frames.map((f) => f.says))].join(", ")}, not Cancel at once`;
    else if (greyed)
      walk.wrong = `the head said Cancel greyed out for ${greyed} of ${seen.frames.length} frames after Search, so a press there did nothing`;
    else if (seen.after !== "Cancelling") walk.wrong = `Cancel pressed, the head said ${seen.after}, not Cancelling`;
    console.log(`      Cancel pressed ${seen.frames.length} frames after Search, ${greyed} of them greyed out`);
    // Cancel has reached the Go side before the search held on its way
    // there, which the watch lets go of.
    await settle(page);
    await watch.step("head", s);
  },
  // ["hearing"]        a search still hears, so what came before was
  //                    about a search that had not come to the model yet
  async hearing(walk) {
    const { page } = walk;
    const jobs = await ask(page, "Jobs");
    if (!jobs.some((j) => j.kind === "search" && j.state === "running" && j.step === "hearing")) {
      walk.wrong = `no search hears now: ${JSON.stringify(jobs.map((j) => [j.kind, j.state, j.step]))}`;
    }
  },
  // ["wait for", word] the clip list's head button comes to say this, in a
  //                    minute at most
  async "wait for"(walk, arg, s) {
    const { page, watch } = walk;
    await pageUntil(
      page,
      (word) => {
        const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
        return pane?.querySelector(".listhead button.search")?.textContent.replace(/\s+/g, " ").trim() === word;
      },
      arg,
    );
    // The clip on screen may be a new one now, the first of a video
    // just added, and the watch follows it.
    await watch.step("wait", s);
  },
  // ["move window", handle, share]
  //                    drags the window's mark on the range picker, its
  //                    "to" or "from" triangle or its "bar", by this share
  //                    of the track's width, and remembers the episode's
  //                    last search and where the window was left
  async "move window"(walk, arg, s) {
    const { page, watch } = walk;
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
      walk.wrong = "the range picker shows no window to take hold of";
      return;
    }
    const path = await episodeOn(page);
    const searches = (await ask(page, "Jobs")).filter((j) => j.episode === path && j.kind === "search");
    await drag(page, box.x + box.width / 2, box.y + box.height / 2, share * track.width);
    const after = await mark();
    if (!after || (Math.abs(after.from - before.from) < 0.5 && Math.abs(after.to - before.to) < 0.5)) {
      walk.wrong = `the window did not move: ${JSON.stringify(before)} before, ${JSON.stringify(after)} after`;
      return;
    }
    walk.movedOff = { search: searches[searches.length - 1]?.id ?? "", window: after };
    await watch.step("move window", s);
  },
  // ["searched moved"] the episode's newest search is a new one, not the
  //                    search there was at "move window", and it searched
  //                    the window where "move window" left it
  async "searched moved"(walk) {
    const { page } = walk;
    const path = await episodeOn(page);
    const last = (await ask(page, "Jobs")).filter((j) => j.episode === path && j.kind === "search").pop();
    const w = walk.movedOff?.window;
    const to = last && last.to > 0 ? last.to : Infinity;
    if (!last || !w) walk.wrong = "there is no search, or no window was moved";
    else if (last.id === walk.movedOff.search) walk.wrong = `the search there was before the window moved was carried on, ${last.from}–${last.to} s`;
    else if (Math.abs((last.from ?? 0) - w.from) > 0.5 || Math.abs(to - w.to) > 0.5) {
      walk.wrong = `the search looked at ${last.from}–${last.to} s, the window was moved to ${w.from}–${w.to} s`;
    }
  },
  // ["said fails", "on"|"check"]
  async "said fails"(walk, arg, s) {
    const { page, watch } = walk;
    if (arg === "on") {
      await page.route("**/call", (route) => {
        const body = route.request().postDataJSON?.() ?? {};
        if (String(body.name ?? "").endsWith("Sentences")) return route.fulfill({ status: 500, body: "the disk went away" });
        return route.fallback();
      });
      return;
    }
    const from = await page.evaluate(() => window.__calls.length);
    await page.locator('.listhead button[aria-label="Search the transcript"]').click();
    // Once the read of the cards has failed and the list has had its
    // frames to say so.
    await nextCall(page, "Sentences", from);
    await settle(page);
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
    const again = await page.evaluate(() => ({
      failed: !!document.querySelector("ol.said li.failed"),
      cards: document.querySelectorAll("ol.said li[data-at]").length,
    }));
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    if (!failing) walk.wrong = "the cards could not be read and the list said nothing of it";
    else if (failing.says !== "The transcript could not be read" || !failing.why.includes("the disk went away"))
      walk.wrong = `the list said "${failing.says}", with "${failing.why}" under the pointer`;
    else if (again.failed || !again.cards) walk.wrong = `read again, the list still says it could not be read, over ${again.cards} cards`;
  },
  // ["search said", word]
  // ["search said", [word, n]]
  //                    presses the transcript button in the head of the clip column
  //                    and types the word, and sees the field take the
  //                    head's place at its height with the keyboard in it,
  //                    a card for every sentence the engine has that holds
  //                    the word and for no sentence it does not have, the
  //                    word bold in each, the clip a sentence is in named
  //                    on its card, a dashed card and a dashed part of the
  //                    range picker for every part not transcribed yet, at
  //                    least n of them, sentences in the first frame after
  //                    the press, a card's edge and its clip's mark lit
  //                    under the hand, a part not transcribed lit on the
  //                    range picker under the hand, a clip chosen there
  //                    bringing its sentence into view, a click on a card
  //                    putting the playhead on its sentence, and Escape,
  //                    with the keyboard on that card, giving the head and
  //                    the clips back. A word of "*" is the longest word
  //                    of the first sentence
  async "search said"(walk, arg, s) {
    const { page, settled, watch } = walk;
    const path = await episodeOn(page);
    const headHigh = () => page.evaluate(() => document.querySelector(".listhead").getBoundingClientRect().height);
    // The clips are on their cards first, so a sentence can be told
    // which clip it is in.
    const listed = (await ask(page, "Clips", path)).filter((c) => !c.rejected).length;
    await pageUntil(page, (n) => document.querySelectorAll("aside ol li[data-key] button.pick").length >= n, listed);
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
      walk.wrong = `the column stood empty for ${blank} of 20 frames after the transcript button`;
      await page.keyboard.press("Escape");
      return;
    }
    // The cards open at the playhead, not where the clip list was
    // scrolled to, found in the review of #367. Once the cards read
    // as it opens have come.
    await settle(page);
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
      walk.wrong = `the cards opened at no card of the playhead, ${await page.evaluate(() => JSON.stringify({ f: document.querySelector("ol.said").dataset.followed ?? null, at: document.querySelector(".screen")?.dataset.playhead, n: document.querySelectorAll("ol.said li").length }))}`;
      await page.keyboard.press("Escape");
      return;
    }
    // In view, the way the clip timeline shows the playhead's page.
    if (!opened.seen) {
      walk.wrong = `the cards opened with the card of the playhead ${opened.top.toFixed(0)} px down, out of view`;
      await page.keyboard.press("Escape");
      return;
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
    const typedTop = await page.evaluate(() => {
      const list = document.querySelector(".scroll.list");
      return { top: list.scrollTop, more: list.scrollHeight - list.clientHeight };
    });
    await page.keyboard.press("Backspace");
    await page.keyboard.type(word);
    await watch.step("type", s);
    if (scrolledTo > 0 && typedTop.more > 0 && typedTop.top !== 0) {
      walk.wrong = `typing in the search left the list ${typedTop.top} px down, where the hand had scrolled it to ${scrolledTo} px`;
      await page.keyboard.press("Escape");
      await page.keyboard.press("Escape");
      return;
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
    await settle(page);
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
      edges: document.querySelectorAll(".track .unheard").length,
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
    if (seen.focused !== "Search transcript") walk.wrong = `the keyboard is in ${seen.focused ?? "nothing"}, not the search field`;
    else if (seen.head !== was) walk.wrong = `the head is ${seen.head} px high with the field, ${was} px without it`;
    else if (!holds.length) walk.wrong = `the engine has no sentence with ${word}, so the step proves nothing`;
    else if (holds.some((x) => !seen.cards.some((c) => Math.abs(c.at - x.start) < 0.0005)))
      walk.wrong = `no card for "${holds.find((x) => !seen.cards.some((c) => Math.abs(c.at - x.start) < 0.0005)).text}"`;
    // Only the cards that hold the word, where any does: three letters
    // answer a lot of words the way a font answers its name, found in
    // the review of #367.
    else if (seen.cards.length !== holds.length) walk.wrong = `${seen.cards.length} cards answer ${word}, and ${holds.length} hold it`;
    else if (seen.cards.some((c) => !texts.has(c.text))) walk.wrong = `a card reads "${seen.cards.find((c) => !texts.has(c.text)).text}", which the engine never said`;
    else if (seen.cards.some((c) => !c.bold.length)) walk.wrong = `"${seen.cards.find((c) => !c.bold.length).text}" has no word in bold`;
    else if (seen.cards.some((c) => c.text.toLowerCase().includes(word.toLowerCase()) && !c.bold.some((b) => b.toLowerCase().includes(word.toLowerCase()))))
      walk.wrong = `${word} is not bold in a card that holds it`;
    else if (seen.cards.some((c) => !sentences.some((y) => Math.abs(y.start - c.at) < 0.0005 && y.text === c.text)))
      walk.wrong = `the card at ${seen.cards.find((c) => !sentences.some((y) => Math.abs(y.start - c.at) < 0.0005)).at} is of no sentence the engine has there`;
    else if (sentences.some((x) => x.end - x.start > 12))
      walk.wrong = `a card of what was said runs ${(() => { const x = sentences.find((y) => y.end - y.start > 12); return `${(x.end - x.start).toFixed(1)} s from ${x.start.toFixed(1)}`; })()}, more than the 12 s a card may, found by Tim`;
    else if (seen.cards.some(spills))
      walk.wrong = `the card at ${seen.cards.find(spills).at.toFixed(2)} is named for its clip and holds what the clip leaves out`;
    else if (seen.cards.some(misnamed)) {
      const c = seen.cards.find(misnamed);
      walk.wrong = `the card at ${c.at.toFixed(2)} names ${c.clip || "no clip"}, and the engine has it in ${namesAt(c).join(", ") || "no clip"}`;
    }
    else if (unlike) walk.wrong = unlike;
    // What no clip keeps reads in a label's grey, a clip's words a step
    // above it and below white, and a word that answers in white, so it
    // stands out, found by Tim.
    else if (seen.grey.out && seen.grey.out !== seen.grey.muted) walk.wrong = `what no clip keeps reads in ${seen.grey.out}, not the grey of a label, ${seen.grey.muted}`;
    else if (seen.grey.clip && !(bright(seen.grey.clip) > bright(seen.grey.muted) && bright(seen.grey.clip) < bright(seen.grey.white)))
      walk.wrong = `a clip's words read in ${seen.grey.clip}, not between ${seen.grey.muted} and ${seen.grey.white}`;
    else if (seen.grey.bold !== seen.grey.white) walk.wrong = `a word that answers reads in ${seen.grey.bold}, not ${seen.grey.white}`;
    // The field says what it is in two words, darker than what is typed
    // and than a label, so it does not read as typed, and the button
    // that opens it wears the transcript, found by Tim.
    else if (seen.says !== "Search transcript") walk.wrong = `the field says "${seen.says}"`;
    else if (seen.grey.placeholder !== seen.grey.unwritten || !(bright(seen.grey.placeholder) < bright(seen.grey.muted)))
      walk.wrong = `the field's placeholder is ${seen.grey.placeholder}, not ${seen.grey.unwritten}, darker than ${seen.grey.muted}`;
    else if (!icon.includes("lucide-message-square-quote")) walk.wrong = `the button that opens the search wears ${icon || "nothing"}`;
    else if (!held && clips.some((k) => !k.rejected)) walk.wrong = "no clip has a card, so a clip's cards could not be held to its captions";
    else if (gaps < unheardLeast)
      walk.wrong = `${gaps} parts are not transcribed yet, fewer than the ${unheardLeast} this step is about: heard ${JSON.stringify(heard)} of ${end}, transcribed ${status?.transcribed}`;
    // A search answers with sentences only. The parts not transcribed
    // yet stay on the range picker, found by Tim.
    else if (seen.holes || seen.edges !== gaps)
      walk.wrong = `${gaps} parts are not transcribed yet, and with ${word} typed the list shows ${seen.holes} of them and the range picker ${seen.edges}`;
    else if (seen.count !== String(seen.cards.length)) walk.wrong = `the field says ${seen.count ?? "nothing"} match, over ${seen.cards.length} cards`;
    else if (!scrolledTo || typedTop.more <= 0)
      walk.wrong = `the sentences with an e scroll ${typedTop.more} px and the hand scrolled to ${scrolledTo} px, so typing could not be seen to go back to the top`;
    if (walk.wrong) {
      await page.keyboard.press("Escape");
      await page.keyboard.press("Escape");
      return;
    }
    // What nothing said matches says so in two words, with no parts
    // not transcribed yet around them, found by Tim.
    const field = page.locator('input[aria-label="Search transcript"]');
    await field.fill("qxzvqxzv");
    await watch.step("type", s);
    const none = await page.evaluate(() => ({
      says: document.querySelector("ol.said li.none")?.textContent.trim() ?? null,
      rows: document.querySelectorAll("ol.said li:not(.none)").length,
      count: document.querySelector(".listhead .search .saycount")?.textContent.trim() ?? null,
    }));
    if (none.says !== "No matches" || none.rows || none.count !== "0")
      walk.wrong = `nonsense typed shows ${none.rows} cards over "${none.says}", and the field says ${none.count}`;
    // Three letters a few words hold, and more words have in that order
    // the way a font has its name, sechs and verstaucht for sch: only
    // the cards that hold them, found in the review of #367.
    await field.fill("sch");
    await watch.step("type", s);
    const short = await page.evaluate(() => document.querySelectorAll("ol.said li[data-at]").length);
    const holdShort = sentences.filter((x) => x.text.toLowerCase().includes("sch")).length;
    if (!walk.wrong && holdShort && short !== holdShort) walk.wrong = `${short} cards answer sch, and ${holdShort} hold it`;
    // The word typed with a letter twice, the way Tim typed arrm for
    // arm, still finds every card that holds the word.
    const slip = word.slice(0, 2) + word.slice(1);
    await field.fill(slip);
    await watch.step("type", s);
    const slipped = await page.evaluate(() => [...document.querySelectorAll("ol.said li[data-at]")].map((li) => Number(li.dataset.at)));
    const missed = holds.find((x) => !slipped.some((at) => Math.abs(at - x.start) < 0.0005));
    if (!walk.wrong && missed) walk.wrong = `${slip} typed for ${word} finds ${slipped.length} cards and not "${missed.text}"`;
    // With nothing typed, the whole of what was said, and a dashed card
    // for every part not transcribed yet.
    await field.fill("");
    await watch.step("type", s);
    const whole = await page.evaluate(() => ({
      holes: document.querySelectorAll("ol.said li.hole").length,
      count: document.querySelector(".listhead .search .saycount")?.textContent.trim() ?? null,
    }));
    if (!walk.wrong && whole.holes !== gaps) walk.wrong = `${gaps} parts are not transcribed yet, and with nothing typed the list shows ${whole.holes}`;
    else if (!walk.wrong && whole.count !== null) walk.wrong = `with nothing typed the field still says ${whole.count} match`;
    if (walk.wrong) {
      await page.keyboard.press("Escape");
      return;
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
          edge: getComputedStyle(li.querySelector("button.pick"), "::before").width,
          mark: !!document.querySelector(".track .clipmark.lit"),
          others: others.length,
          alsoLit: others.filter((x) => getComputedStyle(x.querySelector("button.pick")).backgroundColor === grey).length,
          key,
        };
      }, inClip.at);
      if (lit.edge !== "4px") walk.wrong = `the edge of the card at ${inClip.at.toFixed(2)} is gone under the hand: ${lit.edge}`;
      else if (!lit.mark) walk.wrong = `the hand on a sentence of ${inClip.clip} lit no mark on the range picker`;
      else if (lit.alsoLit) walk.wrong = `the hand on one sentence of ${inClip.clip} lit ${lit.alsoLit} of its ${lit.others} other sentences too`;
    }
    // The hand going from one card of a clip to the next is on a card
    // all the way, so the clip stays lit everywhere else and does not
    // flicker, found by Tim.
    if (!walk.wrong && clips.some((k) => !k.rejected)) {
      await field.fill("");
      await watch.step("type", s);
      const pair = await page.evaluate(() => {
        const cards = [...document.querySelectorAll("ol.said li[data-clip]")];
        const i = cards.findIndex((li, i) => li.nextElementSibling === cards[i + 1] && cards[i + 1]?.dataset.clip === li.dataset.clip);
        if (i < 0) return null;
        cards[i].scrollIntoView({ block: "center" });
        const [a, b] = [cards[i].getBoundingClientRect(), cards[i + 1].getBoundingClientRect()];
        return { x: a.left + a.width / 2, from: a.top + a.height / 2, to: b.top + b.height / 2 };
      });
      if (!pair) walk.wrong = "no clip has two cards one after the other, so going from one to the next could not be seen";
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
          walk.wrong = `the hand going from one card of a clip to the next was on no card for ${off} px and the clip's mark went dark for ${dark} px`;
      }
    }
    // The hand on a part not transcribed yet lights it on the range
    // picker too.
    if (!walk.wrong && gaps) {
      await page.locator("ol.said li.hole").first().hover();
      await watch.step("hover", s);
      // Lit the way every card is under the hand, the dashes gone into
      // the grey they fill with, in the list and on the range picker,
      // and dashed in the grey of a card's edge until then, found by
      // Tim.
      const look = await page.evaluate(() => {
        const hole = getComputedStyle(document.querySelector("ol.said li.hole"));
        const fill = document.querySelector(".track .unheard.lit");
        const ring = getComputedStyle(document.querySelector("ol.said li:not(.hole):not(.none)")).boxShadow;
        const dots = document.querySelector("ol.said li.hole .dots");
        const dashes = document.querySelector("ol.said li.hole .ring");
        return {
          lit: !!fill,
          dots: [dots && getComputedStyle(dots), fill && getComputedStyle(fill, "::before")].map((d) => (d ? [d.backgroundImage, d.maskImage, d.top] : null)),
          card: [dashes ? getComputedStyle(dashes).borderTopColor : null, hole.backgroundColor],
          // The range picker draws no edge round a part, only its dots,
          // found by Tim, and its own border stays over the part lit.
          picker: fill
            ? [fill, ...[...fill.children]].flatMap((el) => [getComputedStyle(el), getComputedStyle(el, "::before"), getComputedStyle(el, "::after")]).map((st) => st.borderTopStyle).filter((b) => b !== "none")
            : ["no part lit"],
          border: getComputedStyle(document.querySelector(".track")).borderTopColor,
          edges: document.querySelectorAll(".unheard-edge").length,
          ring,
        };
      });
      await page.mouse.move(0, 0);
      // The other way round, the hand on the part on the range picker
      // lights its card, the way a clip's mark lights its card, found
      // by Tim.
      const onPart = await page.evaluate(() => {
        const parts = [...document.querySelectorAll(".track .unheard")];
        const r = parts.at(-1).getBoundingClientRect();
        return { x: r.left + r.width / 2, y: r.top + r.height * 0.75, index: parts.length - 1 };
      });
      await page.mouse.move(onPart.x, onPart.y);
      await watch.step("hover", s);
      const fromPart = await page.evaluate(() => ({
        cards: [...document.querySelectorAll("ol.said li.hole")].map((li) => li.classList.contains("lit")),
        part: [...document.querySelectorAll(".track .unheard")].map((u) => u.classList.contains("lit")),
      }));
      await page.mouse.move(0, 0);
      const lightsCard = fromPart.cards[onPart.index] && fromPart.cards.filter(Boolean).length === 1 && fromPart.part[onPart.index];
      const rest = await page.evaluate(() => {
        const ring = document.querySelector("ol.said li.hole .ring");
        return ring ? getComputedStyle(ring).borderTopColor : "no dashes";
      });
      // The dashes of a card stand over its columns of dots and beside
      // its rows: the dots are whole columns and rows of one pitch,
      // centred in the card with room on every side, and the dashes
      // run at that pitch over exactly their span. The browser's own
      // dashes went their own way, dots on to the edge sat too close to
      // the dashes, and a row and a column left clear looked cut off,
      // all found by Tim. On the range picker the dots are whole
      // columns centred in the part.
      const grid = await page.evaluate(() => {
        const li = document.querySelector("ol.said li.hole");
        const dots = li.querySelector(".dots");
        const ring = li.querySelector(".ring");
        const box = (el) => {
          const r = el?.getBoundingClientRect();
          return r ? [r.left, r.top, r.width, r.height].map((v) => Math.round(v * 100) / 100).join(" ") : null;
        };
        const d = getComputedStyle(dots);
        const r = ring ? getComputedStyle(ring) : null;
        const round = (v) => Math.round(v * 100) / 100;
        const c = li.getBoundingClientRect();
        const o = dots.getBoundingClientRect();
        // The room left round the dots, left, right, top and foot.
        const room = [o.left - c.left, c.right - o.right, o.top - c.top, c.bottom - o.bottom].map(round);
        const px = (t) => (t ?? "").match(/-?[\d.]+px/g)?.map(parseFloat) ?? [];
        const pitch = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--dot-pitch"));
        const track = document.querySelector(".track");
        const pad = parseFloat(getComputedStyle(track).borderLeftWidth);
        const columns = [...track.querySelectorAll(".unheard")].map((u) => {
          // A part the work is hearing into has its dots laid from
          // where its hearing stops, up to its left edge, see the walk
          // of the parts giving way. What it is once the work stops is read with
          // that taken off.
          const giving = u.classList.contains("giving");
          const g = getComputedStyle(u, "::before");
          const laid = giving ? [parseFloat(g.left), parseFloat(g.right), g.backgroundPositionX] : null;
          u.classList.remove("giving");
          const b = getComputedStyle(u, "::before");
          const part = u.getBoundingClientRect();
          const left = parseFloat(b.left);
          const width = parseFloat(b.width);
          const at = {
            // What is left either side of the columns of dots, and how
            // many columns there are.
            sides: [left, part.width - left - width].map((v) => Math.round(v * 100) / 100),
            columns: width / pitch,
            height: [parseFloat(b.top), parseFloat(b.height) - part.height],
            foot: b.backgroundPositionY,
            fade: b.maskImage,
            laid,
            worked: u.classList.contains("worked"),
          };
          if (giving) u.classList.add("giving");
          return at;
        });
        return {
          pitch,
          boxes: [box(ring), box(li)],
          room,
          span: [round(o.width), round(o.height)],
          dots: [d.backgroundSize, d.backgroundRepeat, d.maskImage],
          dashes: r ? [r.maskSize, r.maskRepeat, r.borderTopStyle, r.maskPosition] : null,
          laid: r ? [...px(r.maskSize), ...px(r.maskPosition)].map(round) : [],
          columns,
        };
      });
      const tile = `${grid.pitch}px ${grid.pitch}px`;
      // Halftone dots fill both, on nothing else, rising from the foot
      // with none in the top third, where the minutes stand, at rest and
      // under the hand alike, and never brighter than the dashes or the
      // minute lines, found by Tim.
      const still = await page.evaluate(() =>
        [
          document.querySelector("ol.said li.hole:not(.current) .dots") ?? document.querySelector("ol.said li.hole .dots"),
          document.querySelector(".track .unheard:not(.lit)") ?? document.querySelector(".track .unheard"),
        ].map((d, i) => {
          if (!d) return null;
          // The card the playhead is in is marked as such, see below,
          // and its part on the range picker is lit with it, so both
          // at rest are read with the mark taken off.
          const on = i ? d : d.closest("li");
          const mark = i ? "lit" : "current";
          const had = on.classList.contains(mark);
          on.classList.remove(mark);
          const st = i ? getComputedStyle(d, "::before") : getComputedStyle(d);
          const look = [st.backgroundImage, st.maskImage, st.top];
          const under = getComputedStyle(on).backgroundColor;
          if (had) on.classList.add(mark);
          return [...look, under];
        }),
      );
      const lines = await page.evaluate(() => [getComputedStyle(document.querySelector(".track .tick")).borderLeftColor, getComputedStyle(document.querySelector("ol.said li.hole .ring") ?? document.querySelector("ol.said li.hole")).borderTopColor]);
      const lum = (c) => (c.match(/[\d.]+/g) ?? []).slice(0, 3).reduce((a, v) => a + Number(v), 0);
      // A dot's colour, the grey it is drawn in as rgb, and the share
      // of its strength apart.
      const dotColour = (d) => {
        // The first colour of the gradient is the dot's, the second the
        // clear round it.
        const c = d[0].match(/rgba?\(([^)]*)\)|color\(srgb ([\d.]+) ([\d.]+) ([\d.]+)(?: \/ ([\d.]+))?\)/);
        if (!c) return ["", 1];
        if (c[1] !== undefined) {
          const [r, g, b, a = 1] = c[1].split(",").map(Number);
          return [`rgb(${r}, ${g}, ${b})`, a];
        }
        const [r, g, b] = c.slice(2, 5).map((v) => Math.round(Number(v) * 255));
        return [`rgb(${r}, ${g}, ${b})`, c[5] === undefined ? 1 : Number(c[5])];
      };
      const dotted = (d) => !!d && d[0].includes("radial-gradient");
      const fade = (d) => d[1].startsWith("linear-gradient(rgba(0, 0, 0, 0) 30%");
      if (!look.lit) walk.wrong = "the hand on a part not transcribed yet lit nothing on the range picker";
      else if (!lightsCard) walk.wrong = `the hand on a part not transcribed yet on the range picker lit cards ${JSON.stringify(fromPart.cards)} and parts ${JSON.stringify(fromPart.part)}, not that part and its card`;
      else if (!still.every(dotted) || !look.dots.every(dotted))
        walk.wrong = `a part not transcribed yet is not dotted: at rest ${JSON.stringify(still)}, under the hand ${JSON.stringify(look.dots)}`;
      else if (!still.every(fade) || !look.dots.every(fade))
        walk.wrong = `the dots of a part not transcribed yet do not rise from the foot: at rest ${still.map((d) => d[1]).join(" and ")}, under the hand ${look.dots.map((d) => d[1]).join(" and ")}`;
      else if (still.some((d) => d[3] !== "rgba(0, 0, 0, 0)")) walk.wrong = `a part not transcribed yet at rest is on ${still.map((d) => d[3]).join(" and ")}, not on nothing but its dots`;
      else if (lum(dotColour(still[1])[0]) > lum(lines[0]) || lum(dotColour(still[1])[0]) > lum(lines[1]))
        walk.wrong = `the dots, ${dotColour(still[1])[0]}, are brighter than the minute lines, ${lines[0]}, or the dashes round them, ${lines[1]}`;
      // And a share of that grey's strength, so they are quieter than
      // the dashes round them: at full strength Tim found them too
      // strong.
      else if ([...still, ...look.dots].some((d) => !(dotColour(d)[1] > 0 && dotColour(d)[1] < 1)))
        walk.wrong = `the dots are drawn at full strength, ${[...still, ...look.dots].map((d) => dotColour(d)[1]).join(", ")}, not quieter than the dashes`;
      else if (look.card[0] !== look.card[1]) walk.wrong = `the dashed card under the hand has its edge in ${look.card[0]} over ${look.card[1]}, not gone into it`;
      else if (look.picker.length || look.edges || look.border === "rgba(0, 0, 0, 0)")
        walk.wrong = `a part on the range picker, lit, has an edge of its own, ${[...look.picker, ...(look.edges ? [`${look.edges} edges`] : [])].join(" and ")}, over a border in ${look.border}`;
      else if (!look.ring.includes(rest)) walk.wrong = `the dashes are ${rest} on the card, and a card's edge is ${look.ring}`;
      else if (!grid.dashes || grid.dashes[2] !== "solid" || grid.boxes[0] !== grid.boxes[1])
        walk.wrong = `the dashes of a card are not on its edge: dashes ${JSON.stringify(grid.dashes)}, boxes of the dashes and the card ${JSON.stringify(grid.boxes)}`;
      else if (grid.dots[0] !== tile || grid.dots[1] !== "repeat" || grid.span.some((v) => Math.abs(v / grid.pitch - Math.round(v / grid.pitch)) > 0.01))
        walk.wrong = `the dots of a card are not whole columns and rows: ${grid.span.join(" by ")} px in tiles of ${grid.dots[0]}, ${grid.dots[1]}`;
      // Room on every side, the same either way, found by Tim.
      else if (grid.room[0] !== grid.room[1] || grid.room[2] !== grid.room[3] || grid.room.some((v) => v < 2 || v >= 2 + grid.pitch / 2))
        walk.wrong = `the dots of a card are not centred in it with room round them: ${grid.room.join(", ")} px left, right, top and foot`;
      // The dashes over exactly the dots' span, so each stands over a
      // column or beside a row and none falls outside.
      else if (
        !grid.dashes[1].split(", ").every((r) => r === "no-repeat") ||
        grid.laid.join(" ") !== [grid.span[0], grid.span[1], grid.room[0], 0, 0, grid.room[2]].join(" ")
      )
        walk.wrong = `the dashes of a card do not stand on its dots: dots ${grid.span.join(" by ")} px at ${grid.room[0]} and ${grid.room[2]} px in, dashes laid ${grid.dashes[0]} at ${grid.dashes[3]}, ${grid.dashes[1]}`;
      else if (grid.dots[2] !== grid.columns[0]?.fade)
        walk.wrong = `the dots of a card leave a row or a column clear round them: masked by ${grid.dots[2]}`;
      else if (
        grid.columns.some(
          (c) => c.height.some(Boolean) || !c.fade.endsWith("rgb(0, 0, 0))") || !Number.isInteger(c.columns) || c.sides.some((v) => v < 0 || v >= (c.worked ? grid.pitch : grid.pitch / 2)),
        )
      )
        walk.wrong = `the dots of a part on the range picker leave a row or a column clear round them: ${JSON.stringify(grid.columns)}`;
      // Centred, the same either side, found by Tim, who measured it.
      // A part the work heard into keeps its dots where they stood,
      // whole columns laid from where its hearing stops, so none jumps
      // sideways as the work stops, Tim's report of 24 October, see
      // the walk of the dots after a search's window.
      else if (grid.columns.some((c) => !c.worked && c.sides[0] !== c.sides[1]))
        walk.wrong = `the dots of a part on the range picker are not centred in it: ${JSON.stringify(grid.columns.map((c) => [c.worked, c.sides]))}, worked and left and right`;
      // While the work hears into a part, its edge wipes the dots, so
      // they reach its left edge, and end on a whole column.
      else if (grid.columns.some((c) => c.laid && (c.laid[0] !== 0 || !(c.laid[1] >= 0 && c.laid[1] < grid.pitch) || !/px$/.test(c.laid[2]))))
        walk.wrong = `the dots of a part the work hears into do not reach its left edge as whole columns: ${JSON.stringify(grid.columns.map((c) => c.laid))}`;
      // And the foot row a pixel up from the range picker's foot,
      // where it looked clutched to it, found by Tim.
      else if (grid.columns.some((c) => c.foot !== "calc(100% - 1px)"))
        walk.wrong = `the foot row of dots on the range picker is not lifted a pixel: ${grid.columns.map((c) => c.foot).join(", ")}`;
    }
    // The playhead put in a part not transcribed yet, on the range
    // picker, puts that part's card at the top of the list, not the
    // next words heard after it, found by Tim.
    if (!walk.wrong && gaps) {
      await page.evaluate(() => (document.querySelector(".scroll.list").style.overflowAnchor = "none"));
      const part = await page.evaluate(() => {
        const r = [...document.querySelectorAll(".track .unheard")].at(-1).getBoundingClientRect();
        return { x: r.left + r.width / 2, y: r.top + r.height / 2 };
      });
      await page.mouse.click(part.x, part.y);
      await watch.step("click", s);
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
        const hole = [...document.querySelectorAll("ol.said li.hole")].find((li) => li.dataset.row === document.querySelector("ol.said").dataset.followed);
        const r = hole?.getBoundingClientRect();
        return {
          at: Number(document.querySelector(".screen")?.dataset.playhead),
          hole:
            !!hole?.classList.contains("hole") &&
            !!r &&
            r.top >= box.top &&
            r.bottom <= box.bottom &&
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
      if (!hole) walk.wrong = `a press in the last part not transcribed yet put the playhead at ${there.at}, which is heard`;
      else if (!there.hole) walk.wrong = `the playhead in a part not transcribed yet, at ${there.at.toFixed(1)}, left that part's card out of view, ${there.top ?? "no card"} at the top of the list`;
      if (!walk.wrong) {
        // The hand scrolls the card away and the playhead is put in the
        // same part again: the card comes back into view, the way a clip
        // chosen again does, though the playhead never left its card.
        await page.evaluate(() => {
          const list = document.querySelector(".scroll.list");
          const hole = [...document.querySelectorAll("ol.said li.hole")].find((li) => li.dataset.row === document.querySelector("ol.said").dataset.followed);
          const box = list.getBoundingClientRect();
          list.scrollTop = hole.getBoundingClientRect().top - box.top > box.height / 2 ? 0 : list.scrollHeight;
        });
        await nextPaint(page, 2);
        const again = await page.evaluate(() => {
          const r = [...document.querySelectorAll(".track .unheard")].at(-1).getBoundingClientRect();
          return { x: r.left + r.width / 4, y: r.top + r.height / 2 };
        });
        await page.mouse.click(again.x, again.y);
        await watch.step("click", s);
        const back = await page.evaluate(() => {
          const list = document.querySelector(".scroll.list");
          const box = list.getBoundingClientRect();
          const hole = [...document.querySelectorAll("ol.said li.hole")].find((li) => li.dataset.row === document.querySelector("ol.said").dataset.followed);
          const r = hole?.getBoundingClientRect();
          return { at: Number(document.querySelector(".screen")?.dataset.playhead), shown: !!r && r.top >= box.top - 0.5 && r.bottom <= box.bottom + 0.5 };
        });
        if (!back.shown) walk.wrong = `the playhead put again in the same part not transcribed yet, at ${back.at.toFixed(1)}, after the hand scrolled its card away, left the card out of view`;
      }
    }
    // A clip chosen on the range picker brings its first sentence into
    // view, found by Tim.
    const others = clips.filter((c) => !c.rejected);
    if (!walk.wrong && others.length > 1) {
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
                done(r ? { top: r.top - box.top, should: parseFloat(getComputedStyle(head).paddingTop), room: box.height, seen: r.top >= box.top && r.bottom <= box.bottom } : null);
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
      // In view where it was in view, and at the top where the page
      // had to turn, after the hand scrolled away.
      const off = chosen.find((v, i) => v && (i ? Math.abs(v.top - v.should) > 1 : !v.seen));
      if (!target || chosen.some((v) => !v))
        walk.wrong = `the mark ${label} is of no clip with sentences on screen`;
      else if (off)
        walk.wrong = `choosing ${target.title} on the range picker${off === chosen[1] ? " a second time" : ""} left its first sentence ${off.top.toFixed(1)} px down a list ${off.room.toFixed(0)} px high, ${off === chosen[1] ? `not at its top, ${off.should} px` : "out of view"}`;
    }
    // The playhead put on a caption on the clip timeline puts the card
    // of that caption at the top of the list, the way a clip chosen
    // does, found by Tim.
    if (!walk.wrong && clips.some((k) => !k.rejected)) {
      await field.fill("");
      await watch.step("type", s);
      // The chosen clip's mark, so the clip timeline is on the clip and
      // not where the playhead was put before the search opened.
      await page.evaluate(() => document.querySelector(".track .clipmark.selected").click());
      await watch.step("click", s);
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
      if (n < 2 || shown < 1) walk.wrong = `the chosen clip has ${n} captions on the clip timeline, ${shown + 1} of them in view, so going to one could not be seen}`;
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
        if (!card) walk.wrong = `no card after the playhead at ${stood.at}`;
        else if (stood.top === null || Math.abs(stood.top - card.start) > 0.0005)
          walk.wrong = `the playhead put on a caption at ${stood.at.toFixed(2)} left ${stood.top === null ? "no card" : `the card at ${stood.top.toFixed(2)}`} at the top of the list, not the card at ${card.start.toFixed(2)}, "${card.text}"`;
        else if (!card.clip) walk.wrong = `the caption at ${stood.at.toFixed(2)} on the clip timeline is in a card of no clip`;
      }
    }
    // The clip played from its mark: its first caption stands at the
    // top, and as the playhead comes into the next caption, that card
    // stands at the top, found by Tim.
    if (!walk.wrong && clips.some((k) => !k.rejected)) {
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
          return { top: top?.dataset.row ?? null, followed, clip: li?.dataset.clip ?? null, standing: !!followed && !!shown, end };
        });
      // A search let go of without moving the playhead goes back to the
      // card of the playhead, by Escape or by taking the words away,
      // not to the top of the whole transcript, found by Tim.
      if (own.length >= 2) {
        for (const how of ["Escape", "erase"]) {
          const before = await topRow();
          await field.fill(word);
          await watch.step("type", s);
          if (how === "Escape") await field.press("Escape");
          else await field.fill("");
          await watch.step(how === "Escape" ? "press" : "type", s);
          const after = await topRow();
          if (!walk.wrong && (!before.standing || before.clip !== chosenKey))
            walk.wrong = `the clip chosen on the range picker left ${before.top} at the top, not its card ${before.followed}`;
          else if (!walk.wrong && (!after.standing || after.followed !== before.followed))
            walk.wrong = `a search let go of by ${how} left ${after.top} at the top of the list, not ${before.followed}, the card of the playhead it stood at before`;
        }
      }
      if (own.length < 2) walk.wrong = `the chosen clip has ${own.length} cards, so playing into its next one could not be seen`;
      else if (!walk.wrong) {
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
        const started = await page.evaluate(() => ({
          typed: document.querySelector(".listhead .search input")?.value ?? null,
          focused: document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.tagName ?? null,
          playing: document.querySelector('button[aria-label="Pause"]') !== null,
          at: Number(document.querySelector(".screen")?.dataset.playhead),
        }));
        const first = await topRow();
        if (started.typed !== "") walk.wrong = `the space bar in the empty search field typed "${started.typed}"`;
        else if (started.focused === "Search transcript") walk.wrong = "the space bar in the empty search field left the keyboard in it";
        else if (!started.playing) walk.wrong = "the space bar in the empty search field did not play";
        else if (started.at < own[1].start && !first.standing)
          walk.wrong = `playing from ${started.at.toFixed(2)} left ${first.top} at the top of a list scrolled away, not the card of the playhead ${first.followed}`;
        // Read in the frame the playhead comes into the next caption,
        // or stops before it.
        const seen = await playedTo(page, own[1].start + 0.3, () => {
          const list = document.querySelector(".scroll.list");
          const box = list.getBoundingClientRect();
          const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
          const li = document.querySelector("ol.said .pick.current")?.closest("li");
          const r = li?.getBoundingClientRect();
          const inView = !!r && r.top >= box.top + pad - 1 && r.bottom <= box.bottom - pad + 1;
          return { top: li && inView ? Number(li.dataset.at) : null, clip: li?.dataset.clip ?? null };
        });
        await page.keyboard.press("Space");
        await watch.step("press", s);
        // Stopping gives the search field the keyboard back, the one
        // the space bar took it from, found by Tim.
        const stopped = await page.evaluate(() => document.activeElement?.getAttribute("aria-label") ?? document.activeElement?.tagName ?? null);
        // The card the playhead is in, or the one before it in a pause.
        const now = own.filter((x) => x.start <= seen.at).at(-1);
        if (walk.wrong);
        else if (stopped !== "Search transcript") walk.wrong = `stopping with the space bar left the keyboard on ${stopped}, not back in the search field`;
        else if (!(seen.at >= own[1].start + 0.3)) walk.wrong = `the clip played to ${seen.at.toFixed(2)} and never reached its second caption at ${own[1].start.toFixed(2)}`;
        else if (seen.top === null || Math.abs(seen.top - now.start) > 0.0005 || seen.clip !== chosenKey)
          walk.wrong = `the clip played to ${seen.at.toFixed(2)} with ${seen.top === null ? "no card in view" : `the card at ${seen.top.toFixed(2)}`} marked as the playhead's, not its caption at ${now.start.toFixed(2)}, "${now.text}"`;
      }
      // The page turns the way the clip timeline's does: the list
      // stands still while the card of the playhead is in view, and the
      // card that comes in below the foot of the list as the video
      // plays on is turned to the top, found by Tim. The list is made
      // as high as the cards down to the playhead's, so the next is
      // below it.
      if (!walk.wrong && own.length >= 2) {
        // First with both cards in view: playing into the next card
        // leaves the list where it is.
        await page.locator(`ol.said li[data-row="s${own[0].start}|${chosenKey}"] button.pick`).click();
        await watch.step("click", s);
        const both = await page.evaluate((rows) => {
          const list = document.querySelector(".scroll.list");
          const box = list.getBoundingClientRect();
          const inView = (row) => {
            const r = document.querySelector(`ol.said li[data-row="${CSS.escape(row)}"]`).getBoundingClientRect();
            return r.top >= box.top && r.bottom <= box.bottom;
          };
          return { scrolled: list.scrollTop, seen: rows.every(inView) };
        }, [`s${own[0].start}|${chosenKey}`, `s${own[1].start}|${chosenKey}`]);
        if (!both.seen) walk.wrong = "the chosen clip's first two cards are not both in view, so the list standing still could not be seen";
        else {
          await page.evaluate(() => document.activeElement?.blur());
          await page.keyboard.press("Space");
          await watch.step("press", s);
          const frames = await playedTo(page, own[1].start + 0.3, () => ({ scrolled: document.querySelector(".scroll.list").scrollTop }), true);
          const moved = frames.find((now) => now.scrolled !== both.scrolled);
          await page.keyboard.press("Space");
          await watch.step("press", s);
          if (moved) walk.wrong = `playing on into the next card, in view, moved the list ${(moved.scrolled - both.scrolled).toFixed(0)} px at ${moved.at.toFixed(2)}`;
        }
      }
      if (!walk.wrong && own.length >= 2) {
        await page.locator(`ol.said li[data-row="s${own[0].start}|${chosenKey}"] button.pick`).click();
        await watch.step("click", s);
        await page.evaluate((row) => {
          const list = document.querySelector(".scroll.list");
          const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
          const card = document.querySelector(`ol.said li[data-row="${CSS.escape(row)}"]`);
          const box = list.getBoundingClientRect();
          const r = card.getBoundingClientRect();
          list.scrollTop += r.top - box.top - pad;
          list.style.flex = "none";
          list.style.height = `${Math.ceil(r.height + 2 * pad + 4)}px`;
        }, `s${own[0].start}|${chosenKey}`);
        await nextPaint(page, 2);
        const from = await page.evaluate(() => document.querySelector(".scroll.list").scrollTop);
        await page.evaluate(() => document.activeElement?.blur());
        await page.keyboard.press("Space");
        await watch.step("press", s);
        const frames = await playedTo(
          page,
          own[1].start + 0.3,
          () => {
            const list = document.querySelector(".scroll.list");
            const box = list.getBoundingClientRect();
            const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
            const top = [...document.querySelectorAll("ol.said li[data-at]")].find((li) => Math.abs(li.getBoundingClientRect().top - box.top - pad) <= 1);
            return { top: top ? Number(top.dataset.at) : null, scrolled: list.scrollTop };
          },
          true,
        );
        const turned = frames.at(-1);
        const still = !frames.some((now) => now.at < own[1].start - 0.05 && now.scrolled !== from);
        await page.keyboard.press("Space");
        await watch.step("press", s);
        await page.evaluate(() => {
          const list = document.querySelector(".scroll.list");
          list.style.flex = "";
          list.style.height = "";
        });
        if (!still) walk.wrong = `the list moved while the card of the playhead at ${own[0].start.toFixed(2)} was in view`;
        else if (!(turned.at >= own[1].start + 0.3)) walk.wrong = `the clip played to ${turned.at.toFixed(2)} and never reached its second caption`;
        else if (turned.top === null || Math.abs(turned.top - own[1].start) > 0.0005)
          walk.wrong = `played on into the caption at ${own[1].start.toFixed(2)} below the foot of the list, the page turned to ${turned.top === null ? "no card" : `the card at ${turned.top.toFixed(2)}`} at the top`;
      }
    }
    // A card puts the playhead on its sentence, and a sentence of a
    // clip makes the clip the chosen one, found by Tim, with the list
    // left where it is. With nothing typed, so the list can move, and
    // a card of a clip not chosen yet that is not its first, so going
    // to the clip's first card would move it.
    await field.fill("");
    await watch.step("type", s);
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
    if (!pick && others.length > 1) walk.wrong = "no clip but the chosen one has two cards, so choosing one by its sentence could not be seen";
    if (pick) {
      const row = `ol.said li[data-row="${pickAt.row}"]`;
      const button = page.locator(`${row} button.pick`);
      await button.scrollIntoViewIfNeeded();
      const stood = await page.evaluate((row) => document.querySelector(row).getBoundingClientRect().top, row);
      await button.click();
      await watch.step("click", s);
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
      if (!walk.wrong && !(after.at >= pick.start - 0.001 && after.at <= Math.min(pick.end, pick.start + 0.2)))
        walk.wrong = `the card at ${pick.start.toFixed(3)} put the playhead at ${after.at}`;
      else if (!walk.wrong && keyed && after.chosen !== `Clip at ${clockAt(keyed.start)}`)
        walk.wrong = `a sentence of ${keyed.title || keyed.slug} clicked left ${after.chosen ?? "no clip"} chosen`;
      else if (!walk.wrong && Math.abs(after.top - stood) > 1)
        walk.wrong = `a sentence clicked moved the list ${(after.top - stood).toFixed(0)} px`;
    }
    // Shift and the arrows up and down walk the answers to a search,
    // the playhead onto each and the list after it, and letting go of
    // the search goes back to where the playhead and the chosen clip
    // stood, unless Enter said to stay, found by Tim.
    if (!walk.wrong && holds.length >= 2) {
      const where = () =>
        page.evaluate(() => ({
          at: Number(document.querySelector(".screen")?.dataset.playhead),
          chosen: document.querySelector(".track .clipmark.selected")?.getAttribute("aria-label") ?? null,
          followed: document.querySelector("ol.said")?.dataset.followed ?? null,
          current: (() => {
            const pick = document.querySelector("ol.said .pick.current");
            const ink = document.createElement("i");
            ink.style.background = "var(--ink-3)";
            document.body.append(ink);
            const want = getComputedStyle(ink).backgroundColor;
            ink.remove();
            return pick ? { row: pick.closest("li").dataset.row, grey: getComputedStyle(pick).backgroundColor === want } : null;
          })(),
          cards: [...document.querySelectorAll("ol.said li[data-row]")].map((li) => ({ row: li.dataset.row, at: Number(li.dataset.at) })),
        }));
      for (const stay of [false, true]) {
        if (walk.wrong) break;
        await field.fill(word);
        await watch.step("type", s);
        const from = await where();
        const went = [];
        for (let i = 0; i < 2; i++) {
          await field.press("Shift+ArrowDown");
          await watch.step("press", s);
          went.push(await where());
        }
        for (const [i, w] of went.entries()) {
          const card = sentences.find((x) => Math.abs(x.start - from.cards[i].at) < 0.0005);
          if (walk.wrong) break;
          if (!card || !(w.at >= card.start - 0.001 && w.at < card.end))
            walk.wrong = `Shift and the down arrow ${i + 1} times put the playhead at ${w.at.toFixed(2)}, not on answer ${i + 1} at ${from.cards[i].at.toFixed(2)}`;
          else if (w.followed !== from.cards[i].row) walk.wrong = `walked to ${from.cards[i].row}, the list follows ${w.followed}`;
          // Marked where it is, the way the chosen clip is, found by Tim.
          else if (w.current?.row !== from.cards[i].row || !w.current.grey)
            walk.wrong = `walked to ${from.cards[i].row}, the card marked as the playhead's is ${w.current?.row ?? "none"}${w.current && !w.current.grey ? ", not in the grey of a chosen clip" : ""}`;
        }
        if (walk.wrong) break;
        if (stay) {
          await field.press("Enter");
          await watch.step("press", s);
        }
        await field.press("Escape");
        await watch.step("press", s);
        const back = await where();
        if (!stay && (Math.abs(back.at - from.at) > 0.05 || back.chosen !== from.chosen))
          walk.wrong = `a search walked and let go of by Escape left the playhead at ${back.at.toFixed(2)} in ${back.chosen}, not back at ${from.at.toFixed(2)} in ${from.chosen}`;
        else if (stay && Math.abs(back.at - went[1].at) > 0.05)
          walk.wrong = `Enter at the answer walked to, then Escape, moved the playhead from ${went[1].at.toFixed(2)} to ${back.at.toFixed(2)}`;
      }
    }
    // A card walked to out of view turns the page, the way playing
    // does: walked down past the foot, it comes to the top, and walked
    // up past the top, to the foot. It was brought just into view, and
    // went on standing at the foot as the walk went down, found by
    // Tim. The list is made two cards high.
    if (!walk.wrong) {
      await field.fill("");
      await watch.step("type", s);
      const squeezed = await page.evaluate(() => {
        const list = document.querySelector(".scroll.list");
        const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
        const cards = [...document.querySelectorAll("ol.said li[data-row]")];
        const i = cards.findIndex((li) => li.dataset.row === document.querySelector("ol.said").dataset.followed);
        const card = cards[i];
        if (!card || i + 3 >= cards.length) return null;
        const box = list.getBoundingClientRect();
        list.scrollTop += card.getBoundingClientRect().top - box.top - pad;
        const two = cards[i + 1].getBoundingClientRect().bottom - card.getBoundingClientRect().top;
        list.style.flex = "none";
        list.style.height = `${Math.ceil(two + 2 * pad + 2)}px`;
        return { rows: [cards[i + 1].dataset.row, cards[i + 2].dataset.row] };
      });
      if (!squeezed) walk.wrong = "too few cards after the card of the playhead to walk a page";
      else {
        await page.evaluate(() => document.activeElement?.blur());
        // Where a card stands in the list once the list has had a
        // frame to follow the playhead, from its top and from its foot.
        const stood = (row) =>
          page.evaluate(
            (row) =>
              new Promise((done) =>
                requestAnimationFrame(() =>
                  requestAnimationFrame(() => {
                    const list = document.querySelector(".scroll.list");
                    const pad = parseFloat(getComputedStyle(document.querySelector("ol.said")).paddingTop);
                    const box = list.getBoundingClientRect();
                    const r = document.querySelector(`ol.said li[data-row="${CSS.escape(row)}"]`).getBoundingClientRect();
                    done({ top: r.top - (box.top + pad), foot: (box.bottom - pad) - r.bottom, scroll: list.scrollTop, followed: document.querySelector("ol.said").dataset.followed });
                  }),
                ),
              ),
            row,
          );
        const press = async (key) => {
          await page.keyboard.press(key);
          await watch.step("press", s);
        };
        const before = await stood(squeezed.rows[0]);
        await press("Shift+ArrowDown");
        const second = await stood(squeezed.rows[0]);
        await press("Shift+ArrowDown");
        const down = await stood(squeezed.rows[1]);
        await press("Shift+ArrowUp");
        const up = await stood(squeezed.rows[0]);
        await page.evaluate(() => {
          const list = document.querySelector(".scroll.list");
          list.style.flex = "";
          list.style.height = "";
        });
        console.log(JSON.stringify({ page: squeezed.rows, second, down, up }));
        if (second.followed !== squeezed.rows[0]) walk.wrong = `Shift and the down arrow went to ${second.followed}, not ${squeezed.rows[0]}`;
        else if (Math.abs(second.scroll - before.scroll) > 0.5) walk.wrong = `the card walked to in view moved the list ${(second.scroll - before.scroll).toFixed(0)} px`;
        else if (down.followed !== squeezed.rows[1]) walk.wrong = `Shift and the down arrow twice went to ${down.followed}, not ${squeezed.rows[1]}`;
        else if (Math.abs(down.top) > 1)
          walk.wrong = `the card walked down to below the foot of the list stands ${down.top.toFixed(0)} px below the top, not turned to the top`;
        else if (up.followed !== squeezed.rows[0]) walk.wrong = `Shift and the up arrow went to ${up.followed}, not ${squeezed.rows[0]}`;
        else if (Math.abs(up.foot) > 1)
          walk.wrong = `the card walked up to above the top of the list stands ${up.foot.toFixed(0)} px above the foot, not turned to the foot`;
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
    if (!walk.wrong && cleared.typed === null) walk.wrong = "the first Escape after typing closed the search, not what was typed";
    else if (!walk.wrong && (cleared.typed !== "" || cleared.cards < sentences.length))
      walk.wrong = `after the first Escape the field holds "${cleared.typed}" over ${cleared.cards} of ${sentences.length} sentences`;
    await page.keyboard.press("Escape");
    await watch.step("press", s);
    const back = await page.evaluate(() => ({
      search: !!document.querySelector(".listhead button.search"),
      said: !!document.querySelector("ol.said"),
      edges: document.querySelectorAll(".track .unheard").length,
    }));
    // The parts not transcribed yet stay on the range picker after the
    // search, as they are there before it, Tim's pick on issue #432.
    if (!walk.wrong && (!back.search || back.said || back.edges !== gaps))
      walk.wrong = back.said
        ? "Escape left the sentences"
        : !back.search
          ? "Escape left no Search button"
          : `after Escape the range picker shows ${back.edges} parts not transcribed yet, not the ${gaps} there are`;
    if (!walk.wrong && (await headHigh()) !== was) walk.wrong = "the head changed height after the search";
  },
  // ["asks none", name, seconds]
  //                    the interface asks the Go side nothing of this name
  //                    while the Go side sends so many pieces of job news
  async "asks none"(walk, arg) {
    const { page } = walk;
    const [name, seconds] = arg;
    const before = await page.evaluate(() => window.__calls.length);
    // The Go side tells the interface of a running job about once a
    // second, and each time the interface read the clip list again. So
    // as many of its news as seconds, then the app settled on them.
    let heard = await news(page);
    for (let i = 0; i < seconds; i++) heard = await nextNews(page, heard);
    await settle(page);
    const asked = await page.evaluate((n) => window.__calls.slice(n).map((c) => c.name), before);
    const these = asked.filter((c) => c === name).length;
    if (these) walk.wrong = `${name} was asked ${these} times in ${seconds} news of the job, with nothing changed: ${asked.join(", ")}`;
  },
  // ["hand stopped"]   I is pressed on a part not heard yet and Cancel at
  //                    the head of the clip list, so the clip made by hand
  //                    says Stopped with Continue in its card
  async "hand stopped"(walk, arg, s) {
    const { page, watch } = walk;
    const path = await episodeOn(page);
    const before = await page.evaluate(() => [...document.querySelectorAll("li[data-key]")].map((li) => li.dataset.key));
    await page.evaluate(() => document.body.focus());
    await page.keyboard.press("i");
    // The card the clip made by hand comes in, and the job it is on
    // the way from, read off its key.
    const key = await seenUntil(
      page,
      url,
      () => page.evaluate((was) => [...document.querySelectorAll("li[data-key]")].map((li) => li.dataset.key).find((k) => k.includes("/") && !was.includes(k)) ?? null, before),
      (k) => !!k,
    );
    const job = key && (await ask(page, "Jobs")).find((j) => j.id === key.split("/")[0]);
    if (!job || job.kind !== "clip" || job.episode !== path) {
      walk.wrong = key ? `the card that came, ${key}, is not of a clip made by hand` : "I made no clip on a part not heard yet";
      return;
    }
    walk.handJob = job.id;
    const card = page.locator(`li[data-key^="${walk.handJob}/"]`);
    const cancel = page.locator(".listhead button.search", { hasText: "Cancel" });
    // Once the interface has heard the job's news.
    await caughtUp(page, url, [job]);
    const busy = (await cancel.count()) > 0;
    if (!busy) {
      walk.wrong = `the head of the clip list says ${(await clipList(page))?.head}, not Cancel`;
      return;
    }
    await cancel.click();
    // The clip waits on the piece it is hearing, which is held, so
    // Cancel ends it once that piece is let through.
    if (walk.speechHeld) {
      await settle(page);
      const going = async () => ["running", "queued"].includes((await ask(page, "Jobs")).find((j) => j.id === walk.handJob)?.state);
      while (await going()) await onePiece(page, url, path);
    }
    await pageUntil(page, (k) => document.querySelector(`li[data-key^="${k}/"].stopped .title`)?.textContent.trim() === "Stopped. Click Continue", walk.handJob);
    await watch.step("wait", s);
  },
  // ["heard part", [share, seconds]]
  //                    with the speech held, lets it on one piece at a
  //                    time until a video this long is heard as far as
  //                    this share of it
  async "heard part"(walk, arg) {
    const { page } = walk;
    // One piece at a time, until the video, this many seconds long, is
    // heard as far as this share. What is heard is saved only now and
    // then, so how far the work says it heard counts too.
    const [share, seconds] = arg;
    const path = await episodeOn(page);
    const heardOf = async () => {
      const saved = ((await ask(page, "Episode", path))?.heard ?? []).reduce((sum, [a, b]) => sum + Math.max(Math.min(b, seconds) - a, 0), 0);
      const said = (await ask(page, "Jobs")).filter((j) => j.episode === path).map((j) => j.progress?.covered ?? 0);
      return Math.max(saved, ...said);
    };
    while ((await heardOf()) < share * seconds) await onePiece(page, url, path);
  },
  // ["cancel held"]    with the speech held, presses Cancel at the head of
  //                    the clip list and lets the piece being heard land,
  //                    until the head says Continue
  async "cancel held"(walk) {
    const { page } = walk;
    // The search waits on the piece it is hearing, which is held, so
    // Cancel ends it once that piece is let through.
    const path = await episodeOn(page);
    const pressed = await pressHead(page);
    if (pressed.pressed !== "Cancel") {
      walk.wrong = `the head said ${pressed.pressed}, not Cancel`;
      return;
    }
    // Every call the press made is answered, so the search is called
    // off before the piece it waits on lands.
    await settle(page);
    const running = async () => {
      const job = (await ask(page, "Jobs")).findLast((j) => j.episode === path && j.kind === "search");
      return job?.state === "running" || job?.state === "queued";
    };
    while (await running()) await onePiece(page, url, path);
    await settle(page);
    const head = await page.evaluate(() => document.querySelector(".listhead button.search")?.textContent.replace(/\s+/g, " ").trim());
    if (head !== "Continue") walk.wrong = `after Cancel the head says ${head}, not Continue`;
  },
  // ["fills from nothing", seconds]
  //                    with the speech held, on a search stopped part way
  //                    through the window of a video this long, presses
  //                    Continue and lets the speech on one piece at a time
  //                    until the search has heard one, and the fill of the
  //                    row that says Transcribing starts at nothing and
  //                    only grows, however much was heard before, while
  //                    its words say how much of the window is
  //                    transcribed, from what was heard before
  async "fills from nothing"(walk, arg) {
    const { page } = walk;
    // Issue #444: Tim searched windows heard in part already, and the
    // fill of the transcription started well past nothing. Continue on
    // a search stopped while it heard hears only what is left of its
    // window, and its fill goes from nothing over that alone.
    const path = await episodeOn(page);
    const search = (await ask(page, "Jobs")).findLast((j) => j.episode === path && j.kind === "search");
    const from = search?.from ?? 0;
    const to = search?.to > 0 ? search.to : arg;
    const heard = ((await ask(page, "Episode", path))?.heard ?? []).reduce((sum, [a, b]) => sum + Math.max(Math.min(b, to) - Math.max(a, from), 0), 0);
    const before = heard / (to - from);
    if (!(before >= 0.2 && before < 0.9)) {
      walk.wrong = `${Math.round(before * 100)} % of the window ${from} to ${to} s was heard before Continue, too little or too much to tell a fill from nothing from how much is heard: ${JSON.stringify((await ask(page, "Episode", path))?.heard)}`;
      return;
    }
    // Every fill the row of the next clip shows while it says
    // Transcribing, from the press on, read off the fill's place.
    await page.evaluate(() => {
      window.__fills = [];
      window.__said = [];
      const list = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
      const look = () => {
        const li = list.querySelector("ol li.next");
        const i = li?.querySelector(".fill i");
        if (li?.querySelector(".title")?.textContent.trim() !== "Transcribing" || !i) return;
        const said = li.querySelector(".meta")?.textContent.replace(/\s+/g, " ").trim();
        if (window.__said.at(-1) !== said) window.__said.push(said);
        const moved = /translateX\((-?[\d.]+)%\)/.exec(i.style.transform);
        const fraction = moved ? 1 + Number(moved[1]) / 100 : NaN;
        if (window.__fills.at(-1) !== fraction) window.__fills.push(fraction);
      };
      new MutationObserver(look).observe(list, { subtree: true, childList: true, attributes: true, characterData: true });
    });
    const pressed = await pressHead(page);
    if (pressed.pressed !== "Continue") {
      walk.wrong = `the head said ${pressed.pressed}, not Continue`;
      return;
    }
    // Every call the press made is answered and drawn, and then one
    // piece at a time, to whatever hears it, the video heard by itself
    // after Cancel first, until the engine has said how far the search
    // heard of what is left.
    await settle(page);
    for (;;) {
      const job = (await ask(page, "Jobs")).findLast((j) => j.episode === path && j.kind === "search");
      if (job?.state === "running" && job.step === "hearing" && job.progress?.fraction > 0) break;
      if (job?.state !== "running" && job?.state !== "queued") {
        const now = (await ask(page, "Episode", path))?.heard;
        walk.wrong = `the search of ${from} to ${to} s heard nothing more after Continue, and is ${job?.state} ${job?.step ?? ""}, with ${JSON.stringify(now)} heard now and ${Math.round(before * 100)} % of it before`;
        break;
      }
      await onePiece(page, url, path);
    }
    if (walk.wrong) return;
    // Until the row has shown it.
    await settle(page);
    const fills = await page.evaluate(() => window.__fills);
    if (!fills.length) walk.wrong = "the row never showed a fill while it said Transcribing";
    else if (!(fills[0] < 0.05)) walk.wrong = `the fill started at ${Math.round(fills[0] * 100)} %, not at nothing, with ${Math.round(before * 100)} % of the window heard before: ${JSON.stringify(fills)}`;
    else if (fills.some((f, n) => n > 0 && f < fills[n - 1])) walk.wrong = `the fill went back: ${JSON.stringify(fills)}`;
    if (walk.wrong) return;
    // Tim's report of 25 October: the row said Window 0:00 to 30:00
    // under Transcribing, over a window transcribed half already. It
    // says how much of the window is transcribed, from what was heard
    // before Continue, the way the row of the stopped search did, or
    // how long it has left once it can say.
    const said = await page.evaluate(() => window.__said);
    const span = clockOf(to - from);
    const secondsOf = (c) => c.split(":").reduce((t, n) => t * 60 + Number(n), 0);
    const counts = (line) => new RegExp(`^\\d+:\\d\\d of ${span} transcribed$`).test(line ?? "");
    const off = said.find((line) => !counts(line) && !/^About \d+:\d\d left$/.test(line ?? ""));
    if (off !== undefined) walk.wrong = `under Transcribing the row said ${JSON.stringify(off)}, not how much of the ${span} of its window is transcribed: ${JSON.stringify(said)}`;
    else if (!counts(said[0])) walk.wrong = `under Transcribing the row said ${JSON.stringify(said[0])} at first, not how much of the ${span} of its window is transcribed: ${JSON.stringify(said)}`;
    else if (!(secondsOf(said[0].split(" ")[0]) >= Math.floor(heard))) walk.wrong = `under Transcribing the row said ${JSON.stringify(said[0])} at first, less than the ${Math.round(heard)} s heard before Continue`;
  },
  // ["no mark after the end", seconds]
  //                    puts the playhead at the end of a video this long,
  //                    after its last word, and Mark In is not offered and
  //                    says why, I makes no clip, and Mark Out is offered
  async "no mark after the end"(walk, arg, s) {
    const { page, watch } = walk;
    // Tim on #456: I at the end of the video made a card that could
    // only fail, since nothing is said after the last word. The
    // playhead goes to the end the way a hand takes it there: a press
    // in the middle of the range picker, away from the tips of the
    // window at its ends, dragged on past its right end.
    const path = await episodeOn(page);
    const picker = await page.locator(".track[aria-label='The range picker']").boundingBox();
    const y = picker.y + picker.height / 2;
    await page.mouse.move(picker.x + picker.width / 2, y);
    await page.mouse.down();
    for (const share of [0.7, 0.9, 1.1, 1.3]) await page.mouse.move(picker.x + picker.width * share, y, { steps: 2 });
    await page.mouse.up();
    await settle(page);
    const at = await page.evaluate(() => Number(document.querySelector(".screen")?.dataset.playhead));
    const said = (await ask(page, "Episode", path))?.said;
    if (!(at > arg - 0.5) || !said || !(said[1] <= at)) {
      walk.wrong = `the playhead is at ${at} s, with words said from ${JSON.stringify(said)}, not after the last word at the end of the video, ${arg} s`;
      return;
    }
    const marks = await page.evaluate(() => {
      const look = (name) => {
        const b = document.querySelector(`button[aria-label='${name}']`);
        return { off: !!b?.disabled, title: b?.title ?? "" };
      };
      return { in: look("Mark In"), out: look("Mark Out") };
    });
    if (!marks.in.off) walk.wrong = `Mark In is offered after the last word: ${JSON.stringify(marks.in)}`;
    else if (marks.in.title !== "Nothing is said after the playhead, so no clip can start here") walk.wrong = `Mark In says "${marks.in.title}", not why it is not offered`;
    else if (marks.out.off) walk.wrong = `Mark Out is not offered after the last word, where a clip can end: ${JSON.stringify(marks.out)}`;
    if (walk.wrong) return;
    // I does what the button would: nothing. Every call it made is
    // answered by the time the app is quiet, so a clip asked for is in
    // the jobs by then.
    const was = (await ask(page, "Jobs")).filter((j) => j.kind === "clip").length;
    await page.evaluate(() => document.body.focus());
    await page.keyboard.press("i");
    await settle(page);
    const now = (await ask(page, "Jobs")).filter((j) => j.kind === "clip").length;
    const cards = await page.locator("li.stopped.failed").count();
    if (now !== was) walk.wrong = `I after the last word asked for a clip, ${now - was} of them`;
    else if (cards) walk.wrong = `I after the last word made ${cards} cards that failed`;
    await watch.step("wait", s);
  },
  // ["hand failed"]    with the speech silent and held, presses I where
  //                    nothing is heard yet and lets the speech go, and
  //                    the clip's card says Failed. Click Continue and
  //                    where it lies, and a toast with Report says why
  async "hand failed"(walk, arg, s) {
    const { page, watch } = walk;
    // Issue #450: a card whose clip could not be made said why in its
    // second line, cut to a line. I where nothing is heard yet is
    // offered, and with the speech silent the clip finds once it has
    // heard that nothing is said there. The card says Failed with
    // Continue and where it lies, and why only in a toast.
    const path = await episodeOn(page);
    const at = await page.evaluate(() => Number(document.querySelector(".screen")?.dataset.playhead));
    const was = new Set((await ask(page, "Jobs")).map((j) => j.id));
    await page.evaluate(() => document.body.focus());
    await page.keyboard.press("i");
    // Every call the press made is answered, so the clip is asked for,
    // and then the speech is let go, silent still.
    await settle(page);
    const asked = (await ask(page, "Jobs")).find((j) => j.kind === "clip" && j.episode === path && !was.has(j.id));
    if (!asked) {
      walk.wrong = `I at ${at} s, where nothing is heard yet, asked for no clip`;
      return;
    }
    await control(url, "/speech?go");
    walk.speechHeld = false;
    // Until the Go side says the clip asked for has ended, and then the
    // frame the card shows it in.
    let job;
    for (let seen = await news(page); ; seen = await nextNews(page, seen)) {
      job = (await ask(page, "Jobs")).find((j) => j.id === asked.id);
      if (job && job.state !== "running" && job.state !== "queued") break;
    }
    if (job.state !== "failed") {
      walk.wrong = `I at ${at} s on a video with no speech made a clip that is ${job.state}, not failed`;
      return;
    }
    // The failed news is in the store, so its card and its toast are
    // drawn once the app is quiet, and read straight away, well inside
    // the toast's own time.
    await settle(page);
    const said = await page.evaluate((id) => {
      const li = document.querySelector(`li[data-key^="${id}/"]`);
      return {
        failed: !!li?.matches(".stopped.failed"),
        title: li?.querySelector(".title")?.textContent.trim() ?? "",
        meta: li?.querySelector(".meta")?.textContent.trim() ?? "",
      };
    }, job.id);
    const toast = await toastSays(page);
    const report = await page.locator(".toast:not([inert]) button.report").count();
    const where = clockOf(job.underway?.[0]?.start ?? at);
    if (!said.failed) walk.wrong = `the clip that failed has no card that says so: ${JSON.stringify(said)}`;
    else if (said.title !== "Failed. Click Continue") walk.wrong = `the card says "${said.title}", not Failed. Click Continue`;
    else if (said.meta !== where) walk.wrong = `the card's second line says "${said.meta}", not where the clip lies, ${where}`;
    else if (!toast.includes("Nothing is said here yet")) walk.wrong = `the toast says "${toast}", not why the clip could not be made`;
    else if (!report) walk.wrong = "the toast offers no Report";
    await watch.step("wait", s);
  },
  // ["remove stopped", how]
  //                    the bin on that card is clicked, and it says
  //                    Removed with Put it back, and its mark leaves the
  //                    range picker at once. With "put back" that is
  //                    clicked, and the card says Stopped again and the
  //                    work stays. With "for good" the time runs down, the
  //                    card goes and the Go side calls the work off
  async "remove stopped"(walk, arg, s) {
    const { page, removeTime, watch } = walk;
    const card = page.locator(`li[data-key^="${walk.handJob}/"]`).first();
    const key = await card.getAttribute("data-key");
    const stateOf = async () => (await ask(page, "Jobs")).find((j) => j.id === walk.handJob)?.state;
    await card.hover();
    if (!(await card.locator("button.drop").count())) {
      walk.wrong = "the card that stopped has no bin to remove it with";
      return;
    }
    // The time a remove runs is run on by hand, so nothing waits on
    // the clock: everything else goes on in step with it.
    await fakeClock(page);
    // The marks of clips on their way on the range picker, the card's
    // among them.
    const waiting = () => page.evaluate(() => document.querySelectorAll(".clipmark.waiting").length);
    const marked = await waiting();
    await card.locator("button.drop").click();
    await settle(page);
    const removed = (await card.locator(".gone", { hasText: "Removed" }).count()) > 0;
    if (!removed) {
      walk.wrong = "the bin on the card that stopped did not say Removed in its place";
      return;
    }
    const strip = await removedStrip(page, `li[data-key="${key}"]`);
    if (strip) {
      walk.wrong = strip;
      return;
    }
    // The range picker says what the card says, in the same frame.
    const left = await waiting();
    if (left !== marked - 1) {
      walk.wrong = `the card says Removed and the range picker still has ${left} marks of clips on their way, of ${marked}`;
      return;
    }
    if (arg === "put back") {
      await card.getByRole("button", { name: "Put it back" }).click();
      await page.mouse.move(5, 5);
      await settle(page);
      const back = await page.evaluate((k) => document.querySelector(`li[data-key="${k}"] .title`)?.textContent.trim() === "Stopped. Click Continue", key);
      // Past the time a remove runs, so a put back that only hid it
      // would have called the work off by now, and what that set
      // going has come back.
      await runClock(page, (removeTime + 1) * 1000);
      await settle(page);
      const state = await stateOf();
      if (!back) walk.wrong = "Put it back did not bring back the card that said Stopped";
      else if ((await waiting()) !== marked) walk.wrong = `put back, the range picker has ${await waiting()} marks of clips on their way, not ${marked}`;
      else if (state !== "interrupted") walk.wrong = `put back, the work of the clip is ${state}, not interrupted`;
    } else {
      // The time stands still under the hand, see Busy, so the hand
      // goes off the list, and then it runs out.
      await page.mouse.move(5, 5);
      await runClock(page, (removeTime + 1) * 1000);
      await settle(page);
      const gone = await page.evaluate((k) => !document.querySelector(`li[data-key="${k}"]`), key);
      const state = await stateOf();
      if (!gone) walk.wrong = "the card that stopped is still in the list after its time ran down";
      else if (state !== "cancelled") walk.wrong = `removed, the work of the clip is ${state}, not cancelled`;
    }
    await watch.step("wait", s);
  },
  // ["failed red"]     the row of the search that failed, the video's dot
  //                    in the sidebar and the toast are all in the red of
  //                    an error
  async "failed red"(walk) {
    const { page } = walk;
    const colours = await page.evaluate(() => {
      const colour = (el, what) => (el ? getComputedStyle(el)[what] : "");
      return {
        row: colour(document.querySelector("li.next.stopped .title"), "color"),
        dot: colour(document.querySelector("aside button.episode.current .dot"), "backgroundColor"),
        toast: colour(document.querySelector(".toast .mark"), "color"),
      };
    });
    if (!colours.row || !colours.dot || !colours.toast) walk.wrong = `the row, the dot and the toast are not all there: ${JSON.stringify(colours)}`;
    else if (colours.row !== colours.dot || colours.row !== colours.toast) {
      walk.wrong = `the failure is said in more than one colour: ${JSON.stringify(colours)}`;
    }
  },
  // ["remove search", how]
  //                    the bin on the row of the search that stopped is
  //                    clicked, and it says Removed with Put it back. With
  //                    "put back" that is clicked, the row says how the
  //                    search stopped again and the search stays. With "for
  //                    good" the time runs down, the row goes, the Go side
  //                    forgets the search, and the video's dot in the
  //                    sidebar says nothing is wrong. With "away and back"
  //                    another video is opened and this one again while it
  //                    runs down, and Removed is there as it was, standing
  //                    still, and then it is for good
  async "remove search"(walk, arg, s) {
    const { page, removeTime, watch } = walk;
    // The search's own row, the first row still to come. A card on the
    // way has a key of its own.
    const lead = "li.next:not([data-key])";
    const row = page.locator(`${lead}.stopped`).first();
    const path = await episodeOn(page);
    const search = async () => (await ask(page, "Jobs")).filter((j) => j.episode === path && j.kind === "search").pop();
    const job = (await search())?.id;
    const was = (await row.locator(".title").textContent())?.trim();
    await row.hover();
    if (!(await row.locator("button.drop").count())) {
      walk.wrong = `the row "${was}" has no bin to remove the search with`;
      return;
    }
    // The time a remove runs is run on by hand, so nothing waits on
    // the clock.
    await fakeClock(page);
    await row.locator("button.drop").click();
    // Removed takes its place in the frame of the click.
    await realFrames(page);
    const strip = await removedStrip(page, `${lead}:has(.gone)`);
    if (strip) {
      walk.wrong = strip;
      return;
    }
    if (arg === "away and back") {
      const buttons = page.locator("aside button.episode");
      const mine = await buttons.evaluateAll((all) => all.findIndex((b) => b.classList.contains("current")));
      await fromSidebar(page, () => buttons.nth(mine === 0 ? 1 : 0).click());
      await settle(page);
      await fromSidebar(page, () => buttons.nth(mine).click());
      await settle(page);
      const again = await removedStrip(page, `${lead}:has(.gone)`);
      if (again) {
        walk.wrong = `back on the video, ${again}`;
        return;
      }
    }
    if (arg === "put back") {
      await page.locator(`${lead} .gone`).getByRole("button", { name: "Put it back" }).click();
      await page.mouse.move(5, 5);
      await realFrames(page);
      const back = (await page.evaluate((lead) => document.querySelector(`${lead}.stopped .title`)?.textContent.trim(), lead)) === was;
      // Past the time a remove runs, so a put back that only hid it
      // would have forgotten the search by now.
      await runClock(page, (removeTime + 1) * 1000);
      await settle(page);
      const now = await search();
      if (!back) walk.wrong = `Put it back did not bring back the row "${was}"`;
      else if (now?.id !== job || now.state === "cancelled") walk.wrong = `put back, the search is ${now?.id} ${now?.state}, not ${job} as it stopped`;
    } else {
      // The time stands still under the hand, see Busy, so the hand
      // goes off the list, and then it runs out. The Go side is asked
      // to forget the search, and settle waits for its answer.
      await page.mouse.move(5, 5);
      await runClock(page, (removeTime + 1) * 1000);
      await settle(page);
      const left = await page.evaluate(() => document.querySelectorAll("li.next.stopped, li.next .gone").length);
      const now = await search();
      if (left) walk.wrong = `${left} rows or cards of the search are still in the list after its time ran down`;
      else if (now?.state !== "cancelled") walk.wrong = `removed, the search is ${now?.state}, not cancelled`;
    }
    await watch.step("wait", s);
  },
  // ["no hand card"]   no card of a clip made by hand is in the list
  async "no hand card"(walk) {
    const { page } = walk;
    const path = await episodeOn(page);
    const hand = (await ask(page, "Jobs")).filter((j) => j.kind === "clip" && j.episode === path).map((j) => j.id);
    const left = await page.evaluate((ids) => [...document.querySelectorAll("li[data-key]")].filter((li) => ids.includes(li.dataset.key.split("/")[0])).length, hand);
    if (left) walk.wrong = `${left} cards of clips made by hand are in the list`;
  },
  // ["cancel fades"]  presses Cancel on a search, and two frames on the
  //                    frame over its window is fading and standing still,
  //                    with the window at rest under it
  async "cancel fades"(walk, arg, s) {
    const { page, watch } = walk;
    // Cancel pressed on a search: two frames on, the frame over its
    // window is fading and standing still, with the window at rest
    // under it, long before the search has stopped. Tim found it
    // dropped abruptly, his report of 24 October, 21.42.
    const seen = await page.evaluate(
      () =>
        new Promise((done) => {
          const pane = [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
          const button = pane.querySelector(".listhead button.search");
          const frames = () => [...document.querySelectorAll(".searched")];
          const before = frames().filter((p) => !p.classList.contains("going")).length;
          button.click();
          requestAnimationFrame(() =>
            requestAnimationFrame(() =>
              done({
                before,
                going: frames().filter((p) => p.classList.contains("going")).length,
                left: frames().filter((p) => !p.classList.contains("going")).length,
                rest: !!document.querySelector(".aim"),
              }),
            ),
          );
        }),
    );
    if (seen.before !== 1) walk.wrong = `before Cancel, ${seen.before} frames over the window`;
    else if (seen.going !== 1 || seen.left !== 0 || !seen.rest)
      walk.wrong = `the window's frame does not fade at Cancel: ${JSON.stringify(seen)}`;
    await watch.step("head", s);
    return;
  },
  // ["walks on with the clips"]
  //                    lets the speech held go, and watches every frame
  //                    until the clips the search found show: in the first
  //                    frame they show, the window is already past where
  //                    it was, and once all is still, the window drawn is
  //                    the one the Go side keeps
  async "walks on with the clips"(walk, arg, s) {
    const { page } = walk;
    const path = await episodeOn(page);
    const search = (await ask(page, "Jobs")).filter((j) => j.episode === path && j.kind === "search").pop();
    await page.evaluate(() => {
      const pane = () => [...document.querySelectorAll("aside")].find((a) => a.querySelector(".listhead"));
      const from = () => Number(document.querySelector(".aim .tip.top.from")?.getAttribute("aria-valuenow") ?? -1);
      window.__walksOn = new Promise((done) => {
        const seen = [];
        const frame = () => {
          seen.push({ cards: pane()?.querySelectorAll("ol li[data-key]").length ?? 0, from: from() });
          if (seen.filter((f) => f.cards > 0 && f.from >= 0).length >= 30) return done(seen);
          requestAnimationFrame(frame);
        };
        requestAnimationFrame(frame);
      });
    });
    await control(url, "/speech?go");
    walk.speechHeld = false;
    const seen = await page.evaluate(() => window.__walksOn);
    // The first frame with the window at rest, the search's frame gone.
    const rest = seen.find((f) => f.from >= 0);
    if (!search || !(search.to > 0)) {
      walk.wrong = `there was no search with a window when the speech went on: ${JSON.stringify(search)}`;
      return;
    }
    if (rest.from < search.to - 0.5) {
      walk.wrong = `the window was drawn at ${rest.from} s after the search of ${search.from ?? 0}–${search.to} s ended, before it moved on`;
      return;
    }
    if (rest.cards === 0) {
      walk.wrong = "the window moved on before the clips the search found showed";
      return;
    }
    await settle(page);
    const kept = await ask(page, "ChosenWindow", path);
    const drawn = await page.evaluate(() => {
      const n = (sel) => Number(document.querySelector(`.aim .tip.top.${sel}`)?.getAttribute("aria-valuenow"));
      return { from: n("from"), to: n("to") };
    });
    if (!kept || Math.abs(kept.from - drawn.from) > 0.5 || Math.abs(kept.to - drawn.to) > 0.5) {
      walk.wrong = `the window drawn is ${drawn.from}–${drawn.to} s, the one kept ${JSON.stringify(kept)}`;
    }
    return;
  },
  // ["dots begin at the window"]
  //                    once a search heard its window and the window moved
  //                    on, the part not transcribed yet begins where the
  //                    window now begins, and its first column of dots
  //                    stands at that edge, with no room left over there,
  //                    and that edge is a dashed line in the grey of a
  //                    card's edge, a dash beside each row of dots
  async "dots begin at the window"(walk, arg, s) {
    const { page } = walk;
    const path = await episodeOn(page);
    await settle(page);
    const from = await page.evaluate(() => Number(document.querySelector(".aim .tip.top.from")?.getAttribute("aria-valuenow") ?? -1));
    const heard = (await ask(page, "Episode", path))?.heard ?? [];
    const reach = heard.find(([a]) => a <= 0.5)?.[1] ?? 0;
    if (Math.abs(reach - from) > 0.5) {
      walk.wrong = `the window begins at ${from} s and the transcript reaches ${reach} s, so no part begins where the window does`;
      return;
    }
    // The part that begins at the window's edge, and how far in from
    // that edge its first column of dots stands.
    const part = await page.evaluate(() => {
      const pitch = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--dot-pitch"));
      const track = document.querySelector(".track").getBoundingClientRect();
      const parts = [...document.querySelectorAll(".track .unheard")].map((u) => {
        const r = u.getBoundingClientRect();
        const b = getComputedStyle(u, "::before");
        const e = getComputedStyle(u, "::after");
        return {
          at: r.left - track.left,
          width: r.width,
          in: parseFloat(b.left),
          out: parseFloat(b.right),
          worked: u.classList.contains("worked"),
          // Its dashed edges: which sides, in what, and laid how.
          edges: [e.borderLeftStyle, e.borderRightStyle, e.borderTopStyle, e.borderBottomStyle],
          colour: e.borderLeftColor,
          dashes: [e.maskImage.startsWith("repeating-linear-gradient"), e.maskSize, e.maskRepeat, e.maskPositionY ?? e.webkitMaskPositionY],
          rows: [b.height, e.height, b.backgroundPositionY],
        };
      });
      const probe = document.createElement("div");
      probe.style.color = "var(--ink-3)";
      document.body.append(probe);
      const grey = getComputedStyle(probe).color;
      probe.remove();
      return { pitch, parts, track: track.width, grey };
    });
    const first = part.parts.find((p) => p.at > 1);
    if (!first) {
      walk.wrong = `no part not transcribed yet after the window's edge: ${JSON.stringify(part)}`;
      return;
    }
    if (first.in !== 0)
      walk.wrong = `the first column of dots after the window stands ${first.in} px in from the window's edge, not at it: ${JSON.stringify(first)}, dots ${part.pitch} px apart`;
    else if (first.out < 0 || first.out >= part.pitch) walk.wrong = `the dots after the window are not whole columns: ${JSON.stringify(first)}`;
    // Its edge is dashed, in the grey of a card's edge, a dash beside
    // each row of dots, the way the cards of what was said are, and it
    // runs to the end of the video, where the range picker's border is
    // its edge. Tim's report of 26 October, 12.27.
    else if (first.edges.join(" ") !== "solid none none none" || first.colour !== part.grey)
      walk.wrong = `the part not transcribed yet after the window has edges ${first.edges.join(", ")} in ${first.colour}, not a dashed line down its left edge in ${part.grey}`;
    else if (!first.dashes[0] || first.dashes[1] !== `100% ${part.pitch}px` || first.dashes[2] !== "repeat-y" || first.rows[0] !== first.rows[1] || first.dashes[3] !== first.rows[2])
      walk.wrong = `the dashes down the edge of a part do not stand beside its rows of dots: dashes ${JSON.stringify(first.dashes)}, dots ${JSON.stringify(first.rows)}`;
    return;
  },
};

// What is wrong when a row that says Removed leaves a strip of its card
// on either side of it, or stands anywhere but where it is laid out, or
// null. Removed fills its card from edge to edge and does not move, issue
// #431. The row is the Removed's offset parent, see li in ClipList.svelte.
// The app's clock is held while a remove runs, see fakeClock, so a slide
// in would stand at its start and be read here.
async function removedStrip(page, row) {
  const boxes = await page.evaluate((sel) => {
    const li = document.querySelector(sel);
    const gone = li?.querySelector(".gone");
    if (!li || !gone) return null;
    const [a, b] = [li.getBoundingClientRect(), gone.getBoundingClientRect()];
    return {
      left: gone.offsetLeft,
      right: li.clientWidth - gone.offsetLeft - gone.offsetWidth,
      moved: b.left - a.left - gone.offsetLeft,
      opacity: Number(getComputedStyle(gone).opacity),
    };
  }, row);
  if (!boxes) return `no row ${row} says Removed`;
  if (Math.abs(boxes.left) > 0.5 || Math.abs(boxes.right) > 0.5) {
    return `Removed leaves ${boxes.left.toFixed(1)} px of its card on the left and ${boxes.right.toFixed(1)} px on the right`;
  }
  if (Math.abs(boxes.moved) > 0.5 || boxes.opacity !== 1) {
    return `Removed stands ${boxes.moved.toFixed(1)} px from where it is laid out, at opacity ${boxes.opacity}`;
  }
  return null;
}

// What read says in every frame while the video plays, with the playhead
// as at, until the playhead reaches to or the video stops: the last frame,
// or with every set, each frame read.
async function playedTo(page, to, read, every = false) {
  const frames = await page.evaluate(
    ([to, read]) =>
      new Promise((done) => {
        const look = new Function(`return (${read})()`);
        const seen = [];
        const tick = () => {
          const at = Number(document.querySelector(".screen")?.dataset.playhead);
          seen.push({ at, ...look() });
          const playing = !!document.querySelector('button[aria-label="Pause"]');
          if (at >= to || !playing) done(seen);
          else requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      }),
    [to, read.toString()],
  );
  return every ? frames : frames.at(-1);
}
