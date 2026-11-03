import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { RemoteEntry } from "./api";
import { SFTPEntryList } from "./SFTPEntryList";
import type { SFTPEntryView } from "./sftpPanes";
import { useSFTPEntryList } from "./useSFTPEntryList";

const entries: RemoteEntry[] = [
  { name: "project", path: "/home/edge/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-09-01T07:00:00Z", revision: "project" },
  { name: "notes.txt", path: "/home/edge/notes.txt", type: "file", size: 12, mode: "0644", modifiedAt: "2026-09-01T07:00:00Z", revision: "notes" },
];

function LockedList({ mobileInteraction, locked = true, view = "list", draggable, onActivate, onOpenParent, onContextMenu }: {
  mobileInteraction: boolean;
  locked?: boolean;
  view?: SFTPEntryView;
  draggable?: (entry: RemoteEntry) => boolean;
  onActivate: (entry: RemoteEntry) => void;
  onOpenParent: () => void;
  onContextMenu?: (entry: RemoteEntry, x: number, y: number) => void;
}) {
  const model = useSFTPEntryList({
    entries,
    loadedEntries: entries,
    parentRowVisible: !mobileInteraction,
    busy: false,
    locked,
    mobileInteraction,
    onActivate,
    onOpenParent,
    ...(onContextMenu === undefined ? {} : { onContextMenu }),
  });
  return (
    <div onKeyDown={model.handleListKeys}>
      <SFTPEntryList
        model={model}
        entries={entries}
        sort={{ key: "name", direction: "ascending" }}
        onSort={() => undefined}
        mobileInteraction={mobileInteraction}
        busy={false}
        locked={locked}
        parentRowVisible={!mobileInteraction}
        view={view}
        {...(draggable === undefined ? {} : { draggable })}
      />
    </div>
  );
}

// holdRow keeps a finger on a row until the long press has been taken, then lifts it.
async function holdRow(row: HTMLElement, chosen: string) {
  fireEvent.pointerDown(row, { pointerId: 1, pointerType: "touch", clientX: 10, clientY: 10 });
  const checkbox = await screen.findByRole("checkbox", { name: chosen }, { timeout: 2_000 });
  fireEvent.pointerUp(row, { pointerId: 1, pointerType: "touch", clientX: 10, clientY: 10 });
  // The browser's click at the end of the press.
  fireEvent.click(row);
  return checkbox;
}

describe("SFTPEntryList", () => {
  it("shows a touch row's size and date under its name, leaving the permissions to the details", () => {
    render(<LockedList mobileInteraction onActivate={vi.fn()} onOpenParent={vi.fn()} />);

    const notes = screen.getByRole("button", { name: "notes.txt" });
    expect(notes).toHaveTextContent(/12 B · /u);
    expect(notes).not.toHaveTextContent("0644");
  });

  it("names a file's kind in the wide table's type column", () => {
    render(<LockedList mobileInteraction={false} onActivate={vi.fn()} onOpenParent={vi.fn()} />);

    expect(screen.getByRole("row", { name: /notes\.txt/u })).toHaveTextContent("Text");
    expect(screen.getByRole("row", { name: /project/u })).toHaveTextContent("Folder");
  });

  it("selects a clicked row of a locked table but opens neither the row nor the parent", async () => {
    const onActivate = vi.fn();
    const onOpenParent = vi.fn();
    render(<LockedList mobileInteraction={false} onActivate={onActivate} onOpenParent={onOpenParent} />);

    const project = screen.getByRole("button", { name: "project" });
    await userEvent.click(project);
    expect(project).toHaveAttribute("aria-pressed", "true");

    await userEvent.dblClick(project);
    await userEvent.keyboard("{Enter}");
    expect(screen.getByRole("button", { name: "Parent directory" })).toBeDisabled();
    expect(onActivate).not.toHaveBeenCalled();
    expect(onOpenParent).not.toHaveBeenCalled();
  });

  it("has no checkboxes on a phone until a long press chooses a row, and a tap then chooses instead of opening", async () => {
    const onActivate = vi.fn();
    render(<LockedList mobileInteraction locked={false} onActivate={onActivate} onOpenParent={vi.fn()} />);
    expect(screen.queryByRole("checkbox")).toBeNull();

    const checkbox = await holdRow(screen.getByRole("button", { name: "notes.txt" }), "Select notes.txt");
    expect(checkbox).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Select project" })).not.toBeChecked();
    expect(onActivate).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("button", { name: "project" }));
    expect(screen.getByRole("checkbox", { name: "Select project" })).toBeChecked();
    expect(onActivate).not.toHaveBeenCalled();

    await userEvent.click(screen.getByRole("checkbox", { name: "Select project" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Select notes.txt" }));
    expect(screen.queryByRole("checkbox")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "notes.txt" }));
    expect(onActivate).toHaveBeenCalledOnce();
  });

  it("keeps a row chosen when Android also reports the long press as a context menu", async () => {
    render(<LockedList mobileInteraction locked={false} onActivate={vi.fn()} onOpenParent={vi.fn()} />);
    const notes = screen.getByRole("button", { name: "notes.txt" });

    fireEvent.contextMenu(notes);
    fireEvent.contextMenu(notes);

    expect(await holdRow(notes, "Select notes.txt")).toBeChecked();
  });

  it("does not take a row for held when the finger moves away to scroll", async () => {
    render(<LockedList mobileInteraction locked={false} onActivate={vi.fn()} onOpenParent={vi.fn()} />);
    const notes = screen.getByRole("button", { name: "notes.txt" });

    fireEvent.pointerDown(notes, { pointerId: 1, pointerType: "touch", clientX: 10, clientY: 10 });
    fireEvent.pointerMove(notes, { pointerId: 1, pointerType: "touch", clientX: 10, clientY: 60 });
    await new Promise((resolve) => { setTimeout(resolve, 600); });

    expect(screen.queryByRole("checkbox")).toBeNull();
  });

  it("adds a tapped row to the selection of a locked touch list but does not open it", async () => {
    const onActivate = vi.fn();
    render(<LockedList mobileInteraction onActivate={onActivate} onOpenParent={vi.fn()} />);

    const project = screen.getByRole("button", { name: "project" });
    await userEvent.click(project);
    expect(project).toHaveAttribute("aria-pressed", "false");

    await holdRow(screen.getByRole("button", { name: "notes.txt" }), "Select notes.txt");
    await userEvent.click(project);
    expect(screen.getByRole("button", { name: "notes.txt" })).toHaveAttribute("aria-pressed", "true");
    expect(project).toHaveAttribute("aria-pressed", "true");
    expect(onActivate).not.toHaveBeenCalled();
  });

  describe("as a grid", () => {
    it("opens a cell with a tap on a phone, and a long press starts choosing", async () => {
      const onActivate = vi.fn();
      render(<LockedList mobileInteraction locked={false} view="grid" onActivate={onActivate} onOpenParent={vi.fn()} />);
      const notes = screen.getByRole("button", { name: "notes.txt" });
      expect(screen.queryByRole("checkbox")).toBeNull();

      await holdRow(notes, "Select notes.txt");
      expect(onActivate).not.toHaveBeenCalled();
      expect(notes).toHaveAttribute("aria-pressed", "true");

      // While rows are chosen, a tap chooses another rather than opening it.
      await userEvent.click(screen.getByRole("button", { name: "project" }));
      expect(onActivate).not.toHaveBeenCalled();
      expect(screen.getByRole("button", { name: "project" })).toHaveAttribute("aria-pressed", "true");
    });

    it("can be dragged out on a pointer's screen, and leaves a held finger to the long press on a phone", () => {
      const pointer = render(<LockedList mobileInteraction={false} locked={false} view="grid" draggable={() => true} onActivate={vi.fn()} onOpenParent={vi.fn()} />);
      expect(screen.getByRole("button", { name: "notes.txt" }).closest("li")).toHaveAttribute("draggable", "true");
      pointer.unmount();
      render(<LockedList mobileInteraction locked={false} view="grid" draggable={() => true} onActivate={vi.fn()} onOpenParent={vi.fn()} />);
      expect(screen.getByRole("button", { name: "notes.txt" }).closest("li")).toHaveAttribute("draggable", "false");
    });

    it("selects a cell with a click and opens it with a double click on a pointer's screen", async () => {
      const onActivate = vi.fn();
      render(<LockedList mobileInteraction={false} locked={false} view="grid" onActivate={onActivate} onOpenParent={vi.fn()} />);
      const notes = screen.getByRole("button", { name: "notes.txt" });
      await userEvent.click(notes);
      expect(notes).toHaveAttribute("aria-pressed", "true");
      expect(onActivate).not.toHaveBeenCalled();
      await userEvent.dblClick(notes);
      expect(onActivate).toHaveBeenCalledOnce();
    });

    it("gives a right click the row's menu", () => {
      const onContextMenu = vi.fn();
      render(<LockedList mobileInteraction={false} locked={false} view="grid" onActivate={vi.fn()} onOpenParent={vi.fn()} onContextMenu={onContextMenu} />);
      fireEvent.contextMenu(screen.getByRole("button", { name: "notes.txt" }), { clientX: 30, clientY: 40 });
      expect(onContextMenu).toHaveBeenCalledWith(expect.objectContaining({ name: "notes.txt" }), 30, 40);
    });

    it("moves with the arrow keys and opens with Enter", async () => {
      const onActivate = vi.fn();
      render(<LockedList mobileInteraction={false} locked={false} view="grid" onActivate={onActivate} onOpenParent={vi.fn()} />);
      const project = screen.getByRole("button", { name: "project" });
      project.focus();
      await userEvent.keyboard("{ArrowRight}");
      const notes = screen.getByRole("button", { name: "notes.txt" });
      expect(notes).toHaveFocus();
      expect(notes).toHaveAttribute("aria-pressed", "true");
      await userEvent.keyboard("{ArrowLeft}");
      expect(project).toHaveFocus();
      await userEvent.keyboard("{Enter}");
      expect(onActivate).toHaveBeenCalledWith(expect.objectContaining({ name: "project" }));
    });

    it("keeps the parent as the first cell, and it goes up", async () => {
      const onOpenParent = vi.fn();
      render(<LockedList mobileInteraction={false} locked={false} view="grid" onActivate={vi.fn()} onOpenParent={onOpenParent} />);
      await userEvent.click(screen.getByRole("button", { name: "Parent directory" }));
      expect(onOpenParent).toHaveBeenCalledOnce();
    });
  });
});
