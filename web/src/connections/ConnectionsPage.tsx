import { useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { toProblem } from "../api/guards";
import {
  configApi,
  hostMetadataEditRequest,
  type CreateConnectionResponse,
  type HostDetail,
  type HostEntry,
  type Overview,
} from "../api/config";
import type { ActionMenuItem } from "../ui/ActionMenu";
import { ConnectionListPane } from "./ConnectionListPane";
import { useToast } from "../ui/useToast";
import { usePendingResult } from "../ui/usePendingResult";
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
import { Icon } from "../ui/icons";
import { PageBoard } from "../ui/PageBoard";
import { PageHeader } from "../ui/page";
import { workspaceColumnsLayout, workspaceDetailPane, workspaceHeaderLayout } from "../ui/workspaceLayout";
import { useUnsavedDraftGuard } from "../routing/useUnsavedDraftGuard";
import type {
  BrowserLocation,
  NavigationBlocker,
  NavigateLocationOptions,
} from "../routing/useSectionRoute";
import { parseConnectionLocation } from "../routing/connectionRoute";
import type { GeneratedPrivateKeyHandoff } from "../keys/workflow";
import { ConnectionSummary } from "./ConnectionSummary";
import { ConnectionActionPanel, type ConnectionAction } from "./ConnectionActionPanel";
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
import { folderNoticesOf } from "./groupFolderContents";
import { GroupFolders } from "./GroupFolders";
import { useConnectionListActions } from "./useConnectionListActions";
import { ConnectionListDialogs } from "./ConnectionListDialogs";
import { manageConnectionHandlers } from "./manageConnectionHandlers";
import { droppedMessage } from "./movedMessage";
import { SelectionBar } from "./SelectionBar";
import { GroupEditor } from "./GroupEditor";
import { ConnectionGroupFacets } from "./ConnectionGroupFacets";
import { useConnectionDrag } from "./useConnectionDrag";

// The design's width for the connection list; the groups sit beside the card, outside it. The editor keeps the rest.
const connectionListWidth: StoredColumnWidth = { key: localStorageKeys.connectionListWidth, fallback: 300, minimum: 256, maximum: 720 };

// グループのフォルダについての注意は、右側ではなくグループの一覧の上に出す（folderNoticesOf）。
const groupNoticeCodes = new Set([
  "group_not_declared",
  "group_directory_missing",
  "group_empty",
  "group_directory_leftover",
  "group_file_unreached",
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
  const showToast = useToast();
  const retry = usePendingResult();
  const compact = useMediaQuery(mobileViewportQuery);
  const [overview, setOverview] = useState<Overview | null>(null);
  const [listWidth, setListWidth] = useStoredColumnWidth(connectionListWidth);
  const [discardIntent, setDiscardIntent] = useState<DiscardIntent | null>(null);
  // 見出しの「…」の［削除…］で、開いている接続先を消してよいかを確かめている間。
  const [confirmingDelete, setConfirmingDelete] = useState(false);
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
      setManaging(null);
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
    reorderGroups: groups.reorderGroups,
    setGroupHidden: groups.setGroupHidden,
  });
  const { checked, checkedHosts, clearChecked, selectionMode, groupEditorName } = listActions;
  const drag = useConnectionDrag({
    movesDisabled,
    groupNames: overview?.groups.map((group) => group.name) ?? [],
    onDrop: (payload, target) => {
      void management.dropOnTree(payload, target).then((saved) => {
        if (!saved) return;
        if (payload.kind === "connections") clearChecked();
        // 落とした行は絞り込みの外へ出て見えなくなることがあるので、動いたことを知らせる。
        showToast(droppedMessage(t, { payload, target }), { tone: "success" });
      });
    },
  });
  const detailOpen = selection !== null || (compact && listActions.editingGroup !== null);
  // 宣言だけあって metadata に設定が無いグループは、名前だけの設定から編集を始める。
  const editedGroup = groupEditorName === null
    ? null
    : (overview?.metadata.groups ?? []).find((group) => group.name === groupEditorName) ?? { name: groupEditorName };
  const editedGroupView = overview?.groups.find((group) => group.name === groupEditorName);

  // 右側に出す設定全体の注意。接続先を開いているときは、その接続先に関わるものだけにする。
  // alias を持つ注意（重複など）は alias で、持たない注意はファイルで見分ける。同じファイルにある
  // ほかの接続先の注意まで出さないため。ほかの接続先の注意は、一覧のその行の印で分かる。
  const folderNotices = folderNoticesOf(overview?.notices ?? []);
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
    setManaging(null);
    followCommittedIdentity(result.identity, "Basic", "Settings");
    await reload();
  }

  // connectionHeaderActions は、開いている接続先の見出しの「…」に並べる操作である。入力が要る操作は見出しの下に
  // 欄を開き、複製はすぐ行い、削除だけ確かめる。
  function connectionHeaderActions(opened: HostDetail): ActionMenuItem[] {
    const blocked = editorDirty || refreshState !== "idle";
    const handlers = manageConnectionHandlers({ saves: management, alias: opened.form.entry.identity.alias, t, showToast });
    const open = (action: ConnectionAction) => ({ onSelect: () => setManaging(action), disabled: blocked });
    return [
      { label: t("tree.rename"), ...open("rename") },
      { label: t("tree.moveToGroup"), ...open("group") },
      { label: t("tree.duplicate"), onSelect: () => void handlers.onDuplicate(), disabled: blocked },
      { label: t("host.comment"), ...open("comment") },
      { label: t("conn.move"), ...open("file") },
      { label: t("conn.editAsText"), onSelect: () => { if (selection !== null) navigateTarget(selection, "Advanced", "Raw"); } },
      { label: t("tree.deleteEllipsis"), tone: "danger", onSelect: () => setConfirmingDelete(true), disabled: blocked },
    ];
  }

  async function connectHost() {
    if (selection === null || launching || editorDirty || refreshState !== "idle") return;
    setLaunching(true);
    setLocalError("");
    await onOpenSSHSession(selection.alias);
    setLaunching(false);
  }

  const newConnectionButton = (enabled: boolean) => (
    <Button kind="primary" className="inline-flex shrink-0 items-center gap-1.5" disabled={!enabled} onClick={beginCreation}>
      <Icon name="plus" className="size-[15px]" />
      {t("conn.new")}
    </Button>
  );

  if (overview === null) {
    // 読み込むあいだも、見出しと一覧・右側の枠は出しておく。読み込みが終わったときに画面が組み変わって見えないように。
    // 板の横のグループの列は、グループを読み終えてから出る。
    return (
      <PageBoard fill labelledBy="connections-heading">
        <PageHeader title={t("conn.heading")} headingId="connections-heading" actions={newConnectionButton(false)} className={workspaceHeaderLayout} />
        <div style={{ "--list-width": `${listWidth}px` } as CSSProperties} className={workspaceColumnsLayout(compact)}>
          <div className={`flex min-h-0 flex-col ${compact ? "" : "border-r border-line"}`}>
            {problem === null ? (
              <PanelState tone="loading" title={t("conn.loading")} />
            ) : (
              <PanelState
                tone="failed"
                title={problem.message}
                {...(problem.detail === undefined ? {} : { detail: problem.detail })}
                action={<Button busy={retry.pending} onClick={() => retry.watch(reload())}>{t("shell.bootstrapRetry")}</Button>}
              />
            )}
          </div>
          {compact ? null : <div className="hidden md:block" />}
        </div>
      </PageBoard>
    );
  }

  return (
    <>
    <PageBoard
      fill
      labelledBy="connections-heading"
      aside={(
        <ConnectionGroupFacets
          overview={overview}
          scope={listActions.scope}
          onScopeChange={listActions.changeScope}
          drag={drag}
          groupActions={(name) => listActions.groupActionItems(name)}
          onCreateGroup={() => listActions.openCreateGroup("")}
          folderNotices={folderNotices}
          onDeclareGroup={(name) => void groups.createGroup(name)}
        />
      )}
    >
      {/* On a phone the editor takes the screen; the row with New connection would stay above it and
          cover the unsaved badge, so only the heading's name stays, for screen readers. */}
      {compact && detailOpen ? <h2 id="connections-heading" className="sr-only">{t("conn.heading")}</h2> : (
        <PageHeader
          title={t("conn.heading")}
          headingId="connections-heading"
          meta={t("conn.count", { count: overview.hosts.filter((host) => host.identity.alias !== "").length })}
          actions={newConnectionButton(true)}
          className={workspaceHeaderLayout}
        />
      )}
      <div style={{ "--list-width": `${listWidth}px` } as CSSProperties} className={workspaceColumnsLayout(compact)}>
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
          drag={drag}
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
            folderNotices,
            onDeclareGroup: (name: string) => void groups.createGroup(name),
            topBar: !compact && checked.keys.size >= 2 ? selectionBar("top") : null,
            bottomBar: compact && selectionMode ? selectionBar("bottom") : null,
          }}
        />
        <div className={`gap-4 ${workspaceDetailPane(compact, detailOpen)}`}>

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
              folders={editedGroupView === undefined ? null : <GroupFolders group={editedGroupView} hosts={overview.hosts} />}
              onSave={groups.saveGroup}
              onDirtyChange={setGroupDraftDirty}
            />
          </>
        ) : detail === null && missingSelection && selection !== null ? (
          <MissingConnection onBackToList={() => clearTarget({ replace: true })} />
        ) : (detail === null || savedState === null) && selection !== null ? (
          // 一覧では選ばれているので、読み終えるまで「接続を選択」を出さずに読み込み中と出す。
          <PanelState tone="loading" title={t("conn.loadingConnection", { alias: selection.alias })} />
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
              actions={connectionHeaderActions(detail)}
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
            {managing === null ? null : (
              <ConnectionActionPanel
                key={`manage:${managing}:${identityKey(detail.form.entry.identity)}`}
                action={managing}
                detail={detail}
                groups={overview.groups}
                files={overview.files}
                disabled={editorDirty || refreshState !== "idle"}
                {...manageConnectionHandlers({ saves: management, alias: detail.form.entry.identity.alias, t, showToast })}
                onClose={() => setManaging(null)}
              />
            )}
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
    </PageBoard>
    {creating ? (
      <CreateConnectionModal
        groups={overview.groups}
        initialDraft={creationDraft ?? undefined}
        onOpenPrerequisite={leaveForCreationPrerequisite}
        onCreateGroup={groups.createGroup}
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
      failure={problem === null ? "" : saveProblemMessage(problem, t)}
    />
    {confirmingDelete && detail !== null ? (
      <ConfirmDialog
        id="delete-connection-heading"
        heading={t("conn.deleteHeading", { alias: detail.form.entry.identity.alias })}
        body={<p className="text-sm text-ink-muted">{t("conn.deleteBody")}</p>}
        confirmLabel={t("conn.confirmDelete")}
        cancelLabel={t("conn.deleteCancel")}
        onCancel={() => setConfirmingDelete(false)}
        onConfirm={() => {
          // 先に閉じてから消す。断られた理由は背面の画面に出るので、閉じておかないと隠れる。
          setConfirmingDelete(false);
          void manageConnectionHandlers({ saves: management, alias: detail.form.entry.identity.alias, t, showToast }).onDelete();
        }}
      />
    ) : null}
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
