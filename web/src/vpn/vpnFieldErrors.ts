import { ApiError } from "../api/client";
import type { Translate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";

// 項目ひとつを受け取れない理由の形と、その画面での言い方。engine の vpn.FieldError と
// 同じ語を使う。field は保存形式（API の VPNProfile と VPNSecrets）の JSON のパスで、
// シークレットは `secrets.` を前に付ける。

const vpnFieldReasonMessages = {
  required: "vpn.field.required",
  format: "vpn.field.format",
  too_long: "vpn.field.too_long",
  too_many: "vpn.field.too_many",
  out_of_range: "vpn.field.out_of_range",
  not_ipv4: "vpn.field.not_ipv4",
  unroutable: "vpn.field.unroutable",
  unsupported: "vpn.field.unsupported",
  unexpected: "vpn.field.unexpected",
} as const satisfies Record<string, MessageKey>;

// 項目の JSON のパスと、フォームでのその項目の名前。項目の横に出せないとき（名前を
// 変更したときなど）に、どの項目の誤りかを文で言うのに使う。
const vpnFieldLabels: Record<string, MessageKey> = {
  name: "vpn.name",
  backend: "vpn.backend",
  dns: "vpn.dns",
  "wireguard.server": "vpn.server",
  "wireguard.peerPublicKey": "vpn.peerPublicKey",
  "wireguard.address": "vpn.address",
  "l2tp.server": "vpn.server",
  "l2tp.username": "vpn.username",
  "l2tp.ike": "vpn.ike",
  "l2tp.esp": "vpn.esp",
  "openconnect.server": "vpn.server",
  "openconnect.username": "vpn.username",
  "openconnect.protocol": "vpn.protocol",
  "openconnect.serverCertificate": "vpn.serverCertificate",
  "openconnect.secondFactor": "vpn.secondFactor",
  "openconnect.approvalWord": "vpn.approvalWord",
  "secrets.wireguardPrivateKey": "vpn.privateKey",
  "secrets.l2tpPassword": "vpn.password",
  "secrets.ipsecPsk": "vpn.psk",
  "secrets.openconnectPassword": "vpn.password",
  "secrets.openconnectTotpSecret": "vpn.secondFactorSecret",
};

export type VPNFieldReason = keyof typeof vpnFieldReasonMessages;

export type VPNFieldError = {
  field: string;
  reason: VPNFieldReason;
  // limit は、too_long と too_many のときの上限である。
  limit?: number;
};

function isFieldReason(reason: string): reason is VPNFieldReason {
  return Object.hasOwn(vpnFieldReasonMessages, reason);
}

// vpnFieldErrorOf は、engine の拒否が項目の誤りを名指ししていれば、それを返す。
// 知らない理由の語は、この画面では項目の横に出せないので null にする。
export function vpnFieldErrorOf(error: unknown): VPNFieldError | null {
  if (!(error instanceof ApiError)) return null;
  const { field, reason, limit } = error.problem ?? {};
  if (field === undefined || field === "" || reason === undefined || !isFieldReason(reason)) return null;
  return limit === undefined ? { field, reason } : { field, reason, limit };
}

// describeVPNFieldError は、項目の誤りを、その項目の横に出す1文にする。
export function describeVPNFieldError(t: Translate, error: VPNFieldError): string {
  return t(vpnFieldReasonMessages[error.reason], error.limit === undefined ? {} : { limit: error.limit });
}

// describeVPNFieldRefusal は、項目の誤りを「項目: 理由」の1文にする。項目の名前が
// 分からなければ null を返す。
export function describeVPNFieldRefusal(t: Translate, error: VPNFieldError): string | null {
  const label = vpnFieldLabels[error.field];
  if (label === undefined) return null;
  return t("vpn.fieldRefusedAt", { field: t(label), reason: describeVPNFieldError(t, error) });
}
