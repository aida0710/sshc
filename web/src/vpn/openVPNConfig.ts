import type { VPNFieldError, VPNFieldReason } from "./vpnFieldErrors";
import { utf8Length } from "./utf8Length";

// OpenVPN の設定ファイル（.ovpn）を、送る前に engine と同じ規則で確かめ、remote の
// サーバーを取り出す。
//
// 規則の正本は Go（internal/vpn/openvpn_config.go の InspectOpenVPNConfig）にあり、ここは
// その写しである。同じ入力に同じ行と理由を返すことを、
// internal/vpn/testdata/openvpn-config-cases.json に対するテストで保つ。読み方は OpenVPN 2.6
// の parse_line と同じにする。読み方がずれると、画面が通した設定ファイルを engine が断る。

// OpenVPNConfigSummary は、設定ファイルから読み取った、シークレットでない事実である。
export type OpenVPNConfigSummary = {
  // servers は、remote のサーバーを、書かれた順に重複を除いて並べたものである。
  servers: string[];
  // asksCredentials は、引数の無い auth-user-pass が有効で、ユーザー名とパスワードが要ることを表す。
  asksCredentials: boolean;
};

export type OpenVPNConfigInspection = { summary: OpenVPNConfigSummary } | { refusal: VPNFieldError };

const configField = "secrets.openvpnConfig";

// maxLineLength は、1行の長さの上限（改行を除く UTF-8 のバイト数）である。OpenVPN は
// それより長い行を途中で切って、続きを次の行として読む。
const maxLineLength = 254;
// maxParameters は、1行から読む語の数の上限である（OpenVPN の MAX_PARMS）。
const maxParameters = 16;
// maxRemotes は、remote の数の上限である（OpenVPN の CONNECTION_LIST_SIZE）。
const maxRemotes = 64;
const maxServerLength = 320;
const serverForbidden = /[ \t\r\n"\\]/;
// openVPNSpaces は、OpenVPN が空白とみなす字である（C の isspace と NUL）。
const openVPNSpaces = " \t\n\v\f\r\0";
const managementPrefix = "management";
const tunDevice = /^tun[0-9]*$/;
const httpProxyAutoCredentials = new Set(["auto", "auto-nct"]);

// refusedDirectives は、引数に関係なく断る指示と、その理由である（Go の refusedOpenVPNDirectives）。
// 設定ファイルの語で引くので Map にする。オブジェクトだと constructor などがプロトタイプから当たる。
const refusedDirectives = new Map<string, VPNFieldReason>(Object.entries({
  up: "runs_command", down: "runs_command", "route-up": "runs_command", "route-pre-down": "runs_command",
  ipchange: "runs_command", "tls-verify": "runs_command", "client-connect": "runs_command",
  "client-disconnect": "runs_command", "client-crresponse": "runs_command", "learn-address": "runs_command",
  "auth-user-pass-verify": "runs_command", "tls-crypt-v2-verify": "runs_command", iproute: "runs_command",
  plugin: "runs_command", engine: "runs_command", "pkcs11-providers": "runs_command", "script-security": "runs_command",
  route: "changes_routes", "route-ipv6": "changes_routes", "redirect-gateway": "changes_routes",
  "redirect-private": "changes_routes", "client-nat": "changes_routes",
  "dev-node": "decided_by_sshc", lladdr: "decided_by_sshc", mktun: "decided_by_sshc", rmtun: "decided_by_sshc",
  daemon: "decided_by_sshc", log: "decided_by_sshc", "log-append": "decided_by_sshc", syslog: "decided_by_sshc",
  status: "decided_by_sshc", writepid: "decided_by_sshc", "tmp-dir": "decided_by_sshc", chroot: "decided_by_sshc",
  cd: "decided_by_sshc", user: "decided_by_sshc", group: "decided_by_sshc", setcon: "decided_by_sshc",
  askpass: "decided_by_sshc",
  config: "file_reference", capath: "file_reference", "tls-export-cert": "file_reference",
  "replay-persist": "file_reference", genkey: "file_reference",
  mode: "server_mode", server: "server_mode", "server-ipv6": "server_mode", "server-bridge": "server_mode",
  "tls-server": "server_mode",
} satisfies Record<string, VPNFieldReason>));

// inlineDirectives は、インラインのブロックで書ける指示である。true のものは、ファイル名で
// ない引数も受け付ける（Go の openVPNInlineDirectives）。
const inlineDirectives = new Map<string, boolean>(Object.entries({
  ca: false, cert: false, dh: false, "extra-certs": false, key: false, pkcs12: false, secret: false,
  "crl-verify": false, "http-proxy-user-pass": false, "tls-auth": false, "auth-gen-token-secret": false,
  "tls-crypt": false, "tls-crypt-v2": false, "peer-fingerprint": true, "verify-hash": true, "auth-user-pass": false,
}));

function refuse(reason: VPNFieldReason, line: number, directive = "", limit?: number): OpenVPNConfigInspection {
  return {
    refusal: {
      field: configField, reason, line,
      ...(directive === "" ? {} : { directive }),
      ...(limit === undefined ? {} : { limit }),
    },
  };
}

function isSpace(character: string): boolean {
  return character !== "" && openVPNSpaces.includes(character);
}

// splitLine は、1行を語に分ける（OpenVPN の parse_line）。読めない行は null を返す。
export function splitLine(line: string): string[] | null {
  const words: string[] = [];
  let word = "";
  let state: "between" | "unquoted" | "double" | "single" = "between";
  let backslash = false;
  // 行の終わりは NUL として扱い、読みかけの語を閉じる。
  for (let index = 0; index <= line.length; index++) {
    const input = index < line.length ? line.charAt(index) : "\0";
    if (!backslash && input === "\\" && state !== "single") {
      backslash = true;
      continue;
    }
    let output = "";
    let done = false;
    switch (state) {
      case "between":
        if (!isSpace(input)) {
          if (input === ";" || input === "#") return words;
          if (!backslash && input === "\"") state = "double";
          else if (!backslash && input === "'") state = "single";
          else {
            output = input;
            state = "unquoted";
          }
        }
        break;
      case "unquoted":
        if (!backslash && isSpace(input)) done = true;
        else output = input;
        break;
      case "double":
        if (!backslash && input === "\"") done = true;
        else output = input;
        break;
      case "single":
        if (input === "'") done = true;
        else output = input;
        break;
    }
    if (done) {
      words.push(word);
      word = "";
      state = "between";
    }
    // NUL は語に入らない（C の文字列の終わり）。
    if (output === "\0") output = "";
    if (backslash && output !== "" && output !== "\\" && output !== "\"" && !isSpace(output)) return null;
    backslash = false;
    word += output;
    if (words.length >= maxParameters) return words;
  }
  return state === "between" ? words : null;
}

function withoutDoubleDash(word: string): string {
  return word.length >= 3 && word.startsWith("--") ? word.slice(2) : word;
}

// withoutSetenvOpt は、`setenv opt` の頭を外す。OpenVPN はこの頭の付いた指示もふつうの指示として使う。
function withoutSetenvOpt(words: string[]): string[] {
  let rest = words;
  while (rest.length >= 2 && rest[0] === "setenv" && rest[1] === "opt") {
    const [name, ...args] = rest.slice(2);
    rest = name === undefined ? [] : [withoutDoubleDash(name), ...args];
  }
  return rest;
}

function inlineTag(words: string[]): string | null {
  const [word] = words;
  if (words.length !== 1 || word === undefined) return null;
  return word.length >= 2 && word.startsWith("<") && word.endsWith(">") ? word.slice(1, -1) : null;
}

function isValidServer(server: string): boolean {
  return server !== "" && utf8Length(server) <= maxServerLength && !serverForbidden.test(server);
}

// directiveReason は、指示を断るなら、その理由を返す（Go の openVPNDirectiveReason）。
function directiveReason(name: string, args: string[]): VPNFieldReason | null {
  const listed = refusedDirectives.get(name);
  if (listed !== undefined) return listed;
  if (name.startsWith(managementPrefix)) return "decided_by_sshc";
  const [first, , third] = args;
  switch (name) {
    case "dev":
      return first !== undefined && !tunDevice.test(first) ? "decided_by_sshc" : null;
    case "dev-type":
      return first !== undefined && first !== "tun" ? "decided_by_sshc" : null;
    case "http-proxy":
      // http-proxy <サーバー> <ポート> [<ファイル>|auto|auto-nct] [<認証方式>]
      return third !== undefined && !httpProxyAutoCredentials.has(third) ? "file_reference" : null;
    case "socks-proxy":
      // socks-proxy <サーバー> [<ポート>] [<ファイル>]
      return third !== undefined ? "file_reference" : null;
  }
  const acceptsValue = inlineDirectives.get(name);
  return acceptsValue === false && args.length > 0 ? "file_reference" : null;
}

type Reader = {
  lines: string[];
  summary: OpenVPNConfigSummary;
  isClient: boolean;
  remotes: number;
};

function closingLine(reader: Reader, tag: string, from: number, end: number): number {
  const closing = `</${tag}>`;
  for (let index = from; index < end; index++) {
    if ((reader.lines[index] ?? "").replace(/^[ \t\n\v\f\r]+/, "").startsWith(closing)) return index;
  }
  return -1;
}

function readDirective(reader: Reader, allWords: string[], lineNumber: number): OpenVPNConfigInspection | null {
  const words = withoutSetenvOpt(allWords);
  if (words.length === 0) return null;
  const [name = "", ...args] = words;
  const reason = directiveReason(name, args);
  if (reason !== null) return refuse(reason, lineNumber, name);
  switch (name) {
    case "client":
    case "tls-client":
      reader.isClient = true;
      return null;
    case "auth-user-pass":
      reader.summary.asksCredentials = true;
      return null;
    case "remote": {
      const [server] = args;
      if (server === undefined || !isValidServer(server)) return refuse("format", lineNumber, "remote");
      reader.remotes++;
      if (reader.remotes > maxRemotes) return refuse("too_many", lineNumber, "remote", maxRemotes);
      if (!reader.summary.servers.includes(server)) reader.summary.servers.push(server);
      return null;
    }
  }
  return null;
}

function readInline(reader: Reader, tag: string, open: number, close: number): OpenVPNConfigInspection | null {
  const lineNumber = open + 1;
  const listed = refusedDirectives.get(tag);
  if (listed !== undefined) return refuse(listed, lineNumber, tag);
  if (tag === "connection") return readBlock(reader, open + 1, close);
  if (!inlineDirectives.has(tag)) return refuse("unsupported_inline", lineNumber);
  if (tag === "auth-user-pass") reader.summary.asksCredentials = false;
  return null;
}

function readBlock(reader: Reader, start: number, end: number): OpenVPNConfigInspection | null {
  for (let index = start; index < end; index++) {
    const lineNumber = index + 1;
    const split = splitLine(reader.lines[index] ?? "");
    if (split === null) return refuse("format", lineNumber);
    const [name, ...args] = split;
    if (name === undefined) continue;
    const words = [withoutDoubleDash(name), ...args];
    const tag = inlineTag(words);
    if (tag === null) {
      const refused = readDirective(reader, words, lineNumber);
      if (refused !== null) return refused;
      continue;
    }
    const closing = closingLine(reader, tag, index + 1, end);
    if (closing < 0) return refuse("unclosed_inline", lineNumber);
    const refused = readInline(reader, tag, index, closing);
    if (refused !== null) return refused;
    index = closing;
  }
  return null;
}

function splitConfigLines(config: string): string[] {
  const lines = (config.startsWith("\uFEFF") ? config.slice(1) : config).split("\n");
  if (lines.length > 0 && lines[lines.length - 1] === "") lines.pop();
  return lines;
}

// inspectOpenVPNConfig は、設定ファイルを確かめ、使えれば読み取った事実を、使えなければ
// 最初に断る行と理由を返す。
export function inspectOpenVPNConfig(config: string): OpenVPNConfigInspection {
  const reader: Reader = { lines: splitConfigLines(config), summary: { servers: [], asksCredentials: false }, isClient: false, remotes: 0 };
  for (const [index, line] of reader.lines.entries()) {
    if (utf8Length(line) > maxLineLength) return refuse("too_long", index + 1, "", maxLineLength);
    if (line.includes("\0")) return refuse("format", index + 1);
  }
  const refused = readBlock(reader, 0, reader.lines.length);
  if (refused !== null) return refused;
  if (!reader.isClient) return refuse("not_client", 0);
  if (reader.remotes === 0) return refuse("no_remote", 0);
  return { summary: reader.summary };
}
