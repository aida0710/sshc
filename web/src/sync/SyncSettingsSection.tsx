import type { ReactNode } from "react";
import type { SyncDirection, SyncStatus } from "../api/sync";
import { useTranslate } from "../i18n/context";
import { CheckboxField, control, hintText, sectionHeading } from "../ui/form";
import { Icon } from "../ui/icons";
import { PasswordInput } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { SyncKeySection } from "./SyncKeySection";
import type { SyncSetupForm } from "./useSyncSetupForm";

function SyncRow({
  label,
  children,
  hint,
  interactiveChildren = false,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
  interactiveChildren?: boolean;
}) {
  const contents = (
    <>
      <span className="w-full shrink-0 text-sm text-ink-muted sm:w-32">
        {label}
      </span>
      <span className="flex min-w-0 flex-1 justify-start sm:ml-auto sm:justify-end">
        {children}
      </span>
    </>
  );
  return (
    <div className="border-t border-hairline first:border-t-0">
      {interactiveChildren ? (
        <div className="flex flex-col items-stretch gap-2 px-3 py-3 sm:flex-row sm:items-center sm:gap-3 sm:py-2">
          {contents}
        </div>
      ) : (
        <label className="flex flex-col items-stretch gap-2 px-3 py-3 sm:flex-row sm:items-center sm:gap-3 sm:py-2">
          {contents}
        </label>
      )}
      {hint === undefined ? null : (
        <p className={`px-3 pb-3 sm:pb-2 ${hintText}`}>{hint}</p>
      )}
    </div>
  );
}

// Bucket settings and the shared key. Until sync is configured this is the
// whole screen; afterwards it folds away behind "manage settings".
export function SyncSettingsSection({ status, busy, form, onCheckSetup, onCompleteSetup, onSaveKey }: {
  status: SyncStatus;
  busy: boolean;
  form: SyncSetupForm;
  // Asks the engine what the bucket path holds before anything is saved.
  onCheckSetup: () => void;
  onCompleteSetup: () => void;
  onSaveKey: () => void;
}) {
  const t = useTranslate();
  const {
    endpoint,
    bucket,
    path,
    region,
    accessKeyId,
    secretAccessKey,
    direction,
    setupCheck,
    ownKey,
    chooseOwn,
    editingSettings,
    settingsOpen,
    editSettings,
    setEndpoint,
    setBucket,
    setPath,
    setRegion,
    setAccessKeyId,
    setSecretAccessKey,
    setDirection,
    setOwnKey,
    setChooseOwn,
    setEditingSettings,
    setSettingsOpen,
  } = form;
  return (
    <details
      open={!status.configured || settingsOpen}
      onToggle={(event) => {
        if (status.configured) setSettingsOpen(event.currentTarget.open);
      }}
      className={
        status.configured
          ? "group overflow-hidden rounded-md border border-control-line bg-card"
          : "group"
      }
    >
      {status.configured ? (
        <summary className="flex cursor-pointer list-none items-center gap-3 bg-toolbar px-4 py-3 text-sm font-medium text-ink marker:hidden hover:bg-select-fill">
          <span
            aria-hidden="true"
            className="inline-flex size-5 shrink-0 items-center justify-center text-base text-ink-muted transition-transform group-open:rotate-90"
          >
            ›
          </span>
          <span>{t("sync.manageSettings")}</span>
        </summary>
      ) : null}
      <div
        className={
          status.configured
            ? "flex flex-col gap-4 border-t border-line bg-surface-subtle p-4"
            : "flex flex-col gap-6"
        }
      >
        <p className={`rounded-md bg-toolbar px-4 py-3 ${hintText}`}>
          {t("sync.warning")}
        </p>
        <section className="overflow-hidden rounded-md border border-line bg-card">
          <header className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-toolbar px-4 py-3">
            <div className="flex items-center gap-2">
              <Icon name="remoteKeys" className="h-4 w-4 text-ink-muted" />
              <h3 className={sectionHeading}>{t("sync.bucketHeading")}</h3>
            </div>
            {status.configured ? (
              <div className="flex flex-wrap items-center justify-between gap-3">
                <p className="font-mono text-xs text-ink-muted">
                  {[status.endpoint, status.bucket, status.path]
                    .filter((part) => part !== "" && part !== undefined)
                    .join("/")}

                  {status.region !== undefined && status.region !== ""
                    ? ` (${status.region})`
                    : ""}
                </p>
                {!editingSettings ? (
                  <Button onClick={() => editSettings(status)}>
                    {t("sync.editSettings")}
                  </Button>
                ) : null}
              </div>
            ) : (
              <p className="text-xs text-ink-muted">
                {t("sync.notConfigured")}
              </p>
            )}
          </header>

          {!status.configured || editingSettings ? (
            <>
              <div className="px-1 py-2 sm:px-3">
                <SyncRow
                  label={t("sync.endpoint")}
                  hint={t("sync.endpointHint")}
                >
                  <input
                    value={endpoint}
                    onChange={(event) => setEndpoint(event.target.value)}
                    placeholder="https://<account>.r2.cloudflarestorage.com"
                    className={control}
                  />
                </SyncRow>
                <SyncRow label={t("sync.bucket")}>
                  <input
                    value={bucket}
                    onChange={(event) => setBucket(event.target.value)}
                    className={control}
                  />
                </SyncRow>

                <SyncRow label={t("sync.path")} hint={t("sync.pathHint")}>
                  <input
                    value={path}
                    onChange={(event) => setPath(event.target.value)}
                    className={control}
                  />
                </SyncRow>

                <SyncRow label={t("sync.region")} hint={t("sync.regionHint")}>
                  <input
                    value={region}
                    onChange={(event) => setRegion(event.target.value)}
                    className={control}
                  />
                </SyncRow>
                <SyncRow label={t("sync.accessKeyId")}>
                  <input
                    value={accessKeyId}
                    onChange={(event) => setAccessKeyId(event.target.value)}
                    className={control}
                  />
                </SyncRow>

                <SyncRow
                  label={t("sync.secretAccessKey")}
                  hint={t("sync.credentialsNote")}
                  interactiveChildren
                >
                  <PasswordInput
                    label={t("sync.secretAccessKey")}
                    value={secretAccessKey}
                    onChange={setSecretAccessKey}
                  />
                </SyncRow>

                <SyncRow
                  label={t("sync.direction")}
                  hint={t(`sync.direction.${direction}.hint`)}
                >
                  <select
                    value={direction}
                    onChange={(event) =>
                      setDirection(event.target.value as SyncDirection)
                    }
                    className={control}
                  >
                    <option value="both">{t("sync.role.main")}</option>
                    <option value="pull">{t("sync.role.receive")}</option>
                    <optgroup label={t("sync.role.advanced")}>
                      <option value="push">{t("sync.role.send")}</option>
                    </optgroup>
                  </select>
                </SyncRow>
              </div>
              <div className="flex flex-col gap-3 border-t border-line bg-toolbar px-4 py-3">
                {setupCheck === null ? null : (
                  <Notice
                    tone={
                      setupCheck.state === "incomplete" ? "danger" : "notice"
                    }
                  >
                    {t(`sync.setup.${setupCheck.state}`)}
                    {setupCheck.state === "incomplete"
                      ? ` ${t("sync.setup.useAnotherPath")}`
                      : ""}
                  </Notice>
                )}
                {setupCheck === null ||
                setupCheck.state === "incomplete" ? null : (
                  <div className="flex flex-col gap-3 rounded-lg border border-line bg-surface p-3">
                    <p className="text-sm text-ink">
                      {setupCheck.state === "existing"
                        ? t("sync.setup.existingKey")
                        : t("sync.setup.emptyKey")}
                    </p>
                    {setupCheck.state === "empty" ? (
                      <CheckboxField label={t("sync.keyChooseOwn")} checked={chooseOwn} onChange={setChooseOwn} />
                    ) : null}
                    {setupCheck.state === "existing" || chooseOwn ? (
                      <PasswordInput
                        label={t("sync.keyOwnValue")}
                        value={ownKey}
                        onChange={setOwnKey}
                      />
                    ) : null}
                    <Button
                      kind="primary"
                      disabled={
                        busy ||
                        (setupCheck.state === "existing" && ownKey === "") ||
                        (chooseOwn && ownKey === "")
                      }
                      onClick={onCompleteSetup}
                    >
                      {t("sync.setup.save")}
                    </Button>
                  </div>
                )}
                <div className="flex flex-wrap gap-2">
                  <Button
                    disabled={
                      busy ||
                      endpoint === "" ||
                      bucket === "" ||
                      accessKeyId === "" ||
                      secretAccessKey === ""
                    }
                    onClick={onCheckSetup}
                  >
                    {t("sync.setup.check")}
                  </Button>
                  {status.configured ? (
                    <Button
                      disabled={busy}
                      onClick={() => setEditingSettings(false)}
                    >
                      {t("sync.cancelSettings")}
                    </Button>
                  ) : null}
                </div>
              </div>
            </>
          ) : null}
        </section>

        {status.configured ? (
          <SyncKeySection status={status} busy={busy} form={form} onSave={onSaveKey} />
        ) : null}
      </div>
    </details>
  );
}
