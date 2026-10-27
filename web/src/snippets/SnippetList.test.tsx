import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SnippetList } from "./SnippetList";

const savedAt = { createdAt: "2026-10-01T00:00:00Z", updatedAt: "2026-10-01T00:00:00Z" };
const snippets = [
  { id: "one", name: "Check disk", description: "", command: "df -h", variables: [], ...savedAt },
  { id: "two", name: "GPU", description: "nvidia-smi output", command: "nvidia-smi\nnvidia-smi -L", variables: [], ...savedAt },
];

it("narrows the list by name, description or command, and says when nothing matches", async () => {
  const user = userEvent.setup();
  render(<SnippetList libraryState="ready" snippets={snippets} openedId={null} empty={null} onRetry={vi.fn()} onOpen={vi.fn()} />);
  const filter = screen.getByRole("searchbox", { name: "Filter by name or command" });

  await user.type(filter, "DF");
  expect(screen.getByRole("button", { name: /Check disk/ })).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /GPU/ })).not.toBeInTheDocument();

  await user.clear(filter);
  await user.type(filter, "output");
  expect(screen.getByRole("button", { name: /GPU/ })).toBeInTheDocument();

  await user.clear(filter);
  await user.type(filter, "reboot");
  expect(screen.getByRole("status")).toHaveTextContent("No snippet matches.");
});

it("shows only the first line of a command under the name", () => {
  render(<SnippetList libraryState="ready" snippets={snippets} openedId="two" empty={null} onRetry={vi.fn()} onOpen={vi.fn()} />);

  const opened = screen.getByRole("button", { name: /GPU/ });
  expect(opened).toHaveAttribute("aria-current", "true");
  expect(opened).toHaveTextContent(/^GPUnvidia-smi$/);
});
