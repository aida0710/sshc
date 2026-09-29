import type { VPNProfile, VPNSecrets } from "../api/vpn";
import { inspectOpenVPNConfig } from "./openVPNConfig";
import type { VPNBackend } from "./vpnBackends";
import { inspectWireGuardConfig, wireGuardServers } from "./wireGuardConfig";
import { openConnectProtocols, type IKEv2Authentication } from "./vpnProfileRules";
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
  // username は、L2TP/IPsec、OpenConnect、OpenVPN のユーザー名であり、IKEv2 の ID（EAP
  // ではユーザー名）である。
  username: string;
  ikev2Authentication: IKEv2Authentication;
  serverIdentity: string;
  // caCertificate は、IKEv2 でサーバーの証明書を確かめる CA の証明書（PEM）である。
  caCertificate: string;
  ike: string;
  esp: string;
  protocol: string;
  serverCertificate: string;
  secondFactor: SecondFactor;
  approvalWord: string;
  // servers は、設定ファイルから読むサーバーである（OpenVPN の remote、WireGuard の Endpoint）。
  servers: string[];
  secrets: Record<VPNSecretKey, string>;
};

export const emptySecrets: Record<VPNSecretKey, string> = {
  wireguardConfig: "", l2tpPassword: "", ipsecPsk: "",
  openconnectPassword: "", openconnectTotpSecret: "",
  openvpnConfig: "", openvpnPassword: "",
  ikev2Password: "", ikev2Psk: "",
};

export const emptyDraft: VPNProfileDraft = {
  name: "", backend: "wireguard", resolvers: "", server: "", username: "", ike: "", esp: "",
  protocol: openConnectProtocols[0], serverCertificate: "", secondFactor: "", approvalWord: "",
  servers: [],
  ikev2Authentication: "eap-mschapv2", serverIdentity: "", caCertificate: "",
  secrets: emptySecrets,
};

// settingsSection は、方式ごとの設定の節の名前である。engine が返す項目の JSON パスの
// 先頭になる。
export const settingsSection: Record<VPNBackend, string> = {
  wireguard: "wireguard",
  l2tp_ipsec: "l2tp",
  openconnect: "openconnect",
  openvpn: "openvpn",
  ikev2: "ikev2",
};

// splitResolvers は、入力された DNS の並びを一件ずつに分ける。
function splitResolvers(value: string): string[] {
  return value
    .split(",")
    .map((resolver) => resolver.trim())
    .filter((resolver) => resolver !== "");
}

// draftOf は、保存済みのプロファイルを編集用の下書きにする。シークレットは一覧の応答に
// 無いので空にする。フォームは、別に取り出した保存済みのシークレットを withSecrets で入れる。
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
      return settings === undefined ? common : { ...common, servers: settings.servers };
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
    case "openvpn": {
      const settings = profile.openvpn;
      return settings === undefined ? common : { ...common, servers: settings.servers, username: settings.username ?? "" };
    }
    case "ikev2": {
      const settings = profile.ikev2;
      return settings === undefined ? common : {
        ...common,
        server: settings.server,
        username: settings.identity,
        ikev2Authentication: settings.authentication as IKEv2Authentication,
        serverIdentity: settings.serverIdentity ?? "",
        caCertificate: settings.caCertificate ?? "",
        ike: settings.ike ?? "",
        esp: settings.esp ?? "",
      };
    }
  }
}

export function profileOf(draft: VPNProfileDraft): VPNProfile {
  const dns = splitResolvers(draft.resolvers);
  const common = { name: draft.name, backend: draft.backend, ...(dns.length === 0 ? {} : { dns }) };
  switch (draft.backend) {
    case "wireguard":
      // DNS は設定ファイルの DNS で、withWireGuardConfig が resolvers に入れてある。
      return { ...common, wireguard: { servers: draft.servers } };
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
    case "openvpn":
      return {
        ...common,
        openvpn: { servers: draft.servers, ...(draft.username === "" ? {} : { username: draft.username }) },
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
    case "ikev2":
      return {
        ...common,
        ikev2: {
          server: draft.server,
          authentication: draft.ikev2Authentication,
          identity: draft.username,
          ...(draft.serverIdentity === "" ? {} : { serverIdentity: draft.serverIdentity }),
          // 事前共有鍵では、サーバーも事前共有鍵で認証するので、CA の証明書は送らない。
          ...(draft.ikev2Authentication === "psk" || draft.caCertificate === "" ? {} : { caCertificate: draft.caCertificate }),
          ...(draft.ike === "" ? {} : { ike: draft.ike }),
          ...(draft.esp === "" ? {} : { esp: draft.esp }),
        },
      };
  }
}

// storedSecretKeys は、保存済みのプロファイルが Vault に持っているシークレットである。
// 編集で空欄のまま送ると、engine はこれらの保存済みの値をそのまま使う。保存済みの値を
// 取り出せなかったときに使う。
export function storedSecretKeys(profile: VPNProfile): ReadonlySet<VPNSecretKey> {
  return new Set(vpnSecretKeys({
    backend: profile.backend,
    secondFactor: profile.openconnect?.secondFactor ?? "",
    username: profile.openvpn?.username ?? "",
    ikev2Authentication: profile.ikev2?.authentication ?? "",
  }));
}

// secretsOf は、選んだ方式が使うシークレットのうち、入力されたものだけを送る形にする。
// 空欄の項目は送らない。API は空の項目を受け付けず、編集では保存済みの値を残す意味になる。
export function secretsOf(draft: VPNProfileDraft): VPNSecrets {
  const secrets: VPNSecrets = {};
  for (const key of vpnSecretKeys(draft)) {
    if (draft.secrets[key] !== "") secrets[key] = draft.secrets[key];
  }
  return secrets;
}

// hasRequiredValues は、方式が要る値がすべて入っているかを返す。stored にある
// シークレットは、空欄でも保存済みの値を使うので入っているものとして扱う。
export function hasRequiredValues(draft: VPNProfileDraft, stored: ReadonlySet<VPNSecretKey>): boolean {
  if (draft.backend === "openvpn" || draft.backend === "wireguard") {
    // サーバーは設定ファイルから読むので、設定ファイル（と、OpenVPN でユーザー名を書いたなら
    // パスワード）があればよい。
    return draft.name !== "" &&
      vpnSecretKeys(draft).every((key) => draft.secrets[key] !== "" || stored.has(key));
  }
  if (draft.name === "" || draft.server === "" || draft.username === "") return false;
  return vpnSecretKeys(draft).every((key) => draft.secrets[key] !== "" || stored.has(key));
}

// withOpenVPNConfig は、OpenVPN の設定ファイルを入れ替えた下書きを返す。remote のサーバーは
// 設定ファイルから読む。空欄に戻したら、保存済みの設定ファイルのサーバー（storedServers）に
// 戻す。読めない設定ファイルは保存の前の検査で断るので、サーバーは前のままにする。
export function withOpenVPNConfig(draft: VPNProfileDraft, config: string, storedServers: string[]): VPNProfileDraft {
  const secrets = { ...draft.secrets, openvpnConfig: config };
  if (config === "") return { ...draft, secrets, servers: storedServers };
  const inspected = inspectOpenVPNConfig(config);
  return { ...draft, secrets, servers: "summary" in inspected ? inspected.summary.servers : draft.servers };
}

// StoredConfigFacts は、保存済みの設定ファイルから読んで、プロファイルに持っている値である。
// 設定ファイルの欄を空欄に戻したときは、これに戻す（engine が保存済みの設定ファイルを使う）。
export type StoredConfigFacts = { servers: string[]; resolvers: string };

// withWireGuardConfig は、WireGuard の設定ファイルを入れ替えた下書きを返す。Endpoint のサーバーと
// DNS は設定ファイルから読む。読めない設定ファイルは保存の前の検査で断るので、前のままにする。
export function withWireGuardConfig(draft: VPNProfileDraft, config: string, stored: StoredConfigFacts): VPNProfileDraft {
  const secrets = { ...draft.secrets, wireguardConfig: config };
  if (config === "") return { ...draft, secrets, servers: stored.servers, resolvers: stored.resolvers };
  const inspected = inspectWireGuardConfig(config);
  if (!("config" in inspected)) return { ...draft, secrets };
  return {
    ...draft, secrets, servers: wireGuardServers(inspected.config), resolvers: inspected.config.dns.join(", "),
  };
}

// withSecrets は、取り出した保存済みのシークレットを、下書きのシークレットの欄に入れる。
// 設定ファイルからは、サーバー（WireGuard では DNS も）を読み直す。
export function withSecrets(draft: VPNProfileDraft, revealed: VPNSecrets): VPNProfileDraft {
  const secrets = { ...emptySecrets };
  for (const key of Object.keys(emptySecrets) as VPNSecretKey[]) secrets[key] = revealed[key] ?? "";
  const filled = { ...draft, secrets };
  if (draft.backend === "openvpn" && secrets.openvpnConfig !== "") {
    return withOpenVPNConfig(filled, secrets.openvpnConfig, draft.servers);
  }
  if (draft.backend === "wireguard" && secrets.wireguardConfig !== "") {
    return withWireGuardConfig(filled, secrets.wireguardConfig, { servers: draft.servers, resolvers: draft.resolvers });
  }
  return filled;
}
