import { useEffect, useState } from "react";
import type { RemoteEntry } from "./api";
import { hasThumbnail } from "./entryThumbnails";
import { requestThumbnail } from "./thumbnailQueue";

// thumbnailSettleMs is how long a row must stay in view before its thumbnail is asked for: a flick through a long
// folder passes over rows nobody is looking at, and each one read would cost data.
const thumbnailSettleMs = 150;

type VisibilityChange = (visible: boolean) => void;

const watchers = new Map<Element, VisibilityChange>();
let observer: IntersectionObserver | null = null;

// One observer serves every row. With no root it sees the scrolling list's clipping as well as the window's, and a pane
// in a tab not on screen counts as out of view.
function watchVisibility(element: Element, change: VisibilityChange): () => void {
  if (typeof IntersectionObserver === "undefined") return () => undefined;
  observer ??= new IntersectionObserver((changes) => {
    for (const { target, isIntersecting } of changes) watchers.get(target)?.(isIntersecting);
  });
  watchers.set(element, change);
  observer.observe(element);
  return () => {
    watchers.delete(element);
    observer?.unobserve(element);
  };
}

type MadeThumbnail = { key: string; url: string };

// useEntryThumbnail gives a row's picture once it has been made, and null until then or if the file has none. alias is
// null while the list draws no thumbnails. A row asks while it is in view and cancels when it leaves. A row that got no
// picture asks again the next time it comes into view: the engine remembers a file that has none and reads nothing, and
// after a lost connection this is what gets the picture after all.
export function useEntryThumbnail(alias: string | null, entry: RemoteEntry): { boxRef: (element: Element | null) => void; url: string | null } {
  const [box, setBox] = useState<Element | null>(null);
  const [made, setMade] = useState<MadeThumbnail | null>(null);
  const { path, revision } = entry;
  const key = alias === null || !hasThumbnail(entry) ? null : `${alias}\0${path}\0${revision}`;

  useEffect(() => {
    if (alias === null || key === null || box === null) return;
    let settling: ReturnType<typeof setTimeout> | null = null;
    let cancelRequest: (() => void) | null = null;
    let made = false;
    const stop = () => {
      if (settling !== null) clearTimeout(settling);
      settling = null;
      cancelRequest?.();
      cancelRequest = null;
    };
    const stopWatching = watchVisibility(box, (visible) => {
      if (!visible) {
        stop();
        return;
      }
      if (made || settling !== null || cancelRequest !== null) return;
      settling = setTimeout(() => {
        settling = null;
        cancelRequest = requestThumbnail({ alias, path, revision }, (url) => {
          cancelRequest = null;
          if (url === null) return;
          made = true;
          setMade({ key, url });
        });
      }, thumbnailSettleMs);
    });
    return () => {
      stopWatching();
      stop();
    };
  }, [alias, box, key, path, revision]);

  return { boxRef: setBox, url: made?.key === key ? made.url : null };
}
