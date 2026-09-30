import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { SnippetsPanel } from "./SnippetsPanel";
import { snippetsApi } from "./api";

vi.mock("./api", () => ({ snippetsApi: { library: vi.fn(), create: vi.fn() } }));

const library = {
  snippets: [{ id: "one", name: "Check disk", command: "df -h", variables: [] }],
  startup: [],
} as never;

beforeEach(() => {
  vi.mocked(snippetsApi.library).mockReset();
  vi.mocked(snippetsApi.create).mockReset();
});

it("says the library could not be read instead of calling it empty, and reads it again on request", async () => {
  vi.mocked(snippetsApi.library)
    .mockRejectedValueOnce(new ApiError("snippet_failed", 500, null))
    .mockResolvedValue(library);
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);

  expect(await screen.findByText("Snippets could not be read.")).toBeInTheDocument();
  expect(screen.queryByText("No snippets have been saved.")).not.toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Try again" }));
  expect(await screen.findByRole("option", { name: "Check disk" })).toBeInTheDocument();
  expect(screen.queryByText("Snippets could not be read.")).not.toBeInTheDocument();
});

it("explains a refused save in words instead of showing the failure code", async () => {
  vi.mocked(snippetsApi.library).mockResolvedValue(library);
  vi.mocked(snippetsApi.create).mockRejectedValue(new ApiError("invalid_snippet", 400, null));
  const user = userEvent.setup();
  render(<SnippetsPanel aliases={["test-host"]} />);
  await screen.findByRole("option", { name: "Check disk" });

  await user.type(screen.getByRole("textbox", { name: "Name" }), "Broken");
  await user.type(screen.getByRole("textbox", { name: "Command" }), "echo {{");
  await user.click(screen.getByRole("button", { name: "Save" }));

  expect(await screen.findByRole("alert")).toHaveTextContent("The snippet or its variables have values that cannot be used.");
  expect(screen.queryByText("invalid_snippet")).not.toBeInTheDocument();
});
