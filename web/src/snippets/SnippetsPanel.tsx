import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { EmptyState } from "../ui/EmptyState";
import { Icon } from "../ui/icons";
import { PageBoard } from "../ui/PageBoard";
import { PageHeader } from "../ui/page";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation, type RunOptions } from "../ui/useAsyncOperation";
import { usePendingAction } from "../ui/usePendingAction";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import {
  fixedListWidthStyle,
  workspaceColumnsLayout,
  workspaceDetailPane,
  workspaceHeaderLayout,
  workspaceListPane,
} from "../ui/workspaceLayout";
import type { NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import { useDraftDiscardConfirmation } from "../routing/useDraftDiscardConfirmation";
import { usePolling } from "../ui/usePolling";
import { describeSnippetFailure } from "./snippetFailure";
import { variablesFor } from "./snippetVariables";
import { SnippetList, type SnippetLibraryState } from "./SnippetList";
import { SnippetEditor } from "./SnippetEditor";
import { SnippetRunSection } from "./SnippetRunSection";
import { SnippetStartupSection } from "./SnippetStartupSection";
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

// SnippetAction は、この画面が engine に頼む操作である。一度に一つだけ走らせ、押したボタン
// だけに回転の印を出す。
type SnippetAction = "save" | "remove" | "preview" | "run" | "cancel" | "setStartup" | "clearStartup";

// Opened は、右の列に開いているものである。none のあいだ、PCでは選ぶよう促し、スマホでは一覧を見せる。
type Opened = { kind: "none" } | { kind: "new" } | { kind: "saved"; id: string };

const nothingOpened: Opened = { kind: "none" };

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
  const compact = useMediaQuery(mobileViewportQuery);
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const [startupAssignments, setStartupAssignments] = useState<Startup[]>([]);
  const [opened, setOpened] = useState<Opened>(nothingOpened);
  const [draft, setDraft] = useState<SnippetDraft>(blank);
  const [targets, setTargets] = useState<string[]>([]);
  const [inputs, setInputs] = useState<Record<string, string>>({});
  const [preview, setPreview] = useState<Preview | null>(null);
  const [job, setJob] = useState<Job | null>(null);
  const [chosenStartupAlias, setChosenStartupAlias] = useState("");
  const [libraryState, setLibraryState] = useState<SnippetLibraryState>("loading");
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
  const selected = opened.kind === "saved" ? opened.id : null;
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
  const draftVariables = variablesFor(draft.command, draft.variables);
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

  // forgetRunState は、開くものを替えたときに、前のスニペットに入れた値・確認・実行の結果を捨てる。
  const forgetRunState = useCallback(() => {
    setInputs({});
    setPreview(null);
    setJob(null);
    clearError();
    touch();
  }, [clearError, touch]);

  const openSnippet = useCallback((snippet: Snippet) => {
    setOpened({ kind: "saved", id: snippet.id });
    setDraft(draftOf(snippet));
    forgetRunState();
  }, [forgetRunState]);

  const openNewSnippet = useCallback(() => {
    setOpened({ kind: "new" });
    setDraft(blank);
    forgetRunState();
  }, [forgetRunState]);

  const closeEditor = useCallback(() => {
    setOpened(nothingOpened);
    setDraft(blank);
    forgetRunState();
  }, [forgetRunState]);

  // changeDraft は、編集欄の入力を変える。変えた時点で、前の保存の成功はもう今の入力を指さない。
  // コマンドが変われば、確認した展開結果ももう今のコマンドではない。
  function changeDraft(next: SnippetDraft) {
    if (next.command !== draft.command) setPreview(null);
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

  // The snippet the address names (the palette opens one this way) is opened once. Closing it, going back
  // to the list on a phone or deleting it must not bring it back while the address stays the same.
  const openedFromLocation = useRef<string | null>(null);
  useEffect(() => {
    if (selectedSnippetId === null || openedFromLocation.current === selectedSnippetId) return;
    const snippet = snippets.find(
      (candidate) => candidate.id === selectedSnippetId,
    );
    if (snippet === undefined) return;
    openedFromLocation.current = selectedSnippetId;
    if (selected !== snippet.id) openSnippet(snippet);
  }, [openSnippet, selected, selectedSnippetId, snippets]);

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
      const value = { ...draft, variables: draftVariables };
      const saved =
        selected === null
          ? await snippetsApi.create(value)
          : await snippetsApi.update(selected, value);
      await reload();
      openSnippet(saved);
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
      variables: draftVariables.map((variable) =>
        variable.name === name ? { ...variable, ...update } : variable,
      ),
    });
    setPreview(null);
  }

  function updateInput(name: string, value: string) {
    setInputs({ ...inputs, [name]: value });
    setPreview(null);
  }

  async function removeCurrent() {
    if (current === null) return;
    await perform("remove", async () => {
      await snippetsApi.remove(current.id);
      closeEditor();
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

  const addButton = (
    <Button kind="primary" className="inline-flex shrink-0 items-center gap-1.5" onClick={() => draftDiscard.confirmBefore(openNewSnippet)}>
      <Icon name="plus" className="size-[15px]" />
      {t("snippets.new")}
    </Button>
  );
  const emptyLibrary = <EmptyState icon="snippets" title={t("snippets.emptyHeading")} action={addButton} />;
  const detailOpen = opened.kind !== "none";
  // 広い画面で何も開いておらず知らせることも無いあいだは、一覧に全幅を渡し、右側が「選択」だけで大きく空くのを避ける。
  const listAlone = !compact && !detailOpen && libraryState === "ready" && snippets.length > 0 && operation.error === "";

  return (
    <>
      <PageBoard fill labelledBy="snippets-heading">
        {/* スマホで編集欄を開いているあいだは、上のバーに［スニペットを追加］を残さず、見出しは読み上げ用に残す。 */}
        {compact && detailOpen ? <h2 id="snippets-heading" className="sr-only">{t("snippets.heading")}</h2> : (
          <PageHeader
            title={t("snippets.heading")}
            headingId="snippets-heading"
            meta={libraryState === "ready" ? t("snippets.count", { count: snippets.length }) : undefined}
            actions={addButton}
            className={workspaceHeaderLayout}
          />
        )}
        <div style={fixedListWidthStyle} className={workspaceColumnsLayout(compact, { listAlone })}>
          <div className={workspaceListPane(compact, detailOpen, { listAlone })}>
            <SnippetList
              libraryState={libraryState}
              snippets={snippets}
              openedId={selected}
              empty={compact ? emptyLibrary : null}
              onRetry={() => void loadLibrary()}
              onOpen={(snippet) => draftDiscard.confirmBefore(() => openSnippet(snippet))}
              columns={listAlone}
            />
          </div>
          <div className={listAlone ? "hidden" : workspaceDetailPane(compact, detailOpen)}>
            {compact ? (
              <Button className="mb-3 w-fit" onClick={() => draftDiscard.confirmBefore(closeEditor)}>
                {t("snippets.allSnippets")}
              </Button>
            ) : null}
            {operation.error === "" ? null : <div className="mb-4"><Notice tone="danger">{operation.error}</Notice></div>}
            {opened.kind === "none" ? (
              // スマホでは何も開いていないあいだ一覧だけを見せるので、空の状態は一覧の側に出す。
              compact || libraryState !== "ready" ? null : snippets.length === 0 ? emptyLibrary : (
                <EmptyState icon="snippets" title={t("snippets.chooseHeading")} action={addButton} />
              )
            ) : (
              <div className={compact ? "flex flex-col gap-3" : "flex flex-col gap-5 [&>section:first-child]:border-t-0 [&>section:first-child]:pt-0"}>
                <SnippetEditor
                  heading={current?.name ?? t("snippets.newHeading")}
                  draft={draft}
                  variables={draftVariables}
                  inputs={inputs}
                  savedJustNow={savedJustNow}
                  deletable={current !== null}
                  saving={pending.running === "save"}
                  deleting={pending.running === "remove"}
                  busy={busy}
                  onDraftChange={changeDraft}
                  onVariableChange={updateVariable}
                  onInputChange={updateInput}
                  onSave={() => void save()}
                  onDelete={() => setConfirmingDelete(true)}
                />
                {current === null ? null : (
                  <>
                    <SnippetRunSection
                      aliases={aliases}
                      targets={targets}
                      preview={preview}
                      job={job}
                      previewing={pending.running === "preview"}
                      running={pending.running === "run"}
                      cancelling={pending.running === "cancel"}
                      busy={busy}
                      onTargetsChange={(next) => {
                        setTargets(next);
                        setPreview(null);
                      }}
                      onPreview={() => void makePreview()}
                      onRun={() => void run()}
                      onCancel={(jobId) => void cancelJob(jobId)}
                    />
                    <SnippetStartupSection
                      aliases={aliases}
                      alias={startupAlias}
                      staleAliases={staleStartupAliases}
                      sendsSecrets={draftVariables.some((variable) => variable.type === "secret")}
                      assigning={pending.running === "setStartup"}
                      busy={busy}
                      onAliasChange={setChosenStartupAlias}
                      onAssign={() => void updateStartup(current.id)}
                      onClear={() => void updateStartup("")}
                    />
                  </>
                )}
              </div>
            )}
          </div>
        </div>
      </PageBoard>
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
    </>
  );
}
