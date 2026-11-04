import { useTranslate } from "../i18n/context";
import { PageBoard } from "../ui/PageBoard";
import { StartTiles } from "../ui/StartTiles";

// Nothing is open: the ways to open the first session, in the same card as the pages and the terminal.
export function TerminalEmptyState({
  runTarget,
  onOpenLocalShell,
  onAddConnection,
  onOpenSync,
}: {
  // runTarget is the engine a new shell opens on; null is this machine.
  runTarget: string | null;
  onOpenLocalShell: () => void;
  onAddConnection: () => void;
  onOpenSync: () => void;
}) {
  const t = useTranslate();
  return (
    // The card is not named, so it does not become a region beside the panels that explain a refused run target.
    // A small phone (below 400px) gives the margin up, so that the heading and the notes stay on one line. The list scrolls
    // clear of a phone's bottom bar, which floats over the page.
    <PageBoard fill className="overflow-y-auto p-6 pb-[calc(1.5rem+var(--mobile-navigation-inset))] max-[399px]:p-3 max-[399px]:pb-[calc(0.75rem+var(--mobile-navigation-inset))]">
      <div className="m-auto flex w-full" role="status">
        <StartTiles
          heading={t("terminal.emptyHeading")}
          headingLevel="h2"
          brand
          tiles={[
            {
              icon: "terminal",
              title: t("start.openLocalShell"),
              note: runTarget === null ? t("start.openLocalShellNote") : t("start.openLocalShellNoteOnRunTarget", { alias: runTarget }),
              primary: true,
              onSelect: onOpenLocalShell,
            },
            { icon: "connections", title: t("conn.new"), note: t("start.addConnectionNote"), onSelect: onAddConnection },
            { icon: "sync", title: t("start.receiveSync"), note: t("start.receiveSyncNote"), onSelect: onOpenSync },
          ]}
        />
      </div>
    </PageBoard>
  );
}
