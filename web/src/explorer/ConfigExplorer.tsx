import { useCallback, useEffect, useRef, useState } from "react";
import { toProblem } from "../api/guards";
import { Button } from "../ui/surface";
import { useTranslate } from "../i18n/context";
import type { Problem } from "../api/client";
import { configApi, type FileContents, type Overview, type SavePreview } from "../api/config";
import { SavePreviewPanel } from "../connections/SavePreview";
import { Icon } from "../ui/icons";
import {
  control,
  fieldLabel,
  hintText,
  sectionHeading,
} from "../ui/form";
import { PageHeader } from "../ui/page";

export type FileTarget = { path: string; line: number };

type ConfigExplorerProps = {
  target?: FileTarget | null;
};


function lineRange(contents: string, line: number): { start: number; end: number } {
  const lines = contents.split("\n");
  const index = Math.min(Math.max(line, 1), lines.length) - 1;
  const start = lines.slice(0, index).reduce((total, text) => total + text.length + 1, 0);
  return { start, end: start + (lines[index]?.length ?? 0) };
}

export function ConfigExplorer({ target = null }: ConfigExplorerProps) {
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
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const jumped = useRef<FileTarget | null>(null);
  const openRequest = useRef(0);
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

  useEffect(() => {
    if (target === null) return;
    void open(target.path);
  }, [target]);

  useEffect(() => {
    if (autoOpened.current || target !== null || overview === null || file !== null) return;
    if (overview.entry.path === undefined) return;
    autoOpened.current = true;
    void open(overview.entry.path);
  }, [file, overview, target]);

  useEffect(() => {
    if (target === null || jumped.current === target) return;
    if (file === null || file.file.path !== target.path) return;
    const editor = editorRef.current;
    if (editor === null) return;
    jumped.current = target;
    const range = lineRange(file.contents, target.line);
    editor.focus();
    editor.setSelectionRange(range.start, range.end);
    setJump(t("explorer.opened", { path: target.path, line: target.line }));
  }, [file, target, t]);

  async function open(path: string) {
    const request = ++openRequest.current;
    setFile(null);
    setDraft("");
    try {
      const loaded = await configApi.file(path);
      if (request !== openRequest.current) return;
      setFile(loaded);
      setDraft(loaded.contents);
      setPreview(null);
      setProblem(null);
      setRenameTo("");
      setConfirmingDelete(false);
    } catch (error) {
      if (request !== openRequest.current) return;
      setProblem(toProblem(error));
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

  if (overview === null) {
    return <p role="status" className="text-sm text-ink-muted">{t("explorer.loading")}</p>;
  }

  const openPath = file?.file.path ?? file?.file.absolute ?? "";
  const modified = file !== null && draft !== file.contents;
  const editableFiles = overview.files.filter((node) => node.editable).length;

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <PageHeader title={t("explorer.pageTitle")} description={t("explorer.pageDescription")} />
      <dl data-config-metrics className="sshc-card grid grid-cols-3 divide-x divide-hairline overflow-hidden rounded-md bg-toolbar">
        {[
          [t("explorer.metricFiles"), overview.files.length, false],
          [t("explorer.metricEditable"), editableFiles, false],
          [t("explorer.metricDiagnostics"), overview.diagnostics.length, overview.diagnostics.length > 0],
        ].map(([label, value, attention]) => (
          <div key={String(label)} className="flex min-w-0 flex-col items-start gap-1 px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4 sm:px-4">
            <dt className={`min-w-0 break-words text-xs font-medium ${attention ? "text-notice-ink" : "text-ink-muted"}`}>{label}</dt>
            <dd className={`font-mono text-sm font-semibold ${attention ? "text-notice-ink" : "text-ink"}`}>{value}</dd>
          </div>
        ))}
      </dl>

      {jump === "" ? null : <p aria-live="polite" className={hintText}>{jump}</p>}

      <div data-config-explorer className="sshc-card grid min-h-0 grid-cols-1 overflow-hidden rounded-md bg-card lg:grid-cols-[19rem_minmax(0,1fr)]">
        <section aria-labelledby="explorer-heading" className="flex min-h-0 flex-col bg-tree lg:border-r lg:border-line">
          <div data-explorer-header="tree" className="flex min-h-12 items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-2">
            <div className="flex min-w-0 items-center gap-2">
              <Icon name="config" className="h-4 w-4 text-ink-muted" />
              <h3 id="explorer-heading" className={sectionHeading}>{t("explorer.hierarchy")}</h3>
            </div>
            <span className="rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-ink-muted">{overview.files.length}</span>
          </div>

          <ul className="flex max-h-96 flex-col gap-0.5 overflow-y-auto p-2 lg:max-h-none lg:flex-1">
            {overview.files.map((node) => {
              const current = (node.file.path ?? node.file.absolute) === openPath;
              const state = t("explorer.fileState", {
                missing: node.missing === true ? t("explorer.missing") : "",
                loads: node.loads > 1 ? t("explorer.readTimes", { count: node.loads }) : "",
                editable: node.editable ? t("explorer.editable") : t("explorer.readOnly"),
              });
              return (
                <li key={node.file.absolute} className={`rounded-lg ${current ? "bg-select-fill" : "hover:bg-surface-subtle"}`}>
                  <div className="flex items-start gap-2.5 px-2.5 py-2">
                    <span data-config-node-icon aria-hidden="true" className={`mt-5 shrink-0 md:mt-2.5 ${current ? "text-accent" : "text-ink-faint"}`}>
                      <Icon name="config" className="h-4 w-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      {node.file.path === undefined ? (
                        <p className="text-sm text-ink-muted">
                          <span className="block break-all font-mono text-xs">{node.file.absolute}</span>
                          <span className="mt-1 block text-xs leading-5 text-notice-ink">{t("explorer.externalFile")}</span>
                        </p>
                      ) : (
                        <button
                          type="button"
                          aria-current={current ? "true" : "false"}
                          onClick={() => void open(node.file.path ?? "")}
                          className={`block min-h-10 w-full truncate text-left font-mono text-sm md:min-h-0 ${current ? "font-semibold text-ink" : "text-ink-muted hover:text-ink"}`}
                        >
                          {node.file.path}
                        </button>
                      )}
                      <p className="mt-0.5 text-xs text-ink-faint">{state}</p>
                      {(node.includes ?? []).map((include) => (
                        <div key={`${node.file.absolute}:${include.line}:${include.pattern}`} className="mt-2 min-w-0 overflow-hidden rounded-md bg-surface-subtle px-2 py-1.5 text-xs text-ink-muted">
                          <span className="block break-all font-mono leading-5">{include.pattern}</span>
                          {include.condition === undefined ? null : (
                            <span className="mt-0.5 block break-words leading-5 text-notice-ink">{t("explorer.insideCondition", { condition: include.condition })}</span>
                          )}
                          <ul className="mt-1">
                            {(include.matches ?? []).map((match) => (
                              <li key={match.absolute} className="truncate font-mono text-ink-faint" title={match.path ?? match.absolute}>{`→ ${match.path ?? match.absolute}`}</li>
                            ))}
                          </ul>
                        </div>
                      ))}
                    </div>
                  </div>
                </li>
              );
            })}
          </ul>

          <div className="flex flex-col gap-2 border-t border-line bg-toolbar p-3">
            <h3 className={sectionHeading}>{t("explorer.workspaceActions")}</h3>
            <label htmlFor="new-file-path" className={fieldLabel}>{t("explorer.newFilePath")}</label>
            <input
              id="new-file-path"
              value={newPath}
              onChange={(event) => setNewPath(event.target.value)}
              placeholder="conf.d/30-lab.conf"
              className={`${control} min-h-10 font-mono text-xs md:min-h-0`}
            />
            <div className="flex flex-wrap gap-2">
              <Button className="min-h-10 md:min-h-0" onClick={() => void createFile()} disabled={newPath === ""}>{t("explorer.createFile")}</Button>
              <Button className="min-h-10 md:min-h-0" onClick={() => void createDirectory()} disabled={newPath === ""}>{t("explorer.createDirectory")}</Button>
              <Button kind="danger" className="min-h-10 md:min-h-0" onClick={() => void deleteDirectory()} disabled={newPath === ""}>{t("explorer.deleteDirectory")}</Button>
            </div>
            <p className={hintText}>{t("explorer.newFileNote")}</p>
            <details className="text-xs text-ink-muted">
              <summary className="cursor-pointer text-ink">{t("explorer.directoryHelp")}</summary>
              <p className="mt-2 leading-5">{t("explorer.directoryNote")}</p>
            </details>
          </div>

          <div className={`border-t border-line p-3 ${overview.diagnostics.length > 0 ? "bg-notice" : "bg-toolbar"}`}>
            <div className="mb-2 flex items-center justify-between gap-2">
              <h3 className={sectionHeading}>{t("explorer.diagnostics")}</h3>
              <span className={`font-mono text-xs ${overview.diagnostics.length > 0 ? "text-notice-ink" : "text-ink-faint"}`}>{overview.diagnostics.length}</span>
            </div>
            {overview.diagnostics.length === 0 ? (
              <p className={hintText}>{t("explorer.noIncludeProblem")}</p>
            ) : (
              <ul className="flex flex-col gap-1">
                {overview.diagnostics.map((diagnostic, index) => (
                  <li key={`${diagnostic.code}-${index}`} className={`font-mono text-xs ${diagnostic.severity === "error" ? "text-danger" : diagnostic.severity === "warning" ? "text-notice-ink" : "text-ink-muted"}`}>
                    {`${diagnostic.code} ${diagnostic.path ?? diagnostic.absolute ?? ""}${diagnostic.line === undefined ? "" : `:${diagnostic.line}`} ${diagnostic.detail ?? ""}`}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </section>

        <section className="flex min-w-0 flex-col border-t border-line lg:border-t-0">
          {file === null ? (
            <div role="status" className="flex min-h-96 flex-1 flex-col items-center justify-center gap-2 bg-surface-subtle p-8 text-center">
              <span className="flex h-12 w-12 items-center justify-center rounded-md bg-surface text-ink-faint">
                <Icon name="config" className="h-6 w-6" />
              </span>
              <h3 className={sectionHeading}>{t("explorer.emptyHeading")}</h3>
              <p className={hintText}>{t("explorer.selectFile")}</p>
            </div>
          ) : (
            <>
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
              <div className="flex flex-wrap items-center justify-end gap-2 border-t border-line bg-toolbar px-4 py-3">
                <Button className="min-h-10 md:min-h-0" onClick={() => void run("preview")}>{t("explorer.preview")}</Button>
                <Button kind="primary" className="min-h-10 md:min-h-0" onClick={() => void run("save")} disabled={!file.editable}>{t("explorer.saveFile")}</Button>
              </div>

              {file.file.path === undefined || !file.editable ? null : (
                <div className="flex flex-col gap-2 border-t border-line bg-surface-subtle p-4">
                  <div className="flex flex-wrap items-center justify-between gap-3">
                    <div>
                      <h4 className={sectionHeading}>{t("explorer.fileOperations")}</h4>
                      <p className={`mt-1 ${hintText}`}>{t("explorer.fileOperationsNote")}</p>
                    </div>
                    <div className="flex min-w-0 flex-1 flex-wrap items-end justify-end gap-2 sm:flex-nowrap">
                      <label htmlFor="rename-file-path" className="sr-only">{t("explorer.renameTo")}</label>
                      <input
                        id="rename-file-path"
                        value={renameTo}
                        onChange={(event) => setRenameTo(event.target.value)}
                        placeholder={file.file.path}
                        className={`${control} min-h-10 min-w-48 max-w-sm font-mono text-xs md:min-h-0`}
                      />
                      <Button className="min-h-10 md:min-h-0" onClick={() => void renameFile()} disabled={renameTo === "" || renameTo === file.file.path || modified}>{t("explorer.renameFile")}</Button>
                      {confirmingDelete ? (
                        <>
                          <Button kind="danger" className="min-h-10 md:min-h-0" onClick={() => void deleteFile()}>{t("explorer.confirmDelete")}</Button>
                          <Button className="min-h-10 md:min-h-0" onClick={() => setConfirmingDelete(false)}>{t("explorer.cancelDelete")}</Button>
                        </>
                      ) : (
                        <Button className="min-h-10 md:min-h-0" onClick={() => setConfirmingDelete(true)} disabled={modified}>{t("explorer.deleteFile")}</Button>
                      )}
                    </div>
                  </div>
                  <p className={hintText}>{modified ? t("explorer.saveOrDiscardFirst") : t("explorer.deleteIsRecoverable")}</p>
                </div>
              )}
            </>
          )}
        </section>
      </div>

      <SavePreviewPanel preview={preview} conflict={problem?.conflict ?? null} problem={problem} />
    </div>
  );
}
