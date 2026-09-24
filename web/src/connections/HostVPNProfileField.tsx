import type { HostDetail, HostMetadata } from "../api/config";
import type { VPNProfile } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Field, control } from "../ui/form";
import { vpnDestinationHostRefusal } from "../vpn/vpnDestination";
import { deriveBasicField } from "./basicFields";

// この接続に付けるVPNプロファイルを選ぶ。接続先は、この接続の HostName と Port である。
//
// 接続先をホスト名で書いた接続は、プロファイルに VPN 内の DNS サーバーが無いと、
// 繋ぐときに engine が断る（vpn_destination_invalid の name_needs_dns）。選んだ時点で
// そのことを注記する。

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
  profiles,
  onMetadata,
}: {
  detail: HostDetail;
  // profiles は、保存されているVPNプロファイルである。
  profiles: VPNProfile[];
  onMetadata: (metadata: HostMetadata) => void;
}) {
  const t = useTranslate();
  const chosen = detail.metadata.vpn ?? "";
  const chosenProfile = profiles.find((profile) => profile.name === chosen);
  const note =
    chosenProfile === undefined || !needsDNS(detail, chosenProfile)
      ? undefined
      : t("connection.vpnNeedsDNS", { name: chosenProfile.name });

  return (
    <Field label={t("connection.vpnLabel")} hint={t("connection.vpnHint")} error={note}>
      <select
        value={chosen}
        onChange={(event) => {
          const metadata = { ...detail.metadata };
          const profile = event.target.value;
          if (profile === "") delete metadata.vpn;
          else metadata.vpn = profile;
          onMetadata(metadata);
        }}
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
  );
}
