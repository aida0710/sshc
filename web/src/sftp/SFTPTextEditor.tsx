import { Suspense, lazy, useId } from "react";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { ModalShell } from "../ui/ModalShell";
import { Button, Notice } from "../ui/surface";
import { loadMonacoEditor } from "./loadMonacoEditor";
import type { SFTPTextEditorModel } from "./useSFTPTextEditor";

const MonacoEditor = lazy(() => loadMonacoEditor().then(({ MonacoEditor }) => ({ default: MonacoEditor })));

export function SFTPTextEditor({ editor, busy = false }: {
  editor: SFTPTextEditorModel;
  // Anything else the pane is doing that should hold writing the file (Save
  // and Overwrite).
  busy?: boolean;
}) {
  const t = useTranslate();
  const id = useId();
  const { opened, contents, setContents, dirty, problem, confirming, leaving } = editor;
  const path = opened?.entry.path ?? "";
  const writeDisabled = busy || editor.busy || !dirty;
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
            <Button disabled={writeDisabled} onClick={() => void editor.save()}>{t("sftp.save")}</Button>
            <button type="button" className="text-xs text-ink-muted" onClick={editor.dismiss}>{t("sftp.close")}</button>
          </div>
          {problem === null ? null : (
            <div className="border-b border-line px-3 py-2">
              <Notice tone="danger">
                {/* On a narrow screen the actions wrap below the message instead of squeezing it. */}
                <span className="flex grow flex-wrap items-center gap-x-3 gap-y-1">
                  <span className="grow basis-48">{problem.message}</span>
                  {problem.conflict ? (
                    <span className="flex shrink-0 gap-3">
                      <button type="button" disabled={editor.busy} className="text-accent disabled:text-ink-faint" onClick={editor.requestReload}>{t("sftp.editorReload")}</button>
                      <button type="button" disabled={writeDisabled} className="text-accent disabled:text-ink-faint" onClick={() => void editor.requestOverwrite()}>{t("sftp.editorOverwrite")}</button>
                    </span>
                  ) : null}
                </span>
              </Notice>
            </div>
          )}
          <div className="min-h-0 flex-1">
            <Suspense fallback={<div className="p-4 text-sm text-ink-muted">{t("sftp.editorLoading")}</div>}>
              <MonacoEditor path={opened.entry.path} value={contents} onChange={setContents} readOnly={editor.busy} initialLine={editor.initialLine} />
            </Suspense>
          </div>
        </ModalShell>
      )}
      {confirming === null ? null : (
        <ConfirmDialog
          id={`${id}-discard`}
          heading={t(confirming === "close" ? "sftp.editorCloseHeading" : "sftp.editorReloadHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path })}</p>}
          confirmLabel={t(confirming === "close" ? "sftp.editorCloseDiscard" : "sftp.editorReloadDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={confirming === "close" ? editor.close : () => void editor.reload()}
          onCancel={editor.keepEditing}
        />
      )}
      {editor.confirmingOverwrite ? (
        <ConfirmDialog
          id={`${id}-overwrite`}
          heading={t("sftp.editorOverwriteHeading")}
          body={(
            <>
              <p className="text-sm text-ink-muted">{t("sftp.editorOverwriteBody", { path })}</p>
              {/* Says why Overwrite cannot be pressed yet: over a slow route the read can take seconds. */}
              {editor.readingOverwriteRevision ? <p role="status" className="text-sm text-ink-muted">{t("sftp.editorOverwriteReading")}</p> : null}
            </>
          )}
          confirmLabel={t("sftp.editorOverwrite")}
          cancelLabel={t("sftp.cancel")}
          confirmDisabled={editor.readingOverwriteRevision}
          onConfirm={() => void editor.overwrite()}
          onCancel={editor.cancelOverwrite}
        />
      ) : null}
      {leaving === null ? null : (
        <ConfirmDialog
          id={`${id}-leave`}
          heading={t("sftp.leaveHeading")}
          body={<p className="text-sm text-ink-muted">{t("sftp.leaveBody", { path })}</p>}
          confirmLabel={t("sftp.leaveDiscard")}
          cancelLabel={t("sftp.leaveStay")}
          onConfirm={editor.discardAndLeave}
          onCancel={editor.stay}
        />
      )}
    </>
  );
}
