import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FolderPane } from "./FolderPane";
import type { FolderRow } from "./organizer";

const rows: FolderRow[] = [
  { folder: { kind: "all" }, count: 3, depth: 0 },
  { folder: { kind: "group", name: "work" }, count: 2, depth: 1 },
  { folder: { kind: "group", name: "work/ci" }, count: 0, depth: 2 },
  { folder: { kind: "ungrouped" }, count: 1, depth: 0 },
];

function paint(overrides: Partial<Parameters<typeof FolderPane>[0]> = {}) {
  const onSelect = vi.fn();
  const onDropInto = vi.fn();
  render(
    <FolderPane
      rows={rows}
      selected={{ kind: "all" }}
      dragging={false}
      onSelect={onSelect}
      onDropInto={onDropInto}
      {...overrides}
    />,
  );
  return { onSelect, onDropInto };
}

describe("FolderPane", () => {
  it("lists every folder with what is directly inside it", () => {
    paint();

    expect(screen.getByRole("button", { name: "All keys, 3" })).toHaveTextContent("3");
    expect(screen.getByRole("button", { name: "work/ci, 0" })).toHaveTextContent("0");
    expect(screen.getByRole("button", { name: "No group, 1" })).toHaveTextContent("1");
  });

  it("marks the folder being shown", () => {
    paint({ selected: { kind: "group", name: "work" } });

    expect(screen.getByRole("button", { name: "work, 2" })).toHaveAttribute("aria-current", "true");
    expect(screen.getByRole("button", { name: "All keys, 3" })).toHaveAttribute("aria-current", "false");
  });

  it("opens a folder when it is chosen", async () => {
    const user = userEvent.setup();
    const { onSelect } = paint();

    await user.click(screen.getByRole("button", { name: "work, 2" }));

    expect(onSelect).toHaveBeenCalledWith({ kind: "group", name: "work" });
  });

  it("takes keys dropped onto a folder", () => {
    const { onDropInto } = paint({ dragging: true });

    fireEvent.drop(screen.getByRole("button", { name: "work/ci, 0" }));

    expect(onDropInto).toHaveBeenCalledWith({ kind: "group", name: "work/ci" });
  });

  it("takes keys dropped onto the folder for keys that belong to none", () => {
    const { onDropInto } = paint({ dragging: true });

    fireEvent.drop(screen.getByRole("button", { name: "No group, 1" }));

    expect(onDropInto).toHaveBeenCalledWith({ kind: "ungrouped" });
  });

  // **「すべて」は置き場ではない。** あれは絞り込みを外すことであって、
  // ~/.ssh の中の実在の場所ではない。放れてしまうと、利用者は鍵をどこかへ
  // 移したつもりになるが、移った先は無い。
  it("refuses a key dropped onto all keys, which is a filter and not a place", () => {
    const { onDropInto } = paint({ dragging: true });

    fireEvent.drop(screen.getByRole("button", { name: "All keys, 3" }));

    expect(onDropInto).not.toHaveBeenCalled();
  });
});
