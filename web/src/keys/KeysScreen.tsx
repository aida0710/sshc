import { useEffect, useState, type DragEvent } from "react";
import {
  useAgentForm,
  useOrganiser,
  usePassphraseForm,
  useRelocateForm,
  useStoredPassphraseForm,
} from "./forms";
import { useKeyPassphrases } from "./useKeyPassphrases";
import { useKeyOperation } from "./useKeyOperation";
import { keySecretsApi, type KeySecretsApi } from "./secretsApi";
import { KeyTable, type KeyRowActions } from "./KeyTable";
import {
  AgentForm,
  PassphraseForm,
  RelocateForm,
  RelocateResult,
  StoredPassphrasePanel,
} from "./KeyForms";
import { AgentSection } from "./AgentSection";
import { useEngineOS } from "../about/useEngineOS";
import { aboutApi, type AboutApi } from "../api/about";
import { KeyTrashConfirmation } from "./KeyTrashConfirmation";
import { KeyTrashTable } from "./KeyTrashTable";
import { KeyGenerationSection } from "./KeyGenerationSection";
import { useTranslate } from "../i18n/context";
import { primaryAction } from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { Icon } from "../ui/icons";
import { PageHeader, PageSection, PageStateFrame, SummaryStat, SummaryStats } from "../ui/page";
import { PanelState } from "../ui/PanelState";
import { keysApi, type KeyItem, type KeysApi, type RelocateKeyResponse } from "./api";
import type {
  GeneratedPrivateKeyHandoff,
  GeneratedPublicKeyHandoff,
} from "./workflow";
import { FolderPane } from "./FolderPane";
import type { InspectorContent } from "../ui/Inspector";
import { useCurrentTime } from "../ui/useCurrentTime";
import { KeyInspector } from "./KeyInspector";
import {
  folderRows,
  groupOfKeyPath,
  itemsInFolder,
  keyItemGroups,
  moveInto,
  relocateStem,
  shownItems,
  type MoveTarget,
} from "./organizer";
import { searchKeyItems } from "./keySearch";
import { useKeyInventory } from "./useKeyInventory";
import { useKeyTrash } from "./useKeyTrash";
import { createKeyPassphraseActions } from "./keyPassphraseActions";
import { KeyRowPanel, type KeyRowPanelState } from "./KeyRowPanel";
import { ChosenKeysBar, KeyMoveOutcomeNotice } from "./KeyMoveControls";
import { KeyListToolbar } from "./KeyListToolbar";
import { KeyInventoryProblems } from "./KeyInventoryProblems";


type KeysScreenProps = {
  api?: KeysApi;
  onInspector?: (content: InspectorContent) => void;
  // onShowInspector opens the details pane, when a key is chosen from the table.
  onShowInspector?: () => void;
  groups?: string[];
  secrets?: KeySecretsApi;
  onAssignGeneratedKey?: (key: GeneratedPrivateKeyHandoff) => void;
  onInstallGeneratedKey?: (key: GeneratedPublicKeyHandoff) => void;
  // about tells which system the engine runs on, for the ssh-agent advice.
  about?: AboutApi;
};

// Certificate expiry is shown to the minute, so the clock only needs to move
// once a minute for an expiring certificate to be marked while the screen is open.
const certificateClockRefreshMs = 60_000;

export function KeysScreen({
  onInspector,
  onShowInspector,
  api = keysApi,
  groups = [],
  secrets = keySecretsApi,
  onAssignGeneratedKey,
  onInstallGeneratedKey,
  about = aboutApi,
}: KeysScreenProps) {
  const t = useTranslate();
  const engineOS = useEngineOS(true, about);
  const [rowPanel, setRowPanel] = useState<KeyRowPanelState | null>(null);
  const storedPhrases = useKeyPassphrases(secrets);
  const { load: loadPhrases } = storedPhrases;
  const passphraseForm = usePassphraseForm();
  const { setChangingPassphrase, close: closePassphraseForm } = passphraseForm;
  const agentForm = useAgentForm(storedPhrases);
  const { setRegistering, close: closeAgentForm } = agentForm;
  const storedPassphraseForm = useStoredPassphraseForm(storedPhrases);
  const { setManagingPassphrase, close: closeStoredPassphraseForm } = storedPassphraseForm;
  const [relocated, setRelocated] = useState<RelocateKeyResponse | null>(null);
  const relocateForm = useRelocateForm();
  const {
    setRelocating,
    newName,
    setNewName,
    newGroup,
    setNewGroup,
    close: closeRelocateForm,
  } = relocateForm;
  const { failure, fail, clearFailure, run: runKeyOperation } = useKeyOperation();
  const {
    folder,
    setFolder,
    chosen,
    setChosen,
    dragging,
    setDragging,
    moveOutcome,
    setMoveOutcome,
    moveTarget,
    setMoveTarget,
    listFilter,
    setListFilter,
    keyQuery,
    setKeyQuery,
    selectedKey,
    setSelectedKey,
  } = useOrganiser();

  const { state, inventory, trash, variants, load: loadKeys, refresh } = useKeyInventory({ api, fail });
  const {
    changePassphrase,
    registerWithAgent,
    removeFromAgent,
    assignPhrase,
    storeAndAssignPhrase,
    unassignPhrase,
  } = createKeyPassphraseActions({
    api,
    refresh,
    runKeyOperation,
    storedPhrases,
    passphraseForm,
    agentForm,
    storedPassphraseForm,
  });
  const { pendingTrash, setPendingTrash, trashMembers, moveToTrash, restore, purge } = useKeyTrash({
    api,
    inventory,
    refresh,
    runKeyOperation,
    clearFailure,
  });

  const now = useCurrentTime(certificateClockRefreshMs);

  // Only one panel opens under the rows at a time: a form, or a key's text.
  function closeAllForms() {
    closePassphraseForm();
    closeAgentForm();
    closeStoredPassphraseForm();
    closeRelocateForm();
    setRowPanel(null);
  }

  const rowActions: KeyRowActions = {
    onSelect: (item) => {
      setSelectedKey(item.id);
      onShowInspector?.();
    },
    onToggleChosen: (item, picked) => {
      const next = new Set(chosen);
      if (picked) next.add(item.id);
      else next.delete(item.id);
      setChosen(next);
    },
    onBeginDrag: beginDrag,
    onEndDrag: () => setDragging(false),
    onReveal: (item) => {
      closeAllForms();
      setRowPanel({ kind: "reveal", item });
    },
    onShowPublicKey: (item) => void showPublicKey(item),
    onManageStoredPassphrase: (item) => {
      closeAllForms();
      setManagingPassphrase(item);
      void loadPhrases();
    },
    onAddToAgent: (item) => {
      closeAllForms();
      setRegistering(item);
      if (item.encrypted) void loadPhrases();
    },
    onRemoveFromAgent: (item) => {
      closeAllForms();
      void removeFromAgent(item.id);
    },
    onChangePassphrase: (item) => {
      closeAllForms();
      setChangingPassphrase(item);
    },
    onRelocate: (item) => {
      closeAllForms();
      setRelocated(null);
      setNewName(relocateStem(item));
      setNewGroup(groupOfKeyPath(item.relativePath));
      setRelocating(item);
    },
    onMoveToTrash: (item) => {
      closeAllForms();
      clearFailure();
      setPendingTrash(item);
    },
  };

  useEffect(() => {
    if (onInspector === undefined) return;
    const item = inventory?.items.find(
      (candidate) => candidate.id === selectedKey,
    );
    if (item === undefined) {
      onInspector(null);
      return;
    }
    const related = keyItemGroups(inventory?.items ?? []).find((group) => group.primary.id === item.id)?.related ?? [];
    onInspector({
      label: t("keys.inspectorLabel"),
      attention: item.permissionRisk,
      body: <KeyInspector item={item} related={related} now={now} />,
    });
  }, [selectedKey, inventory, onInspector, t, now]);

  async function showPublicKey(item: KeyItem) {
    closeAllForms();
    await runKeyOperation(async () => {
      const response = await api.publicKey(item.id);
      setRowPanel({
        kind: "publicKey",
        item,
        relativePath: response.relativePath,
        text: response.publicKey.trimEnd(),
      });
    }, "keys.publicKeyFailed");
  }

  // 移せなかった鍵は、失敗の通知ではなく moveOutcome が1件ずつ示す。
  async function moveChosen(target: MoveTarget) {
    if (inventory === null) return;
    const items = inventory.items.filter((item) => chosen.has(item.id));
    if (items.length === 0) return;
    await runKeyOperation(async () => {
      const outcome = await moveInto(
        (keyId, change) => api.relocate(keyId, change),
        items,
        target,
      );
      setMoveOutcome(outcome);
      setChosen(new Set());
      await refresh();
    }, "keys.relocateFailed");
  }

  function beginDrag(event: DragEvent<HTMLSpanElement>, item: KeyItem) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", item.relativePath);
    setDragging(true);
    if (!chosen.has(item.id)) setChosen(new Set([item.id]));
  }

  async function submitRelocation(item: KeyItem) {
    setRelocated(null);
    await runKeyOperation(async () => {
      const currentGroup = groupOfKeyPath(item.relativePath);
      const response = await api.relocate(item.id, {
        ...(newName === relocateStem(item) ? {} : { newName }),
        ...(newGroup === currentGroup ? {} : { group: newGroup }),
      });
      setRelocated(response);
      if (response.blockers.length > 0) return;
      closeRelocateForm();
      await refresh();
    }, "keys.relocateFailed");
  }

  if (state === "loading") {
    return (
      <PageStateFrame title={t("keys.heading")}>
        <PanelState tone="loading" title={t("keys.reading")} />
      </PageStateFrame>
    );
  }
  if (state === "error" || inventory === null || trash === null) {
    return (
      <PageStateFrame title={t("keys.heading")}>
        <PanelState
          tone="failed"
          title={t("keys.unreadable")}
          action={<Button onClick={() => void loadKeys()}>{t("shell.bootstrapRetry")}</Button>}
        />
      </PageStateFrame>
    );
  }

  const searching = keyQuery.trim() !== "";
  const shown = shownItems(inventory.items, listFilter);
  const rows = folderRows(shown, groups, trash.entries.length);
  const visibleItems = searchKeyItems(itemsInFolder(shown, folder), keyQuery);
  const keyAttention =
    inventory.unreadable.length +
    inventory.unresolvedReferences.length +
    inventory.items.filter((item) => item.permissionRisk).length;
  const panelItem =
    rowPanel?.item ??
    passphraseForm.changingPassphrase ??
    agentForm.registering ??
    storedPassphraseForm.managingPassphrase ??
    relocateForm.relocating;
  // A public key or certificate read from its private key's menu opens under that key's row, since it has
  // no row of its own unless a search lists it.
  const primaryOfRelated = new Map(
    keyItemGroups(visibleItems).flatMap((group) => group.related.map((item) => [item.id, group.primary.id] as const)),
  );
  const panelRow = panelItem === null ? "" : searching ? panelItem.id : primaryOfRelated.get(panelItem.id) ?? panelItem.id;
  // Each form draws nothing unless it is the one open, so the panel holds just that one.
  const inlineKeyPanel = panelItem === null ? null : {
    itemId: panelRow,
    content: (
      <>
        {rowPanel === null ? null : <KeyRowPanel panel={rowPanel} api={api} onClose={() => setRowPanel(null)} />}
        <StoredPassphrasePanel
          form={storedPassphraseForm}
          storedPhrases={storedPhrases}
          onAssign={(item) => void assignPhrase(item)}
          onUnassign={(item) => void unassignPhrase(item)}
          onStoreAndAssign={storeAndAssignPhrase}
        />
        <AgentForm
          form={agentForm}
          storedPhrases={storedPhrases}
          onSubmit={registerWithAgent}
          onAssignPhrase={(item) => void assignPhrase(item)}
        />
        <RelocateForm form={relocateForm} groups={groups} onSubmit={submitRelocation} />
        {/* A refused move keeps its form open, with the reasons under it. */}
        {relocateForm.relocating === null ? null : <RelocateResult result={relocated} onClose={() => setRelocated(null)} />}
        <PassphraseForm form={passphraseForm} onSubmit={changePassphrase} />
      </>
    ),
  };

  return (
    <section className="flex flex-col gap-[22px] [&_button]:min-h-10 sm:[&_button]:min-h-0">
      <PageHeader
        title={t("keys.heading")}
        actions={
          <a href="#create-key-heading" className={`${primaryAction} inline-flex min-h-11 items-center gap-1.5 md:min-h-0`}>
            <Icon name="plus" className="size-[15px]" />
            {t("keys.createHeading")}
          </a>
        }
      />
      <SummaryStats>
        <SummaryStat label={t("keys.metricFiles")} value={inventory.items.length} />
        <SummaryStat label={t("keys.metricPrivate")} value={inventory.items.filter((item) => item.kind === "private_key").length} />
        <SummaryStat label={t("keys.metricAttention")} value={keyAttention} attention={keyAttention > 0} />
      </SummaryStats>
      {failure !== "" && <Notice tone="danger">{failure}</Notice>}

      {chosen.size === 0 ? null : (
        <ChosenKeysBar
          count={chosen.size}
          groups={groups}
          target={moveTarget}
          onTargetChange={setMoveTarget}
          onMove={(target) => void moveChosen(target)}
          onClear={() => setChosen(new Set())}
        />
      )}
      {moveOutcome === null ? null : <KeyMoveOutcomeNotice outcome={moveOutcome} />}
      {relocateForm.relocating === null ? <RelocateResult result={relocated} onClose={() => setRelocated(null)} /> : null}

      <PageSection
        heading={t("keys.listHeading")}
        meta="~/.ssh"
        actions={folder.kind === "trash" ? undefined : (
          <KeyListToolbar
            listFilter={listFilter}
            onListFilterChange={setListFilter}
            query={keyQuery}
            onQueryChange={setKeyQuery}
          />
        )}
      >
        {/* keylist decides whether the groups fit beside the table; keytable whether the table has room for
            its columns (KeyTable, KeyTrashTable). */}
        <div className="@container/keylist">
          <div className="flex flex-col gap-3 @3xl/keylist:flex-row @3xl/keylist:items-start @3xl/keylist:gap-5">
            <FolderPane
              rows={rows}
              selected={folder}
              dragging={dragging}
              onSelect={(next) => {
                setFolder(next);
                setMoveOutcome(null);
              }}
              onDropInto={(target) => void moveChosen(target)}
            />
            <div className="@container/keytable min-w-0 grow">
              {folder.kind === "trash" ? (
                <KeyTrashTable trash={trash} onRestore={restore} onPurge={purge} />
              ) : (
                <KeyTable
                  items={visibleItems}
                  inventory={inventory}
                  chosen={chosen}
                  selected={selectedKey}
                  now={now}
                  revealRelated={searching}
                  inlinePanel={inlineKeyPanel}
                  actions={rowActions}
                />
              )}
            </div>
          </div>
        </div>
      </PageSection>

      <KeyTrashConfirmation
        item={pendingTrash}
        members={pendingTrash === null ? [] : trashMembers(pendingTrash)}
        error={failure}
        onConfirm={moveToTrash}
        onCancel={() => setPendingTrash(null)}
      />

      <KeyInventoryProblems inventory={inventory} />

      <AgentSection inventory={inventory} engineOS={engineOS} />

      <KeyGenerationSection
        api={api}
        variants={variants}
        groups={groups}
        onGenerated={refresh}
        runOperation={runKeyOperation}
        onAssignGeneratedKey={onAssignGeneratedKey}
        onInstallGeneratedKey={onInstallGeneratedKey}
      />
    </section>
  );
}
