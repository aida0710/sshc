import type { VPNProfile } from "../api/vpn";
import { isUnroutableIPv4, parseAddress, parsePrefix } from "./vpnAddressSyntax";
import type { VPNFieldError, VPNFieldReason } from "./vpnFieldErrors";
import { isVPNBackend } from "./vpnBackends";
import { parseVPNEndpoint, type VPNEndpoint } from "./vpnEndpoint";

// VPN プロファイルの設定（秘密を除く）を送る前に、engine と同じ規則で確かめる。
//
// 規則の正本は Go（internal/application の VPNProfile.Profile と internal/vpn の検査）に
// あり、ここはその写しである。同じ入力に同じ項目と理由を返すことを、
// internal/vpn/testdata/profile-cases.json に対するテストで保つ。どちらかだけを変えると
// そのテストが落ちる。検査の順番も Go と同じにする。最初に断られる項目が変わるからである。

// maxProfileNameLength は、コンテナ名とソケットのパスに入る長さである（Go と同じ）。
// Go は len でバイト数を数えるので、ここも UTF-8 のバイト数で数える。
const maxProfileNameLength = 48;
// maxResolvers は、1つの経路が使う DNS サーバーの数の上限である（Go と同じ）。
const maxResolvers = 3;
// DNS の名前と、その1段の長さの上限である（RFC 1035）。
const maxHostNameLength = 253;
const maxHostLabelLength = 63;
// WireGuard の鍵は32バイトの base64 で、`=` で終わる44文字になる。
const wireGuardKeyLength = 44;
const maxPort = 65535;

// openConnectProtocols は、openconnect の --protocol に渡してよい方式である（Go と同じ）。
export const openConnectProtocols = ["anyconnect", "nc", "pulse", "gp", "f5", "fortinet", "array"] as const;
const openConnectFingerprintPrefixes = ["sha256:", "pin-sha256:"];
const secondFactors = ["", "approve", "totp"];

// 設定ファイルとコマンド引数へそのまま書けない字である。
const serverForbidden = /[ \t\r\n"\\]/;
const usernameForbidden = /[\r\n"\\]/;
const whitespace = /[ \t\r\n]/;

type Outcome = VPNFieldError | null;

function refuse(field: string, reason: VPNFieldReason): VPNFieldError {
  return { field, reason };
}

function isASCIIAlphanumeric(character: string): boolean {
  return /^[A-Za-z0-9]$/.test(character);
}

function utf8Length(text: string): number {
  return new TextEncoder().encode(text).length;
}

function validateProfileName(name: string): Outcome {
  if (name === "") return refuse("name", "required");
  if (utf8Length(name) > maxProfileNameLength) {
    return { field: "name", reason: "too_long", limit: maxProfileNameLength };
  }
  for (const character of name) {
    if (!isASCIIAlphanumeric(character) && character !== "-" && character !== "_") {
      return refuse("name", "format");
    }
  }
  return null;
}

function validatePort(field: string, port: number): Outcome {
  return port <= 0 || port > maxPort ? refuse(field, "out_of_range") : null;
}

function validHostName(name: string): boolean {
  if (name === "" || utf8Length(name) > maxHostNameLength) return false;
  const withoutRoot = name.endsWith(".") ? name.slice(0, -1) : name;
  return withoutRoot.split(".").every((label) => {
    if (label === "" || utf8Length(label) > maxHostLabelLength) return false;
    if (label.startsWith("-") || label.endsWith("-")) return false;
    return [...label].every((character) => isASCIIAlphanumeric(character) || character === "-");
  });
}

function validateRoutableIPv4(field: string, address: { version: 4 | 6; octets: number[] }): Outcome {
  if (address.version !== 4) return refuse(field, "not_ipv4");
  return isUnroutableIPv4(address.octets) ? refuse(field, "unroutable") : null;
}

// validateTarget は、接続先が IPv4 アドレスか、VPN の中の DNS で引ける名前かを確かめる。
function validateTarget(target: VPNEndpoint, resolvers: string[]): Outcome {
  const port = validatePort("target", target.port);
  if (port !== null) return port;
  if (target.host === "") return refuse("target", "required");
  const address = parseAddress(target.host);
  if (address === null) {
    if (!validHostName(target.host)) return refuse("target", "format");
    return resolvers.length === 0 ? refuse("target", "name_needs_dns") : null;
  }
  return validateRoutableIPv4("target", address);
}

function validateResolvers(resolvers: string[]): Outcome {
  if (resolvers.length > maxResolvers) return { field: "dns", reason: "too_many", limit: maxResolvers };
  for (const resolver of resolvers) {
    const address = parseAddress(resolver);
    if (address === null) return refuse("dns", "not_ipv4");
    const refused = validateRoutableIPv4("dns", address);
    if (refused !== null) return refused;
  }
  return null;
}

function validateServerName(field: string, server: string): Outcome {
  if (server === "") return refuse(field, "required");
  return serverForbidden.test(server) ? refuse(field, "format") : null;
}

function validateUsername(field: string, username: string): Outcome {
  if (username === "") return refuse(field, "required");
  return usernameForbidden.test(username) ? refuse(field, "format") : null;
}

function validateWireGuardKey(field: string, key: string): Outcome {
  if (key === "") return refuse(field, "required");
  if (key.length !== wireGuardKeyLength || !key.endsWith("=")) return refuse(field, "format");
  const body = key.slice(0, -1);
  const base64 = [...body].every((character) => isASCIIAlphanumeric(character) || character === "+" || character === "/");
  return base64 ? null : refuse(field, "format");
}

function validateWireGuard(settings: NonNullable<VPNProfile["wireguard"]>, server: VPNEndpoint): Outcome {
  if (server.host === "") return refuse("wireguard.server", "required");
  if (whitespace.test(server.host)) return refuse("wireguard.server", "format");
  const port = validatePort("wireguard.server", server.port);
  if (port !== null) return port;
  const key = validateWireGuardKey("wireguard.peerPublicKey", settings.peerPublicKey);
  if (key !== null) return key;
  if (settings.address === "") return refuse("wireguard.address", "required");
  const prefix = parsePrefix(settings.address);
  if (prefix === null) return refuse("wireguard.address", "format");
  return prefix.version === 4 ? null : refuse("wireguard.address", "not_ipv4");
}

function validateL2TP(settings: NonNullable<VPNProfile["l2tp"]>): Outcome {
  const server = validateServerName("l2tp.server", settings.server);
  if (server !== null) return server;
  const username = validateUsername("l2tp.username", settings.username);
  if (username !== null) return username;
  for (const [field, value] of [["l2tp.ike", settings.ike], ["l2tp.esp", settings.esp]] as const) {
    if (serverForbidden.test(value ?? "")) return refuse(field, "format");
  }
  return null;
}

function validateFingerprint(fingerprint: string): Outcome {
  if (fingerprint === "") return null;
  const known = openConnectFingerprintPrefixes.some((prefix) => fingerprint.startsWith(prefix));
  return !known || serverForbidden.test(fingerprint) ? refuse("openconnect.serverCertificate", "format") : null;
}

function validateOpenConnect(settings: NonNullable<VPNProfile["openconnect"]>): Outcome {
  const server = validateServerName("openconnect.server", settings.server);
  if (server !== null) return server;
  const username = validateUsername("openconnect.username", settings.username);
  if (username !== null) return username;
  const protocol = settings.protocol ?? "";
  if (protocol !== "" && !(openConnectProtocols as readonly string[]).includes(protocol)) {
    return refuse("openconnect.protocol", "unsupported");
  }
  const fingerprint = validateFingerprint(settings.serverCertificate ?? "");
  if (fingerprint !== null) return fingerprint;
  if (!secondFactors.includes(settings.secondFactor ?? "")) return refuse("openconnect.secondFactor", "unsupported");
  // 答えは1行として送る。改行が混じると、装置が受け取る問答がずれる。
  return whitespace.test(settings.approvalWord ?? "") ? refuse("openconnect.approvalWord", "format") : null;
}

// vpnProfileFieldError は、engine がこのプロファイルを断るなら、最初に断る項目と理由を
// 返す。通るなら null を返す。
export function vpnProfileFieldError(profile: VPNProfile): VPNFieldError | null {
  const target = parseVPNEndpoint(profile.target);
  if (target === null) return refuse("target", "format");
  // backend と違う節は、保存するときに落とすので見ない（Go の Normalized と同じ）。
  const wireGuard = profile.backend === "wireguard" ? profile.wireguard : undefined;
  let wireGuardServer: VPNEndpoint | null = null;
  if (wireGuard !== undefined) {
    wireGuardServer = parseVPNEndpoint(wireGuard.server);
    if (wireGuardServer === null) return refuse("wireguard.server", "format");
  }
  const resolvers = profile.dns ?? [];
  return (
    validateProfileName(profile.name) ??
    (isVPNBackend(profile.backend) ? null : refuse("backend", "unsupported")) ??
    validateResolvers(resolvers) ??
    validateTarget(target, resolvers) ??
    validateBackendSettings(profile, wireGuardServer)
  );
}

function validateBackendSettings(profile: VPNProfile, wireGuardServer: VPNEndpoint | null): Outcome {
  switch (profile.backend) {
    case "wireguard":
      return profile.wireguard === undefined || wireGuardServer === null
        ? refuse("wireguard", "required")
        : validateWireGuard(profile.wireguard, wireGuardServer);
    case "l2tp_ipsec":
      return profile.l2tp === undefined ? refuse("l2tp", "required") : validateL2TP(profile.l2tp);
    case "openconnect":
      return profile.openconnect === undefined ? refuse("openconnect", "required") : validateOpenConnect(profile.openconnect);
    default:
      return null;
  }
}
