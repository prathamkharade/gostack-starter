import { test, expect } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { watchConsole } from "./helpers";

const publicPages = ["/", "/login", "/privacy", "/terms", "/cookies", "/accessibility"];

for (const path of publicPages) {
  test(`${path} renders, has no console/CSP errors and no serious a11y issues`, async ({ page }) => {
    const errors = watchConsole(page);
    const res = await page.goto(path);
    expect(res?.status()).toBe(200);
    expect(res?.headers()["content-security-policy"]).toContain("script-src 'self'");
    await expect(page.locator("h1").first()).toBeVisible();

    const axe = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"]).analyze();
    const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    expect(serious, JSON.stringify(serious.map((v) => v.id))).toEqual([]);
    expect(errors).toEqual([]);
  });
}

test("dark mode toggle persists", async ({ page }) => {
  await page.goto("/");
  const html = page.locator("html");
  const before = await html.getAttribute("data-theme");
  await page.getByRole("button", { name: "Toggle dark mode" }).click();
  await expect(html).not.toHaveAttribute("data-theme", before ?? "");
  await page.reload();
  await expect(html).not.toHaveAttribute("data-theme", before ?? "");
});

test("SEO files exist", async ({ request }) => {
  for (const p of ["/robots.txt", "/sitemap.xml", "/llms.txt", "/.well-known/security.txt", "/manifest.webmanifest"]) {
    expect((await request.get(p)).status(), p).toBe(200);
  }
});
