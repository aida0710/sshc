import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import type { VPNProfileDraft } from "./vpnProfileDraft";
import type { IKEv2Authentication } from "./vpnProfileRules";
import type { VPNSecretKey } from "./vpnSecretRules";

// VPN プロファイルのフォームのうち、IKEv2/IPsec の欄。認証の方式で、ユーザー名と
// パスワード（EAP）か、ローカル ID と事前共有鍵かが変わる。CA の証明書は EAP のとき
// だけ使う。事前共有鍵では、サーバーも事前共有鍵で認証する。

export function IKEv2ProfileFields({
  draft,
  errorFor,
  onEdit,
  secretField,
}: {
  draft: VPNProfileDraft;
  // errorFor は、その項目が断られていれば理由の1文を返す。
  errorFor: (field: string) => string | undefined;
  onEdit: <K extends keyof VPNProfileDraft>(key: K) => (value: VPNProfileDraft[K]) => void;
  // secretField は、シークレットの欄を作る。
  secretField: (key: VPNSecretKey, label: string) => ReactNode;
}) {
  const t = useTranslate();
  const psk = draft.ikev2Authentication === "psk";
  return (
    <>
      <FieldRow label={t("vpn.ikev2Authentication")} hint={t("vpn.ikev2AuthenticationHint")} warning={errorFor("ikev2.authentication")}>
        <select
          className={control}
          value={draft.ikev2Authentication}
          onChange={(event) => onEdit("ikev2Authentication")(event.target.value as IKEv2Authentication)}
        >
          <option value="eap-mschapv2">{t("vpn.ikev2AuthenticationEAP")}</option>
          <option value="psk">{t("vpn.ikev2AuthenticationPSK")}</option>
        </select>
      </FieldRow>
      <FieldRow
        label={t(psk ? "vpn.localIdentity" : "vpn.username")}
        {...(psk ? { hint: t("vpn.localIdentityHint") } : {})}
        warning={errorFor("ikev2.identity")}
      >
        <input className={control} value={draft.username} onChange={(event) => onEdit("username")(event.target.value)} />
      </FieldRow>
      {psk ? secretField("ikev2Psk", t("vpn.psk")) : secretField("ikev2Password", t("vpn.password"))}
      <FieldRow label={t("vpn.serverIdentity")} hint={t("vpn.serverIdentityHint")} warning={errorFor("ikev2.serverIdentity")}>
        <input
          className={control}
          value={draft.serverIdentity}
          onChange={(event) => onEdit("serverIdentity")(event.target.value)}
        />
      </FieldRow>
      {psk ? null : (
        <FieldRow label={t("vpn.caCertificate")} hint={t("vpn.caCertificateHint")} warning={errorFor("ikev2.caCertificate")}>
          <textarea
            className={`${control} font-mono text-xs leading-5`}
            rows={4}
            spellCheck={false}
            placeholder="-----BEGIN CERTIFICATE-----"
            value={draft.caCertificate}
            onChange={(event) => onEdit("caCertificate")(event.target.value)}
          />
        </FieldRow>
      )}
      <FieldRow label={t("vpn.ike")} hint={t("vpn.ikev2ProposalsHint")} warning={errorFor("ikev2.ike")}>
        <input className={control} value={draft.ike} onChange={(event) => onEdit("ike")(event.target.value)} />
      </FieldRow>
      <FieldRow label={t("vpn.esp")} hint={t("vpn.ikev2ProposalsHint")} warning={errorFor("ikev2.esp")}>
        <input className={control} value={draft.esp} onChange={(event) => onEdit("esp")(event.target.value)} />
      </FieldRow>
    </>
  );
}
