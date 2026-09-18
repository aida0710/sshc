import { vaultApi, type VaultApi } from "../api/vault";
import { credentialsApi, type CredentialsApi } from "../api/credentials";
import { useCallback, useEffect, useState, type DragEvent } from "react";
import {
  useAgentForm,
  useOrganiser,
  usePassphraseForm,
  useRelocateForm,
  useStoredPassphraseForm,
  useStoredPhrases,
} from "./forms";
import { RevealDialog } from "./RevealDialog";
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
import { CopyButton } from "../ui/CopyButton";
import { useTranslate } from "../i18n/context";
import { control, primaryAction, sectionHeading } from "../ui/form";
import { Button, Card, Notice } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
import { PanelState } from "../ui/PanelState";
import {
  keysApi,
  type KeyInventoryResponse,
  type KeyItem,
  type KeysApi,
  type KeyVariant,
  type RelocateKeyResponse,
  type TrashListResponse,
} from "./api";
import type {
  GeneratedPrivateKeyHandoff,
  GeneratedPublicKeyHandoff,
} from "./workflow";
import { FolderPane } from "./FolderPane";
import type { InspectorContent } from "../ui/Inspector";
import { KeyInspector } from "./KeyInspector";
import {
  folderRows,
  groupOfKeyPath,
  includeKeyPairContext,
  itemsInFolder,
  moveInto,
  shownItems,
  type ListFilter,
  type MoveTarget,
} from "./organizer";

// The key list stores passphrases as credentials and assigns them to keys,
// which needs the vault open.
export type KeySecretsApi = Pick<VaultApi, "passwordVault"> &
  Pick<CredentialsApi, "credentials" | "storeCredential" | "assignCredential" | "unassignCredential">;
export const keySecretsApi: KeySecretsApi = { ...vaultApi, ...credentialsApi };

type KeysScreenProps = {
  api?: KeysApi;
  onInspector?: (content: InspectorContent) => void;
  groups?: string[];
  secrets?: KeySecretsApi;
  onAssignGeneratedKey?: (key: GeneratedPrivateKeyHandoff) => void;
  onInstallGeneratedKey?: (key: GeneratedPublicKeyHandoff) => void;
};

type ScreenState = "loading" | "ready" | "error";

function relocateStem(item: KeyItem): string {
  const base = item.relativePath.split("/").pop() ?? item.relativePath;
  if (item.kind === "private_key") return base;
  for (const suffix of ["-cert.pub", ".pub"]) {
    if (base.endsWith(suffix) && base.length > suffix.length)
      return base.slice(0, -suffix.length);
  }
  return base;
}

export function KeysScreen({
  onInspector,
  api = keysApi,
  groups = [],
  secrets = keySecretsApi,
  onAssignGeneratedKey,
  onInstallGeneratedKey,
}: KeysScreenProps) {
  const t = useTranslate();
  const [state, setState] = useState<ScreenState>("loading");
  const [inventory, setInventory] = useState<KeyInventoryResponse | null>(null);
  const [trash, setTrash] = useState<TrashListResponse | null>(null);
  const [variants, setVariants] = useState<KeyVariant[]>([]);
  const [revealing, setRevealing] = useState<KeyItem | null>(null);
  const storedPhrases = useStoredPhrases();
  const {
    phrases,
    setPhrases,
    setDedicatedPhrasePaths,
    chosenPhrase,
    setChosenPhrase,
  } = storedPhrases;
  const passphraseForm = usePassphraseForm();
  const {
    setChangingPassphrase,
    currentPassphrase,
    setCurrentPassphrase,
    newPassphrase,
    setNewPassphrase,
    removePassphrase,
    close: closePassphraseForm,
  } = passphraseForm;
  const agentForm = useAgentForm(storedPhrases);
  const {
    setRegistering,
    agentPassphrase,
    setAgentPassphrase,
    agentLifetime,
    close: closeAgentForm,
  } = agentForm;
  const storedPassphraseForm = useStoredPassphraseForm(storedPhrases);
  const {
    setManagingPassphrase,
    storedPhraseName,
    storedPhraseSecret,
    setStoredPhraseSecret,
    close: closeStoredPassphraseForm,
  } = storedPassphraseForm;
  const [publicKeyView, setPublicKeyView] = useState<{
    id: string;
    relativePath: string;
    text: string;
  } | null>(null);
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
  const [pendingTrash, setPendingTrash] = useState<KeyItem | null>(null);
  const [failure, setFailure] = useState("");
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

  const refresh = useCallback(async () => {
    try {
      const [nextInventory, nextTrash, nextAlgorithms] = await Promise.all([
        api.inventory(),
        api.listTrash(),
        api.algorithms(),
      ]);
      setInventory(nextInventory);
      setTrash(nextTrash);
      setVariants(nextAlgorithms.variants);
      setState("ready");
    } catch {
      setState("error");
    }
  }, [api]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const now = Date.now();

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
      setPublicKeyView(null);
      setRevealing(item);
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
      setRevealing(null);
      setPublicKeyView(null);
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

  async function submitPassphrase(item: KeyItem) {
    setFailure("");
    try {
      await api.changePassphrase(item.id, {
        currentPassphrase,
        newPassphrase: removePassphrase ? "" : newPassphrase,
        unencrypted: removePassphrase,
      });
      closePassphraseForm();
      await refresh();
    } catch {
      setCurrentPassphrase("");
      setNewPassphrase("");
      setFailure(t("keys.passphraseFailed"));
    }
  }

  async function removeFromAgent(keyId: string) {
    try {
      await api.deregisterFromAgent(keyId);
      await refresh();
    } catch {
      setFailure(t("keys.agentRemoveFailed"));
    }
  }

  async function loadPhrases() {
    try {
      const status = await secrets.passwordVault();
      setDedicatedPhrasePaths(status.dedicatedKeyPassphrases);
      if (!status.unlocked) {
        setPhrases([]);
        return;
      }
      const listed = await secrets.credentials();
      setPhrases(
        listed.credentials.filter(
          (credential) => credential.kind === "key_passphrase",
        ),
      );
    } catch {
      setPhrases([]);
      setDedicatedPhrasePaths([]);
    }
  }

  async function assignPhrase(item: KeyItem) {
    try {
      const listed = await secrets.assignCredential(
        "key_passphrase",
        item.relativePath,
        chosenPhrase,
      );
      setPhrases(
        listed.credentials.filter(
          (credential) => credential.kind === "key_passphrase",
        ),
      );
      setDedicatedPhrasePaths((current) =>
        current.filter((path) => path !== item.relativePath),
      );
      setChosenPhrase("");
    } catch {
      setFailure(t("keys.assignPassphraseFailed"));
    }
  }

  async function storeAndAssignPhrase(item: KeyItem) {
    if (storedPhraseName === "" || storedPhraseSecret === "") return;
    setFailure("");
    if (phrases.some((credential) => credential.name === storedPhraseName)) {
      setStoredPhraseSecret("");
      setFailure(t("keys.storedPassphraseExists"));
      return;
    }
    try {
      await secrets.storeCredential(
        "key_passphrase",
        storedPhraseName,
        storedPhraseSecret,
      );
      const listed = await secrets.assignCredential(
        "key_passphrase",
        item.relativePath,
        storedPhraseName,
      );
      setPhrases(
        listed.credentials.filter(
          (credential) => credential.kind === "key_passphrase",
        ),
      );
      setDedicatedPhrasePaths((current) =>
        current.filter((path) => path !== item.relativePath),
      );
      closeStoredPassphraseForm();
    } catch {
      setStoredPhraseSecret("");
      setFailure(t("keys.storePassphraseFailed"));
    }
  }

  async function unassignPhrase(item: KeyItem) {
    setFailure("");
    try {
      const listed = await secrets.unassignCredential(
        "key_passphrase",
        item.relativePath,
      );
      setPhrases(
        listed.credentials.filter(
          (credential) => credential.kind === "key_passphrase",
        ),
      );
      setDedicatedPhrasePaths((current) =>
        current.filter((path) => path !== item.relativePath),
      );
      setChosenPhrase("");
    } catch {
      setFailure(t("keys.unassignPassphraseFailed"));
    }
  }

  async function submitRegistration(item: KeyItem) {
    setFailure("");
    try {
      await api.registerWithAgent(item.id, {
        passphrase: agentPassphrase,
        lifetimeSeconds: agentLifetime,
      });
      closeAgentForm();
      await refresh();
    } catch {
      setAgentPassphrase("");
      setFailure(t("keys.agentFailed"));
    }
  }

  async function showPublicKey(item: KeyItem) {
    setFailure("");
    setRevealing(null);
    setPublicKeyView(null);
    try {
      const response = await api.publicKey(item.id);
      setPublicKeyView({
        id: item.id,
        relativePath: response.relativePath,
        text: response.publicKey.trimEnd(),
      });
    } catch {
      setPublicKeyView(null);
      setFailure(t("keys.publicKeyFailed"));
    }
  }

  async function moveChosen(target: MoveTarget) {
    if (inventory === null) return;
    const items = inventory.items.filter((item) => chosen.has(item.id));
    if (items.length === 0) return;
    setFailure("");
    const outcome = await moveInto(
      (keyId, change) => api.relocate(keyId, change),
      items,
      target,
    );
    setMoveOutcome(outcome);
    setChosen(new Set());
    await refresh();
  }

  function beginDrag(event: DragEvent<HTMLSpanElement>, item: KeyItem) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", item.relativePath);
    setDragging(true);
    if (!chosen.has(item.id)) setChosen(new Set([item.id]));
  }

  async function submitRelocation(item: KeyItem) {
    setFailure("");
    setRelocated(null);
    try {
      const currentGroup = groupOfKeyPath(item.relativePath);
      const response = await api.relocate(item.id, {
        ...(newName === relocateStem(item) ? {} : { newName }),
        ...(newGroup === currentGroup ? {} : { group: newGroup }),
      });
      setRelocated(response);
      if (response.blockers.length > 0) return;
      closeRelocateForm();
      await refresh();
    } catch {
      setFailure(t("keys.relocateFailed"));
    }
  }

  async function moveToTrash(keyId: string) {
    setFailure("");
    try {
      await api.trash(keyId);
      setPendingTrash(null);
      await refresh();
    } catch {
      setFailure(t("keys.trashFailed"));
    }
  }

  function trashGroup(item: KeyItem): KeyItem[] {
    const fingerprint = item.fingerprint;
    if (fingerprint === "") return [item];
    return inventory === null
      ? [item]
      : inventory.items.filter(
          (candidate) => candidate.fingerprint === fingerprint,
        );
  }

  async function restore(entryId: string) {
    setFailure("");
    try {
      const response = await api.restore(entryId);
      if (response.blockers.length > 0) {
        setFailure(
          t("keys.restoreRefused", { blockers: response.blockers.join(", ") }),
        );
        return;
      }
      await refresh();
    } catch {
      setFailure(t("keys.restoreFailed"));
    }
  }

  async function purge(entryId: string): Promise<boolean> {
    setFailure("");
    try {
      await api.purge(entryId);
      await refresh();
      return true;
    } catch {
      setFailure(t("keys.purgeFailed"));
      return false;
    }
  }

  if (state === "loading") {
    return <PanelState tone="loading" title={t("keys.reading")} />;
  }
  if (state === "error" || inventory === null || trash === null) {
    return <PanelState tone="failed" title={t("keys.unreadable")} />;
  }

  const query = keyQuery.trim().toLowerCase();
  const shown = shownItems(inventory.items, listFilter);
  const rows = folderRows(shown, groups);
  const folderItems = itemsInFolder(shown, folder);
  const directlyMatchedItems = folderItems.filter(
    (item) =>
      query === "" ||
      item.relativePath.toLowerCase().includes(query) ||
      item.kind.toLowerCase().includes(query) ||
      item.algorithm.toLowerCase().includes(query) ||
      item.fingerprint.toLowerCase().includes(query) ||
      item.references.some((reference) =>
        reference.hostPatterns.some((pattern) =>
          pattern.toLowerCase().includes(query),
        ),
      ),
  );
  const visibleItems =
    query === ""
      ? directlyMatchedItems
      : includeKeyPairContext(folderItems, directlyMatchedItems);
  const keyAttention =
    inventory.unreadable.length +
    inventory.unresolvedReferences.length +
    inventory.items.filter((item) => item.permissionRisk).length;
  const inlineKeyPanel =
    revealing !== null
      ? {
          itemId: revealing.id,
          content: (
            <RevealDialog
              keyId={revealing.id}
              relativePath={revealing.relativePath}
              api={api}
              onClose={() => setRevealing(null)}
            />
          ),
        }
      : publicKeyView !== null
        ? {
            itemId: publicKeyView.id,
            content: (
              <section
                aria-labelledby="public-key-heading"
                className="flex flex-col gap-3 rounded-md border border-line bg-surface-subtle p-3 sm:p-4"
              >
                <h3 id="public-key-heading" className={sectionHeading}>
                  {t("keys.publicKeyHeading", {
                    path: publicKeyView.relativePath,
                  })}
                </h3>
                <pre
                  aria-label={t("keys.publicKeyLabel")}
                  className="overflow-x-auto rounded-md bg-canvas p-4 text-xs"
                >
                  {publicKeyView.text}
                </pre>
                <div className="flex flex-wrap gap-2">
                  <CopyButton
                    value={publicKeyView.text}
                    label="copy.publicKey"
                  />
                  <Button onClick={() => setPublicKeyView(null)}>
                    {t("keys.close")}
                  </Button>
                </div>
              </section>
            ),
          }
        : null;

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
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-control-line bg-card p-3">
          <p className="grow text-sm text-ink">
            {t("keys.chosenCount", { count: chosen.size })}
          </p>
          <select
            aria-label={t("keys.moveTargetLabel")}
            className={control}
            value={moveTarget}
            onChange={(event) => setMoveTarget(event.target.value)}
          >
            <option value="">{t("keys.folderUngrouped")}</option>
            {groups.map((group) => (
              <option key={group} value={group}>
                {group}
              </option>
            ))}
          </select>
          <Button
            kind="primary"
            onClick={() =>
              void moveChosen(
                moveTarget === ""
                  ? { kind: "ungrouped" }
                  : { kind: "group", name: moveTarget },
              )
            }
          >
            {t("keys.moveChosen")}
          </Button>
          <Button onClick={() => setChosen(new Set())}>
            {t("keys.clearChosen")}
          </Button>
        </div>
      )}
      {moveOutcome === null ? null : (
        <div
          role="status"
          className="rounded-lg border border-line bg-card p-3 text-sm"
        >
          <p className="text-ink">
            {t("keys.moveMoved", { count: moveOutcome.moved.length })}
          </p>
          {moveOutcome.blocked.map((entry) => (
            <p key={entry.path} className="text-danger">
              {t("keys.moveBlocked", {
                path: entry.path,
                reason: entry.blockers.join(" / "),
              })}
            </p>
          ))}
          {moveOutcome.failed.map((path) => (
            <p key={path} className="text-danger">
              {t("keys.moveFailed", { path })}
            </p>
          ))}
        </div>
      )}

      <section
        aria-label={t("keys.metricFiles")}
        className="flex flex-col gap-3"
      >
        <div className="flex flex-wrap items-end justify-between gap-3">
          <p className="font-mono text-xs font-medium text-ink-muted">~/.ssh</p>
          <div className="flex w-full flex-wrap gap-2 sm:w-auto">
            <label>
              <span className="sr-only">{t("keys.listFilter")}</span>
              <select
                className={control}
                value={listFilter}
                onChange={(event) =>
                  setListFilter(event.target.value as ListFilter)
                }
              >
                <option value="keys">{t("keys.listFilterKeys")}</option>
                <option value="all">{t("keys.listFilterAll")}</option>
              </select>
            </label>
            <label className="min-w-0 grow sm:w-72 sm:grow-0">
              <span className="sr-only">{t("keys.search")}</span>
              <input
                type="search"
                value={keyQuery}
                onChange={(event) => setKeyQuery(event.target.value)}
                placeholder={t("keys.searchPlaceholder")}
                className={control}
              />
            </label>
          </div>
        </div>
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
                revealRelated={query !== ""}
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
        members={pendingTrash === null ? [] : trashGroup(pendingTrash)}
        onConfirm={(id) => void moveToTrash(id)}
        onCancel={() => setPendingTrash(null)}
      />

      {inventory.unreadable.length > 0 && (
        <section
          aria-labelledby="unreadable-heading"
          className="flex flex-col gap-2"
        >
          <h3
            id="unreadable-heading"
            className="text-sm font-medium text-notice-ink"
          >
            {t("keys.unreadableHeading")}
          </h3>
          <p className="text-sm text-ink-muted">{t("keys.unreadableNote")}</p>
          <ul className="text-sm text-ink-muted">
            {inventory.unreadable.map((file) => (
              <li key={file.relativePath}>
                {t("keys.unreadableEntry", {
                  path: file.relativePath,
                  reason: file.reason,
                })}
              </li>
            ))}
          </ul>
        </section>
      )}

      {inventory.unresolvedReferences.length > 0 && (
        <section
          aria-labelledby="unresolved-heading"
          className="flex flex-col gap-2"
        >
          <h3
            id="unresolved-heading"
            className="text-sm font-medium text-notice-ink"
          >
            {t("keys.unresolvedHeading")}
          </h3>
          <ul className="text-sm text-ink-muted">
            {inventory.unresolvedReferences.map((reference) => (
              <li
                key={`${reference.configPath}:${reference.line}:${reference.value}`}
              >
                {t("keys.referenceWithReason", {
                  directive: reference.directive,
                  value: reference.value,
                  path: reference.configPath,
                  line: reference.line,
                  reason: reference.reason,
                })}
              </li>
            ))}
          </ul>
        </section>
      )}

      <AgentSection inventory={inventory} />

      <AgentForm
        form={agentForm}
        storedPhrases={storedPhrases}
        onSubmit={(item) => void submitRegistration(item)}
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
        onSubmit={(item) => void submitPassphrase(item)}
      />

      <KeyGenerationSection
        api={api}
        variants={variants}
        groups={groups}
        onGenerated={refresh}
        onFailure={setFailure}
        onAssignGeneratedKey={onAssignGeneratedKey}
        onInstallGeneratedKey={onInstallGeneratedKey}
      />
    </section>
  );
}
