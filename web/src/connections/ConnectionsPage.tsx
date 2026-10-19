import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { toProblem } from "../api/guards";
import {
  configApi,
  hostMetadataEditRequest,
  type CreateConnectionResponse,
  type HostEntry,
  type Overview,
} from "../api/config";
import { ConnectionListPane } from "./ConnectionListPane";
import { ColumnResizeHandle } from "../ui/ColumnResizeHandle";
import { useStoredColumnWidth, type StoredColumnWidth } from "../ui/useStoredColumnWidth";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { MissingConnection, NoConnectionSelected } from "./DetailPlaceholders";
import { useOverlays, useSaveFeedback } from "./pageState";
import { HostDetailPanel } from "./HostDetail";
import {
  CreateConnectionModal,
  type CreateConnectionDraft,
  type CreationPrerequisite,
} from "./CreateConnectionModal";
import { NoticeList } from "./NoticeList";
import { saveProblemMessage } from "../ui/saveProblemMessage";
import { OrphanPanel } from "./OrphanPanel";
import { useTranslate } from "../i18n/context";
import type { InspectorContent } from "../ui/Inspector";
import { Button, Notice } from "../ui/surface";
import { SectionBar } from "../ui/SectionBar";
import { useUnsavedDraftGuard } from "../routing/useUnsavedDraftGuard";
import type {
  BrowserLocation,
  NavigationBlocker,
  NavigateLocationOptions,
} from "../routing/useSectionRoute";
import { parseConnectionLocation } from "../routing/connectionRoute";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import { ConnectionSummary } from "./ConnectionSummary";
import { ManageConnection } from "./ManageConnection";
import { identityKey } from "./connectionBrowser";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { PanelState } from "../ui/PanelState";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { hostDetailApi } from "./HostDetail";
import { useConnectionSelection } from "./useConnectionSelection";
import { useSelectedConnection } from "./useSelectedConnection";
import { useConnectionSave } from "./useConnectionSave";
import { useConnectionManagement } from "./useConnectionManagement";
import { useGroupOperations } from "./useGroupOperations";
import { useConnectionListActions } from "./useConnectionListActions";
import { ConnectionListDialogs } from "./ConnectionListDialogs";
import { SelectionBar } from "./SelectionBar";
import { GroupEditor } from "./GroupEditor";

// Wide enough for the group column plus an alias, host and status on one row; the editor keeps the rest.
const connectionListWidth: StoredColumnWidth = { key: localStorageKeys.connectionListWidth, fallback: 560, minimum: 256, maximum: 720 };

const groupNoticeCodes = new Set([
  "group_not_declared",
  "group_directory_missing",
  "group_empty",
  "group_directory_leftover",
]);

const selectionNoticeCodes = new Set([
  "complex_external_rule",
  "wildcard_shadow",
  "negated_pattern",
  "unnamed_host_block",
  "match_block",
  "dangerous_directive",
  "explained_values_only",
]);


type ConnectionsPageProps = {
  onInspector: (content: InspectorContent) => void;
  creationDraft?: CreateConnectionDraft | null;
  onCreationDraftChange?: (draft: CreateConnectionDraft | null) => void;
  onNavigateForCreation?: (section: CreationPrerequisite) => void;
  location?: BrowserLocation;
  onNavigateLocation?: (url: string, options?: NavigateLocationOptions) => boolean | void;
  onNavigationBlockerChange?: (blocker: NavigationBlocker | null) => void;
  preferredKey?: GeneratedPrivateKeyHandoff | null;
  onPreferredKeyApplied?: () => void;
  onOpenSSHSession: (alias: string) => Promise<void>;
};

type DiscardIntent =
  | { kind: "create" }
  // グループの絞り込みや編集欄の切り替えなど、確かめたあとに続ける操作。
  | { kind: "proceed"; proceed: () => void }
  | { kind: "select"; host: HostEntry }
  | { kind: "navigate"; location: BrowserLocation };

// ConnectionsPage は、接続の一覧と選んだ接続のエディタを並べ、未保存の下書きを捨てる前に確かめる。
// URLと選択の同期、保存済みの内容の読み込み、保存、管理の操作は、それぞれのhookが持つ。
export function ConnectionsPage({
  onInspector,
  creationDraft = null,
  onCreationDraftChange,
  onNavigateForCreation,
  location = { pathname: "/connections", search: "" },
  onNavigateLocation,
  onNavigationBlockerChange,
  preferredKey = null,
  onPreferredKeyApplied,
  onOpenSSHSession,
}: ConnectionsPageProps) {
  const t = useTranslate();
  const compact = useMediaQuery(mobileViewportQuery);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [listWidth, setListWidth] = useStoredColumnWidth(connectionListWidth);
  const [discardIntent, setDiscardIntent] = useState<DiscardIntent | null>(null);
  const draftDiscardRef = useRef<(() => void) | null>(null);
  const feedback = useSaveFeedback();
  const {
    editorDirty, setEditorDirty,
    preview, setPreview,
    problem, setProblem,
    localError, setLocalError,
  } = feedback;
  const editorDirtyRef = useRef(editorDirty);
  editorDirtyRef.current = editorDirty;
  // グループの編集欄に保存していない変更があるか。
  const [groupDraftDirty, setGroupDraftDirty] = useState(false);
  const groupDraftDirtyRef = useRef(groupDraftDirty);
  groupDraftDirtyRef.current = groupDraftDirty;
  const {
    creatingConnection: creating, setCreatingConnection: setCreating,
    launching, setLaunching,
    managing, setManaging,
  } = useOverlays(creationDraft !== null);

  const {
    selection,
    invalidLocation,
    activePanel,
    activeAdvanced,
    isCurrentSelection,
    navigateTarget,
    clearTarget,
    dismissInvalidLocation,
    followCommittedIdentity,
    leaveCommittedIdentityUnknown,
    clearSelection,
    selectHost,
  } = useConnectionSelection({
    location,
    onNavigateLocation,
    // selected はすぐ下で作る。この2つは描画のあと（effect と操作）でしか呼ばれないので参照できる。
    onSelectionMoved: () => {
      selected.reset();
      setEditorDirty(false);
      setPreview(null);
      setProblem(null);
      setLocalError("");
      setManaging(false);
    },
    onIdentityFollowed: () => selected.forget(),
  });
  const selected = useSelectedConnection({
    selection,
    isCurrentSelection,
    onOverviewLoaded: setOverview,
    setProblem,
    setLocalError,
  });
  const { detail, savedState, missingSelection, refreshState, savedRevision, refreshCommittedConnection } = selected;

  const reload = useCallback(async (): Promise<Overview | null> => {
    try {
      const loaded = await configApi.overview();
      setOverview(loaded);
      return loaded;
    } catch (error) {
      setProblem(toProblem(error));
      return null;
    }
  }, [setProblem]);

  const save = useConnectionSave({
    selection,
    selected,
    feedback,
    reload,
    followCommittedIdentity,
  });
  const movesDisabled = editorDirty || refreshState !== "idle";
  const management = useConnectionManagement({
    overview,
    selection,
    detail,
    movesDisabled,
    feedback,
    submit: save.submit,
    reload,
    followCommittedIdentity,
    leaveCommittedIdentityUnknown,
    clearSelection,
    clearTarget,
  });

  const groups = useGroupOperations({ overview, feedback, reload });
  const listActions = useConnectionListActions({
    overview,
    compact,
    selection,
    movesDisabled,
    clearTarget: () => void clearTarget(),
    confirmBeforeLeaving,
    onOpenSSHSession,
    moveConnections: management.moveConnections,
    deleteConnections: management.deleteConnections,
    duplicateConnection: management.duplicateConnection,
  });
  const { checked, checkedHosts, clearChecked, selectionMode, groupEditorName } = listActions;
  const detailOpen = selection !== null || (compact && listActions.editingGroup !== null);
  // 宣言だけあって metadata に設定が無いグループは、名前だけの設定から編集を始める。
  const editedGroup = groupEditorName === null
    ? null
    : (overview?.metadata.groups ?? []).find((group) => group.name === groupEditorName) ?? { name: groupEditorName };

  // 右側に出す設定全体の注意。接続先を開いているときは、その接続先に関わるものだけにする。
  // alias を持つ注意（重複など）は alias で、持たない注意はファイルで見分ける。同じファイルにある
  // ほかの接続先の注意まで出さないため。ほかの接続先の注意は、一覧のその行の印で分かる。
  const paneNotices = (overview?.notices ?? []).filter((notice) => {
    if (groupNoticeCodes.has(notice.code) || selectionNoticeCodes.has(notice.code)) return false;
    if (selection === null) return true;
    if (notice.detail !== undefined && notice.detail !== "") return notice.detail === selection.alias;
    return notice.path === selection.path;
  });

  // 選んだ接続先への操作の帯。広い画面では一覧の上、スマホでは一覧の下に置く。
  function selectionBar(placement: "top" | "bottom") {
    return (
      <SelectionBar
        count={checked.keys.size}
        placement={placement}
        onMove={() => listActions.chooseMoveTarget(checkedHosts)}
        onDelete={() => listActions.confirmDeleting(checkedHosts)}
        onClear={clearChecked}
      />
    );
  }

  function confirmBeforeLeaving(proceed: () => void, { connectionDraft }: { connectionDraft: boolean }) {
    if (groupDraftDirtyRef.current || (connectionDraft && editorDirtyRef.current)) {
      setDiscardIntent({ kind: "proceed", proceed });
      return;
    }
    proceed();
  }

  function beginCreation() {
    if (editorDirty || groupDraftDirty) {
      setDiscardIntent({ kind: "create" });
      return;
    }
    onCreationDraftChange?.(null);
    setCreating(true);
  }

  function confirmDiscard() {
    const intent = discardIntent;
    if (intent === null) return;
    setDiscardIntent(null);
    editorDirtyRef.current = false;
    setEditorDirty(false);
    groupDraftDirtyRef.current = false;
    setGroupDraftDirty(false);
    draftDiscardRef.current?.();
    if (intent.kind === "proceed") {
      intent.proceed();
      return;
    }
    if (intent.kind === "create") {
      onCreationDraftChange?.(null);
      setCreating(true);
      return;
    }
    if (intent.kind === "select") {
      listActions.closeGroupEditor();
      selectHost(intent.host);
      return;
    }
    onNavigateLocation?.(`${intent.location.pathname}${intent.location.search}`);
  }

  function leaveForCreationPrerequisite(section: CreationPrerequisite, draft: CreateConnectionDraft) {
    onCreationDraftChange?.(draft);
    setCreating(false);
    onNavigateForCreation?.(section);
  }

  useEffect(() => {
    void reload();
  }, [reload]);

  // 同じ接続への移動（保存後の URL の置き換えなど）は、下書きを捨てないので通す。
  const leaveEditorBlocker = useCallback<NavigationBlocker>((next) => {
    if (!editorDirtyRef.current && !groupDraftDirtyRef.current) return true;
    const parsed = parseConnectionLocation(next);
    if (parsed.kind === "valid" && selection !== null) {
      const target = parsed.target;
      if (target !== null && target.path === selection.path && target.alias === selection.alias) {
        return true;
      }
    }
    setDiscardIntent({ kind: "navigate", location: next });
    return false;
  }, [selection]);
  useUnsavedDraftGuard({ dirty: editorDirty || groupDraftDirty, blocker: leaveEditorBlocker, onNavigationBlockerChange });

  useEffect(() => {
    onInspector(null);
    return () => onInspector(null);
  }, [onInspector]);

  function onSelect(host: HostEntry) {
    if ((editorDirty && !isCurrentSelection(host.identity)) || groupDraftDirty) {
      setDiscardIntent({ kind: "select", host });
      return;
    }
    listActions.closeGroupEditor();
    selectHost(host);
  }

  async function onConnectionCreated(result: CreateConnectionResponse) {
    setCreating(false);
    onCreationDraftChange?.(null);
    setPreview(result.preview);
    setProblem(null);
    setLocalError("");
    setManaging(false);
    followCommittedIdentity(result.identity, "Basic", "Jump");
    await reload();
  }

  async function connectHost() {
    if (selection === null || launching || editorDirty || refreshState !== "idle") return;
    setLaunching(true);
    setLocalError("");
    await onOpenSSHSession(selection.alias);
    setLaunching(false);
  }

  if (overview === null) {
    return problem === null ? (
      <PanelState tone="loading" title={t("conn.loading")} />
    ) : (
      <PanelState
        tone="failed"
        title={problem.message}
        {...(problem.detail === undefined ? {} : { detail: problem.detail })}
        action={<Button onClick={() => void reload()}>{t("shell.bootstrapRetry")}</Button>}
      />
    );
  }

  return (
    <>
    <div className="flex h-full min-h-0 flex-col bg-canvas">
      <SectionBar
        title={t("conn.heading")}
        meta={t("conn.count", { count: overview.hosts.filter((host) => host.identity.alias !== "").length })}
        actions={(
          <Button kind="primary" className="shrink-0" onClick={beginCreation}>
            {t("conn.new")}
          </Button>
        )}
      />
      <div
        style={{ "--list-width": `${listWidth}px` } as CSSProperties}
        className={`grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] ${compact ? "" : "md:grid-cols-[minmax(16rem,var(--list-width))_minmax(0,1fr)]"}`}
      >
        <ConnectionListPane
          compact={compact}
          resizeHandle={compact ? null : (
            <ColumnResizeHandle
              label={t("conn.resizeList")}
              width={listWidth}
              minimum={connectionListWidth.minimum}
              maximum={connectionListWidth.maximum}
              onWidthChange={setListWidth}
            />
          )}
          overview={overview}
          selection={selection}
          invalidLocation={invalidLocation}
          onDismissInvalidLocation={dismissInvalidLocation}
          onSelect={onSelect}
          onDrop={(payload, target) => {
            void management.dropOnTree(payload, target).then((saved) => {
              if (saved && payload.kind === "connections") clearChecked();
            });
          }}
          movesDisabled={movesDisabled}
          detailOpen={detailOpen}
          treeOptions={{
            scope: listActions.scope,
            onScopeChange: listActions.changeScope,
            checked,
            onCheckedChange: listActions.setChecked,
            multiSelectDisabled: editorDirty,
            selectionMode,
            onSelectionModeChange: listActions.setSelectionMode,
            groupActions: (name) => listActions.groupActionItems(name),
            connectionActions: listActions.connectionActionItems,
            onCreateGroup: () => listActions.openCreateGroup(""),
            topBar: !compact && checked.keys.size >= 2 ? selectionBar("top") : null,
            bottomBar: compact && selectionMode ? selectionBar("bottom") : null,
          }}
        />
        <div
          className={`min-h-0 flex-col gap-4 overflow-y-auto bg-card p-4 lg:p-5 ${compact ? "" : "md:flex"} ${
            detailOpen ? "flex" : "hidden"
          }`}
        >

        <Button
          className={`w-fit ${compact ? "" : "md:hidden"}`}
          onClick={() => confirmBeforeLeaving(() => {
            listActions.closeGroupEditor();
            clearTarget();
          }, { connectionDraft: false })}
        >
          {t("conn.allConnections")}
        </Button>

        <NoticeList collapsible notices={paneNotices} />
        <OrphanPanel
          metadata={overview.metadata}
          hosts={overview.hosts}
          onSave={(base, next) => void save.submit(hostMetadataEditRequest(base, next))}
        />
        {localError === "" ? null : <Notice tone="danger">{localError}</Notice>}
        {editedGroup !== null && selection === null ? (
          <>
            {problem === null ? null : <Notice tone="danger">{saveProblemMessage(problem, t)}</Notice>}
            {/* 保存して読み直した設定で下書きを作り直すため、key に保存済みの設定を含める。 */}
            <GroupEditor
              key={`group:${JSON.stringify(editedGroup)}`}
              group={editedGroup}
              memberCount={listActions.groupListings.find((group) => group.name === editedGroup.name)?.count ?? 0}
              actions={listActions.groupActionItems(editedGroup.name, { inEditor: true })}
              onSave={groups.saveGroup}
              onDirtyChange={setGroupDraftDirty}
            />
          </>
        ) : detail === null && missingSelection && selection !== null ? (
          <MissingConnection onBackToList={() => clearTarget({ replace: true })} />
        ) : detail === null || savedState === null ? (
          <>
            {/* 接続を開いていないと、保存の結果を出す HostDetailPanel が無い。接続先が存在しない
                設定の関連付け直しや破棄を断られたときは、その理由をここに出す。 */}
            {problem === null ? null : <Notice tone="danger">{saveProblemMessage(problem, t)}</Notice>}
            <NoConnectionSelected preferredKey={preferredKey} onBeginCreation={beginCreation} />
          </>
        ) : (
          <>
            <ConnectionSummary
              state={savedState}
              dirty={editorDirty}
              refreshState={refreshState}
              onConnect={() => void connectHost()}
              connecting={launching}
              onToggleManage={() => setManaging((current) => !current)}
              managing={managing}
            />
            {/* 読み直しに失敗したときだけ現れるボタンなので、現れたときにフォーカスを移す。
                タブの下にある保存のバーで保存すると、失敗の文とこのボタンは画面の外にあるので、
                フォーカスでここまでスクロールして失敗に気づけるようにする。成功したときは現れない。 */}
            {refreshState === "failed" ? (
              <Button autoFocus className="self-start" onClick={() => void refreshCommittedConnection()}>
                {t("conn.reloadConnection")}
              </Button>
            ) : null}
            {/* 別の接続を開いたら管理の欄と編集欄を作り直す。前の接続の下書きのまま1回描画されることがなく、
                各タブのeffectは、同じ接続で版が変わったときだけを扱えばよい。
                2つは同じfragmentの兄弟なので、keyの頭に欄の名前を付けて重ならないようにする。
                同じkeyにすると、Reactが描き直すたびに古い管理の欄を消さずに新しい欄を足す。 */}
            {managing ? (
              <ManageConnection
                key={`manage:${identityKey(detail.form.entry.identity)}`}
                detail={detail}
                groups={overview.groups}
                files={overview.files}
                disabled={editorDirty || refreshState !== "idle"}
                onRename={management.rename}
                onMoveToGroup={(group) => void management.moveToGroup(group)}
                onComment={management.comment}
                onDuplicate={management.duplicate}
                onMoveToFile={(path) => void management.moveToFile(path)}
                onDelete={() => void management.remove()}
              />
            ) : null}
            <HostDetailPanel
              key={`detail:${identityKey(detail.form.entry.identity)}`}
              detail={detail}
              savedState={savedState}
              preview={preview}
              problem={problem}
              onFieldEdits={save.onFieldEdits}
              onBlockRaw={save.onBlockRaw}
              onBasicSave={save.onBasicSave}
              onMetadataSave={save.onMetadataSave}
              integrations={hostDetailApi}
              panel={activePanel}
              advanced={activeAdvanced}
              onLocationChange={(panel, advanced) => {
                if (selection !== null) navigateTarget(selection, panel, advanced);
              }}
              preferredKey={preferredKey}
              onPreferredKeyApplied={onPreferredKeyApplied}
              onDirtyChange={setEditorDirty}
              onDiscardReady={(discard) => {
                draftDiscardRef.current = discard;
              }}
              savedRevision={savedRevision}
              vpnProfiles={overview.metadata.vpnProfiles ?? []}
              disabled={refreshState !== "idle"}
            />
          </>
        )}
        </div>
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
    <ConnectionListDialogs
      actions={listActions}
      createGroup={groups.createGroup}
      renameGroup={management.renameGroup}
      deleteGroup={management.deleteGroup}
      renameConnection={management.renameConnection}
    />
    {discardIntent === null ? null : (
      <ConfirmDialog
        id="connection-discard-heading"
        heading={t("conn.discardChanges")}
        body={<p className="text-sm text-ink-muted">{t(groupDraftDirty ? "groupEditor.discardPrompt" : "conn.discardPrompt")}</p>}
        confirmLabel={t("conn.discardChanges")}
        cancelLabel={t("conn.keepEditing")}
        onConfirm={confirmDiscard}
        onCancel={() => setDiscardIntent(null)}
      />
    )}
    </>
  );
}
