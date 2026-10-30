import { expect, openApplication, openLocalShell, test } from "./support/environment";
import { standardPresetMetadata } from "./support/settings";
import { drawnRows, mountedTerminals, terminalKeyboard } from "./support/terminal";

// A full-screen program draws its screen once and then rewrites only what changes, as Claude Code does. This one
// draws its messages on the alternate screen and then rewrites one row far past the engine's replay buffer, here
// at its smallest (16 KiB). With one terminal kept hidden, two more sessions rebuild its terminal on the way back:
// the engine's screen (internal/terminal/screen_checkpoint.go) must bring the messages back.
const program = [
  "printf '\\033[?1049h\\033[2J'",
  "for n in 1 2 3 4 5 6; do printf '\\033[%d;1H* Message %d drawn once' $n $n; done",
  "for i in $(seq 1 2000); do printf '\\033[?2026h\\033[12;1H\\033[2K| Working %d\\033[?2026l' $i; done",
  "sleep 600",
].join("\n");

test("a terminal rebuilt for a full-screen program shows what the program drew before the replay buffer", async ({ page, installation }) => {
  await installation.write("sshc/metadata.json", standardPresetMetadata({ terminal: { scrollbackBytes: 16 << 10, retainedTerminals: 1 } }));
  await installation.write("full-screen.sh", program);
  await openApplication(page, installation);
  await openLocalShell(page);
  await terminalKeyboard(page).focus();
  await page.keyboard.type(`bash "${installation.home}/.ssh/full-screen.sh"\n`);
  await expect(drawnRows(page)).toContainText("Working 2000", { timeout: 20_000 });
  await expect(drawnRows(page)).toContainText("Message 1 drawn once");

  const navigation = page.getByRole("navigation", { name: "Primary" });
  const first = navigation.getByRole("button", { name: /^bash/ }).first();
  for (let opened = 0; opened < 2; opened++) await openLocalShell(page);
  // The one shown and the one kept hidden: the first session's terminal is gone.
  await expect(mountedTerminals(page)).toHaveCount(2);

  await first.click();
  const rows = drawnRows(page);
  await expect(rows).toContainText("Working 2000");
  for (let n = 1; n <= 6; n++) await expect(rows).toContainText(`Message ${n} drawn once`);
  // The replay starts outside any escape sequence: none of one shows as text.
  await expect(rows).not.toContainText("026");
});
