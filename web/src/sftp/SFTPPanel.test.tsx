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
  rename: vi.fn(),
  search: vi.fn(),
  chmod: vi.fn(),
  remove: vi.fn(),
  download: vi.fn(),
  startUpload: vi.fn(),
	appendUpload: vi.fn(),
	appendUploadRange: vi.fn(),
  completeUpload: vi.fn(),
  cancelUpload: vi.fn(),
  createTransfer: vi.fn(),
  updateTransfer: vi.fn(),
  streamDownload: vi.fn(),
  saveDownload: vi.fn(),
  listTransfers: vi.fn(),
  previewFile: vi.fn(),
  clearFinishedTransfers: vi.fn(),
}));
const clipboard = vi.hoisted(() => ({ writeText: vi.fn(async () => undefined) }));

vi.mock("./api", () => ({ sftpApi: api }));
vi.mock("../ui/clipboard", () => ({ clipboard: { readText: vi.fn(), writeText: clipboard.writeText } }));
vi.mock("../api/integrations", () => ({ integrationsApi: { recentConnections: vi.fn(async () => ({ connections: [] })) } }));

async function chooseHost(alias: string) {
  await userEvent.click(screen.getByRole("button", { name: "Host" }));
  const label = await screen.findByText(alias, { selector: "span.font-medium" });
  await userEvent.click(label.closest("button")!);
}

describe("SFTPPanel uploads", () => {
  beforeEach(async () => {
	window.localStorage.clear();
	window.localStorage.setItem("sshc.sftp.queueView", JSON.stringify({ collapsed: false, height: 224 }));
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
    api.rename.mockResolvedValue(undefined);
    api.previewFile.mockRejectedValue(new ApiError("sftp_preview_type", 415, null));
    api.remove.mockResolvedValue(undefined);
	api.startUpload.mockImplementation(async (_alias: string, id: string, path: string, size: number) => ({ id, path, offset: 0, size, expectedRevision: "absent", completedRanges: [], parallelism: 1, chunkBytes: 32 << 20 }));
	api.appendUpload.mockImplementation(async (_alias: string, id: string, path: string, _offset: number, total: number) => ({ id, path, offset: total, size: total, expectedRevision: "", completedRanges: [], parallelism: 1, chunkBytes: 32 << 20 }));
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
    api.listTransfers.mockImplementation(async () => ({
      maxConcurrent: 2, clearCompletedAfterSeconds: 0, processingStopped: false,
      largeFileThresholdBytes: 100 << 20, largeFileParallelism: 4, largeFileChunkBytes: 32 << 20, jobs: [...server.values()],
    }));
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
    await chooseHost("edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", ""));

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

    expect(screen.getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "");
    expect(api.list).not.toHaveBeenCalled();

    await chooseHost("edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", ""));
    expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/remote");
  });

  it("opens a terminal at the displayed remote directory", async () => {
    const onOpenTerminal = vi.fn();
    render(<SFTPPanel aliases={["edge"]} onOpenTerminal={onOpenTerminal} />);
    await chooseHost("edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", ""));

    await userEvent.click(screen.getByRole("button", { name: "Open Terminal here" }));

    expect(onOpenTerminal).toHaveBeenCalledWith("edge", "/remote");
  });

  it("shows a parent-directory row first and uses it instead of a separate up button", async () => {
    api.list.mockImplementation(async (_alias: string, path: string) => ({
      path: path === "" ? "/remote" : path,
      entries: path === "" ? [{ name: "project", path: "/remote/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-08-24T10:00:00Z", revision: "rev" }] : [],
    }));
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");

    const table = await screen.findByRole("table");
    const rows = within(table).getAllByRole("row");
    expect(rows[1]).toHaveAccessibleName("Parent directory");
    expect(within(rows[1] as HTMLElement).getByText("..")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Up" })).not.toBeInTheDocument();

    await userEvent.click(within(rows[1] as HTMLElement).getByRole("button", { name: "Parent directory" }));
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/"));
  });

  it("navigates back, forward, home, and root without losing directory history", async () => {
    api.list.mockImplementation(async (_alias: string, requestedPath: string) => {
      const resolvedPath = requestedPath === "" ? "/home/edge" : requestedPath;
      return {
        path: resolvedPath,
        entries: resolvedPath === "/home/edge"
          ? [{ name: "project", path: "/home/edge/project", type: "directory", size: 0, mode: "0755", modifiedAt: "", revision: "project" }]
          : [],
      };
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.dblClick(await screen.findByRole("button", { name: "project" }));
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/home/edge/project"));

    await userEvent.click(screen.getByRole("button", { name: "Back" }));
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/home/edge"));
    expect(screen.getByRole("button", { name: "Forward" })).toBeEnabled();

    await userEvent.click(screen.getByRole("button", { name: "Forward" }));
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/home/edge/project"));
    await userEvent.click(screen.getByRole("button", { name: "Root directory" }));
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/"));
    await userEvent.click(screen.getByRole("button", { name: "Home directory" }));
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/home/edge"));
    expect(api.list).toHaveBeenCalledWith("edge", "");
  });

  it("filters the current directory without clearing hidden selections", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "alpha.txt", path: "/remote/alpha.txt", type: "file", size: 1, mode: "0644", modifiedAt: "", revision: "alpha" },
        { name: "beta.txt", path: "/remote/beta.txt", type: "file", size: 2, mode: "0644", modifiedAt: "", revision: "beta" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await screen.findByRole("button", { name: "alpha.txt" });

    await userEvent.type(screen.getByRole("searchbox", { name: "Filter remote entries" }), "alpha");
    expect(screen.getByRole("button", { name: "alpha.txt" })).toBeVisible();
    expect(screen.queryByRole("button", { name: "beta.txt" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("checkbox", { name: "Select all entries" }));

    await userEvent.clear(screen.getByRole("searchbox", { name: "Filter remote entries" }));
    expect(screen.getByRole("checkbox", { name: "Select alpha.txt" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Select beta.txt" })).not.toBeChecked();
  });

  it("selects a compact row without opening it and opens it on double click", async () => {
    api.list.mockImplementation(async (_alias: string, path: string) => path === "/remote/project"
      ? { path, entries: [] }
      : { path: "/remote", entries: [{ name: "project", path: "/remote/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-08-24T10:00:00Z", revision: "rev" }] });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");

    const project = await screen.findByRole("button", { name: "project" });
    await userEvent.click(project);
    expect(project).toHaveAttribute("aria-pressed", "true");
    expect(api.list).not.toHaveBeenCalledWith("edge", "/remote/project");
    expect(screen.getByRole("button", { name: "Download" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Actions for project" })).toBeEnabled();

    await userEvent.dblClick(project);
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/remote/project"));
  });

  it("selects multiple entries with checkboxes and limits the batch action menu", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "project", path: "/remote/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-08-24T10:00:00Z", revision: "dir" },
        { name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 12, mode: "0644", modifiedAt: "2026-08-24T11:00:00Z", revision: "file" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");

    await userEvent.click(await screen.findByRole("checkbox", { name: "Select project" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "Select notes.txt" }));
    expect(screen.getByText(new RegExp("2 selected"))).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Actions for 2 selected items" }));
    expect(screen.queryByRole("menuitem", { name: "Open folder" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Change permissions" })).not.toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Rename" })).not.toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Download" })).toBeEnabled();
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeEnabled();
  });

  it("supports range/additive selection and copies selected names or full paths", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "alpha.txt", path: "/remote/alpha.txt", type: "file", size: 1, mode: "0644", modifiedAt: "", revision: "alpha" },
        { name: "beta.txt", path: "/remote/beta.txt", type: "file", size: 2, mode: "0644", modifiedAt: "", revision: "beta" },
        { name: "gamma.txt", path: "/remote/gamma.txt", type: "file", size: 3, mode: "0644", modifiedAt: "", revision: "gamma" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");

    fireEvent.click(await screen.findByRole("button", { name: "alpha.txt" }));
    fireEvent.click(screen.getByRole("button", { name: "gamma.txt" }), { shiftKey: true });
    expect(screen.getByText(new RegExp("3 selected"))).toBeVisible();
    fireEvent.click(screen.getByRole("button", { name: "beta.txt" }), { ctrlKey: true });
    expect(screen.getByText(new RegExp("2 selected"))).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Actions for 2 selected items" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Copy names" }));
    expect(clipboard.writeText).toHaveBeenLastCalledWith("alpha.txt\ngamma.txt");

    await userEvent.click(screen.getByRole("button", { name: "Actions for 2 selected items" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Copy full paths" }));
    expect(clipboard.writeText).toHaveBeenLastCalledWith("/remote/alpha.txt\n/remote/gamma.txt");
  });

  it("deletes multiple selected entries after one confirmation", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "first.txt", path: "/remote/first.txt", type: "file", size: 1, mode: "0644", modifiedAt: "", revision: "first" },
        { name: "second.txt", path: "/remote/second.txt", type: "file", size: 2, mode: "0644", modifiedAt: "", revision: "second" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.click(await screen.findByRole("checkbox", { name: "Select all entries" }));
    await userEvent.click(screen.getByRole("button", { name: "Actions for 2 selected items" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    const dialog = screen.getByRole("dialog", { name: "Delete 2 remote entries?" });
    expect(dialog).toHaveTextContent("/remote/first.txt");
    expect(dialog).toHaveTextContent("/remote/second.txt");
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(api.remove).toHaveBeenCalledTimes(2));
    expect(api.remove.mock.calls).toEqual([
      ["edge", "/remote/first.txt"],
      ["edge", "/remote/second.txt"],
    ]);
  });

  it("opens the text editor as a modal without resizing the file list", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [{ name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 12, mode: "0644", modifiedAt: "", revision: "rev" }],
    });
    api.readText.mockResolvedValue({
      entry: { name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 12, mode: "0644", modifiedAt: "", revision: "rev" },
      contents: "hello\n",
      revision: "rev",
    });
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.dblClick(await screen.findByRole("button", { name: "notes.txt" }));

    const details = await screen.findByRole("dialog", { name: "Details for notes.txt" });
    expect(within(details).getByText("/remote/notes.txt")).toBeVisible();
    await userEvent.click(within(details).getByRole("button", { name: "Edit file" }));

    const dialog = await screen.findByRole("dialog", { name: "/remote/notes.txt" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(container.querySelector(".grid-cols-1")).not.toBeNull();
    expect(screen.getByRole("table")).toBeInTheDocument();
    await userEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog", { name: "/remote/notes.txt" })).not.toBeInTheDocument();
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
    await chooseHost("edge");
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", ""));
    await chooseHost("miyabi");
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/current"));

    resolveEdge?.({ path: "/stale", entries: [] });
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/current"));
    expect(screen.getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "miyabi");
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
    await chooseHost("edge");
    await userEvent.dblClick(await screen.findByRole("button", { name: "old.txt" }));
    const details = await screen.findByRole("dialog", { name: "Details for old.txt" });
    await userEvent.click(within(details).getByRole("button", { name: "Edit file" }));
    await waitFor(() => expect(api.readText).toHaveBeenCalledWith("edge", "/edge/old.txt"));

    await chooseHost("miyabi");
    expect(screen.queryByRole("button", { name: "old.txt" })).not.toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/miyabi"));

    resolveRead?.({
      entry: { name: "old.txt", path: "/edge/old.txt", type: "file", size: 3, mode: "0644", modifiedAt: "", revision: "old" },
      contents: "old",
      revision: "old",
    });
    await waitFor(() => expect(screen.queryByText("/edge/old.txt")).not.toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "miyabi");
  });

  it("opens the parent directory requested by a terminal path action", async () => {
    const handled = vi.fn();
    render(<SFTPPanel aliases={["edge"]} target={{ alias: "edge", path: "/var/log/app.log", action: "browse", request: 1 }} onTargetHandled={handled} />);

    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/var/log"));
    expect(handled).toHaveBeenCalledWith(1);
    expect(screen.getByRole("button", { name: "Host" })).toHaveAttribute("data-value", "edge");
    expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/remote");
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
    expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/var/log");
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

    await chooseHost("miyabi");

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
    await chooseHost("edge");
    await screen.findByRole("button", { name: "alpha" });

    const table = screen.getByRole("table");
    expect(within(table).getAllByRole("row")[2]).toHaveTextContent("alpha");
    await userEvent.click(within(table).getByRole("button", { name: /Bytes.*sort ascending/ }));
    expect(within(table).getAllByRole("row")[2]).toHaveTextContent("zeta");
    await userEvent.click(within(table).getByRole("button", { name: /Bytes.*sort descending/ }));
    expect(within(table).getAllByRole("row")[2]).toHaveTextContent("alpha");
  });

  it("keeps the type header and values on one line in a scrollable table", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "project", path: "/remote/project", type: "directory", size: 0, mode: "0755", modifiedAt: "2026-08-24T10:00:00Z", revision: "r1" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");

    const table = await screen.findByRole("table");
    expect(table).toHaveClass("min-w-[44rem]");
    expect(within(table).getByRole("columnheader", { name: /Type/ }))
      .toHaveClass("whitespace-nowrap");
    expect(within(table).getByText("Folder")).toHaveClass("whitespace-nowrap");
  });

  it("preserves folder paths and creates parent directories before upload", async () => {
    const { container } = render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
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
    await chooseHost("edge");
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
    await chooseHost("edge");
    await waitFor(() => expect(api.list).toHaveBeenCalled());
    const file = new File(["replacement"], "existing.txt");
    const picker = container.querySelectorAll<HTMLInputElement>('input[type="file"]')[0];
    fireEvent.change(picker as HTMLInputElement, { target: { files: [file] } });

    expect(await screen.findByText("Confirm overwrite")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Overwrite" }));
    await waitFor(() => expect(api.startUpload).toHaveBeenCalledTimes(2));
    expect(api.startUpload).toHaveBeenLastCalledWith("edge", expect.any(String), "/remote/existing.txt", file.size, expect.stringMatching(/^tree-sha256:/));
  });

  it("moves the row cursor with the arrow keys, Home and End while selecting the row it lands on", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "alpha.txt", path: "/remote/alpha.txt", type: "file", size: 1, mode: "0644", modifiedAt: "", revision: "alpha" },
        { name: "beta.txt", path: "/remote/beta.txt", type: "file", size: 2, mode: "0644", modifiedAt: "", revision: "beta" },
        { name: "gamma.txt", path: "/remote/gamma.txt", type: "file", size: 3, mode: "0644", modifiedAt: "", revision: "gamma" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    const alpha = await screen.findByRole("button", { name: "alpha.txt" });
    alpha.focus();

    fireEvent.keyDown(alpha, { key: "ArrowDown" });
    expect(screen.getByRole("button", { name: "beta.txt" })).toHaveFocus();
    expect(screen.getByText("Selected: beta.txt")).toBeVisible();

    fireEvent.keyDown(screen.getByRole("button", { name: "beta.txt" }), { key: "End" });
    expect(screen.getByRole("button", { name: "gamma.txt" })).toHaveFocus();
    expect(screen.getByText("Selected: gamma.txt")).toBeVisible();

    fireEvent.keyDown(screen.getByRole("button", { name: "gamma.txt" }), { key: "ArrowUp", shiftKey: true });
    expect(screen.getByText(new RegExp("2 selected"))).toBeVisible();

    fireEvent.keyDown(screen.getByRole("button", { name: "beta.txt" }), { key: "Home" });
    expect(screen.getByRole("button", { name: "Parent directory" })).toHaveFocus();

    fireEvent.keyDown(screen.getByRole("button", { name: "Parent directory" }), { key: "ArrowDown" });
    expect(screen.getByRole("button", { name: "alpha.txt" })).toHaveFocus();
    expect(screen.getByText("Selected: alpha.txt")).toBeVisible();
  });

  it("toggles selection with Space, opens with Enter and clears the selection with Escape", async () => {
    api.list.mockImplementation(async (_alias: string, path: string) => path === "/remote/project"
      ? { path, entries: [] }
      : {
          path: "/remote",
          entries: [
            { name: "project", path: "/remote/project", type: "directory", size: 0, mode: "0755", modifiedAt: "", revision: "dir" },
            { name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 4, mode: "0644", modifiedAt: "", revision: "file" },
          ],
        });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    const project = await screen.findByRole("button", { name: "project" });
    project.focus();

    fireEvent.keyDown(project, { key: " " });
    expect(screen.getByRole("checkbox", { name: "Select project" })).toBeChecked();
    expect(api.list).not.toHaveBeenCalledWith("edge", "/remote/project");

    fireEvent.keyDown(project, { key: "Escape" });
    expect(screen.getByRole("checkbox", { name: "Select project" })).not.toBeChecked();

    fireEvent.keyDown(project, { key: "Enter" });
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/remote/project"));
  });

  it("renames with F2 and deletes with Delete, returning focus to the row when the dialog is cancelled", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [{ name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 4, mode: "0644", modifiedAt: "", revision: "file" }],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    const notes = await screen.findByRole("button", { name: "notes.txt" });
    await userEvent.click(notes);

    fireEvent.keyDown(notes, { key: "F2" });
    const renameDialog = await screen.findByRole("dialog", { name: "Rename" });
    expect(within(renameDialog).getByRole("textbox", { name: "New name" })).toHaveValue("notes.txt");
    await userEvent.click(within(renameDialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "notes.txt" })).toHaveFocus());

    fireEvent.keyDown(screen.getByRole("button", { name: "notes.txt" }), { key: "Delete" });
    const deleteDialog = await screen.findByRole("dialog", { name: "Delete this remote entry?" });
    await userEvent.click(within(deleteDialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.getByRole("button", { name: "notes.txt" })).toHaveFocus());
    expect(api.remove).not.toHaveBeenCalled();
  });

  it("opens the same actions from a right click as from the overflow button", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 4, mode: "0644", modifiedAt: "", revision: "file" },
        { name: "other.txt", path: "/remote/other.txt", type: "file", size: 5, mode: "0644", modifiedAt: "", revision: "other" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");

    await userEvent.click(await screen.findByRole("button", { name: "notes.txt" }));
    await userEvent.click(screen.getByRole("button", { name: "Actions for notes.txt" }));
    const overflowItems = within(screen.getByRole("menu", { name: "Actions for notes.txt" }))
      .getAllByRole("menuitem").map((item) => item.textContent);
    await userEvent.keyboard("{Escape}");

    fireEvent.contextMenu(screen.getByRole("button", { name: "other.txt" }));
    const contextMenu = await screen.findByRole("menu", { name: "Actions for other.txt" });
    expect(within(contextMenu).getAllByRole("menuitem").map((item) => item.textContent)).toEqual(overflowItems);
    expect(screen.getByRole("checkbox", { name: "Select other.txt" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Select notes.txt" })).not.toBeChecked();
  });

  it("previews an image and lists its properties in the details dialog", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [{ name: "photo.png", path: "/remote/photo.png", type: "file", size: 2048, mode: "-rw-r--r--", modifiedAt: "2026-08-24T10:00:00Z", revision: "rev-7" }],
    });
    api.previewFile.mockResolvedValue({ contentType: "image/png", revision: "rev-7", blob: new Blob(["png"], { type: "image/png" }) });

    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.dblClick(await screen.findByRole("button", { name: "photo.png" }));

    const dialog = await screen.findByRole("dialog", { name: "Details for photo.png" });
    const image = await within(dialog).findByRole("img", { name: "photo.png" });
    // A data: URL, because the app's CSP allows data: for images and nothing else.
    expect(image.getAttribute("src")).toMatch(/^data:image\/png;base64,/);
    expect(api.previewFile).toHaveBeenCalledWith("edge", "/remote/photo.png");
    expect(api.readText).not.toHaveBeenCalled();
    const properties = within(dialog).getByRole("group", { name: "Properties" });
    expect(properties).toHaveTextContent("/remote/photo.png");
    expect(properties).toHaveTextContent("2,048 (2.0 KiB)");
    expect(properties).toHaveTextContent("-rw-r--r-- (644)");
    expect(properties).toHaveTextContent("rev-7");

    await userEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog", { name: "Details for photo.png" })).not.toBeInTheDocument();
  });

  it("summarises the count and the total size when several entries are inspected together", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [
        { name: "first.bin", path: "/remote/first.bin", type: "file", size: 1024, mode: "0644", modifiedAt: "", revision: "one" },
        { name: "second.bin", path: "/remote/second.bin", type: "file", size: 3072, mode: "0644", modifiedAt: "", revision: "two" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.click(await screen.findByRole("checkbox", { name: "Select all entries" }));
    expect(screen.getByText("2 selected · 4.0 KiB")).toBeVisible();

    await userEvent.click(screen.getByRole("button", { name: "Actions for 2 selected items" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Details" }));

    const dialog = await screen.findByRole("dialog", { name: "Details for 2 selected items" });
    const properties = within(dialog).getByRole("group", { name: "Properties" });
    expect(properties).toHaveTextContent("4,096 (4.0 KiB)");
    expect(within(dialog).getByRole("list", { name: "Selected items" })).toHaveTextContent("/remote/second.bin");
    expect(api.previewFile).not.toHaveBeenCalled();
  });

  it("bookmarks the current folder and reopens it from the places menu", async () => {
    api.list.mockImplementation(async (_alias: string, requestedPath: string) => ({
      path: requestedPath === "" ? "/srv/app" : requestedPath,
      entries: [],
    }));
    // A host of its own: the place book is a singleton shared by the suite.
    render(<SFTPPanel aliases={["placebook"]} />);
    await chooseHost("placebook");
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/srv/app"));

    await userEvent.click(screen.getByRole("button", { name: "Bookmarks and recent paths" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Bookmark this folder" }));

    // The menu stays open so the toggle can report what it just did.
    const menu = screen.getByRole("menu", { name: "Bookmarks and recent paths" });
    expect(within(menu).getByRole("menuitem", { name: "Remove this bookmark" })).toBeVisible();
    expect(within(menu).getByRole("menuitem", { name: "/srv/app" })).toBeVisible();
    await userEvent.keyboard("{Escape}");

    await userEvent.click(screen.getByRole("button", { name: "Root directory" }));
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/"));

    await userEvent.click(screen.getByRole("button", { name: "Bookmarks and recent paths" }));
    await userEvent.click(within(screen.getByRole("menu", { name: "Bookmarks and recent paths" }))
      .getByRole("menuitem", { name: "/srv/app" }));
    await waitFor(() => expect(screen.getByTestId("sftp-current-path")).toHaveAttribute("data-path", "/srv/app"));
  });

  it("offers to undo a rename and puts the old name back", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [{ name: "notes.txt", path: "/remote/notes.txt", type: "file", size: 4, mode: "0644", modifiedAt: "", revision: "file" }],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.click(await screen.findByRole("button", { name: "notes.txt" }));
    await userEvent.click(screen.getByRole("button", { name: "Actions for notes.txt" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Rename" }));

    const dialog = screen.getByRole("dialog", { name: "Rename" });
    await userEvent.clear(within(dialog).getByRole("textbox", { name: "New name" }));
    await userEvent.type(within(dialog).getByRole("textbox", { name: "New name" }), "diary.txt");
    await userEvent.click(within(dialog).getByRole("button", { name: "Rename" }));
    await waitFor(() => expect(api.rename).toHaveBeenCalledWith("edge", "/remote/notes.txt", "/remote/diary.txt"));

    expect(await screen.findByText("Renamed to diary.txt.")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    await waitFor(() => expect(api.rename).toHaveBeenLastCalledWith("edge", "/remote/diary.txt", "/remote/notes.txt"));
    expect(screen.queryByText("Renamed to diary.txt.")).not.toBeInTheDocument();
  });

  it("says an empty directory is empty rather than showing a bare table", async () => {
    api.list.mockResolvedValue({ path: "/", entries: [] });
    render(<SFTPPanel aliases={["edge"]} />);

    expect(await screen.findByText("Pick a saved SSH host to browse its files.")).toBeVisible();
    await chooseHost("edge");

    expect(await screen.findByText("This directory is empty.")).toBeVisible();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });

  it("explains a failed listing where the rows would be and retries from there", async () => {
    api.list.mockRejectedValueOnce(new ApiError("sftp_failed", 502, null));
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");

    const failure = await screen.findByRole("alert");
    expect(failure).toHaveTextContent("Could not connect.");
    expect(screen.getAllByRole("alert")).toHaveLength(1);

    api.list.mockResolvedValue({ path: "/remote", entries: [] });
    await userEvent.click(within(failure).getByRole("button", { name: "Try again" }));
    expect(await screen.findByText("This directory is empty.")).toBeVisible();
  });

  it("searches below the open directory and shows where each match lives", async () => {
    api.list.mockResolvedValue({
      path: "/srv",
      entries: [{ name: "app", path: "/srv/app", type: "directory", size: 0, mode: "0755", modifiedAt: "", revision: "dir" }],
    });
    api.search.mockResolvedValue({
      path: "/srv",
      query: "log",
      truncated: false,
      entries: [
        { name: "report.log", path: "/srv/app/report.log", type: "file", size: 3, mode: "0644", modifiedAt: "", revision: "one" },
        { name: "other.log", path: "/srv/app/logs/other.log", type: "file", size: 5, mode: "0644", modifiedAt: "", revision: "two" },
      ],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await screen.findByRole("button", { name: "app" });

    await userEvent.type(screen.getByRole("searchbox", { name: "Filter remote entries" }), "log{Enter}");
    await waitFor(() => expect(api.search).toHaveBeenCalledWith("edge", "/srv", "log"));

    expect(await screen.findByText("2 matches for “log” under /srv")).toBeVisible();
    const table = screen.getByRole("table");
    expect(within(table).getByRole("button", { name: "report.log" })).toBeVisible();
    // Each row says which directory it came from, since they differ.
    expect(within(table).getByText("/srv/app/logs")).toBeVisible();
    expect(screen.queryByRole("button", { name: "app" })).not.toBeInTheDocument();

    await userEvent.click(within(table).getByRole("button", { name: "other.log" }));
    await userEvent.click(screen.getByRole("button", { name: "Actions for other.log" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Go to containing folder" }));
    await waitFor(() => expect(api.list).toHaveBeenCalledWith("edge", "/srv/app/logs"));
    expect(screen.queryByText(/matches for/)).not.toBeInTheDocument();
  });

  it("renames a search result inside its own directory and stays in the results", async () => {
    api.list.mockResolvedValue({ path: "/srv", entries: [] });
    api.search.mockResolvedValue({
      path: "/srv",
      query: "log",
      truncated: false,
      entries: [{ name: "report.log", path: "/srv/app/report.log", type: "file", size: 3, mode: "0644", modifiedAt: "", revision: "one" }],
    });
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.type(screen.getByRole("searchbox", { name: "Filter remote entries" }), "log{Enter}");
    await userEvent.click(await screen.findByRole("button", { name: "report.log" }));

    await userEvent.click(screen.getByRole("button", { name: "Actions for report.log" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Rename" }));
    const dialog = screen.getByRole("dialog", { name: "Rename" });
    await userEvent.clear(within(dialog).getByRole("textbox", { name: "New name" }));
    await userEvent.type(within(dialog).getByRole("textbox", { name: "New name" }), "old.log");
    await userEvent.click(within(dialog).getByRole("button", { name: "Rename" }));

    // The entry's own directory, not the one the panel happens to show.
    await waitFor(() => expect(api.rename).toHaveBeenCalledWith("edge", "/srv/app/report.log", "/srv/app/old.log"));
    await waitFor(() => expect(api.search).toHaveBeenCalledTimes(2));
    expect(screen.getByText(/matches for/)).toBeVisible();
  });

  it("downloads folders as archives and changes permissions with the current revision", async () => {
    api.list.mockResolvedValue({
      path: "/remote",
      entries: [{ name: "project", path: "/remote/project", type: "directory", size: 0, mode: "drwxr-x---", modifiedAt: "2026-08-24T10:00:00Z", revision: "rev" }],
    });
    api.chmod.mockResolvedValue(undefined);
    render(<SFTPPanel aliases={["edge"]} />);
    await chooseHost("edge");
    await userEvent.click(await screen.findByRole("button", { name: "project" }));

    await userEvent.click(screen.getByRole("button", { name: "Actions for project" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Download" }));
    await waitFor(() => expect(api.streamDownload).toHaveBeenCalledWith("edge", expect.any(String), "/remote/project", true, 0, expect.objectContaining({ signal: expect.any(AbortSignal) })));
    await userEvent.click(screen.getByRole("button", { name: "Actions for project" }));
    await userEvent.click(screen.getByRole("menuitem", { name: "Change permissions" }));
    const dialog = screen.getByRole("dialog", { name: "Change permissions" });
    expect(within(dialog).getByRole("textbox", { name: "Permissions (octal, for example 640)" })).toHaveValue("750");
    await userEvent.click(within(dialog).getByRole("button", { name: "Change permissions" }));
    await waitFor(() => expect(api.chmod).toHaveBeenCalledWith("edge", "/remote/project", "750", "rev"));
  });
});
