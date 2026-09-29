import type { Page, Route } from "@playwright/test";
import { expect, openApplication, test } from "./support/environment";

// VPN画面の「切断」。経路の状態は画面の外でも変わるので、画面は開いているあいだ
// 読み直す。経路を使っている接続があれば、切断の前に本数を示して確かめる。
//
// 本物の経路には Docker と VPN サーバーが要るので、VPN の API は固定の値で答える。

type RouteState = { running: boolean; openConnections: number };

const labProfile = {
  name: "lab",
  backend: "wireguard",
  wireguard: {
    server: "vpn.example.jp:51820",
    peerPublicKey: "bBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBbBA=",
    address: "10.9.9.2/32",
  },
};

function overviewOf(state: RouteState) {
  return {
    available: true,
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
  await page.route("**/api/v1/vpn", reply);
  await page.route("**/api/v1/vpn/profiles/lab/session", (route) => {
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
