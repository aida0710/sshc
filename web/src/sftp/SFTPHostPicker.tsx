import { useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import type { RecentConnection } from "../api/recentConnections";
import { useTranslate } from "../i18n/context";
import { HostPickerDialog } from "../shell/HostPickerDialog";
import { Icon } from "../ui/icons";
import { localHostAlias } from "./localHost";

const noHosts: HostEntry[] = [];

// SFTPHostPicker is the pane's destination button. The dialog behind it is
// shared with the console list so every place a host is chosen looks alike.
export function SFTPHostPicker({
  aliases,
  hosts = noHosts,
  value,
  disabled = false,
  compact = false,
  includeLocal = false,
  loadRecent,
  onChange,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  value: string;
  disabled?: boolean;
  compact?: boolean;
  includeLocal?: boolean;
  loadRecent?: () => Promise<{ connections: RecentConnection[] }>;
  onChange: (alias: string) => void;
}) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const localName = t("sftp.local.connection");
  const local = includeLocal ? [{ id: "engine", label: localName, detail: t("sftp.local.engine") }] : [];

  function choose(alias: string) {
    onChange(alias);
    setOpen(false);
  }

  return (
    <>
      <button ref={trigger} type="button" aria-label={t("sftp.host")} data-value={value} disabled={disabled || (aliases.length === 0 && !includeLocal)} onClick={() => setOpen(true)} title={value === localHostAlias ? localName : value || t("sftp.chooseHost")} className={compact ? "flex size-11 shrink-0 items-center justify-center rounded-md border border-control-line bg-control text-ink-muted active:bg-select-fill disabled:text-ink-faint" : "flex min-h-9 min-w-0 max-w-full items-center justify-between gap-2 rounded-md border border-control-line bg-control px-3 py-1.5 text-left text-sm disabled:text-ink-faint md:min-h-8 md:py-1"}>
        {compact ? <Icon name={value === localHostAlias ? "home" : "terminal"} className="size-4" /> : <><span className="truncate">{value === localHostAlias ? localName : value || t(aliases.length === 0 && !includeLocal ? "sftp.noHosts" : "sftp.chooseHost")}</span><Icon name="chevronRight" className="size-3 rotate-90 text-ink-muted" /></>}
      </button>
      <HostPickerDialog
        open={open}
        heading={t("sftp.chooseHostHeading")}
        aliases={aliases}
        hosts={hosts}
        value={value}
        local={local}
        {...(loadRecent === undefined ? {} : { loadRecent })}
        initialFocus={compact ? "close" : "search"}
        returnFocusRef={trigger}
        onChoose={choose}
        onChooseLocal={() => choose(localHostAlias)}
        onClose={() => setOpen(false)}
      />
    </>
  );
}
