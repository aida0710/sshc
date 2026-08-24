import { useCallback, useEffect, useState, type ReactNode } from "react";
import { failureCode } from "../api/client";
import {
  integrationsApi,
  type IntegrationsApi,
  type PullResponse,
  type SyncDirection,
  type SyncStatus,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import {
  Field,
  control,
  hintText,
  sectionHeading,
} from "../ui/form";
import { Button, Notice } from "../ui/surface";
import { PageHeader } from "../ui/page";
import { Icon } from "../ui/icons";
import { SyncResultCard, type SyncResultView } from "./SyncResultCard";

type SyncPanelProps = { api?: IntegrationsApi };

const mobileTouchTargets = "[&_button]:min-h-10 md:[&_button]:min-h-0";

function SyncRow({ label, children, hint }: { label: string; children: ReactNode; hint?: string }) {
  return (
    <div className="border-t border-hairline first:border-t-0">
      <label className="flex flex-col items-stretch gap-2 px-3 py-3 sm:flex-row sm:items-center sm:gap-3 sm:py-2">
        <span className="w-full shrink-0 text-sm text-ink-muted sm:w-32">{label}</span>
        <span className="flex min-w-0 flex-1 justify-start sm:ml-auto sm:justify-end">{children}</span>
      </label>
      {hint === undefined ? null : <p className={`px-3 pb-3 sm:pb-2 ${hintText}`}>{hint}</p>}
    </div>
  );
}

type SyncStatusState =
  | { phase: "loading" }
  | { phase: "error"; message: string }
  | { phase: "ready"; value: SyncStatus };

const refusals: Record<string, MessageKey> = {
  wrong_master_password: "sync.wrongMaster",
  wrong_passphrase: "sync.wrongKey",
  sync_key_missing: "sync.keyMissing",
  passphrase_too_short: "sync.keyTooShort",
  bucket_refused: "sync.unreachable",
  sync_failed: "sync.unreachable",
  endpoint_must_have_no_path: "sync.endpointPath",
  sync_remote_moved: "sync.remoteMoved",
};

export function SyncPanel({ api = integrationsApi }: SyncPanelProps) {
  const t = useTranslate();
  const [statusState, setStatusState] = useState<SyncStatusState>({ phase: "loading" });
  const [endpoint, setEndpoint] = useState("");
  const [bucket, setBucket] = useState("");
  const [path, setPath] = useState("");
  const [region, setRegion] = useState("");
  const [accessKeyId, setAccessKeyId] = useState("");
  const [secretAccessKey, setSecretAccessKey] = useState("");
  const [direction, setDirection] = useState<SyncDirection>("both");
  const [master, setMaster] = useState("");
  const [revealed, setRevealed] = useState("");
  const [ownKey, setOwnKey] = useState("");
  const [chooseOwn, setChooseOwn] = useState(false);
  const [oldKey, setOldKey] = useState("");
  const [acceptedRemovals, setAcceptedRemovals] = useState(false);
  const [resolve, setResolve] = useState<"local" | "remote" | undefined>(undefined);
  const [preview, setPreview] = useState<PullResponse | null>(null);
  const [resultView, setResultView] = useState<SyncResultView | null>(null);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [editingSettings, setEditingSettings] = useState(false);

  function editSettings(current: SyncStatus) {
    setEndpoint(current.endpoint ?? "");
    setBucket(current.bucket ?? "");
    setPath(current.path ?? "");
    setRegion(current.region ?? "");
    setDirection(current.direction);
    setAccessKeyId("");
    setSecretAccessKey("");
    setEditingSettings(true);
  }

  const reload = useCallback(async () => {
    setStatusState({ phase: "loading" });
    try {
      setStatusState({ phase: "ready", value: await api.syncStatus() });
    } catch {
      setStatusState({ phase: "error", message: t("sync.statusFailed") });
    }
  }, [api, t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  async function run<T>(
    operation: () => Promise<T>,
    apply: (value: T) => void,
    failure: string,
    explain?: (code: string) => string,
  ) {
    setError("");
    setNotice("");
    setBusy(true);
    try {
      apply(await operation());
    } catch (caught) {
      const code = failureCode(caught);
      const named = refusals[code];
      setError(explain?.(code) || (named === undefined ? failure : t(named)));
    } finally {
      setBusy(false);
    }
  }

  async function previewWith(choice?: "local" | "remote") {
    setResolve(choice);
    await run(
      () => api.pullSnapshot(false, choice),
      (next) => {
        setPreview(next);
        setAcceptedRemovals(false);
        setResultView({ kind: "preview", result: next });
        setNotice(
          next.written.length + next.removed.length + next.conflicts.length === 0
            ? t("sync.alreadyMatches")
            : "",
        );
      },
      t("sync.pullFailed"),
    );
  }

  if (statusState.phase === "loading") {
    return <p role="status" className={hintText}>{t("sync.loading")}</p>;
  }

  if (statusState.phase === "error") {
    return (
      <div className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}>
        <PageHeader title={t("sync.heading")} description={t("sync.pageDescription")} />
        <Notice tone="danger">{statusState.message}</Notice>
        <Button onClick={() => void reload()} className="self-start">
          {t("shell.bootstrapRetry")}
        </Button>
      </div>
    );
  }

  const status = statusState.value;

  if (status.locked) {
    return (
      <div className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}>
        <PageHeader title={t("sync.heading")} description={t("sync.pageDescription")} />
        {error === "" ? null : <Notice tone="danger">{error}</Notice>}
        <section className="sshc-card grid overflow-hidden rounded-xl bg-card md:grid-cols-[minmax(0,0.9fr)_minmax(18rem,1.1fr)]">
          <div className="flex flex-col justify-between gap-8 bg-toolbar p-6 md:p-8">
            <span className="flex h-12 w-12 items-center justify-center rounded-xl bg-select-fill text-accent">
              <Icon name="sync" className="h-6 w-6" />
            </span>
            <div>
              <h3 className="text-lg font-semibold text-ink">{t("sync.bucketHeading")}</h3>
              <p className="mt-2 text-sm leading-6 text-ink-muted">{t("sync.sealed")}</p>
            </div>
          </div>
          <div className="flex flex-col justify-center gap-4 p-6 md:p-8">
            <Field label={t("secrets.master")}>
              <input type="password" value={master} onChange={(event) => setMaster(event.target.value)} className={control} />
            </Field>
            <Button
              kind="primary"
              disabled={busy || master === ""}
              onClick={() =>
                void run(
                  () => api.unlockVault(master),
                  () => {
                    setMaster("");
                    void reload();
                  },
                  t("sync.unlockFailed"),
                  (code) => (code === "vault_missing" ? t("sync.noVault") : ""),
                )
              } className="self-start"
            >
              {t("secrets.unlock")}
            </Button>
          </div>
        </section>
      </div>
    );
  }

  const conflicted = (preview?.conflicts ?? []).length > 0;

  return (
    <div className={`mx-auto flex w-full max-w-5xl flex-col gap-6 ${mobileTouchTargets}`}>
      <PageHeader title={t("sync.heading")} description={t("sync.pageDescription")} />
      <dl className="sshc-card flex flex-wrap overflow-hidden rounded-xl bg-toolbar">
        {[
          [t("sync.metricConfiguration"), t(status.configured ? "sync.stateConfigured" : "sync.stateNotConfigured")],
          [t("sync.metricDirection"), t(`sync.direction.${status.direction}`)],
          [t("sync.metricSnapshot"), status.synced ? status.fileCount ?? 0 : "—"],
        ].map(([label, value]) => (
          <div key={String(label)} className="flex min-w-40 flex-1 items-center justify-between gap-4 border-r border-hairline px-4 py-2.5 last:border-r-0">
            <dt className="text-xs font-medium text-ink-muted">{label}</dt>
            <dd className="font-mono text-sm font-semibold text-ink">{value}</dd>
          </div>
        ))}
      </dl>

      <p className={hintText}>{t("sync.warning")}</p>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {notice === "" ? null : <p role="status" className="text-sm text-ink-muted">{notice}</p>}

      <section className="sshc-card overflow-hidden rounded-xl bg-card">
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
          <div className="flex items-center gap-2">
            <Icon name="remoteKeys" className="h-4 w-4 text-ink-muted" />
            <h3 className={sectionHeading}>{t("sync.bucketHeading")}</h3>
          </div>
        {status.configured ? (
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p className="font-mono text-xs text-ink-muted">
              {[status.endpoint, status.bucket, status.path].filter((part) => part !== "" && part !== undefined).join("/")}

              {status.region !== undefined && status.region !== "" ? ` (${status.region})` : ""}
            </p>
            {!editingSettings ? (
              <Button onClick={() => editSettings(status)}>
                {t("sync.editSettings")}
              </Button>
            ) : null}
          </div>
        ) : (
          <p className="text-xs text-ink-muted">{t("sync.notConfigured")}</p>
        )}
        </header>

        {!status.configured || editingSettings ? <>
        <div className="px-1 py-2 sm:px-3">
          <SyncRow label={t("sync.endpoint")} hint={t("sync.endpointHint")}>
            <input
              value={endpoint}
              onChange={(event) => setEndpoint(event.target.value)}
              placeholder="https://<account>.r2.cloudflarestorage.com"
              className={control}
            />
          </SyncRow>
          <SyncRow label={t("sync.bucket")}>
            <input value={bucket} onChange={(event) => setBucket(event.target.value)} className={control} />
          </SyncRow>

          <SyncRow label={t("sync.path")} hint={t("sync.pathHint")}>
            <input value={path} onChange={(event) => setPath(event.target.value)} className={control} />
          </SyncRow>

          <SyncRow label={t("sync.region")} hint={t("sync.regionHint")}>
            <input value={region} onChange={(event) => setRegion(event.target.value)} className={control} />
          </SyncRow>
          <SyncRow label={t("sync.accessKeyId")}>
            <input
              value={accessKeyId}
              onChange={(event) => setAccessKeyId(event.target.value)}
              className={control}
            />
          </SyncRow>

          <SyncRow label={t("sync.secretAccessKey")} hint={t("sync.credentialsNote")}>
            <input
              type="password"
              value={secretAccessKey}
              onChange={(event) => setSecretAccessKey(event.target.value)}
              className={control}
            />
          </SyncRow>

          <SyncRow label={t("sync.direction")} hint={t(`sync.direction.${direction}.hint`)}>
            <select
              value={direction}
              onChange={(event) => setDirection(event.target.value as SyncDirection)}
              className={control}
            >
              <option value="both">{t("sync.direction.both")}</option>
              <option value="push">{t("sync.direction.push")}</option>
              <option value="pull">{t("sync.direction.pull")}</option>
            </select>
          </SyncRow>
        </div>
        <div className="flex flex-wrap gap-2 border-t border-line bg-toolbar px-4 py-3">
          <Button
            kind="primary"
            disabled={busy || endpoint === "" || bucket === "" || accessKeyId === "" || secretAccessKey === ""}
            onClick={() =>
              void run(
                () => api.configureSync({ endpoint, bucket, path, region, accessKeyId, secretAccessKey, direction }),
                (next) => {
                  setStatusState({ phase: "ready", value: next });
                  setAccessKeyId("");
                  setSecretAccessKey("");
                  setEditingSettings(false);
                },
                t("sync.configureFailed"),
              )
            }
          >
            {t("sync.configure")}
          </Button>
          {status.configured ? (
            <Button disabled={busy} onClick={() => setEditingSettings(false)}>{t("sync.cancelSettings")}</Button>
          ) : null}
        </div>
        </> : null}
      </section>

      <section className="sshc-card overflow-hidden rounded-xl bg-card">
        <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
          <div className="flex items-center gap-2">
            <Icon name="sync" className="h-4 w-4 text-ink-muted" />
            <h3 className={sectionHeading}>{t("sync.snapshotHeading")}</h3>
          </div>
          <p className={hintText}>
            {status.synced
              ? t("sync.lastSynced", { at: status.lastSyncedAt ?? "", count: status.fileCount ?? 0 })
              : t("sync.neverSynced")}
          </p>
        </header>
        <div className="grid gap-4 p-4 lg:grid-cols-2">
          <section aria-labelledby="sync-key-heading" className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h4 id="sync-key-heading" className={sectionHeading}>{t("sync.key")}</h4>
              <span className={`rounded-full px-2 py-1 text-xs font-medium ${status.keyConfigured ? "bg-select-fill text-accent" : "bg-notice text-notice-ink"}`}>
                {status.keyConfigured ? t("sync.keyReady") : t("sync.keyNeeded")}
              </span>
            </div>
            <p className="text-sm leading-6 text-ink-muted">{t("sync.keyHint")}</p>
            <p className="text-sm font-medium text-ink">{t(status.keyConfigured ? "sync.keySet" : "sync.keyMissing")}</p>
            <div className="flex flex-col gap-3 border-t border-line pt-3">
              {revealed === "" ? (
                null
              ) : (
                <>
                  <output className="select-all break-all rounded border border-line bg-surface px-3 py-2 font-mono text-sm text-ink">
                    {revealed}
                  </output>
                  <p className="text-sm text-notice-ink">{t("sync.keyShownOnce")}</p>
                </>
              )}
              <label className="flex items-center gap-2 text-sm text-ink-muted">
                <input
                  type="checkbox"
                  checked={chooseOwn}
                  onChange={(event) => setChooseOwn(event.target.checked)}
                />
                {t("sync.keyChooseOwn")}
              </label>
              {chooseOwn ? (
                <input
                  type="password"
                  aria-label={t("sync.keyOwnValue")}
                  value={ownKey}
                  onChange={(event) => setOwnKey(event.target.value)}
                  className={control}
                />
              ) : null}
              <Button
                kind="primary"
                disabled={busy || (chooseOwn && ownKey === "")}
                onClick={() =>
                  void run(
                    () => api.setSyncKey(chooseOwn ? ownKey : undefined),
                    (next) => {
                      setRevealed(chooseOwn ? "" : next.key);
                      setOwnKey("");
                      setNotice(t("sync.keySaved"));
                      void reload();
                    },
                    t("sync.keyFailed"),
                  )
                } className="self-start"
              >
                {status.keyConfigured ? t("sync.keyReplace") : t("sync.keyCreate")}
              </Button>
            </div>
          </section>

          <details className="rounded-lg border border-line bg-surface-subtle p-4">
            <summary className="cursor-pointer text-sm font-medium text-ink">{t("sync.rekeyHeading")}</summary>
            <div className="mt-3 flex flex-col gap-3 border-t border-line pt-3">
              <p className="text-sm leading-6 text-ink-muted">{t("sync.rekeyHint")}</p>
              <input
                type="password"
                aria-label={t("sync.rekeyOldKey")}
                value={oldKey}
                onChange={(event) => setOldKey(event.target.value)}
                className={control}
              />
              <button
                type="button"
                disabled={busy || !status.configured || !status.keyConfigured || oldKey === ""}
                onClick={() =>
                  void run(
                    () => api.rekeySnapshot(oldKey),
                    (next) => {
                      setStatusState({ phase: "ready", value: next });
                      setOldKey("");
                      setNotice(t("sync.rekeyed"));
                    },
                    t("sync.rekeyFailed"),
                  )
                }
                className="self-start rounded border border-line px-3 py-1.5 text-sm text-ink"
              >
                {t("sync.rekey")}
              </button>
            </div>
          </details>

          <section aria-labelledby="sync-auto-heading" className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4">
            <h4 id="sync-auto-heading" className={sectionHeading}>{t("sync.auto")}</h4>
            <p className="text-sm leading-6 text-ink-muted">{t("sync.autoHint")}</p>
            <div className="flex flex-col gap-3 border-t border-line pt-3">
              <label className="flex items-center gap-2 text-sm text-ink">
                <input
                  type="checkbox"
                  checked={status.auto.enabled}
                  disabled={busy || !status.configured || !status.keyConfigured}
                  onChange={(event) =>
                    void run(
                      () => api.setAutoSync(event.target.checked),
                      (next) => setStatusState({ phase: "ready", value: next }),
                      t("sync.autoFailed"),
                    )
                  }
                />
                {t("sync.autoEnable")}
              </label>
              <p role="status" className={hintText}>
                {status.auto.phase === "blocked"
                  ? t(status.auto.detail === "conflicts" ? "sync.autoBlockedConflicts" : "sync.autoBlockedRemovals")
                  : status.auto.phase === "failed"
                    ? t("sync.autoFailedLast")
                    : status.auto.at === undefined || !status.auto.enabled
                      ? t("sync.autoIdle")
                      : t("sync.autoLastRan", { at: status.auto.at })}
              </p>
              <button
                type="button"
                disabled={busy || !status.auto.enabled}
                onClick={() =>
                  void run(
                    () => api.syncNow(),
                    (next) => setStatusState({ phase: "ready", value: next }),
                    t("sync.autoNowFailed"),
                  )
                }
                className="self-start rounded border border-line px-3 py-1.5 text-sm text-ink"
              >
                {t("sync.autoNow")}
              </button>
            </div>
          </section>

          <section aria-labelledby="sync-transfer-heading" className="flex flex-col gap-3 rounded-lg border border-line bg-surface-subtle p-4">
            <h4 id="sync-transfer-heading" className={sectionHeading}>{t("sync.transferHeading")}</h4>
            <p className="text-sm leading-6 text-ink-muted">{t("sync.transferHint")}</p>
            <div className="flex flex-wrap gap-2 border-t border-line pt-3">
              <Button
                kind="primary"
                disabled={busy || !status.configured || !status.keyConfigured || status.direction === "pull"}
                onClick={() =>
                  void run(
                    () => api.pushSnapshot(),
                    (next) => {
                      setStatusState({ phase: "ready", value: next.status });
                      setPreview(null);
                      setResultView({ kind: "push", result: next.result });
                      setNotice(t("sync.pushed"));
                    },
                    t("sync.pushFailed"),
                  )
                }
              >
                {t("sync.push")}
              </Button>
              <Button
                disabled={busy || !status.configured || !status.keyConfigured}
                onClick={() => void previewWith(undefined)}
              >
                {t("sync.preview")}
              </Button>
            </div>
          </section>
        </div>
        {status.direction === "both" ? null : (
          <p role="status" className="border-t border-notice-line bg-notice px-4 py-3 text-sm text-notice-ink">
            {t(`sync.direction.${status.direction}.active`)}
          </p>
        )}
      </section>

      {resultView !== null ? (
        <SyncResultCard view={resultView} />
      ) : status.lastOperation === undefined ? null : (
        <SyncResultCard view={{ kind: "previous", operation: status.lastOperation }} />
      )}

      {preview === null ? null : (
        <section className="sshc-card overflow-hidden rounded-xl bg-card">
          <header className="flex items-center gap-2 border-b border-line bg-toolbar px-4 py-3">
            <Icon name="config" className="h-4 w-4 text-ink-muted" />
            <h3 className={sectionHeading}>{t("sync.previewHeading")}</h3>
          </header>
          <div className="flex flex-col gap-3 px-4 py-4">
          {conflicted ? (
            <>

              <p className="text-sm text-notice-ink">{t("sync.conflictExplain")}</p>
              <ul className="flex flex-col gap-1 font-mono text-xs text-notice-ink">
                {preview.conflicts.map((conflict) => (
                  <li key={conflict.path}>{conflict.path}</li>
                ))}
              </ul>

              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void previewWith("local")}
                  className="rounded border border-line px-3 py-1.5 text-sm text-ink"
                >
                  {t("sync.keepMine")}
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => void previewWith("remote")}
                  className="rounded border border-line px-3 py-1.5 text-sm text-ink"
                >
                  {t("sync.takeTheirs")}
                </button>
              </div>
            </>
          ) : null}
          {preview.written.length === 0 ? null : (
            <>
              <p className={hintText}>{t("sync.wouldWrite", { count: preview.written.length })}</p>
              <ul className="flex flex-col gap-1 font-mono text-xs text-ink-muted">
                {preview.written.map((path) => (
                  <li key={path}>{path}</li>
                ))}
              </ul>
            </>
          )}
          {preview.removed.length === 0 ? null : (
            <>
              <p className={hintText}>{t("sync.wouldRemove", { count: preview.removed.length })}</p>
              <ul className="flex flex-col gap-1 font-mono text-xs text-danger">
                {preview.removed.map((path) => (
                  <li key={path}>{path}</li>
                ))}
              </ul>
            </>
          )}
          {preview.removed.length === 0 ? null : (
            <label className="flex items-start gap-2 rounded border border-notice-line bg-notice p-3 text-sm text-notice-ink">
              <input
                type="checkbox"
                checked={acceptedRemovals}
                onChange={(event) => setAcceptedRemovals(event.target.checked)}
                className="mt-0.5"
              />
              <span>{t("sync.confirmOverwrite")}</span>
            </label>
          )}
          <Button
            kind="primary"
            disabled={
              busy ||
              conflicted ||
              status.direction === "push" ||
              (preview.removed.length > 0 && !acceptedRemovals) ||
              preview.written.length + preview.removed.length === 0
            }
            onClick={() =>
              void run(
                () => api.pullSnapshot(true, resolve),
                (next) => {
                  setPreview(next);
                  setResultView({ kind: "apply", result: next });
                  setNotice(t("sync.applied"));
                  void reload();
                },
                t("sync.applyFailed"),
              )
            } className="self-start"
          >
            {t("sync.apply")}
          </Button>
          </div>
        </section>
      )}
    </div>
  );
}
