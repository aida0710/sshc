import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SFTPCompareDialog } from "./SFTPCompareDialog";
import { sftpApi, type DirectoryComparison } from "./api";
import { sftpTransferManager } from "./transferManager";
import { localHostAlias } from "./localHost";

vi.mock("./api", () => ({ sftpApi: { compareDirectories: vi.fn() } }));
vi.mock("./transferManager", () => ({ sftpTransferManager: { addRemoteTransfers: vi.fn() } }));

const readme = {
  name: "readme.md",
  path: "/srv/docs/readme.md",
  type: "file",
  size: 12,
  mode: "0644",
  modifiedAt: "2026-09-30T00:00:00Z",
  revision: "r1",
} as const;

it("names each difference checkbox after the path it selects", async () => {
  const comparison: DirectoryComparison = {
    leftPath: "/srv",
    rightPath: "/srv",
    entries: [{ relativePath: "docs/readme.md", status: "left_only", left: readme }],
  };
  vi.mocked(sftpApi.compareDirectories).mockResolvedValue(comparison);

  render(<SFTPCompareDialog left={{ alias: "web", path: "/srv" }} right={{ alias: "db", path: "/srv" }} onDismiss={() => undefined} />);

  expect(await screen.findByRole("checkbox", { name: "Select docs/readme.md" })).toBeChecked();
});

it("shows local and remote differences without offering copy or synchronization", async () => {
  vi.mocked(sftpApi.compareDirectories).mockResolvedValue({
    leftPath: "C:/Users/alice/Documents",
    rightPath: "/srv",
    entries: [{ relativePath: "docs/readme.md", status: "left_only", left: readme }],
  });
  render(<SFTPCompareDialog left={{ alias: localHostAlias, path: "C:/Users/alice/Documents" }} right={{ alias: "edge", path: "/srv" }} onDismiss={() => undefined} />);

  expect(await screen.findByText("docs/readme.md")).toBeVisible();
  expect(screen.getByRole("columnheader", { name: "Local" })).toBeVisible();
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  expect(screen.queryByRole("button", { name: /Copy/ })).not.toBeInTheDocument();
  expect(sftpApi.compareDirectories).toHaveBeenCalledWith({ left: { alias: localHostAlias, path: "C:/Users/alice/Documents" }, right: { alias: "edge", path: "/srv" }, mode: "metadata", signal: expect.any(AbortSignal) });
  expect(sftpTransferManager.addRemoteTransfers).not.toHaveBeenCalled();
});

it("does not report no differences when a local comparison fails", async () => {
  vi.mocked(sftpApi.compareDirectories).mockRejectedValue(new Error("comparison unavailable"));
  render(<SFTPCompareDialog left={{ alias: localHostAlias, path: "/missing" }} right={{ alias: "edge", path: "/srv" }} onDismiss={() => undefined} />);

  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.queryByText("The two directories have matching metadata.")).not.toBeInTheDocument();
});

it("compares SHA256 after switching mode and aborts the retired metadata request", async () => {
  let finishMetadata!: (comparison: DirectoryComparison) => void;
  vi.mocked(sftpApi.compareDirectories)
    .mockImplementationOnce(() => new Promise((resolve) => { finishMetadata = resolve; }))
    .mockResolvedValueOnce({ leftPath: "/srv", rightPath: "/srv", mode: "content", bytesRead: 24, truncated: false, entries: [{ relativePath: "docs/readme.md", status: "different", left: readme, right: readme }] });
  const rendered = render(<SFTPCompareDialog left={{ alias: "web", path: "/srv" }} right={{ alias: "db", path: "/srv" }} onDismiss={() => undefined} />);
  const first = vi.mocked(sftpApi.compareDirectories).mock.calls.at(-1)?.[0];
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Compare mode" }), "content");
  expect(first?.signal?.aborted).toBe(true);
  expect(await screen.findByText("docs/readme.md")).toBeVisible();
  expect(sftpApi.compareDirectories).toHaveBeenLastCalledWith(expect.objectContaining({ mode: "content" }));
  await act(async () => finishMetadata({ leftPath: "/srv", rightPath: "/srv", entries: [] }));
  expect(screen.getByText("docs/readme.md")).toBeVisible();
  const active = vi.mocked(sftpApi.compareDirectories).mock.calls.at(-1)?.[0];
  rendered.unmount();
  expect(active?.signal?.aborted).toBe(true);
});

it("shows over-budget contents as unverified and excludes them from selected copies", async () => {
  vi.mocked(sftpApi.compareDirectories).mockResolvedValue({ leftPath: "/srv", rightPath: "/srv", entries: [{ relativePath: "docs/readme.md", status: "unverified", omission: "byte_limit", left: readme, right: readme }], mode: "content", bytesRead: 0, truncated: true });
  render(<SFTPCompareDialog left={{ alias: "web", path: "/srv" }} right={{ alias: "db", path: "/srv" }} onDismiss={() => undefined} />);
  await userEvent.selectOptions(screen.getByRole("combobox", { name: "Compare mode" }), "content");
  const selection = await screen.findByRole("checkbox", { name: "Select docs/readme.md" });
  expect(selection).not.toBeChecked();
  expect(selection).toBeDisabled();
  expect(screen.getByText("256 MiB read limit")).toBeVisible();
  await waitFor(() => expect(screen.getByRole("button", { name: "Copy left → right" })).toBeDisabled());
  expect(screen.queryByText("The two directories have matching file contents.")).not.toBeInTheDocument();
});
