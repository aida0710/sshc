import { useMemo, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Field, control, hintText } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button } from "../ui/surface";
import { inspectOpenVPNConfig } from "./openVPNConfig";
import { describeVPNFieldError } from "./vpnFieldErrors";

// VPN プロファイルのフォームのうち、OpenVPN の欄。設定ファイル（.ovpn）を貼り付けるか
// ファイルから読み込み、remote のサーバーと、使えない指示をその場で見せる。
//
// 設定ファイルはシークレットで、engine は返さない。編集では、空欄なら保存済みの設定ファイルを
// そのまま使う。

// configRows は、設定ファイルの欄の高さ（行数）である。remote とインラインのブロックの
// 始まりが見える程度にする。
const configRows = 8;

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
  const chooser = useRef<HTMLInputElement>(null);
  const [readFailed, setReadFailed] = useState(false);
  const inspection = useMemo(() => (config === "" ? null : inspectOpenVPNConfig(config)), [config]);
  // 使えない指示は、保存を押す前に、貼り付けた時点で見せる。
  const inspectionError = inspection !== null && "refusal" in inspection
    ? describeVPNFieldError(t, inspection.refusal)
    : undefined;
  const asksCredentials = inspection !== null && "summary" in inspection && inspection.summary.asksCredentials;

  function edit(next: string) {
    setReadFailed(false);
    onConfig(next);
  }

  async function load(file: File) {
    try {
      edit(await file.text());
    } catch {
      setReadFailed(true);
    }
  }

  return (
    <>
      <div className="flex flex-col gap-2 sm:col-span-2">
        <Field
          label={t("vpn.openVPNConfig")}
          hint={t(keepsConfig ? "vpn.openVPNConfigKeepHint" : "vpn.openVPNConfigHint")}
          error={readFailed
            ? t("vpn.openVPNConfigReadFailed")
            : errorFor("secrets.openvpnConfig") ?? errorFor("openvpn.servers") ?? inspectionError}
        >
          <textarea
            className={`${control} font-mono text-xs`}
            rows={configRows}
            spellCheck={false}
            autoComplete="off"
            value={config}
            onChange={(event) => edit(event.target.value)}
          />
        </Field>
        <div className="flex flex-wrap items-center gap-3">
          <input
            ref={chooser}
            type="file"
            className="hidden"
            accept=".ovpn,.conf,text/plain"
            onChange={(event) => {
              const file = event.target.files?.[0];
              event.target.value = "";
              if (file !== undefined) void load(file);
            }}
          />
          <Button className="inline-flex items-center gap-1.5" onClick={() => chooser.current?.click()}>
            <Icon name="openFile" />
            {t("vpn.openVPNConfigChoose")}
          </Button>
          {servers.length === 0 ? null : (
            <span className={hintText}>{t("vpn.openVPNServers", { servers: servers.join(", ") })}</span>
          )}
        </div>
        {asksCredentials ? <p className={hintText}>{t("vpn.openVPNAsksCredentials")}</p> : null}
      </div>
      <Field label={t("vpn.username")} hint={t("vpn.openVPNUsernameHint")} error={errorFor("openvpn.username")}>
        <input className={control} value={username} onChange={(event) => onUsername(event.target.value)} />
      </Field>
      <PasswordField
        label={t("vpn.password")}
        hint={t(keepsPassword ? "vpn.secretKeepHint" : "vpn.openVPNPasswordHint")}
        error={errorFor("secrets.openvpnPassword")}
        value={password}
        onChange={onPassword}
      />
    </>
  );
}
