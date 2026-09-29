import { useMemo } from "react";
import { useTranslate } from "../i18n/context";
import { Field, control, hintText } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { ConfigFileField } from "./ConfigFileField";
import { inspectOpenVPNConfig } from "./openVPNConfig";
import { describeVPNFieldError } from "./vpnFieldErrors";

// VPN プロファイルのフォームのうち、OpenVPN の欄。設定ファイル（.ovpn）を貼り付けるか
// ファイルから読み込み、remote のサーバーと、使えない指示をその場で見せる。
//
// 設定ファイルはシークレットで、Vault に置く。編集では、保存済みの設定ファイルを取り出して
// この欄に入れる。取り出せなかったときは空欄で、空欄のまま保存すれば保存済みの設定ファイルを
// 使う。

export function OpenVPNProfileFields({
  config,
  servers,
  username,
  password,
  keepsConfig,
  keepsPassword,
  errorFor,
  onConfig,
  onUsername,
  onPassword,
}: {
  config: string;
  // servers は、設定ファイルの remote のサーバーである（保存済みのものか、読み込んだもの）。
  servers: string[];
  username: string;
  password: string;
  // keepsConfig と keepsPassword は、空欄なら保存済みの値を使うことを表す。
  keepsConfig: boolean;
  keepsPassword: boolean;
  // errorFor は、engine が断った項目の理由を返す。
  errorFor: (field: string) => string | undefined;
  onConfig: (config: string) => void;
  onUsername: (username: string) => void;
  onPassword: (password: string) => void;
}) {
  const t = useTranslate();
  const inspection = useMemo(() => (config === "" ? null : inspectOpenVPNConfig(config)), [config]);
  // 使えない指示は、保存を押す前に、貼り付けた時点で見せる。
  const inspectionError = inspection !== null && "refusal" in inspection
    ? describeVPNFieldError(t, inspection.refusal)
    : undefined;
  const asksCredentials = inspection !== null && "summary" in inspection && inspection.summary.asksCredentials;

  return (
    <>
      <ConfigFileField
        label={t("vpn.openVPNConfig")}
        hint={t(keepsConfig && config === "" ? "vpn.configFileKeepHint" : "vpn.openVPNConfigHint")}
        error={errorFor("secrets.openvpnConfig") ?? errorFor("openvpn.servers") ?? inspectionError}
        value={config}
        accept=".ovpn,.conf,text/plain"
        onChange={onConfig}
        summary={
          <>
            {servers.length === 0 ? null : (
              <span className={hintText}>{t("vpn.openVPNServers", { servers: servers.join(", ") })}</span>
            )}
            {asksCredentials ? <span className={hintText}>{t("vpn.openVPNAsksCredentials")}</span> : null}
          </>
        }
      />
      <Field label={t("vpn.username")} hint={t("vpn.openVPNUsernameHint")} error={errorFor("openvpn.username")}>
        <input className={control} value={username} onChange={(event) => onUsername(event.target.value)} />
      </Field>
      <PasswordField
        label={t("vpn.password")}
        hint={t(keepsPassword && password === "" ? "vpn.secretKeepHint" : "vpn.openVPNPasswordHint")}
        error={errorFor("secrets.openvpnPassword")}
        value={password}
        onChange={onPassword}
      />
    </>
  );
}
