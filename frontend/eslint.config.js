// ESLint holds one rule here and nothing else: a test waits on a signal,
// never on the clock, see CLAUDE.md. It reads the walks and the
// interface's own tests, and has no rule about style.
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

const why = "A test waits on a signal, never on the clock. See CLAUDE.md and docs/TESTING.md.";

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
    files: ["preview/walks/*.mjs"],
    languageOptions: { ecmaVersion: "latest", sourceType: "module" },
    plugins: { playwright },
    rules: {
      ...clock,
      "playwright/no-wait-for-timeout": "error",
      // The plugin knows a page by its name, page or frame. A walk that
      // drives a second window calls it main, so any other waitForTimeout
      // is caught here. A timeout or a polling handed to a wait, like
      // waitForFunction or waitFor, is a deadline or a look on the clock,
      // and the -timeout of make walks ends a wait that never ends.
      "no-restricted-syntax": [
        "error",
        {
          selector:
            "CallExpression[callee.property.name='waitForTimeout']:not([callee.object.name=/(^(page|frame)|(Page|Frame)$)/])",
          message: why,
        },
        { selector: "CallExpression > ObjectExpression > Property[key.name=/^(timeout|polling)$/]", message: why },
      ],
    },
  },
  {
    files: ["src/**/*.test.ts"],
    languageOptions: { parser: typescript, ecmaVersion: "latest", sourceType: "module" },
    rules: clock,
  },
];
