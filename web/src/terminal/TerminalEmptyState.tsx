import type { ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { presetInUse, useSettings } from "../settings/settingsStore";
import { PageBoard } from "../ui/PageBoard";
import { StartTiles } from "../ui/StartTiles";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { AsciiArtCanvas } from "./emptyState/AsciiArtCanvas";
import { ConnectionActivityPanel } from "./emptyState/ConnectionActivityPanel";
import { emptyTerminalArtOf, type ArtOptions, type DrawnArt } from "./emptyState/emptyTerminalArt";
import { MachineStatusLine } from "./emptyState/MachineStatusLine";
import { RecentConnectionsPanel } from "./emptyState/RecentConnectionsPanel";
import { useTerminalOverview, type TerminalOverview } from "./emptyState/useTerminalOverview";

// How each layout draws its art. On a PC the art fills the card behind the tiles, the globe rising from below like a
// horizon. On a phone the tiles and the cards would hide such a picture, so it is a banner above the tiles, where
// the landscape is shrunk and darkened to show relief in so little height.
const wideArt = { fontSize: 13, globe: { x: 0.5, y: 1.42, radius: 0.92 }, zoom: 1, intensity: 1 } as const;
const bannerArt = { fontSize: 10, globe: { x: 0.5, y: 2.2, radius: 1.85 }, zoom: 3.2, intensity: 1.5 } as const;

type Props = {
  // runTarget is the engine a new shell opens on; null is this machine.
  runTarget: string | null;
  // visible is false while another page is shown over the terminal: the machine is not read then.
  visible: boolean;
  onOpenLocalShell: () => void;
  onAddConnection: () => void;
  onOpenSync: () => void;
  onOpenConnection: (alias: string) => void;
  onShowConnections: () => void;
};

// Nothing is open: the ways to open the first session, in the same card as the pages and the terminal, with what
// the engine new shells open on can tell about itself: its machine, and the connections opened from it.
export function TerminalEmptyState(props: Props) {
  const phone = useMediaQuery(mobileViewportQuery);
  const art = emptyTerminalArtOf(presetInUse(useSettings().document));
  const overview = useTerminalOverview(props.runTarget, props.visible);
  const tiles = <StartingTiles {...props} />;
  return phone
    ? <PhoneLayout art={art === "none" ? null : art} overview={overview} runTarget={props.runTarget} tiles={tiles} />
    : <WideLayout art={art === "none" ? null : art} overview={overview} tiles={tiles} {...props} />;
}

function StartingTiles({ runTarget, onOpenLocalShell, onAddConnection, onOpenSync }: Props) {
  const t = useTranslate();
  return (
    <div role="status">
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
  );
}

function artOptions(overview: TerminalOverview, layout: typeof wideArt | typeof bannerArt): ArtOptions {
  // The network names its points after the connections used last.
  return { labels: (overview.history?.recent ?? []).map((connection) => connection.alias), globe: layout.globe, zoom: layout.zoom };
}

// The card does not scroll itself, so the art behind stays put; the content scrolls inside it. A small phone
// (below 400px) gives the margin up, so that the heading and the notes stay on one line. The content scrolls
// clear of a phone's bottom bar, which floats over the page.
const scrollingContent = "flex min-h-0 flex-1 flex-col overflow-y-auto p-6 pb-[calc(1.5rem+var(--mobile-navigation-inset))] max-[399px]:p-3 max-[399px]:pb-[calc(0.75rem+var(--mobile-navigation-inset))]";

function WideLayout({ art, overview, tiles, runTarget, onOpenConnection, onShowConnections }: Props & {
  art: DrawnArt | null;
  overview: TerminalOverview;
  tiles: ReactNode;
}) {
  const hours = overview.history?.hours ?? [];
  const recent = overview.history?.recent ?? [];
  return (
    // The card is not named, so it does not become a region beside the panels that explain a refused run target.
    // isolate keeps the art behind the content but in front of the card's own background.
    <PageBoard fill className="relative isolate">
      {art === null ? null : (
        <div className="pointer-events-none absolute inset-0 -z-10">
          <AsciiArtCanvas art={art} fontSize={wideArt.fontSize} intensity={wideArt.intensity} options={artOptions(overview, wideArt)} />
        </div>
      )}
      {/* The machine sits in the corner, so the content leaves room under it. */}
      <div className={`${scrollingContent} ${overview.machine === null ? "" : "pb-16"}`}>
        <div className="m-auto flex w-full flex-col gap-2">
          {tiles}
          {hours.length === 0 && recent.length === 0 ? null : (
            <div className="mx-auto grid w-full max-w-[900px] grid-cols-1 gap-3 @2xl:grid-cols-[minmax(0,1.1fr)_minmax(0,1fr)]">
              {hours.length === 0 ? null : <ConnectionActivityPanel hours={hours} />}
              {recent.length === 0 ? null : <RecentConnectionsPanel recent={recent} onOpenConnection={onOpenConnection} onShowConnections={onShowConnections} />}
            </div>
          )}
        </div>
      </div>
      {overview.machine === null ? null : (
        <MachineStatusLine machine={overview.machine} cpuHistory={overview.cpuHistory} runTarget={runTarget}
          className="absolute bottom-[18px] left-5 z-[1] rounded-[10px] border border-line bg-[color-mix(in_srgb,var(--ui-card)_80%,transparent)] px-3 py-1.5 backdrop-blur-[8px]" />
      )}
    </PageBoard>
  );
}

function PhoneLayout({ art, overview, tiles, runTarget }: {
  art: DrawnArt | null;
  overview: TerminalOverview;
  tiles: ReactNode;
  runTarget: string | null;
}) {
  const hours = overview.history?.hours ?? [];
  const machine = overview.machine === null ? null : (
    <MachineStatusLine machine={overview.machine} cpuHistory={overview.cpuHistory} runTarget={runTarget}
      className={art === null ? "px-0.5" : "absolute inset-x-0 bottom-0 bg-linear-to-t from-[color-mix(in_srgb,var(--ui-card)_96%,transparent)] from-60% to-transparent px-4 pb-3 pt-[34px]"} />
  );
  return (
    <PageBoard fill>
      <div className={scrollingContent}>
        <div className="m-auto flex w-full max-w-[900px] flex-col gap-3.5">
          {art === null ? machine : (
            <div className="relative h-[132px] shrink-0 overflow-hidden rounded-[14px] border border-line bg-[color-mix(in_srgb,var(--ui-ink)_3%,var(--ui-card))]">
              <AsciiArtCanvas art={art} fontSize={bannerArt.fontSize} intensity={art === "contours" ? bannerArt.intensity : 1} options={artOptions(overview, bannerArt)} />
              {machine}
            </div>
          )}
          {tiles}
          {hours.length === 0 ? null : <ConnectionActivityPanel hours={hours} />}
        </div>
      </div>
    </PageBoard>
  );
}
