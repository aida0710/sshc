import { useEffect, useRef } from "react";
import { notifyAndroidBackClosersChanged } from "./androidBackClosers";

// A BackStop is an open part of a page, such as the search bar over a terminal, that Android's Back closes before it
// leaves the page. It is not a layer (useDismissibleLayer): opening a layer closes the layer under it, and a layer takes
// Escape and the focus, while such a part stays open beside whatever else the page opens.
type BackStop = { close: () => void };

const openStops: BackStop[] = [];

// useAndroidBackStop keeps close as what Back does while open is true. The part opened last closes first.
export function useAndroidBackStop({ open, close }: { open: boolean; close: () => void }) {
  const latestClose = useRef(close);
  latestClose.current = close;
  useEffect(() => {
    if (!open) return;
    const stop: BackStop = { close: () => latestClose.current() };
    openStops.push(stop);
    notifyAndroidBackClosersChanged();
    return () => {
      openStops.splice(openStops.indexOf(stop), 1);
      notifyAndroidBackClosersChanged();
    };
  }, [open]);
}

// isAndroidBackStopOpen tells whether a part of a page is open for Back to close.
export function isAndroidBackStopOpen(): boolean {
  return openStops.length > 0;
}

// closeTopAndroidBackStop closes the part opened last, and tells whether there was one to close.
export function closeTopAndroidBackStop(): boolean {
  const top = openStops[openStops.length - 1];
  if (top === undefined) return false;
  top.close();
  return true;
}
