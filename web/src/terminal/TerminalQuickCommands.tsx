import { useEffect, useMemo, useRef, useState } from "react";
import type { TerminalSession } from "../api/integrations";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { snippetsApi, type Snippet } from "../snippets/api";
import { clipboard } from "../ui/clipboard";
import { terminalCommandApi } from "../features/workspaces/commandApi";

type Prepared = {
  command: string;
  preview: Awaited<ReturnType<typeof terminalCommandApi.preview>>;
  request: Parameters<typeof terminalCommandApi.preview>[0];
};

export function TerminalQuickCommands({
  session,
  onClose,
  initialCommand = "",
}: {
  session: TerminalSession;
  onClose: () => void;
  initialCommand?: string;
}) {
  const t = useTranslate();
  const panel = useRef<HTMLDivElement>(null);
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [prepared, setPrepared] = useState<Prepared | null>(null);
  const [busy, setBusy] = useState(true);
  const [problem, setProblem] = useState("");
  const [saveOpen, setSaveOpen] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [terminalSelectionSaved, setTerminalSelectionSaved] = useState(false);
  const selected = useMemo(() => snippets.find((snippet) => snippet.id === selectedId) ?? null, [selectedId, snippets]);

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

  useEffect(() => {
    setPrepared(null);
  }, [session.id, session.startedAt, session.state]);

  function invalidate(nextInputs = inputs) {
    setInputs(nextInputs);
    setPrepared(null);
    setProblem("");
  }

  async function saveSelection() {
    const name = saveName.trim();
    const command = initialCommand;
    if (name === "" || command.trim() === "" || busy) return;
    setBusy(true);
    setProblem("");
    try {
      const saved = await snippetsApi.create({ name, command, description: "", variables: [] });
      setSnippets((current) => [saved, ...current]);
      setSelectedId(saved.id);
      setSaveOpen(false);
      setSaveName("");
      setTerminalSelectionSaved(true);
    } catch (error) {
      setProblem(failureCode(error) || "snippet_failed");
    } finally {
      setBusy(false);
    }
  }

  async function prepare() {
    if (selected === null || busy) return;
    setBusy(true);
    setProblem("");
    try {
      const request = {
        snippetId: selected.id,
        inputs,
        targets: [{ targetId: session.id, sessionId: session.id }],
      };
      const preview = await terminalCommandApi.preview({ ...request, issueAction: false });
      const target = preview.targets.find((item) => item.sessionId === session.id) ?? preview.targets[0];
      if (target === undefined || target.command === "") throw new Error("invalid_response");
      setPrepared({ command: target.command, preview, request });
    } catch (error) {
      setProblem(failureCode(error) || (error instanceof Error ? error.message : "terminal_command_failed"));
      setPrepared(null);
    } finally {
      setBusy(false);
    }
  }

  async function copyPrepared() {
    if (prepared === null) return;
    const current = prepared;
    setBusy(true);
    try {
      await clipboard.writeText(await revealPrepared());
      onClose();
    } catch (error) {
      if (failureCode(error) === "terminal_command_preview_changed") {
        await handleActionFailure(error, current);
      } else {
        setProblem(t("terminal.clipboardRefused"));
      }
    } finally {
      setBusy(false);
    }
  }

  async function revealPrepared(): Promise<string> {
    if (prepared === null) throw new Error("invalid_response");
    const revealed = await terminalCommandApi.preview({
      ...prepared.request,
      revealCommand: true,
      issueAction: false,
      expectedReviewEvidence: prepared.preview.reviewEvidence,
    });
    const target = revealed.targets.find((item) => item.sessionId === session.id) ?? revealed.targets[0];
    if (target === undefined || target.command === "") throw new Error("invalid_response");
    return target.command;
  }

  async function insertPrepared() {
    if (prepared === null) return;
    const current = prepared;
    setBusy(true); setProblem("");
    try {
      const confirmed = await terminalCommandApi.preview({
        ...current.request,
        issueAction: true,
        submit: false,
        expectedReviewEvidence: current.preview.reviewEvidence,
      });
      await terminalCommandApi.dispatch(confirmed, current.request, false);
      onClose();
    } catch (error) {
      await handleActionFailure(error, current);
    } finally {
      setBusy(false);
    }
  }

  async function runPrepared() {
    if (prepared === null) return;
    const current = prepared;
    setBusy(true); setProblem("");
    try {
      const confirmed = await terminalCommandApi.preview({
        ...current.request,
        issueAction: true,
        submit: true,
        expectedReviewEvidence: current.preview.reviewEvidence,
      });
      await terminalCommandApi.dispatch(confirmed, current.request, true);
      onClose();
    } catch (error) {
      await handleActionFailure(error, current);
    } finally {
      setBusy(false);
    }
  }

  async function handleActionFailure(error: unknown, current: Prepared) {
    const code = failureCode(error);
    if (code !== "terminal_command_preview_changed") {
      setProblem(code === "terminal_command_insert_unsafe" ? t("terminal.quickCommandInsertUnsafe") : code || "terminal_command_failed");
      return;
    }
    try {
      const preview = await terminalCommandApi.preview({ ...current.request, issueAction: false });
      const target = preview.targets.find((item) => item.sessionId === session.id) ?? preview.targets[0];
      if (target === undefined || target.command === "") throw new Error("invalid_response");
      setPrepared({ command: target.command, preview, request: current.request });
      setProblem(t("terminal.quickCommandChanged"));
    } catch (refreshError) {
      setPrepared(null);
      setProblem(failureCode(refreshError) || "terminal_command_failed");
    }
  }

  return (
    <div ref={panel} role="dialog" aria-label={t("terminal.quickCommands")} className="absolute right-2 top-11 z-30 flex max-h-[min(32rem,75vh)] w-[min(24rem,calc(100vw-1rem))] flex-col gap-3 overflow-auto rounded-md border border-control-line bg-card p-3 shadow-2xl">
      <div className="flex items-center gap-2">
        <h3 className="grow text-sm font-semibold">{t("terminal.quickCommands")}</h3>
        <button type="button" aria-label={t("terminal.quickCommandsClose")} className="rounded px-2 text-ink-muted hover:bg-select-fill" onClick={onClose}>×</button>
      </div>
      {problem === "" ? null : <p role="alert" className="rounded bg-notice px-2 py-1.5 text-xs text-notice-ink">{problem}</p>}
      {terminalSelectionSaved ? <p role="status" className="rounded bg-notice px-2 py-1.5 text-xs text-notice-ink">{t("terminal.quickCommandSaved")}</p> : null}
      {initialCommand.trim() === "" ? null : saveOpen ? (
        <div className="grid gap-2 rounded border border-line p-2">
          <label className="text-xs text-ink-muted">
            {t("terminal.quickCommandName")}
            <input autoFocus value={saveName} onChange={(event) => setSaveName(event.target.value)} className="mt-1 block w-full rounded border border-control-line bg-control px-2 py-1.5 text-sm text-ink" />
          </label>
          <pre className="max-h-24 overflow-auto whitespace-pre-wrap rounded bg-code-bg p-2 text-xs text-code-fg">{initialCommand}</pre>
          <div className="flex gap-2">
            <button type="button" disabled={busy || saveName.trim() === ""} onClick={() => void saveSelection()} className="min-h-8 rounded bg-accent px-3 py-1 text-sm font-medium text-accent-contrast disabled:opacity-50">{t("terminal.quickCommandSave")}</button>
            <button type="button" onClick={() => setSaveOpen(false)} className="min-h-8 rounded border border-control-line px-3 py-1 text-sm hover:bg-select-fill">{t("snippets.cancel")}</button>
          </div>
        </div>
      ) : (
        <button type="button" className="min-h-8 self-start rounded border border-control-line px-3 py-1 text-sm hover:bg-select-fill" onClick={() => setSaveOpen(true)}>{t("terminal.quickCommandSaveSelection")}</button>
      )}
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
          {prepared === null ? (
            <button type="button" disabled={busy || selected === null} onClick={() => void prepare()} className="min-h-8 self-start rounded border border-control-line bg-control px-3 py-1 text-sm font-medium hover:bg-select-fill disabled:opacity-50">{t("snippets.preview")}</button>
          ) : (
            <>
              <pre className="max-h-32 overflow-auto whitespace-pre-wrap rounded bg-code-bg p-2 text-xs text-code-fg">{prepared.command}</pre>
              <p className="text-[11px] leading-4 text-ink-muted">{t("terminal.quickCommandContextWarning")}</p>
              <div className="flex flex-wrap gap-2">
                <button type="button" disabled={busy} onClick={() => void insertPrepared()} className="min-h-8 rounded border border-control-line bg-control px-3 py-1 text-sm hover:bg-select-fill disabled:opacity-50">{t("terminal.quickCommandInsert")}</button>
                <button type="button" disabled={busy} onClick={() => void runPrepared()} className="min-h-8 rounded bg-accent px-3 py-1 text-sm font-medium text-accent-contrast disabled:opacity-50">{t("terminal.quickCommandRun")}</button>
                <button type="button" disabled={busy} onClick={() => void copyPrepared()} className="min-h-8 rounded border border-control-line px-3 py-1 text-sm hover:bg-select-fill disabled:opacity-50">{t("terminal.quickCommandCopy")}</button>
              </div>
            </>
          )}
        </>
      )}
    </div>
  );
}
