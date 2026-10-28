// kittyStoredImages keeps account of the kitty images the image addon holds
// for later placement: how much data the images sent only to be stored
// (a=t) take, and how much decoding placements (a=p) may ask for.

// The addon keeps at most this many kitty images and drops undisplayed ones
// beyond it. The ledger drops its oldest entry at the same count; it does not
// copy the addon's choice of which image goes.
const maxStoredImages = 256;

// The addon decodes a stored image again for every placement, as much work as
// the first time: about 75 ms for 4096 × 4096 pixels in Chromium on the
// machine this was measured on, so 200 placements of 27 bytes each held a
// terminal's output for 15 seconds. Placements may decode four images of the
// largest size a second (about 0.3 s of decoding), and wait beyond that.
const largestImagesDecodedPerSecond = 4;

export type StoredKittyImage = {
  pixels: number;
  // The bytes of an image sent only to be stored (a=t). An image that was
  // also displayed (a=T) is bounded by the addon's image storage instead.
  storedOnlyBytes: number;
};

export type KittyStoredImages = {
  // stored records an image the addon has taken. Without an id the addon
  // numbers it itself, so the ledger counts it without being able to find it.
  stored(id: number | undefined, image: StoredKittyImage): void;
  deleted(id: number): void;
  cleared(): void;
  // hasRoomFor tells whether another image sent only to be stored fits. One
  // image may go past the limit, since its size is known only at its end.
  hasRoomFor(id: number | undefined): boolean;
  // mayPlace takes the decoding a placement asks for from the budget.
  mayPlace(id: number): boolean;
};

export function createKittyStoredImages({ pixelLimit, storedOnlyByteLimit, now }: {
  // Most pixels of one image; an image the ledger does not know costs this.
  pixelLimit: number;
  storedOnlyByteLimit: number;
  now: () => number;
}): KittyStoredImages {
  const images = new Map<number | string, StoredKittyImage>();
  let unnamed = 0;
  const placementBudget = largestImagesDecodedPerSecond * pixelLimit;
  let budget = placementBudget;
  let budgetAt = now();

  const storedOnlyBytes = (except: number | undefined) => {
    let total = 0;
    for (const [key, image] of images) if (key !== except) total += image.storedOnlyBytes;
    return total;
  };

  return {
    stored(id, image) {
      const key = id ?? `unnamed-${unnamed++}`;
      images.delete(key);
      images.set(key, image);
      if (images.size > maxStoredImages) images.delete(images.keys().next().value!);
    },
    deleted(id) {
      images.delete(id);
    },
    cleared() {
      images.clear();
    },
    hasRoomFor(id) {
      return storedOnlyBytes(id) < storedOnlyByteLimit;
    },
    mayPlace(id) {
      const at = now();
      budget = Math.min(placementBudget, budget + (at - budgetAt) / 1000 * placementBudget);
      budgetAt = at;
      const cost = images.get(id)?.pixels ?? pixelLimit;
      if (budget < cost) return false;
      budget -= cost;
      return true;
    },
  };
}
