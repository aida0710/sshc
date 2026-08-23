import { describe, expect, it, vi } from "vitest";
import type { KeyItem, RelocateKeyResponse } from "./api";
import { folderRows, itemsInFolder, moveInto, sameFolder, shownItems, type Folder } from "./organizer";

function item(relativePath: string, id = relativePath): KeyItem {
  return {
    id,
    relativePath,
    kind: "private_key",
    container: "OPENSSH PRIVATE KEY",
    algorithm: "ed25519",
    keyType: "ssh-ed25519",
    bits: 256,
    encrypted: true,
    fingerprint: "SHA256:" + id,
    comment: "",
    permission: "0600",
    permissionRisk: false,
    sizeBytes: 444,
    references: [],
    notes: [],
  } as unknown as KeyItem;
}

function relocated(overrides: Partial<RelocateKeyResponse> = {}): RelocateKeyResponse {
  return {
    id: "",
    relativePath: "",
    group: "",
    files: [],
    references: [],
    skipped: [],
    notes: [],
    blockers: [],
    transactionId: "t",
    ...overrides,
  };
}

describe("folderRows", () => {
  it("counts what is directly inside each folder", () => {
    const items = [
      item("keys/work/id_a"),
      item("keys/work/id_b"),
      item("keys/work/ci/id_c"),
      item("id_loose"),
    ];

    const rows = folderRows(items, ["work", "work/ci", "empty"]);

    expect(rows.map((row) => [row.folder.kind === "group" ? row.folder.name : row.folder.kind, row.count])).toEqual([
      ["all", 4],
      ["empty", 0],
      ["work", 2],
      ["work/ci", 1],
      ["ungrouped", 1],
    ]);
  });

  it("puts a parent before its children and reports the depth", () => {
    const rows = folderRows([], ["b", "a/deep", "a"]);
    const groups = rows.filter((row) => row.folder.kind === "group");

    expect(groups.map((row) => (row.folder.kind === "group" ? row.folder.name : ""))).toEqual(["a", "a/deep", "b"]);
    expect(groups.map((row) => row.depth)).toEqual([1, 2, 1]);
  });

  it("does not invent a folder from a path nothing declares", () => {
    const rows = folderRows([item("keys/ghost/id_a")], []);

    expect(rows.some((row) => row.folder.kind === "group")).toBe(false);
    expect(rows.find((row) => row.folder.kind === "ungrouped")?.count).toBe(0);
  });
});

describe("itemsInFolder", () => {
  const items = [item("keys/work/id_a"), item("keys/work/ci/id_b"), item("id_loose")];

  it("shows every key under all", () => {
    expect(itemsInFolder(items, { kind: "all" })).toHaveLength(3);
  });

  it("shows only the keys directly inside a group", () => {
    const inside = itemsInFolder(items, { kind: "group", name: "work" });
    expect(inside.map((found) => found.relativePath)).toEqual(["keys/work/id_a"]);
  });

  it("shows the keys that belong to no group", () => {
    const inside = itemsInFolder(items, { kind: "ungrouped" });
    expect(inside.map((found) => found.relativePath)).toEqual(["id_loose"]);
  });
});

describe("sameFolder", () => {
  it("tells the folders apart", () => {
    const cases: [Folder, Folder, boolean][] = [
      [{ kind: "all" }, { kind: "all" }, true],
      [{ kind: "ungrouped" }, { kind: "all" }, false],
      [{ kind: "group", name: "a" }, { kind: "group", name: "a" }, true],
      [{ kind: "group", name: "a" }, { kind: "group", name: "b" }, false],
    ];
    for (const [left, right, want] of cases) {
      expect(sameFolder(left, right)).toBe(want);
    }
  });
});

describe("moveInto", () => {
  it("moves what it can and names what it could not", async () => {
    const relocate = vi.fn(async (id: string) =>
      id === "keys/work/id_b"
        ? relocated({ blockers: ["Include glob would read the destination as configuration"] })
        : relocated(),
    );

    const outcome = await moveInto(relocate, [item("keys/work/id_a"), item("keys/work/id_b"), item("keys/work/id_c")], {
      kind: "group",
      name: "personal",
    });

    expect(relocate).toHaveBeenCalledTimes(3);
    expect(outcome.moved).toEqual(["keys/work/id_a", "keys/work/id_c"]);
    expect(outcome.blocked).toEqual([
      { path: "keys/work/id_b", blockers: ["Include glob would read the destination as configuration"] },
    ]);
    expect(outcome.failed).toEqual([]);
  });

  it("keeps going when one call fails outright", async () => {
    const relocate = vi.fn(async (id: string) => {
      if (id === "keys/work/id_a") throw new Error("network");
      return relocated();
    });

    const outcome = await moveInto(relocate, [item("keys/work/id_a"), item("keys/work/id_b")], {
      kind: "group",
      name: "personal",
    });

    expect(outcome.failed).toEqual(["keys/work/id_a"]);
    expect(outcome.moved).toEqual(["keys/work/id_b"]);
  });

  it("leaves a key that is already there alone", async () => {
    const relocate = vi.fn(async () => relocated());

    const outcome = await moveInto(relocate, [item("keys/work/id_a"), item("id_loose")], {
      kind: "group",
      name: "work",
    });

    expect(relocate).toHaveBeenCalledTimes(1);
    expect(relocate).toHaveBeenCalledWith("id_loose", { group: "work" });
    expect(outcome.unchanged).toEqual(["keys/work/id_a"]);
  });

  it("moves to the root of ~/.ssh with an empty group", async () => {
    const relocate = vi.fn(async () => relocated());

    await moveInto(relocate, [item("keys/work/id_a")], { kind: "ungrouped" });

    expect(relocate).toHaveBeenCalledWith("keys/work/id_a", { group: "" });
  });
});

describe("shownItems", () => {
  function kinded(relativePath: string, kind: string, permissionRisk = false): KeyItem {
    return { ...item(relativePath), kind, permissionRisk } as unknown as KeyItem;
  }

  const everything = [
    kinded("id_a", "private_key"),
    kinded("id_a.pub", "public_key"),
    kinded("id_a-cert.pub", "certificate"),
    kinded("config", "config"),
    kinded("known_hosts", "known_hosts"),
    kinded(".DS_Store", "other"),
    kinded("connections/work/box.conf", "config"),
  ];

  it("shows key material and nothing else", () => {
    const shown = shownItems(everything, "keys");

    expect(shown.map((found) => found.relativePath)).toEqual(["id_a", "id_a.pub", "id_a-cert.pub"]);
  });

  it("keeps a file that needs attention even when it is not a key", () => {
    const risky = [...everything, kinded("secret.txt", "other", true)];

    const shown = shownItems(risky, "keys");

    expect(shown.map((found) => found.relativePath)).toContain("secret.txt");
  });

  it("shows everything when asked to", () => {
    expect(shownItems(everything, "all")).toHaveLength(everything.length);
  });
});
