import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostEntry, Overview } from "../api/config";
import type { ConnectionBrowserLocation } from "../routing/connectionRoute";
import { ConnectionBrowser } from "./ConnectionBrowserView";
import { dragMimeType, type DragPayload } from "./dragdrop";

function host(path: string, alias: string, group?: string): HostEntry {
  return {
    identity: { path, alias },
    file: { path, absolute: `/home/tester/.ssh/${path}` },
    line: 1,
    patterns: alias === "" ? ["*"] : [alias],
    ...(alias === "" ? { wildcard: true } : {}),
    editable: true,
    ...(group === undefined ? {} : { group }),
  };
}

const homeNas = host("connections/home/nas.conf", "nas", "home");
const workNas = host("connections/work/nas.conf", "nas", "work");
const euApi = host("connections/home/eu/api.conf", "eu-api", "home/eu");
const bastion = host("config", "bastion");

const overview: Overview = {
  entry: { path: "config", absolute: "/home/tester/.ssh/config" },
  files: [],
  hosts: [bastion, euApi, homeNas, workNas, host("config", "")],
  groups: [
    {
      name: "home",
      directory: "connections/home",
      keyDirectory: "keys/home",
      memberCount: 1,
      directoryPresent: true,
    },
    {
      name: "home/eu",
      parent: "home",
      directory: "connections/home/eu",
      keyDirectory: "keys/home/eu",
      memberCount: 1,
      directoryPresent: true,
    },
    {
      name: "home/us",
      parent: "home",
      directory: "connections/home/us",
      keyDirectory: "keys/home/us",
      memberCount: 0,
      directoryPresent: true,
    },
    {
      name: "work",
      directory: "connections/work",
      keyDirectory: "keys/work",
      memberCount: 1,
      directoryPresent: true,
    },
    {
      name: "empty",
      directory: "connections/empty",
      keyDirectory: "keys/empty",
      memberCount: 0,
      directoryPresent: true,
    },
  ],
  metadata: {
    schemaVersion: 2,
    groups: [
      { name: "home", order: 1 },
      { name: "work", order: 2 },
      { name: "empty", order: 3 },
      { name: "home/eu", order: 1 },
      { name: "home/us", order: 2 },
    ],
    hosts: [
      { identity: homeNas.identity, order: -2, favourite: true, tags: ["storage"] },
      { identity: workNas.identity, order: -1 },
      { identity: euApi.identity, favourite: true },
    ],
  },
  diagnostics: [],
  notices: [],
};

const noopProps = {
  selected: null,
  movesDisabled: false,
  onBrowse: vi.fn(),
  onSelect: vi.fn(),
  onDrop: vi.fn(),
};

function renderBrowser(browser: ConnectionBrowserLocation, overrides = {}) {
  const props = {
    ...noopProps,
    onBrowse: vi.fn(),
    onSelect: vi.fn(),
    onDrop: vi.fn(),
    ...overrides,
  };
  render(<ConnectionBrowser overview={overview} browser={browser} {...props} />);
  return props;
}

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

function drag(source: HTMLElement, target: HTMLElement, payload: DragPayload) {
  const dataTransfer = transfer(payload);
  fireEvent.dragStart(source, { dataTransfer });
  fireEvent.dragOver(target, { dataTransfer });
  fireEvent.drop(target, { dataTransfer });
}

describe("ConnectionBrowser server mode", () => {
  it("starts with one flat concrete-server list and disambiguates duplicate aliases", () => {
    renderBrowser({ view: "servers" });

    expect(screen.getByRole("button", { name: "Servers" })).toHaveAttribute("aria-pressed", "true");
    const list = screen.getByRole("list", { name: "Servers" });
    expect(within(list).getAllByRole("button", { name: /^nas/ })).toHaveLength(2);
    expect(within(list).getByText("home/eu", { exact: true })).toBeInTheDocument();
    expect(within(list).getByText("connections/home/nas.conf")).toBeInTheDocument();
    expect(within(list).getByText("connections/work/nas.conf")).toBeInTheDocument();
    expect(screen.queryByText("Host *")).not.toBeInTheDocument();
  });

  it("searches every group and keeps favourite servers in metadata order", async () => {
    const user = userEvent.setup();
    renderBrowser({ view: "servers" });

    const search = screen.getByRole("searchbox", { name: "Filter connections" });
    await user.type(search, "home/eu");
    expect(screen.getByRole("button", { name: /^eu-api/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^nas/ })).not.toBeInTheDocument();

    await user.clear(search);
    await user.click(screen.getByRole("button", { name: "Favourites" }));
    const buttons = within(screen.getByRole("list", { name: "Servers" })).getAllByRole("button");
    expect(buttons.map((button) => button.textContent)).toEqual([
      expect.stringContaining("nas"),
      expect.stringContaining("eu-api"),
    ]);
  });
});

describe("ConnectionBrowser group drilldown", () => {
  it("shows only top-level groups at root and drills into a selected child", async () => {
    const user = userEvent.setup();
    const { onBrowse } = renderBrowser({ view: "groups", scope: "root" });

    const groups = screen.getByRole("list", { name: "Groups" });
    expect(within(groups).getByRole("button", { name: /^home,/ })).toBeInTheDocument();
    expect(within(groups).queryByRole("button", { name: /^eu,/ })).not.toBeInTheDocument();
    expect(within(groups).getByRole("button", { name: /^Ungrouped,/ })).toBeInTheDocument();

    await user.click(within(groups).getByRole("button", { name: /^home,/ }));
    expect(onBrowse).toHaveBeenCalledWith({ view: "groups", scope: "named", group: "home" });
  });

  it("shows breadcrumbs, direct children, and only direct servers", async () => {
    const user = userEvent.setup();
    const { onBrowse } = renderBrowser({ view: "groups", scope: "named", group: "home" });

    const path = screen.getByRole("navigation", { name: "Group path" });
    expect(within(path).getByRole("button", { name: "Groups" })).toBeInTheDocument();
    expect(path).toHaveTextContent("home");
    const groups = screen.getByRole("list", { name: "Groups" });
    expect(within(groups).getByRole("button", { name: /^eu,/ })).toBeInTheDocument();
    expect(within(groups).getByRole("button", { name: /^us,/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^nas/ })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^eu-api/ })).not.toBeInTheDocument();

    await user.click(within(groups).getByRole("button", { name: /^eu,/ }));
    expect(onBrowse).toHaveBeenCalledWith({ view: "groups", scope: "named", group: "home/eu" });
  });

  it("flattens descendant matches during search and labels them with full group paths", async () => {
    const user = userEvent.setup();
    renderBrowser({ view: "groups", scope: "named", group: "home" });

    await user.type(screen.getByRole("searchbox", { name: "Filter connections" }), "eu-api");

    expect(screen.getByRole("button", { name: /^eu-api/ })).toBeInTheDocument();
    expect(screen.getByText("home/eu", { exact: true })).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Groups" })).not.toBeInTheDocument();
  });

  it("distinguishes an empty group, ungrouped servers, and a missing group", async () => {
    const { rerender } = render(
      <ConnectionBrowser
        overview={overview}
        browser={{ view: "groups", scope: "named", group: "empty" }}
        {...noopProps}
      />,
    );
    expect(screen.getByText("No servers are directly in this group.")).toBeInTheDocument();

    rerender(
      <ConnectionBrowser
        overview={overview}
        browser={{ view: "groups", scope: "ungrouped" }}
        {...noopProps}
      />,
    );
    expect(screen.getByRole("button", { name: /^bastion/ })).toBeInTheDocument();

    const onBrowse = vi.fn();
    rerender(
      <ConnectionBrowser
        overview={overview}
        browser={{ view: "groups", scope: "named", group: "gone" }}
        {...noopProps}
        onBrowse={onBrowse}
      />,
    );
    expect(screen.getByText("Group not found.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Back to group root" }));
    expect(onBrowse).toHaveBeenCalledWith({ view: "groups", scope: "root" });
  });
});

describe("ConnectionBrowser drag targets", () => {
  it("moves a direct server to a visible child and a child group to its sibling", () => {
    const { onDrop } = renderBrowser({ view: "groups", scope: "named", group: "home" });
    const groups = screen.getByRole("list", { name: "Groups" });
    const eu = within(groups).getByRole("button", { name: /^eu,/ });
    const us = within(groups).getByRole("button", { name: /^us,/ });
    const nas = screen.getByRole("button", { name: /^nas/ });

    const nasPayload: DragPayload = {
      kind: "connection",
      path: homeNas.identity.path,
      alias: homeNas.identity.alias,
      group: "home",
    };
    drag(nas, eu, nasPayload);
    expect(onDrop).toHaveBeenCalledWith(nasPayload, "home/eu");

    const groupPayload: DragPayload = { kind: "group", name: "home/eu" };
    drag(eu, us, groupPayload);
    expect(onDrop).toHaveBeenCalledWith(groupPayload, "home/us");
  });

  it("moves a nested server to the parent breadcrumb", () => {
    const { onDrop } = renderBrowser({ view: "groups", scope: "named", group: "home/eu" });
    const payload: DragPayload = {
      kind: "connection",
      path: euApi.identity.path,
      alias: euApi.identity.alias,
      group: "home/eu",
    };
    drag(
      screen.getByRole("button", { name: /^eu-api/ }),
      within(screen.getByRole("navigation", { name: "Group path" })).getByRole("button", {
        name: "home",
      }),
      payload,
    );
    expect(onDrop).toHaveBeenCalledWith(payload, "home");
  });

  it("does not offer dragging in server mode", () => {
    renderBrowser({ view: "servers" });
    expect(screen.getAllByRole("button", { name: /^nas/ })[0]).toHaveAttribute("draggable", "false");
  });

  it("rejects disabled moves", () => {
    const { onDrop } = renderBrowser(
      { view: "groups", scope: "named", group: "home" },
      { movesDisabled: true },
    );
    expect(screen.getByRole("button", { name: /^nas/ })).toHaveAttribute("draggable", "false");
    expect(screen.getByRole("button", { name: /^eu,/ })).toHaveAttribute("draggable", "false");
    expect(onDrop).not.toHaveBeenCalled();
  });

  it("rejects a group dropped onto itself", () => {
    const { onDrop } = renderBrowser({ view: "groups", scope: "named", group: "home" });
    const eu = screen.getByRole("button", { name: /^eu,/ });
    drag(eu, eu, { kind: "group", name: "home/eu" });
    expect(onDrop).not.toHaveBeenCalled();
  });
});
