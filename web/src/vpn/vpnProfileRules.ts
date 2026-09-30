import type { VPNProfile } from "../api/vpn";
import { isUnroutableIPv4, parseAddress, parsePrefix } from "./vpnAddressSyntax";
import { isVPNBackend, settingsSection } from "./vpnBackends";
import { isPEMCertificateList } from "./vpnCertificateSyntax";
import type { VPNFieldError, VPNFieldReason } from "./vpnFieldErrors";
import { utf8Length } from "./utf8Length";

// VPN プロファイルの設定（シークレットを除く）を送る前に、engine と同じ規則で確かめる。
//
// 規則の正本は Go（internal/application の VPNProfile.Profile と internal/vpn の検査）に
// あり、ここはその写しである。同じ入力に同じ項目と理由を返すことを、
// internal/vpn/testdata/profile-cases.json に対するテストで保つ。どちらかだけを変えると
// そのテストが落ちる。検査の順番も Go と同じにする。最初に断られる項目が変わるからである。
//
// Go は名前のほかは len で UTF-8 のバイト数を数えるので、長さはここもバイト数で数える。
// API の maxLength（文字数で数える）より厳しいか同じなので、ここを通った値は送る前の
// 検査でも断られない。名前だけは、Go も API も文字数で数える。

// maxProfileNameLength は、プロファイル名の上限（文字数）である。一覧の1行に収まる長さで、
// Go と API と同じ値である。名前はコンテナ名やパスには入らないので、字の種類は問わない。
const maxProfileNameLength = 48;
// maxResolvers は、1つの経路が使う DNS サーバーの数の上限である。
const maxResolvers = 3;
// 次の上限は、Go（internal/vpn/validation.go）と API（api/openapi.yaml の VPNProfile）と
// 同じ値である。
const maxServerLength = 320;
const maxUsernameLength = 256;
const maxProposalLength = 256;
const maxFingerprintLength = 128;
const maxApprovalWordLength = 32;
// maxConfigServers は、設定ファイルから読んだサーバー（OpenVPN の remote、WireGuard の
// Endpoint）を、プロファイルに持つ数の上限である（Go と API と同じ値）。
const maxConfigServers = 64;
// maxCACertificateLength は、IKEv2 の CA の証明書（PEM）の上限である（Go と API と同じ値）。
const maxCACertificateLength = 16384;

// openConnectProtocols は、openconnect の --protocol に渡してよいプロトコルである。
export const openConnectProtocols = ["anyconnect", "nc", "pulse", "gp", "f5", "fortinet", "array"] as const;

// openConnectProducts は、プロトコルごとに、そのプロトコルを話す製品の名前である。
// 利用者が知っているのは製品の名前で、openconnect のプロトコル名ではない。
// 製品名は固有名詞で言語によって変わらないので、i18n の文言にはしない。
export const openConnectProducts: Record<(typeof openConnectProtocols)[number], string> = {
  anyconnect: "Cisco AnyConnect / Secure Client, ocserv",
  nc: "Juniper Network Connect",
  pulse: "Ivanti Connect Secure (Pulse Secure)",
  gp: "Palo Alto Networks GlobalProtect",
  f5: "F5 BIG-IP",
  fortinet: "Fortinet FortiGate",
  array: "Array Networks",
};
const openConnectFingerprintPrefixes = ["sha256:", "pin-sha256:"];
const secondFactors = ["", "approve", "totp"];
// ikev2Authentications は、IKEv2 でこちらを認証する方式である（engine と同じ語）。
export const ikev2Authentications = ["eap-mschapv2", "psk"] as const;
export type IKEv2Authentication = (typeof ikev2Authentications)[number];

// 設定ファイルとコマンド引数へそのまま書けない字である。
const serverForbidden = /[ \t\r\n"\\]/;
const usernameForbidden = /[\r\n"\\]/;
const whitespace = /[ \t\r\n]/;

type Outcome = VPNFieldError | null;

function refuse(field: string, reason: VPNFieldReason): VPNFieldError {
  return { field, reason };
}

function validateLength(field: string, value: string, limit: number): Outcome {
  return utf8Length(value) > limit ? { field, reason: "too_long", limit } : null;
}

// 名前に使えない字である。Go の unicode.IsGraphic の外（制御文字、書式の字、私用の字、
// 行と段落の区切り）と、API のパスの区切りと読まれうる / と \ である。
const profileNameForbidden = /[\p{Cc}\p{Cf}\p{Cs}\p{Co}\p{Cn}\p{Zl}\p{Zp}/\\]/u;
// 名前の前後に置けない空白である。前後の空白は、見た目では同じ名前を2つ作る。
const surroundingWhitespace = /^\s|\s$/u;

// vpnProfileNameError は、プロファイル名として使えないなら、その理由を返す。
// 作成と「名前を変更」の両方が使う。長さは Go と同じく文字数（コードポイントの数）で数える。
export function vpnProfileNameError(name: string): VPNFieldError | null {
  if (name === "") return refuse("name", "required");
  if ([...name].length > maxProfileNameLength) return { field: "name", reason: "too_long", limit: maxProfileNameLength };
  if (profileNameForbidden.test(name) || surroundingWhitespace.test(name)) return refuse("name", "format");
  return null;
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

// validateConfigServers は、設定ファイルから読んだサーバーの並びを確かめる。field は項目の
// JSON パスである（openvpn.servers、wireguard.servers）。
function validateConfigServers(field: string, servers: string[]): Outcome {
  if (servers.length === 0) return refuse(field, "required");
  if (servers.length > maxConfigServers) return { field, reason: "too_many", limit: maxConfigServers };
  for (const server of servers) {
    const refused = validateServerName(field, server);
    if (refused !== null) return refused;
  }
  return null;
}

// validateProposals は、IKE と ESP の暗号スイートを確かめる。section は節の名前である。
function validateProposals(section: string, ike: string, esp: string): Outcome {
  for (const [field, value] of [[`${section}.ike`, ike], [`${section}.esp`, esp]] as const) {
    const refused = validateLength(field, value, maxProposalLength) ??
      (serverForbidden.test(value) ? refuse(field, "format") : null);
    if (refused !== null) return refused;
  }
  return null;
}

function validateL2TP(settings: NonNullable<VPNProfile["l2tp"]>): Outcome {
  const server = validateServerName("l2tp.server", settings.server);
  if (server !== null) return server;
  const username = validateUsername("l2tp.username", settings.username);
  if (username !== null) return username;
  return validateProposals("l2tp", settings.ike ?? "", settings.esp ?? "");
}

// identityRangeTypes は、strongSwan がアドレスの網や範囲として読む ID の型の接頭辞である。
const identityRangeTypes = ["ipv4net:", "ipv6net:", "ipv4range:", "ipv6range:"];

// matchesManyServers は、Go の vpn.ServerIdentityMatchesManyServers と同じく、strongSwan が
// どの ID にも、または複数の ID に一致すると読む値かを返す（`%any`、`*` を含む値、未指定の
// アドレス、アドレスの網と範囲）。
function matchesManyServers(identity: string): boolean {
  if (identity.startsWith("%") || identity.includes("*")) return true;
  const lowered = identity.toLowerCase();
  if (identityRangeTypes.some((prefix) => lowered.startsWith(prefix))) return true;
  const address = lowered.replace(/^ipv4:/, "").replace(/^ipv6:/, "");
  const parsed = parseAddress(address);
  if (parsed !== null && parsed.zone === "" && parsed.octets.every((octet) => octet === 0)) return true;
  if (parsePrefix(address) !== null) return true;
  const dash = address.indexOf("-");
  return dash >= 0 && parseAddress(address.slice(0, dash)) !== null && parseAddress(address.slice(dash + 1)) !== null;
}

// validateServerIdentity は、サーバーの ID を確かめる。ひとつのサーバーに決まらない値は
// 断る。認証局の証明書を持つ誰とでも繋いでしまうからである。
function validateServerIdentity(identity: string): Outcome {
  if (identity === "") return null;
  const field = "ikev2.serverIdentity";
  return validateLength(field, identity, maxUsernameLength) ??
    (usernameForbidden.test(identity) || matchesManyServers(identity) ? refuse(field, "format") : null);
}

function validateCACertificate(settings: NonNullable<VPNProfile["ikev2"]>): Outcome {
  const certificate = settings.caCertificate ?? "";
  if (certificate === "") return null;
  const field = "ikev2.caCertificate";
  if (settings.authentication === "psk") return refuse(field, "unexpected");
  return validateLength(field, certificate, maxCACertificateLength) ??
    (isPEMCertificateList(certificate) ? null : refuse(field, "format"));
}

function validateIKEv2(settings: NonNullable<VPNProfile["ikev2"]>): Outcome {
  const server = validateServerName("ikev2.server", settings.server);
  if (server !== null) return server;
  if (settings.authentication === "") return refuse("ikev2.authentication", "required");
  if (!(ikev2Authentications as readonly string[]).includes(settings.authentication)) {
    return refuse("ikev2.authentication", "unsupported");
  }
  return (
    validateUsername("ikev2.identity", settings.identity) ??
    validateServerIdentity(settings.serverIdentity ?? "") ??
    validateCACertificate(settings) ??
    validateProposals("ikev2", settings.ike ?? "", settings.esp ?? "")
  );
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

function validateOpenVPN(settings: NonNullable<VPNProfile["openvpn"]>): Outcome {
  const servers = validateConfigServers("openvpn.servers", settings.servers);
  if (servers !== null) return servers;
  // ユーザー名は任意である。書いたときだけ形を確かめる。
  const username = settings.username ?? "";
  return username === "" ? null : validateUsername("openvpn.username", username);
}

function validateBackendSettings(profile: VPNProfile): Outcome {
  switch (profile.backend) {
    case "wireguard":
      return profile.wireguard === undefined
        ? refuse("wireguard", "required")
        : validateConfigServers("wireguard.servers", profile.wireguard.servers);
    case "l2tp_ipsec":
      return profile.l2tp === undefined ? refuse("l2tp", "required") : validateL2TP(profile.l2tp);
    case "openconnect":
      return profile.openconnect === undefined ? refuse("openconnect", "required") : validateOpenConnect(profile.openconnect);
    case "openvpn":
      return profile.openvpn === undefined ? refuse("openvpn", "required") : validateOpenVPN(profile.openvpn);
    case "ikev2":
      return profile.ikev2 === undefined ? refuse("ikev2", "required") : validateIKEv2(profile.ikev2);
  }
}

// validateForeignSection は、backend と違う方式の節があれば、その節を断る。画面は
// 選んだ方式の節だけを送る（profileOf）が、engine と同じ答えを返すために確かめる。
function validateForeignSection(profile: VPNProfile): Outcome {
  const own = settingsSection[profile.backend];
  const foreign = Object.values(settingsSection).find(
    (section) => section !== own && profile[section] !== undefined,
  );
  return foreign === undefined ? null : refuse(foreign, "unexpected");
}

// vpnProfileFieldError は、engine がこのプロファイルを断るなら、最初に断る項目と理由を
// 返す。通るなら null を返す。
export function vpnProfileFieldError(profile: VPNProfile): VPNFieldError | null {
  return (
    vpnProfileNameError(profile.name) ??
    (isVPNBackend(profile.backend) ? null : refuse("backend", "unsupported")) ??
    validateResolvers(profile.dns ?? []) ??
    validateForeignSection(profile) ??
    validateBackendSettings(profile)
  );
}
