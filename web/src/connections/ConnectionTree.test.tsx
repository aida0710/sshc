import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionGroupFacets } from "./ConnectionGroupFacets";
import { ConnectionTree, type ConnectionTreeProps, type HostSelection, type Scope } from "./ConnectionTree";
import { dragMimeType, type DragPayload } from "./dragdrop";
import { emptyMultiSelection } from "./multiSelection";
import { useConnectionDrag } from "./useConnectionDrag";

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

// ConnectionLists puts the list of groups beside the connection list with one drag between them, as the
// Connections page does.
function ConnectionLists({
  overview: shown = overview,
  onDrop = vi.fn(),
  movesDisabled = false,
  ...treeProps
}: Partial<Omit<ConnectionTreeProps, "drag">> & {
  onDrop?: (payload: DragPayload, target: string) => void;
  movesDisabled?: boolean;
}) {
  const [scope, setScope] = useState<Scope>({ kind: "all" });
  const drag = useConnectionDrag({ movesDisabled, groupNames: shown.groups.map((group) => group.name), onDrop });
  return (
    <>
      <ConnectionGroupFacets overview={shown} scope={scope} onScopeChange={setScope} drag={drag} />
      <ConnectionTree overview={shown} selected={null} onSelect={vi.fn()} scope={scope} onScopeChange={setScope} drag={drag} {...treeProps} />
    </>
  );
}

describe("the connection list and the list of groups", () => {
  it("shows the complete declared hierarchy and ungrouped connections at once", () => {
    render(
      <ConnectionLists />,
    );

    const groups = screen.getByRole("navigation", { name: "Groups" });
    const tree = screen.getByRole("navigation", { name: "Connections" });
    expect(within(tree).queryByRole("group", { name: "Arrange connections by" })).not.toBeInTheDocument();
    expect(within(groups).getByRole("button", { name: "home" })).toBeInTheDocument();
    expect(within(groups).getByRole("button", { name: "home/eu" })).toBeInTheDocument();
    expect(within(groups).getByRole("button", { name: "empty" })).toBeInTheDocument();
    expect(within(groups).getByRole("button", { name: "Ungrouped" })).toBeInTheDocument();
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
      <ConnectionLists />,
    );

    const handle = screen.getByRole("separator", { name: "Resize the groups column" });
    expect(handle).toHaveAttribute("aria-valuenow", "220");
    fireEvent.keyDown(handle, { key: "ArrowRight", shiftKey: true });
    expect(handle).toHaveAttribute("aria-valuenow", "252");
    expect(window.localStorage.getItem("sshc.connections.groups-width.v1")).toBe("252");
    fireEvent.keyDown(handle, { key: "Home" });
    expect(handle).toHaveAttribute("aria-valuenow", "112");
  });

  it("keeps pattern rules out of Connections", () => {
    render(
      <ConnectionLists />,
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
      <ConnectionLists overview={orderOverview} />,
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
      <ConnectionLists />,
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
        <ConnectionLists overview={repeatedOverview} />,
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
      <ConnectionLists />,
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
      <ConnectionLists onDrop={onDrop} />,
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
      <ConnectionLists onDrop={onDrop} movesDisabled />,
    );
    const disabledSource = screen.getByRole("button", { name: "nas" });
    expect(disabledSource).not.toHaveAttribute("draggable", "true");
    fireEvent.dragStart(disabledSource, { dataTransfer });
    fireEvent.drop(screen.getByRole("button", { name: "empty" }), { dataTransfer });
    expect(onDrop).not.toHaveBeenCalled();
  });

  it("outlines the groups a dragged connection can land on, and fills only the one under the pointer", () => {
    render(
      <ConnectionLists />,
    );
    const groups = screen.getByRole("navigation", { name: "Groups" });
    const groupButton = (name: string) => within(groups).getByRole("button", { name });
    // 落とし先の見た目は、グループのボタンを包む行に付く。
    const dropZone = (name: string) => groupButton(name).parentElement!;
    const source = screen.getByRole("button", { name: "nas" });
    const dataTransfer = transfer({ kind: "connection", path: homeNas.identity.path, alias: "nas", group: "home" });

    expect(dropZone("empty")).not.toHaveClass("outline-dashed");
    fireEvent.dragStart(source, { dataTransfer });
    expect(dropZone("empty")).toHaveClass("outline-dashed");
    expect(dropZone("home/eu")).toHaveClass("outline-dashed");
    expect(dropZone("Ungrouped")).toHaveClass("outline-dashed");
    // 今いるグループへは落とせないので、点線も付けない。
    expect(dropZone("home")).not.toHaveClass("outline-dashed");

    fireEvent.dragOver(groupButton("empty"), { dataTransfer });
    expect(dropZone("empty")).toHaveClass("outline-solid", "bg-accent/20");
    expect(dropZone("empty")).not.toHaveClass("outline-dashed");
    expect(dropZone("home/eu")).toHaveClass("outline-dashed");
    expect(dropZone("Ungrouped")).toHaveClass("outline-dashed");

    fireEvent.dragOver(groupButton("home/eu"), { dataTransfer });
    expect(dropZone("home/eu")).toHaveClass("outline-solid");
    expect(dropZone("empty")).toHaveClass("outline-dashed");
    expect(dropZone("empty")).not.toHaveClass("outline-solid");

    fireEvent.dragLeave(dropZone("home/eu"), { dataTransfer });
    expect(dropZone("home/eu")).toHaveClass("outline-dashed");
    expect(dropZone("home/eu")).not.toHaveClass("outline-solid");

    fireEvent.dragEnd(source, { dataTransfer });
    for (const name of ["empty", "home/eu", "Ungrouped"]) {
      expect(dropZone(name)).not.toHaveClass("outline-dashed");
      expect(dropZone(name)).not.toHaveClass("outline-solid");
    }
  });

  describe("choosing several connections", () => {
    function ChoosingTree({ onDrop = vi.fn(), multiSelectDisabled = false }: { onDrop?: (payload: DragPayload, target: string) => void; multiSelectDisabled?: boolean }) {
      const [selected, setSelected] = useState<HostSelection | null>(null);
      const [checked, setChecked] = useState(emptyMultiSelection);
      return (
        <ConnectionLists
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

    it.each([
      { isComposing: true },
      { isComposing: false, keyCode: 229 },
    ])("preserves selected connections on composition Escape (%j)", (composition) => {
      render(<ChoosingTree />);
      fireEvent.click(screen.getByRole("button", { name: "nas" }));
      fireEvent.click(screen.getByRole("button", { name: "bastion" }), { ctrlKey: true });
      fireEvent.keyDown(window, { key: "Escape", ...composition });
      expect(chosen()).toEqual(["nas", "bastion"]);
      fireEvent.keyDown(window, { key: "Escape" });
      expect(chosen()).toEqual([]);
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
