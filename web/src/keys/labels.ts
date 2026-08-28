import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";


export const rowAction =
  "rounded-md border border-transparent px-2 py-1.5 text-xs text-ink-muted hover:border-control-line hover:bg-select-fill hover:text-ink disabled:text-ink-faint";
export const rowPrimary =
  "rounded-md bg-accent px-2.5 py-1.5 text-xs font-medium text-accent-ink hover:brightness-110 disabled:bg-line disabled:text-ink-faint";
export const rowDanger = "rounded border border-control-line px-2 py-1 text-xs text-danger hover:bg-select-fill";

export const noteLabels: Record<string, MessageKey> = {
  fingerprint_unavailable: "keys.noteFingerprintUnavailable",
  symbolic_link: "keys.noteSymbolicLink",
  empty_file: "keys.noteEmptyFile",
  not_regular_file: "keys.noteNotRegularFile",
  comment_not_preserved: "keys.noteCommentNotPreserved",
};

const keyKindLabels: Record<string, MessageKey> = {
  private_key: "keys.kind.privateKey",
  public_key: "keys.kind.publicKey",
  certificate: "keys.kind.certificate",
  other: "keys.kind.other",
  unreadable: "keys.kind.unreadable",
};

export function describeKeyKind(kind: string, t: Translate): string {
  return t(keyKindLabels[kind] ?? "keys.kind.other");
}

const blockerLabels: Record<string, MessageKey> = {
  key_destination_occupied: "keys.blockerTargetOccupied",
  key_reference_unresolved: "keys.blockerUnresolved",
  key_reference_outside_workspace: "keys.blockerReferenceExternal",
  key_group_not_declared: "keys.blockerGroupNotDeclared",
  key_destination_is_config: "keys.blockerDestinationIsConfig",
  key_in_state_directory: "keys.blockerStateDirectory",
};

export function describeBlocker(blocker: string, t: Translate): string {
  const separator = blocker.indexOf(":");
  const code = separator < 0 ? blocker : blocker.slice(0, separator);
  const detail = separator < 0 ? blocker : blocker.slice(separator + 1);
  return t(blockerLabels[code] ?? "keys.blockerOther", { detail });
}
