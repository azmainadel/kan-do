import { expect, test, type Page } from "@playwright/test";

// One board, used in order, like a person would: each step builds on the last.
test.describe.configure({ mode: "serial" });

let page: Page;
test.beforeAll(async ({ browser }) => {
  page = await browser.newPage();
  await page.goto("/");
});

const column = (status: string) => page.locator(`section[aria-label="${status}"]`);
const card = (title: string) => page.locator("[data-id]", { hasText: title });
const panel = () => page.locator("#task-panel");

test("creates a project from the sidebar", async () => {
  await expect(page.getByText("Welcome to kan-do")).toBeVisible();
  await page.getByRole("button", { name: "New project" }).click();
  await page.getByPlaceholder("slug, e.g. api").fill("docs");
  await page.getByPlaceholder("Name (optional)").fill("Docs site");
  await page.getByRole("button", { name: "Create" }).click();
  await expect(page.getByRole("heading", { name: "Docs site" })).toBeVisible();
  await expect(page).toHaveURL(/#docs$/);
});

test("adds a task with n, title first, then the description", async () => {
  await page.keyboard.press("n");
  const title = panel().getByLabel("Title");
  await expect(title).toBeFocused();
  await page.keyboard.type("Write the install guide");
  await page.keyboard.press("Enter");
  await expect(panel().getByLabel("Description")).toBeFocused();
  await page.keyboard.type("Covers **curl** and `go install`");
  await page.getByRole("button", { name: "Close", exact: true }).click();
  await expect(panel()).toBeHidden();
  await expect(card("Write the install guide")).toContainText("Covers curl and go install");
});

test("a new task closed untouched is deleted again", async () => {
  await page.keyboard.press("n");
  await expect(panel().getByLabel("Title")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(panel()).toBeHidden();
  await expect(card("New task")).toHaveCount(0);
});

test("edits tags and comments in the panel", async () => {
  await card("Write the install guide").click();
  await panel().getByLabel("Tags").click();
  await page.keyboard.type("Docs, v1");
  await page.keyboard.press("Enter");
  await expect(card("Write the install guide")).toContainText("docs");
  await expect(card("Write the install guide")).toContainText("v1");

  await panel().getByRole("textbox", { name: "Comment" }).fill("First draft is up.");
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(panel().getByText("First draft is up.")).toBeVisible();
  await expect(panel().getByText("me", { exact: true })).toBeVisible();
  // The first Esc leaves the comment box, the second closes the panel.
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await expect(panel()).toBeHidden();
});

test("drags a card to another column", async () => {
  const from = await card("Write the install guide").boundingBox();
  const to = await column("doing").boundingBox();
  if (!from || !to) throw new Error("no layout");
  await page.mouse.move(from.x + 20, from.y + 15);
  await page.mouse.down();
  // Small steps, so the drag starts and the column notices the card over it.
  await page.mouse.move(from.x + 40, from.y + 25, { steps: 5 });
  await page.mouse.move(to.x + to.width / 2, to.y + 80, { steps: 15 });
  await page.mouse.up();
  await expect(column("doing").locator("[data-id]", { hasText: "Write the install guide" })).toBeVisible();
  // The move was saved, not just drawn.
  await page.reload();
  await expect(column("doing").locator("[data-id]", { hasText: "Write the install guide" })).toBeVisible();
});

test("shows an agent's change without a reload", async () => {
  const res = await page.request.post("/api/tasks", { data: { project: "docs", title: "Added by an agent" } });
  expect(res.ok()).toBeTruthy();
  await expect(column("todo").locator("[data-id]", { hasText: "Added by an agent" })).toBeVisible();
});

test("finds a task with search and opens it", async () => {
  await page.keyboard.press("/");
  await page.keyboard.type("install guide");
  await expect(page.getByRole("option", { name: /Write the install guide/ })).toBeVisible();
  await page.keyboard.press("Enter");
  await expect(panel().getByLabel("Title")).toHaveValue("Write the install guide");
});

test("deletes a task after asking", async () => {
  await panel().getByRole("button", { name: "Delete task" }).click();
  await page.getByRole("alertdialog").getByRole("button", { name: "Delete task" }).click();
  await expect(panel()).toBeHidden();
  await expect(card("Write the install guide")).toHaveCount(0);
});
