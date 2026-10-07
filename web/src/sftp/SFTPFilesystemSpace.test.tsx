import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import { remoteMetadataApi } from "./remoteMetadataApi";
import { SFTPFilesystemSpace } from "./SFTPFilesystemSpace";

afterEach(() => vi.restoreAllMocks());
describe("remote filesystem capacity", () => {
  it("displays the exact available and total byte counts beyond JavaScript number precision", async () => {
    vi.spyOn(remoteMetadataApi, "filesystemSpace").mockResolvedValue({ path: "/srv", availableBytes: "18446744073709551615", totalBytes: "18446744073709551615" });
    render(<SFTPFilesystemSpace alias="edge" path="/srv" refreshKey={1} />);
    expect(await screen.findByRole("status")).toHaveTextContent("18,446,744,073,709,551,615 bytes");
  });
  it("explains an unsupported server without failing the list", async () => {
    vi.spyOn(remoteMetadataApi, "filesystemSpace").mockRejectedValue(new ApiError("sftp_unsupported_operation", 501, null));
    render(<><p>directory entries remain visible</p><SFTPFilesystemSpace alias="edge" path="/srv" refreshKey={1} /></>);
    expect(await screen.findByRole("status")).toHaveTextContent("This server does not support filesystem capacity reporting.");
    expect(screen.getByText("directory entries remain visible")).toBeInTheDocument();
  });
});
