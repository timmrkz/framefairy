// A walk that waits on the page, or on Playwright's clock, passes.
export async function signals(page) {
  await page.clock.install();
  await page.clock.runFor(1000);
  await page.waitForFunction(() => document.body.dataset.ready === "yes");
  await page.locator("menu").waitFor({ state: "detached" });
}
