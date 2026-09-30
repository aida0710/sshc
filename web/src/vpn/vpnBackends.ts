import type { VPNProfile } from "../api/vpn";

// 経路の方式と、その画面での表示名。engine の BackendName と同じ語を使う。
// 表示名は製品や規格の名前なので、言語によって変えない。

export type VPNBackend = VPNProfile["backend"];

const vpnBackendLabels: Record<VPNBackend, string> = {
  wireguard: "WireGuard",
  l2tp_ipsec: "L2TP/IPsec",
  openconnect: "OpenConnect",
  openvpn: "OpenVPN",
  ikev2: "IKEv2/IPsec",
};

export const vpnBackends = Object.keys(vpnBackendLabels) as VPNBackend[];

// isVPNBackend は、engine が受け付ける方式かを返す。送る前の検査が使う。
export function isVPNBackend(name: string): name is VPNBackend {
  return Object.hasOwn(vpnBackendLabels, name);
}

export function vpnBackendLabel(backend: VPNBackend): string {
  return vpnBackendLabels[backend];
}

// VPNSettingsSection は、VPNProfile のうち、方式ごとの設定を持つ項目の名前である。
export type VPNSettingsSection = "wireguard" | "l2tp" | "openconnect" | "openvpn" | "ikev2";

// settingsSection は、方式ごとの設定の節の名前である。engine が返す項目の JSON パスの
// 先頭になる。並びは Go の foreignSection と同じにする。違う節が2つあるとき、最初に断る
// 節が同じになるからである。
export const settingsSection: Record<VPNBackend, VPNSettingsSection> = {
  wireguard: "wireguard",
  l2tp_ipsec: "l2tp",
  openconnect: "openconnect",
  openvpn: "openvpn",
  ikev2: "ikev2",
};
