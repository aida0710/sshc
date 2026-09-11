import { LockScreen } from "./LockScreen";
import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import {
  integrationsApi,
  type CredentialList,
  type CredentialKind,
  type IntegrationsApi,
  type PasswordVaultStatus,
  type TOTPCodeSet,
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
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { useMenuKeyboard } from "../ui/useMenuKeyboard";

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

type SecretsPanelProps = {
  api?: IntegrationsApi;
  onLock?: () => void;
  kind?: CredentialKind;
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

const kindDescriptions: Record<CredentialKind, MessageKey> = {
  password: "secrets.passwordsDescription",
  key_passphrase: "secrets.passphrasesDescription",
  totp: "secrets.totpDescription",
};

function readableCode(code: string): string {
  return code.length === 6 ? `${code.slice(0, 3)} ${code.slice(3)}` : code;
}

function TOTPCodeCard({ name, api }: { name: string; api: IntegrationsApi }) {
  const t = useTranslate();
  const [codes, setCodes] = useState<TOTPCodeSet | null>(null);
  const [remaining, setRemaining] = useState(0);
  const [expanded, setExpanded] = useState(false);
  const [failed, setFailed] = useState(false);

  const reload = useCallback(async () => {
    try {
      const next = await api.totpCodes(name);
      setCodes(next);
      setRemaining(next.remainingSeconds);
      setFailed(false);
    } catch {
      setFailed(true);
    }
  }, [api, name]);

  useEffect(() => {
    void reload();
  }, [reload]);

  useEffect(() => {
    if (codes === null) return;
    const timer = window.setInterval(() => {
      setRemaining((current) => {
        if (current > 1) return current - 1;
        void reload();
        return 0;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [codes, reload]);

  if (failed) {
    return <Button onClick={() => void reload()}>{t("secrets.totpRetry")}</Button>;
  }
  if (codes === null) {
    return <p className={hintText}>{t("secrets.totpLoading")}</p>;
  }
  return (
    <div className="min-w-0 rounded-md bg-surface-subtle px-3 py-2 sm:min-w-72">
      <div className="flex items-center gap-3">
        <span className="min-w-0 flex-1 whitespace-nowrap font-mono text-lg font-semibold tracking-wider text-ink">
          {readableCode(codes.current)}
        </span>
        <span className="shrink-0 text-xs tabular-nums text-ink-muted">
          {t("secrets.totpRemaining", { seconds: remaining })}
        </span>
        <button
          type="button"
          aria-expanded={expanded}
          aria-label={t(expanded ? "secrets.totpCollapse" : "secrets.totpExpand", { name })}
          className="flex size-8 shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill hover:text-ink"
          onClick={() => setExpanded((current) => !current)}
        >
          <Icon name="chevronRight" className={`size-4 transition-transform ${expanded ? "rotate-90" : ""}`} />
        </button>
      </div>
      {expanded ? (
        <div className="mt-2 grid grid-cols-2 gap-2 border-t border-line pt-2 text-xs">
          <div>
            <p className="text-ink-faint">{t("secrets.totpPrevious")}</p>
            <p className="mt-1 whitespace-nowrap font-mono tracking-wider text-ink-muted">{readableCode(codes.previous)}</p>
          </div>
          <div>
            <p className="text-ink-faint">{t("secrets.totpNext")}</p>
            <p className="mt-1 whitespace-nowrap font-mono tracking-wider text-ink-muted">{readableCode(codes.next)}</p>
          </div>
        </div>
      ) : null}
    </div>
  );
}

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

function UsageDisclosure({ label, values, emptyLabel, onRemove, removeLabel, owner }: UsageListProps & { owner: string }) {
  const t = useTranslate();
  const [expanded, setExpanded] = useState(false);
  return (
    <div className="rounded-md bg-surface-subtle">
      <button
        type="button"
        aria-expanded={expanded}
        aria-label={t(expanded ? "secrets.usageCollapse" : "secrets.usageExpand", { label, name: owner })}
        className="flex min-h-10 w-full items-center gap-2 px-3 py-2 text-left text-sm text-ink-muted hover:bg-select-fill hover:text-ink"
        onClick={() => setExpanded((current) => !current)}
      >
        <Icon name="chevronRight" className={`size-3.5 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`} />
        <span className="min-w-0 flex-1 font-medium">{label}</span>
        <span className="rounded bg-surface px-1.5 py-0.5 font-mono text-xs tabular-nums text-ink-faint">{values.length}</span>
      </button>
      {expanded ? (
        <div className="border-t border-line px-3 py-3">
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
      ) : null}
    </div>
  );
}

type CredentialActionsProps = {
  name: string;
  edit?: { label: string; onSelect: () => void };
  remove: { label: string; onSelect: () => void };
};

function CredentialActions({ name, edit, remove }: CredentialActionsProps) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useDismissibleLayer({
    open,
    containerRefs: [rootRef],
    onDismiss: () => setOpen(false),
    returnFocusRef: triggerRef,
  });
  useMenuKeyboard({ open, menuRef, onClose: () => setOpen(false) });

  function select(action: () => void) {
    setOpen(false);
    action();
  }

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        ref={triggerRef}
        type="button"
        aria-label={t("secrets.actions", { name })}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
        className="flex size-9 items-center justify-center rounded-md text-ink-muted hover:bg-select-fill hover:text-ink"
      >
        <Icon name="moreHorizontal" className="size-5" />
      </button>
      {open ? (
        <div ref={menuRef} role="menu" className="absolute right-0 top-full z-20 mt-1 min-w-48 rounded-md border border-line bg-card p-1 shadow-lg">
          {edit === undefined ? null : (
            <button type="button" role="menuitem" className="block w-full rounded px-3 py-2 text-left text-sm text-ink hover:bg-select-fill focus:bg-select-fill focus:outline-none" onClick={() => select(edit.onSelect)}>
              {edit.label}
            </button>
          )}
          <button type="button" role="menuitem" className="block w-full rounded px-3 py-2 text-left text-sm text-danger hover:bg-select-fill focus:bg-select-fill focus:outline-none" onClick={() => select(remove.onSelect)}>
            {remove.label}
          </button>
        </div>
      ) : null}
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
  kind,
}: SecretsPanelProps) {
  const t = useTranslate();
  const [status, setStatus] = useState<PasswordVaultStatus | null>(null);
  const [credentialList, setCredentialList] =
    useState<CredentialList>(emptyCredentialList);
  const [drafts, setDrafts] = useState<
    Record<string, { name: string; secret: string }>
  >({});
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<{
    kind: CredentialKind;
    name: string;
  } | null>(null);

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
    return <LockScreen exists={status.exists} passwordless={status.passwordless ?? false} api={api} onOpen={() => void reload()} />;
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
  const visibleKinds = kind === undefined
    ? kinds
    : kinds.filter((candidate) => candidate.kind === kind);
  const selectedGroup = kind === undefined
    ? null
    : kinds.find((candidate) => candidate.kind === kind) ?? null;

  return (
    <div
      className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}
    >
      <PageHeader
        title={selectedGroup === null ? t("secrets.heading") : t(selectedGroup.heading)}
        description={kind === undefined ? t("secrets.pageDescription") : t(kindDescriptions[kind])}
        actions={
          status.passwordless ? null : <Button onClick={() => void api.lockVault().then((next) => {
            setStatus(next);
            if (!next.unlocked) onLock?.();
          }).catch(() => setError(t("secrets.failed")))}>
            {t("secrets.lock")}
          </Button>
        }
      />
      {kind === undefined ? <MetricGrid className="sm:grid-cols-2 lg:grid-cols-4">
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
      </MetricGrid> : null}
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {keyHostUsageComplete ? null : (
        <Notice>{t("secrets.keyHostUsageIncomplete")}</Notice>
      )}

      {visibleKinds.map((group) => {
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
                    <article aria-label={credential.name} className="px-4 py-4">
                      <div className="flex min-w-0 items-start gap-3">
                        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-select-fill text-ink-muted">
                          <Icon
                            name={
                              group.kind === "key_passphrase" ? "keys" : "secrets"
                            }
                            className="h-4 w-4"
                          />
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center">
                            <h4 className="min-w-0 flex-1 truncate font-mono text-sm font-semibold text-ink">
                              {credential.name}
                            </h4>
                            {credential.kind === "totp" ? (
                              <TOTPCodeCard name={credential.name} api={api} />
                            ) : null}
                          </div>
                          <div className="mt-3 flex flex-col gap-2">
                            {credential.kind === "key_passphrase" ? (
                              <UsageDisclosure
                                owner={credential.name}
                                label={t("secrets.keys")}
                                values={credential.uses}
                                emptyLabel={t("secrets.noKeys")}
                              />
                            ) : null}
                            <UsageDisclosure
                              owner={credential.name}
                              label={t("secrets.assignedHosts")}
                              values={credential.hosts}
                              emptyLabel={credential.kind === "key_passphrase" && !keyHostUsageComplete
                                ? t("secrets.keyHostsUnavailable")
                                : t("secrets.noAssignedHosts")}
                              {...(credential.kind === "totp"
                                ? {
                                    onRemove: (host: string) => {
                                      void run(
                                        () => api.unassignCredential("totp", host),
                                        t("secrets.unassignTOTPFailed"),
                                      );
                                    },
                                    removeLabel: (host: string) => t("secrets.unassignTOTP", { host }),
                                  }
                                : {})}
                            />
                          </div>
                        </div>
                        <CredentialActions
                          name={credential.name}
                          edit={{
                            label: t("secrets.edit", { name: credential.name }),
                            onSelect: () => setEditing({ kind: group.kind, name: credential.name }),
                          }}
                          remove={{
                            label: t("secrets.delete", { name: credential.name }),
                            onSelect: () =>
                            void run(
                              () =>
                                api.deleteCredential(
                                  group.kind,
                                  credential.name,
                                ),
                              t("secrets.deleteFailed"),
                            ),
                          }}
                        />
                      </div>
                    </article>
                  </li>
                ))}
                {dedicated.map((credential) => (
                  <li key={credential.key}>
                    <article aria-label={credential.key} className="px-4 py-4">
                      <div className="flex min-w-0 items-start gap-3">
                        <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-select-fill text-ink-muted">
                          <Icon name="keys" className="h-4 w-4" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <h4 className="truncate font-semibold text-ink">
                            {keyBasename(credential.key)}
                          </h4>
                          <p className={hintText}>{t("secrets.dedicated")}</p>
                          <div className="mt-3 flex flex-col gap-2">
                            <UsageDisclosure
                              owner={credential.key}
                              label={t("secrets.keys")}
                              values={[credential.key]}
                              emptyLabel={t("secrets.noKeys")}
                            />
                            <UsageDisclosure
                              owner={credential.key}
                              label={t("secrets.assignedHosts")}
                              values={credential.hosts}
                              emptyLabel={keyHostUsageComplete
                                ? t("secrets.noAssignedHosts")
                                : t("secrets.keyHostsUnavailable")}
                            />
                          </div>
                        </div>
                        <CredentialActions
                          name={credential.key}
                          remove={{
                            label: t("secrets.removeDedicated", { key: credential.key }),
                            onSelect: () =>
                              void run(
                                () => api.unassignCredential("key_passphrase", credential.key),
                                t("secrets.deleteFailed"),
                              ),
                          }}
                        />
                      </div>
                    </article>
                  </li>
                ))}
              </ul>
            )}

            {group.kind !== "totp" ? null : (
              <div className="border-t border-line bg-surface-subtle px-4 py-4">
                <p className={hintText}>{t("secrets.totpAssignInConnection")}</p>
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
