import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { HostEntry } from "../api/config";
import { DeleteConnectionsBody } from "./DeleteConnectionsBody";

function host(alias: string, proxyJump?: string): HostEntry {
  return { identity: { path: "config", alias }, file: { path: "config", absolute: "/home/me/.ssh/config" }, line: 1, patterns: [alias], editable: true, ...(proxyJump === undefined ? {} : { proxyJump }) };
}

describe("DeleteConnectionsBody", () => {
  it("names the connections that go through the deleted one as a jump host", () => {
    render(<DeleteConnectionsBody body="This removes the Host block." deleting={["jump"]} hosts={[host("jump"), host("target-b", "jump"), host("direct")]} />);
    expect(screen.getByText(/go through a connection you are deleting as a jump host/)).toBeVisible();
    expect(within(screen.getByRole("list")).getByText("target-b")).toBeVisible();
    expect(screen.queryByText("direct")).not.toBeInTheDocument();
  });

  it("says nothing about jump hosts when no other connection goes through the deleted ones", () => {
    render(<DeleteConnectionsBody body="This removes the Host block." deleting={["direct"]} hosts={[host("jump"), host("target-b", "jump"), host("direct")]} />);
    expect(screen.queryByText(/jump host/)).not.toBeInTheDocument();
  });
});
