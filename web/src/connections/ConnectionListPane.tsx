import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionTree, type ConnectionTreeProps, type HostSelection } from "./ConnectionTree";
import type { DragPayload } from "./dragdrop";
import { Button } from "../ui/surface";


export function ConnectionListPane({
  compact = false,
  overview,
  selection,
  invalidLocation,
  onDismissInvalidLocation,
  onSelect,
  onDrop,
  movesDisabled,
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
  onDrop: (payload: DragPayload, target: string) => void;
  movesDisabled: boolean;
  // resizeHandle sits on the pane's right edge; the page owns the width.
  resizeHandle?: ReactNode;
  // 複数選択とグループの操作。ConnectionTree へそのまま渡す。
  treeOptions?: Omit<ConnectionTreeProps, "overview" | "selected" | "onSelect" | "onDrop" | "movesDisabled">;
  // 狭い画面では、右側（接続先やグループの編集）を開いているあいだ一覧を隠す。
  detailOpen?: boolean;
}) {
  const t = useTranslate();
  return (
  <div
    className={`relative min-h-0 flex-col border-r border-line bg-tree ${compact ? "" : "md:flex"} ${
      detailOpen ? "hidden" : "flex"
    }`}
  >
    {resizeHandle}
    <div className="min-h-0 flex-1">
      {invalidLocation ? (
        <section className="m-4 flex flex-col gap-2 rounded-lg border border-line bg-card p-3 text-sm" role="status">
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
          onDrop={onDrop}
          movesDisabled={movesDisabled}
          compact={compact}
          {...treeOptions}
        />
      )}
    </div>
  </div>
  );
}
