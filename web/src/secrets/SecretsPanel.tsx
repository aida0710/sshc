import { LockScreen } from "./LockScreen";
import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { credentialsApi, type CredentialList, type CredentialKind, type CredentialsApi } from "../api/credentials";
import { vaultApi, type PasswordVaultStatus, type VaultApi } from "../api/vault";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Button, Notice } from "../ui/surface";
import { PageHeader, phoneCardFrame } from "../ui/page";
import { Icon } from "../ui/icons";
import { PanelState } from "../ui/PanelState";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { CredentialAddPanel } from "./CredentialAddPanel";
import { CredentialEditPanel } from "./CredentialEditPanel";
import { CredentialRow, plainColumns, totpColumns, type RowPanel } from "./CredentialRow";
import { credentialRows, type CredentialRow as Row } from "./credentialRows";

// The secrets screen edits credentials, and opens, locks and re-keys the
// vault that holds them.
export type SecretsApi = CredentialsApi & VaultApi;
export const secretsApi: SecretsApi = { ...credentialsApi, ...vaultApi };

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

// A deletion the user has asked for but not yet confirmed. The dialog names
// the credential; confirming runs the call that removes it.
type PendingDeletion = { name: string; perform: () => Promise<unknown> };

type SecretsPanelProps = {
  kind: CredentialKind;
  api?: SecretsApi;
  onLock?: () => void;
};

// Each kind has a page of its own. title is the same English name as in Menu and the top bar; heading names
// the list for screen readers; assignmentColumn heads the column that says where each one is used.
const pages: Record<CredentialKind, { title: MessageKey; heading: MessageKey; assignmentColumn: MessageKey }> = {
  password: { title: "section.passwords", heading: "secrets.passwordsHeading", assignmentColumn: "secrets.assignedHosts" },
  key_passphrase: { title: "section.keyPassphrases", heading: "secrets.passphrasesHeading", assignmentColumn: "secrets.colKeysAndHosts" },
  totp: { title: "section.otp", heading: "secrets.totpHeading", assignmentColumn: "secrets.assignedHosts" },
};

function emptyCredentialList(): CredentialList {
  return {
    credentials: [],
    dedicatedKeyPassphrases: [],
    keyHostUsageComplete: true,
  };
}

// SecretsPanel is one Vault page: the saved credentials of one kind, one row each, with ［＋ 追加］ opening the
// form above the list and each row's menu opening what it needs under the row.
export function SecretsPanel({
  kind,
  api = secretsApi,
  onLock,
}: SecretsPanelProps) {
  const t = useTranslate();
  const compact = useMediaQuery(mobileViewportQuery);
  const [status, setStatus] = useState<PasswordVaultStatus | null>(null);
  const [credentialList, setCredentialList] =
    useState<CredentialList>(emptyCredentialList);
  const [error, setError] = useState("");
  const [adding, setAdding] = useState(false);
  const [openPanel, setOpenPanel] = useState<{ rowId: string; panel: RowPanel } | null>(null);
  const [pendingDeletion, setPendingDeletion] = useState<PendingDeletion | null>(null);

  const reload = useCallback(async () => {
    try {
      const vault = await api.passwordVault();
      setStatus(vault);
      if (!vault.unlocked) {
        setCredentialList(emptyCredentialList());
        return;
      }
      setCredentialList(await api.credentials());
    } catch (caught) {
      setError(failureCode(caught) || t("secrets.failed"));
    }
  }, [api, t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  async function run(action: () => Promise<unknown>, fallback: string): Promise<boolean> {
    try {
      await action();
      setError("");
      await reload();
      return true;
    } catch (caught) {
      setError(
        failureCode(caught) === "credential_in_use"
          ? t("secrets.inUse")
          : fallback,
      );
      return false;
    }
  }

  if (status === null) {
    return error === "" ? (
      <PanelState tone="loading" title={t("secrets.loading")} />
    ) : (
      <PanelState tone="failed" title={error} action={<Button onClick={() => void reload()}>{t("shell.bootstrapRetry")}</Button>} />
    );
  }

  if (!status.unlocked) {
    return <LockScreen exists={status.exists} passwordless={status.passwordless ?? false} minPassphraseLength={status.minPassphraseLength} api={api} onOpen={() => void reload()} />;
  }

  const page = pages[kind];
  const rows = credentialRows(credentialList, kind);
  const columns = kind === "totp" ? totpColumns : plainColumns;

  function lockVault() {
    void api.lockVault().then((next) => {
      setStatus(next);
      if (!next.unlocked) onLock?.();
    }).catch(() => setError(t("secrets.failed")));
  }

  function deletionOf(row: Row): PendingDeletion {
    return row.source === "named"
      ? { name: row.name, perform: () => api.deleteCredential(kind, row.name) }
      : { name: row.name, perform: () => api.unassignCredential("key_passphrase", row.passphrase.key) };
  }

  return (
    <div className={`flex flex-col gap-[22px] ${mobileTouchTargets}`}>
      <PageHeader
        title={t(page.title)}
        meta={t("secrets.count", { count: rows.length })}
        // The one thing the page does not show by itself: stored values stay hidden until edited. A TOTP seed
        // is never shown, so its page needs no line.
        description={kind === "totp" ? undefined : t("secrets.valuesShownWhileEditing")}
        actions={
          <>
            {status.passwordless ? null : compact ? (
              <Button aria-label={t("secrets.lock")} title={t("secrets.lock")} onClick={lockVault} className="inline-flex items-center justify-center px-2.5">
                <Icon name="secrets" className="size-4" />
              </Button>
            ) : (
              <Button onClick={lockVault} className="inline-flex items-center gap-1.5">
                <Icon name="secrets" className="size-4" />
                {t("secrets.lock")}
              </Button>
            )}
            <Button kind="primary" onClick={() => setAdding(true)} className="inline-flex items-center gap-1.5">
              <Icon name="plus" className="size-4" />
              {t("secrets.add")}
            </Button>
          </>
        }
      />
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {kind === "key_passphrase" && !credentialList.keyHostUsageComplete ? (
        <Notice>{t("secrets.keyHostUsageIncomplete")}</Notice>
      ) : null}

      <div>
        {adding ? (
          <CredentialAddPanel
            kind={kind}
            onCancel={() => setAdding(false)}
            onStore={async (name, secret) => {
              const stored = await run(() => api.storeCredential(kind, name, secret), t("secrets.storeFailed"));
              if (stored) setAdding(false);
              return stored;
            }}
          />
        ) : null}
        <section aria-label={t(page.heading)} className={compact ? `${phoneCardFrame} pl-3.5 pr-1.5` : ""}>
          {rows.length === 0 ? (
            <p className="py-4 text-sm text-ink-muted">{t("secrets.none")}</p>
          ) : (
            <>
              <div aria-hidden="true" className={`hidden gap-x-3 border-b border-line pb-2 text-xs text-ink-faint @2xl:grid ${columns}`}>
                <span>{t("secrets.colName")}</span>
                <span>{t(page.assignmentColumn)}</span>
                {kind === "totp" ? <span>{t("secrets.colCode")}</span> : null}
              </div>
              <ul>
                {rows.map((row) => {
                  const panel = openPanel?.rowId === row.id ? openPanel.panel : null;
                  return (
                    <CredentialRow
                      key={row.id}
                      row={row}
                      keyHostUsageComplete={credentialList.keyHostUsageComplete}
                      panel={panel}
                      api={api}
                      editPanel={panel === "edit" ? (
                        <CredentialEditPanel
                          kind={kind}
                          name={row.name}
                          api={api}
                          onSaved={(list) => {
                            setCredentialList(list);
                            setError("");
                          }}
                          onClose={() => setOpenPanel(null)}
                        />
                      ) : null}
                      actions={{
                        onOpenPanel: (next) => setOpenPanel(next === null ? null : { rowId: row.id, panel: next }),
                        onDelete: () => setPendingDeletion(deletionOf(row)),
                        onUnassignTOTP: (host) => void run(() => api.unassignCredential("totp", host), t("secrets.unassignTOTPFailed")),
                      }}
                    />
                  );
                })}
              </ul>
            </>
          )}
        </section>
        {kind === "totp" ? (
          <p className="mt-3 text-xs leading-[18px] text-ink-faint">{t("secrets.totpCopyHint")}{t("secrets.totpAssignInConnection")}</p>
        ) : null}
      </div>

      {pendingDeletion === null ? null : (
        <ConfirmDialog
          id="secrets-delete-heading"
          heading={t("secrets.deleteHeading", { name: pendingDeletion.name })}
          body={<p className="text-sm text-ink-muted">{t("secrets.deleteBody")}</p>}
          confirmLabel={t("secrets.confirmDelete")}
          cancelLabel={t("secrets.deleteCancel")}
          onCancel={() => setPendingDeletion(null)}
          onConfirm={() => {
            setPendingDeletion(null);
            setOpenPanel(null);
            void run(pendingDeletion.perform, t("secrets.deleteFailed"));
          }}
        />
      )}
    </div>
  );
}
