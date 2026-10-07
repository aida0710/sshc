import { useId, useRef, useState, type RefObject } from "react";
import { useTranslate } from "../i18n/context";
import { ModalShell } from "../ui/ModalShell";
import { control, hintText } from "../ui/form";
import { Button } from "../ui/surface";
import type { MetadataIntent, SFTPMetadataActionsModel } from "./useSFTPMetadataActions";

// SFTP v3 encodes numeric owner IDs as unsigned 32-bit integers.
const maxSFTPOwnerId = 4294967295;
// Keep link names within the usual server filename limit.
const maxSymlinkNameCharacters = 255;
// Match the link target maximum declared in OpenAPI.
const maxSymlinkTargetCharacters = 4096;

function validOwnerId(value: string): boolean {
  return /^[0-9]+$/.test(value) && Number(value) <= maxSFTPOwnerId;
}

export function SFTPMetadataActionDialog({ actions, returnFocusRef }: {
  actions: SFTPMetadataActionsModel; returnFocusRef: RefObject<HTMLElement | null>;
}) {
  return actions.intent === null ? null : <MetadataForm intent={actions.intent} actions={actions} returnFocusRef={returnFocusRef} />;
}

function MetadataForm({ intent, actions, returnFocusRef }: {
  intent: MetadataIntent; actions: SFTPMetadataActionsModel; returnFocusRef: RefObject<HTMLElement | null>;
}) {
  const t = useTranslate();
  const id = useId();
  const initialFocus = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [target, setTarget] = useState(intent.kind === "changeSymlink" ? intent.entry.linkTarget ?? "" : "");
  const [uid, setUID] = useState(intent.kind === "ownership" ? String(intent.entry.uid ?? "") : "");
  const [gid, setGID] = useState(intent.kind === "ownership" ? String(intent.entry.gid ?? "") : "");
  const [validation, setValidation] = useState("");
  const heading = t(intent.kind === "createSymlink" ? "sftp.newSymlink" : intent.kind === "changeSymlink" ? "sftp.changeLinkTarget" : "sftp.changeOwnership");
  const path = intent.kind === "createSymlink" ? actions.directory : intent.entry.path;

  function submit() {
    let problem = "";
    if (intent.kind === "ownership") {
      if (!validOwnerId(uid) || !validOwnerId(gid)) problem = t("sftp.ownerIDInvalid");
    } else if (target === "" || target.includes("\0")) {
      problem = t("sftp.linkTargetRequired");
    } else if (intent.kind === "createSymlink" && (name === "" || name === "." || name === ".." || /[/\0]/.test(name))) {
      problem = t("sftp.nameInvalid");
    }
    setValidation(problem);
    if (problem === "") void actions.submit({ name, target, uid: Number(uid), gid: Number(gid) });
  }

  return <ModalShell labelledBy={id} onDismiss={actions.cancel} dismissible={!actions.acting} initialFocusRef={initialFocus} returnFocusRef={returnFocusRef} panelClassName="w-full max-w-md rounded-lg p-5">
    <h2 id={id} className="text-base font-semibold">{heading}</h2>
    <p className="mt-2 break-all font-mono text-xs text-ink-muted">{path}</p>
    <p className={`mt-2 ${hintText}`}>{t(intent.kind === "ownership" ? "sftp.ownershipHint" : "sftp.symlinkHint")}</p>
    {intent.kind === "ownership" ? <p className={`mt-2 ${hintText}`}>{t("sftp.currentOwnership", { uid: intent.entry.uid ?? "—", gid: intent.entry.gid ?? "—" })}</p> : null}
    <form className="mt-4 space-y-3" onSubmit={(event) => { event.preventDefault(); submit(); }}>
      {intent.kind === "createSymlink" ? <label className="block text-sm">{t("sftp.name")}<input ref={initialFocus} className={control} value={name} maxLength={maxSymlinkNameCharacters} disabled={actions.acting} onChange={(event) => setName(event.target.value)} /></label> : null}
      {intent.kind === "ownership" ? <>
        <label className="block text-sm">{t("sftp.uid")}<input ref={initialFocus} className={control} inputMode="numeric" value={uid} disabled={actions.acting} onChange={(event) => setUID(event.target.value)} /></label>
        <label className="block text-sm">{t("sftp.gid")}<input className={control} inputMode="numeric" value={gid} disabled={actions.acting} onChange={(event) => setGID(event.target.value)} /></label>
      </> : <label className="block text-sm">{t("sftp.linkTarget")}<input ref={intent.kind === "changeSymlink" ? initialFocus : undefined} className={control} value={target} maxLength={maxSymlinkTargetCharacters} disabled={actions.acting} onChange={(event) => setTarget(event.target.value)} /></label>}
      {validation || actions.problem ? <p role="alert" className="text-sm text-danger">{validation || actions.problem}</p> : null}
      <div className="flex justify-end gap-2">
        <Button disabled={actions.acting} onClick={actions.cancel}>{t("sftp.cancel")}</Button>
        <Button kind="primary" type="submit" disabled={actions.acting}>{t(intent.kind === "createSymlink" ? "sftp.create" : "sftp.apply")}</Button>
      </div>
    </form>
  </ModalShell>;
}
