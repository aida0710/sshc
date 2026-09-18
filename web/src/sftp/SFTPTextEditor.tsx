import { Suspense, lazy, useEffect, useId, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import type { BrowserLocation, NavigationBlocker } from "../routing/useSectionRoute";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { sftpApi, type RemoteEntry, type RemoteTextFile } from "./api";

const MonacoEditor = lazy(() =>
  import("./MonacoEditor").then(({ MonacoEditor }) => ({ default: MonacoEditor })),
);

export type SFTPTextSource = {
  readText(alias: string, path: string): Promise<RemoteTextFile>;
  saveText(alias: string, path: string, contents: string, expectedRevision: string): Promise<RemoteTextFile>;
};

type OpenedText = { alias: string; file: RemoteTextFile };

// The text editor is one modal over the file list: it knows how to read,
// edit and save a file with its revision, and how to keep the user from
// losing an unsaved change by leaving. It does not know what a directory is;
// the pane tells it when a save should be followed by a refresh.
export function useSFTPTextEditor({
  source = sftpApi,
  onProblem,
  onSaved,
  onNavigationBlockerChange,
  onDirtyChange,
  onNavigateLocation,
}: {
  source?: SFTPTextSource;
  // Shown in the pane's banner. An empty string clears it.
  onProblem: (message: string) => void;
  // Runs after the file is written and before the editor shows the saved
  // revision. Returning false keeps the previous revision on screen, for
  // example because the directory could not be re-read.
  onSaved: (alias: string, saved: RemoteTextFile) => Promise<boolean>;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onDirtyChange?: ((path: string | null) => void) | undefined;
  onNavigateLocation?: ((url: string) => void) | undefined;
}) {
  const t = useTranslate();
  const [opened, setOpened] = useState<OpenedText | null>(null);
  const [contents, setContents] = useState("");
  const [busy, setBusy] = useState(false);
  const [leaving, setLeaving] = useState<BrowserLocation | null>(null);
  // Bumped whenever the editor is closed or reset, so that a read or a save
  // that was still in flight cannot reopen a file the user has left.
  const generation = useRef(0);
  const dirty = opened !== null && contents !== opened.file.contents;

  useEffect(() => {
    onDirtyChange?.(dirty ? opened?.file.entry.path ?? "" : null);
    return () => onDirtyChange?.(null);
  }, [dirty, onDirtyChange, opened?.file.entry.path]);

  useEffect(() => {
    if (!dirty) {
      onNavigationBlockerChange?.(null);
      return;
    }
    onNavigationBlockerChange?.((next) => {
      setLeaving(next);
      return false;
    });
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnBeforeUnload);
    return () => {
      onNavigationBlockerChange?.(null);
      window.removeEventListener("beforeunload", warnBeforeUnload);
    };
  }, [dirty, onNavigationBlockerChange]);

  function close() {
    generation.current += 1;
    setOpened(null);
    setContents("");
    setBusy(false);
    setLeaving(null);
  }

  async function open(alias: string, entry: RemoteEntry) {
    if (dirty) {
      onProblem(t("sftp.unsavedBlocked"));
      return;
    }
    const current = ++generation.current;
    setBusy(true);
    onProblem("");
    try {
      const file = await source.readText(alias, entry.path);
      if (current !== generation.current) return;
      setOpened({ alias, file });
      setContents(file.contents);
    } catch (error) {
      if (current !== generation.current) return;
      const code = failureCode(error);
      if (code === "sftp_not_utf8" || code === "sftp_text_too_large") {
        onProblem(t(code === "sftp_not_utf8" ? "sftp.binaryHint" : "sftp.tooLargeHint"));
      } else {
        onProblem(code || (error instanceof Error ? error.message : "sftp_failed"));
      }
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  async function save() {
    if (opened === null) return;
    const current = generation.current;
    const { alias, file } = opened;
    setBusy(true);
    onProblem("");
    try {
      const saved = await source.saveText(alias, file.entry.path, contents, file.revision);
      if (current !== generation.current) return;
      const shown = await onSaved(alias, saved);
      if (current !== generation.current || !shown) return;
      setOpened({ alias, file: saved });
      setContents(saved.contents);
    } catch (error) {
      if (current !== generation.current) return;
      onProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  // Closing with an unsaved change is refused rather than confirmed: the
  // banner says why, and the user can save or keep editing.
  function dismiss() {
    if (dirty) onProblem(t("sftp.unsavedBlocked"));
    else close();
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
    leaving,
    open,
    save,
    close,
    dismiss,
    discardAndLeave,
    stay: () => setLeaving(null),
  };
}

export type SFTPTextEditorModel = ReturnType<typeof useSFTPTextEditor>;

export function SFTPTextEditor({ editor, busy = false }: {
  editor: SFTPTextEditorModel;
  // Anything else the pane is doing that should hold the save button.
  busy?: boolean;
}) {
  const t = useTranslate();
  const id = useId();
  const { opened, contents, setContents, dirty, leaving } = editor;
  return (
    <>
      {opened === null ? null : (
        <ModalShell
          labelledBy={`${id}-editor`}
          onDismiss={editor.dismiss}
          panelClassName="flex h-[min(52rem,calc(100dvh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-lg"
        >
          <div className="flex items-center gap-2 border-b border-line bg-toolbar px-3 py-2">
            <h2 id={`${id}-editor`} className="min-w-0 grow truncate font-mono text-xs">{opened.entry.path}</h2>
            {dirty ? <span className="text-xs text-notice-ink">{t("sftp.unsaved")}</span> : null}
            <Button disabled={busy || editor.busy || !dirty} onClick={() => void editor.save()}>{t("sftp.save")}</Button>
            <button type="button" disabled={dirty} className="text-xs text-ink-muted disabled:text-ink-faint" onClick={editor.close}>{t("sftp.close")}</button>
          </div>
          <div className="min-h-0 flex-1">
            <Suspense fallback={<div className="p-4 text-sm text-ink-muted">{t("sftp.editorLoading")}</div>}>
              <MonacoEditor path={opened.entry.path} value={contents} onChange={setContents} readOnly={editor.busy} />
            </Suspense>
          </div>
        </ModalShell>
      )}
      {leaving === null ? null : (
        <ConfirmDialog
          id={`${id}-leave`}
          heading={t("sftp.leaveHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path: opened?.entry.path ?? "" })}</p>}
          confirmLabel={t("sftp.leaveDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={editor.discardAndLeave}
          onCancel={editor.stay}
        />
      )}
    </>
  );
}
