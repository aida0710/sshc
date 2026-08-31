import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { SFTPPanel } from "./SFTPPanel";
import { ApiError } from "../api/client";
import { sftpTransferManager } from "./transferManager";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  readText: vi.fn(),
  upload: vi.fn(),
  mkdir: vi.fn(),
  chmod: vi.fn(),
  download: vi.fn(),
  startUpload: vi.fn(),
  appendUpload: vi.fn(),
  completeUpload: vi.fn(),
  cancelUpload: vi.fn(),
  createTransfer: vi.fn(),
  updateTransfer: vi.fn(),
  streamDownload: vi.fn(),
  saveDownload: vi.fn(),
  listTransfers: vi.fn(),
  clearFinishedTransfers: vi.fn(),
}));

vi.mock("./api", () => ({ sftpApi: api }));

describe("SFTPPanel uploads", () => {
  beforeEach(async () => {
	await Promise.all(sftpTransferManager.getSnapshot().map((job) => sftpTransferManager.cancel(job.id)));
	await sftpTransferManager.clearFinished();
    vi.clearAllMocks();
    api.list.mockResolvedValue({ path: "/remote", entries: [] });
    api.readText.mockResolvedValue({
      entry: { name: "app.log", path: "/var/log/app.log", type: "file", size: 12, mode: "0644", modifiedAt: "", revision: "rev" },
      contents: "hello\n",
      revision: "rev",
    });
    api.mkdir.mockResolvedValue(undefined);
	api.startUpload.mockImplementation(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset: 0, size, expectedRevision: "absent" }));
	api.appendUpload.mockImplementation(async (_alias: string, id: string, path: string, _offset: number, total: number) => ({ id, path, offset: total, size: total, expectedRevision: "" }));
	api.completeUpload.mockResolvedValue(undefined);
	api.cancelUpload.mockResolvedValue(undefined);
    const server = new Map<string, Record<string, unknown>>();
    const allowedActions = (status: string): string[] => {
      if (status === "queued" || status === "running") return ["pause", "cancel"];
      if (status === "paused" || status === "reattach" || status === "needs_overwrite") return ["resume", "cancel"];
      if (status === "failed") return ["retry", "cancel"];
      return [];
    };
    api.createTransfer.mockImplementation(async (input: Record<string, unknown>) => {
      const existing = server.get(input.id as string);
      if (existing !== undefined) return existing;
      const created = { ...input, transferredBytes: 0, bytesPerSecond: 0, remainingSeconds: -1, status: "queued", allowedActions: ["pause", "cancel"], attempt: 1, problem: "", expectedRevision: "", sourceFingerprint: "", overwrite: false, downloadRevision: "", createdAt: new Date().toISOString(), updatedAt: new Date().toISOString() };
      server.set(input.id as string, created);
      return created;
    });
    api.updateTransfer.mockImplementation(async (id: string, action: string, options: { transferredBytes?: number; problem?: string } = {}) => {
      const current = server.get(id) ?? {};
      const statuses: Record<string, string> = { start: "running", pause: "paused", resume: "queued", retry: "queued", cancel: "cancelled", complete: "completed", fail: "failed", needs_overwrite: "needs_overwrite" };
      const status = statuses[action] ?? current.status as string;
      const updated = { ...current, ...options, status, allowedActions: allowedActions(status), overwrite: action === "resume" && current.status === "needs_overwrite" ? true : current.overwrite, problem: action === "fail" ? options.problem ?? "sftp_failed" : current.problem ?? "" };
      server.set(id, updated);
      return updated;
    });
    api.streamDownload.mockResolvedValue({ bytes: 10, total: 10 });
    api.saveDownload.mockReturnValue(undefined);
    api.completeUpload.mockImplementation(async (_alias: string, id: string, _path: string, size: number) => {
      const current = server.get(id);
      if (current !== undefined) server.set(id, { ...current, transferredBytes: size, status: "completed", allowedActions: [], remainingSeconds: 0 });
    });
    api.listTransfers.mockImplementation(async () => ({ maxConcurrent: 2, jobs: [...server.values()] }));
    api.clearFinishedTransfers.mockImplementation(async () => {
      for (const [id, job] of server) if (job.status === "completed" || job.status === "cancelled") server.delete(id);
    });
  });

  it("uploads every dropped file separately and keeps per-file results", async () => {
    api.startUpload.mockImplementation(async (_alias, _id, remotePath, size) => {
      if (remotePath.endsWith("second.txt")) throw new Error("upload_failed");
      return { id: "one", path: remotePath, offset: 0, size, expectedRevision: "absent" };
    });
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
    expect(api.startUpload).toHaveBeenCalledWith("edge", expect.any(String), "/remote/first.txt", first.size, expect.stringMatching(/^tree-sha256:/));
    expect(api.startUpload).toHaveBeenCalledWith("edge", expect.any(String), "/remote/second.txt", second.size, expect.stringMatching(/^tree-sha256:/));
    expect(await screen.findByText("Completed")).toBeInTheDocument();
    expect(await screen.findByText("upload_failed")).toBeInTheDocument();
  });

  it("does not connect until a host is selected", async () => {
    render(<SFTPPanel aliases={["edge"]} />);

    expect(screen.getByLabelText("Host")).toHaveValue("");
    expect(api.list).not.toHaveBeenCalled();

    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/"));
  });

  it("ignores a stale host listing that resolves after the current host", async () => {
    let resolveEdge: ((listing: { path: string; entries: never[] }) => void) | undefined;
    api.list.mockImplementation((alias: string) => {
      if (alias === "edge") {
        return new Promise((resolve) => { resolveEdge = resolve; });
      }
      return Promise.resolve({ path: "/current", entries: [] });
    });

    render(<SFTPPanel aliases={["edge", "miyabi"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/"));
    await userEvent.selectOptions(screen.getByLabelText("Host"), "miyabi");
    await waitFor(() => expect(screen.getByLabelText("Remote path")).toHaveValue("/current"));

    resolveEdge?.({ path: "/stale", entries: [] });
    await waitFor(() => expect(screen.getByLabelText("Remote path")).toHaveValue("/current"));
    expect(screen.getByLabelText("Host")).toHaveValue("miyabi");
  });

  it("clears old rows immediately and ignores a file read completed after a host switch", async () => {
    let resolveRead: ((file: {
      entry: { name: string; path: string; type: "file"; size: number; mode: string; modifiedAt: string; revision: string };
      contents: string;
      revision: string;
    }) => void) | undefined;
    api.list.mockImplementation(async (alias: string) => alias === "edge"
      ? {
          path: "/edge",
          entries: [{ name: "old.txt", path: "/edge/old.txt", type: "file", size: 3, mode: "0644", modifiedAt: "", revision: "old" }],
        }
      : { path: "/miyabi", entries: [] });
    api.readText.mockImplementation(() => new Promise((resolve) => { resolveRead = resolve; }));

    render(<SFTPPanel aliases={["edge", "miyabi"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await userEvent.click(await screen.findByRole("button", { name: "old.txt" }));
    await waitFor(() => expect(api.readText).toHaveBeenCalledWith("edge", "/edge/old.txt"));

    await userEvent.selectOptions(screen.getByLabelText("Host"), "miyabi");
    expect(screen.queryByRole("button", { name: "old.txt" })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByLabelText("Remote path")).toHaveValue("/miyabi"));

    resolveRead?.({
      entry: { name: "old.txt", path: "/edge/old.txt", type: "file", size: 3, mode: "0644", modifiedAt: "", revision: "old" },
      contents: "old",
      revision: "old",
    });
    await waitFor(() => expect(screen.queryByText("/edge/old.txt")).not.toBeInTheDocument());
    expect(screen.getByLabelText("Host")).toHaveValue("miyabi");
  });

  it("opens the parent directory requested by a terminal path action", async () => {
    const handled = vi.fn();
    render(<SFTPPanel aliases={["edge"]} target={{ alias: "edge", path: "/var/log/app.log", action: "browse", request: 1 }} onTargetHandled={handled} />);

    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/var/log"));
    expect(handled).toHaveBeenCalledWith(1);
    expect(screen.getByLabelText("Host")).toHaveValue("edge");
    expect(screen.getByLabelText("Remote path")).toHaveValue("/remote");
  });

  it("opens a terminal-linked remote file in the editor", async () => {
    api.list.mockResolvedValue({
      path: "/var/log",
      entries: [{ name: "app.log", path: "/var/log/app.log", type: "file", size: 12, mode: "0644", modifiedAt: "", revision: "rev" }],
    });

    render(<SFTPPanel aliases={["edge"]} target={{ alias: "edge", path: "/var/log/app.log", action: "edit", request: 2 }} />);

    await waitFor(() => expect(api.readText).toHaveBeenCalledWith("edge", "/var/log/app.log"));
  });

  it("enters a remote directory selected from a terminal link", async () => {
    api.list.mockImplementation(async (_alias: string, path: string) => path === "/var"
      ? { path, entries: [{ name: "log", path: "/var/log", type: "directory", size: 0, mode: "0755", modifiedAt: "", revision: "rev" }] }
      : { path, entries: [] });

    render(<SFTPPanel aliases={["edge"]} target={{ alias: "edge", path: "/var/log", action: "browse", request: 5 }} />);

    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/var/log"));
    expect(screen.getByLabelText("Remote path")).toHaveValue("/var/log");
  });

  it("queues a terminal-linked remote file for download", async () => {
    api.list.mockResolvedValue({
      path: "/var/log",
      entries: [{ name: "app.log", path: "/var/log/app.log", type: "file", size: 12, mode: "0644", modifiedAt: "", revision: "rev" }],
    });
    const addDownload = vi.spyOn(sftpTransferManager, "addDownload").mockResolvedValue("download-one");

    render(<SFTPPanel aliases={["edge"]} target={{ alias: "edge", path: "/var/log/app.log", action: "download", request: 3 }} />);

    await waitFor(() => expect(addDownload).toHaveBeenCalledWith("edge", "/var/log/app.log", "file", 12));
    addDownload.mockRestore();
  });

  it("rejects a terminal path action for an unknown host before connecting", async () => {
    render(<SFTPPanel aliases={["edge"]} target={{ alias: "missing", path: "/etc/hosts", action: "browse", request: 4 }} />);

    expect(await screen.findByRole("alert")).toHaveTextContent("This terminal link is no longer available.");
    expect(api.list).not.toHaveBeenCalled();
  });

  it("shows a friendly inline message when SFTP cannot connect", async () => {
    api.list.mockRejectedValueOnce(new ApiError("sftp_failed", 502, null));
    render(<SFTPPanel aliases={["miyabi"]} />);

    await userEvent.selectOptions(screen.getByLabelText("Host"), "miyabi");

    expect(await screen.findByRole("alert")).toHaveTextContent("Could not connect.");
    expect(screen.queryByText("sftp_failed")).not.toBeInTheDocument();
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
    expect(api.startUpload).toHaveBeenCalledWith("edge", expect.any(String), "/remote/project/config/file.txt", nested.size, expect.stringMatching(/^tree-sha256:/));
  });

  it("rejects queue overflow before creating remote directories and always releases the busy state", async () => {
    const reserve = vi.spyOn(sftpTransferManager, "reserveUploads")
      .mockImplementationOnce(() => { throw new Error("sftp_transfer_limit"); });
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const nested = new File(["nested"], "file.txt");
    Object.defineProperty(nested, "webkitRelativePath", { value: "project/file.txt" });
    const folderPicker = container.querySelector<HTMLInputElement>('input[webkitdirectory]');
    fireEvent.change(folderPicker as HTMLInputElement, { target: { files: [nested] } });

    expect(await screen.findByText("sftp_transfer_limit")).toBeVisible();
    expect(api.mkdir).not.toHaveBeenCalled();
    const plain = new File(["plain"], "plain.txt");
    const filePicker = container.querySelectorAll<HTMLInputElement>('input[type="file"]')[0];
    fireEvent.change(filePicker as HTMLInputElement, { target: { files: [plain] } });
    await waitFor(() => expect(api.startUpload).toHaveBeenCalledOnce());
    reserve.mockRestore();
  });

  it("asks before retrying an existing remote file with overwrite enabled", async () => {
    api.startUpload.mockRejectedValueOnce(new ApiError("sftp_exists", 409, null)).mockImplementationOnce(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset: 0, size, expectedRevision: "meta" }));
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const file = new File(["replacement"], "existing.txt");
    const picker = container.querySelectorAll<HTMLInputElement>('input[type="file"]')[0];
    fireEvent.change(picker as HTMLInputElement, { target: { files: [file] } });

    expect(await screen.findByText("Confirm overwrite")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Overwrite" }));
    await waitFor(() => expect(api.startUpload).toHaveBeenCalledTimes(2));
    expect(api.startUpload).toHaveBeenLastCalledWith("edge", expect.any(String), "/remote/existing.txt", file.size, expect.stringMatching(/^tree-sha256:/));
  });

  it("downloads folders as archives and changes permissions with the current revision", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [{ name: "project", path: "/remote/project", type: "directory", size: 0, mode: "drwxr-x---", modifiedAt: "2026-08-24T10:00:00Z", revision: "rev" }],
    });
    api.chmod.mockResolvedValue(undefined);
    vi.spyOn(window, "prompt").mockReturnValue("750");
    render(<SFTPPanel aliases={["edge"]} />);
    await userEvent.selectOptions(screen.getByLabelText("Host"), "edge");
    await screen.findByRole("button", { name: /project/ });

    await userEvent.click(screen.getByRole("button", { name: "Download" }));
    await waitFor(() => expect(api.streamDownload).toHaveBeenCalledWith("edge", expect.any(String), "/remote/project", true, 0, expect.objectContaining({ signal: expect.any(AbortSignal) })));
    await userEvent.click(screen.getByRole("button", { name: "Change permissions" }));
    await waitFor(() => expect(api.chmod).toHaveBeenCalledWith("edge", "/remote/project", "750", "rev"));
  });
});
