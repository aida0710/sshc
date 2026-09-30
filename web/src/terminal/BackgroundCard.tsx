import type { TerminalBackground } from "../api/settings";
import { useTranslate } from "../i18n/context";
import { ActionMenu } from "../ui/ActionMenu";
import { formatBytes } from "../ui/format";
import { BackgroundThumbnail } from "./BackgroundThumbnail";

type BackgroundCardProps = {
  background: TerminalBackground;
  chosen: boolean;
  onChoose: () => void;
  onRename: () => void;
  onDelete: () => void;
};

// One saved image in the background library: pressing the card chooses it,
// and its "…" menu renames or deletes it.
export function BackgroundCard({ background, chosen, onChoose, onRename, onDelete }: BackgroundCardProps) {
  const t = useTranslate();
  return (
    <li className={`relative overflow-hidden rounded-md bg-surface-subtle ring-1 ${chosen ? "ring-accent" : "ring-hairline"}`}>
      <button type="button" onClick={onChoose} className="block w-full text-left">
        <BackgroundThumbnail name={background.name} chosen={chosen} className="aspect-video w-full" />
        <span className="block px-2.5 pb-2.5 pt-2">
          <span className="block truncate text-sm font-medium text-ink">{background.name}</span>
          <span className="mt-0.5 block text-xs text-ink-faint">{formatBytes(background.bytes)}</span>
        </span>
      </button>
      <div className="absolute right-2 top-2">
        <ActionMenu
          label={t("terminal.backgroundActions", { name: background.name })}
          triggerClassName="flex size-8 items-center justify-center rounded-md bg-canvas/80 text-ink shadow-sm hover:bg-card"
          items={[
            { label: t("terminal.backgroundRenameAction"), onSelect: onRename },
            { label: t("terminal.backgroundDeleteAction"), onSelect: onDelete, tone: "danger" },
          ]}
        />
      </div>
    </li>
  );
}
