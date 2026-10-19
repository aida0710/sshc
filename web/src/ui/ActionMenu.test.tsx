import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { ActionMenu } from "./ActionMenu";

function ControlledMenu({ onSelect }: { onSelect: () => void }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {/* 右クリックの代わり。呼び手が開閉を持つときに、ボタンの外から開けることを確かめる。 */}
      <button type="button" onClick={() => setOpen(true)}>Open from outside</button>
      <ActionMenu label="Actions for nas" items={[{ label: "Rename", onSelect }]} open={open} onOpenChange={setOpen} triggerClassName="" />
    </>
  );
}

describe("ActionMenu", () => {
  it("opens from its own button and runs the chosen action", async () => {
    const onSelect = vi.fn();
    render(<ActionMenu label="Actions for nas" items={[{ label: "Rename", onSelect }]} triggerClassName="" />);
    await userEvent.click(screen.getByRole("button", { name: "Actions for nas" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Rename" }));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
  });

  it("opens when the caller that holds its state asks, and returns focus to its button after an action", async () => {
    const onSelect = vi.fn();
    render(<ControlledMenu onSelect={onSelect} />);
    await userEvent.click(screen.getByRole("button", { name: "Open from outside" }));
    expect(screen.getByRole("menu")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("menuitem", { name: "Rename" }));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(screen.queryByRole("menu")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Actions for nas" })).toHaveFocus();
  });
});
