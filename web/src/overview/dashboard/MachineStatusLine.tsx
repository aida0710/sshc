import { Fragment, type ReactNode } from "react";
import type { MachineStatus } from "../../api/terminalOverview";
import { useTranslate } from "../../i18n/context";
import { formatBytes } from "../../ui/format";
import { operatingSystemName, uptimeText } from "./machineStatusText";

// A bar of the CPU chart is a quarter of a pixel high for each percent, up to 25px; at least 2px so 0% shows.
const pixelsPerPercent = 0.25;
const lowestBar = 2;

// MachineStatusLine is the machine new shells open on, in one line: the run target's name when it is another
// engine, the machine's own name and OS, then what the machine gives of its CPU, memory and uptime. This machine
// is told by its name alone, so the line starts with it. A value the machine does not give is left out, not shown
// as zero.
export function MachineStatusLine({
  machine,
  cpuHistory,
  runTarget,
  className = "",
}: {
  machine: MachineStatus;
  cpuHistory: readonly number[];
  runTarget: string | null;
  className?: string;
}) {
  const t = useTranslate();
  const values: { key: string; shown: ReactNode }[] = [];
  if (machine.cpuPercent !== undefined) {
    values.push({ key: "cpu", shown: <span className="inline-flex items-center gap-1.5">
      {t("terminal.overview.cpu", { percent: Math.round(machine.cpuPercent) })}
      <span aria-hidden="true" className="inline-flex h-3 items-end gap-px">
        {cpuHistory.map((percent, index) => (
          <span key={index} className="w-[3px] rounded-[1px] bg-[color-mix(in_srgb,var(--ui-accent)_70%,transparent)]" style={{ height: Math.max(lowestBar, Math.round(percent * pixelsPerPercent)) }} />
        ))}
      </span>
    </span> });
  }
  if (machine.memoryTotalBytes !== undefined) {
    const total = formatBytes(machine.memoryTotalBytes);
    values.push({ key: "memory", shown: machine.memoryUsedBytes === undefined
      ? t("terminal.overview.memoryTotal", { total })
      : t("terminal.overview.memory", { used: formatBytes(machine.memoryUsedBytes), total }) });
  }
  if (machine.uptimeSeconds !== undefined) {
    values.push({ key: "uptime", shown: t("terminal.overview.uptime", { duration: uptimeText(machine.uptimeSeconds, t) }) });
  }
  const operatingSystem = operatingSystemName(machine.os);
  // The first name on the line stands out: the run target's, or this machine's own.
  const hostname = runTarget === null && machine.hostname !== "" ? <span className="font-semibold text-ink">{machine.hostname}</span> : machine.hostname;
  return (
    <div aria-label={t("terminal.overview.machineLabel")} className={`flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-xs leading-[18px] text-ink-muted ${className}`}>
      <span aria-hidden="true" className="size-[7px] shrink-0 rounded-full bg-accent shadow-[0_0_0_3px_color-mix(in_srgb,var(--ui-accent)_25%,transparent)]" />
      {runTarget === null ? null : <span className="font-semibold text-ink">{runTarget}</span>}
      <span>
        {hostname}
        {machine.hostname === "" || operatingSystem === "" ? null : " · "}
        {operatingSystem}
      </span>
      {values.map(({ key, shown }) => (
        <Fragment key={key}>
          <span aria-hidden="true" className="text-ink-faint">·</span>
          <span>{shown}</span>
        </Fragment>
      ))}
    </div>
  );
}
