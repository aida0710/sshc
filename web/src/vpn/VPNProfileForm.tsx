import { useState } from "react";
import type { VPNProfile, VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Card } from "../ui/surface";
import { vpnBackendLabel, vpnBackends, type VPNBackend } from "./vpnBackends";
import { describeVPNFieldError, type VPNFieldError } from "./vpnFieldErrors";
import { openConnectProtocols, vpnProfileFieldError } from "./vpnProfileRules";

// VPNプロファイルをひとつ作る。接続先はひとつだけ持つ。秘密は保存のときに送るだけで、
// 保存できればフォームごと空に戻す。断られたときは、入れ直さずに直せるよう秘密も残す。

// VPNProfileSaveResult は、保存を頼んだ結果である。断られた項目が分かれば、
// その項目の横に理由を出す。
export type VPNProfileSaveResult = { saved: true } | { saved: false; fieldError: VPNFieldError | null };

type DraftSecrets = {
  wireguardPrivateKey: string;
  l2tpPassword: string;
  ipsecPsk: string;
  openconnectPassword: string;
  openconnectTotpSecret: string;
};

// 装置がパスワードのあとにもう一問聞くときの答え方。engine と同じ語を使う。
type SecondFactor = "" | "approve" | "totp";

type Draft = {
  name: string;
  backend: VPNBackend;
  target: string;
  resolvers: string;
  server: string;
  peerPublicKey: string;
  address: string;
  username: string;
  ike: string;
  esp: string;
  protocol: string;
  serverCertificate: string;
  secondFactor: SecondFactor;
  approvalWord: string;
  secrets: DraftSecrets;
};

const emptySecrets: DraftSecrets = {
  wireguardPrivateKey: "", l2tpPassword: "", ipsecPsk: "",
  openconnectPassword: "", openconnectTotpSecret: "",
};

// トンネル側のアドレスは、1台だけを名乗る /32 がほとんどなので、例として入れておく。
const suggestedTunnelAddress = "10.0.0.2/32";

const emptyDraft: Draft = {
  name: "", backend: "wireguard", target: "", resolvers: "", server: "",
  peerPublicKey: "", address: suggestedTunnelAddress, username: "", ike: "", esp: "",
  protocol: openConnectProtocols[0], serverCertificate: "", secondFactor: "", approvalWord: "",
  secrets: emptySecrets,
};

// 方式ごとの設定の節の名前。engine が返す項目の JSON パスの先頭になる。
const settingsSection: Record<VPNBackend, string> = {
  wireguard: "wireguard",
  l2tp_ipsec: "l2tp",
  openconnect: "openconnect",
};

// splitResolvers は、読み取った DNS の並びを一件ずつに分ける。
function splitResolvers(value: string): string[] {
  return value
    .split(",")
    .map((resolver) => resolver.trim())
    .filter((resolver) => resolver !== "");
}

// isComplete は、方式が要る値がすべて入っているかを返す。揃うまでは保存させない。
function isComplete(draft: Draft): boolean {
  const { secrets } = draft;
  if (draft.name === "" || draft.target === "" || draft.server === "") return false;
  switch (draft.backend) {
    case "wireguard":
      return draft.peerPublicKey !== "" && draft.address !== "" && secrets.wireguardPrivateKey !== "";
    case "openconnect":
      return draft.username !== "" && secrets.openconnectPassword !== "" &&
        (draft.secondFactor !== "totp" || secrets.openconnectTotpSecret !== "");
    case "l2tp_ipsec":
      return draft.username !== "" && secrets.l2tpPassword !== "" && secrets.ipsecPsk !== "";
  }
}

function profileOf(draft: Draft): VPNProfile {
  const dns = splitResolvers(draft.resolvers);
  const common = { name: draft.name, backend: draft.backend, target: draft.target, ...(dns.length === 0 ? {} : { dns }) };
  switch (draft.backend) {
    case "wireguard":
      return { ...common, wireguard: { server: draft.server, peerPublicKey: draft.peerPublicKey, address: draft.address } };
    case "openconnect":
      return {
        ...common,
        openconnect: {
          server: draft.server,
          username: draft.username,
          protocol: draft.protocol,
          ...(draft.serverCertificate === "" ? {} : { serverCertificate: draft.serverCertificate }),
          ...(draft.secondFactor === "" ? {} : { secondFactor: draft.secondFactor }),
          ...(draft.secondFactor === "approve" && draft.approvalWord !== "" ? { approvalWord: draft.approvalWord } : {}),
        },
      };
    case "l2tp_ipsec":
      return {
        ...common,
        l2tp: {
          server: draft.server,
          username: draft.username,
          ...(draft.ike === "" ? {} : { ike: draft.ike }),
          ...(draft.esp === "" ? {} : { esp: draft.esp }),
        },
      };
  }
}

// secretsOf は、選んだ方式が使う秘密だけを送る形にする。
function secretsOf(draft: Draft): VPNSecrets {
  const { secrets } = draft;
  switch (draft.backend) {
    case "wireguard":
      return { wireguardPrivateKey: secrets.wireguardPrivateKey };
    case "openconnect":
      return {
        openconnectPassword: secrets.openconnectPassword,
        ...(draft.secondFactor === "totp" ? { openconnectTotpSecret: secrets.openconnectTotpSecret } : {}),
      };
    case "l2tp_ipsec":
      return { l2tpPassword: secrets.l2tpPassword, ipsecPsk: secrets.ipsecPsk };
  }
}

export function VPNProfileForm({
  busy,
  onSave,
}: {
  busy: boolean;
  onSave: (profile: VPNProfile, secrets: VPNSecrets) => Promise<VPNProfileSaveResult>;
}) {
  const t = useTranslate();
  const [draft, setDraft] = useState<Draft>(emptyDraft);
  const [refusal, setRefusal] = useState<VPNFieldError | null>(null);
  const section = settingsSection[draft.backend];

  function edit<K extends keyof Draft>(key: K) {
    return (value: Draft[K]) => {
      setRefusal(null);
      setDraft((current) => ({ ...current, [key]: value }));
    };
  }

  function editSecret(key: keyof DraftSecrets) {
    return (value: string) => {
      setRefusal(null);
      setDraft((current) => ({ ...current, secrets: { ...current.secrets, [key]: value } }));
    };
  }

  // 方式を変えたら、それまでの方式の秘密を手元にも残さない。
  function chooseBackend(backend: VPNBackend) {
    setRefusal(null);
    setDraft((current) => ({ ...current, backend, secrets: emptySecrets }));
  }

  // errorFor は、その項目が断られていれば理由の1文を返す。
  function errorFor(field: string): string | undefined {
    return refusal !== null && refusal.field === field ? describeVPNFieldError(t, refusal) : undefined;
  }

  async function save() {
    const profile = profileOf(draft);
    const refused = vpnProfileFieldError(profile);
    if (refused !== null) {
      setRefusal(refused);
      return;
    }
    const result = await onSave(profile, secretsOf(draft));
    if (result.saved) {
      setDraft(emptyDraft);
      setRefusal(null);
    } else {
      setRefusal(result.fieldError);
    }
  }

  return (
    <Card as="section" padded aria-label={t("vpn.addHeading")}>
      <p className={sectionHeading}>{t("vpn.addHeading")}</p>
      <p className={hintText}>{t("vpn.addHint")}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("vpn.name")} error={errorFor("name")}>
          <input className={control} value={draft.name} onChange={(event) => edit("name")(event.target.value)} />
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
        <Field label={t("vpn.target")} hint={t("vpn.targetHint")} error={errorFor("target")}>
          <input className={control} value={draft.target} onChange={(event) => edit("target")(event.target.value)} />
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
            <PasswordField
              label={t("vpn.privateKey")}
              hint={t("vpn.secretHint")}
              error={errorFor("secrets.wireguardPrivateKey")}
              value={draft.secrets.wireguardPrivateKey}
              onChange={editSecret("wireguardPrivateKey")}
            />
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
            <PasswordField
              label={t("vpn.password")}
              hint={t("vpn.secretHint")}
              error={errorFor("secrets.openconnectPassword")}
              value={draft.secrets.openconnectPassword}
              onChange={editSecret("openconnectPassword")}
            />
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
            {draft.secondFactor === "totp" ? (
              <PasswordField
                label={t("vpn.secondFactorSecret")}
                hint={t("vpn.secretHint")}
                error={errorFor("secrets.openconnectTotpSecret")}
                value={draft.secrets.openconnectTotpSecret}
                onChange={editSecret("openconnectTotpSecret")}
              />
            ) : null}
          </>
        ) : (
          <>
            <Field label={t("vpn.username")} error={errorFor("l2tp.username")}>
              <input className={control} value={draft.username} onChange={(event) => edit("username")(event.target.value)} />
            </Field>
            <PasswordField
              label={t("vpn.password")}
              hint={t("vpn.secretHint")}
              error={errorFor("secrets.l2tpPassword")}
              value={draft.secrets.l2tpPassword}
              onChange={editSecret("l2tpPassword")}
            />
            <PasswordField
              label={t("vpn.psk")}
              hint={t("vpn.secretHint")}
              error={errorFor("secrets.ipsecPsk")}
              value={draft.secrets.ipsecPsk}
              onChange={editSecret("ipsecPsk")}
            />
            <Field label={t("vpn.ike")} hint={t("vpn.proposalsHint")} error={errorFor("l2tp.ike")}>
              <input className={control} value={draft.ike} onChange={(event) => edit("ike")(event.target.value)} />
            </Field>
            <Field label={t("vpn.esp")} hint={t("vpn.proposalsHint")} error={errorFor("l2tp.esp")}>
              <input className={control} value={draft.esp} onChange={(event) => edit("esp")(event.target.value)} />
            </Field>
          </>
        )}
      </div>
      <div>
        <Button kind="primary" disabled={busy || !isComplete(draft)} onClick={() => void save()}>
          {t("vpn.save")}
        </Button>
      </div>
    </Card>
  );
}
