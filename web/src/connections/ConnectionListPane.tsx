import { useTranslate } from "../i18n/context";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionTree, type HostSelection } from "./ConnectionTree";
import type { DragPayload } from "./dragdrop";


export function ConnectionListPane({
  overview,
  selection,
  invalidLocation,
  onDismissInvalidLocation,
  onSelect,
  onDrop,
  movesDisabled,
}: {
  overview: Overview;
  selection: HostSelection | null;
  invalidLocation: boolean;
  onDismissInvalidLocation: () => void;
  onSelect: (host: HostEntry) => void;
  onDrop: (payload: DragPayload, target: string) => void;
  movesDisabled: boolean;
}) {
  const t = useTranslate();
  return (
  <div
    className={`min-h-0 flex-col border-r border-line bg-tree md:flex ${
      selection === null ? "flex" : "hidden"
    }`}
  >
    <div className="min-h-0 flex-1">
      {invalidLocation ? (
        <section className="m-4 flex flex-col gap-2 rounded-lg border border-line bg-card p-3 text-sm" role="status">
          <p className="font-medium">{t("browser.invalidUrl")}</p>
          <button
            type="button"
            className="min-h-10 self-start rounded-md border border-control-line bg-card px-3 py-1.5 text-sm text-ink hover:bg-select-fill md:min-h-0"
            onClick={onDismissInvalidLocation}
          >
            {t("browser.backToServers")}
          </button>
        </section>
      ) : (
        <ConnectionTree
          overview={overview}
          selected={selection}
          onSelect={onSelect}
          onDrop={onDrop}
          movesDisabled={movesDisabled}
        />
      )}
    </div>
  </div>
  );
}
