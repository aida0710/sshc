import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { SFTPCompareDialog } from "./SFTPCompareDialog";
import { sftpApi, type DirectoryComparison } from "./api";

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
