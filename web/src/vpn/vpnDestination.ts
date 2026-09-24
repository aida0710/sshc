import { isUnroutableIPv4, parseAddress, parseGoInt, splitHostPort } from "./vpnAddressSyntax";

// VPN プロファイルを付けた接続の接続先（HostName:Port）を、そのプロファイル経由で
// 使えるかを確かめる。
//
// 規則の正本は Go の vpn.Profile.Destination で、ここはその写しである。同じ入力に同じ
// 理由を返すことを、internal/vpn/testdata/destination-cases.json に対するテストで保つ。
// 検査の順番も Go と同じにする。最初に当たる理由が変わるからである。

// VPNDestinationReason は、接続先を使えない理由の語である（engine の vpn.Reason と同じ）。
export type VPNDestinationReason = "format" | "out_of_range" | "not_ipv4" | "unroutable" | "name_needs_dns";

const maxPort = 65535;
// DNS の名前と、その1段の長さの上限である（RFC 1035）。
const maxHostNameLength = 253;
const maxHostLabelLength = 63;
// コンテナの中の connect へ引数として渡せる名前の1段である。英数字とハイフンだけで、
// ハイフンで始まらず、ハイフンで終わらない。
const hostLabel = /^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$/;

// isHostName は、Go の validHostName と同じく、コンテナの中でそのまま名前解決できる
// 名前かを返す。末尾の `.` は許す。
function isHostName(name: string): boolean {
  if (name === "" || name.length > maxHostNameLength) return false;
  const withoutRoot = name.endsWith(".") ? name.slice(0, -1) : name;
  return withoutRoot.split(".").every((label) => label.length <= maxHostLabelLength && hostLabel.test(label));
}

// vpnDestinationHostRefusal は、接続先のホスト（HostName）だけを確かめる。hasDNS は、
// プロファイルに VPN 内の DNS サーバーがあるかである。使えるなら null を返す。
export function vpnDestinationHostRefusal(host: string, hasDNS: boolean): VPNDestinationReason | null {
  const address = parseAddress(host);
  if (address !== null) {
    if (address.version !== 4) return "not_ipv4";
    return isUnroutableIPv4(address.octets) ? "unroutable" : null;
  }
  if (!isHostName(host)) return "format";
  // 名前は、プロファイルの DNS サーバーだけを使ってコンテナの中で名前解決する。
  return hasDNS ? null : "name_needs_dns";
}

// vpnDestinationRefusal は、接続先 address（`host:port`）を確かめる。使えるなら null を返す。
export function vpnDestinationRefusal(address: string, hasDNS: boolean): VPNDestinationReason | null {
  const split = splitHostPort(address);
  if (split === null || split.host === "") return "format";
  const port = parseGoInt(split.port);
  if (port === null || port <= 0 || port > maxPort) return "out_of_range";
  return vpnDestinationHostRefusal(split.host, hasDNS);
}
