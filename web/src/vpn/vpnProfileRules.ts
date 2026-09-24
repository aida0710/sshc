import type { VPNProfile } from "../api/vpn";
import { isUnroutableIPv4, joinHostPort, parseAddress, parseEndpoint, parsePrefix, type Endpoint } from "./vpnAddressSyntax";
import { isVPNBackend } from "./vpnBackends";
import type { VPNFieldError, VPNFieldReason } from "./vpnFieldErrors";

// VPN プロファイルの設定（シークレットを除く）を送る前に、engine と同じ規則で確かめる。
//
// 規則の正本は Go（internal/application の VPNProfile.Profile と internal/vpn の検査）に
// あり、ここはその写しである。同じ入力に同じ項目と理由を返すことを、
// internal/vpn/testdata/profile-cases.json に対するテストで保つ。どちらかだけを変えると
// そのテストが落ちる。検査の順番も Go と同じにする。最初に断られる項目が変わるからである。
//
// Go は len で UTF-8 のバイト数を数えるので、長さはここもバイト数で数える。API の
// maxLength（送る前の検査は UTF-16 の長さで数える）より厳しいか同じなので、ここを
// 通った値は送る前の検査でも断られない。

// maxProfileNameLength は、コンテナ名とソケットのパスに入る長さである。
const maxProfileNameLength = 48;
// maxResolvers は、1つの経路が使う DNS サーバーの数の上限である。
const maxResolvers = 3;
// WireGuard の鍵は32バイトの base64 で、`=` で終わる44文字になる。
const wireGuardKeyLength = 44;
const maxPort = 65535;
// 次の上限は、Go（internal/vpn/validation.go）と API（api/openapi.yaml の VPNProfile）と
// 同じ値である。
const maxServerLength = 320;
const maxUsernameLength = 256;
const maxProposalLength = 256;
const maxFingerprintLength = 128;
const maxApprovalWordLength = 32;

// openConnectProtocols は、openconnect の --protocol に渡してよいプロトコルである。
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

function validateLength(field: string, value: string, limit: number): Outcome {
  return utf8Length(value) > limit ? { field, reason: "too_long", limit } : null;
}

// vpnProfileNameError は、プロファイル名として使えないなら、その理由を返す。
// 作成と「名前を変更」の両方が使う。
export function vpnProfileNameError(name: string): VPNFieldError | null {
  if (name === "") return refuse("name", "required");
  const tooLong = validateLength("name", name, maxProfileNameLength);
  if (tooLong !== null) return tooLong;
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

function validateResolvers(resolvers: string[]): Outcome {
  if (resolvers.length > maxResolvers) return { field: "dns", reason: "too_many", limit: maxResolvers };
  for (const resolver of resolvers) {
    const address = parseAddress(resolver);
    if (address === null || address.version !== 4) return refuse("dns", "not_ipv4");
    if (isUnroutableIPv4(address.octets)) return refuse("dns", "unroutable");
  }
  return null;
}

function validateServerName(field: string, server: string): Outcome {
  if (server === "") return refuse(field, "required");
  return validateLength(field, server, maxServerLength) ?? (serverForbidden.test(server) ? refuse(field, "format") : null);
}

function validateUsername(field: string, username: string): Outcome {
  if (username === "") return refuse(field, "required");
  return validateLength(field, username, maxUsernameLength) ??
    (usernameForbidden.test(username) ? refuse(field, "format") : null);
}

// vpnWireGuardKeyError は、WireGuard の鍵（相手の公開鍵、自分の秘密鍵）が base64 の
// 32バイトでなければ、その理由を返す。
export function vpnWireGuardKeyError(field: string, key: string): VPNFieldError | null {
  if (key === "") return refuse(field, "required");
  if (key.length !== wireGuardKeyLength || !key.endsWith("=")) return refuse(field, "format");
  const body = key.slice(0, -1);
  const base64 = [...body].every((character) => isASCIIAlphanumeric(character) || character === "+" || character === "/");
  return base64 ? null : refuse(field, "format");
}

function validateWireGuard(settings: NonNullable<VPNProfile["wireguard"]>, server: Endpoint): Outcome {
  if (server.host === "") return refuse("wireguard.server", "required");
  const tooLong = validateLength("wireguard.server", joinHostPort(server.host, server.port), maxServerLength);
  if (tooLong !== null) return tooLong;
  if (whitespace.test(server.host)) return refuse("wireguard.server", "format");
  const port = validatePort("wireguard.server", server.port);
  if (port !== null) return port;
  const key = vpnWireGuardKeyError("wireguard.peerPublicKey", settings.peerPublicKey);
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
  for (const [field, value] of [["l2tp.ike", settings.ike ?? ""], ["l2tp.esp", settings.esp ?? ""]] as const) {
    const refused = validateLength(field, value, maxProposalLength) ??
      (serverForbidden.test(value) ? refuse(field, "format") : null);
    if (refused !== null) return refused;
  }
  return null;
}

function validateFingerprint(fingerprint: string): Outcome {
  if (fingerprint === "") return null;
  const field = "openconnect.serverCertificate";
  const tooLong = validateLength(field, fingerprint, maxFingerprintLength);
  if (tooLong !== null) return tooLong;
  const known = openConnectFingerprintPrefixes.some((prefix) => fingerprint.startsWith(prefix));
  return !known || serverForbidden.test(fingerprint) ? refuse(field, "format") : null;
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
  const approvalWord = settings.approvalWord ?? "";
  const tooLong = validateLength("openconnect.approvalWord", approvalWord, maxApprovalWordLength);
  if (tooLong !== null) return tooLong;
  // 答えは1行として送る。改行が混じると、サーバーが受け取る問答がずれる。
  return whitespace.test(approvalWord) ? refuse("openconnect.approvalWord", "format") : null;
}

function validateBackendSettings(profile: VPNProfile, wireGuardServer: Endpoint | null): Outcome {
  switch (profile.backend) {
    case "wireguard":
      return profile.wireguard === undefined || wireGuardServer === null
        ? refuse("wireguard", "required")
        : validateWireGuard(profile.wireguard, wireGuardServer);
    case "l2tp_ipsec":
      return profile.l2tp === undefined ? refuse("l2tp", "required") : validateL2TP(profile.l2tp);
    case "openconnect":
      return profile.openconnect === undefined ? refuse("openconnect", "required") : validateOpenConnect(profile.openconnect);
  }
}

// vpnProfileFieldError は、engine がこのプロファイルを断るなら、最初に断る項目と理由を
// 返す。通るなら null を返す。
export function vpnProfileFieldError(profile: VPNProfile): VPNFieldError | null {
  // backend と違う節は、保存するときに落とすので見ない（Go の Normalized と同じ）。
  // WireGuard のサーバーは、ほかの検査より前に `host:port` として読む（Go と同じ）。
  const wireGuard = profile.backend === "wireguard" ? profile.wireguard : undefined;
  let wireGuardServer: Endpoint | null = null;
  if (wireGuard !== undefined) {
    wireGuardServer = parseEndpoint(wireGuard.server);
    if (wireGuardServer === null) return refuse("wireguard.server", "format");
  }
  return (
    vpnProfileNameError(profile.name) ??
    (isVPNBackend(profile.backend) ? null : refuse("backend", "unsupported")) ??
    validateResolvers(profile.dns ?? []) ??
    validateBackendSettings(profile, wireGuardServer)
  );
}
