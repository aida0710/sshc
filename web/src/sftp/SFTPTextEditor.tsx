import { Suspense, lazy, useId } from "react";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { ModalShell } from "../ui/ModalShell";
import { Button, Notice } from "../ui/surface";
import type { SFTPTextEditorModel } from "./useSFTPTextEditor";

const MonacoEditor = lazy(() =>
  import("./MonacoEditor").then(({ MonacoEditor }) => ({ default: MonacoEditor })),
);

export function SFTPTextEditor({ editor, busy = false }: {
  editor: SFTPTextEditorModel;
  // Anything else the pane is doing that should hold the save button.
  busy?: boolean;
}) {
  const t = useTranslate();
  const id = useId();
  const { opened, contents, setContents, dirty, problem, confirming, leaving } = editor;
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
            <button type="button" className="text-xs text-ink-muted" onClick={editor.dismiss}>{t("sftp.close")}</button>
          </div>
          {problem === null ? null : (
            <div className="border-b border-line px-3 py-2">
              <Notice tone="danger">
                <span className="grow">{problem.message}</span>
                {problem.reloadable ? (
                  <button type="button" disabled={editor.busy} className="shrink-0 text-accent disabled:text-ink-faint" onClick={editor.requestReload}>{t("sftp.editorReload")}</button>
                ) : null}
              </Notice>
            </div>
          )}
          <div className="min-h-0 flex-1">
            <Suspense fallback={<div className="p-4 text-sm text-ink-muted">{t("sftp.editorLoading")}</div>}>
              <MonacoEditor path={opened.entry.path} value={contents} onChange={setContents} readOnly={editor.busy} />
            </Suspense>
          </div>
        </ModalShell>
      )}
      {confirming === null ? null : (
        <ConfirmDialog
          id={`${id}-discard`}
          heading={t(confirming === "close" ? "sftp.editorCloseHeading" : "sftp.editorReloadHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path: opened?.entry.path ?? "" })}</p>}
          confirmLabel={t(confirming === "close" ? "sftp.editorCloseDiscard" : "sftp.editorReloadDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={confirming === "close" ? editor.close : () => void editor.reload()}
          onCancel={editor.keepEditing}
        />
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
