// Every way of waiting on the clock that ESLint refuses in a walk.
export async function waits(page, main) {
  await page.waitForTimeout(100);
  await main.waitForTimeout(100);
  await new Promise((done) => setTimeout(done, 100));
  return Date.now() - performance.now();
}
