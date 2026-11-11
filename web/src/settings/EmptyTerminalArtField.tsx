import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { aquariumWater } from "../terminal/emptyState/aquariumWater";
import { AsciiArtCanvas } from "../terminal/emptyState/AsciiArtCanvas";
import { emptyTerminalArtOf, emptyTerminalArts, withEmptyTerminalArt, type DrawnArt, type EmptyTerminalArt } from "../terminal/emptyState/emptyTerminalArt";
import { FieldRow } from "../ui/FieldRow";
import { Icon } from "../ui/icons";
import { Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { updatePreset, useSettings } from "./settingsStore";

const artLabelKeys: Record<EmptyTerminalArt, MessageKey> = {
  none: "appearance.emptyTerminalArt.none",
  network: "appearance.emptyTerminalArt.network",
  contours: "appearance.emptyTerminalArt.contours",
  globe: "appearance.emptyTerminalArt.globe",
  aquarium: "appearance.emptyTerminalArt.aquarium",
};

// A sample is the art standing still, drawn smaller and darker than on the terminal so that it reads at this size.
// The landscape is shrunk to show relief, at a moment when the sample has high ground in it.
const sampleFontSize = 7;
const sampleIntensity = 2.2;
const sampleOptions = { labels: [], globe: { x: 0.5, y: 0.55, radius: 0.46 }, zoom: 2.5, fish: [], fishLabels: false, swimDepth: 1 } as const;
const sampleMoments: Record<DrawnArt, number> = { network: 5, contours: 30.5, globe: 5, aquarium: 5 };

// Which art the terminal of the preset being edited draws while no session is open, chosen from samples.
export function EmptyTerminalArtField({ presetId }: { presetId: string }) {
  const t = useTranslate();
  const preset = useSettings().document?.presets.find((candidate) => candidate.id === presetId) ?? { id: presetId };
  const chosen = emptyTerminalArtOf(preset);
  const saving = useAsyncOperation();

  function choose(next: EmptyTerminalArt) {
    if (next === chosen) return;
    void saving.run(
      () => updatePreset(presetId, (current) => withEmptyTerminalArt(current, next)),
      { describe: () => t("appearance.emptyTerminalArtSaveFailed") },
    );
  }

  return (
    <FieldRow label={t("appearance.emptyTerminalArt")} hint={t("appearance.emptyTerminalArtHint")} interactiveChildren>
      <div className="flex flex-col gap-3">
        <div className="grid grid-cols-2 gap-3.5 md:grid-cols-[repeat(5,minmax(0,132px))]" role="radiogroup" aria-label={t("appearance.emptyTerminalArt")}>
          {emptyTerminalArts.map((art) => (
            <label key={art} className="flex min-w-0 cursor-pointer flex-col gap-2 text-ui">
              <input type="radio" name="appearance-empty-terminal-art" value={art} checked={chosen === art} disabled={saving.busy}
                onChange={() => choose(art)} className="peer sr-only" />
              <span aria-hidden="true" className="relative block aspect-[16/10] overflow-hidden rounded-inset border border-line bg-[color-mix(in_srgb,var(--ui-ink)_3%,var(--ui-card))] peer-checked:outline-2 peer-checked:outline-offset-2 peer-checked:outline-accent peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent">
                {art === "none"
                  ? <span className="absolute inset-0 flex items-center justify-center text-ink-faint"><Icon name="noBackground" className="size-5" /></span>
                  : (
                    <>
                      {art === "aquarium" ? <span className={`absolute inset-0 ${aquariumWater}`} /> : null}
                      <AsciiArtCanvas art={art} fontSize={sampleFontSize} intensity={sampleIntensity} options={sampleOptions} still stillAt={sampleMoments[art]} className="relative" />
                    </>
                  )}
              </span>
              <span className="flex flex-wrap items-center gap-x-2 text-ink-muted peer-checked:text-ink peer-checked:[&>span:first-child]:border-[5px] peer-checked:[&>span:first-child]:border-accent">
                <span aria-hidden="true" className="size-4 shrink-0 rounded-full border-[1.5px] border-ink-faint" />
                {t(artLabelKeys[art])}
              </span>
            </label>
          ))}
        </div>
        {saving.error === "" ? null : <Notice tone="danger">{saving.error}</Notice>}
      </div>
    </FieldRow>
  );
}
