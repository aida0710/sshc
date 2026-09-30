import { describe, expect, it } from "vitest";
import { localStorageKeys, sessionStorageKeys } from "./browserStorageKeys";

const registeredKeys: string[] = [...Object.values(localStorageKeys), ...Object.values(sessionStorageKeys)];
// These keys were named before the rule in browserStorageKeys.ts and keep
// their names so that the stored values survive.
const namedBeforeTheRule = [
  "sshc.theme",
  "sshc.language",
  "sshc.navigation.width",
  "sshc.home.quick-connect-view",
  "sshc.sftp.splitRatio",
  "sshc.sftp.queueView",
  "sshc.shortcuts.v1",
  "sshc.terminal-notification-sound.v1",
  "sshc.session.csrf",
];
const kebabSegment = "[a-z0-9]+(?:-[a-z0-9]+)*";
const namingRule = new RegExp(`^sshc\\.${kebabSegment}\\.${kebabSegment}\\.v[1-9][0-9]*$`);

describe("browser storage keys", () => {
  it("gives every stored value its own key", () => {
    expect(new Set(registeredKeys).size).toBe(registeredKeys.length);
  });

  it("names every newer key sshc.<area>.<name>.v<version> in kebab-case", () => {
    const offRule = registeredKeys.filter((key) => !namingRule.test(key) && !namedBeforeTheRule.includes(key));
    expect(offRule).toEqual([]);
  });

  it("excuses only keys that are still registered", () => {
    expect(namedBeforeTheRule.filter((key) => !registeredKeys.includes(key))).toEqual([]);
  });
});
