import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { Icon } from "../ui/icons";
import type { TerminalSession } from "../api/terminalSessions";
import { failureCode } from "../api/client";
import { useTranslate } from "../i18n/context";
import { snippetsApi, type Snippet } from "../snippets/api";
import { clipboard } from "../ui/clipboard";
import { terminalCommandApi, type TerminalCommandApi, type TerminalCommandPreview, type TerminalCommandRequest } from "./commands/commandApi";
import { describeTerminalCommandFailure, terminalCommandProblemText } from "./commands/terminalCommandProblem";
import { describeSnippetFailure } from "../snippets/snippetFailure";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { SnippetVariableInputs } from "../snippets/SnippetVariableInputs";
import { Button, Notice } from "../ui/surface";
import { codeBlock, control, iconAction } from "../ui/form";

type Prepared = {
  command: string;
  preview: TerminalCommandPreview;
  request: TerminalCommandRequest;
};

export function TerminalQuickCommands({
  session,
  onClose,
  onCopied,
  initialCommand = "",
  returnFocusRef,
  api = terminalCommandApi,
}: {
  session: TerminalSession;
  // api is the command API of the engine the session runs on (terminalCommandApiFor).
  api?: TerminalCommandApi;
  onClose: () => void;
  // コピーは画面に何も起きないので、閉じたあとに呼び手が「コピーしました」を出す。
  onCopied: () => void;
  initialCommand?: string;
  returnFocusRef?: RefObject<HTMLElement | null>;
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
  const selected = useMemo(
    () => snippets.find((snippet) => snippet.id === selectedId) ?? null,
    [selectedId, snippets],
  );

  // 表示の言語を変えると読み直すので、選んでいたスニペットは残す。
  useEffect(() => {
    let active = true;
    void snippetsApi
      .library()
      .then((library) => {
        if (!active) return;
        setSnippets(library.snippets);
        setSelectedId((current) => current || (library.snippets[0]?.id ?? ""));
      })
      .catch(() => active && setProblem(t("snippets.loadFailed")))
      .finally(() => active && setBusy(false));
    return () => {
      active = false;
    };
  }, [t]);

  // キーボードで「…」から開いた人がパネルへ入れるように、開いたらパネルへフォーカスを
  // 移し、Tab をパネルの中で回す。外へ出すと xterm が Tab をシェルへ送ってしまい、
  // 戻る道がない。スニペットの select は読み込みの後に出るので、最初の行き先は
  // 描画直後からあるパネル自身にする。
  useDismissibleLayer({
    open: true,
    containerRefs: [panel],
    onDismiss: onClose,
    initialFocusRef: panel,
    trapFocus: true,
    ...(returnFocusRef === undefined ? {} : { returnFocusRef }),
  });

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
      const saved = await snippetsApi.create({
        name,
        command,
        description: "",
        variables: [],
      });
      setSnippets((current) => [saved, ...current]);
      setSelectedId(saved.id);
      setSaveOpen(false);
      setSaveName("");
      setTerminalSelectionSaved(true);
    } catch (error) {
      setProblem(describeSnippetFailure(t, error));
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
      const preview = await api.preview({
        ...request,
        issueAction: false,
      });
      const target =
        preview.targets.find((item) => item.sessionId === session.id) ??
        preview.targets[0];
      if (target === undefined || target.command === "")
        throw new Error("invalid_response");
      setPrepared({ command: target.command, preview, request });
    } catch (error) {
      setProblem(describeTerminalCommandFailure(t, error));
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
      onCopied();
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
    const revealed = await api.preview({
      ...prepared.request,
      revealCommand: true,
      issueAction: false,
      expectedReviewEvidence: prepared.preview.reviewEvidence,
    });
    const target =
      revealed.targets.find((item) => item.sessionId === session.id) ??
      revealed.targets[0];
    if (target === undefined || target.command === "")
      throw new Error("invalid_response");
    return target.command;
  }

  async function insertPrepared() {
    if (prepared === null) return;
    const current = prepared;
    setBusy(true);
    setProblem("");
    try {
      const confirmed = await api.preview({
        ...current.request,
        issueAction: true,
        submit: false,
        expectedReviewEvidence: current.preview.reviewEvidence,
      });
      await api.dispatch(confirmed, current.request, false);
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
    setBusy(true);
    setProblem("");
    try {
      const confirmed = await api.preview({
        ...current.request,
        issueAction: true,
        submit: true,
        expectedReviewEvidence: current.preview.reviewEvidence,
      });
      await api.dispatch(confirmed, current.request, true);
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
      setProblem(terminalCommandProblemText(t, code));
      return;
    }
    try {
      const preview = await api.preview({
        ...current.request,
        issueAction: false,
      });
      const target =
        preview.targets.find((item) => item.sessionId === session.id) ??
        preview.targets[0];
      if (target === undefined || target.command === "")
        throw new Error("invalid_response");
      setPrepared({
        command: target.command,
        preview,
        request: current.request,
      });
      setProblem(t("terminal.quickCommandChanged"));
    } catch (refreshError) {
      setPrepared(null);
      setProblem(describeTerminalCommandFailure(t, refreshError));
    }
  }

  return (
    <div
      ref={panel}
      role="dialog"
      aria-label={t("terminal.quickCommands")}
      tabIndex={-1}
      className="focus:outline-none absolute right-2 top-11 z-30 flex max-h-[min(32rem,75vh)] w-[min(24rem,calc(100vw-1rem))] flex-col gap-3 overflow-auto rounded-lg border border-line bg-card p-3 shadow-float"
    >
      <div className="flex items-center gap-2">
        <h3 className="grow text-sm font-semibold">
          {t("terminal.quickCommands")}
        </h3>
        <button
          type="button"
          aria-label={t("terminal.quickCommandsClose")}
          className={`${iconAction} size-8`}
          onClick={onClose}
        >
          <Icon name="close" className="size-3.5" />
        </button>
      </div>
      {problem === "" ? null : <Notice tone="danger" compact>{problem}</Notice>}
      {terminalSelectionSaved ? (
        <Notice compact>{t("terminal.quickCommandSaved")}</Notice>
      ) : null}
      {initialCommand.trim() === "" ? null : saveOpen ? (
        <div className="grid gap-2 rounded-md border border-line p-2">
          <label className="text-xs text-ink-muted">
            {t("terminal.quickCommandName")}
            <input
              autoFocus
              value={saveName}
              onChange={(event) => setSaveName(event.target.value)}
              className={`mt-1 block ${control}`}
            />
          </label>
          <pre className={`max-h-24 whitespace-pre-wrap ${codeBlock}`}>
            {initialCommand}
          </pre>
          <div className="flex gap-2">
            <Button kind="primary" disabled={busy || saveName.trim() === ""} onClick={() => void saveSelection()}>
              {t("terminal.quickCommandSave")}
            </Button>
            <Button onClick={() => setSaveOpen(false)}>
              {t("snippets.cancel")}
            </Button>
          </div>
        </div>
      ) : (
        <Button className="self-start" onClick={() => setSaveOpen(true)}>
          {t("terminal.quickCommandSaveSelection")}
        </Button>
      )}
      {busy && snippets.length === 0 ? (
        <p className="text-xs text-ink-muted">{t("palette.loading")}</p>
      ) : snippets.length === 0 ? (
        <p className="text-xs text-ink-muted">{t("snippets.empty")}</p>
      ) : (
        <>
          <label className="text-xs text-ink-muted">
            {t("workspace.savedSnippet")}
            <select
              value={selectedId}
              onChange={(event) => {
                setSelectedId(event.target.value);
                invalidate({});
              }}
              className={`mt-1 block ${control}`}
            >
              {snippets.map((snippet) => (
                <option key={snippet.id} value={snippet.id}>
                  {snippet.name}
                </option>
              ))}
            </select>
          </label>
          {selected?.description ? (
            <p className="text-xs text-ink-muted">{selected.description}</p>
          ) : null}
          {selected === null ? null : (
            <SnippetVariableInputs
              variables={selected.variables}
              inputs={inputs}
              onChange={(variable, value) => invalidate({ ...inputs, [variable.name]: value })}
            />
          )}
          {prepared === null ? (
            <Button className="self-start" disabled={busy || selected === null} onClick={() => void prepare()}>
              {t("snippets.preview")}
            </Button>
          ) : (
            <>
              <pre className={`max-h-32 whitespace-pre-wrap ${codeBlock}`}>
                {prepared.command}
              </pre>
              <p className="text-[11px] leading-4 text-ink-muted">
                {t("terminal.quickCommandContextWarning")}
              </p>
              <div className="flex flex-wrap gap-2">
                <Button disabled={busy} onClick={() => void insertPrepared()}>
                  {t("terminal.quickCommandInsert")}
                </Button>
                <Button kind="primary" disabled={busy} onClick={() => void runPrepared()}>
                  {t("terminal.quickCommandRun")}
                </Button>
                <Button disabled={busy} onClick={() => void copyPrepared()}>
                  {t("terminal.quickCommandCopy")}
                </Button>
              </div>
            </>
          )}
        </>
      )}
    </div>
  );
}
