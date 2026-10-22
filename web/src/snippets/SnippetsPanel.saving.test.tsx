import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi, type Preview, type Snippet } from "./api";

vi.mock("./api", () => ({ snippetsApi: { library: vi.fn(), create: vi.fn(), preview: vi.fn() } }));

const savedAt = { createdAt: "2026-10-01T00:00:00Z", updatedAt: "2026-10-01T00:00:00Z" };
const checkDisk: Snippet = { id: "one", name: "Check disk", description: "", command: "df -h", variables: [], ...savedAt };
const restartWeb: Snippet = { id: "two", name: "Restart web", description: "", command: "systemctl restart web", variables: [], ...savedAt };

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockReset();
  vi.mocked(snippetsApi.create).mockReset();
  vi.mocked(snippetsApi.preview).mockReset();
});

it("says the library is loading instead of showing an empty list", async () => {
  let answer: () => void = () => undefined;
  vi.mocked(snippetsApi.library).mockImplementation(() => new Promise((resolve) => {
    answer = () => resolve({ snippets: [checkDisk], startup: [] });
  }));
  render(<SnippetsPanel aliases={["test-host"]} />);

  expect(screen.getByRole("status")).toHaveTextContent("Loading snippets");
  await act(async () => answer());
  expect(await screen.findByRole("option", { name: "Check disk" })).toBeInTheDocument();
  expect(screen.queryByText("Loading snippets…")).not.toBeInTheDocument();
});

it("shows the save working, then says the snippet was saved until the next edit", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({ snippets: [checkDisk], startup: [] });
  let answer: () => void = () => undefined;
  vi.mocked(snippetsApi.create).mockImplementation(() => new Promise((resolve) => {
    answer = () => {
      vi.mocked(snippetsApi.library).mockResolvedValue({ snippets: [checkDisk, restartWeb], startup: [] });
      resolve(restartWeb);
    };
  }));
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);
  await screen.findByRole("option", { name: "Check disk" });

  await user.type(screen.getByRole("textbox", { name: "Name" }), restartWeb.name);
  await user.type(screen.getByRole("textbox", { name: "Command" }), restartWeb.command);
  const save = screen.getByRole("button", { name: "Save" });
  await user.click(save);

  expect(save).toHaveAttribute("aria-busy", "true");
  expect(screen.queryByText("Snippet saved.")).not.toBeInTheDocument();
  await act(async () => answer());
  expect(await screen.findByText("Snippet saved.")).toBeInTheDocument();
  expect(save).not.toHaveAttribute("aria-busy");

  await user.type(screen.getByRole("textbox", { name: "Description" }), "x");
  expect(screen.queryByText("Snippet saved.")).not.toBeInTheDocument();
});

it("does not call a finished preview a save", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue({ snippets: [checkDisk], startup: [] });
  vi.mocked(snippetsApi.preview).mockResolvedValue({ targets: [] } as unknown as Preview);
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);

  await user.click(await screen.findByRole("button", { name: /Check disk/ }));
  await user.click(screen.getByRole("checkbox", { name: "test-host" }));
  await user.click(screen.getByRole("button", { name: "Preview execution" }));

  expect(await screen.findByText("Review exact commands")).toBeInTheDocument();
  expect(screen.queryByText("Snippet saved.")).not.toBeInTheDocument();
});
