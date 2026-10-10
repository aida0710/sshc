import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { remoteMetadataApi } from "./remoteMetadataApi";
import { SFTPFilesystemSpace } from "./SFTPFilesystemSpace";

afterEach(() => vi.restoreAllMocks());
describe("remote filesystem capacity", () => {
  it("shows available and total capacity in a unit that fits their size", async () => {
    vi.spyOn(remoteMetadataApi, "filesystemSpace").mockResolvedValue({ path: "/srv", availableBytes: "23322673152", totalBytes: "41267757056" });
    render(<SFTPFilesystemSpace alias="edge" path="/srv" refreshKey={1} />);
    expect(await screen.findByRole("status")).toHaveTextContent("Available: 21.7 GiB / Total: 38.4 GiB");
  });
  it("shows the largest 64-bit count the server may report without overflowing the units", async () => {
    vi.spyOn(remoteMetadataApi, "filesystemSpace").mockResolvedValue({ path: "/srv", availableBytes: "18446744073709551615", totalBytes: "18446744073709551615" });
    render(<SFTPFilesystemSpace alias="edge" path="/srv" refreshKey={1} />);
    expect(await screen.findByRole("status")).toHaveTextContent("Available: 16.0 EiB / Total: 16.0 EiB");
  });
  it("explains an unsupported server without failing the list", async () => {
    vi.spyOn(remoteMetadataApi, "filesystemSpace").mockRejectedValue(new ApiError("sftp_unsupported_operation", 501, null));
    render(<><p>directory entries remain visible</p><SFTPFilesystemSpace alias="edge" path="/srv" refreshKey={1} /></>);
    expect(await screen.findByRole("status")).toHaveTextContent("This server does not support filesystem capacity reporting.");
    expect(screen.getByText("directory entries remain visible")).toBeInTheDocument();
  });
});
