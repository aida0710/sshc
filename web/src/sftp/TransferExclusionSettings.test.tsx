import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { TransferExclusionSettings } from "./TransferExclusionSettings";

it("saves the rules when the box is left only if every one is valid, and keeps the draft until they arrive", async () => {
  const onCommit = vi.fn();
  const { rerender } = render(<TransferExclusionSettings patterns={[]} onCommit={onCommit} />);
  const input = screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" });
  await userEvent.type(input, ".git\n*.log\n../secret");
  await userEvent.tab();
  expect(onCommit).not.toHaveBeenCalled();
  expect(input).toHaveAccessibleDescription(expect.stringContaining("Check the patterns"));

  await userEvent.clear(input);
  await userEvent.type(input, ".git\n*.log");
  await userEvent.tab();
  expect(onCommit).toHaveBeenCalledWith({ excludePatterns: [".git", "*.log"] });
  expect(input).toHaveValue(".git\n*.log");

  rerender(<TransferExclusionSettings patterns={[".git", "*.log"]} onCommit={onCommit} />);
  await userEvent.click(input);
  await userEvent.tab();
  expect(onCommit).toHaveBeenCalledTimes(1);
  rerender(<TransferExclusionSettings patterns={["node_modules"]} onCommit={onCommit} />);
  expect(input).toHaveValue("node_modules");
});
