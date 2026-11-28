import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { HostEntry } from "../api/config";
import { LanguageProvider } from "../i18n/context";
import type { HostLook } from "../shell/useDeclaredConfig";
import { localHostAlias } from "./localHost";
import { SFTPQuickHosts } from "./SFTPQuickHosts";

function host(alias: string, fields: Partial<HostEntry> = {}): HostEntry {
  return {
    identity: { path: "conf.d/hosts.conf", alias },
    file: { absolute: "/home/user/.ssh/conf.d/hosts.conf" },
    line: 1,
    patterns: [alias],
    editable: true,
    ...fields,
  };
}

const hosts = [
  host("web-01", { hostName: "203.0.113.11", user: "deploy", group: "production" }),
  host("web-02", { hostName: "203.0.113.12", user: "deploy" }),
  host("db"),
];
const looks = new Map<string, HostLook>([["web-02", { os: "ubuntu", colour: "#2f7fd8" }]]);

function renderQuickHosts(onChoose = vi.fn()) {
  const loadRecent = vi.fn().mockResolvedValue({
    connections: [
      { alias: "web-02", lastConnectedAt: "2026-10-30T00:00:00Z" },
      { alias: "removed", lastConnectedAt: "2026-10-29T00:00:00Z" },
      { alias: "web-01", lastConnectedAt: "2026-10-28T00:00:00Z" },
    ],
  });
  render(
    <LanguageProvider initial="en">
      <SFTPQuickHosts aliases={["web-01", "web-02", "db"]} hosts={hosts} hostLook={looks} onChoose={onChoose} loadRecent={loadRecent} />
    </LanguageProvider>,
  );
  return screen.getByRole("list", { name: "Open quickly" });
}

describe("SFTPQuickHosts", () => {
  it("offers this machine and the recent connections that still exist, and opens one with a press", async () => {
    const onChoose = vi.fn();
    const list = renderQuickHosts(onChoose);

    await within(list).findByRole("button", { name: "web-02" });
    expect(within(list).getAllByRole("button").map((button) => button.getAttribute("aria-label"))).toEqual(["Local", "web-02", "web-01"]);
    await userEvent.click(within(list).getByRole("button", { name: "web-01" }));
    expect(onChoose).toHaveBeenCalledWith("web-01");
    await userEvent.click(within(list).getByRole("button", { name: "Local" }));
    expect(onChoose).toHaveBeenCalledWith(localHostAlias);
  });

  it("shows each connection as on Home: its address, group, operating system and when it was last connected", async () => {
    const list = renderQuickHosts();

    const web01 = await within(list).findByRole("button", { name: "web-01" });
    expect(web01).toHaveTextContent("deploy@203.0.113.11");
    expect(web01).toHaveTextContent("production");
    expect(within(web01).getByLabelText(/^Last connected/)).toBeInTheDocument();
    const web02 = within(list).getByRole("button", { name: "web-02" });
    expect(within(web02).getByRole("img", { name: "Ubuntu" })).toBeInTheDocument();
    expect(within(list).getByRole("button", { name: "Local" })).toHaveTextContent("sshc engine");
  });
});
