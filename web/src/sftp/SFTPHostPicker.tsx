import { useEffect, useRef, useState } from "react";
import type { HostEntry } from "../api/config";
import type { RecentConnection } from "../api/recentConnections";
import { useTranslate } from "../i18n/context";
import { HostPickerDialog } from "../shell/HostPickerDialog";
import { Icon } from "../ui/icons";
import { localHostAlias } from "./localHost";

const noHosts: HostEntry[] = [];

// The shared chooser opens from the workspace tab or a standalone toolbar.
// A request opens the same dialog without duplicating the source label.
export function SFTPHostPicker({
  aliases,
  hosts = noHosts,
  value,
  disabled = false,
  compact = false,
  includeLocal = false,
  hideTrigger = false,
  openRequest = 0,
  loadRecent,
  onChange,
}: {
  aliases: string[];
  hosts?: HostEntry[];
  value: string;
  disabled?: boolean;
  compact?: boolean;
  includeLocal?: boolean;
  hideTrigger?: boolean;
  openRequest?: number;
  loadRecent?: () => Promise<{ connections: RecentConnection[] }>;
  onChange: (alias: string) => void;
}) {
  const t = useTranslate();
  const [open, setOpen] = useState(false);
  const trigger = useRef<HTMLButtonElement>(null);
  const handledRequest = useRef(openRequest);
  useEffect(() => {
    if (handledRequest.current === openRequest) return;
    handledRequest.current = openRequest;
    if (disabled) return;
    if (document.activeElement instanceof HTMLButtonElement) trigger.current = document.activeElement;
    setOpen(true);
  }, [openRequest, disabled]);
  const localName = t("sftp.local.connection");
  const local = includeLocal
    ? [{ id: "engine", label: localName, detail: t("sftp.local.engine"), current: value === localHostAlias }]
    : [];

  function choose(alias: string) {
    onChange(alias);
    setOpen(false);
  }

  return (
    <>
      {hideTrigger ? null : <button
        ref={trigger}
        type="button"
        aria-label={t("sftp.host")}
        data-value={value}
        disabled={disabled || (aliases.length === 0 && !includeLocal)}
        onClick={() => setOpen(true)}
        title={value === localHostAlias ? localName : value || t("sftp.chooseHost")}
        className={compact
          ? "flex size-12 shrink-0 items-center justify-center rounded-md border border-control-line bg-control text-ink-muted active:bg-select-fill disabled:text-ink-faint"
          : "flex min-h-9 min-w-0 max-w-full items-center justify-between gap-2 rounded-md border border-control-line bg-control px-3 py-1.5 text-left text-sm disabled:text-ink-faint md:min-h-8 md:py-1"}
      >
        {compact ? (
          <Icon name={value === localHostAlias ? "home" : "terminal"} className="size-5" />
        ) : (
          <>
            <span className="truncate">
              {value === localHostAlias
                ? localName
                : value || t(aliases.length === 0 && !includeLocal ? "sftp.noHosts" : "sftp.chooseHost")}
            </span>
            <Icon name="chevronRight" className="size-3 rotate-90 text-ink-muted" />
          </>
        )}
      </button>}
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
