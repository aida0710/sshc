import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { failureCode } from "../../api/client";
import { useTranslate } from "../../i18n/context";
import { snippetsApi, type ExecutionPreviewRequest, type Job, type Preview, type Snippet } from "../../snippets/api";
import { Button, Segmented } from "../../ui/surface";
import type { ExecutionTarget } from "./layout";

type Prepared = { preview: Preview; request: ExecutionPreviewRequest };

export function WorkspaceCommandCenter({ paneTargets, onClose }: { paneTargets: ExecutionTarget[]; onClose: () => void }) {
  const t = useTranslate();
  const [source, setSource] = useState<"command" | "snippet">("command");
  const [targetMode, setTargetMode] = useState<"host" | "pane">("host");
  const [command, setCommand] = useState("");
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [snippetId, setSnippetId] = useState("");
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [prepared, setPrepared] = useState<Prepared | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const closeButton = useRef<HTMLButtonElement>(null);
  const closeCallback = useRef(onClose);
  closeCallback.current = onClose;

  const selectedSnippet = useMemo(() => snippets.find((item) => item.id === snippetId) ?? null, [snippetId, snippets]);
  const targets = useMemo(() => {
    if (targetMode === "pane") return paneTargets;
    const seen = new Set<string>();
    return paneTargets.filter((target) => {
      if (seen.has(target.alias)) return false;
      seen.add(target.alias);
      return true;
    });
  }, [paneTargets, targetMode]);
  const duplicatePanes = new Set(paneTargets.map((target) => target.alias)).size !== paneTargets.length;

  useEffect(() => {
    void snippetsApi.library().then((library) => {
      setSnippets(library.snippets);
      setSnippetId((current) => current || library.snippets[0]?.id || "");
    }).catch((error: unknown) => setProblem(failureCode(error) || "snippet_failed"));
  }, []);

  useEffect(() => {
    if (job?.status !== "running") return;
    const timer = window.setInterval(() => {
      void snippetsApi.job(job.id).then(setJob).catch(() => undefined);
    }, 600);
    return () => window.clearInterval(timer);
  }, [job]);

  useEffect(() => {
    closeButton.current?.focus();
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") closeCallback.current();
    };
    window.addEventListener("keydown", closeOnEscape);
    return () => window.removeEventListener("keydown", closeOnEscape);
  }, []);

  function invalidate() {
    setPrepared(null);
    setProblem("");
  }

  function request(): ExecutionPreviewRequest | null {
    const executionTargets = targets.map(({ targetId, alias }) => ({ targetId, alias }));
    if (executionTargets.length === 0) return null;
    if (source === "command") return command.trim() === "" ? null : { command, targets: executionTargets, inputs: {} };
    return snippetId === "" ? null : { snippetId, targets: executionTargets, inputs };
  }

  async function makePreview() {
    const next = request();
    if (next === null) return;
    setBusy(true); setProblem(""); setJob(null);
    try {
      setPrepared({ request: next, preview: await snippetsApi.previewExecution(next) });
    } catch (error) {
      setProblem(failureCode(error) || "snippet_failed");
    } finally {
      setBusy(false);
    }
  }

  async function run() {
    if (prepared === null) return;
    setBusy(true); setProblem("");
    try {
      setJob(await snippetsApi.startExecution(prepared.preview, prepared.request));
      setPrepared(null);
    } catch (error) {
      setProblem(failureCode(error) || "snippet_failed");
    } finally {
      setBusy(false);
    }
  }

  return createPortal(
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/80 p-4 backdrop-blur-sm" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}>
      <section role="dialog" aria-modal="true" aria-labelledby="workspace-command-heading" className="max-h-[90vh] w-full max-w-5xl overflow-auto rounded-xl border border-control-line bg-card p-4 shadow-2xl">
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-3">
          <div className="grow"><h2 id="workspace-command-heading" className="text-base font-semibold">{t("workspace.broadcastHeading")}</h2><p className="mt-1 text-xs text-ink-muted">{t("workspace.broadcastDescription")}</p></div>
          <button ref={closeButton} type="button" onClick={onClose} aria-label={t("workspace.commandClose")} className="flex size-8 shrink-0 items-center justify-center rounded text-lg text-ink-muted hover:bg-select-fill">×</button>
        </div>
        {problem === "" ? null : <p role="alert" className="rounded bg-notice px-3 py-2 text-xs text-notice-ink">{problem}</p>}
        <div className="grid gap-3 lg:grid-cols-[minmax(18rem,0.9fr)_minmax(20rem,1.1fr)]">
          <div className="flex min-w-0 flex-col gap-3">
            <Segmented label={t("workspace.commandSource")} value={source} options={[{ value: "command", label: t("workspace.adHocCommand") }, { value: "snippet", label: t("workspace.savedSnippet") }]} onChange={(value) => { setSource(value); setInputs({}); invalidate(); }} />
            {source === "command" ? (
              <label className="text-xs text-ink-muted">{t("workspace.command")}<textarea rows={3} value={command} onChange={(event) => { setCommand(event.target.value); invalidate(); }} placeholder="uname -a" className="mt-1 block w-full rounded border border-control-line bg-control p-2 font-mono text-sm text-ink" /></label>
            ) : (
              <>
                <label className="text-xs text-ink-muted">{t("workspace.savedSnippet")}<select value={snippetId} onChange={(event) => { setSnippetId(event.target.value); setInputs({}); invalidate(); }} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm"><option value="">{t("workspace.chooseSnippet")}</option>{snippets.map((snippet) => <option key={snippet.id} value={snippet.id}>{snippet.name}</option>)}</select></label>
                {selectedSnippet === null ? null : <code className="whitespace-pre-wrap rounded bg-code-bg p-2 text-xs text-code-fg">{selectedSnippet.command}</code>}
                {selectedSnippet?.variables.map((variable) => <label key={variable.name} className="text-xs text-ink-muted"><code>{`{{${variable.name}}}`}</code>{variable.description ? ` · ${variable.description}` : ""}{variable.type === "boolean" ? <select value={inputs[variable.name] ?? ""} onChange={(event) => { const value = event.target.value; setInputs((current) => value === "" ? Object.fromEntries(Object.entries(current).filter(([key]) => key !== variable.name)) : { ...current, [variable.name]: value }); invalidate(); }} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm"><option value="">{variable.default === undefined ? "" : `${t("workspace.useDefault")} (${variable.default})`}</option><option value="true">true</option><option value="false">false</option></select> : <input type={variable.type === "secret" ? "password" : variable.type === "integer" ? "number" : "text"} value={inputs[variable.name] ?? ""} placeholder={variable.default === undefined ? "" : `${t("workspace.useDefault")}: ${variable.default}`} onChange={(event) => { setInputs((current) => ({ ...current, [variable.name]: event.target.value })); invalidate(); }} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm" />}</label>)}
              </>
            )}
          </div>
          <div className="flex min-w-0 flex-col gap-3">
            <div className="flex flex-wrap items-center gap-3">
              <Segmented label={t("workspace.targetMode")} value={targetMode} options={[{ value: "host", label: t("workspace.oncePerHost") }, { value: "pane", label: t("workspace.oncePerPane") }]} onChange={(value) => { setTargetMode(value); invalidate(); }} />
              <span className="text-xs text-ink-muted">{t("workspace.targetCount", { count: targets.length })}</span>
            </div>
            {!duplicatePanes && targetMode === "pane" ? <p className="text-xs text-ink-muted">{t("workspace.noDuplicatePanes")}</p> : null}
            <div className="grid max-h-28 grid-cols-1 gap-1 overflow-auto sm:grid-cols-2">{targets.map((target) => <div key={target.targetId} className="rounded border border-line px-2 py-1 text-xs"><span className="font-medium">{target.alias}</span><span className="ml-2 text-ink-muted">{target.state}</span></div>)}</div>
            <p className="text-xs leading-5 text-ink-muted">{t("workspace.executionNotice")}</p>
            <Button className="self-start" disabled={busy || request() === null} onClick={() => void makePreview()}>{t("snippets.preview")}</Button>
          </div>
        </div>
        {prepared === null ? null : <section className="rounded-lg border border-notice-line bg-notice p-3"><h3 className="text-sm font-medium">{t("workspace.previewHeading")}</h3><div className="mt-2 grid max-h-48 gap-2 overflow-auto md:grid-cols-2">{prepared.preview.targets.map((target) => <div key={target.targetId} className="min-w-0"><p className="truncate text-xs font-medium">{target.target.alias} · {target.target.user}@{target.target.hostName}:{target.target.port}</p>{(target.target.route ?? []).map((hop, index) => <p key={`${hop.alias}-${index}`} className="mt-1 break-all text-xs text-muted">{index + 1}. {hop.user}@{hop.hostName}:{hop.port}{hop.proxyCommand === "" ? "" : ` · ProxyCommand: ${hop.proxyCommand}`}</p>)}<pre className="mt-1 overflow-auto rounded bg-code-bg p-2 text-xs text-code-fg">{target.command}</pre></div>)}</div><Button kind="primary" className="mt-3" disabled={busy} onClick={() => void run()}>{t("workspace.runTargets", { count: prepared.preview.targets.length })}</Button></section>}
        {job === null ? null : <section className="rounded-lg border border-line bg-card p-3"><div className="flex items-center gap-2"><h3 className="grow text-sm font-medium">{t("snippets.results")} · {job.status}</h3>{job.status === "running" ? <Button onClick={() => void snippetsApi.cancel(job.id).then(setJob)}>{t("snippets.cancel")}</Button> : null}</div><div className="mt-2 grid max-h-56 gap-2 overflow-auto md:grid-cols-2">{job.results.map((result) => <div key={result.targetId} className="min-w-0 rounded border border-line p-2"><p className="text-xs font-medium">{result.alias} · {result.status}{result.exitCode === undefined ? "" : ` (${result.exitCode})`}</p>{result.stdout ? <pre className="mt-1 max-h-32 overflow-auto rounded bg-code-bg p-2 text-xs text-code-fg">{result.stdout}</pre> : null}{result.stderr ? <pre className="mt-1 max-h-32 overflow-auto rounded bg-danger/10 p-2 text-xs text-danger">{result.stderr}</pre> : null}{result.problem ? <p className="mt-1 text-xs text-danger">{result.problem}</p> : null}</div>)}</div></section>}
      </div>
      </section>
    </div>,
    document.body,
  );
}
