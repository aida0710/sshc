import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
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
// hidden. `reloadable` offers to read the remote file again, after a save
// refused because someone else changed it.
type EditorProblem = { message: string; reloadable: boolean };

// What the confirmation over the editor is asking before it drops unsaved
// changes: closing the editor, or replacing them with the remote file.
type DiscardConfirmation = "close" | "reload";

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
  const [leaving, setLeaving] = useState<BrowserLocation | null>(null);
  // Retired whenever the editor is closed or reset, so that a read or a save
  // that was still in flight cannot reopen a file the user has left.
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
      setProblem({ message: t("sftp.unsavedBlocked"), reloadable: false });
      return;
    }
    onProblem("");
    // With a file already open, the pane's banner would sit behind the editor.
    await read(alias, entry.path, opened === null ? onProblem : (message) => setProblem({ message, reloadable: false }));
  }

  async function reload() {
    setConfirming(null);
    if (opened === null) return;
    await read(opened.alias, opened.file.entry.path, (message) => setProblem({ message, reloadable: true }));
  }

  async function save() {
    if (opened === null) return;
    const isCurrent = fileGeneration.observe();
    const { alias, file } = opened;
    setBusy(true);
    setProblem(null);
    try {
      const saved = await source.saveText(alias, file.entry.path, contents, file.revision);
      if (!isCurrent()) return;
      // The server holds these contents now, so the next save must send
      // this revision even if refreshing the listing fails.
      setOpened({ alias, file: saved });
      setContents(saved.contents);
      await onSaved(alias, saved);
    } catch (error) {
      if (!isCurrent()) return;
      const conflict = failureCode(error) === "sftp_conflict";
      setProblem({ message: conflict ? t("sftp.conflict") : sftpProblemText(t, error), reloadable: conflict });
    } finally {
      if (isCurrent()) setBusy(false);
    }
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
    leaving,
    open,
    save,
    close,
    dismiss,
    requestReload,
    reload,
    keepEditing: () => setConfirming(null),
    discardAndLeave,
    stay: () => setLeaving(null),
  };
}

export type SFTPTextEditorModel = ReturnType<typeof useSFTPTextEditor>;
