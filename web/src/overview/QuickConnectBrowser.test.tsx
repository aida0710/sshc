import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Overview } from "../api/config";
import type { RecentConnection } from "../api/integrations";
import { QuickConnectBrowser } from "./QuickConnectBrowser";

const overview = {
  entry: { absolute: "/Users/test/.ssh/config", path: "config" },
  files: [],
  hosts: [
    {
      identity: { path: "connections/home/nas.conf", alias: "nas" },
      file: { absolute: "/Users/test/.ssh/connections/home/nas.conf", path: "connections/home/nas.conf" },
      line: 1,
      patterns: ["nas"],
      editable: true,
      group: "home",
      hostName: "nas.lan",
      user: "admin",
      port: "22",
    },
    {
      identity: { path: "connections/home/lab/eu.conf", alias: "eu-api" },
      file: { absolute: "/Users/test/.ssh/connections/home/lab/eu.conf", path: "connections/home/lab/eu.conf" },
      line: 1,
      patterns: ["eu-api"],
      editable: true,
      group: "home/lab",
      hostName: "eu.example",
      user: "deploy",
      port: "2222",
    },
    {
      identity: { path: "config", alias: "bastion" },
      file: { absolute: "/Users/test/.ssh/config", path: "config" },
      line: 4,
      patterns: ["bastion"],
      editable: true,
      hostName: "203.0.113.10",
      user: "ops",
      port: "22",
    },
  ],
  groups: [
    { name: "home", directory: "connections/home", keyDirectory: "keys/home", memberCount: 2, directoryPresent: true },
    { name: "home/lab", directory: "connections/home/lab", keyDirectory: "keys/home/lab", memberCount: 1, directoryPresent: true },
  ],
  metadata: {
    schemaVersion: 1,
    hosts: [
      { identity: { path: "connections/home/lab/eu.conf", alias: "eu-api" }, tags: ["production"] },
    ],
  },
  diagnostics: [],
  notices: [],
  pending: [],
} as Overview;

const recent = [{
  alias: "nas",
  hostName: "old-nas.example",
  user: "previous-user",
  port: "2200",
  lastConnectedAt: "2026-08-24T15:30:00Z",
}] as RecentConnection[];

function renderBrowser(overrides: Partial<React.ComponentProps<typeof QuickConnectBrowser>> = {}) {
  return render(
    <QuickConnectBrowser
      overview={overview}
      recent={recent}
      launching=""
      onConnect={vi.fn()}
      onOpenSettings={vi.fn()}
      {...overrides}
    />,
  );
}

describe("QuickConnectBrowser", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("shows detected OS icons, manual overrides, and unknown hosts", () => {
    const data = structuredClone(overview);
    data.metadata.hosts = [
      { identity: data.hosts[0]!.identity, detectedOS: "ubuntu" },
      { identity: data.hosts[1]!.identity, detectedOS: "redhat", os: "debian" },
    ];
    renderBrowser({ overview: data });
    expect(screen.getByRole("img", { name: "Ubuntu" })).toBeVisible();
    expect(screen.getByRole("img", { name: "Debian" })).toBeVisible();
    expect(screen.queryByRole("img", { name: "Red Hat Enterprise Linux" })).not.toBeInTheDocument();
    expect(screen.getByRole("img", { name: "OS unknown" })).toBeVisible();
  });

  it("orders recent connections first and shows the resolved destination", () => {
    renderBrowser();

    const groupGrid = screen.getByRole("group", { name: "Filter connections by group" });
    expect(within(groupGrid).getAllByRole("button")).toHaveLength(1);
    expect(screen.getByRole("heading", { name: "Connections 3" })).toBeInTheDocument();

    const list = screen.getByRole("list", { name: "Available connections" });
    const cards = within(list).getAllByRole("listitem");
    expect(cards[0]).toHaveTextContent("nas");
    expect(cards[1]).toHaveTextContent("bastion");
    expect(cards[2]).toHaveTextContent("eu-api");
    expect(cards[0]).toHaveTextContent("admin@nas.lan:22");
    expect(screen.queryByText("No group")).not.toBeInTheDocument();
    expect(screen.queryByText("Not connected yet")).not.toBeInTheDocument();
  });

  it("drills into direct child groups and aggregates every descendant connection", async () => {
    renderBrowser();

    expect(screen.queryByRole("button", { name: "Open home/lab, 1 connections" })).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Open home, 2 connections" }));

    expect(screen.getByRole("navigation", { name: "Selected group" })).toHaveTextContent("All/home");
    expect(screen.getByRole("heading", { name: "Connections 2" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Open home/lab, 1 connections" })).toBeInTheDocument();
    expect(screen.getByText("nas")).toBeInTheDocument();
    expect(screen.getByText("eu-api")).toBeInTheDocument();
    expect(screen.queryByText("bastion")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: "Open home/lab, 1 connections" }));
    expect(screen.queryByRole("group", { name: "Filter connections by group" })).not.toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Connections 1" })).toBeInTheDocument();
    expect(screen.queryByText("nas")).toBeNull();
    expect(screen.getByText("eu-api")).toBeInTheDocument();

    await userEvent.click(within(screen.getByRole("navigation", { name: "Selected group" })).getByRole("button", { name: "All" }));
    expect(screen.getByRole("heading", { name: "Connections 3" })).toBeInTheDocument();
    expect(screen.getByText("bastion")).toBeInTheDocument();
  });

  it("searches only inside the selected group subtree", async () => {
    renderBrowser();

    await userEvent.click(screen.getByRole("button", { name: "Open home, 2 connections" }));

    await userEvent.type(screen.getByRole("searchbox", { name: "Search connections" }), "production");
    expect(screen.queryByText("nas")).toBeNull();
    expect(screen.getByText("eu-api")).toBeInTheDocument();
  });

  it("selects with one mouse click, connects with a double click, and connects with one touch", async () => {
    const connect = vi.fn();
    renderBrowser({ onConnect: connect });

    const nas = screen.getByRole("button", { name: /^Connect to nas\./ });
    fireEvent.pointerDown(nas, { pointerType: "mouse" });
    fireEvent.click(nas);
    expect(connect).not.toHaveBeenCalled();
    expect(nas).toHaveAttribute("aria-pressed", "true");

    fireEvent.doubleClick(nas);
    expect(connect).toHaveBeenCalledWith("nas");

    connect.mockClear();
    const eu = screen.getByRole("button", { name: /^Connect to eu-api\./ });
    fireEvent.pointerDown(eu, { pointerType: "touch" });
    fireEvent.click(eu);
    expect(connect).toHaveBeenCalledWith("eu-api");

    connect.mockClear();
    nas.focus();
    await userEvent.keyboard("{Enter}");
    expect(connect).toHaveBeenCalledExactlyOnceWith("nas");
  });

  it("connects with the separate button and keeps other card actions independent", async () => {
    const connect = vi.fn();
    renderBrowser({ onConnect: connect });

    const button = screen.getByRole("button", { name: "Connect to nas" });
    const card = button.closest("li")!;
    await userEvent.click(button);
    expect(connect).toHaveBeenCalledExactlyOnceWith("nas");
    expect(within(card).getByRole("button", { name: /^Connect to nas\./ })).toHaveAttribute("aria-pressed", "true");

    await userEvent.click(within(card).getByRole("button", { name: "Actions for nas" }));
    expect(connect).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("menu")).toBeVisible();
  });

  it("shows the pending host and prevents another launch while opening", async () => {
    const connect = vi.fn();
    renderBrowser({ onConnect: connect, launching: "nas" });
    const opening = screen.getByRole("button", { name: "Opening nas…" });
    expect(opening).toBeDisabled();
    expect(opening).toHaveTextContent("Opening…");
    expect(opening.closest("li")).toHaveAttribute("aria-busy", "true");
    const another = screen.getByRole("button", { name: "Connect to eu-api" });
    expect(another).toBeDisabled();
    await userEvent.click(another);
    expect(connect).not.toHaveBeenCalled();
  });

  it("persists the selected display mode", async () => {
    const first = renderBrowser();
    const layout = screen.getByRole("group", { name: "Connection layout" });
    await userEvent.click(within(layout).getByRole("button", { name: "List" }));
    expect(screen.getByRole("list", { name: "Available connections" })).toHaveClass("flex", "flex-col");
    first.unmount();

    renderBrowser();
    expect(within(screen.getByRole("group", { name: "Connection layout" })).getByRole("button", { name: "List" })).toHaveAttribute("aria-pressed", "true");
  });

  it("keeps settings and connect as explicit menu actions", async () => {
    const connect = vi.fn();
    const openSettings = vi.fn();
    renderBrowser({ onConnect: connect, onOpenSettings: openSettings });

    await userEvent.click(screen.getByRole("button", { name: "Actions for nas" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Open connection settings" }));
    expect(openSettings).toHaveBeenCalledWith("/connections/servers?path=connections%2Fhome%2Fnas.conf&host=nas&panel=basic");
    expect(connect).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "Actions for nas" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Connect" }));
    expect(connect).toHaveBeenCalledWith("nas");
  });
});
