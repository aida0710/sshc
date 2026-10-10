import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider } from "../i18n/context";
import { ApiError } from "../api/client";
import type { RemoteEntry } from "./api";
import { SFTPPanel } from "./SFTPPanel";
import { localHostAlias } from "./localHost";
import { sftpTransferManager } from "./transferManager";

const api = vi.hoisted(() => ({ listLocal: vi.fn(), listTransfers: vi.fn() }));
const localMutations = vi.hoisted(() => ({ mkdir: vi.fn(), rename: vi.fn(), remove: vi.fn() }));
vi.mock("./api", () => ({ sftpApi: api }));
vi.mock("./localMutationApi", () => ({ localMutationApi: localMutations }));
vi.mock("../api/recentConnections", () => ({ recentConnectionsApi: { recentConnections: vi.fn(async () => ({ connections: [] })) } }));

const notes: RemoteEntry = { name: "notes.txt", path: "C:/Users/engine/notes.txt", type: "file", size: 4, mode: "0600", modifiedAt: "2026-10-07T01:00:00Z", revision: "notes-revision" };
const link: RemoteEntry = { ...notes, name: "shortcut", path: "C:/Users/engine/shortcut", type: "symlink", revision: "link-revision", linkTarget: "C:/outside/keep.txt", targetType: "file" };

async function openLocalPanel(language: "en" | "ja" = "en") {
  const queueOpened = vi.fn();
  render(<LanguageProvider initial={language}><SFTPPanel aliases={[]} initialLocation={{ alias: localHostAlias, path: "C:/Users/engine" }} showTransfers={false} onQueueOpen={queueOpened} /></LanguageProvider>);
  await screen.findByRole("button", { name: "notes.txt" });
  return queueOpened;
}

async function askLocalDelete(selectionLabel = "Actions for notes.txt", deleteLabel = "Delete") {
  await userEvent.click(screen.getByRole("button", { name: selectionLabel }));
  await userEvent.click(within(screen.getByRole("menu", { name: selectionLabel })).getByRole("menuitem", { name: deleteLabel }));
}

describe("local entry actions through the shared pane", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    api.listLocal.mockResolvedValue({ path: "C:/Users/engine", home: "C:/Users/engine", entries: [notes, link] });
    localMutations.remove.mockResolvedValue(undefined);
  });

  it("offers local editing, details, folder creation, rename and deletion without chmod", async () => {
    await openLocalPanel();
    await userEvent.click(screen.getByRole("button", { name: "Create or upload" }));
    expect(screen.getByRole("menuitem", { name: "New folder" })).toBeVisible();
    expect(screen.queryByRole("menuitem", { name: "New empty file" })).toBeNull();
    await userEvent.click(screen.getByRole("menuitem", { name: "New folder" }));
    const dialog = screen.getByRole("dialog", { name: "New folder" });
    fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: "documents" } });
    localMutations.mkdir.mockResolvedValue({ ...notes, name: "documents", path: "C:/Users/engine/documents", type: "directory" });
    await userEvent.click(within(dialog).getByRole("button", { name: "New folder" }));
    await waitFor(() => expect(localMutations.mkdir).toHaveBeenCalledWith("C:/Users/engine", "documents"));
    await userEvent.click(screen.getByRole("button", { name: "notes.txt" }));
    await userEvent.click(screen.getByRole("button", { name: "Actions for notes.txt" }));
    const menu = screen.getByRole("menu", { name: "Actions for notes.txt" });
    expect(within(menu).getByRole("menuitem", { name: "Rename" })).toBeVisible();
    expect(within(menu).getByRole("menuitem", { name: "Delete" })).toBeVisible();
    expect(within(menu).getByRole("menuitem", { name: "Edit file" })).toBeVisible();
    expect(within(menu).getByRole("menuitem", { name: "Details" })).toBeVisible();
    expect(within(menu).queryByRole("menuitem", { name: /permission|duplicate|move/i })).toBeNull();
  });

  it("renames Windows paths through the local source and uses the returned revision for undo", async () => {
    await openLocalPanel();
    await userEvent.click(screen.getByRole("button", { name: "notes.txt" }));
    await userEvent.click(screen.getByRole("button", { name: "Rename" }));
    const dialog = screen.getByRole("dialog", { name: "Rename" });
    fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: "renamed.txt" } });
    const renamed = { ...notes, name: "renamed.txt", path: "C:/Users/engine/renamed.txt", revision: "renamed-revision" };
    localMutations.rename.mockResolvedValue(renamed);
    api.listLocal.mockResolvedValue({ path: "C:/Users/engine", home: "C:/Users/engine", entries: [renamed, link] });
    await userEvent.click(within(dialog).getByRole("button", { name: "Rename" }));
    await screen.findByRole("button", { name: "Undo" });
    expect(localMutations.rename).toHaveBeenCalledWith(notes, "renamed.txt");
    await userEvent.click(screen.getByRole("button", { name: "Undo" }));
    await waitFor(() => expect(localMutations.rename).toHaveBeenLastCalledWith(renamed, "notes.txt"));
  });

  it.each(["en", "ja"] as const)("identifies the engine filesystem in the %s confirmation and cancels without mutations", async (language) => {
    await openLocalPanel(language);
    await userEvent.click(screen.getByRole("button", { name: "notes.txt" }));
    await askLocalDelete(language === "ja" ? "notes.txtの操作" : "Actions for notes.txt", language === "ja" ? "削除" : "Delete");
    const dialog = screen.getByRole("dialog", { name: language === "ja" ? "このローカル項目を削除しますか？" : "Delete this local entry?" });
    expect(dialog).toHaveTextContent(notes.path);
    expect(dialog).toHaveTextContent(language === "ja" ? "sshcエンジンが動いているマシン" : "machine running the sshc engine");
    await userEvent.click(within(dialog).getByRole("button", { name: language === "ja" ? "キャンセル" : "Cancel" }));
    expect(localMutations.remove).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("row", { name: /notes.txt/ })).toHaveAttribute("aria-selected", "true");
  });

  it("deletes multiple local entries including a link and reloads without opening the remote queue", async () => {
    const queueOpened = await openLocalPanel();
    const remoteTransfers = vi.spyOn(sftpTransferManager, "addRemoteTransfers");
    await userEvent.click(screen.getByRole("button", { name: "notes.txt" }));
    fireEvent.click(screen.getByRole("button", { name: "shortcut" }), { ctrlKey: true });
    await askLocalDelete("Actions for 2 selected items");
    const dialog = screen.getByRole("dialog", { name: "Delete 2 local entries?" });
    expect(dialog).toHaveTextContent(link.path);
    api.listLocal.mockResolvedValue({ path: "C:/Users/engine", home: "C:/Users/engine", entries: [] });
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(localMutations.remove).toHaveBeenCalledWith([notes, link]);
    expect(remoteTransfers).not.toHaveBeenCalled();
    expect(queueOpened).not.toHaveBeenCalled();
    expect(screen.queryByRole("button", { name: "notes.txt" })).toBeNull();
  });

  it("keeps a refused deletion open and describes the local conflict", async () => {
    await openLocalPanel();
    localMutations.remove.mockRejectedValue(new ApiError("sftp_conflict", 409, null));
    await userEvent.click(screen.getByRole("button", { name: "notes.txt" }));
    await askLocalDelete();
    const dialog = screen.getByRole("dialog", { name: "Delete this local entry?" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    expect(await within(dialog).findByRole("alert")).toHaveTextContent("A local entry changed or is in use by a transfer.");
    expect(within(dialog).getByRole("button", { name: "Cancel" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "notes.txt" })).toBeInTheDocument();
  });

  it("rejects path traversal in the shared name dialog before contacting the API", async () => {
    await openLocalPanel();
    await userEvent.click(screen.getByRole("button", { name: "notes.txt" }));
    await userEvent.click(screen.getByRole("button", { name: "Rename" }));
    const dialog = screen.getByRole("dialog", { name: "Rename" });
    for (const name of ["..", ".", "../outside", "child\\outside"]) {
      fireEvent.change(within(dialog).getByRole("textbox"), { target: { value: name } });
      await userEvent.click(within(dialog).getByRole("button", { name: "Rename" }));
      expect(localMutations.rename).not.toHaveBeenCalled();
    }
  });
});
