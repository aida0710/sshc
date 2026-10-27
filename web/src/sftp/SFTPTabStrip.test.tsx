import { createEvent, fireEvent, render, screen, within } from "@testing-library/react";
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

  it("moves the focused tab one place along with Ctrl+Shift and the arrow keys", async () => {
    const onReorder = vi.fn();
    render(<SFTPTabStrip {...tabStripProps()} onReorder={onReorder} />);
    const remote = screen.getByRole("tab", { selected: true });

    remote.focus();
    await userEvent.keyboard("{Control>}{Shift>}{ArrowRight}{/Shift}{/Control}");
    expect(onReorder).toHaveBeenCalledWith("remote", 1);
    await userEvent.keyboard("{Control>}{Shift>}{ArrowLeft}{/Shift}{/Control}");
    expect(onReorder).toHaveBeenCalledWith("remote", -1);
  });

  it("drops a dragged tab before the tab whose middle is right of the pointer, and marks the place", () => {
    const onDropAt = vi.fn();
    render(<SFTPTabStrip {...tabStripProps()} onDropAt={onDropAt} />);
    const tabs = Array.from(document.querySelectorAll<HTMLElement>("[data-sftp-tab]"));
    tabs.forEach((element, index) => {
      element.getBoundingClientRect = () => ({ left: index * 210, width: 208, top: 0, height: 30, right: index * 210 + 208, bottom: 30, x: index * 210, y: 0, toJSON: () => ({}) });
    });
    const list = screen.getByRole("tablist", { name: "File tabs" });
    const dataTransfer = { types: ["application/x-sshc-sftp-tab"], dropEffect: "" };
    // jsdom has no DragEvent, so the pointer's place is put on the event by hand.
    const dragAt = (type: "dragOver" | "drop", clientX: number) => {
      const event = createEvent[type](list, { dataTransfer });
      Object.defineProperty(event, "clientX", { value: clientX });
      fireEvent(list, event);
    };

    dragAt("dragOver", 300);
    expect(document.querySelectorAll("[data-sftp-tab-drop-marker]")).toHaveLength(1);
    dragAt("drop", 300);
    expect(onDropAt).toHaveBeenLastCalledWith(1);

    dragAt("drop", 500);
    expect(onDropAt).toHaveBeenLastCalledWith(2);
    expect(document.querySelectorAll("[data-sftp-tab-drop-marker]")).toHaveLength(0);
  });

  it("leaves files dragged from a list or the desktop to the pane under the strip", () => {
    const onDropAt = vi.fn();
    render(<SFTPTabStrip {...tabStripProps()} onDropAt={onDropAt} />);
    const list = screen.getByRole("tablist", { name: "File tabs" });

    fireEvent.drop(list, { dataTransfer: { types: ["Files"] }, clientX: 10 });

    expect(onDropAt).not.toHaveBeenCalled();
  });

  it("opens the selected tab's actions from its button and any tab's from a right-click", async () => {
    const onSelect = vi.fn();
    const moveLeft = vi.fn();
    const tabActions = vi.fn(() => [{ label: "Move left", onSelect: moveLeft }]);
    const props = { ...tabStripProps(), onSelect, tabActions };
    const view = render(<SFTPTabStrip {...props} />);

    await userEvent.click(screen.getByRole("button", { name: "Actions for the application-server:config tab" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Move left" }));
    expect(moveLeft).toHaveBeenCalledOnce();

    fireEvent.contextMenu(screen.getByRole("tab", { name: "Local:projects" }));
    expect(onSelect).toHaveBeenCalledWith("local");
    view.rerender(<SFTPTabStrip {...props} pane={{ ...pane, activeId: "local" }} />);
    expect(await screen.findByRole("menuitem", { name: "Move left" })).toBeInTheDocument();
  });
});
