import { DisclosureChevron } from "../ui/DisclosureChevron";
import type { SyncDirection, SyncStatus } from "../api/sync";
import { useTranslate } from "../i18n/context";
import { CheckboxField, control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import { PageSection } from "../ui/page";
import { PasswordInput } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";
import { SyncKeySection } from "./SyncKeySection";
import { syncDirectionHintKeys, syncSetupStateKeys } from "./syncMessageKeys";
import type { SyncSetupForm } from "./useSyncSetupForm";

// Bucket settings and the shared key. Until sync is configured this is the
// whole screen; afterwards it folds away behind "manage settings". Only then is it a <details>: one
// without a summary gets the browser's own "Details" toggle, which would hide the whole setup.
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
  const body = (
      <div className={status.configured ? "flex flex-col gap-5 pt-4" : "flex flex-col gap-[22px]"}>
        <p className="rounded-[9px] bg-well px-3 py-2.5 text-xs leading-[18px] text-ink-muted">
          {t("sync.warning")}
        </p>
        <PageSection
          heading={t("sync.bucketHeading")}
          actions={status.configured ? (
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
              <span className="rounded-full bg-well px-2.5 py-0.5 text-[11.5px] text-ink-muted">
                {t("sync.notConfigured")}
              </span>
            )}
        >

          {!status.configured || editingSettings ? (
            <>
              <div>
                <FieldRow
                  label={t("sync.endpoint")}
                  hint={t("sync.endpointHint")}
                >
                  <input
                    value={endpoint}
                    onChange={(event) => setEndpoint(event.target.value)}
                    placeholder="https://<account>.r2.cloudflarestorage.com"
                    className={control}
                  />
                </FieldRow>
                <FieldRow label={t("sync.bucket")}>
                  <input
                    value={bucket}
                    onChange={(event) => setBucket(event.target.value)}
                    className={control}
                  />
                </FieldRow>

                <FieldRow label={t("sync.path")} hint={t("sync.pathHint")}>
                  <input
                    value={path}
                    onChange={(event) => setPath(event.target.value)}
                    className={control}
                  />
                </FieldRow>

                <FieldRow label={t("sync.region")} hint={t("sync.regionHint")}>
                  <input
                    value={region}
                    onChange={(event) => setRegion(event.target.value)}
                    className={control}
                  />
                </FieldRow>
                <FieldRow label={t("sync.accessKeyId")}>
                  <input
                    value={accessKeyId}
                    onChange={(event) => setAccessKeyId(event.target.value)}
                    className={control}
                  />
                </FieldRow>

                <FieldRow
                  label={t("sync.secretAccessKey")}
                  hint={t("sync.credentialsNote")}
                  interactiveChildren
                >
                  <PasswordInput
                    label={t("sync.secretAccessKey")}
                    value={secretAccessKey}
                    onChange={setSecretAccessKey}
                  />
                </FieldRow>

                <FieldRow
                  label={t("sync.direction")}
                  hint={t(syncDirectionHintKeys[direction])}
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
                </FieldRow>
              </div>
              <div className="mt-1 flex flex-col gap-3 border-t border-line pt-4">
                {setupCheck === null ? null : (
                  <Notice
                    tone={
                      setupCheck.state === "incomplete" ? "danger" : "notice"
                    }
                  >
                    {t(syncSetupStateKeys[setupCheck.state])}
                    {setupCheck.state === "incomplete"
                      ? ` ${t("sync.setup.useAnotherPath")}`
                      : ""}
                  </Notice>
                )}
                {setupCheck === null ||
                setupCheck.state === "incomplete" ? null : (
                  <div className="flex flex-col gap-3 rounded-[10px] bg-well p-3">
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
                <div className="flex flex-wrap justify-end gap-2">
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
        </PageSection>

        {status.configured ? (
          <SyncKeySection status={status} busy={busy} form={form} onSave={onSaveKey} />
        ) : null}
      </div>
  );
  if (!status.configured) return body;
  return (
    <details
      open={settingsOpen}
      onToggle={(event) => setSettingsOpen(event.currentTarget.open)}
      className="border-t border-line pt-4"
    >
      <summary className="flex cursor-pointer list-none items-center gap-1.5 text-[13px] text-ink-muted marker:hidden hover:text-ink">
        <DisclosureChevron className="size-3.5" />
        {t("sync.manageSettings")}
      </summary>
      {body}
    </details>
  );
}
