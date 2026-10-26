import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionTree, type ConnectionTreeProps, type HostSelection } from "./ConnectionTree";
import type { ConnectionDrag } from "./useConnectionDrag";
import { Button } from "../ui/surface";


export function ConnectionListPane({
  compact = false,
  overview,
  selection,
  invalidLocation,
  onDismissInvalidLocation,
  onSelect,
  drag,
  resizeHandle = null,
  treeOptions = {},
  detailOpen = selection !== null,
}: {
  compact?: boolean;
  overview: Overview;
  selection: HostSelection | null;
  invalidLocation: boolean;
  onDismissInvalidLocation: () => void;
  onSelect: (host: HostEntry) => void;
  drag: ConnectionDrag;
  // resizeHandle sits on the pane's right edge; the page owns the width.
  resizeHandle?: ReactNode;
  // 複数選択とグループの操作。ConnectionTree へそのまま渡す。
  treeOptions?: Omit<ConnectionTreeProps, "overview" | "selected" | "onSelect" | "drag">;
  // 狭い画面では、右側（接続先やグループの編集）を開いているあいだ一覧を隠す。
  detailOpen?: boolean;
}) {
  const t = useTranslate();
  return (
  <div
    className={`relative min-h-0 flex-col ${compact ? "" : "border-r border-line md:flex"} ${
      detailOpen ? "hidden" : "flex"
    }`}
  >
    {resizeHandle}
    <div className="min-h-0 flex-1">
      {invalidLocation ? (
        <section className="m-3.5 flex flex-col gap-2 rounded-[10px] bg-well p-3 text-sm" role="status">
          <p className="font-medium">{t("browser.invalidUrl")}</p>
          <Button
            className="min-h-10 self-start md:min-h-0"
            onClick={onDismissInvalidLocation}
          >
            {t("browser.backToServers")}
          </Button>
        </section>
      ) : (
        <ConnectionTree
          overview={overview}
          selected={selection}
          onSelect={onSelect}
          drag={drag}
          compact={compact}
          {...treeOptions}
        />
      )}
    </div>
  </div>
  );
}
