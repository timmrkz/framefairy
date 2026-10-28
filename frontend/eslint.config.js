// ESLint holds two rules here. A test waits on a signal, never on the
// clock, see CLAUDE.md, and in the walks and the probes one more: no name
// is used that nothing declares or imports. The walks are split over a
// folder for each part of the app, and a step that uses what it does not
// import would otherwise fail only once a walk reaches it. It reads the
// walks, the probes in preview/ and preview/frames/, and the interface's
// own tests, and has no rule about style. And the interface's scripts open
// no microphone, camera or screen and read no clipboard by themselves,
// issue #369, see What is never signed in docs/UPDATES.md. A component
// is not read, for ESLint here has no parser for Svelte, and there macOS
// is the wall: the app gives no reason for the camera or the screen, so
// they never open, and WebKit asks before a page reads the clipboard.
//
// What waited on the clock before the rule was enforced is listed in
// eslint-suppressions.json, file by file and rule by rule, so only a new
// one fails. Taking one out makes ESLint fail until
// npm run lint -- --prune-suppressions shortens the list. The list only
// ever shrinks. What it counts for the walks are reads of the clock that
// measure, and the stalls and late answers a step puts on the app on
// purpose. A clock the test controls is not the clock: Playwright's
// page.clock and Vitest's vi.useFakeTimers and vi.advanceTimersByTime are
// not touched by any rule below.
import playwright from "eslint-plugin-playwright";
import typescript from "@typescript-eslint/parser";
import globals from "globals";

const why = "A test waits on a signal, never on the clock. See CLAUDE.md and docs/TESTING.md.";
const given = "The app takes no input it is not given. See What is never signed in docs/UPDATES.md.";

// The same four reads of the clock, by any name they are reached by.
const clock = {
  "no-restricted-globals": [
    "error",
    { name: "setTimeout", message: why },
    { name: "setInterval", message: why },
  ],
  "no-restricted-properties": [
    "error",
    { object: "Date", property: "now", message: why },
    { object: "performance", property: "now", message: why },
    { object: "window", property: "setTimeout", message: why },
    { object: "window", property: "setInterval", message: why },
    { object: "globalThis", property: "setTimeout", message: why },
    { object: "globalThis", property: "setInterval", message: why },
  ],
  // The timers Node hands out by import are the same clock under the
  // same names.
  "no-restricted-imports": [
    "error",
    { name: "timers", message: why },
    { name: "node:timers", message: why },
    { name: "timers/promises", message: why },
    { name: "node:timers/promises", message: why },
  ],
};

export default [
  {
    files: ["preview/walks/**/*.mjs", "preview/*.mjs", "preview/frames/*.mjs"],
    // Node's names, and the page's, for what a walk hands the page to run.
    languageOptions: { ecmaVersion: "latest", sourceType: "module", globals: { ...globals.node, ...globals.browser } },
    plugins: { playwright },
    rules: {
      ...clock,
      "no-undef": "error",
      "playwright/no-wait-for-timeout": "error",
      // The plugin knows a page by its name, page or frame. A walk that
      // drives a second window calls it main, so any other waitForTimeout
      // is caught here. A timeout or a polling handed to a wait, like
      // waitForFunction or waitFor, is a deadline or a look on the clock,
      // and the -timeout of make walks ends a wait that never ends, see
      // noDeadlines in walks/bridge.mjs. Options kept in a variable, or
      // spread in from one, are not seen here.
      "no-restricted-syntax": [
        "error",
        {
          selector:
            "CallExpression[callee.property.name='waitForTimeout']:not([callee.object.name=/(^(page|frame)|(Page|Frame)$)/])",
          message: why,
        },
        { selector: "CallExpression > ObjectExpression > Property[key.name=/^(timeout|polling)$/]", message: why },
        { selector: "CallExpression > ObjectExpression > Property[key.value=/^(timeout|polling)$/]", message: why },
      ],
    },
  },
  {
    files: ["src/**/*.ts"],
    languageOptions: { parser: typescript, ecmaVersion: "latest", sourceType: "module" },
    rules: {
      // Wails' Clipboard.Text reads the system's clipboard, as the
      // browser's clipboard.read and readText do.
      "no-restricted-syntax": [
        "error",
        { selector: "MemberExpression[property.name=/^get(User|Display)Media$/]", message: given },
        { selector: "MemberExpression[object.property.name='clipboard'][property.name=/^read/]", message: given },
        { selector: "MemberExpression[object.name='clipboard'][property.name=/^read/]", message: given },
        { selector: "MemberExpression[object.name='Clipboard'][property.name='Text']", message: given },
      ],
    },
  },
  {
    files: ["src/**/*.test.ts"],
    languageOptions: { parser: typescript, ecmaVersion: "latest", sourceType: "module" },
    rules: {
      ...clock,
      // Vitest's waitFor and waitUntil look again every 50 ms until a
      // deadline of a second: a poll on the clock.
      "no-restricted-properties": [
        ...clock["no-restricted-properties"],
        { object: "vi", property: "waitFor", message: why },
        { object: "vi", property: "waitUntil", message: why },
      ],
    },
  },
];
