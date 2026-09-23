import type { HostDetail, HostMetadata } from "../api/config";
import type { VPNProfile } from "../api/vpn";
import { useTranslate } from "../i18n/context";
import { Field, control } from "../ui/form";
import { joinHostPort, vpnTargetReaches } from "../vpn/vpnEndpoint";
import { deriveBasicField } from "./basicFields";

// この接続が通るVPNプロファイルを選ぶ。プロファイルは接続先ひとつだけへ届くので、
// その接続先がこの接続の相手（HostName:Port）と違うものは、注記を付けて選べなくする。
// 選んでも engine が繋ぐときに vpn_target_mismatch で断るだけだからである。

// OpenSSH が Port の指定が無いときに使うポートである。
const defaultSSHPort = "22";

// connectionAddress は、engine が繋ぐときに照合する相手を返す。HostName や Port が
// 複数の書き方で決まっていて1つに定まらないときは null を返し、照合しない。
function connectionAddress(detail: HostDetail): string | null {
  const hostName = deriveBasicField(detail, "HostName");
  const port = deriveBasicField(detail, "Port");
  if (hostName.origin === "complex" || port.origin === "complex") return null;
  return joinHostPort(hostName.value || detail.form.entry.identity.alias, port.value || defaultSSHPort);
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
  const address = connectionAddress(detail);
  const reaches = (profile: VPNProfile) => address === null || vpnTargetReaches(profile.target, address);
  const chosenProfile = profiles.find((profile) => profile.name === chosen);
  const mismatch =
    chosenProfile === undefined || address === null || reaches(chosenProfile)
      ? undefined
      : t("connection.vpnTargetMismatchHint", { address, name: chosenProfile.name, target: chosenProfile.target });

  return (
    <Field label={t("connection.vpnLabel")} hint={t("connection.vpnHint")} error={mismatch}>
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
        {profiles.map((profile) =>
          reaches(profile) ? (
            <option key={profile.name} value={profile.name}>
              {profile.name}
            </option>
          ) : (
            <option key={profile.name} value={profile.name} disabled>
              {t("connection.vpnTargetMismatch", { name: profile.name, target: profile.target })}
            </option>
          ),
        )}
        {/* 消えたプロファイルを指したままでも、いま何を指しているかは見えるようにする。 */}
        {chosen === "" || chosenProfile !== undefined ? null : (
          <option value={chosen}>{t("connection.vpnMissing", { name: chosen })}</option>
        )}
      </select>
    </Field>
  );
}
