import type { ConnectionHistory } from "../../api/terminalOverview";
import { mobileViewportQuery, useMediaQuery } from "../../ui/useMediaQuery";
import { aquariumFish } from "./aquariumFish";
import { aquariumWater } from "./aquariumWater";
import { AsciiArtCanvas } from "./AsciiArtCanvas";
import type { DrawnArt } from "./emptyTerminalArt";

// The globe rises from below the card like a horizon, so the list above it stays on the dark of the sky.
const globe = { x: 0.5, y: 1.42, radius: 0.92 } as const;
// The characters of the art, in CSS pixels. A phone's card is narrow, so smaller characters keep the picture's shapes.
const wideFontSize = 13;
const phoneFontSize = 10;
// The art is drawn fainter than on a page of its own, so the connections over it stay the thing that is read.
const backdropOpacity = "opacity-60";

// HomeBackdrop draws the art the preset chose behind Home. It stays in the pane while Home scrolls: the card grows
// with the list, and a picture as tall as the list would be drawn mostly out of sight.
export function HomeBackdrop({ art, history }: { art: DrawnArt; history: ConnectionHistory | null }) {
  const phone = useMediaQuery(mobileViewportQuery);
  return (
    <div aria-hidden="true" className={`pointer-events-none absolute inset-0 -z-10 ${backdropOpacity}`}>
      <div className={`sticky top-0 h-dvh max-h-full ${art === "aquarium" ? aquariumWater : ""}`}>
        <AsciiArtCanvas
          art={art}
          fontSize={phone ? phoneFontSize : wideFontSize}
          intensity={1}
          options={{
            // The network names its points after the connections used last, and the aquarium's fish stand for them.
            labels: (history?.recent ?? []).map((connection) => connection.alias),
            globe,
            zoom: 1,
            fish: aquariumFish(history, new Date()),
            // On a phone the names would cover the fish, so a name shows only while its fish eats.
            fishLabels: !phone,
            swimDepth: 1,
          }}
        />
      </div>
    </div>
  );
}
