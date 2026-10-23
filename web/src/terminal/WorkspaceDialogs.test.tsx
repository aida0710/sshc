import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { WorkspaceNameDialog } from "./WorkspaceDialogs";

it.each([
  { isComposing: true },
  { isComposing: false, keyCode: 229 },
])("keeps workspace naming open on composition Escape (%j)", async (composition) => {
  const onCancel = vi.fn();
  const onSubmit = vi.fn();
  render(<WorkspaceNameDialog heading="Rename workspace" submitLabel="Save" initialName="Development"
    allowEmpty={false} onSubmit={onSubmit} onCancel={onCancel} />);
  const input = screen.getByRole("textbox", { name: "Workspace name" });
  fireEvent.change(input, { target: { value: "開発" } });
  expect(fireEvent.keyDown(input, { key: "Escape", ...composition })).toBe(true);
  expect(input).toHaveFocus();
  expect(input).toHaveValue("開発");
  expect(onCancel).not.toHaveBeenCalled();
  expect(onSubmit).not.toHaveBeenCalled();

  await userEvent.keyboard("{Escape}");
  expect(onCancel).toHaveBeenCalledOnce();
});
