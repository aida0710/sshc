import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { KeyInspector } from "./KeyInspector";
import type { KeyItem } from "./api";

const item: KeyItem = {
  id: "1",
  relativePath: "id_work",
  kind: "private_key",
  container: "",
  algorithm: "ed25519",
  keyType: "ssh-ed25519",
  bits: 256,
  encrypted: true,
  fingerprint: "SHA256:abcdef",
  comment: "aida@laptop",
  permission: "0600",
  permissionRisk: false,
  sizeBytes: 400,
  references: [
    {
      directive: "IdentityFile",
      configPath: "config",
      line: 3,
      condition: "",
      hostPatterns: ["build-*"],
      value: "~/.ssh/id_work",
    },
  ],
  notes: [],
};

describe("KeyInspector", () => {
  // 表から降ろしたものは、ここに揃っていなければならない。
  it("shows what the table no longer carries", () => {
    render(<KeyInspector item={item} now={0} />);

    expect(screen.getByText("SHA256:abcdef")).toBeInTheDocument();
    expect(screen.getByText("ed25519 · 256")).toBeInTheDocument();
    expect(screen.getByText("0600")).toBeInTheDocument();
    expect(screen.getByText("build-*")).toBeInTheDocument();
  });

  // 使っている接続が無いことは、空欄ではなく文で言う。
  it("says so when nothing names the key", () => {
    render(<KeyInspector item={{ ...item, references: [] }} now={0} />);

    expect(screen.getByText("No connection names this key")).toBeInTheDocument();
  });

  it("marks permissions that are too open", () => {
    render(<KeyInspector item={{ ...item, permission: "0644", permissionRisk: true }} now={0} />);

    expect(screen.getByText("Permissions too open")).toBeInTheDocument();
  });
});
