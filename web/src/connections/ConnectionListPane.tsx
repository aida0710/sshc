import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionTree, type ConnectionTreeProps, type HostSelection } from "./ConnectionTree";
import type { ConnectionDrag } from "./useConnectionDrag";
import { Button } from "../ui/surface";
import { workspaceListPane } from "../ui/workspaceLayout";


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
  listAlone = false,
  startTiles = null,
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
  // 広い画面で何も開いていないあいだ、一覧がページの幅を全部使う（workspaceColumnsLayout）。
  listAlone?: boolean;
  // startTiles takes the list's place while there is nothing to list.
  startTiles?: ReactNode;
}) {
  const t = useTranslate();
  return (
  <div className={workspaceListPane(compact, detailOpen, { listAlone })}>
    {resizeHandle}
    <div className="min-h-0 flex-1">
      {startTiles !== null ? (
        <div className="flex h-full overflow-y-auto px-5 pb-(--mobile-navigation-inset)">{startTiles}</div>
      ) : invalidLocation ? (
        <section className="m-3.5 flex flex-col gap-2 rounded-inset bg-well p-3 text-sm" role="status">
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
