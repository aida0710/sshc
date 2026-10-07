import { useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { localMutationProblemText, sftpProblemText } from "./sftpProblemText";
import { clipboard } from "../ui/clipboard";
import { sftpApi, type RemoteEntry } from "./api";
import { parentRowKey, type SFTPEntryListModel } from "./useSFTPEntryList";
import { remoteJoin as join, remoteParentOf as parentOf, type SFTPSource } from "./sftpSource";
import { sftpTransferManager } from "./transferManager";
import type { SFTPBrowserModel } from "./useSFTPBrowser";

type SFTPInputIntent =
  | { kind: "mkdir" }
  | { kind: "createFile" }
  | { kind: "duplicate"; entry: RemoteEntry }
  | { kind: "moveTo"; entries: RemoteEntry[] }
  | { kind: "rename"; entry: RemoteEntry };

// Everything that changes entries on a host: creating, renaming, moving,
// copying and deleting, with the state of the dialogs that ask
// for a name or a confirmation (drawn by SFTPEntryActionDialogs), and the
// one-step undo for the changes that have one.
// Delete has no undo on purpose: SFTP has no trash, so offering one would be
// a lie.
export function useSFTPEntryActions({
  browser,
  list,
  refreshAfterChange,
  onQueueOpen,
  onInteract,
}: {
  browser: SFTPBrowserModel;
  // Only the selection and the focus request: the list's keyboard and row
  // handling stay its own.
  list: Pick<SFTPEntryListModel,
    "selectedEntries" | "selectedEntry" | "rowKeys" | "setSelectedPaths" | "focusAfterReload" | "cancelPendingFocus">;
  // Re-reads whatever the rows currently show, which is the search results
  // rather than a directory while a search is open.
  refreshAfterChange: (directory: string, alias: string) => Promise<unknown>;
  onQueueOpen: () => void;
  // Called before a dialog opens, so that an open menu can close.
  onInteract?: () => void;
}) {
  const t = useTranslate();
  const { alias, path, source, setProblem } = browser;
  const generation = browser.generation;
  const { selectedEntries, selectedEntry, rowKeys, setSelectedPaths, focusAfterReload, cancelPendingFocus } = list;
  const [acting, setActing] = useState(false);
  const [inputIntent, setInputIntent] = useState<SFTPInputIntent | null>(null);
  const [deleteIntent, setDeleteIntent] = useState<{
    entries: RemoteEntry[]; source: SFTPSource; path: string; isCurrent: () => boolean;
  } | null>(null);
  const deleting = deleteIntent?.entries ?? null;
  const [deleteProblem, setDeleteProblem] = useState("");
  // What the last change was, and how to put it back.
  const [undo, setUndo] = useState<{ label: string; run: () => Promise<void> } | null>(null);

  function report(error: unknown, fallback: MessageKey = "sftp.problem.failed") {
    setProblem(source?.local ? localMutationProblemText(t, error, fallback) : sftpProblemText(t, error, fallback));
  }

  // The offer stands until the next thing happens. A timer would take it away
  // exactly while the user is deciding whether they meant it.
  function offerUndo(label: string, run: () => Promise<void>) {
    setUndo({
      label,
      run: async () => {
        setUndo(null);
        try {
          await run();
        } catch (error) {
          report(error);
        }
      },
    });
  }

  async function makeDirectory(name: string) {
    if (source === null || !source.can.createDirectory) return;
    const isCurrent = generation.observe();
    const targetAlias = alias;
    const targetPath = path;
    setActing(true);
    try {
      await source.mkdir(targetPath, name);
      if (!isCurrent()) return;
      focusAfterReload(source.join(targetPath, name));
      await browser.load(targetPath, { alias: targetAlias });
    } catch (error) {
      if (!isCurrent()) return;
      report(error);
    } finally {
      setActing(false);
    }
  }

  async function makeEmptyFile(name: string) {
    if (!source?.can.createEntries) return;
    const isCurrent = generation.observe();
    const targetAlias = alias;
    const targetPath = path;
    const createdPath = join(targetPath, name);
    setActing(true);
    setProblem("");
    try {
      await sftpApi.createEmptyFile(targetAlias, createdPath);
      if (!isCurrent()) return;
      focusAfterReload(createdPath);
      await browser.load(targetPath, { alias: targetAlias });
    } catch (error) {
      if (!isCurrent()) return;
      report(error);
    } finally {
      setActing(false);
    }
  }

  async function rename(entry: RemoteEntry, name: string) {
    if (source === null || !source.can.rename) return;
    const isCurrent = generation.observe();
    const targetAlias = alias;
    const targetPath = source.parentOf(entry.path);
    const renamed = source.join(targetPath, name);
    setActing(true);
    try {
      const updated = await source.rename(entry, name);
      if (!isCurrent()) return;
      focusAfterReload(renamed);
      await refreshAfterChange(targetPath, targetAlias);
      offerUndo(t("sftp.renamedTo", { name }), async () => {
        await source.rename({ ...entry, path: renamed, name, revision: updated?.revision ?? entry.revision }, entry.name);
        focusAfterReload(entry.path);
        await refreshAfterChange(targetPath, targetAlias);
      });
    } catch (error) {
      if (!isCurrent()) return;
      report(error);
    } finally {
      setActing(false);
    }
  }

  async function queueRemoteOperation(entries: RemoteEntry[], operation: "copy" | "move", destination: (entry: RemoteEntry) => string) {
    if (operation === "move" ? !source?.can.moveEntries : !source?.can.createEntries) return;
    setActing(true);
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(entries.map((entry) => ({
        sourceAlias: alias,
        sourcePath: entry.path,
        targetAlias: alias,
        targetPath: destination(entry),
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name,
        totalBytes: entry.type === "file" ? entry.size : -1,
      })), operation);
      setSelectedPaths(new Set());
    } catch (error) {
      report(error);
    } finally {
      setActing(false);
    }
  }

  // 削除ジョブを積めなかったときは、確認ダイアログを開いたまま、その中に失敗を出す。
  // 一部でも積めたらダイアログを閉じ、失敗は画面に出す。
  async function remove() {
    if (deleteIntent === null || acting || !deleteIntent.isCurrent()) return;
    if (deleteIntent.source.removeEntries !== undefined) {
      await removeImmediate(deleteIntent);
      return;
    }
    const deleting = deleteIntent.entries;
    const targetAlias = deleteIntent.source.alias;
    const existingJobIds = new Set(sftpTransferManager.getSnapshot().map((job) => job.id));
    const queued = () => {
      setDeleteIntent(null);
      setSelectedPaths(new Set());
      onQueueOpen();
    };
    setActing(true);
    setProblem("");
    setDeleteProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(deleting.map((entry) => ({
        sourceAlias: targetAlias, sourcePath: entry.path,
        targetAlias, targetPath: entry.path,
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name, totalBytes: -1,
      })), "delete");
      queued();
    } catch (error) {
      if (sftpTransferManager.getSnapshot().some((job) => !existingJobIds.has(job.id) && job.operation === "delete")) {
        queued();
        report(error, "sftp.problem.deleteFailed");
      } else {
        setDeleteProblem(sftpProblemText(t, error, "sftp.problem.deleteFailed"));
      }
    } finally {
      setActing(false);
    }
  }

  async function removeImmediate(intent: NonNullable<typeof deleteIntent>) {
    setActing(true);
    setProblem("");
    setDeleteProblem("");
    try {
      await intent.source.removeEntries?.(intent.entries);
      if (!intent.isCurrent()) return;
      setDeleteIntent(null);
      setSelectedPaths(new Set());
      await refreshAfterChange(intent.path, intent.source.alias);
    } catch (error) {
      if (intent.isCurrent()) setDeleteProblem(localMutationProblemText(t, error, "sftp.problem.deleteFailed"));
    } finally {
      setActing(false);
    }
  }

  function deleteSelection() {
    if (selectedEntries.length === 0 || acting || source === null || !source.can.delete) return;
    // Focus survives the reload by moving to whatever takes the topmost
    // removed row's place.
    const removed = new Set(selectedEntries.map((entry) => entry.path));
    const survivor = rowKeys.slice(rowKeys.findIndex((key) => removed.has(key)) + 1).find((key) => !removed.has(key));
    focusAfterReload(survivor ?? rowKeys.filter((key) => !removed.has(key)).pop() ?? parentRowKey);
    onInteract?.();
    setDeleteProblem("");
    setDeleteIntent({ entries: selectedEntries, source, path, isCurrent: generation.observe() });
  }

  function renameSelection() {
    if (selectedEntry === null || acting) return;
    onInteract?.();
    setInputIntent({ kind: "rename", entry: selectedEntry });
  }

  function ask(intent: SFTPInputIntent) {
    onInteract?.();
    setInputIntent(intent);
  }

  async function copySelected(kind: "name" | "path") {
    onInteract?.();
    try {
      await clipboard.writeText(selectedEntries.map((entry) => kind === "name" ? entry.name : entry.path).join("\n"));
    } catch {
      setProblem(t("copy.refused"));
    }
  }

  async function copyCurrentPath() {
    try { await clipboard.writeText(path); setProblem(""); }
    catch { setProblem(t("copy.refused")); }
  }

  return {
    acting,
    undo,
    dismissUndo: () => setUndo(null),
    offerUndo,
    inputIntent,
    deleting,
    deletingLocal: deleteIntent?.source.local ?? false,
    deleteProblem,
    ask,
    cancelInput: () => setInputIntent(null),
    cancelDelete: () => { if (acting) return; cancelPendingFocus(); setDeleteIntent(null); },
    deleteSelection,
    renameSelection,
    copySelected,
    copyCurrentPath,
    // What an answered dialog runs.
    submit(value: string) {
      const intent = inputIntent;
      if (intent === null) return;
      setInputIntent(null);
      if (intent.kind === "mkdir") void makeDirectory(value);
      else if (intent.kind === "createFile") void makeEmptyFile(value);
      else if (intent.kind === "rename") void rename(intent.entry, value);
      else if (intent.kind === "duplicate") void queueRemoteOperation([intent.entry], "copy", () => join(parentOf(intent.entry.path), value));
      else if (intent.kind === "moveTo") void queueRemoteOperation(intent.entries, "move", (entry) => join(value, entry.name));
    },
    remove,
  };
}

export type SFTPEntryActionsModel = ReturnType<typeof useSFTPEntryActions>;
