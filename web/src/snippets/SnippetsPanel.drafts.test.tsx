import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi } from "./api";

vi.mock("./api", () => ({ snippetsApi: { library: vi.fn() } }));

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets: [
      { id: "one", name: "Check disk", command: "df -h", variables: [] },
      { id: "two", name: "Check memory", command: "free -h", variables: [] },
    ],
    startup: [],
  } as never);
});

it("asks before another snippet replaces an unsaved edit, and keeps the edit when editing continues", async () => {
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);
  await user.click(await screen.findByRole("button", { name: /Check disk/ }));
  await user.type(screen.getByRole("textbox", { name: "Command" }), " /var");

  await user.click(screen.getByRole("button", { name: /Check memory/ }));
  expect(screen.getByRole("dialog", { name: "Discard unsaved changes?" })).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Keep editing" }));

  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("df -h /var");

  await user.selectOptions(screen.getByRole("combobox", { name: "Snippets" }), "two");
  await user.click(screen.getByRole("button", { name: "Discard" }));

  expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("Check memory");
  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("free -h");
});

it("asks before a new snippet replaces what was typed for another new one", async () => {
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);
  await screen.findByRole("button", { name: /Check disk/ });
  await user.type(screen.getByRole("textbox", { name: "Name" }), "Restart web");

  await user.click(screen.getByRole("button", { name: "New" }));

  expect(screen.getByRole("dialog", { name: "Discard unsaved changes?" })).toBeVisible();
  expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("Restart web");
});

it("switches snippets without asking while nothing was changed", async () => {
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);
  await user.click(await screen.findByRole("button", { name: /Check disk/ }));

  await user.click(screen.getByRole("button", { name: /Check memory/ }));

  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("free -h");
});
