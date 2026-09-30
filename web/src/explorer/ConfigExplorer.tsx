import { useCallback, useEffect, useRef, useState } from "react";
import { DisclosureChevron } from "../ui/DisclosureChevron";
import { toProblem } from "../api/guards";
import { Button, Card } from "../ui/surface";
import { useTranslate } from "../i18n/context";
import type { Problem } from "../api/client";
import { configApi, type FileContents, type Overview, type SavePreview } from "../api/config";
import { SavePreviewPanel } from "../connections/SavePreview";
import { saveProblemMessage } from "../ui/saveProblemMessage";
import { Icon } from "../ui/icons";
import { hintText, sectionHeading } from "../ui/form";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import type { BrowserLocation, NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import { useDraftDiscardConfirmation } from "../routing/useDraftDiscardConfirmation";
import { sectionPath } from "../routing/sectionRoute";
import { ConfigFileList } from "./ConfigFileList";
import { ConfigPathActions } from "./ConfigPathActions";
import { ConfigDiagnostics } from "./ConfigDiagnostics";
import { ConfigFileOperations } from "./ConfigFileOperations";

// request numbers each handoff, so the handoff that was taken is the one cleared.
export type FileTarget = { path: string; line: number; request: number };

type ConfigExplorerProps = {
  target?: FileTarget | null;
  onTargetHandled?: (request: number) => void;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onNavigateLocation?: ((url: string, options?: NavigateLocationOptions) => void) | undefined;
};


// Config の中の移動（コマンドパレットからファイルを開くなど）は下書きを捨てない。開く
// ファイルが変わるときは requestOpen が確かめる。
function staysInConfig(next: BrowserLocation): boolean {
  return next.pathname === sectionPath("Config");
}

function lineRange(contents: string, line: number): { start: number; end: number } {
  const lines = contents.split("\n");
  const index = Math.min(Math.max(line, 1), lines.length) - 1;
  const start = lines.slice(0, index).reduce((total, text) => total + text.length + 1, 0);
  return { start, end: start + (lines[index]?.length ?? 0) };
}

export function ConfigExplorer({ target = null, onTargetHandled = () => undefined, onNavigationBlockerChange, onNavigateLocation }: ConfigExplorerProps) {
  const t = useTranslate();
  const [overview, setOverview] = useState<Overview | null>(null);
  const [file, setFile] = useState<FileContents | null>(null);
  const [draft, setDraft] = useState("");
  const [preview, setPreview] = useState<SavePreview | null>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [newPath, setNewPath] = useState("");
  const [renameTo, setRenameTo] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [jump, setJump] = useState("");
  const [hierarchyOpen, setHierarchyOpen] = useState(false);
  const [opening, setOpening] = useState(false);
  const editorRef = useRef<HTMLTextAreaElement>(null);
  // The handed line is kept here once the handoff is cleared, until the file
  // it names has opened and the line is selected.
  const [pendingJump, setPendingJump] = useState<FileTarget | null>(null);
  const handledTarget = useRef<number | null>(null);
  const openGeneration = useRequestGeneration();
  const autoOpened = useRef(false);

  const reload = useCallback(async () => {
    try {
      setOverview(await configApi.overview());
    } catch (error) {
      setProblem(toProblem(error));
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  // Clear the handoff as soon as it is taken: otherwise every later visit to
  // Config would open this file at this line again instead of the entry file.
  useEffect(() => {
    if (target === null || target.request === handledTarget.current) return;
    handledTarget.current = target.request;
    onTargetHandled(target.request);
    autoOpened.current = true;
    setPendingJump(target);
    requestOpenRef.current(target.path);
  }, [onTargetHandled, target]);

  useEffect(() => {
    if (autoOpened.current || overview === null || file !== null) return;
    if (overview.entry.path === undefined) return;
    autoOpened.current = true;
    requestOpenRef.current(overview.entry.path);
  }, [file, overview]);

  useEffect(() => {
    if (pendingJump === null || file === null || file.file.path !== pendingJump.path) return;
    const editor = editorRef.current;
    if (editor === null) return;
    setPendingJump(null);
    const range = lineRange(file.contents, pendingJump.line);
    editor.focus();
    editor.setSelectionRange(range.start, range.end);
    setJump(t("explorer.opened", { path: pendingJump.path, line: pendingJump.line }));
  }, [file, pendingJump, t]);

  async function open(path: string) {
    const isCurrent = openGeneration.begin();
    setOpening(true);
    setHierarchyOpen(false);
    setFile(null);
    setDraft("");
    try {
      const loaded = await configApi.file(path);
      if (!isCurrent()) return;
      setFile(loaded);
      setDraft(loaded.contents);
      setPreview(null);
      setProblem(null);
      setRenameTo("");
      setConfirmingDelete(false);
    } catch (error) {
      if (!isCurrent()) return;
      setProblem(toProblem(error));
    } finally {
      if (isCurrent()) setOpening(false);
    }
  }

  async function renameFile() {
    if (file === null || file.file.path === undefined || renameTo === "") return;
    try {
      const result = await configApi.save({
        kind: "file_rename",
        path: file.file.path,
        base: file.contents,
        destinationPath: renameTo,
      });
      setPreview(result.preview);
      setProblem(null);
      setRenameTo("");
      await reload();
      await open(renameTo);
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  async function deleteFile() {
    if (file === null || file.file.path === undefined) return;
    try {
      const result = await configApi.save({
        kind: "file_delete",
        path: file.file.path,
        base: file.contents,
      });
      setPreview(result.preview);
      setProblem(null);
      setConfirmingDelete(false);
      setFile(null);
      setDraft("");
      await reload();
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  // 作ったファイルを開くので、いまの下書きは消える。呼び手が先に破棄を確かめる。
  async function createFile() {
    if (newPath === "") return;
    const path = newPath;
    try {
      await configApi.save({ kind: "file_raw", path, base: "", raw: "# created by sshc\n" });
      setNewPath("");
      setProblem(null);
      await reload();
      await open(path);
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  async function createDirectory() {
    if (newPath === "") return;
    try {
      await configApi.save({ kind: "directory_create", path: newPath });
      setNewPath("");
      setProblem(null);
      await reload();
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  async function deleteDirectory() {
    if (newPath === "") return;
    try {
      await configApi.save({ kind: "directory_delete", path: newPath });
      setNewPath("");
      setProblem(null);
      await reload();
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  async function run(action: "preview" | "save") {
    if (file === null || file.file.path === undefined) return;
    const request = {
      kind: "file_raw" as const,
      path: file.file.path,
      base: file.contents,
      raw: draft,
    };
    try {
      if (action === "preview") {
        setPreview(await configApi.preview(request));
        setProblem(null);
        return;
      }
      const result = await configApi.save(request);
      setPreview(result.preview);
      setProblem(null);
      await reload();
      await open(request.path);
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  const modified = file !== null && draft !== file.contents;
  const draftDiscard = useDraftDiscardConfirmation({
    dirty: modified,
    discard: () => setDraft(file?.contents ?? ""),
    navigationKeepsDraft: staysInConfig,
    onNavigationBlockerChange,
    onNavigateLocation,
  });

  // 開いているファイル自身をもう一度開いても読み直さない。読み直すと手編集の下書きが
  // 消える。別のファイルを開くときは、下書きを捨ててよいかを先に確かめる。
  function requestOpen(path: string) {
    if (file !== null && file.file.path === path) return;
    draftDiscard.confirmBefore(() => void open(path));
  }
  const requestOpenRef = useRef(requestOpen);
  requestOpenRef.current = requestOpen;

  if (overview === null) {
    return problem === null ? (
      <PanelState tone="loading" title={t("explorer.loading")} />
    ) : (
      <PanelState tone="failed" title={problem.message} {...(problem.detail === undefined ? {} : { detail: problem.detail })} action={<Button onClick={() => void reload()}>{t("shell.bootstrapRetry")}</Button>} />
    );
  }

  const openPath = file?.file.path ?? file?.file.absolute ?? "";
  const editableFiles = overview.files.filter((node) => node.editable).length;

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <PageHeader title={t("explorer.pageTitle")} description={t("explorer.pageDescription")} />
      <MetricGrid data-config-metrics className="grid-cols-3 divide-x divide-hairline sm:grid-cols-3 lg:grid-cols-3">
        {([
          [t("explorer.metricFiles"), overview.files.length, false],
          [t("explorer.metricEditable"), editableFiles, false],
          [t("explorer.metricDiagnostics"), overview.diagnostics.length, overview.diagnostics.length > 0],
        ] as const).map(([label, value, attention]) => (
          <MetricCard key={String(label)} label={String(label)} value={value} compact attention={Boolean(attention)} className="min-w-0 flex-col items-start sm:flex-row sm:items-center" />
        ))}
      </MetricGrid>

      {jump === "" ? null : <p aria-live="polite" className={hintText}>{jump}</p>}

      <Card data-config-explorer radius="md" className="grid min-h-0 grid-cols-1 lg:grid-cols-[19rem_minmax(0,1fr)]">
        <section aria-labelledby="explorer-heading" className="flex min-h-0 flex-col bg-tree lg:border-r lg:border-line">
          <div data-explorer-header="tree" className="flex min-h-12 items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-2">
            <div className="flex min-w-0 items-center gap-2">
              <Icon name="config" className="h-4 w-4 text-ink-muted" />
              <h3 id="explorer-heading" className={`${sectionHeading} hidden lg:block`}>{t("explorer.hierarchy")}</h3>
              <button
                type="button"
                aria-expanded={hierarchyOpen}
                aria-controls="config-hierarchy"
                onClick={() => setHierarchyOpen((current) => !current)}
                className="flex items-center gap-2 text-left text-sm font-semibold text-ink lg:hidden"
              >
                {t("explorer.hierarchy")}
                <DisclosureChevron expanded={hierarchyOpen} />
              </button>
            </div>
            <span className="rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-ink-muted">{overview.files.length}</span>
          </div>

          <div id="config-hierarchy" className={`${hierarchyOpen ? "flex" : "hidden"} min-h-0 flex-1 flex-col lg:flex`}>
            <ConfigFileList files={overview.files} openPath={openPath} onOpen={requestOpen} />
            <ConfigPathActions
              path={newPath}
              onPathChange={setNewPath}
              onCreateFile={() => draftDiscard.confirmBefore(() => void createFile())}
              onCreateDirectory={() => void createDirectory()}
              onDeleteDirectory={() => void deleteDirectory()}
            />
            <ConfigDiagnostics diagnostics={overview.diagnostics} />
          </div>
        </section>

        <section aria-busy={opening} className="flex min-w-0 flex-col border-t border-line lg:border-t-0">
          {opening ? <PanelState tone="loading" title={t("explorer.loading")} className="min-h-48 flex-1" /> : file === null ? (
            <PanelState
              tone="empty"
              title={t("explorer.emptyHeading")}
              detail={t("explorer.selectFile")}
              className="min-h-96 flex-1 bg-surface-subtle"
              icon={<span className="flex h-12 w-12 items-center justify-center rounded-md bg-surface text-ink-faint">
                <Icon name="config" className="h-6 w-6" />
              </span>}
            />
          ) : (
            <>
              <div className="flex min-h-0 flex-1 flex-col">
                <div data-explorer-header="file" className="flex min-h-12 flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${modified ? "bg-notice-ink" : file.editable ? "bg-live" : "bg-ink-faint"}`} />
                    <span className="truncate font-mono text-sm font-semibold text-ink">{file.file.path ?? file.file.absolute}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="rounded bg-surface px-2 py-0.5 text-xs text-ink-muted">{file.editable ? t("explorer.editable") : t("explorer.readOnly")}</span>
                    {modified ? <span className="rounded bg-notice px-2 py-0.5 text-xs font-medium text-notice-ink">{t("explorer.unsaved")}</span> : null}
                  </div>
                </div>
                <label htmlFor="file-raw" className="sr-only">{t("explorer.fileText", { path: file.file.path ?? file.file.absolute })}</label>
                <textarea
                  id="file-raw"
                  ref={editorRef}
                  value={draft}
                  onChange={(event) => setDraft(event.target.value)}
                  rows={24}
                  spellCheck={false}
                  disabled={!file.editable}
                  className="min-h-96 w-full flex-1 resize-y border-0 bg-control p-4 font-mono text-xs leading-6 text-ink focus:outline-none focus:ring-2 focus:ring-inset focus:ring-accent disabled:bg-surface-subtle disabled:text-ink-faint"
                />
                <div className="sticky bottom-0 z-10 flex flex-wrap items-center justify-end gap-2 border-t border-line bg-toolbar px-4 pb-[calc(0.75rem+env(safe-area-inset-bottom))] pt-3 md:static md:z-auto md:py-3">
                  <Button className="min-h-10 md:min-h-0" onClick={() => void run("preview")}>{t("explorer.preview")}</Button>
                  <Button kind="primary" className="min-h-10 md:min-h-0" onClick={() => void run("save")} disabled={!file.editable}>{t("explorer.saveFile")}</Button>
                </div>
              </div>

              {file.file.path === undefined || !file.editable ? null : (
                <ConfigFileOperations
                  path={file.file.path}
                  modified={modified}
                  renameTo={renameTo}
                  onRenameToChange={setRenameTo}
                  onRename={() => void renameFile()}
                  onDelete={() => {
                    setProblem(null);
                    setConfirmingDelete(true);
                  }}
                />
              )}
            </>
          )}
        </section>
      </Card>

      {/* 削除の確認ダイアログは、同じ失敗を自分の中に出す。背面にも出すと、
          スクリーンリーダーが同じ文を 2 回読み上げる。 */}
      <SavePreviewPanel preview={preview} conflict={problem?.conflict ?? null} problem={confirmingDelete ? null : problem} />
      {draftDiscard.confirming ? (
        <DiscardDraftDialog id="config-draft-discard-heading" onConfirm={draftDiscard.confirmDiscard} onCancel={draftDiscard.keepEditing} />
      ) : null}
      {confirmingDelete && file?.file.path !== undefined ? (
        <ConfirmDialog
          id="config-file-delete-heading"
          heading={t("explorer.deleteFile")}
          body={<div className="flex flex-col gap-2 text-sm text-ink-muted"><p className="break-all font-mono">{file.file.path}</p><p>{t("explorer.deleteIsRecoverable")}</p></div>}
          confirmLabel={t("explorer.confirmDelete")}
          cancelLabel={t("explorer.cancelDelete")}
          onConfirm={deleteFile}
          onCancel={() => setConfirmingDelete(false)}
          error={problem === null ? "" : saveProblemMessage(problem, t)}
        />
      ) : null}
    </div>
  );
}
