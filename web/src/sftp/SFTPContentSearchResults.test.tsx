import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SFTPContentSearchResults } from "./SFTPContentSearchResults";
import { notesEntry } from "../testing/sftpTextFiles";

it("opens the selected path and line and explains partial results", async () => {
  const match = { entry: notesEntry, line: 12, snippet: "a short matching line" };
  const onOpen = vi.fn();
  render(<SFTPContentSearchResults search={{ path: "/remote", query: "matching", entries: [], matches: [match], truncated: true, bytesRead: 100, omissions: [{ reason: "binary", count: 2 }] }} disabled={false} onOpen={onOpen} />);
  expect(screen.getByText("a short matching line")).toBeVisible();
  expect(screen.getByText("Binary or non-UTF-8 files skipped: 2")).toBeVisible();
  await userEvent.click(screen.getByRole("button", { name: "Open /remote/notes.txt at line 12" }));
  expect(onOpen).toHaveBeenCalledWith(match);
});

it("does not claim the whole tree has no matches when files were omitted", () => {
  render(<SFTPContentSearchResults search={{ path: "/remote", query: "missing", entries: [], matches: [], truncated: true, omissions: [{ reason: "byte_limit", count: 1 }] }} disabled={false} onOpen={() => undefined} />);
  expect(screen.getByText("No matching text was found in the files that could be searched.")).toBeVisible();
  expect(screen.queryByText("No matching text was found.")).not.toBeInTheDocument();
});
