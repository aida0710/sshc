import { parseGoInt, splitHostPort } from "./vpnAddressSyntax";

// VPN プロファイルの接続先（`host:port`）と、ある接続の相手が同じものかを決める。
// Go の vpn.Endpoint.Reaches と同じ答えを出す。両者は target-cases.json で揃える。

export type VPNEndpoint = { host: string; port: number };

// parseVPNEndpoint は、保存形式の `host:port` を読む。application.parseEndpoint と同じく、
// port は strconv.Atoi で読める数でなければならない。
export function parseVPNEndpoint(value: string): VPNEndpoint | null {
  const split = splitHostPort(value);
  if (split === null) return null;
  const port = parseGoInt(split.port);
  return port === null ? null : { host: split.host, port };
}

// sameHost は、名前を大文字と小文字を区別せず、末尾の `.` を無視して比べる。
// DNS の名前として同じものを、書き方の違いだけで食い違いとして扱わない。
function sameHost(left: string, right: string): boolean {
  const withoutRoot = (name: string) => (name.endsWith(".") ? name.slice(0, -1) : name);
  return withoutRoot(left).toLowerCase() === withoutRoot(right).toLowerCase();
}

// vpnTargetReaches は、プロファイルの接続先 target が address（`host:port`）を指すかを返す。
export function vpnTargetReaches(target: string, address: string): boolean {
  const endpoint = parseVPNEndpoint(target);
  const wanted = splitHostPort(address);
  if (endpoint === null || wanted === null) return false;
  return wanted.port === String(endpoint.port) && sameHost(wanted.host, endpoint.host);
}

// joinHostPort は net.JoinHostPort と同じく、`:` を含む host を角括弧で囲む。
export function joinHostPort(host: string, port: string): string {
  return host.includes(":") ? `[${host}]:${port}` : `${host}:${port}`;
}
