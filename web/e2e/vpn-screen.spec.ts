import type { Page, Route } from "@playwright/test";
import { expect, openApplication, test } from "./support/environment";

// VPN画面を開いたときと、「切断」。
//
// 画面を開いたときは、経路の状態を確かめる（docker を読む）のを待たずにプロファイルの
// 一覧を見せる。経路の状態は画面の外でも変わるので、開いているあいだは読み直す。
// 経路を使っている接続があれば、切断の前に本数を示して確かめる。
//
// 本物の経路には Docker と VPN サーバーが要るので、VPN の API は固定の値で答える。

type RouteState = { running: boolean; openConnections: number; checking?: boolean };

// overviewPath は、一覧の API（GET /api/v1/vpn）である。画面は開いたときに
// waitForRoutes=false を付けて読むので、クエリの有無を問わない。
const overviewPath = /\/api\/v1\/vpn(\?.*)?$/;

const labProfile = {
  name: "lab",
  backend: "wireguard",
  wireguard: { servers: ["vpn.example.jp"] },
};

function overviewOf(state: RouteState) {
  return {
    available: true,
    checking: state.checking ?? false,
    profiles: [
      {
        profile: labProfile,
        running: state.running,
        relaySocket: state.running ? "/home/fixture/.ssh/sshc/vpn/lab/engine.sock" : "",
        connections: ["lab-db", "lab-web"],
        openConnections: state.openConnections,
      },
    ],
  };
}

// answerVPN は、VPN の API を state のとおりに答える。切断の要求は state を止め、
// 受け取った回数を数える。
async function answerVPN(page: Page, state: RouteState): Promise<{ disconnects: number }> {
  const received = { disconnects: 0 };
  const reply = (route: Route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(overviewOf(state)) });
  await page.route(overviewPath, reply);
  await page.route("**/api/v1/vpn/profiles/lab/route", (route) => {
    if (route.request().method() === "DELETE") {
      received.disconnects += 1;
      state.running = false;
      state.openConnections = 0;
    }
    return reply(route);
  });
  return received;
}

async function openVPN(page: Page) {
  await page.evaluate(() => {
    window.history.pushState(null, "", "/vpn");
    window.dispatchEvent(new PopStateEvent("popstate"));
  });
  await expect(page.getByRole("heading", { name: "VPN", level: 2 })).toBeVisible();
}

test("the VPN screen shows a route that was started elsewhere, without being reopened", async ({ page, installation }) => {
  const state: RouteState = { running: false, openConnections: 0 };
  await answerVPN(page, state);
  await openApplication(page, installation);
  await openVPN(page);
  const route = page.getByRole("article", { name: "lab" });
  await expect(route).toContainText("stopped");
  await expect(route.getByRole("button", { name: "Disconnect" })).toBeDisabled();

  // ほかの画面の SSH の接続が、経路を起動する。
  state.running = true;
  state.openConnections = 1;

  await expect(route).toContainText("route open", { timeout: 15_000 });
  await expect(route.getByRole("button", { name: "Disconnect" })).toBeEnabled();
});

test("disconnecting a route that connections use asks first and says how many are cut", async ({ page, installation }) => {
  const state: RouteState = { running: true, openConnections: 2 };
  const received = await answerVPN(page, state);
  await openApplication(page, installation);
  await openVPN(page);
  const route = page.getByRole("article", { name: "lab" });
  await expect(route).toContainText("route open");

  await route.getByRole("button", { name: "Disconnect" }).click();
  const dialog = page.getByRole("dialog", { name: "Disconnect lab?" });
  await expect(dialog).toContainText("The connections using this VPN route (2) are disconnected too");
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toBeHidden();
  expect(received.disconnects).toBe(0);

  await route.getByRole("button", { name: "Disconnect" }).click();
  await page.getByRole("dialog", { name: "Disconnect lab?" }).getByRole("button", { name: "Disconnect" }).click();

  await expect(route).toContainText("stopped");
  expect(received.disconnects).toBe(1);
});

test("disconnecting a route that no connection uses does not ask", async ({ page, installation }) => {
  const state: RouteState = { running: true, openConnections: 0 };
  const received = await answerVPN(page, state);
  await openApplication(page, installation);
  await openVPN(page);
  const route = page.getByRole("article", { name: "lab" });

  await route.getByRole("button", { name: "Disconnect" }).click();

  await expect(route).toContainText("stopped");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(received.disconnects).toBe(1);
});

test("the VPN screen lists the profiles before the route states are checked", async ({ page, installation }) => {
  let answerRoutes: () => void = () => undefined;
  const routesChecked = new Promise<void>((resolve) => {
    answerRoutes = resolve;
  });
  await page.route(overviewPath, async (route) => {
    const waitForRoutes = new URL(route.request().url()).searchParams.get("waitForRoutes") !== "false";
    // 経路の状態を確かめる読み取りは、検査が許すまで答えない（mac の遅い docker の代わり）。
    if (waitForRoutes) await routesChecked;
    const body = overviewOf({ running: true, openConnections: 0, checking: !waitForRoutes });
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
  });
  await openApplication(page, installation);
  await openVPN(page);

  const route = page.getByRole("article", { name: "lab" });
  await expect(route).toContainText("checking");
  await expect(route.getByRole("button", { name: "Disconnect" })).toBeDisabled();

  answerRoutes();

  await expect(route).toContainText("route open");
  await expect(route.getByRole("button", { name: "Disconnect" })).toBeEnabled();
});
