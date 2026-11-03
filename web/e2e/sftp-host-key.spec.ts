import { expect, openApplication, openSection, test } from "./support/environment";
import { connectSFTPHost } from "./support/sftp";

// The e2e suite has no SSH server whose key is unknown, so the engine's side is played by the listing (which waits
// for the answer, as the engine's does), its connection attempt and the answer; the engine's side of the question is
// covered by the sshclient, connectionattempt and httpserver tests.
const fingerprint = "SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E";
const checkingKey = { phase: "host_key", alias: "bastion", hostName: "203.0.113.10", user: "ops", hop: 1, hops: 1 };
const asking = {
  progress: checkingKey,
  hostKeyQuestion: { alias: "bastion", hostName: "203.0.113.10", keyType: "ssh-ed25519", fingerprint },
  log: ["[sshc] [203.0.113.10]:2222へ接続します（ユーザー：ops）。"],
  finished: false,
};
const report = {
  name: "report.txt", path: "/srv/report.txt", type: "file", size: 4096, mode: "0644",
  modifiedAt: "2026-10-10T00:00:00Z", revision: "report-v1",
};

async function playEngine(page: import("@playwright/test").Page) {
  const answers: unknown[] = [];
  let attempt: Record<string, unknown> = asking;
  let answer: ((accept: boolean) => void) | undefined;
  const answered = new Promise<boolean>((resolve) => { answer = resolve; });
  await page.route("**/api/v1/sftp/bastion/connection-attempts/**", async (route) => {
    if (route.request().method() === "POST") {
      const posted = route.request().postDataJSON() as { accept: boolean };
      answers.push(posted);
      attempt = posted.accept
        ? { ...asking, hostKeyQuestion: undefined, progress: { ...checkingKey, phase: "authenticating" } }
        : { ...asking, hostKeyQuestion: undefined, log: [...asking.log, "sshc: ssh: handshake failed: this host is not in known_hosts; its host key was not trusted"], finished: true };
      answer?.(posted.accept);
    }
    await route.fulfill({ json: attempt });
  });
  // The first listing waits until the question is answered, as the engine's listing waits on its connection.
  await page.route("**/api/v1/sftp/bastion/entries**", async (route) => {
    const accepted = await answered;
    if (accepted) {
      await route.fulfill({ json: { path: "/srv", entries: [report] } });
      return;
    }
    await route.fulfill({
      status: 502, contentType: "application/problem+json",
      body: JSON.stringify({ code: "host_key_declined", message: "request rejected" }),
    });
  });
  return answers;
}

test("asks about the host key of a first connection in the tab and lists once it is accepted", async ({ page, installation }) => {
  const answers = await playEngine(page);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await connectSFTPHost(page, "bastion");

  const pane = page.getByRole("tabpanel");
  const card = pane.getByRole("region", { name: "Check the host key of bastion" });
  await expect(card).toContainText("First connection to this host");
  await expect(card).toContainText(`ED25519 ${fingerprint}`);
  // The card, not Connect, takes the focus: an Enter meant for something else does not trust the key.
  await expect(card).toBeFocused();
  await page.keyboard.press("Enter");
  expect(answers).toEqual([]);
  await card.getByRole("button", { name: "Connect", exact: true }).click();

  await expect.poll(() => answers).toEqual([{ fingerprint, accept: true }]);
  await expect(pane.getByRole("button", { name: "report.txt", exact: true })).toBeVisible();
  await expect(card).toBeHidden();
});

test("says the host key was not trusted when the question is declined, and keeps the reason in the log", async ({ page, installation }) => {
  const answers = await playEngine(page);
  await openApplication(page, installation);
  await openSection(page, "SFTP");
  await connectSFTPHost(page, "bastion");

  const pane = page.getByRole("tabpanel");
  await pane.getByRole("region", { name: "Check the host key of bastion" }).getByRole("button", { name: "Cancel", exact: true }).click();

  await expect.poll(() => answers).toEqual([{ fingerprint, accept: false }]);
  const failure = pane.getByRole("alert");
  await expect(failure).toContainText("The host key was not trusted, so sshc did not connect.");
  await expect(failure.getByRole("button", { name: "Try again", exact: true })).toBeVisible();
  await expect(pane.getByLabel("Connection log")).toContainText("its host key was not trusted");
});
