import { useMemo, useState } from "react";
import type { VPNProfile, VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Card } from "../ui/surface";
import { vpnBackendLabel, vpnBackends, type VPNBackend } from "./vpnBackends";
import { describeVPNFieldError, type VPNFieldError } from "./vpnFieldErrors";
import {
  draftOf,
  emptyDraft,
  emptySecrets,
  hasRequiredValues,
  profileOf,
  secretsOf,
  settingsSection,
  storedSecretKeys,
  type SecondFactor,
  type VPNProfileDraft,
} from "./vpnProfileDraft";
import { openConnectProtocols, vpnProfileFieldError } from "./vpnProfileRules";
import { vpnSecretsFieldError, type VPNSecretKey } from "./vpnSecretRules";

// VPNプロファイルを作成する、または保存済みのプロファイルを編集するフォーム。接続先は
// 持たない（プロファイルを付けた接続の HostName と Port が接続先になる）。
//
// シークレットは保存のときに送るだけで、engine は返さない。編集では、空欄のシークレットは
// 保存済みの値をそのまま使う。作成で保存できればフォームごと空に戻す。断られたときは、
// 入れ直さずに直せるようシークレットも残す。

// VPNProfileSaveResult は、保存を頼んだ結果である。断られた項目が分かれば、
// その項目の横に理由を出す。
export type VPNProfileSaveResult = { saved: true } | { saved: false; fieldError: VPNFieldError | null };

const noStoredSecrets: ReadonlySet<VPNSecretKey> = new Set();

export function VPNProfileForm({
  busy,
  editing,
  onSave,
  onCancel,
}: {
  busy: boolean;
  // editing は、編集する保存済みのプロファイルである。無ければ新しく作る。
  editing?: VPNProfile;
  onSave: (profile: VPNProfile, secrets: VPNSecrets) => Promise<VPNProfileSaveResult>;
  // onCancel は、編集をやめる。作成のフォームには無い。
  onCancel?: () => void;
}) {
  const t = useTranslate();
  const [draft, setDraft] = useState<VPNProfileDraft>(() => (editing === undefined ? emptyDraft : draftOf(editing)));
  const [refusal, setRefusal] = useState<VPNFieldError | null>(null);
  const section = settingsSection[draft.backend];
  // 方式を変えると engine は前の方式のシークレットを捨てるので、保存済みの値は使えない。
  const stored = useMemo(
    () => (editing === undefined || editing.backend !== draft.backend ? noStoredSecrets : storedSecretKeys(editing)),
    [editing, draft.backend],
  );
  const heading = editing === undefined ? t("vpn.addHeading") : t("vpn.editHeading", { name: editing.name });

  function edit<K extends keyof VPNProfileDraft>(key: K) {
    return (value: VPNProfileDraft[K]) => {
      setRefusal(null);
      setDraft((current) => ({ ...current, [key]: value }));
    };
  }

  function editSecret(key: VPNSecretKey) {
    return (value: string) => {
      setRefusal(null);
      setDraft((current) => ({ ...current, secrets: { ...current.secrets, [key]: value } }));
    };
  }

  // 方式を変えたら、それまでの方式のシークレットを手元にも残さない。
  function chooseBackend(backend: VPNBackend) {
    setRefusal(null);
    setDraft((current) => ({ ...current, backend, secrets: emptySecrets }));
  }

  // errorFor は、その項目が断られていれば理由の1文を返す。
  function errorFor(field: string): string | undefined {
    return refusal !== null && refusal.field === field ? describeVPNFieldError(t, refusal) : undefined;
  }

  function secretField(key: VPNSecretKey, label: string) {
    return (
      <PasswordField
        label={label}
        hint={t(stored.has(key) ? "vpn.secretKeepHint" : "vpn.secretHint")}
        error={errorFor(`secrets.${key}`)}
        value={draft.secrets[key]}
        onChange={editSecret(key)}
      />
    );
  }

  async function save() {
    const profile = profileOf(draft);
    // 送る前の検査で断られると、どの項目かが分からない。先に項目ごとに確かめる。
    const refused = vpnProfileFieldError(profile) ??
      vpnSecretsFieldError({ backend: draft.backend, secondFactor: draft.secondFactor, secrets: draft.secrets, stored });
    if (refused !== null) {
      setRefusal(refused);
      return;
    }
    const result = await onSave(profile, secretsOf(draft));
    if (!result.saved) {
      setRefusal(result.fieldError);
      return;
    }
    if (editing === undefined) {
      setDraft(emptyDraft);
      setRefusal(null);
    }
  }

  return (
    <Card as="section" padded aria-label={heading}>
      <p className={sectionHeading}>{heading}</p>
      <p className={hintText}>{editing === undefined ? t("vpn.addHint") : t("vpn.editHint")}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field
          label={t("vpn.name")}
          {...(editing === undefined ? {} : { hint: t("vpn.nameFixedHint") })}
          error={errorFor("name")}
        >
          <input
            className={control}
            value={draft.name}
            disabled={editing !== undefined}
            onChange={(event) => edit("name")(event.target.value)}
          />
        </Field>
        <Field label={t("vpn.backend")} error={errorFor("backend")}>
          <select
            className={control}
            value={draft.backend}
            onChange={(event) => chooseBackend(event.target.value as VPNBackend)}
          >
            {vpnBackends.map((backend) => (
              <option key={backend} value={backend}>
                {vpnBackendLabel(backend)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("vpn.server")} error={errorFor(`${section}.server`)}>
          <input className={control} value={draft.server} onChange={(event) => edit("server")(event.target.value)} />
        </Field>
        <Field label={t("vpn.dns")} hint={t("vpn.dnsHint")} error={errorFor("dns")}>
          <input className={control} value={draft.resolvers} onChange={(event) => edit("resolvers")(event.target.value)} />
        </Field>
        {draft.backend === "wireguard" ? (
          <>
            <Field label={t("vpn.peerPublicKey")} error={errorFor("wireguard.peerPublicKey")}>
              <input
                className={control}
                value={draft.peerPublicKey}
                onChange={(event) => edit("peerPublicKey")(event.target.value)}
              />
            </Field>
            <Field label={t("vpn.address")} error={errorFor("wireguard.address")}>
              <input className={control} value={draft.address} onChange={(event) => edit("address")(event.target.value)} />
            </Field>
            {secretField("wireguardPrivateKey", t("vpn.privateKey"))}
          </>
        ) : draft.backend === "openconnect" ? (
          <>
            <Field label={t("vpn.username")} error={errorFor("openconnect.username")}>
              <input className={control} value={draft.username} onChange={(event) => edit("username")(event.target.value)} />
            </Field>
            <Field label={t("vpn.protocol")} hint={t("vpn.protocolHint")} error={errorFor("openconnect.protocol")}>
              <select className={control} value={draft.protocol} onChange={(event) => edit("protocol")(event.target.value)}>
                {openConnectProtocols.map((protocol) => (
                  <option key={protocol} value={protocol}>
                    {protocol}
                  </option>
                ))}
              </select>
            </Field>
            <Field
              label={t("vpn.serverCertificate")}
              hint={t("vpn.serverCertificateHint")}
              error={errorFor("openconnect.serverCertificate")}
            >
              <input
                className={control}
                value={draft.serverCertificate}
                onChange={(event) => edit("serverCertificate")(event.target.value)}
              />
            </Field>
            {secretField("openconnectPassword", t("vpn.password"))}
            <Field label={t("vpn.secondFactor")} hint={t("vpn.secondFactorHint")} error={errorFor("openconnect.secondFactor")}>
              <select
                className={control}
                value={draft.secondFactor}
                onChange={(event) => edit("secondFactor")(event.target.value as SecondFactor)}
              >
                <option value="">{t("vpn.secondFactorNone")}</option>
                <option value="approve">{t("vpn.secondFactorApprove")}</option>
                <option value="totp">{t("vpn.secondFactorTOTP")}</option>
              </select>
            </Field>
            {draft.secondFactor === "approve" ? (
              <Field label={t("vpn.approvalWord")} hint={t("vpn.approvalWordHint")} error={errorFor("openconnect.approvalWord")}>
                <input
                  className={control}
                  value={draft.approvalWord}
                  onChange={(event) => edit("approvalWord")(event.target.value)}
                />
              </Field>
            ) : null}
            {draft.secondFactor === "totp" ? secretField("openconnectTotpSecret", t("vpn.secondFactorSecret")) : null}
          </>
        ) : (
          <>
            <Field label={t("vpn.username")} error={errorFor("l2tp.username")}>
              <input className={control} value={draft.username} onChange={(event) => edit("username")(event.target.value)} />
            </Field>
            {secretField("l2tpPassword", t("vpn.password"))}
            {secretField("ipsecPsk", t("vpn.psk"))}
            <Field label={t("vpn.ike")} hint={t("vpn.proposalsHint")} error={errorFor("l2tp.ike")}>
              <input className={control} value={draft.ike} onChange={(event) => edit("ike")(event.target.value)} />
            </Field>
            <Field label={t("vpn.esp")} hint={t("vpn.proposalsHint")} error={errorFor("l2tp.esp")}>
              <input className={control} value={draft.esp} onChange={(event) => edit("esp")(event.target.value)} />
            </Field>
          </>
        )}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button kind="primary" disabled={busy || !hasRequiredValues(draft, stored)} onClick={() => void save()}>
          {t("vpn.save")}
        </Button>
        {onCancel === undefined ? null : (
          <Button disabled={busy} onClick={onCancel}>
            {t("vpn.cancel")}
          </Button>
        )}
      </div>
    </Card>
  );
}
