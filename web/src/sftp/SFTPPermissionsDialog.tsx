import { useId, useRef, useState, type RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { ModalShell } from "../ui/ModalShell";
import { control, hintText } from "../ui/form";
import { Button } from "../ui/surface";
import { symbolicModeToOctal } from "./transfers";
import type { SFTPPermissionsModel } from "./useSFTPPermissions";
import type { PermissionsIntent } from "./permissionsIntent";

// Common defaults give files read access and folders the traversal permission.
const defaultFileMode = "644";
const defaultDirectoryMode = "755";
// Keep the selection within the same limit as the permission plan API.
const maxChmodSelection = 200;

export function SFTPPermissionsDialog({ permissions, returnFocusRef }: {
  permissions: SFTPPermissionsModel;
  returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const id = useId();
  const { intent, confirmation } = permissions;
  if (intent === null) return null;
  if (confirmation === null) return <PermissionsForm intent={intent} permissions={permissions} returnFocusRef={returnFocusRef} />;
  const { plan } = confirmation;
  return <ConfirmDialog
    id={id}
    heading={t("sftp.chmodConfirm")}
    body={<div className="space-y-2 text-sm text-ink-muted">
      <p>{t("sftp.chmodSummary", { selected: plan.selectionCount, files: plan.files, directories: plan.directories })}</p>
      <p>{t("sftp.chmodModes", { files: plan.options.fileMode, directories: plan.options.directoryMode })}</p>
      <p>{t(plan.options.recursive ? "sftp.chmodRecursiveEnabled" : "sftp.chmodRecursiveDisabled")}</p>
      <p>{t("sftp.chmodSkipLinks", { count: plan.skippedSymlinks })}</p>
      <p>{t("sftp.chmodPartialWarning")}</p>
      <p className="break-all font-mono">{intent.alias}</p>
      <ul className="max-h-40 overflow-auto font-mono">
        {intent.entries.map((entry) => <li key={entry.path} className="break-all">{entry.path}</li>)}
      </ul>
    </div>}
    confirmKind="primary"
    confirmLabel={t("sftp.apply")}
    cancelLabel={t(permissions.attempted ? "sftp.close" : "sftp.cancel")}
    onConfirm={permissions.apply}
    onCancel={permissions.cancel}
    busy={permissions.applying}
    confirmDisabled={permissions.attempted}
    error={permissions.problem}
    returnFocusRef={returnFocusRef}
  />;
}

function PermissionsForm({ intent, permissions, returnFocusRef }: {
  intent: PermissionsIntent;
  permissions: SFTPPermissionsModel;
  returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const id = useId();
  const inputRef = useRef<HTMLInputElement>(null);
  const single = intent.entries.length === 1;
  const first = intent.entries[0]!;
  const [fileMode, setFileMode] = useState(single ? intent.initialMode ?? symbolicModeToOctal(first.mode) : defaultFileMode);
  const [directoryMode, setDirectoryMode] = useState(defaultDirectoryMode);
  const [recursive, setRecursive] = useState(intent.recursive);
  const [validation, setValidation] = useState("");
  const hasDirectories = intent.entries.some((entry) => entry.type === "directory");

  function submit() {
    if (intent.entries.length > maxChmodSelection) {
      setValidation(t("sftp.chmodSelectionLimit", { count: maxChmodSelection }));
      return;
    }
    const normalizedFileMode = fileMode.trim();
    const normalizedDirectoryMode = single ? normalizedFileMode : directoryMode.trim();
    if (!/^0?[0-7]{3}$/.test(normalizedFileMode) || !/^0?[0-7]{3}$/.test(normalizedDirectoryMode)) {
      setValidation(t("sftp.chmodInvalid"));
      return;
    }
    setValidation("");
    void permissions.review({ fileMode: normalizedFileMode, directoryMode: normalizedDirectoryMode, recursive });
  }

  return <ModalShell labelledBy={id} onDismiss={permissions.cancel} initialFocusRef={inputRef} returnFocusRef={returnFocusRef} panelClassName="w-full max-w-md rounded-lg p-5">
    <h2 id={id} className="text-base font-semibold">{t(intent.recursive ? "sftp.chmodRecursive" : single ? "sftp.chmod" : "sftp.chmodBatch")}</h2>
    <p className={`mt-2 ${hintText}`}>{t("sftp.chmodSelectionCount", { count: intent.entries.length })}</p>
    <form className="mt-4 space-y-3" onSubmit={(event) => { event.preventDefault(); submit(); }}>
      <label className="block text-sm">{t(single ? "sftp.chmodPrompt" : "sftp.chmodFileMode")}
        <input ref={inputRef} inputMode="numeric" className={control} value={fileMode} disabled={permissions.planning} onChange={(event) => setFileMode(event.target.value)} />
      </label>
      {single ? null : <label className="block text-sm">{t("sftp.chmodDirectoryMode")}
        <input inputMode="numeric" className={control} value={directoryMode} disabled={permissions.planning} onChange={(event) => setDirectoryMode(event.target.value)} />
      </label>}
      {hasDirectories ? <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={recursive} disabled={permissions.planning} onChange={(event) => setRecursive(event.target.checked)} />
        {t("sftp.chmodRecurseOption")}
      </label> : null}
      {validation || permissions.problem ? <p role="alert" className="text-sm text-danger">{validation || permissions.problem}</p> : null}
      <div className="flex justify-end gap-2">
        <Button onClick={permissions.cancel}>{t("sftp.cancel")}</Button>
        <Button kind="primary" type="submit" disabled={permissions.planning}>{t(permissions.planning ? "sftp.chmodPlanning" : "sftp.chmodReview")}</Button>
      </div>
    </form>
  </ModalShell>;
}
