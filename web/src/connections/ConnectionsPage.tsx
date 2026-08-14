import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, type Problem } from "../api/client";
import {
  configApi,
  type EditRequest,
  type CreateConnectionResponse,
  type FieldEdit,
  type HostDetail,
  type HostEntry,
  type HostMetadata,
  type Metadata,
  type Overview,
  type SavePreview,
  type UpdateConnectionRequest,
} from "../api/config";
import { ConnectionTree, type HostSelection } from "./ConnectionTree";
import type { DragPayload } from "./dragdrop";
import { HostDetailPanel } from "./HostDetail";
import {
  CreateConnectionModal,
  type CreateConnectionDraft,
  type CreationPrerequisite,
} from "./CreateConnectionModal";
import { NoticeList } from "./SavePreview";
import { OrphanPanel } from "./OrphanPanel";
import { useTranslate } from "../i18n/context";
import type { InspectorContent } from "../ui/Inspector";
import { HostInspector, hostNeedsAttention } from "./HostInspector";
import { Button, Notice } from "../ui/surface";
import { duplicateHostBlock, removeHostBlock } from "./blocks";
import { integrationsApi } from "../api/integrations";
import type { TerminalSessionsState } from "../terminal/sessions";
import { Icon } from "../ui/icons";
import type {
  BrowserLocation,
  NavigationBlocker,
  NavigateLocationOptions,
} from "../routing/useSectionRoute";
import {
  connectionLocation,
  parseConnectionLocation,
  type AdvancedArea,
  type ConnectionPanel,
} from "../routing/connectionRoute";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import { keysApi } from "../keys/api";
import { ConnectionSummary } from "./ConnectionSummary";
import { loadConnectionSavedState, type ConnectionSavedState } from "./connectionSavedState";
import { ManageConnection } from "./ManageConnection";

// Groups 画面が報告し、この画面は報告しないもの。
//
// `group_empty` がここに無いのは、もはやどこでも notice として報告
// されないからだ。宣言済みグループが何も持たない状態は、作られた直後
// のすべてのグループがある状態そのものであり、Groups 画面はその行自体
// に "Members: none" と表示している。
const groupNoticeCodes = new Set([
  "group_not_declared",
  "group_directory_missing",
  "group_empty",
  "group_directory_leftover",
]);

// These describe one pattern block, host block, or effective-value view. The
// tree and selected detail retain them; showing them above an empty detail pane
// makes a warning look global while offering no object to inspect.
const selectionNoticeCodes = new Set([
  "complex_external_rule",
  "wildcard_shadow",
  "negated_pattern",
  "unnamed_host_block",
  "match_block",
  "dangerous_directive",
  "explained_values_only",
]);

function toProblem(error: unknown): Problem {
  if (error instanceof ApiError && error.problem !== null) return error.problem;
  if (error instanceof ApiError) return { code: error.code, message: "request rejected" };
  return { code: "request_failed", message: "request rejected" };
}

type ConnectionsPageProps = {
  // alias を持たない Host パターンには connection identity がないため、
  // 管理ツリーから Config の該当行へ渡す。
  onOpenFile?: (path: string, line: number) => void;
  // 右側ペインの中身を、シェルへ差し出す。connection が開いていない間は
  // null——何か開くまでは、調べるものが何も無いからだ。
  onInspector: (content: InspectorContent) => void;
  creationDraft?: CreateConnectionDraft | null;
  onCreationDraftChange?: (draft: CreateConnectionDraft | null) => void;
  onNavigateForCreation?: (section: CreationPrerequisite) => void;
  location?: BrowserLocation;
  onNavigateLocation?: (url: string, options?: NavigateLocationOptions) => boolean | void;
  onNavigationBlockerChange?: (blocker: NavigationBlocker | null) => void;
  preferredKey?: GeneratedPrivateKeyHandoff | null;
  onPreferredKeyApplied?: () => void;
  // 開いているセッションはシェルが持つ。**描くのは Terminal 画面である**
  // ——ここは開くだけで、開けたら向こうへ渡す。
  consoles: TerminalSessionsState;
  onShowConsole: (id: string) => void;
};

type SaveAttempt =
  | { saved: false; overview: null }
  | { saved: true; overview: Overview | null };

export function ConnectionsPage({
  onOpenFile = () => undefined,
  onInspector,
  creationDraft = null,
  onCreationDraftChange,
  onNavigateForCreation,
  location = { pathname: "/connections", search: "" },
  onNavigateLocation,
  onNavigationBlockerChange,
  preferredKey = null,
  onPreferredKeyApplied,
  consoles,
  onShowConsole,
}: ConnectionsPageProps) {
  const t = useTranslate();
  const initialRoute = parseConnectionLocation(location);
  const initialTarget = initialRoute.kind === "valid" ? initialRoute.target : null;
  const [overview, setOverview] = useState<Overview | null>(null);
  // どのグループにも属さない connection が向かう先。このページが決め
  // つけるのではなく、サーバーがエントリファイルを報告する。"config" は
  // 最初の overview が届くまでの、あくまで暫定のフォールバックである。
  const entryPath = overview?.entry.path ?? "config";
  const [selection, setSelection] = useState<HostSelection | null>(
    initialTarget === null ? null : { path: initialTarget.path, alias: initialTarget.alias },
  );
  const selectionRef = useRef<HostSelection | null>(selection);
  const [invalidLocation, setInvalidLocation] = useState(initialRoute.kind === "invalid");
  const [activePanel, setActivePanel] = useState<ConnectionPanel>(initialTarget?.panel ?? "Basic");
  const [activeAdvanced, setActiveAdvanced] = useState<AdvancedArea>(initialTarget?.advanced ?? "Jump");
  const [detail, setDetail] = useState<HostDetail | null>(null);
  const [savedState, setSavedState] = useState<ConnectionSavedState | null>(null);
  const [editorDirty, setEditorDirty] = useState(false);
  const [refreshState, setRefreshState] = useState<"idle" | "refreshing" | "failed">("idle");
  const [savedRevision, setSavedRevision] = useState(0);
  const basicDiscardRef = useRef<(() => void) | null>(null);
  const [preview, setPreview] = useState<SavePreview | null>(null);
  const [problem, setProblem] = useState<Problem | null>(null);
  const [localError, setLocalError] = useState("");
  const [creating, setCreating] = useState(creationDraft !== null);
  const [launching, setLaunching] = useState(false);
  const [managing, setManaging] = useState(false);
  const [missingSelection, setMissingSelection] = useState(false);

  useEffect(() => {
    selectionRef.current = selection;
  }, [selection]);

  function emitLocation(url: string, options?: NavigateLocationOptions): boolean {
    const result = options === undefined
      ? onNavigateLocation?.(url)
      : onNavigateLocation?.(url, options);
    return result !== false;
  }

  function navigateTarget(
    identity: HostSelection,
    panel: ConnectionPanel,
    advanced: AdvancedArea,
    options?: NavigateLocationOptions,
  ): boolean {
    if (!emitLocation(connectionLocation({
      path: identity.path,
      alias: identity.alias,
      panel,
      advanced,
    }), options)) return false;
    setActivePanel(panel);
    setActiveAdvanced(advanced);
    setInvalidLocation(false);
    return true;
  }

  function clearTarget(options?: NavigateLocationOptions): boolean {
    return emitLocation(connectionLocation(null), options);
  }

  // 書き込み済みの identity は、後続の GET より先に画面と URL の正本にする。
  // detail は selection effect が新しい identity から読み直す。GET が一時的に
  // 失敗しても、URL がもう存在しない旧 alias/path を指し続けることはない。
  function followCommittedIdentity(
    identity: HostSelection,
    panel: ConnectionPanel = activePanel,
    advanced: AdvancedArea = activeAdvanced,
  ) {
    selectionRef.current = identity;
    setSelection(identity);
    setDetail(null);
    setSavedState(null);
    setMissingSelection(false);
    navigateTarget(identity, panel, advanced, { replace: true });
  }

  function leaveCommittedIdentityUnknown() {
    selectionRef.current = null;
    setSelection(null);
    setDetail(null);
    setSavedState(null);
    setMissingSelection(false);
    clearTarget({ replace: true });
  }

  function beginCreation() {
    if (editorDirty && !window.confirm(t("conn.discardPrompt"))) return;
    if (editorDirty) basicDiscardRef.current?.();
    onCreationDraftChange?.(null);
    setCreating(true);
  }

  function leaveForCreationPrerequisite(section: CreationPrerequisite, draft: CreateConnectionDraft) {
    onCreationDraftChange?.(draft);
    setCreating(false);
    onNavigateForCreation?.(section);
  }

  const reload = useCallback(async (): Promise<Overview | null> => {
    try {
      const loaded = await configApi.overview();
      setOverview(loaded);
      return loaded;
    } catch (error) {
      setProblem(toProblem(error));
      return null;
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  // URL は、選択・表示パネルの共有可能な正本である。popstate を受けた親が
  // location を更新すると、戻る/進むでも同じ connection を復元する。
  // URL に秘密や絶対パスは入らず、parser が安全な相対パスだけを通す。
  useEffect(() => {
    const parsed = parseConnectionLocation(location);
    if (parsed.kind === "redirect") {
      emitLocation(parsed.location, { replace: true });
      setInvalidLocation(false);
      if (selectionRef.current === null) return;
      selectionRef.current = null;
      setSelection(null);
      setDetail(null);
      setSavedState(null);
      setEditorDirty(false);
      setRefreshState("idle");
      setActivePanel("Basic");
      setActiveAdvanced("Jump");
      setMissingSelection(false);
      setPreview(null);
      setProblem(null);
      setManaging(false);
      return;
    }
    if (parsed.kind === "invalid") {
      setInvalidLocation(true);
      if (selectionRef.current === null) return;
      selectionRef.current = null;
      setSelection(null);
      setDetail(null);
      setSavedState(null);
      setEditorDirty(false);
      setRefreshState("idle");
      setActivePanel("Basic");
      setActiveAdvanced("Jump");
      setMissingSelection(false);
      setPreview(null);
      setProblem(null);
      setManaging(false);
      return;
    }

    setInvalidLocation(false);
    const target = parsed.target;
    const current = selectionRef.current;
    if (target === null) {
      if (current === null) return;
      selectionRef.current = null;
      setSelection(null);
      setDetail(null);
      setSavedState(null);
      setEditorDirty(false);
      setRefreshState("idle");
      setActivePanel("Basic");
      setActiveAdvanced("Jump");
      setMissingSelection(false);
      setPreview(null);
      setProblem(null);
      setManaging(false);
      return;
    }

    setActivePanel(target.panel);
    setActiveAdvanced(target.advanced);
    if (current?.path === target.path && current.alias === target.alias) return;
    const nextSelection = { path: target.path, alias: target.alias };
    selectionRef.current = nextSelection;
    setSelection(nextSelection);
    setDetail(null);
    setSavedState(null);
    setEditorDirty(false);
    setRefreshState("idle");
    setMissingSelection(false);
    setPreview(null);
    setProblem(null);
    setManaging(false);
    // emitLocation はこの effect と同じ render の location callback を使う。
    // 親が callback を作り直すこと自体は URL state の変化ではない。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname, location.search]);

  useEffect(() => {
    if (!editorDirty) {
      onNavigationBlockerChange?.(null);
      return;
    }
    const blocker: NavigationBlocker = (next) => {
      const parsed = parseConnectionLocation(next);
      if (parsed.kind === "valid" && selection !== null) {
        const target = parsed.target;
        if (target !== null && target.path === selection.path && target.alias === selection.alias) {
          return true;
        }
      }
      return window.confirm(t("conn.discardPrompt"));
    };
    onNavigationBlockerChange?.(blocker);
    return () => onNavigationBlockerChange?.(null);
  }, [editorDirty, onNavigationBlockerChange, selection, t]);

  useEffect(() => {
    if (!editorDirty) return;
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnBeforeUnload);
    return () => window.removeEventListener("beforeunload", warnBeforeUnload);
  }, [editorDirty]);

  // Home で開かれたセッションは、この画面へ来た時点で選択される。



  useEffect(() => {
    if (detail === null || overview === null) {
      // 開いている接続が無ければ、このペインに調べるものは無い。開いている
      // コンソールの一覧は一番左のナビゲーションにあるので、ここが空でも
      // ローカルシェルへ行く道は残っている。
      onInspector(null);
      return;
    }
    onInspector({
      label: t("inspector.hostLabel"),
      attention: hostNeedsAttention(detail),
      body: <HostInspector detail={detail} onMetadata={onMetadata} />,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detail, overview, onInspector]);

  // 依存配列にしているのは selection オブジェクトではなく二つの値その
  // ものである。保存すると、たった今書き込んだホストを再選択するからだ。
  // 中身が等しくても identity が新しいオブジェクトはこの effect を再実
  // 行させてしまい、submit が既に取得中の detail を再度取得し、その答え
  // が届いた時点で、保存が直後に作ったプレビューを破棄してしまっていた。
  // 書き込んだ内容の diff は、リクエスト一回分の時間だけ画面に出て、その後消えていた。
  const selectedPath = selection === null ? "" : selection.path;
  const selectedAlias = selection === null ? "" : selection.alias;
  useEffect(() => {
    if (selectedAlias === "") return;
    let active = true;
    void configApi
      .host(selectedPath, selectedAlias)
      .then(async (loaded) => ({
        detail: loaded,
        saved: await loadConnectionSavedState(loaded, keysApi, integrationsApi),
      }))
      .then(({ detail: loaded, saved }) => {
        if (active) {
          setDetail(loaded);
          setSavedState(saved);
          setRefreshState("idle");
          setProblem(null);
          setMissingSelection(false);
        }
      })
      .catch((error: unknown) => {
        if (active) {
          setDetail(null);
          setSavedState(null);
          setProblem(toProblem(error));
          setMissingSelection(true);
        }
      });
    return () => {
      active = false;
    };
  }, [selectedPath, selectedAlias]);

  // 編集で開いているホストが削除された場合、reselect は false になる
  // ——消したばかりのブロックをサーバーへすぐに問い合わせずに済ませるためだ。
  async function submit(request: EditRequest, reselect = true): Promise<SaveAttempt> {
    let result: Awaited<ReturnType<typeof configApi.save>>;
    try {
      result = await configApi.save(request);
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      return { saved: false, overview: null };
    }

    setPreview(result.preview);
    setProblem(null);
    const selectedBeforeSave = selection;
    const renamedSelection =
      request.kind === "rename" && selectedBeforeSave !== null
        ? {
            path: selectedBeforeSave.path,
            alias: request.newAlias ?? selectedBeforeSave.alias,
          }
        : null;
    if (renamedSelection !== null) followCommittedIdentity(renamedSelection);

    const nextOverview = await reload();
    if (reselect && selectedBeforeSave !== null && renamedSelection === null && request.kind !== "metadata") {
      try {
        const loaded = await configApi.host(selectedBeforeSave.path, selectedBeforeSave.alias);
        const saved = await loadConnectionSavedState(loaded, keysApi, integrationsApi);
        const currentSelection = selectionRef.current;
        if (currentSelection?.path === selectedBeforeSave.path && currentSelection.alias === selectedBeforeSave.alias) {
          setDetail(loaded);
          setSavedState(saved);
          setSavedRevision((current) => current + 1);
        }
      } catch (error) {
        const currentSelection = selectionRef.current;
        if (currentSelection?.path === selectedBeforeSave.path && currentSelection.alias === selectedBeforeSave.alias) {
          setProblem(toProblem(error));
          setMissingSelection(true);
        }
      }
    }
    return { saved: true, overview: nextOverview };
  }

  // Basic は ssh_config の共通フィールドと vault の関連付けを一つの
  // トランザクションにする専用 use case である。失敗を再送出するのは、
  // フォーム自身が入力済みの秘密だけを破棄し、非秘密の下書きを保持する
  // 必要があるためである。ページは同時に、詳細な Problem と conflict diff
  // を既存の Save preview に残す。
  async function onBasicSave(request: UpdateConnectionRequest) {
    let result: Awaited<ReturnType<typeof configApi.updateConnection>>;
    try {
      result = await configApi.updateConnection(request);
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      throw error;
    }

    // The commit is the success boundary. Reset the draft immediately so
    // secret and non-secret inputs cannot be submitted twice while the saved
    // snapshot is being confirmed by follow-up reads.
    setPreview(result.preview);
    setProblem(null);
    setLocalError("");
    basicDiscardRef.current?.();
    setRefreshState("refreshing");
  }

  function savedResourcesConfirmed(saved: ConnectionSavedState): boolean {
    return saved.keys.status !== "failed" &&
      saved.vault.status !== "failed" &&
      saved.credentials.status !== "failed" &&
      saved.eligibility.status !== "failed";
  }

  async function refreshCommittedConnection() {
    if (selection === null) return;
    const identity = selection;
    setRefreshState("refreshing");
    try {
      const [nextOverview, nextDetail] = await Promise.all([
        configApi.overview(),
        configApi.host(identity.path, identity.alias),
      ]);
      const nextSaved = await loadConnectionSavedState(nextDetail, keysApi, integrationsApi);
      if (!savedResourcesConfirmed(nextSaved)) throw new Error("saved_state_refresh_failed");
      const currentSelection = selectionRef.current;
      if (currentSelection?.path !== identity.path || currentSelection.alias !== identity.alias) return;
      setOverview(nextOverview);
      setDetail(nextDetail);
      setSavedState(nextSaved);
      setSavedRevision((current) => current + 1);
      setRefreshState("idle");
      setLocalError("");
      setProblem(null);
    } catch {
      const currentSelection = selectionRef.current;
      if (currentSelection?.path !== identity.path || currentSelection.alias !== identity.alias) return;
      setRefreshState("failed");
      setLocalError(t("conn.basicConnectionRefreshFailed"));
    }
  }

  // この防護は残す——具体的な alias を持たないエントリには identity が
  // 無く、host エンドポイントはそれに invalid_request を返す。ツリーは
  // そのようなブロックをここへは決してルーティングせず、ファイルビューへ送る。
  // alias の無い selection は将来の呼び出し元が作っても、サーバーへ届いてはならない。
  function onSelect(host: HostEntry) {
    if (host.identity.alias === "") return;
    const nextSelection = { path: host.identity.path, alias: host.identity.alias };
    const currentSelection = selectionRef.current;
    const selectingCurrent = currentSelection?.path === nextSelection.path
      && currentSelection.alias === nextSelection.alias;
    if (!navigateTarget(nextSelection, "Basic", "Jump")) return;
    // URL と selection が同じなら、そのクリックは何も破棄しない。ここで
    // detail を空にしても selection effect は identity の変化を検出できず、
    // 同じ connection を再び開けなくなる。下書きも同じ identity のものなので保つ。
    if (selectingCurrent) return;
    // 別の connection を選ぶと、直前の保存の diff は破棄される——それは
    // もう開いていないブロックのバイトを記述しているからだ。保存はここで
    // はなく submit を通じて再選択を行い、その diff は画面に残しておく。
    setPreview(null);
    setProblem(null);
    // The selection highlight changes immediately, so the detail must not keep
    // showing the previous host while the new request is in flight. Otherwise
    // a fast edit can be submitted against a connection the tree no longer
    // appears to have selected.
    setDetail(null);
    setSavedState(null);
    setMissingSelection(false);
    setManaging(false);
    selectionRef.current = nextSelection;
    setSelection(nextSelection);
    setActivePanel("Basic");
    setActiveAdvanced("Jump");
  }

  function onFieldEdits(fields: FieldEdit[]) {
    if (detail === null || selection === null) return;
    void submit({
      kind: "host_fields",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      fields,
    });
  }

  function onBlockRaw(raw: string) {
    if (detail === null || selection === null) return;
    void submit({ kind: "block_raw", path: selection.path, alias: selection.alias, base: detail.file.contents, raw });
  }

  function onRename(newName: string) {
    if (detail === null || selection === null) return;
    void submit({
      kind: "rename",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      newAlias: newName,
    });
  }

  // connection をグループへ移動するのはファイル移動であるため、リクエ
  // ストはグループ名を渡し、サーバーがそこから移動先パスを導く。パスも
  // 併せて送ってしまうと両者が食い違い得るため、サーバーはそれを即座に拒否する。
  //
  // 空のグループは「どのグループの外にも」を意味し、これはディレクト
  // リへの移動ではなくエントリファイルへ戻す移動である。この形には
  // エントリファイルのバイトが要るため、移動先はファイル間移動と同じ
  // ように、自分自身の事前条件で守られる。
  async function onMoveToGroup(group: string) {
    if (detail === null) return;
    const path = detail.form.entry.file.path ?? "";
    const alias = detail.form.entry.identity.alias;
    if (group !== "") {
      const attempt = await submit(
        { kind: "move", path, base: detail.file.contents, alias, destinationGroup: group },
        false,
      );
      if (!attempt.saved) return;
      const moved = attempt.overview?.hosts.find(
        (host) => host.identity.alias === alias && host.group === group,
      );
      if (moved !== undefined) {
        followCommittedIdentity(moved.identity);
      } else {
        // 保存は完了したが、新しい path を決める overview を読めなかった。
        // 消えた旧 path を共有可能な URL として残すより一覧へ戻す方が正確である。
        leaveCommittedIdentityUnknown();
      }
      return;
    }
    try {
      const destination = await configApi.file(entryPath);
      const attempt = await submit({
        kind: "move",
        path,
        base: detail.file.contents,
        alias,
        destinationPath: entryPath,
        destinationBase: destination.contents,
      }, false);
      if (!attempt.saved) return;
      followCommittedIdentity({ path: entryPath, alias });
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  // ドロップは、このページが既に行っている移動のどれか一つを、ドラッ
  // グされたものに応じて選ぶだけである。サーバーに新しいものは何も届
  // かない——connection は移動であり、親が変わるグループは新しいパスへのリネームである。
  //
  // ドラッグされた connection は選択中のものとは限らないため、そのファ
  // イルのバイトは開いている detail から取るのではなくここで読む。選択
  // されていないものなら現在の detail は保ち、選択中のものなら保存後の
  // identity だけを追う。後者をしないと、画面上は移動済みなのに URL は
  // 存在しなくなった古いファイルを指し続ける。
  async function onTreeDrop(payload: DragPayload, target: string) {
    if (editorDirty || refreshState !== "idle") return;
    try {
      if (payload.kind === "group") {
        const base = payload.name.slice(payload.name.lastIndexOf("/") + 1);
        const destinationName = target === "" ? base : `${target}/${base}`;
        const selectedHost = overview?.hosts.find(
          (host) =>
            host.identity.path === selection?.path && host.identity.alias === selection.alias,
        );
        const selectedDestinationGroup =
          selectedHost?.group === payload.name
            ? destinationName
            : selectedHost?.group?.startsWith(`${payload.name}/`)
              ? `${destinationName}${selectedHost.group.slice(payload.name.length)}`
              : null;
        const result = await configApi.renameGroup(payload.name, destinationName);
        setPreview(result.preview);
        setProblem(null);
        const nextOverview = await reload();
        if (selection !== null && selectedDestinationGroup !== null && nextOverview !== null) {
          const moved = nextOverview.hosts.find(
            (host) =>
              host.identity.alias === selection.alias && host.group === selectedDestinationGroup,
          );
          if (moved !== undefined) {
            followCommittedIdentity(moved.identity);
          } else {
            leaveCommittedIdentityUnknown();
          }
        } else if (selection !== null && selectedDestinationGroup !== null) {
          leaveCommittedIdentityUnknown();
        }
        return;
      }
      const file = await configApi.file(payload.path);
      const followsSelection =
        selection?.path === payload.path && selection.alias === payload.alias;
      if (target !== "") {
        const attempt = await submit({
          kind: "move",
          path: payload.path,
          base: file.contents,
          alias: payload.alias,
          destinationGroup: target,
        }, false);
        if (!attempt.saved) return;
        if (followsSelection && attempt.overview !== null) {
          const moved = attempt.overview.hosts.find(
            (host) => host.identity.alias === payload.alias && host.group === target,
          );
          if (moved !== undefined) {
            followCommittedIdentity(moved.identity);
          } else {
            leaveCommittedIdentityUnknown();
          }
        } else if (followsSelection) {
          leaveCommittedIdentityUnknown();
        }
        return;
      }
      const destination = await configApi.file(entryPath);
      const attempt = await submit({
        kind: "move",
        path: payload.path,
        base: file.contents,
        alias: payload.alias,
        destinationPath: entryPath,
        destinationBase: destination.contents,
      }, false);
      if (!attempt.saved) return;
      if (followsSelection) {
        followCommittedIdentity({ path: entryPath, alias: payload.alias });
      }
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
    }
  }

  // このコメントは configuration ファイルに書き込まれるため、そのファイ
  // ルへの他のあらゆる編集と同じ base と事前条件の経路を通る。
  function onComment(comment: string) {
    if (detail === null || selection === null) return;
    void submit({
      kind: "comment",
      path: selection.path,
      alias: selection.alias,
      base: detail.file.contents,
      comment,
    });
  }

  function onMetadata(host: HostMetadata) {
    if (overview === null) return;
    const others = (overview.metadata.hosts ?? []).filter(
      (entry) => entry.identity.path !== host.identity.path || entry.identity.alias !== host.identity.alias,
    );
    const metadata: Metadata = { ...overview.metadata, hosts: [...others, host] };
    void submit({ kind: "metadata", metadata });
  }

  async function onConnectionCreated(result: CreateConnectionResponse) {
    setCreating(false);
    onCreationDraftChange?.(null);
    setPreview(result.preview);
    setProblem(null);
    setLocalError("");
    setManaging(false);
    setActivePanel("Basic");
    setActiveAdvanced("Jump");
    followCommittedIdentity(result.identity, "Basic", "Jump");
    await reload();
  }

  // 接続はこのアプリケーションの中で開く。外部の端末アプリケーションは
  // 起こさない。action token も要らない——vault ゲートだけが条件である。
  async function connectHost() {
    if (selection === null || launching || editorDirty || refreshState !== "idle") return;
    setLaunching(true);
    setLocalError("");
    const opened = await consoles.open({ kind: "ssh", alias: selection.alias });
    if (opened !== null) onShowConsole(opened.id);
    setLaunching(false);
  }

  function duplicateHost() {
    if (detail === null || selection === null) return;
    try {
      void submit({
        kind: "file_raw",
        path: selection.path,
        base: detail.file.contents,
        raw: duplicateHostBlock(
          detail.file.contents,
          detail.form.raw,
          selection.alias,
          `${selection.alias}-copy`,
          detail.form.entry.line,
          detail.form.commentLines,
        ),
      });
      setLocalError("");
    } catch {
      setLocalError(t("conn.blockMoved"));
    }
  }

  // この移動は読み込み済みの base を両方運び、サーバーが各ファイルをそ
  // れぞれの事前条件で守れるようにする。再選択は submit ではなくここで
  // 行う——移動がコミットされた時点で、ホストは新しいパスに存在するからだ。
  async function moveHost(target: string) {
    if (detail === null || selection === null || target === "") return;
    try {
      const destination = await configApi.file(target);
      const source = selection;
      const attempt = await submit({
        kind: "move",
        path: source.path,
        base: detail.file.contents,
        alias: source.alias,
        destinationPath: target,
        destinationBase: destination.contents,
      }, false);
      if (!attempt.saved) return;
      followCommittedIdentity({ path: target, alias: source.alias });
      setLocalError("");
    } catch (error) {
      setProblem(toProblem(error));
    }
  }

  async function deleteHost() {
    if (detail === null || selection === null) return;
    let raw: string;
    try {
      raw = removeHostBlock(
        detail.file.contents,
        detail.form.entry.line,
        detail.form.raw,
        detail.form.commentLines,
      );
    } catch {
      setLocalError(t("conn.blockMoved"));
      return;
    }
    const path = selection.path;
    const base = detail.file.contents;
    const attempt = await submit({ kind: "file_raw", path, base, raw }, false);
    if (!attempt.saved) return;
    setSelection(null);
    selectionRef.current = null;
    setDetail(null);
    setSavedState(null);
    setLocalError("");
    clearTarget({ replace: true });
  }

  if (overview === null) {
    return <p role="status" className="text-sm text-ink-muted">{t("conn.loading")}</p>;
  }

  return (
    <>
    {/* ウィンドウの端まで届く二つのペイン。detail の minmax(0,…) は、
        inspector が開いたときにも内容幅を保たず縮められるようにする。 */}
    <div className="grid h-full grid-cols-[19rem_minmax(0,1fr)] grid-rows-[minmax(0,1fr)]">
      <div className="flex min-h-0 flex-col border-r border-line bg-tree">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-line px-3 py-3">
          <div className="min-w-0">
            <h2 className="font-semibold">{t("conn.heading")}</h2>
            <p className="text-xs text-ink-muted">
              {t("conn.count", { count: overview.hosts.filter((host) => host.identity.alias !== "").length })}
            </p>
          </div>
          <Button
            kind="primary"
            className="shrink-0 px-2.5 py-1.5 text-xs"
            onClick={beginCreation}
          >
            {t("conn.new")}
          </Button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-3">
          {invalidLocation ? (
            <section className="flex flex-col gap-2 rounded-lg border border-line bg-card p-3 text-sm" role="status">
              <p className="font-medium">{t("browser.invalidUrl")}</p>
              <Button
                className="self-start"
                onClick={() => {
                  if (emitLocation(connectionLocation(null), { replace: true })) {
                    setInvalidLocation(false);
                  }
                }}
              >
                {t("browser.backToServers")}
              </Button>
            </section>
          ) : (
            <ConnectionTree
              overview={overview}
              selected={selection}
              onSelect={onSelect}
              onOpenPatternRule={onOpenFile}
              onDrop={(payload, target) => void onTreeDrop(payload, target)}
              movesDisabled={editorDirty || refreshState !== "idle"}
            />
          )}
        </div>
      </div>
      <div className="flex min-h-0 flex-col gap-4 overflow-y-auto p-6">
        {/*
          グループ単位の notice は Groups 画面のものであり、README にもそう
          書いてある——それらは宣言とディスクが互いについて何を語っているか
          を記述するもので、この画面が対処できることではない。ここに届いて
          いたのは、overview が運ぶすべての notice をこのリストへ渡していた
          からにすぎない。
        */}
        <NoticeList
          notices={overview.notices.filter(
            (notice) => !groupNoticeCodes.has(notice.code) && !selectionNoticeCodes.has(notice.code),
          )}
        />
        <OrphanPanel
          metadata={overview.metadata}
          hosts={overview.hosts}
          onSave={(metadata) => void submit({ kind: "metadata", metadata })}
        />
        {localError === "" ? null : <Notice tone="danger">{localError}</Notice>}
        {detail === null && missingSelection && selection !== null ? (
          <section className="m-auto flex max-w-sm flex-col items-center text-center" role="status">
            <h2 className="text-lg font-semibold text-ink">{t("conn.missingHeading")}</h2>
            <p className="mt-1 text-sm leading-6 text-ink-muted">{t("conn.missingHint")}</p>
            <Button
              kind="primary"
              className="mt-4"
              onClick={() => clearTarget({ replace: true })}
            >
              {t("conn.backToList")}
            </Button>
          </section>
        ) : detail === null || savedState === null ? (
          <section className="m-auto flex max-w-sm flex-col items-center text-center" role="status">
            <span
              aria-hidden="true"
              className="mb-4 flex size-14 items-center justify-center rounded-2xl border border-line bg-card text-ink-muted shadow-sm"
            >
              <Icon name="connections" className="size-7" />
            </span>
            <h2 className="text-lg font-semibold text-ink">
              {t(preferredKey === null ? "conn.emptyHeading" : "conn.assignKeyHeading")}
            </h2>
            <p className="mt-1 text-sm leading-6 text-ink-muted">
              {preferredKey === null
                ? t("conn.emptyHint")
                : t("conn.assignKeyHint", { path: preferredKey.privateRelativePath })}
            </p>
            <Button kind="primary" className="mt-4" onClick={beginCreation}>{t("conn.createAnother")}</Button>
          </section>
        ) : (
          <>
            <ConnectionSummary
              state={savedState}
              dirty={editorDirty}
              refreshing={refreshState !== "idle"}
              onConnect={() => void connectHost()}
              connecting={launching}
              onToggleManage={() => setManaging((current) => !current)}
              managing={managing}
            />
            {refreshState === "failed" ? (
              <Button className="self-start" onClick={() => void refreshCommittedConnection()}>
                {t("conn.reloadConnection")}
              </Button>
            ) : null}
            {managing ? (
              <ManageConnection
                detail={detail}
                groups={overview.groups}
                files={overview.files}
                disabled={editorDirty || refreshState !== "idle"}
                onRename={onRename}
                onMoveToGroup={(group) => void onMoveToGroup(group)}
                onComment={onComment}
                onDuplicate={duplicateHost}
                onMoveToFile={(path) => void moveHost(path)}
                onDelete={() => void deleteHost()}
              />
            ) : null}
            <HostDetailPanel
              detail={detail}
              savedState={savedState}
              preview={preview}
              problem={problem}
              onFieldEdits={onFieldEdits}
              onBlockRaw={onBlockRaw}
              onBasicSave={onBasicSave}
              integrations={integrationsApi}
              panel={activePanel}
              advanced={activeAdvanced}
              onLocationChange={(panel, advanced) => {
                if (selection !== null) navigateTarget(selection, panel, advanced);
              }}
              preferredKey={preferredKey}
              onPreferredKeyApplied={onPreferredKeyApplied}
              onDirtyChange={setEditorDirty}
              onBasicDiscardReady={(discard) => {
                basicDiscardRef.current = discard;
              }}
              onRequestRefresh={refreshCommittedConnection}
              savedRevision={savedRevision}
              disabled={refreshState !== "idle"}
            />
          </>
        )}
      </div>
    </div>
    {creating ? (
      <CreateConnectionModal
        groups={overview.groups}
        initialDraft={creationDraft ?? undefined}
        onOpenPrerequisite={leaveForCreationPrerequisite}
        onClose={() => {
          setCreating(false);
          onCreationDraftChange?.(null);
        }}
        onCreated={(result) => void onConnectionCreated(result)}
      />
    ) : null}
    </>
  );
}
