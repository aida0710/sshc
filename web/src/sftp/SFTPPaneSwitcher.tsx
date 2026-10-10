import { useTranslate } from "../i18n/context";
import { localHostAlias } from "./localHost";
import { activeTab, type SFTPPane } from "./sftpPanes";

export function SFTPPaneSwitcher({ panes, focusedPaneId, onSelect }: {
  panes: SFTPPane[];
  focusedPaneId: string;
  onSelect: (paneId: string) => void;
}) {
  const t = useTranslate();
  return (
    <nav aria-label={t("sftp.paneSwitcher")} className="mb-1 flex shrink-0 gap-1 rounded-md bg-toolbar p-1">
      {panes.map((pane, index) => {
        const selected = pane.id === focusedPaneId;
        const alias = activeTab(pane)?.alias ?? "";
        const host = alias === localHostAlias ? t("sftp.local.connection") : alias || t("sftp.newTab");
        const label = t(index === 0 ? "sftp.leftPane" : "sftp.rightPane", { host });
        return (
          <button key={pane.id} type="button" aria-current={selected ? "page" : undefined}
            className={`min-h-11 min-w-0 flex-1 truncate rounded px-3 text-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent ${selected ? "bg-card font-medium text-ink" : "text-ink-muted"}`}
            onClick={() => onSelect(pane.id)} title={label}>
            {label}
          </button>
        );
      })}
    </nav>
  );
}
