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
  TrashConfirmation,
} from "./KeyForms";
import { AgentSection } from "./AgentSection";
import { KeyTrashSection } from "./KeyTrashSection";
import { KeyGenerationSection } from "./KeyGenerationSection";
import { useTranslate } from "../i18n/context";
import { primaryAction } from "../ui/form";
import { Button, Card, Notice } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
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
  moveInto,
  relocateStem,
  shownItems,
  type MoveTarget,
} from "./organizer";
import { searchKeyItems } from "./keySearch";
import { useKeyInventory } from "./useKeyInventory";
import { useKeyTrash } from "./useKeyTrash";
import { useKeyPassphraseActions } from "./useKeyPassphraseActions";
import { KeyRowPanel, type KeyRowPanelState } from "./KeyRowPanel";
import { ChosenKeysBar, KeyMoveOutcomeNotice } from "./KeyMoveControls";
import { KeyListToolbar } from "./KeyListToolbar";
import { KeyInventoryProblems } from "./KeyInventoryProblems";

export { keySecretsApi, type KeySecretsApi } from "./secretsApi";

type KeysScreenProps = {
  api?: KeysApi;
  onInspector?: (content: InspectorContent) => void;
  groups?: string[];
  secrets?: KeySecretsApi;
  onAssignGeneratedKey?: (key: GeneratedPrivateKeyHandoff) => void;
  onInstallGeneratedKey?: (key: GeneratedPublicKeyHandoff) => void;
};

// Certificate expiry is shown to the minute, so the clock only needs to move
// once a minute for an expiring certificate to be marked while the screen is open.
const certificateClockRefreshMs = 60_000;

export function KeysScreen({
  onInspector,
  api = keysApi,
  groups = [],
  secrets = keySecretsApi,
  onAssignGeneratedKey,
  onInstallGeneratedKey,
}: KeysScreenProps) {
  const t = useTranslate();
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
    detailsFor,
    setDetailsFor,
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
  } = useKeyPassphraseActions({
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

  function closeAllForms() {
    closePassphraseForm();
    closeAgentForm();
    closeStoredPassphraseForm();
  }

  const rowActions: KeyRowActions = {
    onSelect: (item) =>
      setSelectedKey((current) => (current === item.id ? null : item.id)),
    onToggleChosen: (item, picked) => {
      const next = new Set(chosen);
      if (picked) next.add(item.id);
      else next.delete(item.id);
      setChosen(next);
    },
    onBeginDrag: beginDrag,
    onEndDrag: () => setDragging(false),
    onReveal: (item) => {
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
    onToggleDetails: (item) => {
      const closing = detailsFor === item.id;
      setDetailsFor(closing ? "" : item.id);
      setRowPanel(null);
      closeAllForms();
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
    onInspector({
      label: t("keys.inspectorLabel"),
      attention: item.permissionRisk,
      body: <KeyInspector item={item} now={now} />,
    });
  }, [selectedKey, inventory, onInspector, t, now]);

  async function showPublicKey(item: KeyItem) {
    setRowPanel(null);
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
    return <PanelState tone="loading" title={t("keys.reading")} />;
  }
  if (state === "error" || inventory === null || trash === null) {
    return (
      <PanelState
        tone="failed"
        title={t("keys.unreadable")}
        action={<Button onClick={() => void loadKeys()}>{t("shell.bootstrapRetry")}</Button>}
      />
    );
  }

  const searching = keyQuery.trim() !== "";
  const shown = shownItems(inventory.items, listFilter);
  const rows = folderRows(shown, groups);
  const visibleItems = searchKeyItems(itemsInFolder(shown, folder), keyQuery);
  const keyAttention =
    inventory.unreadable.length +
    inventory.unresolvedReferences.length +
    inventory.items.filter((item) => item.permissionRisk).length;
  const inlineKeyPanel = rowPanel === null ? null : {
    itemId: rowPanel.item.id,
    content: <KeyRowPanel panel={rowPanel} api={api} onClose={() => setRowPanel(null)} />,
  };

  return (
    <section className="mx-auto flex w-full max-w-7xl flex-col gap-7 [&_button]:min-h-10 sm:[&_button]:min-h-0">
      <PageHeader
        title={t("keys.heading")}
        description={t("keys.pageDescription")}
        actions={
          <a href="#create-key-heading" className={primaryAction}>
            {t("keys.createHeading")}
          </a>
        }
      />
      <MetricGrid className="sm:grid-cols-3 lg:grid-cols-3">
        <MetricCard label={t("keys.metricFiles")} value={inventory.items.length} icon={<Icon name="keys" className="h-5 w-5" />} />
        <MetricCard label={t("keys.metricPrivate")} value={inventory.items.filter((item) => item.kind === "private_key").length} />
        <MetricCard label={t("keys.metricAttention")} value={keyAttention} attention={keyAttention > 0} />
      </MetricGrid>
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

      <section
        aria-label={t("keys.metricFiles")}
        className="flex flex-col gap-3"
      >
        <KeyListToolbar
          listFilter={listFilter}
          onListFilterChange={setListFilter}
          query={keyQuery}
          onQueryChange={setKeyQuery}
        />
        <Card radius="md">
          <div className="flex flex-col md:flex-row">
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
            <div className="min-w-0 grow md:overflow-x-auto">
              <KeyTable
                items={visibleItems}
                inventory={inventory}
                chosen={chosen}
                selected={selectedKey}
                detailsFor={detailsFor}
                now={now}
                revealRelated={searching}
                inlinePanel={inlineKeyPanel}
                actions={rowActions}
              />
            </div>
          </div>

          <KeyTrashSection
            trash={trash}
            onRestore={restore}
            onPurge={purge}
          />
        </Card>
      </section>

      <StoredPassphrasePanel
        form={storedPassphraseForm}
        storedPhrases={storedPhrases}
        onAssign={(item) => void assignPhrase(item)}
        onUnassign={(item) => void unassignPhrase(item)}
        onStoreAndAssign={(item) => void storeAndAssignPhrase(item)}
      />

      <TrashConfirmation
        item={pendingTrash}
        members={pendingTrash === null ? [] : trashMembers(pendingTrash)}
        onConfirm={(id) => void moveToTrash(id)}
        onCancel={() => setPendingTrash(null)}
      />

      <KeyInventoryProblems inventory={inventory} />

      <AgentSection inventory={inventory} />

      <AgentForm
        form={agentForm}
        storedPhrases={storedPhrases}
        onSubmit={(item) => void registerWithAgent(item)}
        onAssignPhrase={(item) => void assignPhrase(item)}
      />

      <RelocateForm
        form={relocateForm}
        groups={groups}
        onSubmit={(item) => void submitRelocation(item)}
      />

      <RelocateResult result={relocated} onClose={() => setRelocated(null)} />

      <PassphraseForm
        form={passphraseForm}
        onSubmit={(item) => void changePassphrase(item)}
      />

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
