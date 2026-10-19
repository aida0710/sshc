import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionTree, type HostSelection } from "./ConnectionTree";
import { dragMimeType, type DragPayload } from "./dragdrop";
import { emptyMultiSelection } from "./multiSelection";

function host(path: string, alias: string, group?: string): HostEntry {
  return {
    identity: { path, alias },
    file: { path, absolute: `/home/tester/.ssh/${path}` },
    line: alias === "" ? 20 : 1,
    patterns: alias === "" ? ["*"] : [alias],
    ...(alias === "" ? { wildcard: true } : {}),
    editable: true,
    ...(alias === "" ? {} : { hostName: `${alias}.example`, user: "tester", port: "22" }),
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
    hosts: [{ identity: homeNas.identity, colour: "#f97316", tags: ["storage"] }],
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
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onDrop={vi.fn()} />,
    );

    const tree = screen.getByRole("navigation", { name: "Connections" });
    expect(within(tree).queryByRole("group", { name: "Arrange connections by" })).not.toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: "home" })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: "home/eu" })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: "empty" })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: "Ungrouped" })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: "nas" })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: "eu-api" })).toBeInTheDocument();
    expect(within(tree).getByRole("button", { name: "bastion" })).toBeInTheDocument();
    expect(within(tree).getByRole("region", { name: "home group, 1 connections" })).toBeInTheDocument();
    expect(within(tree).getByRole("region", { name: "home/eu group, 1 connections" })).toBeInTheDocument();
    expect(within(tree).getByRole("region", { name: "Ungrouped group, 1 connections" })).toBeInTheDocument();
  });

  it("lets the groups column be resized from the keyboard and remembers the width", () => {
    window.localStorage.removeItem("sshc.connections.groups-width.v1");
    render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onDrop={vi.fn()} />,
    );

    const handle = screen.getByRole("separator", { name: "Resize the groups column" });
    expect(handle).toHaveAttribute("aria-valuenow", "184");
    fireEvent.keyDown(handle, { key: "ArrowRight", shiftKey: true });
    expect(handle).toHaveAttribute("aria-valuenow", "216");
    expect(window.localStorage.getItem("sshc.connections.groups-width.v1")).toBe("216");
    fireEvent.keyDown(handle, { key: "Home" });
    expect(handle).toHaveAttribute("aria-valuenow", "112");
  });

  it("keeps pattern rules out of Connections", () => {
    render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onDrop={vi.fn()} />,
    );

    expect(screen.queryByRole("button", { name: /Host \*/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Files" })).not.toBeInTheDocument();
  });

  it("sorts connections by name by default", async () => {
    const zeta = host("config", "zeta");
    const alpha = host("config", "alpha");
    const orderOverview = { ...overview, hosts: [zeta, alpha], groups: [], metadata: { schemaVersion: 2 } };
    const user = userEvent.setup();
    render(
      <ConnectionTree overview={orderOverview} selected={null} onSelect={vi.fn()} onDrop={vi.fn()} />,
    );

    const order = screen.getByRole("combobox", { name: "Connection order" });
    expect(order).toHaveValue("name");
    const section = screen.getByRole("region", { name: "Ungrouped group, 2 connections" });
    expect(within(section).getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["alpha", "zeta"]);

    await user.selectOptions(order, "configured");
    expect(within(section).getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["zeta", "alpha"]);
  });

  it("keeps filtering and visible metadata", async () => {
    render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onDrop={vi.fn()} />,
    );

    const nas = screen.getByRole("button", { name: "nas" });
    expect(within(nas).getByText("storage")).toBeInTheDocument();

    await userEvent.type(screen.getByRole("searchbox", { name: "Filter connections" }), "bast");
    expect(screen.getByRole("button", { name: "bastion" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "nas" })).not.toBeInTheDocument();
  });

  it("keeps repeated projections of one Host block as distinct accessible items", () => {
    const projected = host("connections/shared.conf", "shared", "home");
    const repeatedOverview: Overview = {
      ...overview,
      hosts: [projected, { ...projected }],
      groups: overview.groups.filter((group) => group.name === "home"),
      metadata: { schemaVersion: 2 },
    };
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => undefined);

    try {
      render(
        <ConnectionTree overview={repeatedOverview} selected={null} onSelect={vi.fn()} onDrop={vi.fn()} />,
      );

      const projectedButtons = screen.getAllByRole("button", { name: "shared" });
      const descriptionIds = projectedButtons.map((button) => button.getAttribute("aria-describedby"));
      expect(descriptionIds).not.toContain(null);
      expect(new Set(descriptionIds).size).toBe(projectedButtons.length);
      descriptionIds.forEach((descriptionId) => {
        expect(document.getElementById(descriptionId!)).toHaveTextContent(
          "tester@shared.example, connections/shared.conf",
        );
      });
      expect(consoleError.mock.calls.flat().join(" ")).not.toContain("same key");
    } finally {
      consoleError.mockRestore();
    }
  });

  it("filters grouped results by subtree, exact group, and resolved destination", async () => {
    const user = userEvent.setup();
    render(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onDrop={vi.fn()} />,
    );

    expect(screen.getByText("tester@nas.example")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "home" }));
    expect(screen.getByRole("button", { name: "nas" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "eu-api" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "bastion" })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "home group, 1 connections" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "home/eu group, 1 connections" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "home/eu" }));
    expect(screen.getByRole("button", { name: "eu-api" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "nas" })).not.toBeInTheDocument();
    expect(screen.queryByRole("region", { name: /^home group/ })).not.toBeInTheDocument();
    expect(screen.getByRole("region", { name: "home/eu group, 1 connections" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Group: home/eu" }));
    await user.click(within(screen.getByRole("dialog", { name: "Groups" })).getByRole("button", { name: /^All/ }));
    await user.type(screen.getByRole("searchbox", { name: "Filter connections" }), "nas.example");
    expect(screen.getByRole("button", { name: "nas" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "bastion" })).not.toBeInTheDocument();
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
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onDrop={onDrop} />,
    );
    const source = screen.getByRole("button", { name: "nas" });
    const target = screen.getByRole("button", { name: "empty" });
    const dataTransfer = transfer(payload);
    fireEvent.dragStart(source, { dataTransfer });
    fireEvent.dragOver(target, { dataTransfer });
    fireEvent.drop(target, { dataTransfer });
    expect(onDrop).toHaveBeenCalledWith(payload, "empty");

    onDrop.mockClear();
    harness.rerender(
      <ConnectionTree overview={overview} selected={null} onSelect={vi.fn()} onDrop={onDrop} movesDisabled />,
    );
    const disabledSource = screen.getByRole("button", { name: "nas" });
    expect(disabledSource).not.toHaveAttribute("draggable", "true");
    fireEvent.dragStart(disabledSource, { dataTransfer });
    fireEvent.drop(screen.getByRole("button", { name: "empty" }), { dataTransfer });
    expect(onDrop).not.toHaveBeenCalled();
  });

  describe("choosing several connections", () => {
    function ChoosingTree({ onDrop = vi.fn(), multiSelectDisabled = false }: { onDrop?: (payload: DragPayload, target: string) => void; multiSelectDisabled?: boolean }) {
      const [selected, setSelected] = useState<HostSelection | null>(null);
      const [checked, setChecked] = useState(emptyMultiSelection);
      return (
        <ConnectionTree
          overview={overview}
          selected={selected}
          onSelect={(chosen) => setSelected(chosen.identity)}
          onDrop={onDrop}
          checked={checked}
          onCheckedChange={setChecked}
          multiSelectDisabled={multiSelectDisabled}
        />
      );
    }

    function chosen(): string[] {
      return screen.getAllByRole("button", { pressed: true }).map((button) => button.getAttribute("aria-label") ?? "")
        .filter((name) => ["nas", "eu-api", "bastion"].includes(name));
    }

    it("chooses everything between the open connection and a Shift click, in screen order", () => {
      render(<ChoosingTree />);
      fireEvent.click(screen.getByRole("button", { name: "nas" }));
      fireEvent.click(screen.getByRole("button", { name: "bastion" }), { shiftKey: true });
      expect(chosen()).toEqual(["nas", "eu-api", "bastion"]);
    });

    it("adds one connection at a time with Ctrl and clears the choice with Escape", () => {
      render(<ChoosingTree />);
      fireEvent.click(screen.getByRole("button", { name: "nas" }));
      fireEvent.click(screen.getByRole("button", { name: "bastion" }), { ctrlKey: true });
      expect(chosen()).toEqual(["nas", "bastion"]);
      fireEvent.keyDown(window, { key: "Escape" });
      expect(screen.queryAllByRole("button", { pressed: true }).filter((button) => button.getAttribute("aria-label") === "nas")).toEqual([]);
      expect(screen.getByRole("button", { name: "nas" })).toHaveAttribute("aria-current", "true");
    });

    it("keeps showing the one connection still chosen after Ctrl takes the open one out", () => {
      render(<ChoosingTree />);
      fireEvent.click(screen.getByRole("button", { name: "nas" }));
      fireEvent.click(screen.getByRole("button", { name: "bastion" }), { ctrlKey: true });
      fireEvent.click(screen.getByRole("button", { name: "nas" }), { ctrlKey: true });
      expect(chosen()).toEqual(["bastion"]);
      expect(screen.getByRole("button", { name: "nas" })).toHaveAttribute("aria-pressed", "false");
      expect(screen.getByRole("button", { name: "nas" })).toHaveAttribute("aria-current", "true");
    });

    it("opens the clicked connection instead while the open one has unsaved edits", () => {
      render(<ChoosingTree multiSelectDisabled />);
      fireEvent.click(screen.getByRole("button", { name: "nas" }));
      fireEvent.click(screen.getByRole("button", { name: "bastion" }), { shiftKey: true });
      expect(screen.getByRole("button", { name: "bastion" })).toHaveAttribute("aria-current", "true");
      expect(chosen()).toEqual([]);
    });

    it("forgets a chosen connection once the search hides it", async () => {
      render(<ChoosingTree />);
      fireEvent.click(screen.getByRole("button", { name: "nas" }));
      fireEvent.click(screen.getByRole("button", { name: "bastion" }), { ctrlKey: true });
      await userEvent.type(screen.getByRole("searchbox"), "bastion");
      await userEvent.clear(screen.getByRole("searchbox"));
      fireEvent.click(screen.getByRole("button", { name: "eu-api" }), { ctrlKey: true });
      expect(chosen()).toEqual(["eu-api", "bastion"]);
    });

    it("drags every chosen connection onto a group together", () => {
      const onDrop = vi.fn();
      render(<ChoosingTree onDrop={onDrop} />);
      fireEvent.click(screen.getByRole("button", { name: "nas" }));
      fireEvent.click(screen.getByRole("button", { name: "bastion" }), { ctrlKey: true });
      const dataTransfer = transfer({ kind: "connection", path: "", alias: "", group: "" });
      fireEvent.dragStart(screen.getByRole("button", { name: "bastion" }), { dataTransfer });
      const target = screen.getByRole("button", { name: "empty" });
      fireEvent.dragOver(target, { dataTransfer });
      fireEvent.drop(target, { dataTransfer });
      expect(onDrop).toHaveBeenCalledWith({
        kind: "connections",
        items: [
          { path: homeNas.identity.path, alias: "nas", group: "home" },
          { path: "config", alias: "bastion", group: "" },
        ],
      }, "empty");
    });
  });
});
