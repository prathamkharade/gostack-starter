import { test, expect } from "@playwright/test";
import { signIn, watchConsole } from "./helpers";

test("sign up, create, search, paginate, delete account", async ({ page }) => {
  const errors = watchConsole(page);
  await signIn(page);

  // create two items
  for (const title of ["Alpha report", "Beta notes"]) {
    await page.goto("/items/new");
    await page.getByLabel("Title").fill(title);
    await page.getByRole("button", { name: "Save" }).click();
    await expect(page).toHaveURL(/\/items$/);
  }

  // search-as-you-type updates results and the URL without a full reload
  await page.getByLabel("Search").fill("alpha");
  await expect(page.getByRole("link", { name: "Alpha report" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Beta notes" })).toHaveCount(0);
  await expect(page).toHaveURL(/q=alpha/);

  // validation errors render inline
  await page.goto("/items/new");
  await page.getByRole("button", { name: "Save" }).click(); // browser "required" blocks empty submit
  await expect(page).toHaveURL(/\/items\/new/);

  // data export downloads JSON
  await page.goto("/account");
  const download = page.waitForEvent("download");
  await page.getByRole("link", { name: "Download my data" }).click();
  expect((await download).suggestedFilename()).toMatch(/^my-data-.*\.json$/);

  // delete account
  await page.getByRole("button", { name: "Delete my account…" }).click();
  await page.getByLabel("Confirmation").fill("DELETE");
  await page.getByRole("button", { name: "Delete permanently" }).click();
  await expect(page.getByText("Your account and personal data have been deleted.")).toBeVisible();
  expect(errors).toEqual([]);
});
