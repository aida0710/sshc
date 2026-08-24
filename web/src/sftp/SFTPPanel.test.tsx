import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { SFTPPanel } from "./SFTPPanel";
import { ApiError } from "../api/client";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  upload: vi.fn(),
  mkdir: vi.fn(),
  chmod: vi.fn(),
  download: vi.fn(),
}));

vi.mock("./api", () => ({ sftpApi: api }));

describe("SFTPPanel uploads", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.list.mockResolvedValue({ path: "/remote", entries: [] });
    api.mkdir.mockResolvedValue(undefined);
  });

  it("uploads every dropped file separately and keeps per-file results", async () => {
    api.upload.mockResolvedValueOnce(undefined).mockRejectedValueOnce(new Error("upload_failed"));
    render(<SFTPPanel aliases={["edge"]} />);
    expect(api.list).not.toHaveBeenCalled();
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/"));

    const first = new File(["alpha"], "first.txt", { type: "text/plain" });
    const second = new File(["beta"], "second.txt", { type: "text/plain" });
    fireEvent.drop(screen.getByLabelText("Upload files or folders to the current remote directory"), {
      dataTransfer: { files: [first, second] },
    });

    await waitFor(() => expect(api.upload).toHaveBeenCalledTimes(2));
    expect(api.upload).toHaveBeenNthCalledWith(1, "edge", "/remote/first.txt", first, false, expect.any(AbortSignal));
    expect(api.upload).toHaveBeenNthCalledWith(2, "edge", "/remote/second.txt", second, false, expect.any(AbortSignal));
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

  it("preserves folder paths and creates parent directories before upload", async () => {
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const nested = new File(["nested"], "file.txt");
    Object.defineProperty(nested, "webkitRelativePath", { value: "project/config/file.txt" });
    const picker = container.querySelector<HTMLInputElement>('input[webkitdirectory]');
    expect(picker).not.toBeNull();
    fireEvent.change(picker as HTMLInputElement, { target: { files: [nested] } });

    await waitFor(() => expect(api.upload).toHaveBeenCalledTimes(1));
    expect(api.mkdir.mock.calls).toEqual([
      ["edge", "/remote/project"],
      ["edge", "/remote/project/config"],
    ]);
    expect(api.upload).toHaveBeenCalledWith("edge", "/remote/project/config/file.txt", nested, false, expect.any(AbortSignal));
  });

  it("asks before retrying an existing remote file with overwrite enabled", async () => {
    api.upload.mockRejectedValueOnce(new ApiError("sftp_exists", 409, null)).mockResolvedValueOnce(undefined);
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const file = new File(["replacement"], "existing.txt");
    const picker = container.querySelectorAll<HTMLInputElement>('input[type="file"]')[0];
    fireEvent.change(picker as HTMLInputElement, { target: { files: [file] } });

    expect(await screen.findByText("Overwrite this remote file?")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Overwrite" }));
    await waitFor(() => expect(api.upload).toHaveBeenCalledTimes(2));
    expect(api.upload).toHaveBeenLastCalledWith("edge", "/remote/existing.txt", file, true, expect.any(AbortSignal));
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
