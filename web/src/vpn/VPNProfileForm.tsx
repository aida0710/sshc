import { useState } from "react";
import type { VPNProfile, VPNSecrets } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText, sectionHeading } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Card } from "../ui/surface";

// 経路をひとつ作る。接続先はひとつだけ持つ。秘密は保存のときに送るだけで、
// 保存が終われば画面からも消える。

type DraftSecrets = {
  wireguardPrivateKey: string;
  l2tpPassword: string;
  ipsecPsk: string;
  openconnectPassword: string;
};

type Backend = "wireguard" | "l2tp_ipsec" | "openconnect";

const emptySecrets: DraftSecrets = {
  wireguardPrivateKey: "", l2tpPassword: "", ipsecPsk: "", openconnectPassword: "",
};

// 装置が話す方式。engine と同じ語を使う。
const openConnectProtocols = ["anyconnect", "nc", "pulse", "gp", "f5", "fortinet", "array"] as const;

// splitResolvers は、読み取った DNS の並びを一件ずつに分ける。
function splitResolvers(value: string): string[] {
  return value
    .split(",")
    .map((resolver) => resolver.trim())
    .filter((resolver) => resolver !== "");
}

export function VPNProfileForm({
  busy,
  onSave,
}: {
  busy: boolean;
  onSave: (profile: VPNProfile, secrets: VPNSecrets) => void;
}) {
  const t = useTranslate();
  const [name, setName] = useState("");
  const [backend, setBackend] = useState<Backend>("wireguard");
  const [target, setTarget] = useState("");
  const [resolvers, setResolvers] = useState("");
  const [server, setServer] = useState("");
  const [peerPublicKey, setPeerPublicKey] = useState("");
  const [address, setAddress] = useState("10.0.0.2/32");
  const [username, setUsername] = useState("");
  const [ike, setIke] = useState("");
  const [esp, setEsp] = useState("");
  const [protocol, setProtocol] = useState<string>(openConnectProtocols[0]);
  const [serverCertificate, setServerCertificate] = useState("");
  const [secrets, setSecrets] = useState<DraftSecrets>(emptySecrets);

  const complete =
    name !== "" &&
    target !== "" &&
    server !== "" &&
    (backend === "wireguard"
      ? peerPublicKey !== "" && address !== "" && secrets.wireguardPrivateKey !== ""
      : backend === "openconnect"
        ? username !== "" && secrets.openconnectPassword !== ""
        : username !== "" && secrets.l2tpPassword !== "" && secrets.ipsecPsk !== "");

  function save() {
    const dns = splitResolvers(resolvers);
    const common = { name, backend, target, ...(dns.length === 0 ? {} : { dns }) };
    const profile: VPNProfile =
      backend === "wireguard"
        ? { ...common, wireguard: { server, peerPublicKey, address } }
        : backend === "openconnect"
          ? {
              ...common,
              openconnect: {
                server,
                username,
                protocol,
                ...(serverCertificate === "" ? {} : { serverCertificate }),
              },
            }
          : {
              ...common,
              l2tp: {
                server,
                username,
                ...(ike === "" ? {} : { ike }),
                ...(esp === "" ? {} : { esp }),
              },
            };
    const carried: VPNSecrets =
      backend === "wireguard"
        ? { wireguardPrivateKey: secrets.wireguardPrivateKey }
        : backend === "openconnect"
          ? { openconnectPassword: secrets.openconnectPassword }
          : { l2tpPassword: secrets.l2tpPassword, ipsecPsk: secrets.ipsecPsk };
    onSave(profile, carried);
    setSecrets(emptySecrets);
  }

  return (
    <Card as="section" padded aria-label={t("vpn.addHeading")}>
      <p className={sectionHeading}>{t("vpn.addHeading")}</p>
      <p className={hintText}>{t("vpn.addHint")}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label={t("vpn.name")}>
          <input className={control} value={name} onChange={(event) => setName(event.target.value)} />
        </Field>
        <Field label={t("vpn.backend")}>
          <select
            className={control}
            value={backend}
            onChange={(event) => setBackend(event.target.value as Backend)}
          >
            <option value="wireguard">WireGuard</option>
            <option value="l2tp_ipsec">L2TP/IPsec</option>
            <option value="openconnect">OpenConnect</option>
          </select>
        </Field>
        <Field label={t("vpn.target")} hint={t("vpn.targetHint")}>
          <input className={control} value={target} onChange={(event) => setTarget(event.target.value)} />
        </Field>
        <Field label={t("vpn.server")}>
          <input className={control} value={server} onChange={(event) => setServer(event.target.value)} />
        </Field>
        <Field label={t("vpn.dns")} hint={t("vpn.dnsHint")}>
          <input className={control} value={resolvers} onChange={(event) => setResolvers(event.target.value)} />
        </Field>
        {backend === "wireguard" ? (
          <>
            <Field label={t("vpn.peerPublicKey")}>
              <input
                className={control}
                value={peerPublicKey}
                onChange={(event) => setPeerPublicKey(event.target.value)}
              />
            </Field>
            <Field label={t("vpn.address")}>
              <input className={control} value={address} onChange={(event) => setAddress(event.target.value)} />
            </Field>
            <PasswordField
              label={t("vpn.privateKey")}
              hint={t("vpn.secretHint")}
              value={secrets.wireguardPrivateKey}
              onChange={(value) => setSecrets({ ...secrets, wireguardPrivateKey: value })}
            />
          </>
        ) : backend === "openconnect" ? (
          <>
            <Field label={t("vpn.username")}>
              <input className={control} value={username} onChange={(event) => setUsername(event.target.value)} />
            </Field>
            <Field label={t("vpn.protocol")} hint={t("vpn.protocolHint")}>
              <select className={control} value={protocol} onChange={(event) => setProtocol(event.target.value)}>
                {openConnectProtocols.map((name) => (
                  <option key={name} value={name}>
                    {name}
                  </option>
                ))}
              </select>
            </Field>
            <Field label={t("vpn.serverCertificate")} hint={t("vpn.serverCertificateHint")}>
              <input
                className={control}
                value={serverCertificate}
                onChange={(event) => setServerCertificate(event.target.value)}
              />
            </Field>
            <PasswordField
              label={t("vpn.password")}
              hint={t("vpn.secretHint")}
              value={secrets.openconnectPassword}
              onChange={(value) => setSecrets({ ...secrets, openconnectPassword: value })}
            />
          </>
        ) : (
          <>
            <Field label={t("vpn.username")}>
              <input className={control} value={username} onChange={(event) => setUsername(event.target.value)} />
            </Field>
            <PasswordField
              label={t("vpn.password")}
              hint={t("vpn.secretHint")}
              value={secrets.l2tpPassword}
              onChange={(value) => setSecrets({ ...secrets, l2tpPassword: value })}
            />
            <PasswordField
              label={t("vpn.psk")}
              hint={t("vpn.secretHint")}
              value={secrets.ipsecPsk}
              onChange={(value) => setSecrets({ ...secrets, ipsecPsk: value })}
            />
            <Field label={t("vpn.ike")} hint={t("vpn.proposalsHint")}>
              <input className={control} value={ike} onChange={(event) => setIke(event.target.value)} />
            </Field>
            <Field label={t("vpn.esp")} hint={t("vpn.proposalsHint")}>
              <input className={control} value={esp} onChange={(event) => setEsp(event.target.value)} />
            </Field>
          </>
        )}
      </div>
      <div>
        <Button kind="primary" disabled={busy || !complete} onClick={save}>
          {t("vpn.save")}
        </Button>
      </div>
    </Card>
  );
}
