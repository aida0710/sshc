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
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { Button, Card, Notice } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
import { CredentialEditDialog } from "./CredentialEditDialog";
import { PanelState } from "../ui/PanelState";

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
  {
    kind: "totp",
    heading: "secrets.totpHeading",
    nameLabel: "secrets.newTOTPName",
    valueLabel: "secrets.newTOTPValue",
    store: "secrets.storeTOTP",
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
  onRemove?: (value: string) => void;
  removeLabel?: (value: string) => string;
};

function UsageList({ label, values, emptyLabel, onRemove, removeLabel }: UsageListProps) {
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
              className="flex items-center gap-1 rounded-md bg-tree px-2 py-1 font-mono text-xs text-ink"
            >
              <span>{value}</span>
              {onRemove === undefined ? null : (
                <button
                  type="button"
                  className="ml-1 text-ink-muted hover:text-danger"
                  aria-label={removeLabel?.(value) ?? value}
                  onClick={() => onRemove(value)}
                >
                  ×
                </button>
              )}
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

export function SecretsPanel({
  api = integrationsApi,
  onLock,
}: SecretsPanelProps) {
  const t = useTranslate();
  const [status, setStatus] = useState<PasswordVaultStatus | null>(null);
  const [credentialList, setCredentialList] =
    useState<CredentialList>(emptyCredentialList);
  const [master, setMaster] = useState("");
  const [drafts, setDrafts] = useState<
    Record<string, { name: string; secret: string }>
  >({});
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<{
    kind: CredentialKind;
    name: string;
  } | null>(null);
  const [totpHost, setTOTPHost] = useState("");
  const [totpCredential, setTOTPCredential] = useState("");

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

  function draftFor(kind: CredentialKind) {
    return drafts[kind] ?? { name: "", secret: "" };
  }

  if (status === null) {
    return error === "" ? (
      <PanelState tone="loading" title={t("secrets.loading")} />
    ) : (
      <PanelState tone="failed" title={error} action={<Button onClick={() => void reload()}>{t("shell.bootstrapRetry")}</Button>} />
    );
  }

  if (!status.unlocked) {
    const creating = !status.exists;
    return (
      <div
        className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
      >
        <PageHeader
          title={t("secrets.heading")}
          description={t("secrets.pageDescription")}
        />
        <Card
          as="section"
          aria-label={t("secrets.heading")}
          radius="md"
          className="grid md:grid-cols-[minmax(0,0.9fr)_minmax(18rem,1.1fr)]"
        >
          <div className="flex flex-col justify-between gap-8 bg-toolbar p-6 md:p-8">
            <span className="flex h-12 w-12 items-center justify-center rounded-md bg-select-fill text-accent">
              <Icon name="secrets" className="h-6 w-6" />
            </span>
            <div>
              <h3 className="text-lg font-semibold text-ink">
                {creating ? t("secrets.create") : t("secrets.unlock")}
              </h3>
              <p className="mt-2 text-sm leading-6 text-ink-muted">
                {creating
                  ? t("secrets.explainNew")
                  : t("secrets.explainLocked")}
              </p>
            </div>
          </div>
          <div className="flex flex-col justify-center gap-4 p-6 md:p-8">
            {error === "" ? null : <Notice tone="danger">{error}</Notice>}
            <PasswordField
              label={t("secrets.master")}
              value={master}
              onChange={setMaster}
            />
            <Button
              kind="primary"
              className="self-start"
              onClick={() =>
                void run(
                  () =>
                    creating
                      ? api.initialiseVault(master)
                      : api.unlockVault(master),
                  creating
                    ? t("secrets.createFailed")
                    : t("secrets.unlockFailed"),
                ).then(() => setMaster(""))
              }
            >
              {creating ? t("secrets.create") : t("secrets.unlock")}
            </Button>
          </div>
        </Card>
      </div>
    );
  }

  const { credentials, dedicatedKeyPassphrases, keyHostUsageComplete } =
    credentialList;
  const passwordCount = credentials.filter(
    (credential) => credential.kind === "password",
  ).length;
  const passphraseCount =
    credentials.filter((credential) => credential.kind === "key_passphrase")
      .length + dedicatedKeyPassphrases.length;
  const totpCount = credentials.filter(
    (credential) => credential.kind === "totp",
  ).length;
  const assignmentCount =
    credentials.reduce(
      (count, credential) => count + credential.uses.length,
      0,
    ) + dedicatedKeyPassphrases.length;

  return (
    <div
      className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
    >
      <PageHeader
        title={t("secrets.heading")}
        description={t("secrets.pageDescription")}
        actions={
          <Button onClick={() => void api.lockVault().then(() => onLock?.())}>
            {t("secrets.lock")}
          </Button>
        }
      />
      <MetricGrid className="sm:grid-cols-2 lg:grid-cols-4">
        {([
          [t("secrets.metricPasswords"), passwordCount],
          [t("secrets.metricPassphrases"), passphraseCount],
          [t("secrets.metricTOTP"), totpCount],
          [t("secrets.metricAssignments"), assignmentCount],
        ] as const).map(([label, value]) => (
          <MetricCard
            key={String(label)}
            label={String(label)}
            value={value}
            compact
          />
        ))}
      </MetricGrid>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {keyHostUsageComplete ? null : (
        <Notice>{t("secrets.keyHostUsageIncomplete")}</Notice>
      )}

      {kinds.map((group) => {
        const draft = draftFor(group.kind);
        const mine = credentials.filter(
          (credential) => credential.kind === group.kind,
        );
        const dedicated =
          group.kind === "key_passphrase" ? dedicatedKeyPassphrases : [];
        return (
          <Card
            as="section"
            key={group.kind}
            aria-label={t(group.heading)}
            radius="md"
          >
            <header className="flex items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
              <div className="flex items-center gap-2">
                <Icon
                  name={group.kind === "key_passphrase" ? "keys" : "connections"}
                  className="h-4 w-4 text-ink-muted"
                />
                <h3 className={sectionHeading}>{t(group.heading)}</h3>
              </div>
              <span className="rounded-md bg-surface px-2 py-0.5 font-mono text-xs text-ink-muted">
                {mine.length + dedicated.length}
              </span>
            </header>
            {mine.length === 0 && dedicated.length === 0 ? (
              <p className="px-4 py-6 text-sm text-ink-muted">
                {t("secrets.none")}
              </p>
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
                          <Icon
                            name={
                              group.kind === "key_passphrase" ? "keys" : "secrets"
                            }
                            className="h-4 w-4"
                          />
                        </span>
                        <h4 className="truncate font-mono text-sm font-semibold text-ink">
                          {credential.name}
                        </h4>
                      </div>
                      <div className="grid gap-4 sm:grid-cols-2">
                        {credential.kind === "key_passphrase" ? (
                          <UsageList
                            label={t("secrets.keys")}
                            values={credential.uses}
                            emptyLabel={t("secrets.noKeys")}
                          />
                        ) : null}
                        {credential.kind !== "key_passphrase" ||
                        keyHostUsageComplete ? (
                          <UsageList
                            label={t("secrets.assignedHosts")}
                            values={credential.hosts}
                            emptyLabel={t("secrets.noAssignedHosts")}
                            {...(credential.kind === "totp"
                              ? {
                                  onRemove: (host: string) => {
                                    void run(
                                      () =>
                                        api.unassignCredential("totp", host),
                                      t("secrets.unassignTOTPFailed"),
                                    );
                                  },
                                  removeLabel: (host: string) =>
                                    t("secrets.unassignTOTP", { host }),
                                }
                              : {})}
                          />
                        ) : (
                          <div className="flex flex-col gap-1">
                            <p className="text-xs font-semibold uppercase tracking-wide text-ink-muted">
                              {t("secrets.assignedHosts")}
                            </p>
                            <p className={hintText}>
                              {t("secrets.keyHostsUnavailable")}
                            </p>
                          </div>
                        )}
                      </div>
                      <div className="flex flex-wrap justify-end gap-2">
                        <Button
                          onClick={() =>
                            setEditing({
                              kind: group.kind,
                              name: credential.name,
                            })
                          }
                        >
                          {t("secrets.edit", { name: credential.name })}
                        </Button>
                        <Button
                          kind="danger"
                          onClick={() =>
                            void run(
                              () =>
                                api.deleteCredential(
                                  group.kind,
                                  credential.name,
                                ),
                              t("secrets.deleteFailed"),
                            )
                          }
                        >
                          {t("secrets.delete", { name: credential.name })}
                        </Button>
                      </div>
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
                            <p className={hintText}>
                              {t("secrets.keyHostsUnavailable")}
                            </p>
                          </div>
                        )}
                      </div>
                      <Button
                        kind="danger"
                        onClick={() =>
                          void run(
                            () =>
                              api.unassignCredential(
                                "key_passphrase",
                                credential.key,
                              ),
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

            {group.kind !== "totp" ? null : (
              <div className="flex flex-col gap-3 border-t border-line bg-surface-subtle px-4 py-4">
                <Notice>{t("secrets.totpWarning")}</Notice>
                <div className="grid items-end gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
                  <Field label={t("secrets.totpHost")}>
                    <input
                      value={totpHost}
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      onChange={(event) => setTOTPHost(event.target.value)}
                      className={control}
                    />
                  </Field>
                  <Field label={t("secrets.totpCredential")}>
                    <select
                      value={totpCredential}
                      onChange={(event) => setTOTPCredential(event.target.value)}
                      className={control}
                    >
                      <option value="">{t("secrets.chooseTOTP")}</option>
                      {mine.map((credential) => (
                        <option key={credential.name} value={credential.name}>
                          {credential.name}
                        </option>
                      ))}
                    </select>
                  </Field>
                  <Button
                    disabled={totpHost === "" || totpCredential === ""}
                    onClick={() =>
                      void run(
                        () =>
                          api.assignCredential(
                            "totp",
                            totpHost,
                            totpCredential,
                          ),
                        t("secrets.assignTOTPFailed"),
                      ).then((assigned) => {
                        if (assigned) setTOTPHost("");
                      })
                    }
                  >
                    {t("secrets.assignTOTP")}
                  </Button>
                </div>
              </div>
            )}

            <div className="grid items-end gap-3 border-t border-line bg-toolbar px-4 py-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]">
              <Field label={t(group.nameLabel)}>
                <input
                  value={draft.name}
                  onChange={(event) =>
                    setDrafts({
                      ...drafts,
                      [group.kind]: { ...draft, name: event.target.value },
                    })
                  }
                  className={control}
                />
              </Field>
              <PasswordField
                label={t(group.valueLabel)}
                value={draft.secret}
                onChange={(value) =>
                  setDrafts({
                    ...drafts,
                    [group.kind]: { ...draft, secret: value },
                  })
                }
              />
              <Button
                kind="primary"
                disabled={draft.name === "" || draft.secret === ""}
                onClick={() =>
                  void run(
                    () =>
                      api.storeCredential(group.kind, draft.name, draft.secret),
                    t("secrets.storeFailed"),
                  ).then((stored) => {
                    if (!stored) return;
                    setDrafts((current) => ({
                      ...current,
                      [group.kind]: { name: "", secret: "" },
                    }));
                  })
                }
              >
                {t(group.store)}
              </Button>
            </div>
          </Card>
        );
      })}

      {editing === null ? null : (
        <CredentialEditDialog
          kind={editing.kind}
          name={editing.name}
          api={api}
          onSaved={(list) => {
            setCredentialList(list);
            setError("");
            setEditing(null);
          }}
          onClose={() => setEditing(null)}
        />
      )}
    </div>
  );
}
