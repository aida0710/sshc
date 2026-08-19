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
  dangerAction,
  hintText,
  primaryAction,
  secondaryAction,
  sectionCard,
  sectionHeading,
} from "../ui/form";
import { Notice } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";

type SecretsPanelProps = {
  api?: IntegrationsApi;
  // onLock はシェル自身の玄関を閉じさせる。vault をロックすることは
  // アプリケーションをロックすることなので、自分自身を再読み込みするだけの
  // パネルでは、次のリクエストが拒否されるシェルの中にユーザーを取り残してしまう。
  onLock?: () => void;
};

// 2 つの名前空間を、引き離して描く。
//
// フォーマットでも API でも型でも分かれているのは、1 つの名前空間だと
// ホストのピッカーが鍵のパスフレーズを提供してしまい、それを選ぶと
// そのパスフレーズがログインパスワードとしてリモートホストに送られて
// しまうからだ。ここで 1 つのリストとして描けば、フォーマットが取り除いた
// はずの選択をユーザーの前に戻してしまう。だから互いのエントリを
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
  return key.split("/").filter(Boolean).at(-1) ?? key;
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
      // 決して保持しない 2 つのリストになっている。
      // 閉じた vault にはその中身を尋ねない。起動時にも何も尋ねない:
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
      // この画面は必要になったときに自分自身のために尋ねる。
      // サーバーが説明する拒否は、そのままの姿で表示する。「使用中」が
      setError(failureCode(caught) === "credential_in_use" ? t("secrets.inUse") : fallback);
    }
  }

  function draftFor(kind: CredentialKind) {
    return drafts[kind] ?? { name: "", secret: "" };
  }

  if (status === null) {
    return <p className={hintText}>{t("secrets.loading")}</p>;
  }

  // 人が出会うものであり、それこそが対処できるものだ。
  // 存在することと開いていることは、この画面にとって同じ意味ではない:
  // 表示するものは何もなく、それを変えるのがマスターパスワードだ。異なるのは
  if (!status.unlocked) {
    const creating = !status.exists;
    return (
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
        <PageHeader title={t("secrets.heading")} description={t("secrets.pageDescription")} />
        <section aria-label={t("secrets.heading")} className={sectionCard}>
          <h3 className={sectionHeading}>{creating ? t("secrets.create") : t("secrets.unlock")}</h3>
          <p className={hintText}>{creating ? t("secrets.explainNew") : t("secrets.explainLocked")}</p>
          {error === "" ? null : <Notice tone="danger">{error}</Notice>}
          <PasswordField label={t("secrets.master")} value={master} onChange={setMaster} />
          <div>
            <button
              type="button"
              className={primaryAction}
              onClick={() =>
                void run(
                  () => (creating ? api.initialiseVault(master) : api.unlockVault(master)),
                  creating ? t("secrets.createFailed") : t("secrets.unlockFailed"),
                ).then(() => setMaster(""))
              }
            >
              {creating ? t("secrets.create") : t("secrets.unlock")}
            </button>
          </div>
        </section>
      </div>
    );
  }

  const { credentials, dedicatedKeyPassphrases, keyHostUsageComplete } = credentialList;

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("secrets.heading")} description={t("secrets.pageDescription")} />
      <MetricGrid>
        <MetricCard
          label={t("secrets.metricPasswords")}
          value={credentials.filter((credential) => credential.kind === "password").length}
        />
        <MetricCard
          label={t("secrets.metricPassphrases")}
          value={
            credentials.filter((credential) => credential.kind === "key_passphrase").length +
            dedicatedKeyPassphrases.length
          }
        />
        <MetricCard
          label={t("secrets.metricAssignments")}
          value={
            credentials.reduce((count, credential) => count + credential.uses.length, 0) +
            dedicatedKeyPassphrases.length
          }
        />
      </MetricGrid>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {keyHostUsageComplete ? null : <Notice>{t("secrets.keyHostUsageIncomplete")}</Notice>}
      <div>
        <button
          type="button"
          className={secondaryAction}
          onClick={() => void api.lockVault().then(() => onLock?.())}
        >
          {t("secrets.lock")}
        </button>
      </div>

      {/*
        **押せないものを見せない。** 錠前の無い端末——Linux、読み取り機の付いて
        いない機械——では、この節そのものが出ない。
      */}
      {status.biometric.available ? (
        <section aria-label={t("secrets.biometricHeading")} className={sectionCard}>
          <h3 className={sectionHeading}>{t("secrets.biometricHeading")}</h3>
          {/*
            何が起きるかを、有効にする前に言う。**秘密がこの端末の OS の錠前にも
            依存するようになる**ことは、押したあとに気づくことではない。
          */}
          <p className={hintText}>{t("secrets.biometricExplain")}</p>
          <label className="flex items-center gap-2 text-sm text-ink">
            <input
              type="checkbox"
              checked={status.biometric.enabled}
              onChange={(event) =>
                void run(
                  () => (event.target.checked ? api.enableBiometric() : api.disableBiometric()),
                  t("secrets.biometricFailed"),
                )
              }
            />
            {t("secrets.biometricEnable")}
          </label>
        </section>
      ) : null}

      {kinds.map((group) => {
        const draft = draftFor(group.kind);
        const mine = credentials.filter((credential) => credential.kind === group.kind);
        const dedicated = group.kind === "key_passphrase" ? dedicatedKeyPassphrases : [];
        return (
          <section key={group.kind} aria-label={t(group.heading)} className={sectionCard}>
            <h3 className={sectionHeading}>{t(group.heading)}</h3>
            {mine.length === 0 && dedicated.length === 0 ? (
              <p className={hintText}>{t("secrets.none")}</p>
            ) : (
              <ul className="grid gap-3 sm:grid-cols-2">
                {mine.map((credential) => (
                  <li key={credential.name}>
                    <article
                      aria-label={credential.name}
                      className="flex h-full flex-col gap-4 rounded-xl border border-line bg-card p-4"
                    >
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <h4 className="font-semibold text-ink">{credential.name}</h4>
                        <button
                          type="button"
                          className={dangerAction}
                          onClick={() =>
                            void run(
                              () => api.deleteCredential(group.kind, credential.name),
                              t("secrets.deleteFailed"),
                            )
                          }
                        >
                          {t("secrets.delete", { name: credential.name })}
                        </button>
                      </div>
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
                    </article>
                  </li>
                ))}
                {dedicated.map((credential) => (
                  <li key={credential.key}>
                    <article
                      aria-label={credential.key}
                      className="flex h-full flex-col gap-4 rounded-xl border border-line bg-card p-4"
                    >
                      <div className="flex flex-wrap items-start justify-between gap-3">
                        <div className="flex flex-col gap-1">
                          <h4 className="font-semibold text-ink">
                            {keyBasename(credential.key)}
                          </h4>
                          <p className={hintText}>{t("secrets.dedicated")}</p>
                        </div>
                        <button
                          type="button"
                          className={dangerAction}
                          onClick={() =>
                            void run(
                              () => api.unassignCredential("key_passphrase", credential.key),
                              t("secrets.deleteFailed"),
                            )
                          }
                        >
                          {t("secrets.removeDedicated", { key: credential.key })}
                        </button>
                      </div>
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
                    </article>
                  </li>
                ))}
              </ul>
            )}

            <div className="flex flex-wrap items-end gap-3">
              <Field label={t(group.nameLabel)}>
                <input
                  value={draft.name}
                  onChange={(event) => setDrafts({ ...drafts, [group.kind]: { ...draft, name: event.target.value } })}
                  className={control}
                />
              </Field>
              <Field label={t(group.valueLabel)}>
                <input
                  type="password"
                  value={draft.secret}
                  onChange={(event) => setDrafts({ ...drafts, [group.kind]: { ...draft, secret: event.target.value } })}
                  className={control}
                />
              </Field>
              <button
                type="button"
                className={primaryAction}
                disabled={draft.name === "" || draft.secret === ""}
                onClick={() =>
                  void run(
                    () => api.storeCredential(group.kind, draft.name, draft.secret),
                    t("secrets.storeFailed"),
                  ).then(() => setDrafts({ ...drafts, [group.kind]: { name: "", secret: "" } }))
                }
              >
                {t(group.store)}
              </button>
            </div>
          </section>
        );
      })}

    </div>
  );
}
