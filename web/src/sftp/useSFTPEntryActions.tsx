import { useId, useState, type RefObject } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { clipboard } from "../ui/clipboard";
import { InputDialog } from "../ui/InputDialog";
import { sftpApi, type RemoteEntry } from "./api";
import { parentRowKey, type SFTPEntryListModel } from "./SFTPEntryList";
import { remoteJoin as join, remoteParentOf as parentOf } from "./sftpSource";
import { sftpTransferManager } from "./transferManager";
import { symbolicModeToOctal } from "./transfers";
import type { SFTPBrowserModel } from "./useSFTPBrowser";

type SFTPInputIntent =
  | { kind: "mkdir" }
  | { kind: "createFile" }
  | { kind: "duplicate"; entry: RemoteEntry }
  | { kind: "moveTo"; entries: RemoteEntry[] }
  | { kind: "rename"; entry: RemoteEntry }
  | { kind: "chmod"; entry: RemoteEntry; recursive: boolean };

// Everything that changes entries on a host: creating, renaming, moving,
// copying, changing modes and deleting, with the dialogs that ask for a name
// or a confirmation, and the one-step undo for the changes that have one.
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
  list: SFTPEntryListModel;
  // Re-reads whatever the rows currently show, which is the search results
  // rather than a directory while a search is open.
  refreshAfterChange: (directory: string, alias: string) => Promise<unknown>;
  onQueueOpen: () => void;
  // Called before a dialog opens, so that an open menu can close.
  onInteract?: () => void;
}) {
  const t = useTranslate();
  const { alias, path, setProblem } = browser;
  const generation = browser.generation;
  const { selectedEntries, selectedEntry, rowKeys, pendingFocus, setSelectedPaths } = list;
  const [acting, setActing] = useState(false);
  const [inputIntent, setInputIntent] = useState<SFTPInputIntent | null>(null);
  const [deleting, setDeleting] = useState<RemoteEntry[] | null>(null);
  // What the last change was, and how to put it back.
  const [undo, setUndo] = useState<{ label: string; run: () => Promise<void> } | null>(null);

  function report(error: unknown, fallback = "sftp_failed") {
    setProblem(failureCode(error) || (error instanceof Error ? error.message : fallback));
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
    const current = generation.current;
    const targetAlias = alias;
    const targetPath = path;
    setActing(true);
    try {
      await sftpApi.mkdir(targetAlias, join(targetPath, name));
      if (current !== generation.current) return;
      pendingFocus.current = join(targetPath, name);
      await browser.load(targetPath, { alias: targetAlias });
    } catch (error) {
      if (current !== generation.current) return;
      report(error);
    } finally {
      setActing(false);
    }
  }

  async function makeEmptyFile(name: string) {
    const current = generation.current;
    const targetAlias = alias;
    const targetPath = path;
    const createdPath = join(targetPath, name);
    setActing(true);
    setProblem("");
    try {
      await sftpApi.createEmptyFile(targetAlias, createdPath);
      if (current !== generation.current) return;
      pendingFocus.current = createdPath;
      await browser.load(targetPath, { alias: targetAlias });
    } catch (error) {
      if (current !== generation.current) return;
      report(error);
    } finally {
      setActing(false);
    }
  }

  async function rename(entry: RemoteEntry, name: string) {
    const current = generation.current;
    const targetAlias = alias;
    const targetPath = parentOf(entry.path);
    const renamed = join(targetPath, name);
    setActing(true);
    try {
      await sftpApi.rename(targetAlias, entry.path, renamed);
      if (current !== generation.current) return;
      pendingFocus.current = renamed;
      await refreshAfterChange(targetPath, targetAlias);
      offerUndo(t("sftp.renamedTo", { name }), async () => {
        await sftpApi.rename(targetAlias, renamed, entry.path);
        pendingFocus.current = entry.path;
        await refreshAfterChange(targetPath, targetAlias);
      });
    } catch (error) {
      if (current !== generation.current) return;
      report(error);
    } finally {
      setActing(false);
    }
  }

  async function chmod(entry: RemoteEntry, mode: string, recursive: boolean) {
    if (entry.type === "symlink" || entry.type === "other") return;
    const current = generation.current;
    const targetAlias = alias;
    const targetPath = path;
    const previous = symbolicModeToOctal(entry.mode);
    setActing(true);
    try {
      await sftpApi.chmod(targetAlias, entry.path, mode, entry.revision, recursive);
      if (current !== generation.current) return;
      const reloaded = await browser.load(targetPath, { alias: targetAlias, refresh: true });
      const now = reloaded?.find((candidate) => candidate.path === entry.path);
      if (!recursive && now !== undefined && previous !== mode) {
        offerUndo(t("sftp.permissionsChanged", { mode }), async () => {
          await sftpApi.chmod(targetAlias, entry.path, previous, now.revision, false);
          await browser.load(targetPath, { alias: targetAlias, refresh: true });
        });
      }
    } catch (error) {
      if (current !== generation.current) return;
      setProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
    } finally {
      setActing(false);
    }
  }

  async function queueRemoteOperation(entries: RemoteEntry[], operation: "copy" | "move", destination: (entry: RemoteEntry) => string) {
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

  async function remove() {
    if (deleting === null) return;
    const existingJobIds = new Set(sftpTransferManager.getSnapshot().map((job) => job.id));
    const queued = () => {
      setDeleting(null);
      setSelectedPaths(new Set());
      onQueueOpen();
    };
    setActing(true);
    setProblem("");
    try {
      await sftpTransferManager.addRemoteTransfers(deleting.map((entry) => ({
        sourceAlias: alias, sourcePath: entry.path,
        targetAlias: alias, targetPath: entry.path,
        kind: entry.type === "directory" ? "folder" : "file",
        name: entry.name, totalBytes: -1,
      })), "delete");
      queued();
    } catch (error) {
      if (sftpTransferManager.getSnapshot().some((job) => !existingJobIds.has(job.id) && job.operation === "delete")) queued();
      report(error, "delete_failed");
    } finally {
      setActing(false);
    }
  }

  function deleteSelection() {
    if (selectedEntries.length === 0 || acting) return;
    // Focus survives the reload by moving to whatever takes the topmost
    // removed row's place.
    const removed = new Set(selectedEntries.map((entry) => entry.path));
    const survivor = rowKeys.slice(rowKeys.findIndex((key) => removed.has(key)) + 1).find((key) => !removed.has(key));
    pendingFocus.current = survivor ?? rowKeys.filter((key) => !removed.has(key)).pop() ?? parentRowKey;
    onInteract?.();
    setDeleting(selectedEntries);
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
    inputIntent,
    deleting,
    ask,
    cancelInput: () => setInputIntent(null),
    cancelDelete: () => { pendingFocus.current = null; setDeleting(null); },
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
      else void chmod(intent.entry, value, intent.recursive);
    },
    remove,
  };
}

export type SFTPEntryActionsModel = ReturnType<typeof useSFTPEntryActions>;

export function SFTPEntryActionDialogs({ actions, currentPath, returnFocusRef }: {
  actions: SFTPEntryActionsModel;
  currentPath: string;
  returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const id = useId();
  const { inputIntent, deleting } = actions;
  return (
    <>
      {deleting === null ? null : (
        <ConfirmDialog
          id={`${id}-delete`}
          heading={deleting.length === 1 ? t("sftp.deleteHeading") : t("sftp.deleteHeadingCount", { count: deleting.length })}
          body={<div className="space-y-2 text-sm text-ink-muted">
            {deleting.some((entry) => entry.type === "directory") ? <p>{t("sftp.deleteContentsWarning")}</p> : null}
            <ul className="max-h-48 space-y-1 overflow-auto">
              {deleting.map((entry) => <li key={entry.path} className="break-all font-mono">{entry.path}</li>)}
            </ul>
          </div>}
          confirmLabel={t("sftp.delete")}
          cancelLabel={t("sftp.cancel")}
          returnFocusRef={returnFocusRef}
          onConfirm={() => void actions.remove()}
          onCancel={actions.cancelDelete}
        />
      )}
      {inputIntent === null ? null : (
        <InputDialog
          id={`${id}-input`}
          heading={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "createFile" ? "sftp.newFile" : inputIntent.kind === "rename" ? "sftp.rename" : inputIntent.kind === "duplicate" ? "sftp.duplicate" : inputIntent.kind === "moveTo" ? "sftp.moveTo" : inputIntent.recursive ? "sftp.chmodRecursive" : "sftp.chmod")}
          label={t(inputIntent.kind === "chmod" ? "sftp.chmodPrompt" : inputIntent.kind === "rename" || inputIntent.kind === "duplicate" ? "sftp.renamePrompt" : inputIntent.kind === "createFile" ? "sftp.newFilePrompt" : inputIntent.kind === "moveTo" ? "sftp.moveToPrompt" : "sftp.mkdirPrompt")}
          initialValue={inputIntent.kind === "mkdir" || inputIntent.kind === "createFile" ? "" : inputIntent.kind === "rename" ? inputIntent.entry.name : inputIntent.kind === "duplicate" ? `${inputIntent.entry.name}.copy` : inputIntent.kind === "moveTo" ? currentPath : symbolicModeToOctal(inputIntent.entry.mode)}
          inputMode={inputIntent.kind === "chmod" ? "numeric" : "text"}
          submitLabel={t(inputIntent.kind === "mkdir" ? "sftp.newFolder" : inputIntent.kind === "createFile" ? "sftp.newFile" : inputIntent.kind === "rename" ? "sftp.rename" : inputIntent.kind === "duplicate" ? "sftp.duplicate" : inputIntent.kind === "moveTo" ? "sftp.move" : "sftp.chmod")}
          cancelLabel={t("sftp.cancel")}
          returnFocusRef={returnFocusRef}
          validate={(value) => {
            if (inputIntent.kind === "chmod") return /^0?[0-7]{3}$/.test(value) ? "" : t("sftp.chmodInvalid");
            if (inputIntent.kind === "moveTo") return value.startsWith("/") ? "" : t("sftp.pathAbsolute");
            if (value === "") return t("sftp.nameRequired");
            if (value.includes("/")) return t("sftp.nameInvalid");
            if (inputIntent.kind === "rename" && value === inputIntent.entry.name) return t("sftp.renameUnchanged");
            if (inputIntent.kind === "duplicate" && value === inputIntent.entry.name) return t("sftp.renameUnchanged");
            return "";
          }}
          onSubmit={actions.submit}
          onCancel={actions.cancelInput}
        />
      )}
    </>
  );
}
