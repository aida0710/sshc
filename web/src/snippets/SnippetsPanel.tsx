import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { control } from "../ui/form";
import { PageHeader } from "../ui/page";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation, type RunOptions } from "../ui/useAsyncOperation";
import { usePendingAction } from "../ui/usePendingAction";
import { PasswordInput } from "../ui/PasswordField";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import type { NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import { useDraftDiscardConfirmation } from "../routing/useDraftDiscardConfirmation";
import { PanelState } from "../ui/PanelState";
import { usePolling } from "../ui/usePolling";
import { describeSnippetFailure } from "./snippetFailure";
import {
  snippetsApi,
  type Job,
  type Preview,
  type Snippet,
  type SnippetDraft,
  type SnippetVariable,
  type Startup,
} from "./api";

// A command's per-host output should feel live without hammering the engine.
const snippetJobPollIntervalMs = 600;

function placeholders(command: string): string[] {
  return [
    ...new Set(
      [...command.matchAll(/\{\{([A-Za-z_][A-Za-z0-9_]{0,63})\}\}/g)]
        .map((match) => match[1] ?? "")
        .filter(Boolean),
    ),
  ];
}

function variablesFor(
  command: string,
  previous: SnippetVariable[],
): SnippetVariable[] {
  const byName = new Map(previous.map((variable) => [variable.name, variable]));
  return placeholders(command).map(
    (name) => byName.get(name) ?? { name, type: "string", required: true },
  );
}

export function snippetVariableTypeLabelKey(type: string): MessageKey {
  switch (type) {
    case "string":
      return "snippets.variableType.string";
    case "integer":
      return "snippets.variableType.integer";
    case "boolean":
      return "snippets.variableType.boolean";
    case "secret":
      return "snippets.variableType.secret";
    default:
      return "snippets.variableType.unknown";
  }
}

export function snippetStatusLabelKey(status: string): MessageKey {
  switch (status) {
    case "running":
      return "snippets.status.running";
    case "completed":
      return "snippets.status.completed";
    case "cancelled":
      return "snippets.status.cancelled";
    case "queued":
      return "snippets.status.queued";
    case "succeeded":
      return "snippets.status.succeeded";
    case "failed":
      return "snippets.status.failed";
    default:
      return "snippets.status.unknown";
  }
}

// snippetProblemLabelKey は、ホストごとの結果が失敗した理由（API の固定コード）を
// 画面の文言にする。知らないコードには文言を出さず、状態と終了コードだけを見せる。
export function snippetProblemLabelKey(problem: string | undefined): MessageKey | undefined {
  switch (problem) {
    case "timed_out":
      return "snippets.problem.timedOut";
    case "cancelled":
      return "snippets.problem.cancelled";
    case "run_failed":
      return "snippets.problem.runFailed";
    default:
      return undefined;
  }
}

// SnippetAction は、この画面が engine に頼む操作である。一度に一つだけ走らせ、押したボタン
// だけに回転の印を出す。
type SnippetAction = "save" | "remove" | "preview" | "run" | "cancel" | "setStartup" | "clearStartup";

const blank: SnippetDraft = {
  name: "",
  description: "",
  command: "",
  variables: [],
};

// draftOf は、保存済みのスニペット（新規なら空）を編集欄の形にする。未保存の変更の
// 有無は、編集欄をこれと比べて決める。
function draftOf(snippet: Snippet | null): SnippetDraft {
  return snippet === null
    ? blank
    : {
        name: snippet.name,
        description: snippet.description ?? "",
        command: snippet.command,
        variables: snippet.variables,
      };
}

export function SnippetsPanel({
  aliases,
  selectedSnippetId = null,
  onNavigationBlockerChange,
  onNavigateLocation,
}: {
  aliases: string[];
  selectedSnippetId?: string | null;
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onNavigateLocation?: ((url: string, options?: NavigateLocationOptions) => void) | undefined;
}) {
  const t = useTranslate();
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [startupAssignments, setStartupAssignments] = useState<Startup[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [draft, setDraft] = useState<SnippetDraft>(blank);
  const [targets, setTargets] = useState<string[]>([]);
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [preview, setPreview] = useState<Preview | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [chosenStartupAlias, setChosenStartupAlias] = useState("");
  const [libraryState, setLibraryState] = useState<"loading" | "ready" | "failed">("loading");
  const [confirmingDelete, setConfirmingDelete] = useState(false);
  const operation = useAsyncOperation();
  const { clearError, touch } = operation;
  const pending = usePendingAction<SnippetAction>();
  const busy = pending.running !== null;
  // 保存できたことは、次に編集欄を変えるか、別の操作を始めるまで編集欄に出す。operation.saved は
  // どの操作が成功しても立つので、最後に始めたのが保存だったかを覚えておく。
  const [lastStarted, setLastStarted] = useState<SnippetAction | null>(null);
  const savedJustNow = operation.saved && lastStarted === "save";
  const describeFailure = (error: unknown) => describeSnippetFailure(t, error);
  // ホストの一覧は後から届くので、選んだホストが一覧に無いあいだは先頭を選んでいるとみなす。
  // select が表示するホストと、ボタンで送るホストを同じ値にする。
  const startupAlias = aliases.includes(chosenStartupAlias) ? chosenStartupAlias : (aliases[0] ?? "");
  const current = useMemo(
    () => snippets.find((snippet) => snippet.id === selected) ?? null,
    [snippets, selected],
  );
  const staleStartupAliases = startupAssignments
    .filter((assignment) => assignment.stale)
    .map((assignment) => assignment.alias);
  const startupStale = staleStartupAliases.includes(startupAlias);
  const draftChanged = JSON.stringify(draft) !== JSON.stringify(draftOf(current));
  const draftDiscard = useDraftDiscardConfirmation({
    dirty: draftChanged,
    discard: () => setDraft(draftOf(current)),
    onNavigationBlockerChange,
    onNavigateLocation,
  });

  const reload = useCallback(async () => {
    const library = await snippetsApi.library();
    setSnippets(library.snippets);
    setStartupAssignments(library.startup);
    setLibraryState("ready");
  }, []);

  // 読めなかった一覧を「保存済みスニペットはありません」と見せないように、読み込みの
  // 状態を持つ。
  const loadLibrary = useCallback(async () => {
    setLibraryState("loading");
    try {
      await reload();
    } catch {
      setLibraryState("failed");
    }
  }, [reload]);

  useEffect(() => {
    void loadLibrary();
  }, [loadLibrary]);

  const edit = useCallback((snippet: Snippet | null) => {
    setSelected(snippet?.id ?? null);
    setDraft(draftOf(snippet));
    setInputs({});
    setPreview(null);
    setJob(null);
    clearError();
    touch();
  }, [clearError, touch]);

  // changeDraft は、編集欄の入力を変える。変えた時点で、前の保存の成功はもう今の入力を指さない。
  function changeDraft(next: SnippetDraft) {
    setDraft(next);
    touch();
  }

  // 失敗は operation が文にして編集欄の上に出すので、pending.start は断られても reject しない。
  function perform<T>(action: SnippetAction, work: () => Promise<T>, options: RunOptions<T> = {}) {
    return pending.start(action, () => {
      setLastStarted(action);
      return operation.run(work, { describe: describeFailure, ...options });
    });
  }

  useEffect(() => {
    if (selectedSnippetId === null) return;
    const snippet = snippets.find(
      (candidate) => candidate.id === selectedSnippetId,
    );
    if (snippet !== undefined && selected !== snippet.id) edit(snippet);
  }, [edit, selected, selectedSnippetId, snippets]);

  // Choosing another snippet, previewing again or cancelling stops the polling
  // while a question may still be on its way. Its answer is dropped, so it
  // cannot bring back a job the user has left under the snippet now shown.
  usePolling(async (isCurrent) => {
    if (job === null) return;
    const polled = await snippetsApi.job(job.id);
    if (isCurrent()) setJob(polled);
  }, { intervalMs: snippetJobPollIntervalMs, enabled: job?.status === "running", whileHidden: true });

  async function save() {
    await perform("save", async () => {
      const value = {
        ...draft,
        variables: variablesFor(draft.command, draft.variables),
      };
      const saved =
        selected === null
          ? await snippetsApi.create(value)
          : await snippetsApi.update(selected, value);
      await reload();
      edit(saved);
    });
  }

  async function makePreview() {
    if (selected === null || targets.length === 0) return;
    setJob(null);
    await perform("preview", () => snippetsApi.preview(selected, targets, inputs), { apply: setPreview });
  }

  async function run() {
    if (preview === null) return;
    await perform("run", async () => {
      setJob(await snippetsApi.start(preview, targets, inputs));
      setPreview(null);
    });
  }

  // 取り消しの前にジョブが終わると、サーバーは 409 snippet_job_finished で断る。
  async function cancelJob(jobId: string) {
    await perform("cancel", () => snippetsApi.cancel(jobId), { apply: setJob });
  }

  function updateVariable(name: string, update: Partial<SnippetVariable>) {
    changeDraft({
      ...draft,
      variables: variablesFor(draft.command, draft.variables).map((variable) =>
        variable.name === name ? { ...variable, ...update } : variable,
      ),
    });
    setPreview(null);
  }

  async function removeCurrent() {
    if (current === null) return;
    await perform("remove", async () => {
      await snippetsApi.remove(current.id);
      edit(null);
      await reload();
    });
  }

  async function updateStartup(snippetId: string) {
    if (startupAlias === "") return;
    await perform(snippetId === "" ? "clearStartup" : "setStartup", async () => {
      await snippetsApi.setStartup(
        startupAlias,
        snippetId,
        snippetId === "" ? {} : inputs,
      );
      await reload();
    });
  }

  return (
    <section aria-labelledby="snippets-heading" className="flex flex-col gap-[22px]">
      <PageHeader
        title={t("snippets.heading")}
        headingId="snippets-heading"
        actions={<Button onClick={() => draftDiscard.confirmBefore(() => edit(null))}>{t("snippets.new")}</Button>}
      />
      {/* The list, the editor and the targets sit side by side on a wide screen, divided by lines. */}
      <div className="grid min-w-0 content-start gap-5 xl:grid-cols-[16rem_minmax(0,1fr)_minmax(0,0.8fr)]">
      <div className="flex min-w-0 flex-col gap-2">
        {libraryState === "failed" ? (
          <PanelState
            tone="failed"
            title={t("snippets.loadFailed")}
            action={<Button onClick={() => void loadLibrary()}>{t("shell.bootstrapRetry")}</Button>}
          />
        ) : libraryState === "loading" ? (
          <PanelState tone="loading" title={t("snippets.loading")} />
        ) : null}
        <select
          aria-label={t("snippets.heading")}
          value={selected ?? ""}
          onChange={(event) => {
            const next = snippets.find((snippet) => snippet.id === event.target.value) ?? null;
            draftDiscard.confirmBefore(() => edit(next));
          }}
          className={`${control} xl:hidden`}
        >
          <option value="">{t("snippets.new")}</option>
          {snippets.map((snippet) => <option key={snippet.id} value={snippet.id}>{snippet.name}</option>)}
        </select>
        <div className="hidden flex-col gap-1 xl:flex">
          {snippets.map((snippet) => (
            <button
              key={snippet.id}
              type="button"
              onClick={() => draftDiscard.confirmBefore(() => edit(snippet))}
              className={`rounded-md px-3 py-2 text-left text-sm ${selected === snippet.id ? "bg-select-fill" : "hover:bg-select-fill"}`}
            >
              <span className="block truncate font-medium">{snippet.name}</span>
              <code className="block truncate text-xs text-ink-muted">
                {snippet.command}
              </code>
            </button>
          ))}
          {libraryState === "ready" && snippets.length === 0 ? (
            <p className="text-sm text-ink-muted">{t("snippets.empty")}</p>
          ) : null}
        </div>
      </div>

      <div className="flex min-w-0 flex-col gap-3 border-t border-line pt-5 xl:border-l xl:border-t-0 xl:pl-5 xl:pt-0">
        {operation.error === "" ? null : <Notice tone="danger">{operation.error}</Notice>}
        <label className="text-[12.5px] text-ink-muted">
          {t("snippets.name")}
          <input
            value={draft.name}
            onChange={(event) =>
              changeDraft({ ...draft, name: event.target.value })
            }
            className={`${control} mt-1 block`}
          />
        </label>
        <label className="text-[12.5px] text-ink-muted">
          {t("snippets.description")}
          <input
            value={draft.description}
            onChange={(event) =>
              changeDraft({ ...draft, description: event.target.value })
            }
            className={`${control} mt-1 block`}
          />
        </label>
        <label className="text-[12.5px] text-ink-muted">
          {t("snippets.command")}
          <textarea
            rows={7}
            value={draft.command}
            onChange={(event) => {
              changeDraft({ ...draft, command: event.target.value });
              setPreview(null);
            }}
            className={`${control} mt-1 block font-mono`}
          />
        </label>
        <p className="text-[12.5px] text-ink-muted">{t("snippets.variableHint")}</p>
        {variablesFor(draft.command, draft.variables).map((variable) => (
          <div
            key={variable.name}
            className="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-[8rem_minmax(0,1fr)]"
          >
            <label className="text-[12.5px] text-ink-muted">
              <code>{`{{${variable.name}}}`}</code>
              <select
                aria-label={t("snippets.variableType")}
                value={variable.type}
                onChange={(event) =>
                  updateVariable(variable.name, {
                    type: event.target.value as SnippetVariable["type"],
                  })
                }
                className={`${control} mt-1 block`}
              >
                {(["string", "integer", "boolean", "secret"] as const).map(
                  (type) => (
                    <option key={type} value={type}>
                      {t(snippetVariableTypeLabelKey(type))}
                    </option>
                  ),
                )}
              </select>
            </label>
            {variable.type === "secret" ? (
              <div className="text-[12.5px] text-ink-muted">
                {t("snippets.value")}
                <PasswordInput
                  label={`${variable.name} · ${t("snippets.value")}`}
                  value={inputs[variable.name] ?? ""}
                  onChange={(value) => {
                    setInputs({ ...inputs, [variable.name]: value });
                    setPreview(null);
                  }}
                  className={`${control} mt-1 block`}
                />
              </div>
            ) : (
              <label className="text-[12.5px] text-ink-muted">
                {t("snippets.value")}
                {variable.type === "boolean" ? (
                  <select
                    value={inputs[variable.name] ?? ""}
                    onChange={(event) => {
                      setInputs({
                        ...inputs,
                        [variable.name]: event.target.value,
                      });
                      setPreview(null);
                    }}
                    className={`${control} mt-1 block`}
                  >
                    <option value="" />
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                ) : (
                  <input
                    type={variable.type === "integer" ? "number" : "text"}
                    value={inputs[variable.name] ?? ""}
                    onChange={(event) => {
                      setInputs({
                        ...inputs,
                        [variable.name]: event.target.value,
                      });
                      setPreview(null);
                    }}
                    className={`${control} mt-1 block`}
                  />
                )}
              </label>
            )}
          </div>
        ))}
        {savedJustNow ? <Notice tone="success">{t("snippets.saved")}</Notice> : null}
        <div className="flex flex-wrap gap-2">
          <Button
            kind="primary"
            busy={pending.running === "save"}
            disabled={busy || draft.name.trim() === "" || draft.command === ""}
            onClick={() => void save()}
          >
            {t("snippets.save")}
          </Button>
          {current === null ? null : (
            <Button busy={pending.running === "remove"} disabled={busy} onClick={() => setConfirmingDelete(true)}>
              {t("snippets.delete")}
            </Button>
          )}
        </div>
      </div>

      <div className="flex min-w-0 flex-col gap-4 border-t border-line pt-5 xl:border-l xl:border-t-0 xl:pl-5 xl:pt-0">
        <section>
          <h3 className="text-sm font-medium">{t("snippets.targets")}</h3>
          <div className="mt-2 grid max-h-40 grid-cols-1 gap-1 overflow-auto sm:grid-cols-2">
            {aliases.map((alias) => (
              <label key={alias} className="flex min-w-0 items-center gap-2 break-all text-sm">
                <input
                  type="checkbox"
                  className="size-4 shrink-0 accent-accent"
                  checked={targets.includes(alias)}
                  onChange={(event) => {
                    setTargets(
                      event.target.checked
                        ? [...targets, alias]
                        : targets.filter((value) => value !== alias),
                    );
                    setPreview(null);
                  }}
                />
                {alias}
              </label>
            ))}
          </div>
          <Button
            className="mt-3"
            busy={pending.running === "preview"}
            disabled={busy || selected === null || targets.length === 0}
            onClick={() => void makePreview()}
          >
            {t("snippets.preview")}
          </Button>
        </section>
        {preview === null ? null : (
          <section className="min-w-0 rounded-[10px] bg-notice p-4">
            <h3 className="text-sm font-medium">{t("snippets.confirm")}</h3>
            {preview.targets.map((target) => (
              <div key={target.targetId} className="mt-2">
                <p className="text-xs font-medium">
                  {target.target.alias} · {target.target.user}@
                  {target.target.hostName}:{target.target.port}
                </p>
                {(target.target.route ?? []).map((hop, index) => (
                  <p
                    key={`${hop.alias}-${index}`}
                    className="mt-1 break-all text-xs text-notice-ink"
                  >
                    {index + 1}. {hop.user}@{hop.hostName}:{hop.port}
                    {hop.proxyCommand === ""
                      ? ""
                      : ` · ProxyCommand: ${hop.proxyCommand}`}
                  </p>
                ))}
                <pre className="mt-1 overflow-auto rounded bg-code-bg p-2 text-code-fg text-xs">
                  {target.command}
                </pre>
              </div>
            ))}
            <Button kind="primary" className="mt-3" busy={pending.running === "run"} disabled={busy} onClick={() => void run()}>
              {t("snippets.run")}
            </Button>
          </section>
        )}
        {job === null ? null : (
          <section className="border-t border-line pt-4">
            <div className="flex items-center">
              <h3 className="grow text-sm font-medium">
                {t("snippets.results")} · {t(snippetStatusLabelKey(job.status))}
              </h3>
              {job.status === "running" ? (
                <Button
                  busy={pending.running === "cancel"}
                  disabled={busy}
                  onClick={() => void cancelJob(job.id)}
                >
                  {t("snippets.cancel")}
                </Button>
              ) : null}
            </div>
            {job.results.map((result) => {
              const problemKey = result.status === "failed" ? snippetProblemLabelKey(result.problem) : undefined;
              return (
                <div key={result.targetId} className="mt-3">
                  <p className="text-xs font-medium">
                    {result.alias} · {t(snippetStatusLabelKey(result.status))}
                    {result.exitCode === undefined ? "" : ` (${result.exitCode})`}
                    {problemKey ? ` · ${t(problemKey)}` : ""}
                  </p>
                  {result.stdout ? (
                    <pre className="mt-1 max-h-40 overflow-auto rounded bg-code-bg p-2 text-code-fg text-xs">
                      {result.stdout}
                    </pre>
                  ) : null}
                  {result.stderr ? (
                    <pre className="mt-1 max-h-40 overflow-auto rounded bg-danger/10 p-2 text-xs text-danger">
                      {result.stderr}
                    </pre>
                  ) : null}
                </div>
              );
            })}
          </section>
        )}
        <section className="border-t border-line pt-4">
          <h3 className="text-sm font-medium">{t("snippets.startup")}</h3>
          <p className="mt-1 text-xs text-ink-muted">
            {t("snippets.startupHint")}
          </p>
          {staleStartupAliases.length > 0 ? (
            <div className="mt-2 space-y-1">
              <Notice compact>
                {t("snippets.startupStaleCount", {
                  count: staleStartupAliases.length,
                })}
              </Notice>
              <ul
                aria-label={t("snippets.startupStaleHosts")}
                className="flex flex-wrap gap-1"
              >
                {staleStartupAliases.map((alias) => (
                  <li
                    key={alias}
                    className="rounded border border-line px-1.5 py-0.5 font-mono text-xs text-ink-muted"
                  >
                    {alias}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <select
            aria-label={t("snippets.startupHost")}
            value={startupAlias}
            onChange={(event) => setChosenStartupAlias(event.target.value)}
            className={`${control} mt-2`}
          >
            {aliases.map((alias) => (
              <option key={alias}>{alias}</option>
            ))}
          </select>
          {startupStale ? (
            <div className="mt-2">
              <Notice compact>{t("snippets.startupStale")}</Notice>
            </div>
          ) : null}
          <Button
            className="mt-2"
            busy={pending.running === "setStartup"}
            disabled={busy || selected === null || startupAlias === ""}
            onClick={() =>
              selected === null ? undefined : void updateStartup(selected)
            }
          >
            {t("snippets.setStartup")}
          </Button>
          <button
            disabled={busy || startupAlias === ""}
            className="ml-2 text-xs text-ink-muted transition-colors duration-150 hover:text-ink"
            onClick={() => void updateStartup("")}
          >
            {t("snippets.clearStartup")}
          </button>
        </section>
      </div>
      </div>
      {draftDiscard.confirming ? (
        <DiscardDraftDialog id="snippet-draft-discard-heading" onConfirm={draftDiscard.confirmDiscard} onCancel={draftDiscard.keepEditing} />
      ) : null}
      {confirmingDelete && current !== null ? (
        <ConfirmDialog
          id="snippet-delete-heading"
          heading={t("snippets.deleteHeading", { name: current.name })}
          body={<p className="text-sm text-ink-muted">{t("snippets.deleteBody")}</p>}
          confirmLabel={t("snippets.confirmDelete")}
          cancelLabel={t("snippets.deleteCancel")}
          onCancel={() => setConfirmingDelete(false)}
          onConfirm={() => {
            setConfirmingDelete(false);
            void removeCurrent();
          }}
        />
      ) : null}
    </section>
  );
}

export { placeholders, variablesFor };
