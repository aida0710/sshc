import { useCallback, useEffect, useState } from "react";
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
import { Button, Card, Notice, Row } from "../ui/surface";
import { MetricCard, MetricGrid, PageHeader } from "../ui/page";
import { SyncResultCard, type SyncResultView } from "./SyncResultCard";

type SyncPanelProps = { api?: IntegrationsApi };

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
  const [status, setStatus] = useState<SyncStatus | null>(null);
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
    try {
      setStatus(await api.syncStatus());
    } catch {
      setError(t("sync.statusFailed"));
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

  if (status === null) {
    return <p role="status" className={hintText}>{t("sync.loading")}</p>;
  }

  if (status.locked) {
    return (
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
        <PageHeader title={t("sync.heading")} description={t("sync.pageDescription")} />
        {error === "" ? null : <Notice tone="danger">{error}</Notice>}
        <section className="flex flex-col gap-3 rounded-xl border border-line bg-card p-5">
          <h3 className={sectionHeading}>{t("sync.bucketHeading")}</h3>
          <p className="text-sm text-ink-muted">{t("sync.sealed")}</p>
          <Field label={t("secrets.master")}>
            <input
              type="password"
              value={master}
              onChange={(event) => setMaster(event.target.value)}
              className={control}
            />
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
        </section>
      </div>
    );
  }

  const conflicted = (preview?.conflicts ?? []).length > 0;

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("sync.heading")} description={t("sync.pageDescription")} />
      <MetricGrid>
        <MetricCard
          label={t("sync.metricConfiguration")}
          value={t(status.configured ? "sync.stateConfigured" : "sync.stateNotConfigured")}
        />
        <MetricCard label={t("sync.metricDirection")} value={t(`sync.direction.${status.direction}`)} />
        <MetricCard
          label={t("sync.metricSnapshot")}
          value={status.synced ? status.fileCount ?? 0 : "—"}
          detail={status.synced ? status.lastSyncedAt ?? "" : t("sync.neverSynced")}
        />
      </MetricGrid>

      <p className={hintText}>{t("sync.warning")}</p>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {notice === "" ? null : <p role="status" className="text-sm text-ink-muted">{notice}</p>}

      <section className="flex flex-col gap-3 rounded-xl border border-line bg-card p-5">
        <h3 className={sectionHeading}>{t("sync.bucketHeading")}</h3>
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
          <p className="text-sm text-ink-muted">{t("sync.notConfigured")}</p>
        )}

        {!status.configured || editingSettings ? <>
        <Card>
          <Row label={t("sync.endpoint")} hint={t("sync.endpointHint")}>
            <input
              value={endpoint}
              onChange={(event) => setEndpoint(event.target.value)}
              placeholder="https://<account>.r2.cloudflarestorage.com"
              className={control}
            />
          </Row>
          <Row label={t("sync.bucket")}>
            <input value={bucket} onChange={(event) => setBucket(event.target.value)} className={control} />
          </Row>

          <Row label={t("sync.path")} hint={t("sync.pathHint")}>
            <input value={path} onChange={(event) => setPath(event.target.value)} className={control} />
          </Row>

          <Row label={t("sync.region")} hint={t("sync.regionHint")}>
            <input value={region} onChange={(event) => setRegion(event.target.value)} className={control} />
          </Row>
          <Row label={t("sync.accessKeyId")}>
            <input
              value={accessKeyId}
              onChange={(event) => setAccessKeyId(event.target.value)}
              className={control}
            />
          </Row>

          <Row label={t("sync.secretAccessKey")} hint={t("sync.credentialsNote")}>
            <input
              type="password"
              value={secretAccessKey}
              onChange={(event) => setSecretAccessKey(event.target.value)}
              className={control}
            />
          </Row>

          <Row label={t("sync.direction")} hint={t(`sync.direction.${direction}.hint`)}>
            <select
              value={direction}
              onChange={(event) => setDirection(event.target.value as SyncDirection)}
              className={control}
            >
              <option value="both">{t("sync.direction.both")}</option>
              <option value="push">{t("sync.direction.push")}</option>
              <option value="pull">{t("sync.direction.pull")}</option>
            </select>
          </Row>
        </Card>
        <Button
          kind="primary"
          disabled={busy || endpoint === "" || bucket === "" || accessKeyId === "" || secretAccessKey === ""}
          onClick={() =>
            void run(
              () => api.configureSync({ endpoint, bucket, path, region, accessKeyId, secretAccessKey, direction }),
              (next) => {
                setStatus(next);
                setAccessKeyId("");
                setSecretAccessKey("");
                setEditingSettings(false);
              },
              t("sync.configureFailed"),
            )
          } className="self-start"
        >
          {t("sync.configure")}
        </Button>
        {status.configured ? (
          <Button disabled={busy} onClick={() => setEditingSettings(false)}>
            {t("sync.cancelSettings")}
          </Button>
        ) : null}
        </> : null}
      </section>

      <section className="flex flex-col gap-3 rounded-xl border border-line bg-card p-5">
        <h3 className={sectionHeading}>{t("sync.snapshotHeading")}</h3>
        <p className={hintText}>
          {status.synced
            ? t("sync.lastSynced", { at: status.lastSyncedAt ?? "", count: status.fileCount ?? 0 })
            : t("sync.neverSynced")}
        </p>
        <Card>
          <Row label={t("sync.key")} hint={t("sync.keyHint")}>
            <div className="flex flex-col gap-2">
              {revealed === "" ? (
                <p className="text-sm text-ink-muted">
                  {status.keyConfigured ? t("sync.keySet") : t("sync.keyMissing")}
                </p>
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
          </Row>

          <Row label={t("sync.rekey")} hint={t("sync.rekeyHint")}>
            <div className="flex flex-col gap-2">
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
                      setStatus(next);
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
          </Row>
        </Card>

        <Card>
          <Row label={t("sync.auto")} hint={t("sync.autoHint")}>
            <div className="flex flex-col gap-2">
              <label className="flex items-center gap-2 text-sm text-ink">
                <input
                  type="checkbox"
                  checked={status.auto.enabled}
                  disabled={busy || !status.configured || !status.keyConfigured}
                  onChange={(event) =>
                    void run(
                      () => api.setAutoSync(event.target.checked),
                      (next) => setStatus(next),
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
                  void run(() => api.syncNow(), (next) => setStatus(next), t("sync.autoNowFailed"))
                }
                className="self-start rounded border border-line px-3 py-1.5 text-sm text-ink"
              >
                {t("sync.autoNow")}
              </button>
            </div>
          </Row>
        </Card>
        {status.direction === "both" ? null : (
          <p role="status" className="text-sm text-notice-ink">
            {t(`sync.direction.${status.direction}.active`)}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button
            kind="primary"
            disabled={busy || !status.configured || !status.keyConfigured || status.direction === "pull"}
            onClick={() =>
              void run(
                () => api.pushSnapshot(),
                (next) => {
                  setStatus(next.status);
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

      {resultView !== null ? (
        <SyncResultCard view={resultView} />
      ) : status.lastOperation === undefined ? null : (
        <SyncResultCard view={{ kind: "previous", operation: status.lastOperation }} />
      )}

      {preview === null ? null : (
        <section className="flex flex-col gap-3 rounded-xl border border-line bg-card p-5">
          <h3 className={sectionHeading}>{t("sync.previewHeading")}</h3>
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
        </section>
      )}
    </div>
  );
}
