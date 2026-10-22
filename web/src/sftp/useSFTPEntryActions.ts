import { useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { localMutationProblemText, sftpProblemText } from "./sftpProblemText";
import { clipboard } from "../ui/clipboard";
import { useToast } from "../ui/useToast";
import { sftpApi, type RemoteEntry } from "./api";
import { parentRowKey, type SFTPEntryListModel } from "./useSFTPEntryList";
import { remoteJoin as join, remoteParentOf as parentOf, type SFTPSource } from "./sftpSource";
import { sftpTransferManager } from "./transferManager";
import { startsAtHome } from "./remotePathInput";
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
  const showToast = useToast();
  const { alias, path, source, setProblem } = browser;
  const generation = browser.generation;
  const { selectedEntries, selectedEntry, rowKeys, setSelectedPaths, focusAfterReload, cancelPendingFocus } = list;
  const [acting, setActing] = useState(false);
  const [inputIntent, setInputIntent] = useState<SFTPInputIntent | null>(null);
  // Why the change asked for in the name dialog did not go through. It is shown
  // in the dialog, which stays open so that the name can be fixed and sent again.
  const [inputProblem, setInputProblem] = useState("");
  const [deleteIntent, setDeleteIntent] = useState<{
    entries: RemoteEntry[]; source: SFTPSource; path: string; isCurrent: () => boolean;
  } | null>(null);
  const deleting = deleteIntent?.entries ?? null;
  const [deleteProblem, setDeleteProblem] = useState("");
  // What the last change was, and how to put it back.
  const [undo, setUndo] = useState<{ label: string; run: () => Promise<void> } | null>(null);

  function problemText(error: unknown, fallback: MessageKey = "sftp.problem.failed"): string {
    return source?.local ? localMutationProblemText(t, error, fallback) : sftpProblemText(t, error, fallback);
  }

  function report(error: unknown, fallback: MessageKey = "sftp.problem.failed") {
    setProblem(problemText(error, fallback));
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

  // Makes a change asked for in the name dialog. The dialog waits while the
  // change is made and shows why it failed. Once the change is made the dialog
  // closes, and afterChange brings the pane up to date; a problem there is the
  // pane's, since the change itself went through.
  async function changeFromDialog<T>({ change, afterChange }: {
    change: () => Promise<T>;
    afterChange: (result: T) => Promise<void> | void;
  }) {
    setActing(true);
    setInputProblem("");
    setProblem("");
    let result: T;
    try {
      result = await change();
    } catch (error) {
      setInputProblem(problemText(error));
      setActing(false);
      return;
    }
    setInputIntent(null);
    try {
      await afterChange(result);
    } catch (error) {
      report(error);
    } finally {
      setActing(false);
    }
  }

  function makeDirectory(target: SFTPSource, name: string) {
    const isCurrent = generation.observe();
    const targetAlias = alias;
    const targetPath = path;
    return changeFromDialog({
      change: () => target.mkdir(targetPath, name),
      afterChange: async () => {
        if (!isCurrent()) return;
        focusAfterReload(target.join(targetPath, name));
        await browser.load(targetPath, { alias: targetAlias });
      },
    });
  }

  function makeEmptyFile(name: string) {
    const isCurrent = generation.observe();
    const targetAlias = alias;
    const targetPath = path;
    const createdPath = join(targetPath, name);
    return changeFromDialog({
      change: () => sftpApi.createEmptyFile(targetAlias, createdPath),
      afterChange: async () => {
        if (!isCurrent()) return;
        focusAfterReload(createdPath);
        await browser.load(targetPath, { alias: targetAlias });
      },
    });
  }

  function rename(target: SFTPSource, entry: RemoteEntry, name: string) {
    const isCurrent = generation.observe();
    const targetAlias = alias;
    const targetPath = target.parentOf(entry.path);
    const renamed = target.join(targetPath, name);
    return changeFromDialog({
      change: () => target.rename(entry, name),
      afterChange: async (updated) => {
        if (!isCurrent()) return;
        focusAfterReload(renamed);
        await refreshAfterChange(targetPath, targetAlias);
        offerUndo(t("sftp.renamedTo", { name }), async () => {
          await target.rename({ ...entry, path: renamed, name, revision: updated?.revision ?? entry.revision }, entry.name);
          focusAfterReload(entry.path);
          await refreshAfterChange(targetPath, targetAlias);
        });
      },
    });
  }

  function queuedText(entries: RemoteEntry[], operation: "copy" | "move"): string {
    const name = entries[0]?.name ?? "";
    if (operation === "copy") return t("sftp.duplicateQueued", { name });
    return entries.length === 1 ? t("sftp.moveQueued", { name }) : t("sftp.moveQueuedCount", { count: entries.length });
  }

  // A copy or a move runs as a job in the transfer manager. Once it is queued
  // the dialog has closed and the rows have not changed yet, so a toast says
  // that the job was taken.
  function queueRemoteOperation(entries: RemoteEntry[], operation: "copy" | "move", destination: (entry: RemoteEntry) => string) {
    return changeFromDialog({
      change: () => sftpTransferManager.addRemoteTransfers(entries.map((entry) => ({
        sourceAlias: alias,
        sourcePath: entry.path,
        targetAlias: alias,
        targetPath: destination(entry),
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name,
        totalBytes: entry.type === "file" ? entry.size : -1,
      })), operation),
      afterChange: () => {
        setSelectedPaths(new Set());
        showToast(queuedText(entries, operation), { tone: "success" });
      },
    });
  }

  // A destination typed from ~ is listed first: the listing answers with the
  // absolute path a transfer needs, and proves the directory is there.
  async function moveEntriesTo(target: SFTPSource, entries: RemoteEntry[], typed: string) {
    let directory = typed;
    if (startsAtHome(typed)) {
      setInputProblem("");
      try {
        directory = (await target.list(typed)).path;
      } catch (error) {
        setInputProblem(failureCode(error) === "invalid_request" ? t("sftp.pathCannotOpen") : problemText(error));
        return;
      }
    }
    await queueRemoteOperation(entries, "move", (entry) => join(directory, entry.name));
  }

  // Whether the pane's source still takes the change the dialog asks for. The
  // menus offer only the changes it takes; this keeps a dialog left open from
  // acting on a source that has changed since.
  function sourceTakes(intent: SFTPInputIntent): boolean {
    const can = source?.can;
    if (can === undefined) return false;
    switch (intent.kind) {
      case "mkdir": return can.createDirectory;
      case "createFile":
      case "duplicate": return can.createEntries;
      case "rename": return can.rename;
      case "moveTo": return can.moveEntries;
    }
  }

  // What an answered name dialog runs. The dialog waits for the returned
  // promise, so it stays open until the change is made or has failed.
  async function submit(value: string) {
    const intent = inputIntent;
    if (intent === null) return;
    if (source === null || !sourceTakes(intent)) {
      setInputIntent(null);
      return;
    }
    if (intent.kind === "mkdir") await makeDirectory(source, value);
    else if (intent.kind === "createFile") await makeEmptyFile(value);
    else if (intent.kind === "rename") await rename(source, intent.entry, value);
    else if (intent.kind === "duplicate") await queueRemoteOperation([intent.entry], "copy", () => join(parentOf(intent.entry.path), value));
    else await moveEntriesTo(source, intent.entries, value);
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

  function ask(intent: SFTPInputIntent) {
    onInteract?.();
    setInputProblem("");
    setInputIntent(intent);
  }

  function renameSelection() {
    if (selectedEntry === null || acting) return;
    ask({ kind: "rename", entry: selectedEntry });
  }

  async function copySelected(kind: "name" | "path") {
    onInteract?.();
    try {
      await clipboard.writeText(selectedEntries.map((entry) => kind === "name" ? entry.name : entry.path).join("\n"));
      showToast(t("copy.done"), { tone: "success" });
    } catch {
      setProblem(t("copy.refused"));
    }
  }

  async function copyCurrentPath() {
    try {
      await clipboard.writeText(path);
      setProblem("");
      showToast(t("copy.done"), { tone: "success" });
    } catch { setProblem(t("copy.refused")); }
  }

  return {
    acting,
    undo,
    dismissUndo: () => setUndo(null),
    offerUndo,
    inputIntent,
    inputProblem,
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
    submit,
    remove,
  };
}

export type SFTPEntryActionsModel = ReturnType<typeof useSFTPEntryActions>;
