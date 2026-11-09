// A finger that pulls something past its end moves it less and less the further it pulls, as a rubber band
// stretches: the end gives a little and says there is nothing further, where a dead stop feels like a wall.

// How stiff the band is: the share of the first pixels past the end that still move it (iOS's 0.55).
const give = 0.55;

// rubberBand is how far something is drawn past its end when the finger is overflow pixels past it, in a space
// dimension pixels long. It never reaches dimension, however far the finger goes.
export function rubberBand(overflow: number, dimension: number): number {
  if (dimension <= 0 || overflow === 0) return 0;
  const sign = Math.sign(overflow);
  const pulled = Math.abs(overflow);
  return sign * (1 - 1 / ((pulled * give) / dimension + 1)) * dimension;
}

// rubberBandSlope is how far something drawn by rubberBand moves for each pixel the finger moves, overflow pixels past
// the end: what turns the finger's speed into the speed of what it pulls, when it lets go.
export function rubberBandSlope(overflow: number, dimension: number): number {
  if (dimension <= 0) return 0;
  const stretched = (Math.abs(overflow) * give) / dimension + 1;
  return give / (stretched * stretched);
}
