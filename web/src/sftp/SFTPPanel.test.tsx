import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SFTPPanel } from "./SFTPPanel";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  upload: vi.fn(),
}));

vi.mock("./api", () => ({ sftpApi: api }));

describe("SFTPPanel uploads", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ path: "/remote", entries: [] });
  });

  it("uploads every dropped file separately and keeps per-file results", async () => {
    api.upload.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error("upload_failed"));
    render(<SFTPPanel aliases={["edge"]} />);
    await waitFor(() => expect(api.list).toHaveBeenCalled());

    const first = new File(["alpha"], "first.txt", { type: "text/plain" });
    const second = new File(["beta"], "second.txt", { type: "text/plain" });
    fireEvent.drop(screen.getByLabelText("Upload files to the current remote directory"), {
      dataTransfer: { files: [first, second] },
    });

    await waitFor(() => expect(api.upload).toHaveBeenCalledTimes(2));
    expect(api.upload).toHaveBeenNthCalledWith(1, "edge", "/remote/first.txt", first, false);
    expect(api.upload).toHaveBeenNthCalledWith(2, "edge", "/remote/second.txt", second, false);
    expect(await screen.findByText("Uploaded")).toBeInTheDocument();
    expect(await screen.findByText("upload_failed")).toBeInTheDocument();
  });
});
