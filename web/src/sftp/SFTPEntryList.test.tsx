import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { RemoteEntry } from "./api";
import { SFTPEntryList, useSFTPEntryList } from "./SFTPEntryList";

const entries: RemoteEntry[] = [
  { name: "project", path: "/home/edge/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-09-01T07:00:00Z", revision: "project" },
  { name: "notes.txt", path: "/home/edge/notes.txt", type: "file", size: 12, mode: "0644", modifiedAt: "2026-09-01T07:00:00Z", revision: "notes" },
];

function LockedList({ mobileInteraction, onActivate, onOpenParent }: {
  mobileInteraction: boolean;
  onActivate: (entry: RemoteEntry) => void;
  onOpenParent: () => void;
}) {
  const model = useSFTPEntryList({
    entries,
    loadedEntries: entries,
    parentRowVisible: true,
    busy: false,
    locked: true,
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
        locked
        parentRowVisible
      />
    </div>
  );
}

describe("SFTPEntryList", () => {
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

  it("adds a tapped row to the selection of a locked touch list but does not open it", async () => {
    const onActivate = vi.fn();
    const onOpenParent = vi.fn();
    render(<LockedList mobileInteraction onActivate={onActivate} onOpenParent={onOpenParent} />);

    const project = screen.getByRole("button", { name: "project" });
    await userEvent.click(project);
    expect(project).toHaveAttribute("aria-pressed", "false");

    await userEvent.click(screen.getByRole("checkbox", { name: "Select notes.txt" }));
    await userEvent.click(project);
    expect(screen.getByRole("button", { name: "notes.txt" })).toHaveAttribute("aria-pressed", "true");
    expect(project).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Parent directory" })).toBeDisabled();
    expect(onActivate).not.toHaveBeenCalled();
    expect(onOpenParent).not.toHaveBeenCalled();
  });
});
