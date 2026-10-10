import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { TransferExclusionSettings } from "./TransferExclusionSettings";

it("validates the rules before saving and retains the draft until the saved settings arrive", async () => {
  const onCommit = vi.fn();
  const { rerender } = render(<TransferExclusionSettings patterns={[]} onCommit={onCommit} />);
  await userEvent.click(screen.getByText("Transfer exclusions"));
  const input = screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" });
  await userEvent.type(input, ".git\n*.log\n../secret");
  expect(screen.getByRole("button", { name: "Save exclusions" })).toBeDisabled();
  await userEvent.clear(input);
  await userEvent.type(input, ".git\n*.log");
  await userEvent.click(screen.getByRole("button", { name: "Save exclusions" }));
  expect(onCommit).toHaveBeenCalledWith({ excludePatterns: [".git", "*.log"] });
  expect(input).toHaveValue(".git\n*.log");
  rerender(<TransferExclusionSettings patterns={[".git", "*.log"]} onCommit={onCommit} />);
  expect(screen.getByRole("button", { name: "Save exclusions" })).toBeDisabled();
  expect(screen.getByText("2 exclusion rules")).toBeInTheDocument();
  rerender(<TransferExclusionSettings patterns={["node_modules"]} onCommit={onCommit} />);
  expect(input).toHaveValue("node_modules");
});
