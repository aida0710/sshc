import { useMemo } from "react";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { ConfigFileField } from "./ConfigFileField";
import { describeVPNFieldError } from "./vpnFieldErrors";
import { inspectWireGuardConfig, wireGuardServers } from "./wireGuardConfig";

// VPN プロファイルのフォームのうち、WireGuard の欄。設定ファイル（wg-quick の形）を鍵も含めて
// そのまま編集させ、Endpoint のサーバーと DNS と、使えない項目をその場で見せる。
//
// 設定ファイルは全体がシークレットで、Vault に置く。編集では、保存済みの設定ファイルを取り出して
// この欄に入れる。取り出せなかったときは空欄で、空欄のまま保存すれば保存済みの設定ファイルを使う。

// configExample は、新しく作るときに空の欄に見せる書き方の例である。
const configExample = [
  "[Interface]",
  "PrivateKey = …",
  "Address = 10.0.0.2/32",
  "DNS = 10.0.0.1",
  "",
  "[Peer]",
  "PublicKey = …",
  "Endpoint = vpn.example.jp:51820",
  "AllowedIPs = 10.0.0.0/24",
].join("\n");

export function WireGuardProfileFields({
  config,
  servers,
  keepsConfig,
  errorFor,
  onConfig,
}: {
  config: string;
  // servers は、設定ファイルの Endpoint のサーバーである（保存済みのものか、読み込んだもの）。
  servers: string[];
  // keepsConfig は、空欄なら保存済みの設定ファイルを使うことを表す。
  keepsConfig: boolean;
  // errorFor は、engine か送る前の検査が断った項目の理由を返す。
  errorFor: (field: string) => string | undefined;
  onConfig: (config: string) => void;
}) {
  const t = useTranslate();
  const inspection = useMemo(() => (config === "" ? null : inspectWireGuardConfig(config)), [config]);
  // 使えない項目は、保存を押す前に、書いた時点で見せる。
  const inspectionError = inspection !== null && "refusal" in inspection
    ? describeVPNFieldError(t, inspection.refusal)
    : undefined;
  const read = inspection !== null && "config" in inspection ? inspection.config : null;
  const shownServers = read === null ? servers : wireGuardServers(read);

  return (
    <ConfigFileField
      label={t("vpn.wireGuardConfig")}
      hint={t(keepsConfig && config === "" ? "vpn.configFileKeepHint" : "vpn.wireGuardConfigHint")}
      // サーバーと DNS は設定ファイルから読むので、読めない設定ファイルの理由を先に見せる。
      error={errorFor("secrets.wireguardConfig") ?? inspectionError ?? errorFor("wireguard.servers") ?? errorFor("dns")}
      value={config}
      accept=".conf,text/plain"
      placeholder={configExample}
      onChange={onConfig}
      summary={
        <>
          {shownServers.length === 0 ? null : (
            <span className={hintText}>{t("vpn.wireGuardServers", { servers: shownServers.join(", ") })}</span>
          )}
          {read === null || read.dns.length === 0 ? null : (
            <span className={hintText}>{t("vpn.wireGuardDNS", { dns: read.dns.join(", ") })}</span>
          )}
        </>
      }
    />
  );
}
