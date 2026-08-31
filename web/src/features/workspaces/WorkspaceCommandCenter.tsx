import { useEffect, useMemo, useRef, useState } from "react";
import { failureCode } from "../../api/client";
import { useTranslate } from "../../i18n/context";
import { snippetsApi, type Snippet } from "../../snippets/api";
import { Button, Segmented } from "../../ui/surface";
import { ModalShell } from "../../ui/ModalShell";
import {
  terminalCommandApi,
  type TerminalCommandDispatch,
  type TerminalCommandPreview,
  type TerminalCommandRequest,
} from "./commandApi";

export type WorkspaceCommandTarget = {
  targetId: string;
  sessionId?: string;
  alias: string;
  title: string;
  paneNumber: number;
  connected: boolean;
  state: string;
};

type Prepared = { preview: TerminalCommandPreview; request: TerminalCommandRequest };

export function WorkspaceCommandCenter({ paneTargets, onClose }: { paneTargets: WorkspaceCommandTarget[]; onClose: () => void }) {
  const t = useTranslate();
  const [source, setSource] = useState<"command" | "snippet">("command");
  const [command, setCommand] = useState("");
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [snippetId, setSnippetId] = useState("");
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [prepared, setPrepared] = useState<Prepared | null>(null);
  const [dispatch, setDispatch] = useState<TerminalCommandDispatch | null>(null);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const operationSequence = useRef(0);
  const closeButton = useRef<HTMLButtonElement>(null);

  const selectedSnippet = useMemo(() => snippets.find((item) => item.id === snippetId) ?? null, [snippetId, snippets]);
  const targets = useMemo(() => paneTargets.filter((target): target is WorkspaceCommandTarget & { sessionId: string } =>
    target.connected && target.sessionId !== undefined), [paneTargets]);
  const targetFingerprint = paneTargets.map((target) =>
    `${target.targetId}\u0000${target.sessionId ?? ""}\u0000${target.alias}\u0000${target.title}\u0000${target.connected}\u0000${target.state}`,
  ).join("\u0001");

  useEffect(() => {
    void snippetsApi.library().then((library) => {
      setSnippets(library.snippets);
      setSnippetId((current) => current || library.snippets[0]?.id || "");
    }).catch((error: unknown) => setProblem(failureCode(error) || "snippet_failed"));
  }, []);

  useEffect(() => {
    operationSequence.current += 1;
    setPrepared(null);
    setDispatch(null);
    setBusy(false);
  }, [targetFingerprint]);

  function invalidate() {
    operationSequence.current += 1;
    setPrepared(null);
    setDispatch(null);
    setProblem("");
    setBusy(false);
  }

  function request(): TerminalCommandRequest | null {
    const executionTargets = targets.map(({ targetId, sessionId }) => ({ targetId, sessionId }));
    if (executionTargets.length === 0) return null;
    if (source === "command") return command.trim() === "" ? null : { command, targets: executionTargets, inputs: {} };
    return snippetId === "" ? null : { snippetId, targets: executionTargets, inputs };
  }

  async function makePreview() {
    const next = request();
    if (next === null) return;
    const operation = ++operationSequence.current;
    setBusy(true); setProblem(""); setDispatch(null);
    try {
      const nextPreview = await terminalCommandApi.preview(next);
      if (operationSequence.current === operation) setPrepared({ request: next, preview: nextPreview });
    } catch (error) {
      if (operationSequence.current === operation) setProblem(failureCode(error) || "terminal_command_failed");
    } finally {
      if (operationSequence.current === operation) setBusy(false);
    }
  }

  async function run() {
    if (prepared === null) return;
    const operation = ++operationSequence.current;
    setBusy(true); setProblem("");
    try {
      const nextDispatch = await terminalCommandApi.dispatch(prepared.preview, prepared.request);
      if (operationSequence.current === operation) {
        setDispatch(nextDispatch);
        setPrepared(null);
      }
    } catch (error) {
      if (operationSequence.current === operation) setProblem(failureCode(error) || "terminal_command_failed");
    } finally {
      if (operationSequence.current === operation) setBusy(false);
    }
  }

  function targetLabel(target: { targetId: string; alias: string; title: string }): string {
    const pane = paneTargets.find((item) => item.targetId === target.targetId);
    const suffix = pane === undefined ? "" : ` · ${t("workspace.paneNumber", { number: pane.paneNumber })}`;
    return `${target.alias} · ${target.title}${suffix}`;
  }

  return (
    <ModalShell
      labelledBy="workspace-command-heading"
      onDismiss={onClose}
      initialFocusRef={closeButton}
      panelClassName="max-h-[90vh] w-full max-w-5xl overflow-auto rounded-lg p-4"
    >
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
            <div className="flex flex-wrap items-center gap-3"><span className="text-xs font-medium">{t("workspace.targetMode")}</span><span className="text-xs text-ink-muted">{t("workspace.targetCount", { count: targets.length })}</span></div>
            <div className="grid max-h-36 grid-cols-1 gap-1 overflow-auto sm:grid-cols-2">{paneTargets.map((target) => <div key={target.targetId} className={`rounded border border-line px-2 py-1 text-xs ${target.connected ? "" : "opacity-60"}`}><span className="font-medium">{target.alias}</span><span className="ml-2 text-ink-muted">{target.title} · {t("workspace.paneNumber", { number: target.paneNumber })}</span>{target.connected ? null : <span className="ml-2 text-notice-ink">{t("workspace.targetSkipped", { state: target.state })}</span>}</div>)}</div>
            <p className="text-xs leading-5 text-ink-muted">{t("workspace.executionNotice")}</p>
            <Button className="self-start" disabled={busy || request() === null} onClick={() => void makePreview()}>{t("snippets.preview")}</Button>
          </div>
        </div>
        {prepared === null ? null : <section className="rounded-lg border border-notice-line bg-notice p-3"><h3 className="text-sm font-medium">{t("workspace.previewHeading")}</h3><div className="mt-2 grid max-h-48 gap-2 overflow-auto md:grid-cols-2">{prepared.preview.targets.map((target) => <div key={target.targetId} className="min-w-0"><p className="truncate text-xs font-medium">{targetLabel(target)}</p><pre className="mt-1 overflow-auto rounded bg-code-bg p-2 text-xs text-code-fg">{target.command}</pre></div>)}</div><Button kind="primary" className="mt-3" disabled={busy} onClick={() => void run()}>{t("workspace.sendTargets", { count: prepared.preview.targets.length })}</Button></section>}
        {dispatch === null ? null : <section className="rounded-lg border border-line bg-card p-3"><h3 className="text-sm font-medium">{t("workspace.deliveryResults")}</h3><p className="mt-1 text-xs text-ink-muted">{t("workspace.deliveryNotice")}</p><div className="mt-2 grid max-h-56 gap-2 overflow-auto md:grid-cols-2">{dispatch.results.map((result) => <div key={result.targetId} className="min-w-0 rounded border border-line p-2"><p className="text-xs font-medium">{targetLabel(result)} · {t(result.status === "delivered" ? "workspace.delivered" : "workspace.deliveryFailed")}</p>{result.problem ? <p className="mt-1 text-xs text-danger">{result.problem}</p> : null}</div>)}</div></section>}
      </div>
    </ModalShell>
  );
}
