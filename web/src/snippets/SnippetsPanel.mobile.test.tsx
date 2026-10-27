import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi } from "./api";

vi.mock("./api", () => ({ snippetsApi: { library: vi.fn() } }));

// A phone shows the list or the opened snippet, one at a time, as Connections does.
beforeEach(() => {
  vi.stubGlobal("matchMedia", vi.fn((query: string) => ({
    matches: true,
    media: query,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  })));
});

it("opens a snippet from the list on a phone and goes back to the list", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets: [
      { id: "one", name: "Check disk", command: "df -h", variables: [] },
      { id: "two", name: "Check memory", command: "free -h", variables: [] },
    ], startup: [],
  } as never);
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);

  await user.click(await screen.findByRole("button", { name: /Check memory/ }));
  expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("Check memory");
  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("free -h");
  // The heading row with Add snippet makes way for the opened snippet.
  expect(screen.queryByRole("button", { name: "Add snippet" })).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "All snippets" }));
  expect(screen.queryByRole("textbox", { name: "Name" })).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Add snippet" }));
  expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("");
  expect(screen.getByRole("textbox", { name: "Command" })).toHaveValue("");
});

it("offers to add the first snippet when none is saved", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({ snippets: [], startup: [] });
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);

  expect(await screen.findByText("No snippets yet")).toBeInTheDocument();
  await user.click(screen.getAllByRole("button", { name: "Add snippet" })[0]!);
  expect(screen.getByRole("textbox", { name: "Name" })).toHaveValue("");
});

it("stays on the list after going back from the snippet the address names", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({
    snippets: [{ id: "two", name: "Check memory", command: "free -h", variables: [] }], startup: [],
  } as never);
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} selectedSnippetId="two" />);
  expect(await screen.findByRole("textbox", { name: "Name" })).toHaveValue("Check memory");

  await user.click(screen.getByRole("button", { name: "All snippets" }));

  expect(screen.queryByRole("textbox", { name: "Name" })).not.toBeInTheDocument();
});
