import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostEntry } from "../api/config";
import { SFTPHostPicker } from "./SFTPHostPicker";

const hosts = [
  { identity: { alias: "edge", path: "edge.conf" }, group: "work", hostName: "edge.example.com", user: "deploy" },
  { identity: { alias: "miyabi", path: "miyabi.conf" }, group: "home", hostName: "192.0.2.10", user: "aida" },
] as HostEntry[];

describe("SFTPHostPicker", () => {
  it("searches host metadata and switches between recent and grouped views", async () => {
    const onChange = vi.fn();
    render(<SFTPHostPicker
      aliases={["edge", "miyabi"]}
      hosts={hosts}
      value=""
      loadRecent={async () => ({ connections: [{ alias: "miyabi", hostName: "192.0.2.10", user: "aida", port: "22", lastConnectedAt: "2026-09-01T07:00:00Z" }] })}
      onChange={onChange}
    />);

    await userEvent.click(screen.getByRole("button", { name: "Host" }));
    expect(await screen.findByRole("tab", { name: "Recent" })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("button", { name: /miyabi/ })).toHaveTextContent("Last connected");

    await userEvent.click(screen.getByRole("tab", { name: "Groups" }));
    expect(screen.getByRole("region", { name: "home" })).toBeVisible();
    expect(screen.getByRole("region", { name: "work" })).toBeVisible();

    await userEvent.type(screen.getByRole("searchbox", { name: "Search remote hosts" }), "deploy");
    expect(screen.getByRole("button", { name: /edge/ })).toBeVisible();
    expect(screen.queryByRole("button", { name: /miyabi/ })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: /edge/ }));
    expect(onChange).toHaveBeenCalledWith("edge");
  });

  it("moves between host views with the shared tab keyboard behavior", async () => {
    render(<SFTPHostPicker
      aliases={["edge", "miyabi"]}
      hosts={hosts}
      value=""
      loadRecent={async () => ({ connections: [{ alias: "miyabi", hostName: "192.0.2.10", user: "aida", port: "22", lastConnectedAt: "2026-09-01T07:00:00Z" }] })}
      onChange={() => undefined}
    />);

    await userEvent.click(screen.getByRole("button", { name: "Host" }));
    const recent = await screen.findByRole("tab", { name: "Recent" });
    const groups = screen.getByRole("tab", { name: "Groups" });
    recent.focus();
    await userEvent.keyboard("{ArrowRight}");

    expect(groups).toHaveFocus();
    expect(groups).toHaveAttribute("aria-selected", "true");
    expect(recent).toHaveAttribute("tabindex", "-1");
  });
});
