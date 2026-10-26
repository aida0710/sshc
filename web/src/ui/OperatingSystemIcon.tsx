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

type OperatingSystemIconProps = {
  os?: HostMetadata["os"] | undefined;
  colour?: string;
  compact?: boolean;
  // tile puts the mark on a small square in the accent colour, as on the Home cards.
  tile?: boolean;
};

export function OperatingSystemIcon({ os, colour = "", compact = false, tile = false }: OperatingSystemIconProps) {
  const t = useTranslate();
  const name = os === "server" ? t("host.osGeneric") : operatingSystems.find(([value]) => value === os)?.[1] ?? t("host.osUnknown");
  const source = sources[os ?? ""];
  const frame = tile
    ? "size-[34px] rounded-[9px] bg-accent/15 text-accent"
    : `${compact ? "size-5" : "size-6"} text-ink-muted`;
  return (
    <span role="img" aria-label={name} title={name}
      className={`relative grid ${frame} shrink-0 place-items-center`}>
      {source === undefined ? <Icon name="connections" className={compact ? "size-4" : "size-5"} /> : (
        <span aria-hidden="true" className={`${compact ? "size-4" : "size-5"} bg-current [mask-position:center] [mask-repeat:no-repeat] [mask-size:contain]`} style={{ maskImage: `url("${source}")` }} />
      )}
      {colour === "" ? null : <span aria-hidden="true" className={`absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full border ${tile ? "border-well" : "border-card"}`} style={{ backgroundColor: colour }} />}
    </span>
  );
}
