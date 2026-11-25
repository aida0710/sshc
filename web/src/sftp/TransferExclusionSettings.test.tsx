import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { discardSettings, renderSettings, SettingsWithBar } from "../testing/settingsDraft";
import { fakeSettingsServer } from "../testing/settingsServer";
import { TransferExclusionSettings } from "./TransferExclusionSettings";

it("hands on the rules as they are typed, only while every one is valid, and keeps the text", async () => {
  fakeSettingsServer();
  const onChange = vi.fn();
  const { rerender } = renderSettings(<TransferExclusionSettings patterns={[]} onChange={onChange} />);
  // The field is a plain text area until its editor has loaded; this test holds on to the loaded one.
  await act(() => new Promise((resolve) => setTimeout(resolve, 0)));
  const input = screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" });
  await userEvent.type(input, ".git\n../secret");
  expect(onChange).toHaveBeenLastCalledWith([".git"]);
  expect(input).toHaveAccessibleDescription(expect.stringContaining("Check the patterns"));

  await userEvent.clear(input);
  await userEvent.type(input, ".git\n*.log\n");
  expect(onChange).toHaveBeenLastCalledWith([".git", "*.log"]);
  // The empty last line stays while the box is being typed in.
  rerender(<SettingsWithBar><TransferExclusionSettings patterns={[".git", "*.log"]} onChange={onChange} /></SettingsWithBar>);
  expect(input).toHaveValue(".git\n*.log\n");

  await userEvent.tab();
  expect(input).toHaveValue(".git\n*.log");
});

it("goes back to the saved rules when the changes are discarded", async () => {
  fakeSettingsServer();
  renderSettings(<TransferExclusionSettings patterns={["node_modules"]} onChange={vi.fn()} />);
  await act(() => new Promise((resolve) => setTimeout(resolve, 0)));
  const input = screen.getByRole("textbox", { name: "Exclusion patterns (one per line)" });
  await userEvent.type(input, "\n../secret");
  await discardSettings();
  expect(input).toHaveValue("node_modules");
});
