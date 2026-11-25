import type { Page } from "@playwright/test";
import { expect, openApplication, openSection, test } from "./support/environment";
import { terminalScreen } from "./support/terminal";

// Claude Code が出すマウスのモード。押したままの移動（1002）とボタンなしの移動（1003）を、SGR の形（1006）で求める。
const mouseModes = "\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h";

// dragReports は、ページが送ったもののうち、左ボタンを押したままの移動の報告（SGR の 32）が指す桁と行を返す。
function dragReports(sent: string[]): { column: number; row: number }[] {
  return [...sent.join("").matchAll(/\x1b\[<32;(\d+);(\d+)M/g)].map((report) => ({ column: Number(report[1]), row: Number(report[2]) }));
}

// mockProgramThatResendsItsModes は、マウスの報告を求めるプログラムのセッションを出す。プログラムは、押したままの
// 移動を resendAfter 回受け取ったところで、モードを同じ順でもう一度送る。Claude Code は、入力が数秒とぎれた
// あとの最初の入力でこうする。なぞっている途中で手を止めて読み、続きをなぞると、ボタンを押したままの間に届く。
async function mockProgramThatResendsItsModes(page: Page, resendAfter: number): Promise<string[]> {
  const sent: string[] = [];
  const session = { id: "mouse-drag-shell", kind: "shell", workspaceId: "default", title: "claude", startedAt: "2026-11-21T00:00:00Z", state: "connected", problem: "", forwards: [] };
  await page.route("**/api/v1/terminal/sessions**", (route) => {
    if (new URL(route.request().url()).pathname.endsWith("/stream")) return route.fulfill({ status: 201, json: { streamTicket: session.id } });
    return route.fulfill({ json: { sessions: [session], maxSessions: 50 } });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => {
    let resent = false;
    socket.send(Buffer.from(`\x1b[?1049h${mouseModes}\x1b[2J\x1b[HProgram that selects with the mouse`));
    socket.onMessage((data) => {
      sent.push(typeof data === "string" ? data : data.toString("utf8"));
      if (!resent && dragReports(sent).length >= resendAfter) {
        resent = true;
        socket.send(Buffer.from(`${mouseModes}\x1b[2;1HModes sent again`));
      }
    });
  });
  return sent;
}

test("a drag goes on reaching the program after it sends its mouse modes again while the button is held", async ({ page, installation }) => {
  const sent = await mockProgramThatResendsItsModes(page, 3);
  await openApplication(page, installation);
  await openSection(page, "Terminal");
  const screen = terminalScreen(page);
  await expect(screen).toContainText("Program that selects with the mouse");
  const box = (await screen.boundingBox())!;

  await page.mouse.move(box.x + 20, box.y + box.height * 0.3);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width * 0.4, box.y + box.height * 0.5, { steps: 8 });
  await expect(screen).toContainText("Modes sent again");
  const beforeTheRest = dragReports(sent).length;
  await page.mouse.move(box.x + box.width * 0.8, box.y + box.height * 0.8, { steps: 8 });
  await page.mouse.up();

  // 離した位置は、最後に届いた移動の位置と同じ。Claude Code は離した位置ではなく、最後の移動までをコピーする。
  await expect.poll(() => sent.join("")).toMatch(/\x1b\[<0;\d+;\d+m/);
  const released = /\x1b\[<0;(\d+);(\d+)m/.exec(sent.join(""))!;
  const reports = dragReports(sent);
  expect(reports.length).toBeGreaterThan(beforeTheRest);
  expect(reports.at(-1)).toEqual({ column: Number(released[1]), row: Number(released[2]) });
});
