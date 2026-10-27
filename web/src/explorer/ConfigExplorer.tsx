import { useCallback, useEffect, useRef, useState } from "react";
import { toProblem } from "../api/guards";
import { Button } from "../ui/surface";
import { useTranslate } from "../i18n/context";
import type { Problem } from "../api/client";
import { configApi, type FileContents, type Overview, type SavePreview } from "../api/config";
import { SavePreviewPanel } from "../connections/SavePreview";
import { saveProblemMessage } from "../ui/saveProblemMessage";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/icons";
import { PageBoard } from "../ui/PageBoard";
import { PageHeader } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { useRequestGeneration } from "../ui/useRequestGeneration";
import { useToast } from "../ui/useToast";
import { usePendingAction } from "../ui/usePendingAction";
import {
  fixedListWidthStyle,
  workspaceColumnsLayout,
  workspaceDetailPane,
  workspaceHeaderLayout,
  workspaceListPane,
} from "../ui/workspaceLayout";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import type { BrowserLocation, NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import { useDraftDiscardConfirmation } from "../routing/useDraftDiscardConfirmation";
import { sectionPath } from "../routing/sectionRoute";
import { ConfigFileList } from "./ConfigFileList";
import { sshDirectoryPath } from "./sshDirectoryPath";
import { ConfigDiagnostics } from "./ConfigDiagnostics";
import { ConfigFileHead } from "./ConfigFileHead";
import { ConfigPathForm, type ConfigPathTask } from "./ConfigPathForm";

// request numbers each handoff, so the handoff that was taken is the one cleared.
export type FileTarget = { path: string; line: number; request: number };

// ConfigOperation は、SSH Config の画面が走らせるプレビューと書き込みである。一度に一つだけ
// 走らせ、押したボタンだけに回転の印を出し、ほかの操作のボタンは終わるまで止める。
type ConfigOperation = "preview" | "save" | "delete" | ConfigPathTask;

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
  const compact = useMediaQuery(mobileViewportQuery);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [file, setFile] = useState<FileContents | null>(null);
  const [draft, setDraft] = useState("");
  const [preview, setPreview] = useState<SavePreview | null>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [pathTask, setPathTask] = useState<ConfigPathTask | null>(null);
  const [pathInput, setPathInput] = useState("");
  const [pathProblem, setPathProblem] = useState("");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const [jump, setJump] = useState("");
  // showingFile は、スマホで一覧の代わりに開いたファイルを見せているかである。PCでは両方を並べる。
  const [showingFile, setShowingFile] = useState(false);
  const [opening, setOpening] = useState(false);
  const pending = usePendingAction<ConfigOperation>();
  const busy = pending.running !== null;
  const busyWith = (operation: ConfigOperation) => pending.running === operation;
  const showToast = useToast();
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
    requestOpenRef.current(target.path, { show: true });
  }, [onTargetHandled, target]);

  // 入口のファイルは、PCでは開いた状態で見せるために読んでおく。スマホでは一覧のまま、選んだときに見せる。
  useEffect(() => {
    if (autoOpened.current || overview === null || file !== null) return;
    if (overview.entry.path === undefined) return;
    autoOpened.current = true;
    requestOpenRef.current(overview.entry.path, { show: false });
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

  function closePathTask() {
    setPathTask(null);
    setPathInput("");
    setPathProblem("");
  }

  function beginPathTask(task: ConfigPathTask) {
    setPathTask(task);
    setPathInput("");
    setPathProblem("");
    setShowingFile(true);
  }

  async function open(path: string) {
    const isCurrent = openGeneration.begin();
    setOpening(true);
    setFile(null);
    setDraft("");
    try {
      const loaded = await configApi.file(path);
      if (!isCurrent()) return;
      setFile(loaded);
      setDraft(loaded.contents);
      setPreview(null);
      setProblem(null);
      closePathTask();
      setConfirmingDelete(false);
    } catch (error) {
      if (!isCurrent()) return;
      setProblem(toProblem(error));
    } finally {
      if (isCurrent()) setOpening(false);
    }
  }

  // perform は、操作を一つ走らせる。失敗は理由をプレビューの欄（削除なら確認ダイアログの中）に
  // 出し、古いプレビューは残さない。成功を伝えるのは work の役目である。結果が画面のどこにも
  // 残らない操作があるので、書き込みはどれもトーストで知らせる。
  async function perform(operation: ConfigOperation, work: () => Promise<void>) {
    await pending.start(operation, async () => {
      setProblem(null);
      try {
        await work();
      } catch (error) {
        setPreview(null);
        setProblem(toProblem(error));
      }
    });
  }

  function reportWritten(message: string) {
    showToast(message, { tone: "success" });
  }

  // typedPath は、欄の値を ~/.ssh からの相対パスにする。~/.ssh の外を指していれば、
  // 欄の下に理由を出して null を返す。
  function typedPath(): string | null {
    const resolved = sshDirectoryPath(pathInput);
    if ("outside" in resolved) {
      setPathProblem(t("explorer.pathOutsideSSHDirectory"));
      return null;
    }
    setPathProblem("");
    return resolved.relative === "" ? null : resolved.relative;
  }

  async function renameFile() {
    if (file === null || file.file.path === undefined) return;
    const path = file.file.path;
    const destinationPath = typedPath();
    if (destinationPath === null) return;
    await perform("rename", async () => {
      const result = await configApi.save({
        kind: "file_rename",
        path,
        base: file.contents,
        destinationPath,
      });
      reportWritten(t("explorer.renamedFile", { path: destinationPath }));
      setPreview(result.preview);
      await reload();
      await open(destinationPath);
    });
  }

  async function deleteFile() {
    if (file === null || file.file.path === undefined) return;
    const path = file.file.path;
    await perform("delete", async () => {
      const result = await configApi.save({
        kind: "file_delete",
        path,
        base: file.contents,
      });
      reportWritten(t("explorer.deletedFile", { path }));
      setPreview(result.preview);
      setConfirmingDelete(false);
      setFile(null);
      setDraft("");
      await reload();
    });
  }

  // 作ったファイルを開くので、いまの下書きは消える。呼び手が先に破棄を確かめる。
  async function createFile() {
    const path = typedPath();
    if (path === null) return;
    await perform("createFile", async () => {
      await configApi.save({ kind: "file_raw", path, base: "", raw: "# created by sshc\n" });
      reportWritten(t("explorer.createdFile", { path }));
      await reload();
      await open(path);
    });
  }

  async function createDirectory() {
    const path = typedPath();
    if (path === null) return;
    await perform("createDirectory", async () => {
      await configApi.save({ kind: "directory_create", path });
      reportWritten(t("explorer.createdDirectory", { path }));
      closePathTask();
      await reload();
    });
  }

  async function deleteDirectory() {
    const path = typedPath();
    if (path === null) return;
    await perform("deleteDirectory", async () => {
      await configApi.save({ kind: "directory_delete", path });
      reportWritten(t("explorer.deletedDirectory", { path }));
      closePathTask();
      await reload();
    });
  }

  function submitPathTask(task: ConfigPathTask) {
    if (task === "createFile") draftDiscard.confirmBefore(() => void createFile());
    else if (task === "createDirectory") void createDirectory();
    else if (task === "deleteDirectory") void deleteDirectory();
    else void renameFile();
  }

  async function run(action: "preview" | "save") {
    if (file === null || file.file.path === undefined) return;
    const request = {
      kind: "file_raw" as const,
      path: file.file.path,
      base: file.contents,
      raw: draft,
    };
    await perform(action, async () => {
      if (action === "preview") {
        setPreview(await configApi.preview(request));
        return;
      }
      const result = await configApi.save(request);
      reportWritten(t("explorer.savedFile", { path: request.path }));
      setPreview(result.preview);
      await reload();
      await open(request.path);
    });
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
  // 消える。別のファイルを開くときは、下書きを捨ててよいかを先に確かめる。show は、
  // スマホで一覧の代わりにそのファイルを見せるかである。
  function requestOpen(path: string, { show }: { show: boolean }) {
    if (show) setShowingFile(true);
    if (file !== null && file.file.path === path) return;
    draftDiscard.confirmBefore(() => void open(path));
  }
  const requestOpenRef = useRef(requestOpen);
  requestOpenRef.current = requestOpen;

  const createButton = (enabled: boolean) => (
    <Button className="inline-flex shrink-0 items-center gap-1.5" disabled={!enabled} onClick={() => beginPathTask("createFile")}>
      <Icon name="plus" className="size-[15px]" />
      {t("explorer.createFile")}
    </Button>
  );

  if (overview === null) {
    // 読み込むあいだも見出しと2つの列を出しておき、読み終えたときに画面が組み変わって見えないようにする。
    return (
      <PageBoard fill labelledBy="config-heading">
        <PageHeader title={t("explorer.pageTitle")} headingId="config-heading" actions={createButton(false)} className={workspaceHeaderLayout} />
        <div style={fixedListWidthStyle} className={workspaceColumnsLayout(compact)}>
          <div className={workspaceListPane(compact, false)}>
            {problem === null ? (
              <PanelState tone="loading" title={t("explorer.loading")} />
            ) : (
              <PanelState tone="failed" title={problem.message} {...(problem.detail === undefined ? {} : { detail: problem.detail })} action={<Button onClick={() => void reload()}>{t("shell.bootstrapRetry")}</Button>} />
            )}
          </div>
          {compact ? null : <div className="hidden md:block" />}
        </div>
      </PageBoard>
    );
  }

  const openPath = file?.file.path ?? file?.file.absolute ?? "";
  const editableFiles = overview.files.filter((node) => node.editable).length;
  const fileLocked = file === null || file.file.path === undefined || !file.editable || modified;
  const fileActions: ActionMenuItem[] = [
    { label: t("explorer.renameFile"), onSelect: () => beginPathTask("rename"), disabled: fileLocked || busy },
    { label: t("explorer.createDirectory"), onSelect: () => beginPathTask("createDirectory"), disabled: busy },
    {
      label: t("explorer.deleteFile"),
      tone: "danger",
      disabled: fileLocked || busy,
      onSelect: () => {
        setProblem(null);
        setConfirmingDelete(true);
      },
    },
    { label: t("explorer.deleteDirectory"), tone: "danger", onSelect: () => beginPathTask("deleteDirectory"), disabled: busy },
  ];
  // 削除の確認ダイアログは、同じ失敗を自分の中に出す。背面にも出すと、スクリーンリーダーが同じ文を 2 回読み上げる。
  const shownProblem = confirmingDelete ? null : problem;
  const hasResult = preview !== null || (problem?.conflict ?? null) !== null || shownProblem !== null;

  return (
    <>
      <PageBoard fill labelledBy="config-heading">
        {compact && showingFile ? <h2 id="config-heading" className="sr-only">{t("explorer.pageTitle")}</h2> : (
          <PageHeader
            title={t("explorer.pageTitle")}
            headingId="config-heading"
            meta={t("explorer.summary", { files: overview.files.length, editable: editableFiles, diagnostics: overview.diagnostics.length })}
            actions={createButton(true)}
            className={workspaceHeaderLayout}
          />
        )}
        <div style={fixedListWidthStyle} className={workspaceColumnsLayout(compact)}>
          <section aria-labelledby="explorer-heading" className={workspaceListPane(compact, showingFile)}>
            <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-2 pt-3.5">
              <h3 id="explorer-heading" className="px-2.5 pb-1 pt-2.5 text-xs text-ink-faint">{t("explorer.hierarchy")}</h3>
              <ConfigFileList files={overview.files} openPath={openPath} onOpen={(path) => requestOpen(path, { show: true })} />
            </div>
            <ConfigDiagnostics diagnostics={overview.diagnostics} />
          </section>

          <section aria-busy={opening} className={workspaceDetailPane(compact, showingFile)}>
            {compact ? (
              <Button className="mb-3 w-fit" onClick={() => setShowingFile(false)}>{t("explorer.allFiles")}</Button>
            ) : null}
            {file === null || opening ? null : (
              <ConfigFileHead
                file={file}
                modified={modified}
                actions={fileActions}
                previewing={busyWith("preview")}
                saving={busyWith("save")}
                busy={busy}
                onPreview={() => void run("preview")}
                onSave={() => void run("save")}
              />
            )}
            {pathTask === null ? null : (
              <ConfigPathForm
                task={pathTask}
                path={pathInput}
                currentPath={file?.file.path ?? ""}
                problem={pathProblem}
                submitting={busyWith(pathTask)}
                busy={busy}
                onPathChange={(path) => {
                  setPathInput(path);
                  setPathProblem("");
                }}
                onSubmit={() => submitPathTask(pathTask)}
                onCancel={closePathTask}
              />
            )}
            {jump === "" ? null : <p aria-live="polite" className="mb-2 text-xs text-ink-faint">{jump}</p>}
            {hasResult ? (
              <div className="mb-4">
                <SavePreviewPanel preview={preview} conflict={problem?.conflict ?? null} problem={shownProblem} />
              </div>
            ) : null}
            {opening ? <PanelState tone="loading" title={t("explorer.loading")} className="flex-1" /> : file === null ? (
              <EmptyState icon="config" title={t("explorer.emptyHeading")} detail={t("explorer.selectFile")} />
            ) : (
              <>
                <label htmlFor="file-raw" className="sr-only">{t("explorer.fileText", { path: file.file.path ?? file.file.absolute })}</label>
                <textarea
                  id="file-raw"
                  ref={editorRef}
                  value={draft}
                  onChange={(event) => setDraft(event.target.value)}
                  spellCheck={false}
                  disabled={!file.editable}
                  className="min-h-80 w-full flex-1 resize-none rounded-[10px] border border-line bg-well px-3.5 py-2.5 font-mono text-[12.5px] leading-[1.75] text-ink focus:border-accent focus:outline-none focus:ring-[3px] focus:ring-accent/20 disabled:text-ink-faint md:min-h-64"
                />
                <p className="mt-2.5 shrink-0 text-xs text-ink-faint">{t("explorer.formatKept")}</p>
              </>
            )}
          </section>
        </div>
      </PageBoard>
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
    </>
  );
}
