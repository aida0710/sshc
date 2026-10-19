import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { GroupDialogs, type GroupDialogState } from "./GroupDialogs";

const groupNames = ["production", "production/web", "staging"];

function open(dialog: GroupDialogState) {
  const handlers = { onCreate: vi.fn(), onRename: vi.fn(), onDelete: vi.fn(), onClose: vi.fn() };
  render(<GroupDialogs dialog={dialog} groupNames={groupNames} {...handlers} />);
  return handlers;
}

describe("GroupDialogs", () => {
  it("creates a group inside the one whose menu opened the dialog", async () => {
    const { onCreate } = open({ kind: "create", parent: "production" });
    await userEvent.type(screen.getByLabelText("Name"), "db");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(onCreate).toHaveBeenCalledWith("production/db");
  });

  it("asks for one level of the name and refuses a slash", async () => {
    const { onCreate } = open({ kind: "create", parent: "" });
    await userEvent.type(screen.getByLabelText("Name"), "work/eu");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toBeInTheDocument();
  });

  it("refuses a name another group already has, whatever its case", async () => {
    const { onCreate } = open({ kind: "create", parent: "" });
    await userEvent.type(screen.getByLabelText("Name"), "Staging");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(onCreate).not.toHaveBeenCalled();
  });

  it("renames only the last level and keeps the parent", async () => {
    const { onRename } = open({ kind: "rename", name: "production/web" });
    const input = screen.getByLabelText("Name");
    expect(input).toHaveValue("web");
    await userEvent.clear(input);
    await userEvent.type(input, "frontend");
    await userEvent.click(screen.getByRole("button", { name: "Rename" }));
    expect(onRename).toHaveBeenCalledWith("production/web", "production/frontend");
  });

  it("closes without renaming when the name did not change", async () => {
    const { onRename, onClose } = open({ kind: "rename", name: "staging" });
    await userEvent.click(screen.getByRole("button", { name: "Rename" }));
    expect(onRename).not.toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it("moves the members of a deleted group to the chosen group, never into its own subtree", async () => {
    const { onDelete } = open({ kind: "delete", name: "production", memberCount: 4 });
    const destination = screen.getByRole("combobox");
    expect(screen.queryByRole("option", { name: "production/web" })).not.toBeInTheDocument();
    await userEvent.selectOptions(destination, "staging");
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onDelete).toHaveBeenCalledWith("production", "staging");
  });

  it("deletes an empty group without asking where its members go", async () => {
    const { onDelete } = open({ kind: "delete", name: "staging", memberCount: 0 });
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Delete" }));
    expect(onDelete).toHaveBeenCalledWith("staging", "");
  });
});
