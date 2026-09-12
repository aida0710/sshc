import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi } from "./api";

vi.mock("./api", () => ({ snippetsApi: { library: vi.fn() } }));

it("opens snippets from the compact library selector and starts a new draft", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets: [
      { id: "one", name: "Check disk", command: "df -h", variables: [] },
      { id: "two", name: "Check memory", command: "free -h", variables: [] },
    ], startup: [],
  } as never);
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);
  await screen.findByRole("option", { name: "Check memory" });
  const selector = screen.getByRole("combobox", { name: "Snippets" });
  await user.selectOptions(selector, "two");
  expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("Check memory");
  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("free -h");
  await user.selectOptions(selector, "one");
  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("df -h");
  await user.selectOptions(selector, "");
  expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("");
  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("");
});
