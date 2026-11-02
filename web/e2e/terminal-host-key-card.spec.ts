import { expect, openApplication, openSection, test } from "./support/environment";

// The e2e suite has no SSH server whose key is unknown, so the engine's side is played by the session list and the
// answer it would give; the engine's side of the question is covered by the sshclient and httpserver tests.
const asking = {
  id: "host-key-fixture",
  kind: "ssh", workspaceId: "default",
  alias: "web-01",
  title: "web-01",
  startedAt: "2026-11-02T00:00:00Z",
  state: "connecting",
  problem: "",
  progress: { phase: "host_key", alias: "web-01", hostName: "10.0.2.15", user: "deploy", hop: 1, hops: 1 },
  hostKeyQuestion: { alias: "web-01", hostName: "10.0.2.15", keyType: "ssh-ed25519", fingerprint: "SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E" },
  forwards: [],
};
const { hostKeyQuestion: _question, ...answered } = asking;
const prompt = "The authenticity of host 'web-01 (10.0.2.15)' cannot be established.\r\n"
  + "SSH-ED25519 key fingerprint is SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E.\r\n"
  + "Are you sure you want to continue connecting (yes/no)? ";

async function playEngine(page: import("@playwright/test").Page) {
  const answers: unknown[] = [];
  let session: Record<string, unknown> = asking;
  await page.route("**/api/v1/terminal/sessions**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path.endsWith("/host-key")) {
      answers.push(route.request().postDataJSON());
      session = { ...answered, progress: { ...asking.progress, phase: "authenticating" } };
    }
    if (path.endsWith("/stream")) {
      await route.fulfill({ status: 201, contentType: "application/json", body: JSON.stringify({ streamTicket: "host-key" }) });
      return;
    }
    await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ sessions: [session], maxSessions: 12 }) });
  });
  await page.routeWebSocket("**/terminal/stream?**", (socket) => { socket.send(Buffer.from(prompt)); });
  return answers;
}

test("asks about an unknown host key in a card and answers it with a button", async ({ page, installation }) => {
  const answers = await playEngine(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const screen = page.getByRole("region", { name: /^Terminal for / });
  const card = screen.getByRole("region", { name: "Check the host key of web-01" });
  await expect(card).toContainText("ED25519 SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E");
  // The card, not Connect, takes the focus: an Enter meant for the terminal does not trust the key.
  await expect(card).toBeFocused();
  await page.keyboard.press("Enter");
  expect(answers).toEqual([]);
  await card.getByRole("button", { name: "Connect", exact: true }).click();

  await expect.poll(() => answers).toEqual([{ fingerprint: "SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E", accept: true }]);
  await expect(card).toBeHidden();
  await expect(screen.getByText("Check the host key of web-01")).toHaveCount(0);
});

test("answers the host key question from the strip above once the card is hidden", async ({ page, installation }) => {
  const answers = await playEngine(page);
  await openApplication(page, installation);
  await openSection(page, "Terminal");

  const screen = page.getByRole("region", { name: /^Terminal for / });
  const card = screen.getByRole("region", { name: "Check the host key of web-01" });
  await card.getByRole("button", { name: "Hide this notice" }).click();
  await expect(card).toBeHidden();
  await screen.getByRole("button", { name: "Cancel", exact: true }).click();

  await expect.poll(() => answers).toEqual([{ fingerprint: "SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E", accept: false }]);
});
