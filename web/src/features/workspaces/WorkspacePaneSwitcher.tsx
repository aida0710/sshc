import { useTranslate } from "../../i18n/context";
import type { RuntimePane } from "./layout";

// On a phone a workspace shows one pane at a time; this row of buttons picks
// which one.
export function WorkspacePaneSwitcher({
  panes,
  focusedPaneId,
  paneLabel,
  onFocus,
}: {
  panes: RuntimePane[];
  focusedPaneId: string | undefined;
  paneLabel: (pane: RuntimePane) => string;
  onFocus: (pane: RuntimePane) => void;
}) {
  const t = useTranslate();
  return (
    <nav
      aria-label={t("workspace.mobilePaneSwitcher")}
      className="flex shrink-0 gap-1 overflow-x-auto border-b border-line bg-toolbar px-2 py-1"
    >
      {panes.map((pane) => {
        const focused = pane.id === focusedPaneId;
        return (
          <button
            key={pane.id}
            type="button"
            aria-current={focused ? "page" : undefined}
            className={`max-w-40 shrink-0 truncate rounded px-3 py-1.5 text-xs ${focused ? "bg-select-fill text-ink" : "text-ink-muted"}`}
            onClick={() => onFocus(pane)}
          >
            {paneLabel(pane)}
          </button>
        );
      })}
    </nav>
  );
}
