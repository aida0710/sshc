import { isUnroutableIPv4, parseAddress } from "./vpnAddressSyntax";

// VPN プロファイルを付けた接続の接続先のホスト（HostName）を、そのプロファイル経由で
// 使えるかを確かめる。画面（Connections の注記）が使うのはホストの検査だけなので、
// ポートの検査は写さない。
//
// 規則の正本は Go の vpn.Profile.Destination で、ここはそのホストの部分の写しである。
// 同じ入力に同じ理由を返すことを、internal/vpn/testdata/destination-cases.json の
// ポートの正しい行に対するテストで保つ。検査の順番も Go と同じにする。最初に当たる
// 理由が変わるからである。

// VPNDestinationReason は、接続先を使えない理由の語である（engine の vpn.Reason と同じ）。
export type VPNDestinationReason = "format" | "out_of_range" | "not_ipv4" | "unroutable" | "name_needs_dns";

// DNS の名前と、その1段の長さの上限である（RFC 1035）。
const maxHostNameLength = 253;
const maxHostLabelLength = 63;
// コンテナの中の connect へ引数として渡せる名前の1段である。英数字とハイフンだけで、
// ハイフンで始まらず、ハイフンで終わらない。
const hostLabel = /^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$/;
// 数字とドットだけの名前である。getaddrinfo が IPv4 アドレスの略記（10.1 は 10.0.0.1）と
// 読み、経路とパケットフィルタの読み方と食い違うので、Go と同じく断る。
const numericDotted = /^[0-9.]+$/;

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
  if (!isHostName(host) || numericDotted.test(host)) return "format";
  // 名前は、プロファイルの DNS サーバーだけを使ってコンテナの中で名前解決する。
  return hasDNS ? null : "name_needs_dns";
}
