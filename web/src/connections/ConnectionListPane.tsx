import { Button } from "../ui/surface";
import { useTranslate } from "../i18n/context";
import type { HostEntry, Overview } from "../api/config";
import { ConnectionTree, type HostSelection } from "./ConnectionTree";
import type { DragPayload } from "./dragdrop";
import { Icon } from "../ui/icons";


export function ConnectionListPane({
  overview,
  selection,
  invalidLocation,
  onDismissInvalidLocation,
  onBeginCreation,
  onSelect,
  onOpenPatternRule,
  onDrop,
  movesDisabled,
}: {
  overview: Overview;
  selection: HostSelection | null;
  invalidLocation: boolean;
  onDismissInvalidLocation: () => void;
  onBeginCreation: () => void;
  onSelect: (host: HostEntry) => void;
  onOpenPatternRule: (path: string, line: number) => void;
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
    <div className="flex shrink-0 items-center justify-between gap-3 border-b border-line px-4 py-4">
      <div className="flex min-w-0 items-center gap-3">
        <span aria-hidden="true" className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-select-fill text-accent">
          <Icon name="connections" className="size-5" />
        </span>
        <div className="min-w-0">
          <h2 className="font-semibold tracking-tight text-ink">{t("conn.heading")}</h2>
          <p className="text-xs text-ink-muted">
          {t("conn.count", { count: overview.hosts.filter((host) => host.identity.alias !== "").length })}
          </p>
        </div>
      </div>
      <Button
        kind="primary"
        className="min-h-10 shrink-0 px-2.5 py-1.5 text-xs md:min-h-0"
        onClick={onBeginCreation}
      >
        {t("conn.new")}
      </Button>
    </div>
    <div className="min-h-0 flex-1 overflow-y-auto px-3 py-4">
      {invalidLocation ? (
        <section className="flex flex-col gap-2 rounded-lg border border-line bg-card p-3 text-sm" role="status">
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
          onOpenPatternRule={onOpenPatternRule}
          onDrop={onDrop}
          movesDisabled={movesDisabled}
        />
      )}
    </div>
  </div>
  );
}
