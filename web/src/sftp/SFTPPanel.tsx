import { Suspense, lazy, useEffect, useRef, useState, type DragEvent as ReactDragEvent } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { Button } from "../ui/surface";
import { sftpApi, type RemoteEntry, type RemoteTextFile } from "./api";

const MonacoEditor = lazy(() =>
  import("./MonacoEditor").then(({ MonacoEditor }) => ({ default: MonacoEditor })),
);

function parentOf(remotePath: string): string {
  if (remotePath === "/") return "/";
  const pieces = remotePath.split("/").filter(Boolean);
  pieces.pop();
  return `/${pieces.join("/")}` || "/";
}

function join(parent: string, name: string): string {
  return `${parent === "/" ? "" : parent}/${name}`;
}

type UploadItem = { id: string; name: string; status: "pending" | "uploading" | "done" | "failed"; problem?: string };

export function SFTPPanel({ aliases }: { aliases: string[] }) {
  const t = useTranslate();
  const [alias, setAlias] = useState(aliases[0] ?? "");
  const [path, setPath] = useState("/");
  const [pathDraft, setPathDraft] = useState("/");
  const [entries, setEntries] = useState<RemoteEntry[]>([]);
  const [opened, setOpened] = useState<RemoteTextFile | null>(null);
  const [contents, setContents] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [deleting, setDeleting] = useState<RemoteEntry | null>(null);
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [dragging, setDragging] = useState(false);
  const upload = useRef<HTMLInputElement>(null);
  const dirty = opened !== null && contents !== opened.contents;

  async function load(nextPath = path, nextAlias = alias, preserveEditor = false) {
    if (nextAlias === "") return;
    setBusy(true);
    setProblem("");
    try {
      const listing = await sftpApi.list(nextAlias, nextPath);
      setPath(listing.path);
      setPathDraft(listing.path);
      setEntries(listing.entries);
      if (!preserveEditor) {
        setOpened(null);
        setContents("");
      }
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "sftp_failed"));
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    if (alias !== "") void load("/", alias);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [alias]);

  async function openText(entry: RemoteEntry) {
    if (dirty) {
      setProblem(t("sftp.unsavedBlocked"));
      return;
    }
    setBusy(true);
    setProblem("");
    try {
      const file = await sftpApi.readText(alias, entry.path);
      setOpened(file);
      setContents(file.contents);
    } catch (error) {
      const code = failureCode(error);
      if (code === "sftp_not_utf8" || code === "sftp_text_too_large") {
        setProblem(t(code === "sftp_not_utf8" ? "sftp.binaryHint" : "sftp.tooLargeHint"));
      } else {
        setProblem(code || (error instanceof Error ? error.message : "sftp_failed"));
      }
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    if (opened === null) return;
    setBusy(true);
    setProblem("");
    try {
      const saved = await sftpApi.saveText(alias, opened.entry.path, contents, opened.revision);
      setOpened(saved);
      setContents(saved.contents);
      await load(parentOf(saved.entry.path));
      setOpened(saved);
      setContents(saved.contents);
    } catch (error) {
      setProblem(failureCode(error) === "sftp_conflict" ? t("sftp.conflict") : failureCode(error) || "sftp_failed");
    } finally {
      setBusy(false);
    }
  }

  async function makeDirectory() {
    const name = window.prompt(t("sftp.mkdirPrompt"))?.trim() ?? "";
    if (name === "" || name.includes("/")) return;
    setBusy(true);
    try {
      await sftpApi.mkdir(alias, join(path, name));
      await load();
    } catch (error) {
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function rename(entry: RemoteEntry) {
    const name = window.prompt(t("sftp.renamePrompt"), entry.name)?.trim() ?? "";
    if (name === "" || name === entry.name || name.includes("/")) return;
    setBusy(true);
    try {
      await sftpApi.rename(alias, entry.path, join(path, name));
      await load();
    } catch (error) {
      setProblem(failureCode(error) || "sftp_failed");
      setBusy(false);
    }
  }

  async function remove() {
    if (deleting === null) return;
    setBusy(true);
    try {
      await sftpApi.remove(alias, deleting.path);
      setDeleting(null);
      await load();
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "delete_failed"));
      setDeleting(null);
      setBusy(false);
    }
  }

  async function uploadFiles(files: File[]) {
    if (alias === "" || files.length === 0 || busy) return;
    const batch = files.map((file) => ({ id: crypto.randomUUID(), name: file.name, status: "pending" as const }));
    setUploads(batch);
    setBusy(true);
    for (let index = 0; index < files.length; index++) {
      const file = files[index];
      const item = batch[index];
      if (file === undefined || item === undefined) continue;
      setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "uploading" } : candidate));
      try {
        await sftpApi.upload(alias, join(path, file.name), file, false);
        setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "done" } : candidate));
      } catch (error) {
        const uploadProblem = failureCode(error) || (error instanceof Error ? error.message : "upload_failed");
        setUploads((current) => current.map((candidate) => candidate.id === item.id ? { ...candidate, status: "failed", problem: uploadProblem } : candidate));
      }
    }
    await load(path, alias, true);
  }

  function acceptDrop(event: ReactDragEvent<HTMLDivElement>) {
    event.preventDefault();
    setDragging(false);
    if (busy || alias === "") return;
    void uploadFiles(Array.from(event.dataTransfer.files));
  }

  return (
    <section className="flex h-full min-h-0 flex-col gap-3" aria-labelledby="sftp-heading">
      <div className="flex flex-wrap items-center gap-2">
        <h2 id="sftp-heading" className="mr-auto font-medium">{t("sftp.heading")}</h2>
        <select
          aria-label={t("sftp.host")}
          value={alias}
          onChange={(event) => setAlias(event.target.value)}
          className="rounded-md border border-control-line bg-control px-2 py-1.5 text-sm"
        >
          {aliases.length === 0 ? <option value="">{t("sftp.noHosts")}</option> : null}
          {aliases.map((value) => <option key={value}>{value}</option>)}
        </select>
        <input
          aria-label={t("sftp.path")}
          value={pathDraft}
          onChange={(event) => setPathDraft(event.target.value)}
          onKeyDown={(event) => { if (event.key === "Enter" && !dirty) void load(pathDraft); }}
          className="min-w-56 grow rounded-md border border-control-line bg-control px-2 py-1.5 font-mono text-sm"
        />
        <Button disabled={busy || dirty || alias === ""} onClick={() => void load(pathDraft)}>{t("sftp.go")}</Button>
      </div>

      {problem === "" ? null : <p role="alert" className="rounded-md border border-notice-line bg-notice px-3 py-2 text-sm text-notice-ink">{problem}</p>}

      <div className="grid min-h-0 flex-1 gap-3 lg:grid-cols-[minmax(18rem,0.7fr)_minmax(24rem,1.3fr)]">
        <div
          aria-label={t("sftp.dropZone")}
          className={`flex min-h-0 flex-col rounded-lg border bg-card transition-shadow ${dragging ? "border-accent ring-2 ring-accent/40" : "border-line"}`}
          onDragEnter={(event) => { event.preventDefault(); if (!busy && alias !== "") setDragging(true); }}
          onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; }}
          onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
          onDrop={acceptDrop}
        >
          <div className="flex flex-wrap gap-2 border-b border-line p-2">
            <Button disabled={busy || dirty || path === "/"} onClick={() => void load(parentOf(path))}>{t("sftp.up")}</Button>
            <Button disabled={busy || alias === ""} onClick={() => void makeDirectory()}>{t("sftp.newFolder")}</Button>
            <Button disabled={busy || alias === ""} onClick={() => upload.current?.click()}>{t("sftp.upload")}</Button>
            <span className="ml-auto self-center text-xs text-ink-muted">{t(dragging ? "sftp.dropNow" : "sftp.dropHint")}</span>
            <input
              ref={upload}
              type="file"
              multiple
              className="hidden"
              onChange={(event) => {
                const files = Array.from(event.target.files ?? []);
                event.target.value = "";
                void uploadFiles(files);
              }}
            />
          </div>
          {uploads.length === 0 ? null : <ul aria-label={t("sftp.uploads")} className="border-b border-line px-3 py-2 text-xs">{uploads.map((item) => <li key={item.id} className="flex min-w-0 items-center gap-2"><span className="min-w-0 grow truncate font-mono">{item.name}</span><span className={item.status === "failed" ? "text-danger" : item.status === "done" ? "text-live" : "text-ink-muted"}>{item.status === "failed" ? item.problem : t(`sftp.upload.${item.status}`)}</span></li>)}</ul>}
          <div className="min-h-0 overflow-auto">
            <table className="w-full text-left text-sm">
              <thead className="sticky top-0 bg-card text-xs text-ink-muted"><tr><th className="px-3 py-2">{t("sftp.name")}</th><th className="px-3 py-2 text-right">{t("sftp.size")}</th><th><span className="sr-only">{t("sftp.actions")}</span></th></tr></thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.path} className="border-t border-line">
                    <td className="max-w-48 px-3 py-2">
                      <button type="button" className="block w-full truncate text-left font-mono hover:text-accent" onClick={() => entry.type === "directory" ? void load(entry.path) : void openText(entry)}>{entry.type === "directory" ? "▸ " : ""}{entry.name}</button>
                    </td>
                    <td className="px-3 py-2 text-right text-xs text-ink-muted">{entry.type === "file" ? entry.size.toLocaleString() : "—"}</td>
                    <td className="whitespace-nowrap px-2 py-1 text-right">
                      {entry.type === "file" ? <button className="px-1 text-xs text-accent" onClick={() => void sftpApi.download(alias, entry.path)}>{t("sftp.download")}</button> : null}
                      <button className="px-1 text-xs text-accent" onClick={() => void rename(entry)}>{t("sftp.rename")}</button>
                      <button className="px-1 text-xs text-danger" onClick={() => setDeleting(entry)}>{t("sftp.delete")}</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>

        <div className="flex min-h-0 flex-col overflow-hidden rounded-lg border border-line bg-card">
          {opened === null ? (
            <div className="grid h-full min-h-64 place-items-center p-6 text-sm text-ink-muted">{t("sftp.editorEmpty")}</div>
          ) : (
            <>
              <div className="flex items-center gap-2 border-b border-line px-3 py-2">
                <code className="min-w-0 grow truncate text-xs">{opened.entry.path}</code>
                {dirty ? <span className="text-xs text-notice-ink">{t("sftp.unsaved")}</span> : null}
                <Button disabled={busy || !dirty} onClick={() => void save()}>{t("sftp.save")}</Button>
                <button type="button" className="text-xs text-ink-muted" onClick={() => { if (!dirty) setOpened(null); }}>{t("sftp.close")}</button>
              </div>
              <div className="min-h-0 flex-1">
                <Suspense fallback={<div className="p-4 text-sm text-ink-muted">{t("sftp.editorLoading")}</div>}>
                  <MonacoEditor path={opened.entry.path} value={contents} onChange={setContents} />
                </Suspense>
              </div>
            </>
          )}
        </div>
      </div>

      {deleting === null ? null : (
        <ConfirmDialog
          id="sftp-delete-heading"
          heading={t("sftp.deleteHeading")}
          body={<p className="break-all text-sm text-ink-muted">{deleting.path}</p>}
          confirmLabel={t("sftp.delete")}
          cancelLabel={t("sftp.cancel")}
          onConfirm={() => void remove()}
          onCancel={() => setDeleting(null)}
        />
      )}
    </section>
  );
}
