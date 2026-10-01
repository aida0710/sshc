import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { sftpProblemText } from "./sftpProblemText";
import type { BrowserLocation, NavigationBlocker } from "../routing/useSectionRoute";
import { useUnsavedDraftGuard } from "../routing/useUnsavedDraftGuard";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import { sftpApi, type RemoteEntry, type RemoteTextFile } from "./api";

export type SFTPTextSource = {
  readText(alias: string, path: string): Promise<RemoteTextFile>;
  saveText(alias: string, path: string, contents: string, expectedRevision: string): Promise<RemoteTextFile>;
};

type OpenedText = { alias: string; file: RemoteTextFile };

// A problem shown inside the editor, over which the pane's banner would be
// hidden. `conflict` says the remote file is no longer the revision the
// editor read, so a save is refused until the user either reads the remote
// file again or overwrites it.
type EditorProblem = { message: string; conflict: boolean };

// What the confirmation over the editor is asking before it drops unsaved
// changes: closing the editor, or replacing them with the remote file.
type DiscardConfirmation = "close" | "reload";

// A write of the editor's contents: a save over the revision the editor read,
// or an overwrite of the revision read when the user asked to overwrite.
// When the engine refuses either as a conflict, the message says which one.
type WriteKind = "save" | "overwrite";

const conflictMessageKeys: Record<WriteKind, MessageKey> = {
  save: "sftp.editorConflict",
  overwrite: "sftp.editorOverwriteConflict",
};

// The overwrite the user is asked to confirm. `revision` is the remote
// revision it would replace, read when the confirmation opened, and is null
// until that read answers. A change made after the read makes the confirmed
// overwrite a conflict again instead of being lost unseen.
type OverwriteConfirmation = { revision: string | null };

// The text editor is one modal over the file list: it knows how to read,
// edit and save a file with its revision, and how to keep the user from
// losing an unsaved change by leaving. It does not know what a directory is;
// the pane tells it what to refresh after a save.
export function useSFTPTextEditor({
  source = sftpApi,
  onProblem,
  onSaved,
  onNavigationBlockerChange,
  onDirtyChange,
  onNavigateLocation,
}: {
  source?: SFTPTextSource;
  // Shown in the pane's banner when a file cannot be opened. An empty string
  // clears it. Problems while a file is open are shown in the editor.
  onProblem: (message: string) => void;
  // Runs after the file is written. The editor already edits the saved
  // revision, whether or not this refresh succeeds.
  onSaved: (alias: string, saved: RemoteTextFile) => Promise<void>;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onDirtyChange?: ((path: string | null) => void) | undefined;
  onNavigateLocation?: ((url: string) => void) | undefined;
}) {
  const t = useTranslate();
  const [opened, setOpened] = useState<OpenedText | null>(null);
  const [contents, setContents] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<EditorProblem | null>(null);
  const [confirming, setConfirming] = useState<DiscardConfirmation | null>(null);
  const [overwriteConfirmation, setOverwriteConfirmation] = useState<OverwriteConfirmation | null>(null);
  const [leaving, setLeaving] = useState<BrowserLocation | null>(null);
  // Retired whenever the editor is closed or reset, or an overwrite
  // confirmation is cancelled. A read or a save that was still in flight then
  // cannot reopen a file the user has left, and the revision read for a
  // cancelled overwrite is dropped.
  const fileGeneration = useRequestGeneration();
  const dirty = opened !== null && contents !== opened.file.contents;

  useEffect(() => {
    onDirtyChange?.(dirty ? opened?.file.entry.path ?? "" : null);
    return () => onDirtyChange?.(null);
  }, [dirty, onDirtyChange, opened?.file.entry.path]);

  const leaveEditorBlocker = useCallback<NavigationBlocker>((next) => {
    setLeaving(next);
    return false;
  }, []);
  useUnsavedDraftGuard({ dirty, blocker: leaveEditorBlocker, onNavigationBlockerChange });

  function close() {
    fileGeneration.retire();
    setOpened(null);
    setContents("");
    setBusy(false);
    setProblem(null);
    setConfirming(null);
    setOverwriteConfirmation(null);
    setLeaving(null);
  }

  function readProblemText(error: unknown): string {
    const code = failureCode(error);
    if (code === "sftp_not_utf8") return t("sftp.binaryHint");
    if (code === "sftp_text_too_large") return t("sftp.tooLargeHint");
    return sftpProblemText(t, error);
  }

  // Reads a file into the editor, replacing what it shows.
  async function read(alias: string, path: string, reportFailure: (message: string) => void) {
    const isCurrent = fileGeneration.begin();
    setBusy(true);
    try {
      const file = await source.readText(alias, path);
      if (!isCurrent()) return;
      setOpened({ alias, file });
      setContents(file.contents);
      setProblem(null);
    } catch (error) {
      if (!isCurrent()) return;
      reportFailure(readProblemText(error));
    } finally {
      if (isCurrent()) setBusy(false);
    }
  }

  async function open(alias: string, entry: RemoteEntry) {
    if (dirty) {
      setProblem({ message: t("sftp.unsavedBlocked"), conflict: false });
      return;
    }
    onProblem("");
    // With a file already open, the pane's banner would sit behind the editor.
    await read(alias, entry.path, opened === null ? onProblem : (message) => setProblem({ message, conflict: false }));
  }

  async function reload() {
    setConfirming(null);
    if (opened === null) return;
    await read(opened.alias, opened.file.entry.path, (message) => setProblem({ message, conflict: true }));
  }

  // Writes the editor's contents. The engine refuses the write when the
  // remote file is no longer `expectedRevision`.
  async function writeContents(kind: WriteKind, expectedRevision: string) {
    if (opened === null) return;
    const isCurrent = fileGeneration.observe();
    const { alias, file } = opened;
    setBusy(true);
    setProblem(null);
    try {
      const saved = await source.saveText(alias, file.entry.path, contents, expectedRevision);
      if (!isCurrent()) return;
      // The server holds these contents now, so the next save must send
      // this revision even if refreshing the listing fails.
      setOpened({ alias, file: saved });
      setContents(saved.contents);
      await onSaved(alias, saved);
    } catch (error) {
      if (!isCurrent()) return;
      const refusedAsConflict = failureCode(error) === "sftp_conflict";
      setProblem({
        message: refusedAsConflict ? t(conflictMessageKeys[kind]) : sftpProblemText(t, error),
        // An overwrite starts from a conflict. When its write fails for another
        // reason, the editor still holds the revision the remote file has
        // moved on from, so Reload and Overwrite stay offered rather than
        // leaving only a Save that would be refused as a conflict.
        conflict: refusedAsConflict || kind === "overwrite",
      });
    } finally {
      if (isCurrent()) setBusy(false);
    }
  }

  async function save() {
    if (opened === null) return;
    await writeContents("save", opened.file.revision);
  }

  // Opens the confirmation and reads the remote file again to learn the
  // revision it has now. The confirmation opens before the read, while the
  // clicked button inside the editor still has the focus, so it stacks over
  // the editor. A button disabled for the read would drop the focus, and a
  // dialog opened after that would replace the editor instead. The editor's
  // contents stay as they are.
  async function requestOverwrite() {
    if (opened === null) return;
    const isCurrent = fileGeneration.begin();
    const { alias, file } = opened;
    setOverwriteConfirmation({ revision: null });
    try {
      const remote = await source.readText(alias, file.entry.path);
      if (!isCurrent()) return;
      setOverwriteConfirmation({ revision: remote.revision });
    } catch (error) {
      if (!isCurrent()) return;
      setOverwriteConfirmation(null);
      setProblem({ message: readProblemText(error), conflict: true });
    }
  }

  function cancelOverwrite() {
    // Drops the read still in flight: its answer belongs to the confirmation
    // turned down.
    fileGeneration.retire();
    setOverwriteConfirmation(null);
  }

  // The confirmation closes before the write starts, so a refused or failed
  // overwrite is shown in the editor rather than behind the dialog.
  async function overwrite() {
    const revision = overwriteConfirmation?.revision ?? null;
    if (revision === null) return;
    setOverwriteConfirmation(null);
    await writeContents("overwrite", revision);
  }

  // Escape, Android back and the close button all land here. Unsaved changes
  // are dropped only after the user confirms it.
  function dismiss() {
    if (dirty) setConfirming("close");
    else close();
  }

  // Reloading replaces the editor's contents, so unsaved changes are
  // confirmed first, as closing does.
  function requestReload() {
    if (dirty) setConfirming("reload");
    else void reload();
  }

  function discardAndLeave() {
    if (leaving === null) return;
    const destination = leaving;
    close();
    onNavigationBlockerChange?.(null);
    onNavigateLocation?.(`${destination.pathname}${destination.search}`);
  }

  return {
    opened: opened?.file ?? null,
    contents,
    setContents,
    dirty,
    busy,
    problem,
    confirming,
    confirmingOverwrite: overwriteConfirmation !== null,
    readingOverwriteRevision: overwriteConfirmation !== null && overwriteConfirmation.revision === null,
    leaving,
    open,
    save,
    close,
    dismiss,
    requestReload,
    reload,
    requestOverwrite,
    overwrite,
    cancelOverwrite,
    keepEditing: () => setConfirming(null),
    discardAndLeave,
    stay: () => setLeaving(null),
  };
}

export type SFTPTextEditorModel = ReturnType<typeof useSFTPTextEditor>;
