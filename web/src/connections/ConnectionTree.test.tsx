import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionTree } from "./ConnectionTree";
import { dragMimeType, type DragPayload } from "./dragdrop";

function host(path: string, alias: string, group?: string): HostEntry {
  return {
    identity: { path, alias },
    file: { path, absolute: `/home/tester/.ssh/${path}` },
    line: alias === "" ? 20 : 1,
    patterns: alias === "" ? ["*"] : [alias],
    ...(alias === "" ? { wildcard: true } : {}),
    editable: true,
    ...(group === undefined ? {} : { group }),
  };
}

const homeNas = host("connections/home/nas.conf", "nas", "home");
const euApi = host("connections/home/eu/api.conf", "eu-api", "home/eu");
const bastion = host("config", "bastion");
const catchAll = host("config", "");

const overview: Overview = {
  entry: { path: "config", absolute: "/home/tester/.ssh/config" },
  files: [
    { file: { path: "config", absolute: "/home/tester/.ssh/config" }, editable: true, loads: 1 },
    { file: homeNas.file, editable: true, loads: 1 },
    { file: euApi.file, editable: true, loads: 1 },
  ],
  hosts: [homeNas, euApi, bastion, catchAll],
  groups: [
    { name: "home", directory: "connections/home", keyDirectory: "keys/home", memberCount: 1, directoryPresent: true },
    { name: "home/eu", parent: "home", directory: "connections/home/eu", keyDirectory: "keys/home/eu", memberCount: 1, directoryPresent: true },
    { name: "empty", directory: "connections/empty", keyDirectory: "keys/empty", memberCount: 0, directoryPresent: true },
  ],
  metadata: {
    schemaVersion: 2,
    groups: [{ name: "home", order: 1 }, { name: "empty", order: 2 }, { name: "home/eu", order: 1 }],
    hosts: [{ identity: homeNas.identity, favourite: true, colour: "#f97316", tags: ["storage"] }],
  },
  diagnostics: [],
  notices: [],
};

function transfer(payload: DragPayload) {
  const store = new Map<string, string>([[dragMimeType, JSON.stringify(payload)]]);
  return {
    types: [...store.keys()],
    setData: (type: string, value: string) => void store.set(type, value),
    getData: (type: string) => store.get(type) ?? "",
    effectAllowed: "move",
    dropEffect: "move",
  };
}

describe("ConnectionTree", () => {
  it("shows the complete declared hierarchy and ungrouped connections at once", () => {
    render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onOpenPatternRule={vi.fn()} onDrop={vi.fn()} />,
    );

    const tree = screen.getByRole("navigation", { name: "Connections" });
    expect(within(tree).getByRole("group", { name: "Arrange connections by" })).toBeInTheDocument();
    expect(within(tree).queryByRole("group", { name: "Browse connections by" })).not.toBeInTheDocument();
    expect(within(tree).getByRole("heading", { name: "home" })).toBeInTheDocument();
    expect(within(tree).getByRole("heading", { name: "eu" })).toBeInTheDocument();
    expect(within(tree).getByRole("heading", { name: "empty" })).toBeInTheDocument();
    expect(within(tree).getByRole("heading", { name: "Ungrouped" })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: /nas/ })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: /eu-api/ })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: /bastion/ })).toBeInTheDocument();
  });

  it("opens pattern rules from the file arrangement instead of treating them as servers", async () => {
    const onOpenPatternRule = vi.fn();
    render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onOpenPatternRule={onOpenPatternRule} onDrop={vi.fn()} />,
    );

    await userEvent.click(screen.getByRole("button", { name: "Files" }));
    await userEvent.click(screen.getByRole("button", { name: /Host \*/ }));

    expect(onOpenPatternRule).toHaveBeenCalledWith("config", 20);
  });

  it("keeps filtering, visible metadata, and favourite-only browsing", async () => {
    render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onOpenPatternRule={vi.fn()} onDrop={vi.fn()} />,
    );

    const nas = screen.getByRole("button", { name: /nas/ });
    expect(within(nas).getByText("★")).toBeInTheDocument();
    expect(within(nas).getByText("storage")).toBeInTheDocument();

    await userEvent.type(screen.getByRole("searchbox", { name: "Filter connections" }), "bast");
    expect(screen.getByRole("button", { name: /bastion/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /nas/ })).not.toBeInTheDocument();

    await userEvent.clear(screen.getByRole("searchbox", { name: "Filter connections" }));
    await userEvent.click(screen.getByRole("button", { name: "Favourites" }));
    expect(screen.getByRole("button", { name: /nas/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /bastion/ })).not.toBeInTheDocument();
  });

  it("allows management drops only when saved state is stable", () => {
    const onDrop = vi.fn();
    const payload: DragPayload = {
      kind: "connection",
      path: homeNas.identity.path,
      alias: homeNas.identity.alias,
      group: "home",
    };
    const harness = render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onOpenPatternRule={vi.fn()} onDrop={onDrop} />,
    );
    const source = screen.getByRole("button", { name: /nas/ });
    const target = screen.getByRole("heading", { name: "empty" });
    const dataTransfer = transfer(payload);
    fireEvent.dragStart(source, { dataTransfer });
    fireEvent.dragOver(target, { dataTransfer });
    fireEvent.drop(target, { dataTransfer });
    expect(onDrop).toHaveBeenCalledWith(payload, "empty");

    onDrop.mockClear();
    harness.rerender(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onOpenPatternRule={vi.fn()} onDrop={onDrop} movesDisabled />,
    );
    const disabledSource = screen.getByRole("button", { name: /nas/ });
    expect(disabledSource).not.toHaveAttribute("draggable", "true");
    fireEvent.dragStart(disabledSource, { dataTransfer });
    fireEvent.drop(screen.getByRole("heading", { name: "empty" }), { dataTransfer });
    expect(onDrop).not.toHaveBeenCalled();
  });
});
