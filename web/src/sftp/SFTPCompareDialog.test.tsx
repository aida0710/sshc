import { render, screen } from "@testing-library/react";
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
  expect(sftpApi.compareDirectories).toHaveBeenCalledWith(localHostAlias, "C:/Users/alice/Documents", "edge", "/srv");
  expect(sftpTransferManager.addRemoteTransfers).not.toHaveBeenCalled();
});

it("does not report no differences when a local comparison fails", async () => {
  vi.mocked(sftpApi.compareDirectories).mockRejectedValue(new Error("comparison unavailable"));
  render(<SFTPCompareDialog left={{ alias: localHostAlias, path: "/missing" }} right={{ alias: "edge", path: "/srv" }} onDismiss={() => undefined} />);

  expect(await screen.findByRole("alert")).toBeVisible();
  expect(screen.queryByText("The two directories have matching metadata.")).not.toBeInTheDocument();
});
