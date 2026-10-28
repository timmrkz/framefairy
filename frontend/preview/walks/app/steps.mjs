// The steps of the sequences about the app as a whole: its size, its menus,
// the settings and the Updates page. Each is said above it, a list with its
// name first. walkSequence in ../sequences.mjs walks it, and hands it the
// walk so far, the step's argument, what the caption box showed before it
// and the step's name. A sequence of any folder can take it. Below them the
// helpers only these steps use. See docs/TESTING.md.
import { ask, chosen, control, engineState, fromSidebar, nextPaint, seekTo, seenUntil, settle, sidebarStill } from "../bridge.mjs";
import { rate, url } from "../common/steps.mjs";

export const steps = {
  // ["first run"]      makes the bridge's machine one the app never ran
  //                    on and opens the app again: a card for each model,
  //                    nothing downloading until Download is pressed,
  //                    both downloading at once, their rows the same
  //                    shape as the rows waiting, and once both have arrived the app
  //                    on its first page, with one big button to add a
  //                    video and nothing pressed in between
  async "first run"(walk) {
    const { page } = walk;
    await control(url, "/fresh");
    await page.reload();
    const card = (name) => page.locator(".setup .area", { has: page.locator("h2", { hasText: name }) });
    const [speech, clip] = [card("Speech model"), card("Clip model")];
    await clip.locator("li").first().waitFor();
    await settle(page);
    // Where each cell of every row starts, and how high the rows are
    // inside the line between two rows, which app.css adds to a row. The
    // fill of a row on its way is no cell.
    const rows = () =>
      page.evaluate(() =>
        [...document.querySelectorAll(".setup ul.models > li")].map((li) => ({
          height: li.clientHeight,
          busy: !!li.querySelector(":scope > .beam"),
          cells: [...li.children].filter((c) => !c.matches(".beam")).map((c) => c.getBoundingClientRect().left),
          said: [...li.children].filter((c) => !c.matches(".beam")).map((c) => c.textContent.trim()),
          // A cell whose words are wider than it is, cut short.
          cut: [...li.children].filter((c) => c.scrollWidth > c.clientWidth).map((c) => `${c.textContent.trim()} ${c.scrollWidth} in ${c.clientWidth}`),
        })),
      );
    const buttons = () => page.evaluate(() => [...document.querySelectorAll(".setup button.act")].map((b) => b.textContent.trim()));
    const first = await rows();
    const pressed = await buttons();
    const astray = first.find((r) => r.height !== first[0].height || r.cells.join() !== first[0].cells.join());
    const cut = first.flatMap((r) => r.cut);
    if (pressed.some((b) => b !== "Download")) walk.wrong = `before anything is pressed the setup shows ${JSON.stringify(pressed)}, only Download`;
    else if (first.length < 2) walk.wrong = `the setup offers ${first.length} models, a speech model and a clip model at least`;
    else if (astray) walk.wrong = `a row stands out of the table: ${JSON.stringify(astray)} beside ${JSON.stringify(first[0])}`;
    else if (cut.length) walk.wrong = `the setup cuts words short: ${JSON.stringify(cut)}`;
    else if (await page.locator(".setup button.act.primary").count()) walk.wrong = "the setup lights a Download over the others, where every Download is the same plain button";
    if (walk.wrong) return;
    // The space under the title is the space between the two cards.
    const space = await page.evaluate(() => {
      const [head, first, second] = [".setup header", ".setup .area", ".setup .area + .area"].map((s) => document.querySelector(s).getBoundingClientRect());
      return { title: Math.round(first.top - head.bottom), cards: Math.round(second.top - first.bottom) };
    });
    if (space.title !== space.cards) walk.wrong = `the title stands ${space.title} px over the speech model and the two models ${space.cards} px apart, not the same`;
    if (walk.wrong) return;

    await speech.locator("button.act", { hasText: "Download" }).click();
    await clip.locator("button.act", { hasText: "Download" }).first().click();
    await speech.locator("li .cell", { hasText: /B of [\d.]+ [KMG]B/ }).waitFor();
    await clip.locator("li .cell", { hasText: /B of [\d.]+ [KMG]B/ }).waitFor();
    await settle(page);
    // A row on its way keeps the shape of the rows around it: its maker
    // where it was, how far it has come where the memory it needs stood,
    // the time left where its size stood, every cell starting where that
    // column starts in every other row.
    const during = await rows();
    const both = during.filter((r) => r.busy).map((r) => ({ maker: r.said[1], far: r.said[2], left: r.said[3], button: r.said[4] }));
    const shaped = during.find((r) => r.height !== first[0].height || r.cells.join() !== first[0].cells.join());
    if (both.length !== 2 || both.some((r) => r.button !== "Cancel")) walk.wrong = `with both pressed, the setup downloads ${JSON.stringify(both)}, not both at once`;
    else if (both.some((r) => !/B of [\d.]+ [KMG]B/.test(r.far) || !r.maker)) walk.wrong = `a download says ${JSON.stringify(both)}, with its maker gone or how far it came out of its column`;
    else if (both.some((r) => !/^\d+:\d\d left$/.test(r.left))) walk.wrong = `a download says ${JSON.stringify(both.map((r) => r.left))} where its time left goes`;
    else if (shaped) walk.wrong = `a row on its way stands out of the table: ${JSON.stringify(shaped)} beside ${JSON.stringify(first[0])}`;
    if (walk.wrong) return;

    await control(url, "/fresh?arrive=speech");
    await speech.locator(".ready").waitFor();
    await settle(page);
    const half = { ready: await page.locator(".setup .ready").count(), setup: await page.locator(".setup").count(), buttons: await buttons() };
    // The speech model here has no button left, the clip model on its
    // way its Cancel, and the other clip models wait their turn.
    if (half.setup !== 1 || half.ready !== 1 || half.buttons.filter((b) => b === "Cancel").length !== 1 || half.buttons.some((b) => b !== "Cancel" && b !== "Download")) {
      walk.wrong = `with the speech model here and the clip model on its way, the app shows ${JSON.stringify(half)}`;
      return;
    }

    await control(url, "/fresh?arrive=clip");
    const add = page.locator(".welcome button.add-video");
    await add.waitFor();
    await settle(page);
    const end = { setup: await page.locator(".setup").count(), videos: await page.locator("aside li", { hasText: ".mp4" }).count(), add: (await add.textContent()).trim() };
    if (end.setup || end.videos || !end.add.startsWith("Add a video")) walk.wrong = `once both models are here, the app shows ${JSON.stringify(end)}`;
  },
  // ["first run small"] makes the bridge's machine one the app never ran
  //                    on, with 8 GB of memory, which holds no clip
  //                    model: the clip card offers a model in the cloud
  //                    and its key in place of the list, and once the
  //                    speech model is here and a key is saved the app
  //                    is on its first page, finding clips in the cloud
  async "first run small"(walk) {
    const { page } = walk;
    await control(url, "/fresh?memory=8");
    await page.reload();
    const card = (name) => page.locator(".setup .area", { has: page.locator("h2", { hasText: name }) });
    const [speech, clip] = [card("Speech model"), card("Clip model")];
    await speech.locator("li").first().waitFor();
    await settle(page);
    const seen = { lists: await clip.locator("ul.models").count(), cloud: await clip.locator("button.pick").count(), key: await clip.locator("input.key").count() };
    // The two rows have their names in one column, each name whole, and
    // their last controls end in one column too, at the card's edge.
    const shape = await clip.locator(".card > .item").evaluateAll((items) =>
      items.map((i) => {
        const head = i.querySelector(".head");
        const last = [...i.querySelectorAll("button, input")].at(-1).getBoundingClientRect().right;
        return { name: head.getBoundingClientRect().left, high: head.getBoundingClientRect().height, wide: head.scrollWidth > head.clientWidth, end: Math.round(last) };
      }),
    );
    if (seen.lists || seen.cloud !== 1 || seen.key !== 1) walk.wrong = `on a machine no clip model fits, the clip card shows ${JSON.stringify(seen)}, not a model in the cloud and its key`;
    else if (shape.length !== 2 || shape[0].name !== shape[1].name || shape.some((r) => r.wide || r.high !== shape[0].high || r.end !== shape[0].end)) walk.wrong = `the rows of the model in the cloud and its key stand apart: ${JSON.stringify(shape)}`;
    if (walk.wrong) return;
    await speech.locator("button.act", { hasText: "Download" }).click();
    await speech.locator("li .cell", { hasText: /B of [\d.]+ [KMG]B/ }).waitFor();
    await control(url, "/fresh?arrive=speech");
    await speech.locator(".ready").waitFor();
    if (await page.locator(".welcome").count()) {
      walk.wrong = "the app left the setup with the speech model here and no key to find clips with";
      return;
    }
    await clip.locator("input.key").fill("sk-ant-walk");
    await clip.locator("button.act", { hasText: "Save key" }).click();
    const add = page.locator(".welcome button.add-video");
    await add.waitFor();
    const now = await ask(page, "Setup");
    if (!now.chosen || now.planner !== "api") walk.wrong = `once the key is saved, the app finds clips with ${JSON.stringify(now.planner)}, chosen ${now.chosen}, not in the cloud`;
  },
  // ["title bar"]      lays the app's window out the way macOS does on
  //                    Tim's Mac, a title bar 30 points high with its
  //                    buttons from 12 to 72 points in. macOS draws the
  //                    window's edge over the top 2 points and the line
  //                    at the foot takes the last, so the name of the
  //                    video and its label have to be centred between
  //                    the two, and start one space after the buttons
  async "title bar"(walk, arg, s) {
    const { page, watch } = walk;
    // What macOS answered on Tim's Mac, read off his screenshot, and
    // the Go side tells the page the way it does after a resize.
    // The page is watched before the Go side speaks, so the step goes
    // on the moment the shell carries the new bar, and not a moment
    // the clock picked.
    await page.evaluate(() => {
      const shell = document.querySelector(".shell");
      const told = () => shell.getAttribute("style")?.includes("--bar-h: 30px");
      window.__barTold = new Promise((done) => {
        const seen = new MutationObserver(() => told() && (seen.disconnect(), done()));
        seen.observe(shell, { attributes: true, attributeFilter: ["style"] });
      });
    });
    await control(url, "/chrome?bar=30&left=12&right=72");
    await page.evaluate(() => window.__barTold);
    const seen = await page.evaluate(() => {
      const box = (el) => el?.getBoundingClientRect();
      const gap = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--gap"));
      const bar = box(document.querySelector("header.bar"));
      const name = box(document.querySelector("header.bar .what"));
      const pill = box(document.querySelector("header.bar .tag"));
      return { gap, bar: [bar.top, bar.height], name: name && [name.left, name.top, name.height], pill: pill && [pill.top, pill.height] };
    });
    // The middle of the bar Tim sees, read off his pictures: under
    // the window's edge over its top 2 points and over the line on
    // its last. A quarter of a point is the most the letters may be
    // off, because half a point is what Tim saw.
    const centre = seen.bar[0] + 2 + (seen.bar[1] - 3) / 2;
    const off = (top, height) => top + height / 2 - centre;
    if (seen.bar[1] !== 30) walk.wrong = `the title bar is ${seen.bar[1]} px high, macOS laid it out 30 high`;
    else if (!seen.name || !seen.pill) walk.wrong = `the title bar shows ${seen.name ? "no label" : "no name"}`;
    else if (Math.abs(off(seen.name[1], seen.name[2])) > 0.25)
      walk.wrong = `the name's capitals stand ${Math.abs(off(seen.name[1], seen.name[2])).toFixed(2)} px ${off(seen.name[1], seen.name[2]) < 0 ? "above" : "below"} the middle of the bar`;
    else if (Math.abs(off(...seen.pill)) > 0.25)
      walk.wrong = `the label stands ${Math.abs(off(...seen.pill)).toFixed(2)} px ${off(...seen.pill) < 0 ? "above" : "below"} the middle of the bar`;
    else if (Math.abs(seen.name[0] - (72 + seen.gap)) > 0.5)
      walk.wrong = `the name starts at ${seen.name[0].toFixed(1)} px, one space after the buttons is ${72 + seen.gap}`;
    await watch.step("title bar", s);
  },
  // ["two menus"]      opens the three dots of the lowest video in the
  //                    sidebar, then those of the highest while that menu
  //                    is open. The sidebar has to stay open with only the
  //                    second menu in it, and close once the menu is
  //                    closed and the pointer has left
  async "two menus"(walk) {
    const { page } = walk;
    // Tim opened one video's menu while another's was open, and the
    // sidebar closed under the new menu, which hung squeezed over the
    // rail where nothing could be done with it. The second menu heard
    // itself open before the first heard itself close.
    await page.mouse.move(20, 500);
    await sidebarStill(page);
    const rows = page.locator("aside li:has(button.more)");
    const [low, high] = [rows.last(), rows.first()];
    await low.hover();
    await low.locator("button.more").click();
    await page.locator(".video-menu").waitFor();
    await high.hover();
    await high.locator("button.more").click();
    await settle(page);
    await sidebarStill(page);
    const now = await page.evaluate(() => {
      const a = document.querySelector("aside");
      const menus = [...document.querySelectorAll(".video-menu")].map((m) => m.getBoundingClientRect());
      const r = a.getBoundingClientRect();
      return { open: a.classList.contains("open"), menus: menus.length, inside: menus.every((m) => m.left >= r.left && m.right <= r.right) };
    });
    if (!now.open) walk.wrong = `with the second menu open the sidebar is closed, and ${now.menus} menu${now.menus === 1 ? " hangs" : "s hang"} over the rail`;
    else if (now.menus !== 1) walk.wrong = `${now.menus} menus are open, not the one of the row clicked last`;
    else if (!now.inside) walk.wrong = "the second menu reaches out of the sidebar";
    await page.keyboard.press("Escape");
    await page.locator(".video-menu").waitFor({ state: "detached" });
    // And once no menu is open, the sidebar closes the usual way when
    // the pointer leaves it: no menu still counts as holding it.
    await page.mouse.move(400, 300);
    await page.mouse.move(750, 400);
    await sidebarStill(page);
    if (!walk.wrong && (await page.evaluate(() => document.querySelector("aside").classList.contains("open"))))
      walk.wrong = "the sidebar stays open with no menu and the pointer away from it";
  },
  // ["settings"]       opens the settings from the sidebar, and remembers
  //                    the models they name
  async settings(walk) {
    const { page } = walk;
    walk.episodeBefore = (await chosen(page))?.path ?? walk.episodeBefore;
    await fromSidebar(page, () => page.locator("aside").getByText("Settings", { exact: true }).first().click());
    await page.locator(".card.finding").waitFor();
    await settle(page);
    walk.named = await modelsNamed(page);
    // Every card that shakes from here on, by its classes. A card that
    // only wears the class and runs no animation did not shake: the rule
    // that moves it once lived beside it and was lost in a move.
    await page.evaluate(() => {
      window.__shook = [];
      new MutationObserver((changes) => {
        for (const c of changes)
          if (c.target.classList?.contains("shaking") && getComputedStyle(c.target).animationName !== "none") window.__shook.push(c.target.className);
      }).observe(document.body, { attributes: true, attributeFilter: ["class"], subtree: true });
    });
  },
  // ["acknowledgements"]
  //                    Help, Acknowledgements: every group is a head over a
  //                    card with a border, and the words of every row stand
  //                    in from the card's edges, never on them
  async acknowledgements(walk) {
    const { page } = walk;
    await page.evaluate(() => window.__help());
    await page.locator("h2 + ol > li").first().waitFor();
    await settle(page);
    walk.wrong = await page.evaluate(() => {
      const rows = [...document.querySelectorAll("h2 + ol > li")];
      for (const li of rows) {
        const card = li.closest("ol");
        if (getComputedStyle(card).borderTopWidth === "0px") return "a group of Acknowledgements has no card round it";
        const box = card.getBoundingClientRect();
        const words = [...li.querySelector("button").children].map((c) => c.getBoundingClientRect());
        const left = Math.round(words[0].left - box.left);
        const right = Math.round(box.right - words.at(-1).right);
        if (left < 12 || right < 12) {
          return `"${li.textContent.trim().slice(0, 40)}" stands ${left} px from its card's left edge and ${right} px from its right, not 12 or more`;
        }
      }
      return rows.length ? null : "Acknowledgements shows no row";
    });
  },
  // ["pick model"]     opens the list of what finds clips and, if it
  //                    opens, picks the last model here it does not have
  //                    picked
  async "pick model"(walk) {
    const { page } = walk;
    await page.evaluate(() => (window.__shook = []));
    await page.locator(".card.finding button.pick").first().click();
    await settle(page);
    const other = page.locator('[role="option"]:not([data-selected])');
    if (await other.count()) {
      await other.last().click();
      await settle(page);
    }
  },
  // ["remove model", which]
  //                    presses the bin of the "language" model on this
  //                    machine, under Downloaded models, or of the
  //                    "speech" model
  async "remove model"(walk, arg) {
    const { page } = walk;
    await page.evaluate(() => (window.__shook = []));
    if (arg === "language") {
      const fold = page.locator(".card.finding button.disclose-row");
      if ((await fold.getAttribute("aria-expanded")) !== "true") await fold.click();
      await page.locator('.card.finding button.bin[aria-label^="Remove "]').first().click();
    } else {
      await page.locator('.speech button.bin[aria-label^="Remove "]').first().click();
    }
    await settle(page);
  },
  // ["refused", card]  the gesture before it shook the "finding" or the
  //                    "speech" card and opened no list and no box, the
  //                    settings name the models they named at "settings",
  //                    and with "finding" the line under Find clips with
  //                    says a search is finding clips with it
  async refused(walk, arg) {
    const { page } = walk;
    await settle(page);
    const seen = await page.evaluate(() => ({
      shook: window.__shook ?? [],
      box: !!document.querySelector("dialog[open]"),
      list: !!document.querySelector('[role="listbox"]'),
      line: document.querySelector(".card.finding .words .line")?.textContent.trim() ?? "",
    }));
    const now = await modelsNamed(page);
    if (!seen.shook.some((c) => c.split(" ").includes(arg))) walk.wrong = `the ${arg} card did not shake, shaken: ${JSON.stringify(seen.shook)}`;
    else if (seen.box) walk.wrong = "a box opened";
    else if (seen.list) walk.wrong = "the list opened";
    else if (JSON.stringify(now) !== JSON.stringify(walk.named)) walk.wrong = `the settings name ${JSON.stringify(now)}, not ${JSON.stringify(walk.named)}`;
    else if (arg === "finding" && seen.line !== "A search is finding clips with it now.") walk.wrong = `the line under Find clips with says "${seen.line}"`;
  },
  // ["asks"]           the gesture before it opened the box asking whether
  //                    to remove, which Escape closes again
  async asks(walk) {
    const { page } = walk;
    const box = page.locator("dialog[open]");
    if (!(await box.count())) {
      walk.wrong = "no box asked whether to remove";
      return;
    }
    await page.keyboard.press("Escape");
    await settle(page);
  },
  // ["changed"]        the settings name another model than at "settings"
  async changed(walk) {
    const { page } = walk;
    const now = await modelsNamed(page);
    if (JSON.stringify(now) === JSON.stringify(walk.named)) walk.wrong = `the settings still name ${JSON.stringify(now)}`;
  },
  // ["resize"]         drags the app's height and then its width a pixel at
  //                    a time, down to the app's smallest, and on every step every part of the
  //                    workspace moves only one way the whole drag, stands
  //                    on a whole pixel, and meets the app's edges, and the
  //                    two sides beside the picture, measured from the
  //                    app's edges, are the same width, or a pixel apart,
  //                    and stand still while only the height changes
  async resize(walk) {
    const { page } = walk;
    walk.wrong = await resizeProblem(page);
  },
  // ["from the app", n]
  //                    the picture comes from the Go side, and n clicks
  //                    along the clip timeline each show the frame that
  //                    holds the playhead
  async "from the app"(walk, arg) {
    const { page, watch } = walk;
    const fps = await rate(page);
    const { segments } = await engineState(page, watch.at);
    const first = segments[0].start;
    const last = segments[segments.length - 1].end;
    for (let i = 1; i <= arg && !walk.wrong; i++) {
      const at = first + ((last - first) * i) / (arg + 1);
      if (!(await seekTo(page, at, fps))) walk.wrong = `the video preview never showed the frame at ${at.toFixed(2)}`;
    }
    const streams = await page.evaluate(() => window.__appFrames?.stats.streams ?? 0);
    if (!walk.wrong && !streams) walk.wrong = "the picture did not come from the Go side";
    const trouble = await page.evaluate(() => document.querySelector(".screen")?.textContent.trim() ?? "");
    if (!walk.wrong && /decod/i.test(trouble)) walk.wrong = `the video preview says "${trouble}"`;
  },
  // ["licence row whole"]
  //                    at several widths of the app the licence row's
  //                    field, Unlock and bin stand on whole pixels, with
  //                    "refused" a key that is none is typed and Unlock
  //                    refuses it, and the line that says why wraps to
  //                    more lines at one width than at another
  async "licence row whole"(walk, arg) {
    const { page } = walk;
    const was = page.viewportSize();
    const lines = new Set();
    const problems = [];
    if (arg === "refused") {
      await page.locator("input.licence-key").fill("FF1-not-a-key");
      await page.locator("input.licence-key").press("Enter");
      await settle(page);
      const said = await page.evaluate(() => !!document.querySelector(".licence-line.error")?.textContent.trim());
      if (!said) problems.push("the licence row's line says no key was refused");
      // The refusal is the call failing, which is what was asked for.
      await page.evaluate(() => {
        for (const c of window.__calls) if (c.name === "SaveLicence" && c.failed) c.failed = null;
      });
    }
    // Below the app's least width, 960, too: the line a link leaves
    // with its key IDs is long enough to wrap at any width, and the
    // refusal the bridge can give wraps only there.
    // The page has its new width, and two frames have been drawn
    // since, so what is measured is laid out at it.
    const resized = async (width) => {
      await page.evaluate(
        (w) =>
          new Promise((done) => {
            if (innerWidth === w) return done();
            const heard = () => {
              if (innerWidth !== w) return;
              removeEventListener("resize", heard);
              done();
            };
            addEventListener("resize", heard);
          }),
        width,
      );
      await nextPaint(page, 2);
    };
    for (const width of [1728, 1200, 960, 800, 700, 600]) {
      await page.setViewportSize({ width, height: was.height });
      await resized(width);
      const m = await page.evaluate(() => {
        const line = document.querySelector(".licence-line");
        const field = line?.closest(".item");
        const controls = [...(field?.querySelectorAll("input, button.unlock, button.bin") ?? [])];
        return {
          lines: line ? Math.round(line.getBoundingClientRect().height / parseFloat(getComputedStyle(line).lineHeight)) : 0,
          row: field?.getBoundingClientRect().top ?? 0,
          tops: controls.map((c) => [c.matches("input") ? "field" : c.matches(".unlock") ? "Unlock" : "bin", c.getBoundingClientRect().top]),
        };
      });
      lines.add(m.lines);
      if (!m.tops.length) problems.push(`at ${width} px the licence row has no field, Unlock or bin`);
      for (const [name, top] of m.tops) {
        // Narrower than the app gets, what stands above the row can
        // wrap onto half a pixel of its own on CI's fonts, so there
        // only the row's own half pixel is looked for.
        const at = width >= 960 ? top : top - m.row;
        if (at !== Math.round(at)) problems.push(`at ${width} px, with the line on ${m.lines} lines, the ${name} stands between two pixels, its top at ${top}, ${top - m.row} into its row`);
      }
    }
    await page.setViewportSize(was);
    await resized(was.width);
    if (arg === "refused" && lines.size < 2) problems.push(`the refusal stood on ${[...lines].join(", ")} lines at every width, so a wrapped line was never measured`);
    if (problems.length) walk.wrong = problems.join("\n");
  },
  // ["app size", w, h] makes the app w by h pixels
  async "app size"(walk, arg) {
    const { page } = walk;
    await page.setViewportSize({ width: arg[0], height: arg[1] });
    await settle(page);
  },
  // ["fits the app", [w, h]]
  //                    makes the app w by h pixels, and the workspace fits
  //                    it: nothing scrolls, the three columns and the clip
  //                    panel under them reach the app's edge, a gap
  //                    between each, with no strip of nothing anywhere,
  //                    the video preview grows until it runs out of width
  //                    or height, and every button and field lies whole
  //                    inside the app and inside what clips it, unless it
  //                    is in a list or a column that scrolls to it
  async "fits the app"(walk, arg) {
    const { page } = walk;
    const [width, height] = arg;
    await page.setViewportSize({ width, height });
    await settle(page);
    walk.wrong = await page.evaluate(fitProblem);
  },
  // ["square buttons"] every button in the row under the clip timeline
  //                    that wears an icon or a key's letter is as wide as
  //                    it is tall, and as tall as every control
  async "square buttons"(walk) {
    const { page } = walk;
    const got = await page.evaluate(() => {
      const h = parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--control-h"));
      return { h, sizes: [...document.querySelectorAll(".detail .row .glyph")].map((b) => [b.title || b.textContent.trim(), b.getBoundingClientRect().width, b.getBoundingClientRect().height]) };
    });
    const off = got.sizes.filter(([, w, h]) => Math.abs(w - got.h) > 0.01 || Math.abs(h - got.h) > 0.01);
    if (got.sizes.length < 5) walk.wrong = `the row under the clip timeline has ${got.sizes.length} buttons with an icon, not the row`;
    else if (off.length) walk.wrong = `${off.length} of ${got.sizes.length} buttons are not ${got.h} by ${got.h}, the first, "${off[0][0].slice(0, 40)}", is ${off[0][1].toFixed(2)} by ${off[0][2].toFixed(2)}`;
  },
  // ["restart"]        closes the app and opens it again on the episode
  async restart(walk) {
    const { page } = walk;
    const path = await page.evaluate(() => [...window.__calls].reverse().find((c) => c.name === "Clips")?.args[0]);
    await control(url, "/reopen");
    await page.reload();
    await page.locator("aside li", { hasText: path.split("/").pop() }).first().waitFor();
    await fromSidebar(page, () => page.locator("aside li", { hasText: path.split("/").pop() }).first().click());
    await settle(page);
  },
  // ["updates", how]   opens the Updates page from the sidebar on a build
  //                    as how says: from the channel how.from, or made on
  //                    the Mac without it, with how.stored picked by a
  //                    build before it, and the channels how.list on the
  //                    list, each with a newer build, and those in
  //                    how.conflict said to no longer merge into main,
  //                    or with how.offline the list read from a port
  //                    nothing listens on
  async updates(walk, arg) {
    const { page } = walk;
    const q = new URLSearchParams({
      from: arg.from ?? "",
      stored: arg.stored ?? "",
      list: arg.list ?? "",
      conflict: arg.conflict ?? "",
      offline: arg.offline ? "1" : "",
    });
    await control(url, `/updates?${q}`);
    await fromSidebar(page, () => page.locator("aside").getByText("Updates", { exact: true }).first().click());
    await page.locator("#channel").waitFor();
    await settle(page);
  },
  // ["follow", name]   picks the channel of this name in the Updates
  //                    page's list, and reads the page in the next frame
  async follow(walk, arg) {
    const { page } = walk;
    await page.locator("#channel").click();
    await page.getByRole("option", { name: arg, exact: true }).click();
    const read = await page.evaluate(
      (src) => new Promise((done) => requestAnimationFrame(() => done(new Function(`return (${src})()`)()))),
      readUpdates.toString(),
    );
    walk.picked = { name: arg, ...read };
  },
  // ["channel", name]  the Updates page's list says this, in ten seconds
  //                    at most
  async channel(walk, arg) {
    const { page } = walk;
    const now = await updatesComeTo(page, (u) => u.channel === arg);
    if (now.channel !== arg) walk.wrong = `the list says ${now.channel || "nothing"}, not ${arg}`;
  },
  // ["choices", names, picked]
  //                    the list offers these channels and nothing else, in
  //                    this order, with this one picked, or none with ""
  async choices(walk, arg) {
    const { page } = walk;
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
    if (JSON.stringify(got) !== JSON.stringify(names)) walk.wrong = `the list offers ${JSON.stringify(got)}, not ${JSON.stringify(names)}`;
    else if (JSON.stringify(on) !== JSON.stringify(want ? [want] : [])) {
      walk.wrong = `the list has ${on.length ? on.join(", ") : "nothing"} picked, not ${want || "nothing"}`;
    }
  },
  // ["status", head, more]
  //                    the line under the build comes to say this head, in
  //                    ten seconds at most, and more somewhere after it
  async status(walk, arg) {
    const { page } = walk;
    const [head, more] = [arg].flat();
    const now = await updatesComeTo(page, (u) => u.head === head);
    if (now.head !== head) walk.wrong = `the line under the build says ${now.head || "nothing"}, not ${head}`;
    else if (more && !now.more.includes(more)) walk.wrong = `under ${head} it says ${JSON.stringify(now.more)}, not ${JSON.stringify(more)}`;
  },
  // ["build mark", what]
  //                    the line under the build starts with this mark,
  //                    "check", "triangle" or "dot", and ", warning" when
  //                    it wears the colour of a warning
  async "build mark"(walk, arg) {
    const { page } = walk;
    const now = await updatesComeTo(page, (u) => u.mark === arg);
    if (now.mark !== arg) walk.wrong = `the line under the build starts with ${now.mark || "no mark"}, not ${arg}`;
  },
  // ["update detail", starts]
  //                    under the line under the build, what the system
  //                    said starts with these words, on one line at the
  //                    height of a control however long it is, on one row
  //                    with Try again, starting where the words start and
  //                    Try again ending under the list of channels, and its
  //                    button puts it on the clipboard and says Copied
  async "update detail"(walk, arg) {
    const { page } = walk;
    const detail = page.locator(".card.asks .detail");
    await detail.waitFor();
    const seen = await detail.evaluate((d) => {
      const code = d.querySelector("code");
      const row = d.closest(".item");
      const box = (el) => el?.getBoundingClientRect() ?? null;
      const act = box(row?.querySelector("button.act"));
      const line = parseFloat(getComputedStyle(code).lineHeight);
      return {
        text: (code?.textContent ?? "").trim(),
        height: d.getBoundingClientRect().height,
        control: parseFloat(getComputedStyle(document.documentElement).getPropertyValue("--control-h")),
        // The field has the height of a control whatever is in it, so
        // the words are measured: one line high, wider than they are
        // shown and so scrolling, inside the field.
        code: box(code),
        lines: code ? Math.round(code.getBoundingClientRect().height / line) : 0,
        scrolls: !!code && code.scrollWidth > code.clientWidth,
        // Playwright's Chromium hides scroll bars, so one that a Mac
        // set to show them always would draw is not there to measure.
        // What keeps it away is read instead.
        bar: code ? getComputedStyle(code).scrollbarWidth : "",
        field: box(d),
        act,
        words: box(row?.querySelector(".words")),
        pick: box(d.closest(".card")?.querySelector("button.pick")),
      };
    });
    if (!seen.text.startsWith(arg)) {
      walk.wrong = `what the system said is ${JSON.stringify(seen.text)}`;
      return;
    }
    if (seen.height !== seen.control) {
      walk.wrong = `what the system said is ${seen.height} px high, not one line of ${seen.control} px`;
      return;
    }
    if (seen.lines !== 1 || !seen.scrolls) {
      walk.wrong = `what the system said is ${seen.lines} lines, ${seen.scrolls ? "scrolling" : "not scrolling"}, not one line that scrolls`;
      return;
    }
    if (seen.code.top < seen.field.top || seen.code.bottom > seen.field.bottom || seen.code.left < seen.field.left || seen.code.right > seen.field.right) {
      walk.wrong = `what the system said spills out of its field`;
      return;
    }
    if (seen.bar !== "none") {
      walk.wrong = `the field may draw a scroll bar inside its line, its scrollbar-width is ${seen.bar}`;
      return;
    }
    // Tim saw the field under the words and Try again beside them both,
    // lined up with neither (#417): the two share a row, the field
    // starting where the words start, Try again ending where the list
    // of channels above it ends.
    const { field, act, words, pick } = seen;
    if (!act) {
      walk.wrong = "there is no Try again in the row of what the system said";
      return;
    }
    if (act.top !== field.top || act.bottom !== field.bottom) {
      walk.wrong = `Try again spans ${act.top} to ${act.bottom} px, the field ${field.top} to ${field.bottom} px, not one row`;
      return;
    }
    if (act.left <= field.right) {
      walk.wrong = `Try again starts at ${act.left} px, inside the field that ends at ${field.right} px`;
      return;
    }
    if (field.left !== words.left) {
      walk.wrong = `the field starts at ${field.left} px, the words above it at ${words.left} px`;
      return;
    }
    if (pick && act.right !== pick.right) {
      walk.wrong = `Try again ends at ${act.right} px, the list of channels above it at ${pick.right} px`;
      return;
    }
    // The answer to the copy is the signal the clipboard was written.
    const copiedOnGo = page.waitForResponse((r) => {
      if (!r.url().includes("/call")) return false;
      try {
        return JSON.parse(r.request().postData() ?? "{}").name?.split(".").pop() === "Copy";
      } catch {
        return false;
      }
    });
    // The button says Copied in the frame of the click, and goes back
    // a moment later, so it is read in the same call as the click, on
    // the frame after it, never in a later call that may come too late.
    const label = await detail.locator("button.copy").evaluate(async (b) => {
      b.click();
      await new Promise((r) => requestAnimationFrame(() => r()));
      return b.getAttribute("aria-label");
    });
    await copiedOnGo;
    const onClipboard = await control(url, "/clipboard");
    if (label !== "Copied") walk.wrong = `the copy button says ${label} after the click, not Copied`;
    else if (onClipboard !== seen.text) walk.wrong = `the clipboard holds ${JSON.stringify(onClipboard)}, not what the system said`;
  },
  // ["rail mark", what]
  //                    Updates at the foot of the sidebar wears this mark
  //                    now, in the frame the line under the build is read
  //                    in, since both are drawn from the one state: "none",
  //                    or a "dot" and its colour, ", accent", ", warning"
  //                    or ", failure", read from what is painted
  async "rail mark"(walk, arg) {
    const { page } = walk;
    const now = await page.evaluate(railMark);
    if (now !== arg) walk.wrong = `Updates in the sidebar wears ${now}, not ${arg}`;
  },
  // ["warned"]         the list wears the colour of a warning
  async warned(walk) {
    const { page } = walk;
    const now = await updatesComeTo(page, (u) => u.warned);
    if (!now.warned) walk.wrong = `the list does not wear the colour of a warning`;
  },
  // ["fits"]           the list's trigger is as wide as the channel it
  //                    names, with no room kept for a longer one
  async fits(walk) {
    const { page } = walk;
    const now = await updatesPage(page);
    if (now.slack < 0 || now.slack > 60) walk.wrong = `the list's trigger keeps ${Math.round(now.slack)} px beside ${now.channel}, room for a longer name`;
  },
  // ["relaunch", how]  the line under the build offers Relaunch, "waits"
  //                    greyed out with its title saying it waits for the
  //                    search, and "ready" usable again, within a minute,
  //                    and the line wears the card's own background
  async relaunch(walk, arg) {
    const { page } = walk;
    const waits = arg === "waits";
    const read = () =>
      page.evaluate(() => {
        const line = document.querySelectorAll(".card.asks > .item")[1];
        const button = line?.querySelector("button");
        const card = line?.closest(".card");
        return {
          words: (button?.textContent ?? "").trim(),
          off: !!button?.disabled,
          title: button?.title ?? "",
          // A row with nothing chosen in it lets the card show through.
          lit: !!line && getComputedStyle(line).backgroundColor !== "rgba(0, 0, 0, 0)" && getComputedStyle(line).backgroundColor !== getComputedStyle(card).backgroundColor,
        };
      });
    const now = await seenUntil(page, url, read, (now) => now.words === "Relaunch" && now.off === waits);
    if (now.words !== "Relaunch") walk.wrong = `the line under the build offers ${now.words || "nothing"}, not Relaunch`;
    else if (now.off !== waits) walk.wrong = waits ? "Relaunch can be pressed while a search runs" : "Relaunch stayed greyed out after the search ended";
    else if (waits && now.title !== "Waits until the search is done") walk.wrong = `Relaunch's title says ${JSON.stringify(now.title)}`;
    else if (now.lit) walk.wrong = "the line under the build wears a grey of its own, which reads as greyed out";
  },
  // ["no check"]       the line under the build offers no button
  async "no check"(walk) {
    const { page } = walk;
    const now = await updatesPage(page);
    if (now.button) walk.wrong = `the line under the build offers ${now.button}`;
  },
  // ["at once", head]  in the frame after the pick, the list said the
  //                    channel picked and the line under it this head
  async "at once"(walk, arg) {
    if (walk.picked?.channel !== walk.picked?.name) walk.wrong = `in the frame after picking ${walk.picked?.name}, the list said ${walk.picked?.channel || "nothing"}`;
    else if (walk.picked.head !== arg) walk.wrong = `in the frame after the pick, the line under the build said ${walk.picked.head || "nothing"}, not ${arg}`;
  },
};

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
    // In the head, so the read changes nothing a wait on the page's
    // body sees, see seenUntil in bridge.mjs.
    document.head.append(probe);
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

// The mark on Updates in the sidebar, by the colour its dot is painted
// in, "none" without one. Tim followed a pull request that closed and the
// Updates page wore a warning while the sidebar wore nothing, #406.
function railMark() {
  const button = [...document.querySelectorAll("aside button.nav")].find((b) =>
    (b.textContent ?? "").includes("Updates"),
  );
  const dot = button?.querySelector(".mark .dot");
  if (!dot || getComputedStyle(dot).display === "none") return "none";
  const paint = (v) => {
    const probe = document.createElement("span");
    probe.style.color = `var(${v})`;
    document.body.append(probe);
    const c = getComputedStyle(probe).color;
    probe.remove();
    return c;
  };
  const colour = getComputedStyle(dot).backgroundColor;
  const named = { "--accent": "accent", "--warn": "warning", "--err": "failure" };
  for (const [v, name] of Object.entries(named)) if (colour === paint(v)) return `dot, ${name}`;
  return `dot, ${colour}`;
}

// What the Updates page says once it says what ok wants, see seenUntil.
const updatesComeTo = (page, ok) => seenUntil(page, url, () => updatesPage(page), ok);

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

// What is wrong with how the workspace fits the app at its size, or null,
// see ["fits the app"]. In the page.
function fitProblem() {
  const W = innerWidth;
  const H = innerHeight;
  const at = `at ${W} by ${H}`;
  const root = document.documentElement;
  if (root.scrollWidth > W || root.scrollHeight > H) return `${at} the app scrolls, it is ${root.scrollWidth} by ${root.scrollHeight}`;
  const token = (n) => parseFloat(getComputedStyle(root).getPropertyValue(n));
  const [gap, edge] = [token("--gap"), token("--edge")];
  const box = (s) => document.querySelector(s)?.getBoundingClientRect();
  const main = box("main");
  const stage = box(".stage");
  const detail = box(".detail");
  if (!main || !stage || !detail) return `${at} there is no workspace`;
  const [settings, middle, clips] = [...document.querySelector(".stage").children].map((c) => c.getBoundingClientRect());
  const viewer = box(".viewer");
  const range = box(".track[aria-label='The range picker']");
  const timeline = box(".clip-timeline");
  if (!settings || !middle || !clips || !viewer || !range || !timeline) return `${at} a part of the workspace is missing`;
  // Each pair is what it is and what it should be, a strip of nothing or
  // a part pushed off the app wherever they differ.
  const lines = {
    "the columns' left edge after the rail": [stage.left, main.left + gap],
    // The settings column reaches past its edge by its padding, see
    // .settings in Episode.svelte, so its words start at the edge.
    "the settings' left edge": [settings.left + parseFloat(getComputedStyle(document.querySelector(".stage").firstElementChild).paddingLeft), stage.left],
    "the columns' top under the bar": [stage.top, main.top + gap],
    "the columns' right edge": [stage.right, W - edge],
    "the clip panel's left edge": [detail.left, stage.left],
    "the clip panel's right edge": [detail.right, W - edge],
    "the clip panel under the columns": [detail.top, stage.bottom + gap],
    "the clip panel's foot": [detail.bottom, H - edge],
    "the video preview after the settings": [middle.left, settings.right + gap],
    "the clips after the video preview": [clips.left, middle.right + gap],
    "the clips' right edge": [clips.right, stage.right],
    "the settings' foot": [settings.bottom, stage.bottom],
    "the clips' foot": [clips.bottom, stage.bottom],
    "the video preview's top": [viewer.top, stage.top],
    "the range picker under the video preview": [range.top, viewer.bottom + gap],
    "the range picker's foot": [range.bottom, stage.bottom],
    "the clip timeline's left edge": [timeline.left, detail.left],
    "the clip timeline's right edge": [timeline.right, detail.right],
  };
  for (const [what, [is, want]] of Object.entries(lines)) {
    if (Math.abs(is - want) > 0.5) return `${at} ${what} is at ${is}, not ${want}`;
  }
  // The picture grows until it runs out of width or height, whichever
  // comes first, and keeps its shape.
  if (Math.abs(viewer.width - middle.width) > 0.5 && Math.abs(viewer.height - (range.top - gap - stage.top)) > 0.5) {
    return `${at} the video preview is ${viewer.width} by ${viewer.height} in a column ${middle.width} wide with ${range.top - gap - stage.top} of height, a strip of nothing on both`;
  }
  // Whatever clips a button or a field, short of a list or a column that
  // scrolls to it, holds it whole. The grips on the tracks are not among
  // them: they stand where their moment is, and a track zoomed in shows
  // only some of them, by design.
  const scrolls = (e) => {
    const o = getComputedStyle(e);
    return /(auto|scroll)/.test(o.overflowX + o.overflowY) && (e.scrollHeight > e.clientHeight || e.scrollWidth > e.clientWidth);
  };
  const clipsIt = (e) => getComputedStyle(e).overflowX !== "visible" || getComputedStyle(e).overflowY !== "visible";
  for (const c of document.querySelectorAll("main button, main input")) {
    const b = c.getBoundingClientRect();
    if (!b.width || !b.height || getComputedStyle(c).visibility === "hidden") continue;
    const name = c.getAttribute("aria-label") || c.title || c.textContent.trim().slice(0, 30) || c.className;
    let cut = null;
    let scrolled = false;
    for (let e = c.parentElement; e && e !== document.body; e = e.parentElement) {
      if (scrolls(e)) {
        scrolled = true;
        break;
      }
      if (!clipsIt(e)) continue;
      const p = e.getBoundingClientRect();
      if (b.left < p.left - 0.5 || b.right > p.right + 0.5 || b.top < p.top - 0.5 || b.bottom > p.bottom + 0.5) cut ??= e;
    }
    if (scrolled) continue;
    if (b.left < -0.5 || b.top < -0.5 || b.right > W + 0.5 || b.bottom > H + 0.5) return `${at} "${name}" lies past the app's edge, from ${b.left},${b.top} to ${b.right},${b.bottom}`;
    if (cut) return `${at} "${name}" is cut off by the ${cut.tagName.toLowerCase()}.${cut.className.split(" ")[0]} it lies in`;
  }
  return null;
}

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
