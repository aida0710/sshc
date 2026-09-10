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
  ["server", "Server"], ["linux", "Linux"], ["ubuntu", "Ubuntu"], ["debian", "Debian"],
  ["redhat", "Red Hat Enterprise Linux"], ["fedora", "Fedora"], ["centos", "CentOS"],
  ["rocky", "Rocky Linux"], ["almalinux", "AlmaLinux"], ["arch", "Arch Linux"],
  ["alpine", "Alpine Linux"], ["opensuse", "openSUSE / SUSE"], ["freebsd", "FreeBSD"],
  ["macos", "macOS"], ["windows", "Windows"],
] as const;

const sources: Record<string, string> = { ubuntu, debian, redhat, fedora, centos, rocky, almalinux, arch, opensuse, macos, windows, linux, alpine: linux };

export function OperatingSystemIcon({ os, colour = "" }: { os?: HostMetadata["os"] | undefined; colour?: string }) {
  const t = useTranslate();
  const name = os === "server" ? t("host.osGeneric") : operatingSystems.find(([value]) => value === os)?.[1] ?? t("host.osUnknown");
  const source = sources[os ?? ""];
  return (
    <span role="img" aria-label={name} title={name}
      className="relative grid size-10 shrink-0 place-items-center rounded-lg border border-line bg-control">
      {source === undefined ? <Icon name="connections" className="size-6 text-ink-muted" /> : (
        <img src={source} alt="" className={`size-7 ${os === "macos" ? "[[data-theme=dark]_&]:invert" : ""}`} />
      )}
      {colour === "" ? null : <span aria-hidden="true" className="absolute -bottom-0.5 -right-0.5 size-2.5 rounded-full border border-card" style={{ backgroundColor: colour }} />}
    </span>
  );
}
