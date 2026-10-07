import { failureCode } from "../api/client";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { codeText, type CodeMessages } from "../i18n/codeText";
import { describeVPNProblem } from "../vpn/vpnProblemMessage";
import { vpnRefusalMessages } from "../vpn/vpnRefusals";

// SFTP の失敗の code（engine の internal/httpserver/sftp.go の sftpProblem と転送の
// problem、画面の転送が自分で投げる code）の言い方。
const sftpProblemMessages: CodeMessages = {
  sftp_unsupported_operation: "sftp.problem.unsupportedOperation",
  sftp_metadata_unavailable: "sftp.problem.metadataUnavailable",
  sftp_ownership_unavailable: "sftp.ownershipUnavailable",
  sftp_invalid_space: "sftp.spaceUnavailable",
  action_token_invalid: "sftp.problem.confirmationChanged",
  action_token_expired: "sftp.problem.confirmationChanged",
  sftp_connection_lost: "sftp.problem.connectionLost",
  sftp_failed: "sftp.problem.failed",
  sftp_not_found: "sftp.problem.notFound",
  sftp_permission_denied: "sftp.problem.permissionDenied",
  sftp_local_permission_denied: "sftp.problem.localPermissionDenied",
  sftp_local_privacy_protection: "sftp.problem.localPrivacyProtection",
  sftp_conflict: "sftp.problem.conflict",
  sftp_exists: "sftp.problem.exists",
  sftp_transfer_limit: "sftp.problem.transferLimit",
  sftp_transfer_state: "sftp.problem.transferState",
  sftp_transfer_not_found: "sftp.problem.transferNotFound",
  sftp_transfer_too_large: "sftp.problem.transferTooLarge",
  sftp_text_too_large: "sftp.tooLargeHint",
  sftp_not_utf8: "sftp.binaryHint",
  sftp_preview_too_large: "sftp.previewUnavailable",
  sftp_preview_type: "sftp.previewUnavailable",
  sftp_wrong_type: "sftp.problem.wrongType",
  sftp_unsupported_entry: "sftp.problem.unsupportedEntry",
  sftp_compare_limit: "sftp.problem.compareLimit",
  sftp_traversal_limit: "sftp.problem.traversalLimit",
  sftp_target_inside_source: "sftp.problem.targetInsideSource",
  sftp_target_is_source: "sftp.problem.targetIsSource",
  sftp_range_invalid: "sftp.problem.rangeInvalid",
  sftp_cleanup_pending: "sftp.manager.cleanupFailed",
  sftp_reconciliation_required: "sftp.problem.reconciliationRequired",
  sftp_name_collision: "sftp.problem.nameCollision",
  sftp_spool_full: "sftp.problem.spoolFull",
  sftp_spool_unavailable: "sftp.problem.spoolUnavailable",
  transfer_interrupted: "sftp.problem.transferInterrupted",
  sftp_download_checkpoint_failed: "sftp.problem.downloadCheckpointFailed",
  sftp_download_storage_unsupported: "sftp.problem.downloadStorageUnsupported",
  sftp_upload_source_changed: "sftp.problem.uploadSourceChanged",
  download_changed: "sftp.problem.downloadChanged",
  download_failed: "sftp.problem.downloadFailed",
  download_revision_missing: "sftp.problem.downloadFailed",
};

// 転送一覧は失敗を code で記録するので、VPN の経路の拒否も code で残る。
const transferProblemMessages: CodeMessages = { ...vpnRefusalMessages, ...sftpProblemMessages };

// sftpProblemCode は、失敗から表を引く code を取り出す。engine の problem code のほか、
// 画面の転送は Error("sftp_transfer_limit") のように code を投げる。表に無い Error の文
// （fetch の失敗の英語など）は code として扱わない。
export function sftpProblemCode(error: unknown): string {
  const code = failureCode(error);
  if (code !== "") return code;
  return error instanceof Error && Object.hasOwn(sftpProblemMessages, error.message) ? error.message : "";
}

// sftpProblemText は、SFTP の操作の失敗を、画面に出す1文にする。
//
// VPN プロファイルを付けた接続では、VPN の経路や接続先の問題で失敗することがある。その
// ときは VPN 画面と同じ言い方の1文にする。汎用の失敗の文では、VPN の設定を直せば済む
// ことが利用者に分からないからである。表に無い失敗は fallback の文にする。
export function sftpProblemText(t: Translate, error: unknown, fallback: MessageKey = "sftp.problem.failed"): string {
  return describeVPNProblem(t, error) ?? codeText(t, sftpProblemCode(error), { messages: sftpProblemMessages, fallback });
}

export function localMutationProblemText(t: Translate, error: unknown, fallback: MessageKey = "sftp.problem.failed"): string {
  const code = sftpProblemCode(error);
  if (code === "sftp_conflict") return t("sftp.problem.localMutationConflict");
  if (code === "sftp_permission_denied") return t("sftp.problem.localPermissionDenied");
  return sftpProblemText(t, error, fallback);
}

// sftpTransferProblemText は、転送一覧が記録した problem code を1文にする。
export function sftpTransferProblemText(t: Translate, code: string): string {
  return codeText(t, code, { messages: transferProblemMessages, fallback: "sftp.problem.failed" });
}
