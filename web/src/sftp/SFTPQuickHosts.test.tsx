import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { localHostAlias } from "./localHost";
import { SFTPQuickHosts } from "./SFTPQuickHosts";

describe("SFTPQuickHosts", () => {
  it("offers this machine and the recent connections that still exist, and opens one with a press", async () => {
    const onChoose = vi.fn();
    const loadRecent = vi.fn().mockResolvedValue({
      connections: [
        { alias: "web-02", lastConnectedAt: "2026-10-30T00:00:00Z" },
        { alias: "removed", lastConnectedAt: "2026-10-29T00:00:00Z" },
        { alias: "web-01", lastConnectedAt: "2026-10-28T00:00:00Z" },
      ],
    });
    render(
      <LanguageProvider initial="en">
        <SFTPQuickHosts aliases={["web-01", "web-02", "db"]} onChoose={onChoose} loadRecent={loadRecent} />
      </LanguageProvider>,
    );

    const list = screen.getByRole("list", { name: "Open quickly" });
    await within(list).findByRole("button", { name: "web-02" });
    expect(within(list).getAllByRole("button").map((button) => button.textContent)).toEqual(["Local", "web-02", "web-01"]);
    await userEvent.click(within(list).getByRole("button", { name: "web-01" }));
    expect(onChoose).toHaveBeenCalledWith("web-01");
    await userEvent.click(within(list).getByRole("button", { name: "Local" }));
    expect(onChoose).toHaveBeenCalledWith(localHostAlias);
  });
});
