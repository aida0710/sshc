import { describe, expect, it } from "vitest";
import { historyOperationLabelKey } from "./historyLabels";

describe("historyOperationLabelKey", () => {
  it.each([
    ["host.detect-os", "history.operation.hostDetection"],
    ["vpn.profile.create", "history.operation.vpn"],
    ["vpn.connection.bind", "history.operation.vpn"],
    ["known_hosts.add", "history.operation.knownHosts"],
    ["sftp.transferSettings", "history.operation.sftp"],
    ["terminal.background.rename", "history.operation.terminal"],
    ["shortcuts.presets", "history.operation.settings"],
    ["keybar.presets", "history.operation.settings"],
    ["settings.preset", "history.operation.settings"],
  ])("names %s by what it changed", (operation, key) => {
    expect(historyOperationLabelKey(operation)).toBe(key);
  });

  it.each([
    [["sshc/secrets/vault.json"], "history.operation.vault"],
    [["sshc/machines/0123.json"], "history.operation.machine"],
    [["sshc/metadata.json"], "history.operation.settings"],
    [["known_hosts"], "history.operation.knownHosts"],
    [["config"], "history.operation.configuration"],
    [["connections/lab/hosts.conf"], "history.operation.configuration"],
  ])("names a restore of %s by what the file holds", (paths, key) => {
    expect(historyOperationLabelKey("config.restore", paths)).toBe(key);
  });

  it("calls an operation it does not know a change, without showing its internal name", () => {
    expect(historyOperationLabelKey("something.new")).toBe("history.operation.other");
  });
});
