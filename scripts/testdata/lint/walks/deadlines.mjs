// A wait handed a deadline or a look on the clock, which ESLint refuses
// in a walk: four of them.
export async function deadlines(page) {
  await page.waitForFunction(() => document.body.dataset.ready === "yes", null, { timeout: 1500, polling: 100 });
  await page.locator("menu").isVisible({ timeout: 300 });
  await page.waitForEvent("load", { "timeout": 100 });
}
