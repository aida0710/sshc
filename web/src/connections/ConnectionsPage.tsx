import { useCallback, useEffect, useRef, useState } from "react";
import { toProblem } from "../api/guards";
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
  type UpdateConnectionRequest,
} from "../api/config";
import { type HostSelection } from "./ConnectionTree";
import { ConnectionListPane } from "./ConnectionListPane";
import { MissingConnection, NoConnectionSelected } from "./DetailPlaceholders";
import { useOverlays, useSaveFeedback, useSelectionState } from "./pageState";
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
  consoles: TerminalSessionsState;
  onShowConsole: (id: string) => void;
};

type SaveAttempt =
  | { saved: false; overview: null }
  | { saved: true; overview: Overview | null };

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
  consoles,
  onShowConsole,
}: ConnectionsPageProps) {
  const t = useTranslate();
  const initialRoute = parseConnectionLocation(location);
  const initialTarget = initialRoute.kind === "valid" ? initialRoute.target : null;
  const [overview, setOverview] = useState<Overview | null>(null);
  const entryPath = overview?.entry.path ?? "config";
  const {
    selection, setSelection,
    invalidLocation, setInvalidLocation,
    activePanel, setActivePanel,
    activeAdvanced, setActiveAdvanced,
    missingSelection, setMissingSelection,
  } = useSelectionState(initialTarget, initialRoute.kind === "invalid");
  const selectionRef = useRef<HostSelection | null>(selection);
  const [detail, setDetail] = useState<HostDetail | null>(null);
  const [savedState, setSavedState] = useState<ConnectionSavedState | null>(null);
  const [refreshState, setRefreshState] = useState<"idle" | "refreshing" | "failed">("idle");
  const [savedRevision, setSavedRevision] = useState(0);
  const basicDiscardRef = useRef<(() => void) | null>(null);
  const {
    editorDirty, setEditorDirty,
    preview, setPreview,
    problem, setProblem,
    localError, setLocalError,
  } = useSaveFeedback();
  const {
    creatingConnection: creating, setCreatingConnection: setCreating,
    launching, setLaunching,
    managing, setManaging,
  } = useOverlays(creationDraft !== null);

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
  }, [setProblem]);

  useEffect(() => {
    void reload();
  }, [reload]);

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




  useEffect(() => {
    if (detail === null || overview === null) {
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
  }, [selectedPath, selectedAlias, setDetail, setSavedState, setProblem, setMissingSelection]);

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

  async function onBasicSave(request: UpdateConnectionRequest) {
    let result: Awaited<ReturnType<typeof configApi.updateConnection>>;
    try {
      result = await configApi.updateConnection(request);
    } catch (error) {
      setPreview(null);
      setProblem(toProblem(error));
      throw error;
    }

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

  function onSelect(host: HostEntry) {
    if (host.identity.alias === "") return;
    const nextSelection = { path: host.identity.path, alias: host.identity.alias };
    const currentSelection = selectionRef.current;
    const selectingCurrent = currentSelection?.path === nextSelection.path
      && currentSelection.alias === nextSelection.alias;
    if (!navigateTarget(nextSelection, "Basic", "Jump")) return;
    if (selectingCurrent) return;
    setPreview(null);
    setProblem(null);
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
    <div className="flex h-full min-h-0 flex-col bg-page">
      <header className="flex shrink-0 items-center justify-between gap-4 border-b border-line bg-card px-4 py-3 md:px-5">
        <div className="min-w-0">
          <h1 className="text-lg font-semibold tracking-tight text-ink">{t("conn.heading")}</h1>
          <p className="mt-0.5 text-xs text-ink-muted">
            {t("conn.count", { count: overview.hosts.filter((host) => host.identity.alias !== "").length })}
          </p>
        </div>
        <Button kind="primary" className="min-h-10 shrink-0 md:min-h-0" onClick={beginCreation}>
          {t("conn.new")}
        </Button>
      </header>
      <div className="grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] md:grid-cols-[minmax(22rem,0.85fr)_minmax(0,1.15fr)]">
        <ConnectionListPane
          overview={overview}
          selection={selection}
          invalidLocation={invalidLocation}
          onDismissInvalidLocation={() => {
            if (emitLocation(connectionLocation(null), { replace: true })) {
              setInvalidLocation(false);
            }
          }}
          onSelect={onSelect}
          onDrop={(payload, target) => void onTreeDrop(payload, target)}
          movesDisabled={editorDirty || refreshState !== "idle"}
        />
        <div
          className={`min-h-0 flex-col gap-4 overflow-y-auto p-4 md:flex ${
            selection === null ? "hidden" : "flex"
          }`}
        >

        <Button className="w-fit md:hidden" onClick={() => clearTarget()}>
          {t("conn.allConnections")}
        </Button>

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
          <MissingConnection onBackToList={() => clearTarget({ replace: true })} />
        ) : detail === null || savedState === null ? (
          <NoConnectionSelected preferredKey={preferredKey} onBeginCreation={beginCreation} />
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
