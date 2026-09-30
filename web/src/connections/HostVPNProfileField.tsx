import type { HostDetail } from "../api/config";
import type { VPNProfile } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Field, control } from "../ui/form";
import { Notice } from "../ui/surface";
import { vpnDestinationHostRefusal } from "../vpn/vpnDestination";
import { deriveBasicField } from "./basicFields";

// この接続に付けるVPNプロファイルを選ぶ。接続先は、この接続の HostName と Port である。
//
// 接続先をホスト名で書いた接続は、プロファイルに VPN 内の DNS サーバーが無いと、
// 繋ぐときに engine が断る（vpn_destination_invalid の name_needs_dns）。選んだ時点で
// そのことを注記する。
//
// 保存済みのパスワード・TOTP・起動スニペットの割り当ては、接続に付けたプロファイルに
// 結び付く（sshclient.Target.AuthenticationBinding）。付ける、外す、付け替えると停止中に
// なるので、保存済みのプロファイルと違うものを選んでいるあいだ、そのことを知らせる。

// needsDNS は、選んだプロファイルに DNS サーバーが無く、この接続の HostName がホスト名で
// あるかを返す。HostName が複数の書き方で決まっていて1つに定まらないときは注記しない。
function needsDNS(detail: HostDetail, profile: VPNProfile): boolean {
  const hostName = deriveBasicField(detail, "HostName");
  if (hostName.origin === "complex") return false;
  // HostName が無ければ、OpenSSH は alias をそのまま接続先にする。
  const host = hostName.value || detail.form.entry.identity.alias;
  return vpnDestinationHostRefusal(host, (profile.dns ?? []).length > 0) === "name_needs_dns";
}

export function HostVPNProfileField({
  detail,
  value: chosen,
  saved,
  profiles,
  onChange,
}: {
  detail: HostDetail;
  // value は、下書きで選んでいるプロファイルの名前である。空はVPNを使わない。
  value: string;
  // saved は、この接続に保存済みのプロファイルの名前である。空はVPNを使わない。
  saved: string;
  // profiles は、保存されているVPNプロファイルである。
  profiles: VPNProfile[];
  onChange: (profile: string) => void;
}) {
  const t = useTranslate();
  const chosenProfile = profiles.find((profile) => profile.name === chosen);
  const note =
    chosenProfile === undefined || !needsDNS(detail, chosenProfile)
      ? undefined
      : t("connection.vpnNeedsDNS", { name: chosenProfile.name });

  return (
    <div className="flex flex-col gap-2">
      <Field label={t("connection.vpnLabel")} hint={t("connection.vpnHint")} error={note}>
        <select
          value={chosen}
          onChange={(event) => onChange(event.target.value)}
          className={control}
        >
          <option value="">{t("connection.vpnNone")}</option>
          {profiles.map((profile) => (
            <option key={profile.name} value={profile.name}>
              {profile.name}
            </option>
          ))}
          {/* 消えたプロファイルを指したままでも、いま何を指しているかは見えるようにする。 */}
          {chosen === "" || chosenProfile !== undefined ? null : (
            <option value={chosen}>{t("connection.vpnMissing", { name: chosen })}</option>
          )}
        </select>
      </Field>
      {chosen === saved ? null : <Notice compact>{t("connection.vpnChangeStopsAssignments")}</Notice>}
    </div>
  );
}
