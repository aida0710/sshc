import { useMemo, useState } from "react";
import type { VPNProfile, VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Card, Notice } from "../ui/surface";
import { IKEv2ProfileFields } from "./IKEv2ProfileFields";
import { vpnBackendLabel, vpnBackends, type VPNBackend } from "./vpnBackends";
import { describeVPNFieldError, type VPNFieldError } from "./vpnFieldErrors";
import { OpenVPNProfileFields } from "./OpenVPNProfileFields";
import { WireGuardProfileFields } from "./WireGuardProfileFields";
import {
  draftOf,
  emptyDraft,
  emptySecrets,
  hasRequiredValues,
  profileOf,
  secretsOf,
  settingsSection,
  storedSecretKeys,
  withOpenVPNConfig,
  withSecrets,
  withWireGuardConfig,
  type SecondFactor,
  type StoredConfigFacts,
  type VPNProfileDraft,
} from "./vpnProfileDraft";
import { openConnectProducts, openConnectProtocols, vpnProfileFieldError } from "./vpnProfileRules";
import { vpnSecretsFieldError, type VPNSecretKey } from "./vpnSecretRules";
import { useVPNSecretsReveal } from "./useVPNSecretsReveal";

// VPNプロファイルを作成する、または保存済みのプロファイルを編集するフォーム。接続先は
// 持たない（プロファイルを付けた接続の HostName と Port が接続先になる）。
//
// 編集を開くと、保存済みのシークレットを engine から取り出して欄に入れる（WireGuard では鍵を
// 含む設定ファイル全体）。取り出せなかったときは欄を空にし、空欄のシークレットは保存済みの値を
// そのまま使う。取り出した値はこのフォームの中だけに持ち、閉じるときに消す。作成で保存できれば
// フォームごと空に戻す。断られたときは、入れ直さずに直せるようシークレットも残す。

// VPNProfileSaveResult は、保存を頼んだ結果である。断られた項目が分かれば、
// その項目の横に理由を出す。
export type VPNProfileSaveResult = { saved: true } | { saved: false; fieldError: VPNFieldError | null };

const noStoredSecrets: ReadonlySet<VPNSecretKey> = new Set();

export function VPNProfileForm({
  busy,
  editing,
  revealSecrets,
  onSave,
  onCancel,
}: {
  busy: boolean;
  // editing は、編集する保存済みのプロファイルである。無ければ新しく作る。
  editing?: VPNProfile;
  // revealSecrets は、保存済みのプロファイルのシークレットを取り出す。編集のフォームだけが使う。
  revealSecrets?: (name: string) => Promise<VPNSecrets>;
  onSave: (profile: VPNProfile, secrets: VPNSecrets) => Promise<VPNProfileSaveResult>;
  // onCancel は、編集をやめる。作成のフォームには無い。
  onCancel?: () => void;
}) {
  const t = useTranslate();
  const [draft, setDraft] = useState<VPNProfileDraft>(() => (editing === undefined ? emptyDraft : draftOf(editing)));
  const [refusal, setRefusal] = useState<VPNFieldError | null>(null);
  const { reveal, forget } = useVPNSecretsReveal({
    name: editing?.name,
    revealSecrets,
    // 取り出すあいだに方式を変えていたら、その方式の欄には入れない。
    onRevealed: (secrets) => {
      setDraft((current) => (current.backend === editing?.backend ? withSecrets(current, secrets) : current));
    },
  });
  const section = settingsSection[draft.backend];
  const revealed = reveal?.state === "revealed" ? reveal.secrets : null;
  // 保存済みの値を取り出して欄に入れたなら、空欄は「値が無い」である。取り出せなかったときは、
  // 空欄なら保存済みの値を使う。方式を変えると engine は前の方式のシークレットを捨てるので、
  // 保存済みの値は使えない。
  const stored = useMemo(
    () => (editing === undefined || editing.backend !== draft.backend || revealed !== null
      ? noStoredSecrets
      : storedSecretKeys(editing)),
    [editing, draft.backend, revealed],
  );
  const heading = editing === undefined ? t("vpn.addHeading") : t("vpn.editHeading", { name: editing.name });

  // storedConfigFacts は、保存済みの設定ファイルから読んで、プロファイルに持っている値である。
  // WireGuard の設定ファイルの欄を空欄に戻したときは、これに戻す。
  const storedConfigFacts: StoredConfigFacts = editing?.backend === "wireguard"
    ? { servers: editing.wireguard?.servers ?? [], resolvers: (editing.dns ?? []).join(", ") }
    : { servers: [], resolvers: "" };

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

  // 方式を変えたら、それまでの方式のシークレットを欄に残さない。設定ファイルから読んだサーバーも
  // 消す。保存済みのプロファイルの方式へ戻したときは、保存済みのサーバーと DNS と、取り出した
  // シークレットに戻す。
  function chooseBackend(backend: VPNBackend) {
    setRefusal(null);
    setDraft((current) => {
      const cleared: VPNProfileDraft = { ...current, backend, secrets: emptySecrets, servers: [] };
      if (editing === undefined || editing.backend !== backend) return cleared;
      const saved = draftOf(editing);
      const restored = { ...cleared, servers: saved.servers, resolvers: saved.resolvers };
      return revealed === null ? restored : withSecrets(restored, revealed);
    });
  }

  // close は、編集をやめる。取り出したシークレットを、フォームの状態にも残さない。
  function close() {
    setDraft((current) => ({ ...current, secrets: emptySecrets }));
    forget();
    onCancel?.();
  }

  // errorFor は、その項目が断られていれば理由の1文を返す。
  function errorFor(field: string): string | undefined {
    return refusal !== null && refusal.field === field ? describeVPNFieldError(t, refusal) : undefined;
  }

  function secretField(key: VPNSecretKey, label: string) {
    return (
      <PasswordField
        label={label}
        hint={t(stored.has(key) && draft.secrets[key] === "" ? "vpn.secretKeepHint" : "vpn.secretHint")}
        error={errorFor(`secrets.${key}`)}
        value={draft.secrets[key]}
        disabled={reveal?.state === "revealing"}
        onChange={editSecret(key)}
      />
    );
  }

  async function save() {
    const profile = profileOf(draft);
    // 送る前の検査で断られると、どの項目かが分からない。先に項目ごとに確かめる。
    const refused = vpnProfileFieldError(profile) ??
      vpnSecretsFieldError({
        backend: draft.backend,
        secondFactor: draft.secondFactor,
        username: draft.username,
        ikev2Authentication: draft.ikev2Authentication,
        secrets: draft.secrets,
        stored,
      });
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
      return;
    }
    // 保存した編集のフォームは閉じる。取り出したシークレットを残さない。
    setDraft((current) => ({ ...current, secrets: emptySecrets }));
    forget();
  }

  return (
    <Card as="section" padded aria-label={heading}>
      <p className={sectionHeading}>{heading}</p>
      <p className={hintText}>{editing === undefined ? t("vpn.addHint") : t("vpn.editHint")}</p>
      {reveal?.state === "revealing" ? <p className={hintText}>{t("vpn.secretsRevealing")}</p> : null}
      {reveal?.state === "failed" ? <Notice>{t("vpn.secretsRevealFailed", { reason: reveal.reason })}</Notice> : null}
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
        {draft.backend === "openvpn" || draft.backend === "wireguard" ? null : (
          <Field label={t("vpn.server")} error={errorFor(`${section}.server`)}>
            <input className={control} value={draft.server} onChange={(event) => edit("server")(event.target.value)} />
          </Field>
        )}
        {/* WireGuard の DNS は、設定ファイルの DNS の行で書く。 */}
        {draft.backend === "wireguard" ? null : (
          <Field label={t("vpn.dns")} hint={t("vpn.dnsHint")} error={errorFor("dns")}>
            <input className={control} value={draft.resolvers} onChange={(event) => edit("resolvers")(event.target.value)} />
          </Field>
        )}
        {draft.backend === "wireguard" ? (
          // サーバー、鍵、アドレス、DNS はどれも設定ファイルに書くので、ほかの欄は出さない。
          <WireGuardProfileFields
            config={draft.secrets.wireguardConfig}
            servers={draft.servers}
            keepsConfig={stored.has("wireguardConfig")}
            errorFor={errorFor}
            onConfig={(config) => {
              setRefusal(null);
              setDraft((current) => withWireGuardConfig(current, config, storedConfigFacts));
            }}
          />
        ) : draft.backend === "openconnect" ? (
          <>
            <Field label={t("vpn.username")} error={errorFor("openconnect.username")}>
              <input className={control} value={draft.username} onChange={(event) => edit("username")(event.target.value)} />
            </Field>
            <Field label={t("vpn.protocol")} hint={t("vpn.protocolHint")} error={errorFor("openconnect.protocol")}>
              <select className={control} value={draft.protocol} onChange={(event) => edit("protocol")(event.target.value)}>
                {openConnectProtocols.map((protocol) => (
                  <option key={protocol} value={protocol}>
                    {`${openConnectProducts[protocol]} (${protocol})`}
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
        ) : draft.backend === "openvpn" ? (
          // サーバーは設定ファイルの remote から読むので、サーバーの欄は出さない。
          <OpenVPNProfileFields
            config={draft.secrets.openvpnConfig}
            servers={draft.servers}
            username={draft.username}
            password={draft.secrets.openvpnPassword}
            keepsConfig={stored.has("openvpnConfig")}
            keepsPassword={stored.has("openvpnPassword")}
            errorFor={errorFor}
            onConfig={(config) => {
              setRefusal(null);
              setDraft((current) => withOpenVPNConfig(current, config, editing?.openvpn?.servers ?? []));
            }}
            onUsername={edit("username")}
            onPassword={editSecret("openvpnPassword")}
          />
        ) : draft.backend === "ikev2" ? (
          <IKEv2ProfileFields draft={draft} errorFor={errorFor} onEdit={edit} secretField={secretField} />
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
        <Button
          kind="primary"
          disabled={busy || reveal?.state === "revealing" || !hasRequiredValues(draft, stored)}
          onClick={() => void save()}
        >
          {t("vpn.save")}
        </Button>
        {onCancel === undefined ? null : (
          <Button disabled={busy} onClick={close}>
            {t("vpn.cancel")}
          </Button>
        )}
      </div>
    </Card>
  );
}
