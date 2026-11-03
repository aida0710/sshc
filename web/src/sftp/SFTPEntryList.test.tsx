import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { RemoteEntry } from "./api";
import { SFTPEntryList } from "./SFTPEntryList";
import { useSFTPEntryList } from "./useSFTPEntryList";

const entries: RemoteEntry[] = [
  { name: "project", path: "/home/edge/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-09-01T07:00:00Z", revision: "project" },
  { name: "notes.txt", path: "/home/edge/notes.txt", type: "file", size: 12, mode: "0644", modifiedAt: "2026-09-01T07:00:00Z", revision: "notes" },
];

function LockedList({ mobileInteraction, locked = true, onActivate, onOpenParent }: {
  mobileInteraction: boolean;
  locked?: boolean;
  onActivate: (entry: RemoteEntry) => void;
  onOpenParent: () => void;
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
});
