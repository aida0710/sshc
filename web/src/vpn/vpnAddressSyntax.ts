// Go の net.SplitHostPort、net.JoinHostPort、strconv.Atoi、netip.ParseAddr、
// netip.ParsePrefix と同じ規則で、`host:port` と IP アドレスを読み書きする。
//
// VPN プロファイルの検査と接続先の検査は Go に正本があり、画面は同じ入力に同じ答えを
// 出さなければならない。ブラウザの URL 解釈はこれと違う答えを出すので使わない。

export type HostPort = { host: string; port: string };

// splitHostPort は net.SplitHostPort と同じく、`host:port` と `[host]:port` を分ける。
// 読めなければ null を返す。port が数かどうかは見ない。
export function splitHostPort(value: string): HostPort | null {
  const lastColon = value.lastIndexOf(":");
  if (lastColon < 0) return null;
  let host: string;
  let hostStart = 0;
  let portSearchStart = 0;
  if (value.startsWith("[")) {
    const closing = value.indexOf("]");
    if (closing < 0 || closing + 1 !== lastColon) return null;
    host = value.slice(1, closing);
    hostStart = 1;
    portSearchStart = closing + 1;
  } else {
    host = value.slice(0, lastColon);
    if (host.includes(":")) return null;
  }
  if (value.indexOf("[", hostStart) >= 0) return null;
  if (value.indexOf("]", portSearchStart) >= 0) return null;
  return { host, port: value.slice(lastColon + 1) };
}

// Go の int は 64 ビットで、strconv.Atoi はその外を誤りにする。
const goIntMinimum = -(2n ** 63n);
const goIntMaximum = 2n ** 63n - 1n;

// parseGoInt は strconv.Atoi と同じく、符号付きの10進を読む。読めなければ null を返す。
// Number の安全な範囲を超える値は丸まるが、そうした値はポートとして範囲の外なので、
// 答え（out_of_range）は変わらない。
export function parseGoInt(value: string): number | null {
  if (!/^[+-]?[0-9]+$/.test(value)) return null;
  const parsed = BigInt(value);
  if (parsed < goIntMinimum || parsed > goIntMaximum) return null;
  return Number(parsed);
}

export type Endpoint = { host: string; port: number };

// parseEndpoint は、`host:port` を net.SplitHostPort と strconv.Atoi で読む。port の範囲は
// 見ない。読めなければ null を返す。
export function parseEndpoint(value: string): Endpoint | null {
  const split = splitHostPort(value);
  if (split === null) return null;
  const port = parseGoInt(split.port);
  return port === null ? null : { host: split.host, port };
}

// joinHostPort は net.JoinHostPort と同じく、`:` を含む host を角括弧で囲む。
export function joinHostPort(host: string, port: string | number): string {
  return host.includes(":") ? `[${host}]:${port}` : `${host}:${port}`;
}

export type ParsedAddress = { version: 4 | 6; zone: string; octets: number[] };

// IPv4 の各欄の上限と、IPv6 の欄の数と桁数である。
const ipv4FieldMaximum = 255;
const ipv4FieldCount = 4;
const ipv6ByteCount = 16;
const ipv6GroupDigits = 4;

// parseIPv4 は netip の IPv4 の読み方（4欄、先頭の0を許さない、255まで）で読む。
function parseIPv4(text: string): number[] | null {
  const fields = text.split(".");
  if (fields.length !== ipv4FieldCount) return null;
  const octets: number[] = [];
  for (const field of fields) {
    if (!/^[0-9]{1,3}$/.test(field) || (field.length > 1 && field.startsWith("0"))) return null;
    const octet = Number(field);
    if (octet > ipv4FieldMaximum) return null;
    octets.push(octet);
  }
  return octets;
}

// parseIPv6 は netip の IPv6 の読み方で読む。`::` は1つまでで、少なくとも1組の0を
// 表さなければならない。末尾の4バイトは IPv4 の書き方でもよい。
function parseIPv6(text: string): { octets: number[]; zone: string } | null {
  let address = text;
  let zone = "";
  const zoneStart = address.indexOf("%");
  if (zoneStart >= 0) {
    zone = address.slice(zoneStart + 1);
    address = address.slice(0, zoneStart);
    if (zone === "") return null;
  }
  const octets: number[] = [];
  let ellipsis = -1;
  let rest = address;
  if (rest.startsWith("::")) {
    ellipsis = 0;
    rest = rest.slice(2);
    if (rest === "") return { octets: new Array<number>(ipv6ByteCount).fill(0), zone };
  }
  while (octets.length < ipv6ByteCount) {
    const group = /^[0-9a-fA-F]+/.exec(rest)?.[0] ?? "";
    if (group === "" || group.length > ipv6GroupDigits) return null;
    const after = rest.charAt(group.length);
    if (after === ".") {
      // 埋め込みの IPv4 は、残りがちょうど4バイトのときの末尾にだけ置ける。
      if (ellipsis < 0 && octets.length !== ipv6ByteCount - ipv4FieldCount) return null;
      if (octets.length + ipv4FieldCount > ipv6ByteCount) return null;
      const embedded = parseIPv4(rest);
      if (embedded === null) return null;
      octets.push(...embedded);
      rest = "";
      break;
    }
    const value = Number.parseInt(group, 16);
    octets.push(value >> 8, value & 0xff);
    rest = rest.slice(group.length);
    if (rest === "") break;
    if (!rest.startsWith(":")) return null;
    if (rest.startsWith("::")) {
      if (ellipsis >= 0) return null;
      ellipsis = octets.length;
      rest = rest.slice(2);
      if (rest === "") break;
    } else {
      rest = rest.slice(1);
      if (rest === "") return null;
    }
  }
  if (rest !== "") return null;
  if (octets.length < ipv6ByteCount) {
    if (ellipsis < 0) return null;
    const zeros = new Array<number>(ipv6ByteCount - octets.length).fill(0);
    octets.splice(ellipsis, 0, ...zeros);
  } else if (ellipsis >= 0) {
    return null;
  }
  return { octets, zone };
}

// parseAddress は netip.ParseAddr と同じく、最初に現れた `.` か `:` で IPv4 と IPv6 を
// 見分けて読む。読めなければ null を返す。
export function parseAddress(text: string): ParsedAddress | null {
  for (const character of text) {
    if (character === ".") {
      const octets = parseIPv4(text);
      return octets === null ? null : { version: 4, zone: "", octets };
    }
    if (character === ":") {
      const parsed = parseIPv6(text);
      return parsed === null ? null : { version: 6, zone: parsed.zone, octets: parsed.octets };
    }
    if (character === "%") return null;
  }
  return null;
}

const ipv4Bits = 32;
const ipv6Bits = 128;

// parsePrefix は netip.ParsePrefix と同じく、`address/bits` を読む。zone は許さず、
// bits は先頭に0や符号の無い10進で、アドレスの長さを超えない。
export function parsePrefix(text: string): ParsedAddress | null {
  const slash = text.lastIndexOf("/");
  if (slash < 0) return null;
  const address = parseAddress(text.slice(0, slash));
  if (address === null || address.zone !== "") return null;
  const bits = text.slice(slash + 1);
  if (!/^(0|[1-9][0-9]*)$/.test(bits)) return null;
  if (Number(bits) > (address.version === 4 ? ipv4Bits : ipv6Bits)) return null;
  return address;
}

// isUnroutableIPv4 は、未指定・ループバック・マルチキャストの IPv4 アドレスかを返す。
// コンテナの中でこのアドレスへの /32 の経路は作れない。
export function isUnroutableIPv4(octets: number[]): boolean {
  const first = octets[0] ?? 0;
  const unspecified = octets.every((octet) => octet === 0);
  const loopback = first === 127;
  const multicast = first >= 224 && first <= 239;
  return unspecified || loopback || multicast;
}
