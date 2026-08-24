import { useCallback, useEffect, useState, type DragEvent } from "react";
import {
  useAgentForm,
  useGenerationForm,
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
import { rowAction, rowDanger } from "./labels";
import { CopyButton } from "../ui/CopyButton";
import { useTranslate } from "../i18n/context";
import {
  CheckboxField,
  control,
  hintText,
  primaryAction,
  sectionCard,
  sectionHeading,
  tableHeadCell,
  tableHeadRow,
} from "../ui/form";
import { Button, Card, Row } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import {
  keysApi,
  type KeyInventoryResponse,
  type KeyItem,
  type KeysApi,
  type KeyVariant,
  type RelocateKeyResponse,
  type TrashListResponse,
} from "./api";
import type { GeneratedPrivateKeyHandoff, GeneratedPublicKeyHandoff } from "./workflow";
import { FolderPane } from "./FolderPane";
import type { InspectorContent } from "../ui/Inspector";
import { KeyInspector } from "./KeyInspector";
import {
  folderRows,
  groupOfKeyPath,
  itemsInFolder,
  moveInto,
  shownItems,
  type ListFilter,
  type MoveTarget,
} from "./organizer";

type KeysScreenProps = {
  api?: KeysApi;
  onInspector?: (content: InspectorContent) => void;
  groups?: string[];
  secrets?: IntegrationsApi;
  onAssignGeneratedKey?: (key: GeneratedPrivateKeyHandoff) => void;
  onInstallGeneratedKey?: (key: GeneratedPublicKeyHandoff) => void;
};

type ScreenState = "loading" | "ready" | "error";






function relocateStem(item: KeyItem): string {
  const base = item.relativePath.split("/").pop() ?? item.relativePath;
  if (item.kind === "private_key") return base;
  for (const suffix of ["-cert.pub", ".pub"]) {
    if (base.endsWith(suffix) && base.length > suffix.length) return base.slice(0, -suffix.length);
  }
  return base;
}


export function KeysScreen({
  onInspector,
  api = keysApi,
  groups = [],
  secrets = integrationsApi,
  onAssignGeneratedKey,
  onInstallGeneratedKey,
}: KeysScreenProps) {
  const t = useTranslate();
  const [state, setState] = useState<ScreenState>("loading");
  const [inventory, setInventory] = useState<KeyInventoryResponse | null>(null);
  const [trash, setTrash] = useState<TrashListResponse | null>(null);
  const [variants, setVariants] = useState<KeyVariant[]>([]);
  const {
    algorithm, setAlgorithm,
    fileName, setFileName,
    comment, setComment,
    passphrase, setPassphrase,
    unencrypted, setUnencrypted,
  } = useGenerationForm();
  const [terminalCommand, setTerminalCommand] = useState<string[] | null>(null);
  const [revealing, setRevealing] = useState<KeyItem | null>(null);
  const storedPhrases = useStoredPhrases();
  const {
    phrases, setPhrases,
    setDedicatedPhrasePaths,
    chosenPhrase, setChosenPhrase,
  } = storedPhrases;
  const passphraseForm = usePassphraseForm();
  const {
    setChangingPassphrase,
    currentPassphrase, setCurrentPassphrase,
    newPassphrase, setNewPassphrase,
    removePassphrase,
    close: closePassphraseForm,
  } = passphraseForm;
  const agentForm = useAgentForm(storedPhrases);
  const {
    setRegistering,
    agentPassphrase, setAgentPassphrase,
    agentLifetime,
    close: closeAgentForm,
  } = agentForm;
  const storedPassphraseForm = useStoredPassphraseForm(storedPhrases);
  const {
    setManagingPassphrase,
    storedPhraseName,
    storedPhraseSecret, setStoredPhraseSecret,
    close: closeStoredPassphraseForm,
  } = storedPassphraseForm;
  const [publicKeyView, setPublicKeyView] = useState<{ relativePath: string; text: string } | null>(null);
  const [relocated, setRelocated] = useState<RelocateKeyResponse | null>(null);
  const relocateForm = useRelocateForm();
  const {
    setRelocating,
    newName, setNewName,
    newGroup, setNewGroup,
    createGroup, setCreateGroup,
    close: closeRelocateForm,
  } = relocateForm;
  const [pendingPurge, setPendingPurge] = useState("");
  const [pendingTrash, setPendingTrash] = useState<KeyItem | null>(null);
  const [failure, setFailure] = useState("");
  const {
    folder, setFolder,
    chosen, setChosen,
    dragging, setDragging,
    moveOutcome, setMoveOutcome,
    moveTarget, setMoveTarget,
    listFilter, setListFilter,
    keyQuery, setKeyQuery,
    moreActionsFor, setMoreActionsFor,
    selectedKey, setSelectedKey,
  } = useOrganiser();
  const [generated, setGenerated] = useState<{
    private: GeneratedPrivateKeyHandoff;
    public: GeneratedPublicKeyHandoff;
  } | null>(null);

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

  const selected = variants.find((variant) => variant.algorithm === algorithm);
  const inProcess = selected === undefined || selected.inProcess;
  const now = Date.now();

  function closeAllForms() {
    closePassphraseForm();
    closeAgentForm();
    closeStoredPassphraseForm();
  }

  const rowActions: KeyRowActions = {
    onSelect: (item) => setSelectedKey((current) => (current === item.id ? null : item.id)),
    onToggleChosen: (item, picked) => {
      const next = new Set(chosen);
      if (picked) next.add(item.id);
      else next.delete(item.id);
      setChosen(next);
    },
    onBeginDrag: beginDrag,
    onEndDrag: () => setDragging(false),
    onReveal: (item) => setRevealing(item),
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
    onToggleMoreActions: (item) => setMoreActionsFor((current) => (current === item.id ? "" : item.id)),
    onChangePassphrase: (item) => {
      setMoreActionsFor("");
      closeAllForms();
      setChangingPassphrase(item);
    },
    onRelocate: (item) => {
      setMoreActionsFor("");
      closeAllForms();
      setRelocated(null);
      setNewName(relocateStem(item));
      setNewGroup(groupOfKeyPath(item.relativePath));
      setRelocating(item);
    },
    onMoveToTrash: (item) => {
      setMoreActionsFor("");
      closeAllForms();
      setPendingTrash(item);
    },
  };

  useEffect(() => {
    if (onInspector === undefined) return;
    const item = inventory?.items.find((candidate) => candidate.id === selectedKey);
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

  async function submitGeneration() {
    setFailure("");
    setTerminalCommand(null);
    setGenerated(null);
    try {
      if (selected !== undefined && !selected.inProcess) {
        const response = await api.hardwareCommand({ algorithm, fileName, group: createGroup, comment });
        setTerminalCommand(response.command);
        return;
      }
      const response = await api.generate({
        algorithm,
        bits: selected?.bits ?? 0,
        fileName,
        group: createGroup,
        comment,
        passphrase,
        unencrypted,
      });
      setGenerated({
        private: {
          privateKeyId: response.id,
          privateRelativePath: response.relativePath,
        },
        public: { publicRelativePath: response.publicRelativePath },
      });
      setPassphrase("");
      setFileName("");
      await refresh();
    } catch {
      setPassphrase("");
      setFailure(t("keys.createFailed"));
    }
  }


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
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
    } catch {
      setPhrases([]);
      setDedicatedPhrasePaths([]);
    }
  }

  async function assignPhrase(item: KeyItem) {
    try {
      const listed = await secrets.assignCredential("key_passphrase", item.relativePath, chosenPhrase);
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
      setDedicatedPhrasePaths((current) => current.filter((path) => path !== item.relativePath));
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
      await secrets.storeCredential("key_passphrase", storedPhraseName, storedPhraseSecret);
      const listed = await secrets.assignCredential("key_passphrase", item.relativePath, storedPhraseName);
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
      setDedicatedPhrasePaths((current) => current.filter((path) => path !== item.relativePath));
      closeStoredPassphraseForm();
    } catch {
      setStoredPhraseSecret("");
      setFailure(t("keys.storePassphraseFailed"));
    }
  }

  async function unassignPhrase(item: KeyItem) {
    setFailure("");
    try {
      const listed = await secrets.unassignCredential("key_passphrase", item.relativePath);
      setPhrases(listed.credentials.filter((credential) => credential.kind === "key_passphrase"));
      setDedicatedPhrasePaths((current) => current.filter((path) => path !== item.relativePath));
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
    try {
      const response = await api.publicKey(item.id);
      setPublicKeyView({ relativePath: response.relativePath, text: response.publicKey.trimEnd() });
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
    const outcome = await moveInto((keyId, change) => api.relocate(keyId, change), items, target);
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
      : inventory.items.filter((candidate) => candidate.fingerprint === fingerprint);
  }

  async function restore(entryId: string) {
    setFailure("");
    try {
      const response = await api.restore(entryId);
      if (response.blockers.length > 0) {
        setFailure(t("keys.restoreRefused", { blockers: response.blockers.join(", ") }));
        return;
      }
      await refresh();
    } catch {
      setFailure(t("keys.restoreFailed"));
    }
  }

  async function purge(entryId: string) {
    setFailure("");
    try {
      await api.purge(entryId);
      setPendingPurge("");
      await refresh();
    } catch {
      setFailure(t("keys.purgeFailed"));
    }
  }

  if (state === "loading") {
    return <p aria-live="polite">{t("keys.reading")}</p>;
  }
  if (state === "error" || inventory === null || trash === null) {
    return <p role="alert">{t("keys.unreadable")}</p>;
  }

  const query = keyQuery.trim().toLowerCase();
  const shown = shownItems(inventory.items, listFilter);
  const rows = folderRows(shown, groups);
  const visibleItems = itemsInFolder(shown, folder).filter((item) =>
    query === "" ||
    item.relativePath.toLowerCase().includes(query) ||
    item.kind.toLowerCase().includes(query) ||
    item.algorithm.toLowerCase().includes(query) ||
    item.fingerprint.toLowerCase().includes(query) ||
    item.references.some((reference) =>
      reference.hostPatterns.some((pattern) => pattern.toLowerCase().includes(query)),
    ),
  );
  const keyAttention =
    inventory.unreadable.length +
    inventory.unresolvedReferences.length +
    inventory.items.filter((item) => item.permissionRisk).length;

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
      <dl className="sshc-card grid overflow-hidden rounded-xl bg-card sm:grid-cols-3 sm:divide-x sm:divide-line">
        <div className="flex items-center gap-3 border-b border-line px-4 py-3 sm:border-b-0">
          <span className="grid h-9 w-9 place-items-center rounded-lg bg-surface text-accent">
            <Icon name="keys" className="h-5 w-5" />
          </span>
          <div>
            <dt className="text-xs font-medium tracking-wide text-ink-muted">{t("keys.metricFiles")}</dt>
            <dd className="mt-0.5 text-xl font-semibold tabular-nums text-ink">{inventory.items.length}</dd>
          </div>
        </div>
        <div className="border-b border-line px-4 py-3 sm:border-b-0">
          <dt className="text-xs font-medium tracking-wide text-ink-muted">{t("keys.metricPrivate")}</dt>
          <dd className="mt-1 text-xl font-semibold tabular-nums text-ink">
            {inventory.items.filter((item) => item.kind === "private_key").length}
          </dd>
        </div>
        <div className={`px-4 py-3 ${keyAttention > 0 ? "bg-notice" : ""}`}>
          <dt className={`text-xs font-medium tracking-wide ${keyAttention > 0 ? "text-notice-ink" : "text-ink-muted"}`}>
            {t("keys.metricAttention")}
          </dt>
          <dd className={`mt-1 text-xl font-semibold tabular-nums ${keyAttention > 0 ? "text-notice-ink" : "text-ink"}`}>
            {keyAttention}
          </dd>
        </div>
      </dl>
      {failure !== "" && (
        <p role="alert" className="rounded-md border border-control-line p-3 text-sm text-danger">
          {failure}
        </p>
      )}

      {chosen.size === 0 ? null : (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-control-line bg-card p-3">
          <p className="grow text-sm text-ink">{t("keys.chosenCount", { count: chosen.size })}</p>
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
            onClick={() => void moveChosen(moveTarget === "" ? { kind: "ungrouped" } : { kind: "group", name: moveTarget })}
          >
            {t("keys.moveChosen")}
          </Button>
          <Button onClick={() => setChosen(new Set())}>
            {t("keys.clearChosen")}
          </Button>
        </div>
      )}
      {moveOutcome === null ? null : (
        <div role="status" className="rounded-lg border border-line bg-card p-3 text-sm">
          <p className="text-ink">{t("keys.moveMoved", { count: moveOutcome.moved.length })}</p>
          {moveOutcome.blocked.map((entry) => (
            <p key={entry.path} className="text-danger">
              {t("keys.moveBlocked", { path: entry.path, reason: entry.blockers.join(" / ") })}
            </p>
          ))}
          {moveOutcome.failed.map((path) => (
            <p key={path} className="text-danger">
              {t("keys.moveFailed", { path })}
            </p>
          ))}
        </div>
      )}

      <section aria-label={t("keys.metricFiles")} className="flex flex-col gap-3">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <p className="font-mono text-xs font-medium text-ink-muted">~/.ssh</p>
          <div className="flex w-full flex-wrap gap-2 sm:w-auto">
            <label>
              <span className="sr-only">{t("keys.listFilter")}</span>
              <select
                className={control}
                value={listFilter}
                onChange={(event) => setListFilter(event.target.value as ListFilter)}
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
        <div className="sshc-card overflow-hidden rounded-xl bg-card">
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
                moreActionsFor={moreActionsFor}
                now={now}
                actions={rowActions}
              />
            </div>
          </div>

      <details className="border-t border-line bg-surface-subtle p-4">
        <summary className="cursor-pointer text-sm font-medium text-ink">
          {t("keys.trashSummary", { count: trash.entries.length })}
        </summary>
        <div className="mt-3 flex flex-col gap-2 border-t border-line pt-3">
        <h3 className={sectionHeading}>{t("keys.trashHeading")}</h3>
        <p className="text-sm text-ink-muted">{t("keys.trashNote")}</p>
        <div className="overflow-x-auto">
        <table className="w-full min-w-[40rem] text-left text-sm">
          <caption className="sr-only">{t("keys.trashCaption")}</caption>
          <thead>
            <tr className={tableHeadRow}>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFiles")}</th>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colAge")}</th>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colStatus")}</th>
              <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colActions")}</th>
            </tr>
          </thead>
          <tbody>
            {trash.entries.map((entry) => (
              <tr key={entry.id} className="border-b border-line align-top">
                <td className="py-2 pr-3 font-mono text-xs">
                  {entry.files.map((file) => file.originalRelativePath).join(", ")}
                </td>
                <td className="py-2 pr-3">
                  {entry.stale
                    ? t("keys.ageStale", { days: entry.ageDays, retention: trash.retentionDays })
                    : t("keys.age", { days: entry.ageDays })}
                </td>
                <td className="py-2 pr-3">{entry.restorable ? t("keys.restorable") : entry.blockers.join(", ")}</td>
                <td className="py-2">
                  <div className="flex flex-wrap items-center gap-1">
                  <button type="button" className={rowAction} onClick={() => void restore(entry.id)}>
                    {t("keys.restore")}
                  </button>
                  {pendingPurge === entry.id ? (
                    <>
                      <span>{t("keys.purgeWarning")}</span>
                      <button type="button" className={rowDanger} onClick={() => void purge(entry.id)}>
                        {t("keys.confirmPurge")}
                      </button>
                      <button type="button" className={rowAction} onClick={() => setPendingPurge("")}>
                        {t("keys.cancel")}
                      </button>
                    </>
                  ) : (
                    <button type="button" className={rowDanger} onClick={() => setPendingPurge(entry.id)}>
                      {t("keys.purge")}
                    </button>
                  )}
                  </div>
                </td>
              </tr>
            ))}
            {trash.entries.length === 0 && (
              <tr>
                <td colSpan={4} className="py-3 text-sm text-ink-muted">
                  {t("keys.trashEmpty")}
                </td>
              </tr>
            )}
          </tbody>
        </table>
        </div>
        </div>
      </details>
        </div>
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

      {publicKeyView !== null && (
        <section aria-labelledby="public-key-heading" className={sectionCard}>
          <h3 id="public-key-heading" className={sectionHeading}>
            {t("keys.publicKeyHeading", { path: publicKeyView.relativePath })}
          </h3>
          <pre aria-label={t("keys.publicKeyLabel")} className="overflow-x-auto rounded-md bg-canvas p-4 text-xs">
            {publicKeyView.text}
          </pre>
          <div className="flex gap-2">
            <CopyButton value={publicKeyView.text} label="copy.publicKey" />
            <Button onClick={() => setPublicKeyView(null)}>
              {t("keys.close")}
            </Button>
          </div>
        </section>
      )}


      {inventory.unreadable.length > 0 && (
        <section aria-labelledby="unreadable-heading" className="flex flex-col gap-2">
          <h3 id="unreadable-heading" className="text-sm font-medium text-notice-ink">
            {t("keys.unreadableHeading")}
          </h3>
          <p className="text-sm text-ink-muted">{t("keys.unreadableNote")}</p>
          <ul className="text-sm text-ink-muted">
            {inventory.unreadable.map((file) => (
              <li key={file.relativePath}>{t("keys.unreadableEntry", { path: file.relativePath, reason: file.reason })}</li>
            ))}
          </ul>
        </section>
      )}

      {inventory.unresolvedReferences.length > 0 && (
        <section aria-labelledby="unresolved-heading" className="flex flex-col gap-2">
          <h3 id="unresolved-heading" className="text-sm font-medium text-notice-ink">
            {t("keys.unresolvedHeading")}
          </h3>
          <ul className="text-sm text-ink-muted">
            {inventory.unresolvedReferences.map((reference) => (
              <li key={`${reference.configPath}:${reference.line}:${reference.value}`}>
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

      <section aria-labelledby="agent-heading" className="flex flex-col gap-2">
        <h3 id="agent-heading" className={sectionHeading}>
          {t("keys.agentHeading")}
        </h3>
        {inventory.agentAvailable ? (
          inventory.agentIdentities.length === 0 ? (
            <p className="text-sm text-ink-muted">{t("keys.agentEmpty")}</p>
          ) : (
            <table className="w-full min-w-[32rem] text-left text-sm">
              <caption className="sr-only">{t("keys.agentIdentitiesCaption")}</caption>
              <thead>
                <tr className={tableHeadRow}>
                  <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colAlgorithm")}</th>
                  <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colFingerprint")}</th>
                  <th scope="col" className={`${tableHeadCell} whitespace-nowrap`}>{t("keys.colComment")}</th>
                </tr>
              </thead>
              <tbody>
                {inventory.agentIdentities.map((identity) => (
                  <tr key={identity.fingerprint} className="border-b border-line">
                    <td className="py-2 pr-3">
                      {identity.bits > 0 ? `${identity.algorithm} · ${identity.bits}` : identity.algorithm}
                    </td>
                    <td className="py-2 pr-3 font-mono text-xs break-all">{identity.fingerprint}</td>
                    <td className="py-2">{identity.comment}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )
        ) : (
          <p className="text-sm text-notice-ink">
            {t("keys.agentUnavailable")}
          </p>
        )}
        {inventory.agentDelegations.length > 0 && (
          <>
            <p className="text-sm text-ink-muted">
              {t("keys.agentDelegationsNote")}
            </p>
            <ul className="text-sm text-ink-muted">
              {inventory.agentDelegations.map((reference) => (
                <li key={`${reference.configPath}:${reference.line}`}>
                  {t("keys.reference", {
                    directive: reference.directive,
                    value: reference.value,
                    path: reference.configPath,
                    line: reference.line,
                  })}
                  {reference.hostPatterns.length > 0 ? ` (${reference.hostPatterns.join(" ")})` : ""}
                </li>
              ))}
            </ul>
          </>
        )}
      </section>

      <AgentForm
        form={agentForm}
        storedPhrases={storedPhrases}
        onSubmit={(item) => void submitRegistration(item)}
        onAssignPhrase={(item) => void assignPhrase(item)}
      />

      <RelocateForm form={relocateForm} groups={groups} onSubmit={(item) => void submitRelocation(item)} />


      <RelocateResult result={relocated} onClose={() => setRelocated(null)} />

      {revealing !== null && (
        <RevealDialog
          keyId={revealing.id}
          relativePath={revealing.relativePath}
          api={api}
          onClose={() => setRevealing(null)}
        />
      )}

      <PassphraseForm form={passphraseForm} onSubmit={(item) => void submitPassphrase(item)} />

      <form
        aria-labelledby="create-key-heading"
        className="flex flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          void submitGeneration();
        }}
      >
        <h3 id="create-key-heading" className={sectionHeading}>
          {t("keys.createHeading")}
        </h3>
        <Card>
          <Row label={t("keys.algorithm")} stackOnNarrow>
            <select className={`${control} min-h-10 sm:min-h-0`} value={algorithm} onChange={(event) => setAlgorithm(event.target.value)}>
              {variants.map((variant) => (
                <option key={`${variant.algorithm}-${variant.bits}`} value={variant.algorithm}>
                  {variant.label}
                </option>
              ))}
            </select>
          </Row>
          <Row label={t("keys.createGroup")} stackOnNarrow>
            <select className={`${control} min-h-10 sm:min-h-0`} value={createGroup} onChange={(event) => setCreateGroup(event.target.value)}>
              <option value="">{t("keys.groupNone")}</option>
              {groups.map((group) => (
                <option key={group} value={group}>
                  {group}
                </option>
              ))}
            </select>
          </Row>
          <Row label={t("keys.fileName")} stackOnNarrow>
            <input className={`${control} min-h-10 sm:min-h-0`} value={fileName} onChange={(event) => setFileName(event.target.value)} />
          </Row>
          <Row label={t("keys.comment")} stackOnNarrow>
            <input className={`${control} min-h-10 sm:min-h-0`} value={comment} onChange={(event) => setComment(event.target.value)} />
          </Row>
          {inProcess && (
            <Row label={t("keys.passphrase")} stackOnNarrow>
              <input
                className={`${control} min-h-10 sm:min-h-0`}
                type="password"
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
                disabled={unencrypted}
              />
            </Row>
          )}
        </Card>
        {inProcess && (
          <CheckboxField
            label={t("keys.createUnencrypted")}
            checked={unencrypted}
            onChange={(checked) => {
              setUnencrypted(checked);
              setPassphrase("");
            }}
          />
        )}
        <Button kind="primary" type="submit" className="self-start">
          {inProcess ? t("keys.createSubmit") : t("keys.showTerminalCommand")}
        </Button>
      </form>

      {generated === null ? null : (
        <section aria-live="polite" className={sectionCard}>
          <h3 className={sectionHeading}>{t("keys.generatedHeading")}</h3>
          <p className={hintText}>
            {t("keys.generatedNext", { path: generated.private.privateRelativePath })}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              kind="primary"
              onClick={() => onAssignGeneratedKey?.(generated.private)}
            >
              {t("keys.assignGenerated")}
            </Button>
            <Button
              onClick={() => onInstallGeneratedKey?.(generated.public)}
            >
              {t("keys.installGenerated")}
            </Button>
          </div>
        </section>
      )}

      {terminalCommand !== null && (
        <div>
          <p className="text-sm text-ink-muted">
            {t("keys.hardwareNote")}
          </p>
          <pre aria-label={t("copy.terminalCommand")} className="overflow-x-auto rounded-md bg-canvas p-4 text-xs">
            {terminalCommand.join(" ")}
          </pre>
          <div className="mt-2">
            <CopyButton value={terminalCommand.join(" ")} label="copy.terminalCommand" />
          </div>
        </div>
      )}

    </section>
  );
}
