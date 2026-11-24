import { expect, Page } from "@playwright/test";

const MAILPIT = process.env.MAILPIT_URL ?? "http://localhost:8025";

// Collects console errors (including CSP violations) so tests can assert none happened.
export function watchConsole(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (m) => { if (m.type() === "error") errors.push(m.text()); });
  page.on("pageerror", (e) => errors.push(e.message));
  return errors;
}

// Reads the sign-in code from Mailpit (dev SMTP catcher).
export async function codeFor(email: string): Promise<string> {
  for (let i = 0; i < 20; i++) {
    const res = await fetch(`${MAILPIT}/api/v1/search?query=${encodeURIComponent(`to:"${email}"`)}`);
    const data = await res.json();
    const subject: string | undefined = data.messages?.[0]?.Subject;
    const code = subject?.match(/\b\d{6}\b/)?.[0];
    if (code) return code;
    await new Promise((r) => setTimeout(r, 500)); // job worker sends asynchronously
  }
  throw new Error(`no email for ${email}`);
}

export async function signIn(page: Page): Promise<string> {
  const email = `e2e-${Date.now()}-${Math.floor(Math.random() * 1e6)}@example.com`;
  await page.goto("/login");
  await page.getByLabel("Email").fill(email);
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByRole("heading", { name: "Check your email" })).toBeVisible();
  await page.getByLabel("Code").fill(await codeFor(email));
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page).toHaveURL(/\/account/);
  return email;
}
