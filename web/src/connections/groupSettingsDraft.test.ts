import { describe, expect, it } from "vitest";
import type { GroupMetadata } from "../api/config";
import { draftOf, groupFrom, repeatedFieldKeywords } from "./groupSettingsDraft";

const twoKeys: GroupMetadata = {
  name: "work",
  settings: [
    { keyword: "IdentityFile", values: ["~/.ssh/keys/a"] },
    { keyword: "User", values: ["ops"] },
    { keyword: "IdentityFile", values: ["~/.ssh/keys/b"] },
  ],
};

describe("groupSettingsDraft", () => {
  it("keeps every IdentityFile line when only the user name changes", () => {
    const draft = draftOf(twoKeys);
    expect(repeatedFieldKeywords(twoKeys)).toEqual(new Set(["IdentityFile"]));
    expect(draft.defaults).toEqual({ User: "ops" });
    const saved = groupFrom(twoKeys, { ...draft, defaults: { ...draft.defaults, User: "deploy" } });
    expect(saved.settings).toEqual([
      { keyword: "IdentityFile", values: ["~/.ssh/keys/a"] },
      { keyword: "User", values: ["deploy"] },
      { keyword: "IdentityFile", values: ["~/.ssh/keys/b"] },
    ]);
  });

  it("gives back the same settings, in the same order, when nothing was edited", () => {
    const group: GroupMetadata = {
      name: "work",
      settings: [
        { keyword: "Port", values: ["2222"] },
        { keyword: "ServerAliveInterval", values: ["30"] },
        { keyword: "user", values: ["ops"] },
      ],
    };
    const saved = groupFrom(group, draftOf(group));
    expect(saved.settings).toEqual(group.settings);
    saved.settings?.forEach((setting, index) => expect(setting).toBe(group.settings?.[index]));
  });

  it("adds a newly filled field after the settings already there and drops a cleared one", () => {
    const group: GroupMetadata = { name: "work", settings: [{ keyword: "Port", values: ["2222"] }, { keyword: "Compression", values: ["yes"] }] };
    const draft = draftOf(group);
    const saved = groupFrom(group, { ...draft, defaults: { Port: "", ProxyJump: "bastion" } });
    expect(saved.settings).toEqual([
      { keyword: "Compression", values: ["yes"] },
      { keyword: "ProxyJump", values: ["bastion"] },
    ]);
  });

  it("removes every setting and the colour when the draft is emptied", () => {
    const group: GroupMetadata = { name: "work", colour: "#ff0000", settings: [{ keyword: "User", values: ["ops"] }] };
    const saved = groupFrom(group, { colour: "", defaults: {}, other: [] });
    expect(saved).toEqual({ name: "work" });
  });
});
