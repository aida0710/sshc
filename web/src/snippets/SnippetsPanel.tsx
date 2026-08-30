import { useEffect, useMemo, useState } from "react";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Button } from "../ui/surface";
import { snippetsApi, type Job, type Preview, type Snippet, type SnippetDraft, type SnippetVariable } from "./api";

function placeholders(command: string): string[] {
  return [...new Set([...command.matchAll(/\{\{([A-Za-z_][A-Za-z0-9_]{0,63})\}\}/g)].map((match) => match[1] ?? "").filter(Boolean))];
}

function variablesFor(command: string, previous: SnippetVariable[]): SnippetVariable[] {
  const byName = new Map(previous.map((variable) => [variable.name, variable]));
  return placeholders(command).map((name) => byName.get(name) ?? { name, type: "string", required: true });
}

export function snippetVariableTypeLabelKey(type: string): MessageKey {
  switch (type) {
    case "string": return "snippets.variableType.string";
    case "integer": return "snippets.variableType.integer";
    case "boolean": return "snippets.variableType.boolean";
    case "secret": return "snippets.variableType.secret";
    default: return "snippets.variableType.unknown";
  }
}

export function snippetStatusLabelKey(status: string): MessageKey {
  switch (status) {
    case "running": return "snippets.status.running";
    case "completed": return "snippets.status.completed";
    case "cancelled": return "snippets.status.cancelled";
    case "queued": return "snippets.status.queued";
    case "succeeded": return "snippets.status.succeeded";
    case "failed": return "snippets.status.failed";
    default: return "snippets.status.unknown";
  }
}

const blank: SnippetDraft = { name: "", description: "", command: "", variables: [] };

export function SnippetsPanel({ aliases, selectedSnippetId = null }: { aliases: string[]; selectedSnippetId?: string | null }) {
  const t = useTranslate();
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [draft, setDraft] = useState<SnippetDraft>(blank);
  const [targets, setTargets] = useState<string[]>([]);
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [preview, setPreview] = useState<Preview | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [startupAlias, setStartupAlias] = useState(aliases[0] ?? "");
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const current = useMemo(() => snippets.find((snippet) => snippet.id === selected) ?? null, [snippets, selected]);

  async function reload() {
    const library = await snippetsApi.library();
    setSnippets(library.snippets);
  }

  useEffect(() => { void reload().catch((error: unknown) => setProblem(failureCode(error) || "snippet_failed")); }, []);

  useEffect(() => {
    if (selectedSnippetId === null) return;
    const snippet = snippets.find((candidate) => candidate.id === selectedSnippetId);
    if (snippet !== undefined && selected !== snippet.id) edit(snippet);
  }, [selected, selectedSnippetId, snippets]);

  useEffect(() => {
    if (job?.status !== "running") return;
    const timer = window.setInterval(() => {
      void snippetsApi.job(job.id).then(setJob).catch(() => undefined);
    }, 600);
    return () => window.clearInterval(timer);
  }, [job]);

  function edit(snippet: Snippet | null) {
    setSelected(snippet?.id ?? null);
    setDraft(snippet === null ? blank : { name: snippet.name, description: snippet.description ?? "", command: snippet.command, variables: snippet.variables });
    setInputs({});
    setPreview(null);
    setJob(null);
    setProblem("");
  }

  async function save() {
    setBusy(true); setProblem("");
    try {
      const value = { ...draft, variables: variablesFor(draft.command, draft.variables) };
      const saved = selected === null ? await snippetsApi.create(value) : await snippetsApi.update(selected, value);
      await reload(); edit(saved);
    } catch (error) { setProblem(failureCode(error) || "snippet_failed"); }
    finally { setBusy(false); }
  }

  async function makePreview() {
    if (selected === null || targets.length === 0) return;
    setBusy(true); setProblem(""); setJob(null);
    try { setPreview(await snippetsApi.preview(selected, targets, inputs)); }
    catch (error) { setProblem(failureCode(error) || "snippet_failed"); }
    finally { setBusy(false); }
  }

  async function run() {
    if (preview === null) return;
    setBusy(true); setProblem("");
    try { setJob(await snippetsApi.start(preview, targets, inputs)); setPreview(null); }
    catch (error) { setProblem(failureCode(error) || "snippet_failed"); }
    finally { setBusy(false); }
  }

  function updateVariable(name: string, update: Partial<SnippetVariable>) {
    setDraft({
      ...draft,
      variables: variablesFor(draft.command, draft.variables).map((variable) => variable.name === name ? { ...variable, ...update } : variable),
    });
    setPreview(null);
  }

  async function removeCurrent() {
    if (current === null) return;
    setBusy(true); setProblem("");
    try { await snippetsApi.remove(current.id); edit(null); await reload(); }
    catch (error) { setProblem(failureCode(error) || "snippet_failed"); }
    finally { setBusy(false); }
  }

  async function updateStartup(snippetId: string) {
    if (startupAlias === "") return;
    setBusy(true); setProblem("");
    try { await snippetsApi.setStartup(startupAlias, snippetId, snippetId === "" ? {} : inputs); await reload(); }
    catch (error) { setProblem(failureCode(error) || "snippet_failed"); }
    finally { setBusy(false); }
  }

  return (
    <section className="grid min-h-full gap-4 lg:grid-cols-[16rem_minmax(24rem,1fr)_minmax(20rem,0.8fr)]" aria-labelledby="snippets-heading">
      <div className="flex flex-col gap-2">
        <div className="flex items-center"><h2 id="snippets-heading" className="grow font-medium">{t("snippets.heading")}</h2><Button onClick={() => edit(null)}>{t("snippets.new")}</Button></div>
        <div className="flex flex-col gap-1">
          {snippets.map((snippet) => <button key={snippet.id} type="button" onClick={() => edit(snippet)} className={`rounded-md px-3 py-2 text-left text-sm ${selected === snippet.id ? "bg-select-fill" : "hover:bg-select-fill"}`}><span className="block truncate font-medium">{snippet.name}</span><code className="block truncate text-xs text-ink-muted">{snippet.command}</code></button>)}
          {snippets.length === 0 ? <p className="text-sm text-ink-muted">{t("snippets.empty")}</p> : null}
        </div>
      </div>

      <div className="flex min-w-0 flex-col gap-3 rounded-lg border border-line bg-card p-4">
        {problem === "" ? null : <p role="alert" className="rounded bg-notice px-3 py-2 text-sm text-notice-ink">{problem}</p>}
        <label className="text-xs text-ink-muted">{t("snippets.name")}<input value={draft.name} onChange={(event) => setDraft({ ...draft, name: event.target.value })} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm" /></label>
        <label className="text-xs text-ink-muted">{t("snippets.description")}<input value={draft.description} onChange={(event) => setDraft({ ...draft, description: event.target.value })} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm" /></label>
        <label className="text-xs text-ink-muted">{t("snippets.command")}<textarea rows={7} value={draft.command} onChange={(event) => { setDraft({ ...draft, command: event.target.value }); setPreview(null); }} className="mt-1 block w-full rounded border border-control-line bg-control p-2 font-mono text-sm" /></label>
        <p className="text-xs text-ink-muted">{t("snippets.variableHint")}</p>
        {variablesFor(draft.command, draft.variables).map((variable) => (
          <div key={variable.name} className="grid grid-cols-[8rem_minmax(0,1fr)] gap-2">
            <label className="text-xs text-ink-muted"><code>{`{{${variable.name}}}`}</code><select aria-label={t("snippets.variableType")} value={variable.type} onChange={(event) => updateVariable(variable.name, { type: event.target.value as SnippetVariable["type"] })} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm">{(["string", "integer", "boolean", "secret"] as const).map((type) => <option key={type} value={type}>{t(snippetVariableTypeLabelKey(type))}</option>)}</select></label>
            <label className="text-xs text-ink-muted">{t("snippets.value")}{variable.type === "boolean" ? <select value={inputs[variable.name] ?? ""} onChange={(event) => { setInputs({ ...inputs, [variable.name]: event.target.value }); setPreview(null); }} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm"><option value="" /><option value="true">true</option><option value="false">false</option></select> : <input type={variable.type === "secret" ? "password" : variable.type === "integer" ? "number" : "text"} value={inputs[variable.name] ?? ""} onChange={(event) => { setInputs({ ...inputs, [variable.name]: event.target.value }); setPreview(null); }} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm" />}</label>
          </div>
        ))}
        <div className="flex flex-wrap gap-2"><Button kind="primary" disabled={busy || draft.name.trim() === "" || draft.command === ""} onClick={() => void save()}>{t("snippets.save")}</Button>{current === null ? null : <Button disabled={busy} onClick={() => void removeCurrent()}>{t("snippets.delete")}</Button>}</div>
      </div>

      <div className="flex min-w-0 flex-col gap-3">
        <section className="rounded-lg border border-line bg-card p-4"><h3 className="text-sm font-medium">{t("snippets.targets")}</h3><div className="mt-2 grid max-h-40 grid-cols-2 gap-1 overflow-auto">{aliases.map((alias) => <label key={alias} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={targets.includes(alias)} onChange={(event) => { setTargets(event.target.checked ? [...targets, alias] : targets.filter((value) => value !== alias)); setPreview(null); }} />{alias}</label>)}</div><Button className="mt-3" disabled={busy || selected === null || targets.length === 0} onClick={() => void makePreview()}>{t("snippets.preview")}</Button></section>
        {preview === null ? null : <section className="rounded-lg border border-notice-line bg-notice p-4"><h3 className="text-sm font-medium">{t("snippets.confirm")}</h3>{preview.targets.map((target) => <div key={target.targetId} className="mt-2"><p className="text-xs font-medium">{target.target.alias} · {target.target.user}@{target.target.hostName}:{target.target.port}</p>{(target.target.route ?? []).map((hop, index) => <p key={`${hop.alias}-${index}`} className="mt-1 break-all text-xs text-notice-ink">{index + 1}. {hop.user}@{hop.hostName}:{hop.port}{hop.proxyCommand === "" ? "" : ` · ProxyCommand: ${hop.proxyCommand}`}</p>)}<pre className="mt-1 overflow-auto rounded bg-code-bg p-2 text-code-fg text-xs">{target.command}</pre></div>)}<Button kind="primary" className="mt-3" onClick={() => void run()}>{t("snippets.run")}</Button></section>}
        {job === null ? null : <section className="rounded-lg border border-line bg-card p-4"><div className="flex items-center"><h3 className="grow text-sm font-medium">{t("snippets.results")} · {t(snippetStatusLabelKey(job.status))}</h3>{job.status === "running" ? <Button onClick={() => void snippetsApi.cancel(job.id).then(setJob)}>{t("snippets.cancel")}</Button> : null}</div>{job.results.map((result) => <div key={result.targetId} className="mt-3"><p className="text-xs font-medium">{result.alias} · {t(snippetStatusLabelKey(result.status))}{result.exitCode === undefined ? "" : ` (${result.exitCode})`}</p>{result.stdout ? <pre className="mt-1 max-h-40 overflow-auto rounded bg-code-bg p-2 text-code-fg text-xs">{result.stdout}</pre> : null}{result.stderr ? <pre className="mt-1 max-h-40 overflow-auto rounded bg-danger/10 p-2 text-xs text-danger">{result.stderr}</pre> : null}</div>)}</section>}
        <section className="rounded-lg border border-line bg-card p-4"><h3 className="text-sm font-medium">{t("snippets.startup")}</h3><p className="mt-1 text-xs text-ink-muted">{t("snippets.startupHint")}</p><select value={startupAlias} onChange={(event) => setStartupAlias(event.target.value)} className="mt-2 w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm">{aliases.map((alias) => <option key={alias}>{alias}</option>)}</select><Button className="mt-2" disabled={busy || selected === null || startupAlias === ""} onClick={() => selected === null ? undefined : void updateStartup(selected)}>{t("snippets.setStartup")}</Button><button disabled={busy || startupAlias === ""} className="ml-2 text-xs text-ink-muted disabled:opacity-50" onClick={() => void updateStartup("")}>{t("snippets.clearStartup")}</button></section>
      </div>
    </section>
  );
}

export { placeholders, variablesFor };
