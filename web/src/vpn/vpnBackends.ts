// 経路の方式と、その画面での表示名。engine の BackendName と同じ語を使う。
// 表示名は製品や規格の名前なので、言語によって変えない。
const vpnBackendLabels = {
  wireguard: "WireGuard",
  l2tp_ipsec: "L2TP/IPsec",
  openconnect: "OpenConnect",
} as const;

export type VPNBackend = keyof typeof vpnBackendLabels;

export const vpnBackends = Object.keys(vpnBackendLabels) as VPNBackend[];

export function isVPNBackend(name: string): name is VPNBackend {
  return Object.hasOwn(vpnBackendLabels, name);
}

// vpnBackendLabel は、方式の表示名を返す。この版の知らない方式（新しい版が書いた
// プロファイル）は、engine の語のまま見せる。
export function vpnBackendLabel(backend: string): string {
  return isVPNBackend(backend) ? vpnBackendLabels[backend] : backend;
}
