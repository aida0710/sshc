import { useCallback, useEffect, useState } from "react";
import { failureCode } from "../api/client";
import {
  integrationsApi,
  type CredentialList,
  type CredentialKind,
  type IntegrationsApi,
  type PasswordVaultStatus,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { PasswordField } from "../ui/PasswordField";
import {
  Field,
  control,
  hintText,
  sectionHeading,
} from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

type SecretsPanelProps = {
  api?: IntegrationsApi;
  onLock?: () => void;
};

const kinds: {
  kind: CredentialKind;
  heading: MessageKey;
  nameLabel: MessageKey;
  valueLabel: MessageKey;
  store: MessageKey;
}[] = [
  {
    kind: "password",
    heading: "secrets.passwordsHeading",
    nameLabel: "secrets.newPasswordName",
    valueLabel: "secrets.newPasswordValue",
    store: "secrets.storePassword",
  },
  {
    kind: "key_passphrase",
    heading: "secrets.passphrasesHeading",
    nameLabel: "secrets.newPassphraseName",
    valueLabel: "secrets.newPassphraseValue",
    store: "secrets.storePassphrase",
  },
];

function emptyCredentialList(): CredentialList {
  return {
    credentials: [],
    dedicatedKeyPassphrases: [],
    keyHostUsageComplete: true,
  };
}

type UsageListProps = {
  label: string;
  values: string[];
  emptyLabel: string;
};

function UsageList({ label, values, emptyLabel }: UsageListProps) {
  return (
    <div className="flex flex-col gap-1">
      <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
        {label}
      </p>
      {values.length === 0 ? (
        <p className={hintText}>{emptyLabel}</p>
      ) : (
        <ul aria-label={label} className="flex flex-wrap gap-2">
          {values.map((value) => (
            <li
              key={value}
              className="rounded-md bg-tree px-2 py-1 font-mono text-xs text-ink"
            >
              {value}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function keyBasename(key: string): string {
  const components = key.split("/").filter(Boolean);
  return components[components.length - 1] ?? key;
}

export function SecretsPanel({ api = integrationsApi, onLock }: SecretsPanelProps) {
  const t = useTranslate();
  const [status, setStatus] = useState<PasswordVaultStatus | null>(null);
  const [credentialList, setCredentialList] = useState<CredentialList>(emptyCredentialList);
  const [master, setMaster] = useState("");
  const [drafts, setDrafts] = useState<Record<string, { name: string; secret: string }>>({});
  const [error, setError] = useState("");

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

  async function run(action: () => Promise<unknown>, fallback: string) {
    try {
      await action();
      setError("");
      await reload();
    } catch (caught) {
      setError(failureCode(caught) === "credential_in_use" ? t("secrets.inUse") : fallback);
    }
  }

  function draftFor(kind: CredentialKind) {
    return drafts[kind] ?? { name: "", secret: "" };
  }

  if (status === null) {
    return <p className={hintText}>{t("secrets.loading")}</p>;
  }

  if (!status.unlocked) {
    const creating = !status.exists;
    return (
      <div className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}>
        <PageHeader title={t("secrets.heading")} description={t("secrets.pageDescription")} />
        <section aria-label={t("secrets.heading")} className="sshc-card grid overflow-hidden rounded-xl bg-card md:grid-cols-[minmax(0,0.9fr)_minmax(18rem,1.1fr)]">
          <div className="flex flex-col justify-between gap-8 bg-toolbar p-6 md:p-8">
            <span className="flex h-12 w-12 items-center justify-center rounded-xl bg-select-fill text-accent">
              <Icon name="secrets" className="h-6 w-6" />
            </span>
            <div>
              <h3 className="text-lg font-semibold text-ink">{creating ? t("secrets.create") : t("secrets.unlock")}</h3>
              <p className="mt-2 text-sm leading-6 text-ink-muted">{creating ? t("secrets.explainNew") : t("secrets.explainLocked")}</p>
            </div>
          </div>
          <div className="flex flex-col justify-center gap-4 p-6 md:p-8">
            {error === "" ? null : <Notice tone="danger">{error}</Notice>}
            <PasswordField label={t("secrets.master")} value={master} onChange={setMaster} />
            <Button
              kind="primary"
              className="self-start"
              onClick={() =>
                void run(
                  () => (creating ? api.initialiseVault(master) : api.unlockVault(master)),
                  creating ? t("secrets.createFailed") : t("secrets.unlockFailed"),
                ).then(() => setMaster(""))
              }
            >
              {creating ? t("secrets.create") : t("secrets.unlock")}
            </Button>
          </div>
        </section>
      </div>
    );
  }

  const { credentials, dedicatedKeyPassphrases, keyHostUsageComplete } = credentialList;
  const passwordCount = credentials.filter((credential) => credential.kind === "password").length;
  const passphraseCount = credentials.filter((credential) => credential.kind === "key_passphrase").length +
    dedicatedKeyPassphrases.length;
  const assignmentCount = credentials.reduce((count, credential) => count + credential.uses.length, 0) +
    dedicatedKeyPassphrases.length;

  return (
    <div className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}>
      <PageHeader
        title={t("secrets.heading")}
        description={t("secrets.pageDescription")}
        actions={<Button onClick={() => void api.lockVault().then(() => onLock?.())}>{t("secrets.lock")}</Button>}
      />
      <dl className="sshc-card flex flex-wrap overflow-hidden rounded-xl bg-toolbar">
        {[
          [t("secrets.metricPasswords"), passwordCount],
          [t("secrets.metricPassphrases"), passphraseCount],
          [t("secrets.metricAssignments"), assignmentCount],
        ].map(([label, value]) => (
          <div key={String(label)} className="flex min-w-40 flex-1 items-center justify-between gap-4 border-r border-hairline px-4 py-2.5 last:border-r-0">
            <dt className="text-xs font-medium text-ink-muted">{label}</dt>
            <dd className="font-mono text-sm font-semibold text-ink">{value}</dd>
          </div>
        ))}
      </dl>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {keyHostUsageComplete ? null : <Notice>{t("secrets.keyHostUsageIncomplete")}</Notice>}

      {kinds.map((group) => {
        const draft = draftFor(group.kind);
        const mine = credentials.filter((credential) => credential.kind === group.kind);
        const dedicated = group.kind === "key_passphrase" ? dedicatedKeyPassphrases : [];
        return (
          <section key={group.kind} aria-label={t(group.heading)} className="sshc-card overflow-hidden rounded-xl bg-card">
            <header className="flex items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
              <div className="flex items-center gap-2">
                <Icon name={group.kind === "password" ? "connections" : "keys"} className="h-4 w-4 text-ink-muted" />
                <h3 className={sectionHeading}>{t(group.heading)}</h3>
              </div>
              <span className="rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-ink-muted">{mine.length + dedicated.length}</span>
            </header>
            {mine.length === 0 && dedicated.length === 0 ? (
              <p className="px-4 py-6 text-sm text-ink-muted">{t("secrets.none")}</p>
            ) : (
              <ul className="divide-y divide-line">
                {mine.map((credential) => (
                  <li key={credential.name}>
                    <article
                      aria-label={credential.name}
                      className="grid gap-4 px-4 py-4 lg:grid-cols-[minmax(10rem,0.8fr)_minmax(0,1.4fr)_auto] lg:items-start"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-select-fill text-ink-muted">
                          <Icon name={group.kind === "password" ? "secrets" : "keys"} className="h-4 w-4" />
                        </span>
                        <h4 className="truncate font-mono text-sm font-semibold text-ink">{credential.name}</h4>
                      </div>
                      <div className="grid gap-4 sm:grid-cols-2">
                      {credential.kind === "key_passphrase" ? (
                        <UsageList
                          label={t("secrets.keys")}
                          values={credential.uses}
                          emptyLabel={t("secrets.noKeys")}
                        />
                      ) : null}
                      {credential.kind === "password" || keyHostUsageComplete ? (
                        <UsageList
                          label={t("secrets.assignedHosts")}
                          values={credential.hosts}
                          emptyLabel={t("secrets.noAssignedHosts")}
                        />
                      ) : (
                        <div className="flex flex-col gap-1">
                          <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
                            {t("secrets.assignedHosts")}
                          </p>
                          <p className={hintText}>{t("secrets.keyHostsUnavailable")}</p>
                        </div>
                      )}
                      </div>
                      <Button
                        kind="danger"
                        onClick={() =>
                          void run(
                            () => api.deleteCredential(group.kind, credential.name),
                            t("secrets.deleteFailed"),
                          )
                        }
                      >
                        {t("secrets.delete", { name: credential.name })}
                      </Button>
                    </article>
                  </li>
                ))}
                {dedicated.map((credential) => (
                  <li key={credential.key}>
                    <article
                      aria-label={credential.key}
                      className="grid gap-4 px-4 py-4 lg:grid-cols-[minmax(10rem,0.8fr)_minmax(0,1.4fr)_auto] lg:items-start"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-select-fill text-ink-muted">
                          <Icon name="keys" className="h-4 w-4" />
                        </span>
                        <div className="min-w-0">
                          <h4 className="font-semibold text-ink">
                            {keyBasename(credential.key)}
                          </h4>
                          <p className={hintText}>{t("secrets.dedicated")}</p>
                        </div>
                      </div>
                      <div className="grid gap-4 sm:grid-cols-2">
                      <UsageList
                        label={t("secrets.keys")}
                        values={[credential.key]}
                        emptyLabel={t("secrets.noKeys")}
                      />
                      {keyHostUsageComplete ? (
                        <UsageList
                          label={t("secrets.assignedHosts")}
                          values={credential.hosts}
                          emptyLabel={t("secrets.noAssignedHosts")}
                        />
                      ) : (
                        <div className="flex flex-col gap-1">
                          <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
                            {t("secrets.assignedHosts")}
                          </p>
                          <p className={hintText}>{t("secrets.keyHostsUnavailable")}</p>
                        </div>
                      )}
                      </div>
                      <Button
                        kind="danger"
                        onClick={() =>
                          void run(
                            () => api.unassignCredential("key_passphrase", credential.key),
                            t("secrets.deleteFailed"),
                          )
                        }
                      >
                        {t("secrets.removeDedicated", { key: credential.key })}
                      </Button>
                    </article>
                  </li>
                ))}
              </ul>
            )}

            <div className="grid items-end gap-3 border-t border-line bg-toolbar px-4 py-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
              <Field label={t(group.nameLabel)}><input
                value={draft.name}
                onChange={(event) => setDrafts({ ...drafts, [group.kind]: { ...draft, name: event.target.value } })}
                className={control}
              /></Field>
              <Field label={t(group.valueLabel)}><input
                type="password"
                value={draft.secret}
                onChange={(event) => setDrafts({ ...drafts, [group.kind]: { ...draft, secret: event.target.value } })}
                className={control}
              /></Field>
              <Button
                kind="primary"
                disabled={draft.name === "" || draft.secret === ""}
                onClick={() =>
                  void run(
                    () => api.storeCredential(group.kind, draft.name, draft.secret),
                    t("secrets.storeFailed"),
                  ).then(() => setDrafts({ ...drafts, [group.kind]: { name: "", secret: "" } }))
                }
              >
                {t(group.store)}
              </Button>
            </div>
          </section>
        );
      })}

    </div>
  );
}
