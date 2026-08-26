import { expect, openApplication, openSection, test } from "./support/environment";

const savedWorkspace = {
  id: "production",
  name: "Production",
  layout: {
    split: {
      direction: "horizontal",
      ratio: 60,
      first: { pane: { id: "api-pane", alias: "bastion" } },
      second: { pane: { id: "db-pane", alias: "nas" } },
    },
  },
  focusedPaneId: "api-pane",
  createdAt: "2026-08-24T09:00:00Z",
  updatedAt: "2026-08-24T09:00:00Z",
};

test("moves workspace panes by drag and drop and saves the new placement", async ({ page, installation }) => {
  const sessions: Array<Record<string, unknown>> = [];
  const savedRequests: Record<string, unknown>[] = [];

  await page.route("**/api/v1/workspaces**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/restore")) {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ workspace: savedWorkspace }) });
      return;
    }
    if (request.method() === "PUT") {
      const savedRequest = request.postDataJSON() as Record<string, unknown>;
      savedRequests.push(savedRequest);
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ...savedWorkspace, ...savedRequest }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ workspaces: [savedWorkspace] }) });
  });

  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ streamTicket: "fixture-stream-ticket" }) });
      return;
    }
    if (request.method() === "POST") {
      const body = request.postDataJSON() as { alias?: string };
      const alias = body.alias ?? "bastion";
      const session = { id: `${alias}-session`, kind: "ssh", alias, title: alias, startedAt: "2026-08-24T09:00:00Z", state: "connected", problem: "", forwards: [] };
      if (!sessions.some((item) => item.id === session.id)) sessions.push(session);
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ session, streamTicket: "fixture-stream-ticket" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions, maxSessions: 12 }) });
  });

  await openApplication(page, installation);
  await openSection(page, "Terminal");
  await page.getByLabel("Saved workspaces").selectOption("production");
  await page.getByRole("button", { name: "Reopen" }).click();

  const panes = page.locator("[data-workspace-pane]");
  await expect(panes).toHaveCount(2);
  await expect(panes.nth(0)).toHaveAttribute("data-pane-alias", "bastion");
  await expect(panes.nth(1)).toHaveAttribute("data-pane-alias", "nas");
  const destination = await panes.nth(1).boundingBox();
  expect(destination).not.toBeNull();
  await panes.nth(0).getByRole("button", { name: /Move bastion pane/ }).dragTo(panes.nth(1), {
    targetPosition: { x: destination!.width - 8, y: destination!.height / 2 },
  });
  await expect(panes.nth(0)).toHaveAttribute("data-pane-alias", "nas");
  await expect(panes.nth(1)).toHaveAttribute("data-pane-alias", "bastion");
  const separator = page.getByRole("separator", { name: "Resize split" });
  await separator.press("ArrowRight");
  await expect(separator).toHaveAttribute("aria-valuenow", "65");
  await panes.nth(0).getByRole("button", { name: "Focus nas" }).click();
  await expect(panes).toHaveCount(1);
  await page.getByRole("button", { name: "Exit focus mode" }).first().click();
  await expect(panes).toHaveCount(2);

  page.once("dialog", (dialog) => dialog.accept("Production"));
  await page.getByRole("button", { name: "Save layout" }).click();
  await expect.poll(() => savedRequests.length).toBe(1);
  const savedRequest = savedRequests[0];
  if (savedRequest === undefined) throw new Error("workspace save request was not captured");
  const layout = (savedRequest.layout as typeof savedWorkspace.layout).split;
  expect(layout.direction).toBe("horizontal");
  expect(layout.ratio).toBe(65);
  expect(layout.first.pane.alias).toBe("nas");
  expect(layout.second.pane.alias).toBe("bastion");
});

test("opens a saved workspace from Home only after the explicit action", async ({ page, installation }) => {
  const opened: string[] = [];
  const sessions: Array<Record<string, unknown>> = [];
  await page.route("**/api/v1/workspaces**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const body = path.endsWith("/restore") ? { workspace: savedWorkspace } : { workspaces: [savedWorkspace] };
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ streamTicket: "fixture-stream-ticket" }) });
      return;
    }
    if (request.method() === "POST") {
      const alias = (request.postDataJSON() as { alias: string }).alias;
      opened.push(alias);
      const session = {
        id: `${alias}-home-session`, kind: "ssh", alias, title: alias,
        startedAt: "2026-08-24T09:00:00Z", state: "connected", problem: "", forwards: [],
      };
      sessions.push(session);
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ session, streamTicket: "fixture-stream-ticket" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions, maxSessions: 12 }) });
  });

  await openApplication(page, installation);
  const workspaces = page.getByRole("list", { name: "Saved terminal workspaces" });
  await expect(workspaces.getByText("Production", { exact: true })).toBeVisible();
  await expect(workspaces.getByText(/2 panes/)).toBeVisible();
  expect(opened).toEqual([]);

  await workspaces.getByRole("button", { name: "Open workspace" }).click();
  await expect(page).toHaveURL(/\/terminal$/);
  await expect.poll(() => opened.sort()).toEqual(["bastion", "nas"]);
  await expect(page.locator("[data-workspace-pane]")).toHaveCount(2);

  await openSection(page, "Home");
  await page.getByRole("list", { name: "Saved terminal workspaces" })
    .getByRole("button", { name: "Open workspace" }).click();
  await expect(page).toHaveURL(/\/terminal$/);
  await expect.poll(() => opened.length).toBe(4);
  await expect(page.locator("[data-workspace-pane]")).toHaveCount(2);
});
