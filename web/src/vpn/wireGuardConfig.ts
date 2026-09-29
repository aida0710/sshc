import { addressBits, isUnroutableIPv4, parseAddress, parseGoInt, parsePrefix, type ParsedPrefix } from "./vpnAddressSyntax";
import type { VPNFieldError, VPNFieldReason } from "./vpnFieldErrors";

// WireGuard の設定ファイル（wg-quick の形）を、送る前に engine と同じ規則で確かめ、一覧に出す
// Endpoint のサーバーと、プロファイルの DNS にする DNS を読み取る。設定ファイルは鍵を含むので、
// 本文のままシークレットとして送る。
//
// 規則の正本は Go（internal/vpn/wireguard_config.go の ParseWireGuardConfig）にあり、ここは
// その写しである。同じ入力に同じ行と理由を返すことを、
// internal/vpn/testdata/wireguard-config-cases.json に対するテストで保つ。読み方は wg の
// config.c と同じにする（# から先は注釈、行の中の空白はすべて除く、キーは大文字小文字を
// 区別しない）。

// 上限は Go（internal/vpn/wireguard_config.go）と同じ値である。
const maxConfigLength = 16384;
const maxPeers = 64;
const maxKeepaliveSeconds = 120;
const minMTU = 576;
const maxMTU = 65535;
const maxPortNumber = 65535;
const maxResolvers = 3;
const maxServerLength = 320;
const maxFwMark = 0xffffffffn;
const keyLength = 44;

const configField = "secrets.wireguardConfig";
const interfaceSection = "[Interface]";
const peerSection = "[Peer]";
// spaces は、wg が取り除く空白である（ctype.h の char_is_space）。
const spaces = /[ \t\n\v\f\r]/g;

// keyNames は、読むキーを、小文字にした名前から引く表である。inPeer は [Peer] に書くキー。
const keyNames = new Map<string, { name: string; inPeer: boolean }>([
  ...["PrivateKey", "ListenPort", "FwMark", "Address", "DNS", "MTU", "Table", "PreUp", "PostUp", "PreDown", "PostDown", "SaveConfig"]
    .map((name) => [name.toLowerCase(), { name, inPeer: false }] as const),
  ...["PublicKey", "PresharedKey", "AllowedIPs", "Endpoint", "PersistentKeepalive"]
    .map((name) => [name.toLowerCase(), { name, inPeer: true }] as const),
]);
const commandKeys = new Set(["PreUp", "PostUp", "PreDown", "PostDown"]);

// WireGuardKey は、設定ファイルに書いた鍵ひとつである。line が 0 なら書いていない。
export type WireGuardKey = { value: string; line: number };

export type WireGuardPeer = {
  publicKey: string;
  presharedKey: WireGuardKey;
  endpoint: string;
  allowedIPs: ParsedPrefix[];
  // line は、[Peer] の行（1から数える）である。
  line: number;
};

export type WireGuardConfig = {
  // addresses は、トンネル側で名乗る IPv4 アドレス（CIDR）である。
  addresses: string[];
  // dns は、使う DNS サーバー（IPv4）である。
  dns: string[];
  privateKey: WireGuardKey;
  peers: WireGuardPeer[];
};

export type WireGuardInspection = { config: WireGuardConfig } | { refusal: VPNFieldError };

const noKey: WireGuardKey = { value: "", line: 0 };

// Refusal は、読むのをやめる理由である。関数の奥から投げ、inspectWireGuardConfig で受ける。
class Refusal {
  constructor(readonly error: VPNFieldError) {}
}

function refuse(reason: VPNFieldReason, line = 0, directive = "", limit?: number): never {
  throw new Refusal({
    field: configField, reason,
    ...(line === 0 ? {} : { line }),
    ...(directive === "" ? {} : { directive }),
    ...(limit === undefined ? {} : { limit }),
  });
}

function utf8Length(text: string): number {
  return new TextEncoder().encode(text).length;
}

// cleanLine は、注釈を除き、空白をすべて取り除く（config_read_line）。
function cleanLine(line: string): string {
  const comment = line.indexOf("#");
  return (comment >= 0 ? line.slice(0, comment) : line).replace(spaces, "");
}

// parseDecimal は、符号の無い10進の数を読む（Go の parseDecimal）。
function parseDecimal(text: string): number | null {
  return /^[0-9]+$/.test(text) ? parseGoInt(text) : null;
}

function validKey(value: string): boolean {
  return value.length === keyLength && /^[A-Za-z0-9+/]{43}=$/.test(value);
}

// readKey は、鍵を読む。
function readKey(value: string, line: number): WireGuardKey | null {
  return validKey(value) ? { value, line } : null;
}

// parseAddressOrPrefix は、アドレスか、アドレスと長さ（CIDR）を読む。長さが無ければアドレス
// ひとつぶん（/32、/128）とする（Go の parseWireGuardPrefix）。
function parseAddressOrPrefix(entry: string): ParsedPrefix | null {
  if (entry.includes("/")) return parsePrefix(entry);
  const address = parseAddress(entry);
  return address === null || address.zone !== "" ? null : { address, bits: addressBits(address) };
}

function formatIPv4Prefix(prefix: ParsedPrefix): string {
  return `${prefix.address.octets.join(".")}/${prefix.bits}`;
}

function ipv4Number(octets: number[]): number {
  return octets.reduce((total, octet) => total * 256 + octet, 0);
}

// contains は、prefix が IPv4 のアドレス address を含むかを返す（netip.Prefix.Contains）。
function contains(prefix: ParsedPrefix, address: number[]): boolean {
  if (prefix.address.version !== 4) return false;
  const size = 2 ** (32 - prefix.bits);
  return Math.floor(ipv4Number(prefix.address.octets) / size) === Math.floor(ipv4Number(address) / size);
}

function validFwMark(value: string): boolean {
  const hexadecimal = /^0x([0-9a-fA-F]+)$/.exec(value)?.[1];
  if (hexadecimal !== undefined) return BigInt(`0x${hexadecimal}`) <= maxFwMark;
  return /^[0-9]+$/.test(value) && BigInt(value) <= maxFwMark;
}

// validEndpoint は、Endpoint が `host:port`（IPv6 は `[address]:port`）かを返す。
function validEndpoint(endpoint: string): boolean {
  const separator = endpoint.lastIndexOf(":");
  if (separator <= 0) return false;
  const host = endpoint.slice(0, separator);
  const port = parseDecimal(endpoint.slice(separator + 1));
  if (port === null || port === 0 || port > maxPortNumber) return false;
  if (host.startsWith("[") && host.endsWith("]")) return parseAddress(host.slice(1, -1))?.version === 6;
  return utf8Length(host) <= maxServerLength && !/[":\\[\]]/.test(host);
}

type Reader = {
  config: WireGuardConfig;
  section: string;
  interfaceLine: number;
  addressLine: number;
  dnsLines: Map<string, number>;
  publicKeys: Set<string>;
};

function enterSection(reader: Reader, name: string, line: number): void {
  const lower = name.toLowerCase();
  if (lower === interfaceSection.toLowerCase()) {
    if (reader.interfaceLine > 0) refuse("duplicate", line, interfaceSection);
    reader.interfaceLine = line;
    reader.section = interfaceSection;
    return;
  }
  if (lower === peerSection.toLowerCase()) {
    if (reader.config.peers.length === maxPeers) refuse("too_many", line, "", maxPeers);
    reader.config.peers.push({ publicKey: "", presharedKey: noKey, endpoint: "", allowedIPs: [], line });
    reader.section = peerSection;
    return;
  }
  refuse("unknown_directive", line);
}

function readAddresses(reader: Reader, value: string, line: number): void {
  if (reader.addressLine === 0) reader.addressLine = line;
  for (const entry of value.split(",")) {
    const prefix = parseAddressOrPrefix(entry);
    if (prefix === null) refuse("format", line, "Address");
    if (prefix.address.version !== 4) continue;
    if (isUnroutableIPv4(prefix.address.octets)) refuse("unroutable", line, "Address");
    reader.config.addresses.push(formatIPv4Prefix(prefix));
  }
}

// readResolvers は、DNS を読む。wg-quick と同じく、数字と点だけのものと : を含むものを
// DNS サーバー、それ以外を検索ドメインとして読み、IPv4 の DNS サーバーだけを使う。
function readResolvers(reader: Reader, value: string, line: number): void {
  for (const entry of value.split(",")) {
    if (entry === "") refuse("format", line, "DNS");
    if (entry.includes(":")) {
      if (parseAddress(entry)?.version !== 6) refuse("format", line, "DNS");
      continue;
    }
    if (!/^[0-9.]+$/.test(entry)) continue;
    const address = parseAddress(entry);
    if (address?.version !== 4) refuse("format", line, "DNS");
    if (isUnroutableIPv4(address.octets)) refuse("unroutable", line, "DNS");
    if (reader.config.dns.length === maxResolvers) refuse("too_many", line, "DNS", maxResolvers);
    reader.config.dns.push(entry);
    reader.dnsLines.set(entry, line);
  }
}

function readInterfaceKey(reader: Reader, name: string, value: string, line: number): void {
  const lower = value.toLowerCase();
  switch (name) {
    case "PrivateKey": {
      if (reader.config.privateKey.line > 0) refuse("duplicate", line, name);
      const key = readKey(value, line);
      if (key === null) refuse("format", line, name);
      reader.config.privateKey = key;
      return;
    }
    case "ListenPort": {
      const port = parseDecimal(value);
      if (port === null) refuse("format", line, name);
      if (port > maxPortNumber) refuse("out_of_range", line, name);
      return;
    }
    case "FwMark":
      if (lower !== "off" && !validFwMark(value)) refuse("format", line, name);
      return;
    case "Address":
      readAddresses(reader, value, line);
      return;
    case "DNS":
      readResolvers(reader, value, line);
      return;
    case "MTU": {
      const mtu = parseDecimal(value);
      if (mtu === null) refuse("format", line, name);
      if (mtu < minMTU || mtu > maxMTU) refuse("mtu_out_of_range", line, name);
      return;
    }
    case "Table":
      // off は「経路を作らない」で、sshc のすることと同じである。
      if (lower !== "off") refuse("changes_routes", line, name);
      return;
    case "SaveConfig":
      if (lower === "true") refuse("decided_by_sshc", line, name);
      if (lower !== "false") refuse("format", line, name);
      return;
  }
}

function readPeerKey(reader: Reader, name: string, value: string, line: number): void {
  const peer = reader.config.peers[reader.config.peers.length - 1];
  if (peer === undefined) return;
  switch (name) {
    case "PresharedKey": {
      if (peer.presharedKey.line > 0) refuse("duplicate", line, name);
      const key = readKey(value, line);
      if (key === null) refuse("format", line, name);
      peer.presharedKey = key;
      return;
    }
    case "PublicKey":
      if (peer.publicKey !== "") refuse("duplicate", line, name);
      if (!validKey(value)) refuse("format", line, name);
      if (reader.publicKeys.has(value)) refuse("duplicate", line, name);
      reader.publicKeys.add(value);
      peer.publicKey = value;
      return;
    case "AllowedIPs":
      for (const entry of value.split(",")) {
        const prefix = parseAddressOrPrefix(entry);
        if (prefix === null) refuse("format", line, name);
        peer.allowedIPs.push(prefix);
      }
      return;
    case "Endpoint":
      if (!validEndpoint(value)) refuse("format", line, name);
      peer.endpoint = value;
      return;
    case "PersistentKeepalive": {
      if (value.toLowerCase() === "off") return;
      const seconds = parseDecimal(value);
      if (seconds === null) refuse("format", line, name);
      if (seconds > maxKeepaliveSeconds) refuse("keepalive_too_long", line, name, maxKeepaliveSeconds);
      return;
    }
  }
}

// readLine は、1行を読む。
function readLine(reader: Reader, raw: string, line: number): void {
  if (raw.includes("\0")) refuse("format", line);
  const cleaned = cleanLine(raw);
  if (cleaned === "") return;
  if (cleaned.startsWith("[")) {
    enterSection(reader, cleaned, line);
    return;
  }
  const separator = cleaned.indexOf("=");
  if (separator <= 0 || separator === cleaned.length - 1) refuse("format", line);
  const known = keyNames.get(cleaned.slice(0, separator).toLowerCase());
  if (known === undefined) refuse("unknown_directive", line);
  if (commandKeys.has(known.name)) refuse("runs_command", line, known.name);
  if (reader.section !== (known.inPeer ? peerSection : interfaceSection)) refuse("misplaced_directive", line, known.name);
  const value = cleaned.slice(separator + 1);
  if (known.inPeer) readPeerKey(reader, known.name, value, line);
  else readInterfaceKey(reader, known.name, value, line);
}

function finish(reader: Reader): void {
  const { config } = reader;
  if (reader.interfaceLine === 0) refuse("missing_directive", 0, interfaceSection);
  if (config.privateKey.line === 0) refuse("missing_directive", 0, "PrivateKey");
  if (reader.addressLine === 0) refuse("missing_directive", 0, "Address");
  if (config.addresses.length === 0) refuse("not_ipv4", reader.addressLine, "Address");
  if (config.peers.length === 0) refuse("missing_directive", 0, peerSection);
  for (const peer of config.peers) {
    if (peer.publicKey === "") refuse("missing_directive", peer.line, "PublicKey");
  }
  if (!config.peers.some((peer) => peer.endpoint !== "")) refuse("no_endpoint");
  for (const resolver of config.dns) {
    const octets = parseAddress(resolver)?.octets ?? [];
    const allowed = config.peers.some((peer) => peer.allowedIPs.some((prefix) => contains(prefix, octets)));
    if (!allowed) refuse("not_in_allowed_ips", reader.dnsLines.get(resolver) ?? 0, "DNS");
  }
}

// inspectWireGuardConfig は、設定ファイルを確かめ、使えれば読み取った中身を、使えなければ
// 最初に断る行と理由を返す。
export function inspectWireGuardConfig(text: string): WireGuardInspection {
  if (utf8Length(text) > maxConfigLength) {
    return { refusal: { field: configField, reason: "too_long", limit: maxConfigLength } };
  }
  const reader: Reader = {
    config: { addresses: [], dns: [], privateKey: noKey, peers: [] },
    section: "", interfaceLine: 0, addressLine: 0, dnsLines: new Map(), publicKeys: new Set(),
  };
  try {
    for (const [index, raw] of text.split("\n").entries()) readLine(reader, raw, index + 1);
    finish(reader);
  } catch (error) {
    if (error instanceof Refusal) return { refusal: error.error };
    throw error;
  }
  return { config: reader.config };
}

// wireGuardServers は、Endpoint のサーバー（`host:port` の host）を、書かれた順に重複を除いて
// 返す（Go の WireGuardConfig.Servers）。プロファイルの設定として送り、一覧に出す。
export function wireGuardServers(config: WireGuardConfig): string[] {
  const hosts: string[] = [];
  for (const peer of config.peers) {
    if (peer.endpoint === "") continue;
    const host = peer.endpoint.slice(0, peer.endpoint.lastIndexOf(":"));
    if (!hosts.includes(host)) hosts.push(host);
  }
  return hosts;
}
