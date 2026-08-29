import { useEffect, useMemo, useRef, useState } from "react";
import type { TerminalSession } from "../api/integrations";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { snippetsApi, type Snippet } from "../snippets/api";
import { clipboard } from "../ui/clipboard";
import { terminalCommandApi } from "../features/workspaces/commandApi";

export function TerminalQuickCommands({
  session,
  onSend,
  onClose,
}: {
  session: TerminalSession;
  onSend: (command: string, submit: boolean) => void;
  onClose: () => void;
}) {
  const t = useTranslate();
  const panel = useRef<HTMLDivElement>(null);
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [prepared, setPrepared] = useState("");
  const [busy, setBusy] = useState(true);
  const [problem, setProblem] = useState("");
  const selected = useMemo(() => snippets.find((snippet) => snippet.id === selectedId) ?? null, [selectedId, snippets]);
  const hasSecret = selected?.variables.some((variable) => variable.type === "secret") === true;

  useEffect(() => {
    let active = true;
    void snippetsApi.library()
      .then((library) => {
        if (!active) return;
        setSnippets(library.snippets);
        setSelectedId(library.snippets[0]?.id ?? "");
      })
      .catch((error: unknown) => active && setProblem(failureCode(error) || "snippet_failed"))
      .finally(() => active && setBusy(false));
    return () => { active = false; };
  }, []);

  useEffect(() => {
    function dismiss(event: PointerEvent) {
      if (!panel.current?.contains(event.target as Node)) onClose();
    }
    function escape(event: KeyboardEvent) {
      if (event.key === "Escape") onClose();
    }
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", dismiss);
      document.removeEventListener("keydown", escape);
    };
  }, [onClose]);

  function invalidate(nextInputs = inputs) {
    setInputs(nextInputs);
    setPrepared("");
    setProblem("");
  }

  async function prepare() {
    if (selected === null || hasSecret || busy) return;
    setBusy(true);
    setProblem("");
    try {
      const preview = await terminalCommandApi.preview({
        snippetId: selected.id,
        inputs,
        targets: [{ targetId: session.id, sessionId: session.id }],
      });
      const target = preview.targets.find((item) => item.sessionId === session.id) ?? preview.targets[0];
      if (target === undefined || target.command === "") throw new Error("invalid_response");
      setPrepared(target.command);
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "terminal_command_failed"));
      setPrepared("");
    } finally {
      setBusy(false);
    }
  }

  async function copyPrepared() {
    if (prepared === "") return;
    try {
      await clipboard.writeText(prepared);
      onClose();
    } catch {
      setProblem(t("terminal.clipboardRefused"));
    }
  }

  function send(submit: boolean) {
    if (prepared === "") return;
    onSend(prepared, submit);
    onClose();
  }

  return (
    <div ref={panel} role="dialog" aria-label={t("terminal.quickCommands")} className="absolute right-2 top-11 z-30 flex max-h-[min(32rem,75vh)] w-[min(24rem,calc(100vw-1rem))] flex-col gap-3 overflow-auto rounded-md border border-control-line bg-card p-3 shadow-2xl">
      <div className="flex items-center gap-2">
        <h3 className="grow text-sm font-semibold">{t("terminal.quickCommands")}</h3>
        <button type="button" aria-label={t("terminal.quickCommandsClose")} className="rounded px-2 text-ink-muted hover:bg-select-fill" onClick={onClose}>×</button>
      </div>
      {problem === "" ? null : <p role="alert" className="rounded bg-notice px-2 py-1.5 text-xs text-notice-ink">{problem}</p>}
      {busy && snippets.length === 0 ? <p className="text-xs text-ink-muted">{t("palette.loading")}</p> : snippets.length === 0 ? (
        <p className="text-xs text-ink-muted">{t("snippets.empty")}</p>
      ) : (
        <>
          <label className="text-xs text-ink-muted">{t("workspace.savedSnippet")}
            <select value={selectedId} onChange={(event) => { setSelectedId(event.target.value); invalidate({}); }} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm text-ink">
              {snippets.map((snippet) => <option key={snippet.id} value={snippet.id}>{snippet.name}</option>)}
            </select>
          </label>
          {selected?.description ? <p className="text-xs text-ink-muted">{selected.description}</p> : null}
          {selected?.variables.map((variable) => (
            <label key={variable.name} className="text-xs text-ink-muted">
              <code>{`{{${variable.name}}}`}</code>{variable.description ? ` · ${variable.description}` : ""}
              {variable.type === "boolean" ? (
                <select value={inputs[variable.name] ?? ""} onChange={(event) => invalidate({ ...inputs, [variable.name]: event.target.value })} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm text-ink">
                  <option value="">{variable.default === undefined ? "" : `${t("workspace.useDefault")} (${variable.default})`}</option>
                  <option value="true">true</option><option value="false">false</option>
                </select>
              ) : (
                <input
                  type={variable.type === "secret" ? "password" : variable.type === "integer" ? "number" : "text"}
                  value={inputs[variable.name] ?? ""}
                  placeholder={variable.default === undefined ? "" : `${t("workspace.useDefault")}: ${variable.default}`}
                  onChange={(event) => invalidate({ ...inputs, [variable.name]: event.target.value })}
                  className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm text-ink"
                />
              )}
            </label>
          ))}
          {hasSecret ? <p className="rounded bg-notice px-2 py-1.5 text-xs text-notice-ink">{t("workspace.secretSnippetRefused")}</p> : null}
          {prepared === "" ? (
            <button type="button" disabled={busy || selected === null || hasSecret} onClick={() => void prepare()} className="min-h-8 self-start rounded border border-control-line bg-control px-3 py-1 text-sm font-medium hover:bg-select-fill disabled:opacity-50">{t("snippets.preview")}</button>
          ) : (
            <>
              <pre className="max-h-32 overflow-auto whitespace-pre-wrap rounded bg-code-bg p-2 text-xs text-code-fg">{prepared}</pre>
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={() => send(false)} className="min-h-8 rounded border border-control-line bg-control px-3 py-1 text-sm hover:bg-select-fill">{t("terminal.quickCommandInsert")}</button>
                <button type="button" onClick={() => send(true)} className="min-h-8 rounded bg-accent px-3 py-1 text-sm font-medium text-accent-contrast">{t("terminal.quickCommandRun")}</button>
                <button type="button" onClick={() => void copyPrepared()} className="min-h-8 rounded border border-control-line px-3 py-1 text-sm hover:bg-select-fill">{t("terminal.quickCommandCopy")}</button>
              </div>
            </>
          )}
        </>
      )}
    </div>
  );
}
