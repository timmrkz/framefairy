// What a walk waits on in the page, said to it as the wait goes on: each
// time a wait looks again and has to wait on, it says for what, once for
// as long as that stays the same. A walk that waits for something that
// never comes is stopped when the test's time runs out, and then says the
// last of these, see noDeadlines in walks/bridge.mjs and sequencer in
// bridge_test.go. Only a page a program drives says it, so the bridge
// opened by hand in a browser stays quiet.
let said = "";

export function waitsOn(what: string) {
  if (!navigator.webdriver || what === said) return;
  said = what;
  console.debug(`\x1ewaits ${what}`);
}
