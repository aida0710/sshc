import type { VPNProfile } from "../api/vpn";

// 経路の方式と、その画面での表示名。engine の BackendName と同じ語を使う。
// 表示名は製品や規格の名前なので、言語によって変えない。

export type VPNBackend = VPNProfile["backend"];

const vpnBackendLabels: Record<VPNBackend, string> = {
  wireguard: "WireGuard",
  l2tp_ipsec: "L2TP/IPsec",
  openconnect: "OpenConnect",
};

export const vpnBackends = Object.keys(vpnBackendLabels) as VPNBackend[];

// isVPNBackend は、engine が受け付ける方式かを返す。送る前の検査が使う。
export function isVPNBackend(name: string): name is VPNBackend {
  return Object.hasOwn(vpnBackendLabels, name);
}

export function vpnBackendLabel(backend: VPNBackend): string {
  return vpnBackendLabels[backend];
}
