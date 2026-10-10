import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { SFTPTabStrip } from "./SFTPTabStrip";
import { localHostAlias } from "./localHost";
import type { SFTPPane } from "./sftpPanes";

const pane: SFTPPane = {
  id: "files",
  activeId: "remote",
  tabs: [
    { id: "remote", alias: "application-server", path: "/srv/project/config", sort: { key: "name", direction: "ascending" } },
    { id: "local", alias: localHostAlias, path: "/home/demo/projects", sort: { key: "name", direction: "ascending" } },
  ],
};

function tabStripProps() {
  return {
    pane,
    label: "File tabs",
    closable: true,
    movable: () => true,
    onSelect: vi.fn(),
    onClose: vi.fn(),
    onAdd: vi.fn(),
    onDragStart: vi.fn(),
    onDragEnd: vi.fn(),
    onMove: vi.fn(),
  };
}

describe("SFTPTabStrip", () => {
  it("names each folder and host separately while keeping the full location accessible", () => {
    render(<SFTPTabStrip {...tabStripProps()} />);

    const remote = screen.getByRole("tab", { name: "application-server:config" });
    expect(within(remote).getByText("config")).toBeVisible();
    expect(within(remote).getByText("application-server")).toBeVisible();
    expect(remote).toHaveAttribute("title", "application-server:/srv/project/config");

    const local = screen.getByRole("tab", { name: "Local:projects" });
    expect(within(local).getByText("projects")).toBeVisible();
    expect(within(local).getByText("Local")).toBeVisible();
    expect(local).toHaveAttribute("tabindex", "-1");
  });

  it("changes only the selected tab's host and honours its change guard", async () => {
    const onChangeHost = vi.fn();
    const props = tabStripProps();
    const view = render(<SFTPTabStrip {...props} compact onChangeHost={onChangeHost} />);

    const host = screen.getByRole("button", { name: "Host" });
    expect(host).toHaveAttribute("data-value", "application-server");
    expect(host).toHaveAttribute("aria-haspopup", "dialog");
    await userEvent.click(host);
    expect(onChangeHost).toHaveBeenCalledExactlyOnceWith(pane.tabs[0]);

    view.rerender(<SFTPTabStrip {...props} compact onChangeHost={onChangeHost} canChangeHost={() => false} />);
    expect(host).toBeDisabled();
    expect(screen.getByRole("tab", { selected: true })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Close the application-server:config tab" })).toBeEnabled();
  });

  it("keeps keyboard selection and refuses to move or drag a guarded tab", async () => {
    const props = tabStripProps();
    render(<SFTPTabStrip {...props} movable={() => false} />);
    const remote = screen.getByRole("tab", { selected: true });

    remote.focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(props.onSelect).toHaveBeenCalledWith("local");
    remote.focus();
    await userEvent.keyboard("{Shift>}{ArrowRight}{/Shift}");
    expect(props.onMove).not.toHaveBeenCalled();

    const dataTransfer = { setData: vi.fn(), effectAllowed: "" };
    fireEvent.dragStart(remote, { dataTransfer });
    expect(dataTransfer.setData).not.toHaveBeenCalled();
    expect(props.onDragStart).not.toHaveBeenCalled();
  });
});
