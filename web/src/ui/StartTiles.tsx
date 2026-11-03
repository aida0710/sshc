import { BrandMark } from "./BrandMark";
import { Button } from "./surface";
import { Icon, type IconName } from "./icons";

export type StartTile = {
  icon: IconName;
  title: string;
  // note is one line on what the tile does, because a person who sees these has not used the page yet.
  note: string;
  // primary marks the way most people start. A board has one.
  primary?: boolean;
  onSelect: () => void;
};

type StartTilesProps = {
  heading: string;
  headingLevel?: "h2" | "h3";
  // brand puts the app's mark beside the heading, on a board that is the whole page.
  brand?: boolean;
  tiles: readonly StartTile[];
  // alternative is a smaller way to start, under the tiles.
  alternative?: { lead: string; label: string; icon: IconName; onSelect: () => void };
  // inPage is for a board that is one section of a page, at the section's width. Otherwise the board is alone
  // in the space it is given and sits in the middle of it.
  inPage?: boolean;
};

const columnsByTileCount: Record<number, string> = {
  2: "@2xl:grid-cols-2",
  3: "@2xl:grid-cols-3",
};

// StartTiles is what a page shows while it has nothing to list: the ways to begin, as tiles to press. Tiles lie
// side by side where the page's card is wide enough (the card is a size container) and are rows otherwise, as
// on a phone, where the card has no container and the base classes apply.
export function StartTiles({ heading, headingLevel: Heading = "h3", brand = false, tiles, alternative, inPage = false }: StartTilesProps) {
  return (
    <section className={`flex w-full flex-col gap-3 @2xl:gap-4 ${inPage ? "pb-2 pt-4" : "m-auto max-w-[900px] pb-3 pt-1 @2xl:py-6"}`}>
      <div className="flex items-center gap-3">
        {brand ? <BrandMark className="size-10 shrink-0" /> : null}
        <Heading className="text-balance text-[17px] font-semibold leading-6 text-ink @2xl:text-xl @2xl:leading-7">{heading}</Heading>
      </div>
      <div className={`grid grid-cols-1 gap-2.5 @2xl:gap-3 ${columnsByTileCount[tiles.length] ?? ""}`}>
        {tiles.map((tile) => <Tile key={tile.title} tile={tile} />)}
      </div>
      {alternative === undefined ? null : (
        <div className="flex flex-wrap items-center justify-center gap-x-3 gap-y-2 text-[13px] text-ink-muted @2xl:justify-start @2xl:border-t @2xl:border-line @2xl:pt-3.5">
          <span>{alternative.lead}</span>
          <Button className="inline-flex items-center gap-1.5" onClick={alternative.onSelect}>
            <Icon name={alternative.icon} className="size-[15px] shrink-0" />
            {alternative.label}
          </Button>
        </div>
      )}
    </section>
  );
}

function Tile({ tile }: { tile: StartTile }) {
  const primary = tile.primary === true;
  // The tile's height is !important because SSH Keys sets a min-height on every button inside its page.
  return (
    <button
      type="button"
      onClick={tile.onSelect}
      className={`flex items-center gap-3 rounded-[14px] border p-3.5 text-left text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent @2xl:relative @2xl:min-h-[172px]! @2xl:flex-col @2xl:items-start @2xl:gap-1 @2xl:px-[18px] @2xl:pb-5 @2xl:pt-[18px] ${
        primary
          ? "border-[color-mix(in_srgb,var(--ui-accent)_60%,transparent)] bg-[color-mix(in_srgb,var(--ui-accent)_9%,var(--ui-card))] hover:bg-[color-mix(in_srgb,var(--ui-accent)_15%,var(--ui-card))]"
          : "border-line bg-card hover:bg-hover"
      }`}
    >
      <span className={`flex size-10 shrink-0 items-center justify-center rounded-[10px] ${primary ? "bg-accent text-accent-ink" : "bg-[color-mix(in_srgb,var(--ui-ink)_8%,transparent)] text-ink-muted"}`}>
        <Icon name={tile.icon} className="size-5" />
      </span>
      <span className="flex min-w-0 flex-1 flex-col gap-0.5 @2xl:mt-auto @2xl:flex-none @2xl:gap-1">
        <span className="text-[15px] font-semibold leading-[22px]">{tile.title}</span>
        <span className="text-pretty text-[12.5px] leading-[18px] text-ink-muted">{tile.note}</span>
      </span>
      <span className={`shrink-0 @2xl:absolute @2xl:right-4 @2xl:top-[18px] ${primary ? "text-accent" : "text-ink-faint"}`}>
        <Icon name="chevronRight" className="size-4" />
      </span>
    </button>
  );
}
