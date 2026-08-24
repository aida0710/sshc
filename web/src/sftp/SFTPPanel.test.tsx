import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { SFTPPanel } from "./SFTPPanel";
import { ApiError } from "../api/client";
import { sftpTransferQueue } from "./transferQueue";
import { sftpDownloadQueue } from "./downloadQueue";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  upload: vi.fn(),
  mkdir: vi.fn(),
  chmod: vi.fn(),
  download: vi.fn(),
  startUpload: vi.fn(),
  appendUpload: vi.fn(),
  completeUpload: vi.fn(),
  cancelUpload: vi.fn(),
}));

vi.mock("./api", () => ({ sftpApi: api }));

describe("SFTPPanel uploads", () => {
  beforeEach(() => {
	for (const job of sftpTransferQueue.getSnapshot()) void sftpTransferQueue.cancel(job.id);
	sftpTransferQueue.clearFinished();
	for (const job of sftpDownloadQueue.getSnapshot()) sftpDownloadQueue.cancel(job.id);
	sftpDownloadQueue.clearFinished();
    vi.clearAllMocks();
    api.list.mockResolvedValue({ path: "/remote", entries: [] });
    api.mkdir.mockResolvedValue(undefined);
	api.startUpload.mockImplementation(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset: 0, size, expectedRevision: "absent" }));
	api.appendUpload.mockImplementation(async (_alias: string, id: string, path: string, _offset: number, total: number) => ({ id, path, offset: total, size: total, expectedRevision: "" }));
	api.completeUpload.mockResolvedValue(undefined);
	api.cancelUpload.mockResolvedValue(undefined);
  });

  it("uploads every dropped file separately and keeps per-file results", async () => {
    api.startUpload.mockResolvedValueOnce({ id: "one", path: "/remote/first.txt", offset: 0, size: 5, expectedRevision: "absent" }).mockRejectedValueOnce(new Error("upload_failed"));
    render(<SFTPPanel aliases={["edge"]} />);
    expect(api.list).not.toHaveBeenCalled();
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/"));

    const first = new File(["alpha"], "first.txt", { type: "text/plain" });
    const second = new File(["beta"], "second.txt", { type: "text/plain" });
    fireEvent.drop(screen.getByLabelText("Upload files or folders to the current remote directory"), {
      dataTransfer: { files: [first, second] },
    });

    await waitFor(() => expect(api.startUpload).toHaveBeenCalledTimes(2));
    expect(api.startUpload).toHaveBeenNthCalledWith(1, "edge", expect.any(String), "/remote/first.txt", first.size, false, "");
    expect(api.startUpload).toHaveBeenNthCalledWith(2, "edge", expect.any(String), "/remote/second.txt", second.size, false, "");
    expect(await screen.findByText("Uploaded")).toBeInTheDocument();
    expect(await screen.findByText("upload_failed")).toBeInTheDocument();
  });

  it("does not connect until a host is selected", async () => {
    render(<SFTPPanel aliases={["edge"]} />);

    expect(screen.getByLabelText("Host")).toHaveValue("");
    expect(api.list).not.toHaveBeenCalled();

    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/"));
  });

  it("sorts remote entries by every data column", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "zeta", path: "/remote/zeta", type: "file", size: 2, mode: "0600", modifiedAt: "", revision: "z" },
        { name: "alpha", path: "/remote/alpha", type: "file", size: 10, mode: "0600", modifiedAt: "", revision: "a" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await screen.findByRole("button", { name: "alpha" });

    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("alpha");
    await userEvent.click(within(table).getByRole("button", { name: /Bytes.*sort ascending/ }));
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("zeta");
    await userEvent.click(within(table).getByRole("button", { name: /Bytes.*sort descending/ }));
    expect(within(table).getAllByRole("row")[1]).toHaveTextContent("alpha");
  });

  it("keeps the type header and values on one line in a scrollable table", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "project", path: "/remote/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-08-24T10:00:00Z", revision: "r1" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");

    const table = await screen.findByRole("table");
    expect(table).toHaveClass("min-w-[52rem]");
    expect(within(table).getByRole("columnheader", { name: /Type/ }))
      .toHaveClass("whitespace-nowrap");
    expect(within(table).getByText("Folder")).toHaveClass("whitespace-nowrap");
  });

  it("preserves folder paths and creates parent directories before upload", async () => {
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const nested = new File(["nested"], "file.txt");
    Object.defineProperty(nested, "webkitRelativePath", { value: "project/config/file.txt" });
    const picker = container.querySelector<HTMLInputElement>('input[webkitdirectory]');
    expect(picker).not.toBeNull();
    fireEvent.change(picker as HTMLInputElement, { target: { files: [nested] } });

    await waitFor(() => expect(api.startUpload).toHaveBeenCalledTimes(1));
    expect(api.mkdir.mock.calls).toEqual([
      ["edge", "/remote/project"],
      ["edge", "/remote/project/config"],
    ]);
    expect(api.startUpload).toHaveBeenCalledWith("edge", expect.any(String), "/remote/project/config/file.txt", nested.size, false, "");
  });

  it("asks before retrying an existing remote file with overwrite enabled", async () => {
    api.startUpload.mockRejectedValueOnce(new ApiError("sftp_exists", 409, null)).mockImplementationOnce(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset: 0, size, expectedRevision: "meta" }));
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const file = new File(["replacement"], "existing.txt");
    const picker = container.querySelectorAll<HTMLInputElement>('input[type="file"]')[0];
    fireEvent.change(picker as HTMLInputElement, { target: { files: [file] } });

    expect(await screen.findByText("Overwrite confirmation required")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Overwrite" }));
    await waitFor(() => expect(api.startUpload).toHaveBeenCalledTimes(2));
    expect(api.startUpload).toHaveBeenLastCalledWith("edge", expect.any(String), "/remote/existing.txt", file.size, true, "");
  });

  it("downloads folders as archives and changes permissions with the current revision", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [{ name: "project", path: "/remote/project", type: "directory", size: 0, mode: "drwxr-x---", modifiedAt: "2026-08-24T10:00:00Z", revision: "rev" }],
    });
    api.download.mockResolvedValue(10);
    api.chmod.mockResolvedValue(undefined);
    vi.spyOn(window, "prompt").mockReturnValue("750");
    render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await screen.findByRole("button", { name: /project/ });

    await userEvent.click(screen.getByRole("button", { name: "Download" }));
    await waitFor(() => expect(api.download).toHaveBeenCalledWith("edge", "/remote/project", true, expect.objectContaining({ signal: expect.any(AbortSignal) })));
    await userEvent.click(screen.getByRole("button", { name: "Change permissions" }));
    await waitFor(() => expect(api.chmod).toHaveBeenCalledWith("edge", "/remote/project", "750", "rev"));
  });
});
