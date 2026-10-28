import { describe, expect, it } from "vitest";
import type { HostEntry, Notice } from "../api/config";
import type { KeyItem } from "../keys/api";
import { connectionFilesOf, folderNoticesOf, keyFilesOf, ordersAfterMove } from "./groupFolders";

function host(alias: string, path: string, group?: string): HostEntry {
  return {
    identity: { alias, path, line: 1 },
    file: { path, absolute: `/home/a/.ssh/${path}` },
    line: 1,
    patterns: [alias],
    editable: true,
    ...(group === undefined ? {} : { group }),
  } as HostEntry;
}

function key(relativePath: string, kind: string, fingerprint: string): KeyItem {
  return { id: relativePath, relativePath, kind, fingerprint } as KeyItem;
}

describe("the folders of a group", () => {
  it("lists each file of the group's own connections once, with the connections in it", () => {
    const hosts = [
      host("gpu-01", "connections/lab/10-gpu.conf", "lab"),
      host("gpu-02", "connections/lab/10-gpu.conf", "lab"),
      host("db", "connections/lab/sub/10-db.conf", "lab/sub"),
      host("nas", "conf.d/10-home.conf"),
    ];
    expect(connectionFilesOf(hosts, "lab")).toEqual([{ name: "10-gpu.conf", aliases: ["gpu-01", "gpu-02"] }]);
  });

  it("puts a public key beside its private key and keeps only what follows the private key's name", () => {
    const items = [
      key("keys/lab/lab_ed25519", "private_key", "SHA256:a"),
      key("keys/lab/lab_ed25519.pub", "public_key", "SHA256:a"),
      key("keys/lab/sub/other", "private_key", "SHA256:b"),
      key("id_ed25519", "private_key", "SHA256:c"),
    ];
    expect(keyFilesOf(items, "lab")).toEqual([{ name: "lab_ed25519", companions: [".pub"] }]);
  });
});

describe("folder notices", () => {
  it("leaves out a file's notice when the folder it is in is already reported", () => {
    const notices: Notice[] = [
      { code: "group_not_declared", path: "connections/old", detail: "old" },
      { code: "group_file_unreached", path: "connections/old/10-old.conf" },
      { code: "group_file_unreached", path: "connections/10-loose.conf" },
      { code: "group_empty", path: "connections/home", detail: "home" },
      { code: "duplicate_alias", path: "config", detail: "nas" },
    ];
    expect(folderNoticesOf(notices).map((notice) => notice.path)).toEqual(["connections/old", "connections/10-loose.conf"]);
  });
});

describe("moving a group among its siblings", () => {
  it("numbers every sibling in its new place", () => {
    expect([...ordersAfterMove(["home", "lab", "work"], "lab", "up")!]).toEqual([["lab", 0], ["home", 1], ["work", 2]]);
  });

  it("does not move the first group up or the last group down", () => {
    expect(ordersAfterMove(["home", "lab"], "home", "up")).toBeNull();
    expect(ordersAfterMove(["home", "lab"], "lab", "down")).toBeNull();
  });
});
