import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Overview } from "../api/config";
import { QuickConnectBrowser } from "./QuickConnectBrowser";

const overview = {
  entry: { absolute: "/Users/test/.ssh/config", path: "config" },
  files: [],
  hosts: [
    {
      identity: { path: "connections/home.conf", alias: "nas" },
      file: { absolute: "/Users/test/.ssh/connections/home.conf", path: "connections/home.conf" },
      line: 1,
      patterns: ["nas"],
      editable: true,
      group: "home",
    },
    {
      identity: { path: "connections/home/eu.conf", alias: "eu-api" },
      file: { absolute: "/Users/test/.ssh/connections/home/eu.conf", path: "connections/home/eu.conf" },
      line: 1,
      patterns: ["eu-api"],
      editable: true,
      group: "home/eu",
    },
    {
      identity: { path: "config", alias: "bastion" },
      file: { absolute: "/Users/test/.ssh/config", path: "config" },
      line: 4,
      patterns: ["bastion"],
      editable: true,
    },
  ],
  groups: [
    { name: "home", directory: "connections/home", keyDirectory: "keys/home", memberCount: 2, directoryPresent: true },
    { name: "home/eu", directory: "connections/home/eu", keyDirectory: "keys/home/eu", memberCount: 1, directoryPresent: true },
    { name: "empty", directory: "connections/empty", keyDirectory: "keys/empty", memberCount: 0, directoryPresent: true },
  ],
  metadata: {
    schemaVersion: 1,
    hosts: [
      { identity: { path: "connections/home/eu.conf", alias: "eu-api" }, tags: ["production"], favourite: true },
    ],
  },
  diagnostics: [],
  notices: [],
  pending: [],
} as Overview;

describe("QuickConnectBrowser", () => {
  it("starts with every server visible and keeps favourites first", () => {
    const connect = vi.fn();
    render(
      <QuickConnectBrowser
        overview={overview}
        launching=""
        onConnect={connect}
        onOpenSettings={vi.fn()}
      />,
    );

    const modes = screen.getByRole("group", { name: "Browse connections by" });
    expect(within(modes).getByRole("button", { name: "Servers" })).toHaveAttribute("aria-pressed", "true");
    const cards = Array.from(screen.getByRole("list", { name: "Available connections" }).children);
    expect(cards).toHaveLength(3);
    expect(cards[0]).toHaveTextContent("eu-api");
    expect(screen.getByText("nas")).toBeInTheDocument();
    expect(screen.getByText("bastion")).toBeInTheDocument();
    expect(connect).not.toHaveBeenCalled();
  });

  it("drills through group levels without changing or connecting to a server", async () => {
    const connect = vi.fn();
    render(
      <QuickConnectBrowser
        overview={overview}
        launching=""
        onConnect={connect}
        onOpenSettings={vi.fn()}
      />,
    );

    const modes = screen.getByRole("group", { name: "Browse connections by" });
    await userEvent.click(within(modes).getByRole("button", { name: "Groups" }));
    const rootGroups = screen.getByRole("list", { name: "Groups" });
    expect(within(rootGroups).getByRole("button", { name: "home, 2 servers" })).toBeInTheDocument();
    expect(within(rootGroups).getByRole("button", { name: "empty, 0 servers" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "eu, 1 server" })).not.toBeInTheDocument();

    await userEvent.click(within(rootGroups).getByRole("button", { name: "home, 2 servers" }));
    expect(screen.getByRole("button", { name: "eu, 1 server" })).toBeInTheDocument();
    expect(screen.getByText("nas")).toBeInTheDocument();
    expect(screen.queryByText("eu-api")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "eu, 1 server" }));
    expect(screen.getByText("eu-api")).toBeInTheDocument();
    expect(screen.queryByText("nas")).not.toBeInTheDocument();

    const crumbs = screen.getByRole("navigation", { name: "Group path" });
    await userEvent.click(within(crumbs).getByRole("button", { name: "home" }));
    expect(screen.getByText("nas")).toBeInTheDocument();
    expect(connect).not.toHaveBeenCalled();
  });

  it("searches within the selected group and can show favourites only", async () => {
    render(
      <QuickConnectBrowser
        overview={overview}
        launching=""
        onConnect={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    const modes = screen.getByRole("group", { name: "Browse connections by" });
    await userEvent.click(within(modes).getByRole("button", { name: "Groups" }));
    await userEvent.click(screen.getByRole("button", { name: "home, 2 servers" }));
    await userEvent.type(screen.getByRole("searchbox", { name: "Search connections" }), "production");
    expect(screen.getByText("eu-api")).toBeInTheDocument();
    expect(screen.queryByText("nas")).not.toBeInTheDocument();

    await userEvent.clear(screen.getByRole("searchbox", { name: "Search connections" }));
    await userEvent.click(screen.getByRole("button", { name: "Favourites" }));
    expect(screen.getByRole("button", { name: "eu, 1 server" })).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "eu, 1 server" }));
    expect(screen.getByText("eu-api")).toBeInTheDocument();
    expect(screen.queryByText("nas")).not.toBeInTheDocument();
  });

  it("keeps opening settings and connecting as separate explicit actions", async () => {
    const connect = vi.fn();
    const openSettings = vi.fn();
    render(
      <QuickConnectBrowser
        overview={overview}
        launching=""
        onConnect={connect}
        onOpenSettings={openSettings}
      />,
    );

    const card = screen.getByText("nas").closest("li");
    expect(card).not.toBeNull();
    await userEvent.click(within(card as HTMLElement).getByRole("button", { name: "Actions for nas" }));
    expect(within(card as HTMLElement).getByRole("menu")).toHaveClass("bottom-full", "mb-1");
    await userEvent.click(within(card as HTMLElement).getByRole("menuitem", { name: "Open connection settings" }));
    expect(openSettings).toHaveBeenCalledWith(
      "/connections/servers?path=connections%2Fhome.conf&host=nas&panel=basic",
    );
    expect(connect).not.toHaveBeenCalled();

    await userEvent.click(within(card as HTMLElement).getByRole("button", { name: "Actions for nas" }));
    await userEvent.click(within(card as HTMLElement).getByRole("menuitem", { name: "Connect" }));
    expect(connect).toHaveBeenCalledWith("nas");
  });

  it("recovers locally when the selected group disappears after an overview refresh", async () => {
    const harness = render(
      <QuickConnectBrowser
        overview={overview}
        launching=""
        onConnect={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    const modes = screen.getByRole("group", { name: "Browse connections by" });
    await userEvent.click(within(modes).getByRole("button", { name: "Groups" }));
    await userEvent.click(screen.getByRole("button", { name: "home, 2 servers" }));
    harness.rerender(
      <QuickConnectBrowser
        overview={{ ...overview, groups: overview.groups.filter((group) => !group.name.startsWith("home")) }}
        launching=""
        onConnect={vi.fn()}
        onOpenSettings={vi.fn()}
      />,
    );

    expect(screen.getByText("The selected group home no longer exists.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Back to group root" }));
    expect(screen.getByRole("list", { name: "Groups" })).toBeInTheDocument();
  });
});
