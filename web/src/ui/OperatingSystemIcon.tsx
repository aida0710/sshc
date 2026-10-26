import type { HostMetadata } from "../api/config";
import { useTranslate } from "../i18n/context";
import { Icon } from "./icons";
import ubuntu from "./os-icons/ubuntu.svg";
import debian from "./os-icons/debian.svg";
import redhat from "./os-icons/redhat.svg";
import fedora from "./os-icons/fedora.svg";
import centos from "./os-icons/centos.svg";
import rocky from "./os-icons/rockylinux.svg";
import almalinux from "./os-icons/almalinux.svg";
import arch from "./os-icons/archlinux.svg";
import opensuse from "./os-icons/opensuse.svg";
import macos from "./os-icons/apple.svg";
import windows from "./os-icons/windows11.svg";
import linux from "./os-icons/linux.svg";

export const operatingSystems = [
  ["server", "Server"], ["linux", "Linux"], ["amazonlinux", "Amazon Linux"], ["ubuntu", "Ubuntu"], ["debian", "Debian"],
  ["redhat", "Red Hat Enterprise Linux"], ["fedora", "Fedora"], ["centos", "CentOS"],
  ["rocky", "Rocky Linux"], ["almalinux", "AlmaLinux"], ["arch", "Arch Linux"],
  ["alpine", "Alpine Linux"], ["opensuse", "openSUSE / SUSE"], ["freebsd", "FreeBSD"],
  ["macos", "macOS"], ["windows", "Windows"],
] as const;

const sources: Record<string, string> = { ubuntu, debian, redhat, fedora, centos, rocky, almalinux, arch, opensuse, macos, windows, linux, amazonlinux: linux, alpine: linux };

// plain and compact are the bare mark beside text. tile puts it on a small square of the well colour, as
// in the connection list, and accentTile on a larger square of the accent colour, as on the Home cards
// and at the top of an open connection.
type OperatingSystemIconLook = "plain" | "compact" | "tile" | "accentTile";

const frames: Record<OperatingSystemIconLook, string> = {
  plain: "size-6 text-ink-muted",
  compact: "size-5 text-ink-muted",
  tile: "size-7 rounded-md bg-well text-ink-muted",
  accentTile: "size-[34px] rounded-[9px] bg-accent/15 text-accent",
};

const marks: Record<OperatingSystemIconLook, string> = {
  plain: "size-5",
  compact: "size-4",
  tile: "size-4",
  accentTile: "size-5",
};

export function OperatingSystemIcon({ os, colour = "", look = "plain" }: {
  os?: HostMetadata["os"] | undefined;
  colour?: string;
  look?: OperatingSystemIconLook;
}) {
  const t = useTranslate();
  const name = os === "server" ? t("host.osGeneric") : operatingSystems.find(([value]) => value === os)?.[1] ?? t("host.osUnknown");
  const source = sources[os ?? ""];
  const onTile = look === "tile" || look === "accentTile";
  return (
    <span role="img" aria-label={name} title={name}
      className={`relative grid ${frames[look]} shrink-0 place-items-center`}>
      {source === undefined ? <Icon name="connections" className={marks[look]} /> : (
        <span aria-hidden="true" className={`${marks[look]} bg-current [mask-position:center] [mask-repeat:no-repeat] [mask-size:contain]`} style={{ maskImage: `url("${source}")` }} />
      )}
      {colour === "" ? null : <span aria-hidden="true" className={`absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full border ${onTile ? "border-well" : "border-card"}`} style={{ backgroundColor: colour }} />}
    </span>
  );
}
