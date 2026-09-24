import type { VPNProfile, VPNSecrets } from "../api/vpn";
import type { VPNBackend } from "./vpnBackends";
import { openConnectProtocols } from "./vpnProfileRules";
import { vpnSecretKeys, type VPNSecretKey } from "./vpnSecretRules";

// VPN プロファイルのフォームに入力中の値と、API の VPNProfile・VPNSecrets との変換。
// フォームは方式ごとの欄を1つの下書きに持ち、送るときに選んだ方式の節だけにする。

// サーバーがパスワードのあとに要求する二要素認証への答え方。engine と同じ語を使う。
export type SecondFactor = "" | "approve" | "totp";

export type VPNProfileDraft = {
  name: string;
  backend: VPNBackend;
  // resolvers は、VPN 内の DNS サーバーをカンマで区切った入力である。
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
  secrets: Required<VPNSecrets>;
};

export const emptySecrets: Required<VPNSecrets> = {
  wireguardPrivateKey: "", l2tpPassword: "", ipsecPsk: "",
  openconnectPassword: "", openconnectTotpSecret: "",
};

// トンネル側のアドレスは、1台だけを名乗る /32 がほとんどなので、例として入れておく。
const suggestedTunnelAddress = "10.0.0.2/32";

export const emptyDraft: VPNProfileDraft = {
  name: "", backend: "wireguard", resolvers: "", server: "",
  peerPublicKey: "", address: suggestedTunnelAddress, username: "", ike: "", esp: "",
  protocol: openConnectProtocols[0], serverCertificate: "", secondFactor: "", approvalWord: "",
  secrets: emptySecrets,
};

// settingsSection は、方式ごとの設定の節の名前である。engine が返す項目の JSON パスの
// 先頭になる。
export const settingsSection: Record<VPNBackend, string> = {
  wireguard: "wireguard",
  l2tp_ipsec: "l2tp",
  openconnect: "openconnect",
};

// splitResolvers は、入力された DNS の並びを一件ずつに分ける。
function splitResolvers(value: string): string[] {
  return value
    .split(",")
    .map((resolver) => resolver.trim())
    .filter((resolver) => resolver !== "");
}

// draftOf は、保存済みのプロファイルを編集用の下書きにする。シークレットは engine が
// 返さないので空にする。
export function draftOf(profile: VPNProfile): VPNProfileDraft {
  const common: VPNProfileDraft = {
    ...emptyDraft,
    name: profile.name,
    backend: profile.backend,
    resolvers: (profile.dns ?? []).join(", "),
  };
  switch (profile.backend) {
    case "wireguard": {
      const settings = profile.wireguard;
      return settings === undefined ? common : {
        ...common, server: settings.server, peerPublicKey: settings.peerPublicKey, address: settings.address,
      };
    }
    case "l2tp_ipsec": {
      const settings = profile.l2tp;
      return settings === undefined ? common : {
        ...common, server: settings.server, username: settings.username, ike: settings.ike ?? "", esp: settings.esp ?? "",
      };
    }
    case "openconnect": {
      const settings = profile.openconnect;
      return settings === undefined ? common : {
        ...common,
        server: settings.server,
        username: settings.username,
        // 空は engine が anyconnect として扱うので、選択肢でも anyconnect にする。
        protocol: settings.protocol || openConnectProtocols[0],
        serverCertificate: settings.serverCertificate ?? "",
        secondFactor: (settings.secondFactor ?? "") as SecondFactor,
        approvalWord: settings.approvalWord ?? "",
      };
    }
  }
}

export function profileOf(draft: VPNProfileDraft): VPNProfile {
  const dns = splitResolvers(draft.resolvers);
  const common = { name: draft.name, backend: draft.backend, ...(dns.length === 0 ? {} : { dns }) };
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

// storedSecretKeys は、保存済みのプロファイルが Vault に持っているシークレットである。
// 編集で空欄のまま送ると、engine はこれらの保存済みの値をそのまま使う。
export function storedSecretKeys(profile: VPNProfile): ReadonlySet<VPNSecretKey> {
  return new Set(vpnSecretKeys(profile.backend, profile.openconnect?.secondFactor ?? ""));
}

// secretsOf は、選んだ方式が使うシークレットのうち、入力されたものだけを送る形にする。
// 空欄の項目は送らない。API は空の項目を受け付けず、編集では保存済みの値を残す意味になる。
export function secretsOf(draft: VPNProfileDraft): VPNSecrets {
  const secrets: VPNSecrets = {};
  for (const key of vpnSecretKeys(draft.backend, draft.secondFactor)) {
    if (draft.secrets[key] !== "") secrets[key] = draft.secrets[key];
  }
  return secrets;
}

// hasRequiredValues は、方式が要る値がすべて入っているかを返す。stored にある
// シークレットは、空欄でも保存済みの値を使うので入っているものとして扱う。
export function hasRequiredValues(draft: VPNProfileDraft, stored: ReadonlySet<VPNSecretKey>): boolean {
  if (draft.name === "" || draft.server === "") return false;
  const settings = draft.backend === "wireguard"
    ? draft.peerPublicKey !== "" && draft.address !== ""
    : draft.username !== "";
  const secrets = vpnSecretKeys(draft.backend, draft.secondFactor)
    .every((key) => draft.secrets[key] !== "" || stored.has(key));
  return settings && secrets;
}
