import { useCallback, useState } from "react";
import type { Section } from "../routing/sectionRoute";
import type { NavigationBlocker } from "../routing/useSectionRoute";
import { keptWhenLeft, nextKeptSections } from "./sectionKeeping";

// useKeptSections keeps the sections of the phone's bottom bar mounted behind the one shown once they have been
// opened (keptWhenLeft; KeptSections draws them), so that going back to one shows it at once, as it was left:
// where it was scrolled, what was open in it, what it had already read. That is how an app's tabs behave;
// rebuilding a page on every switch showed its skeleton and its "reading" state each time.
//
// A page asks before its draft is lost through the navigation blocker. A section that set one while shown is
// let go when left, because leaving it either saved the draft or discarded it, and a discarded draft must not
// be there when the section is opened again. setNavigationBlocker is the one to hand to the pages.
export function useKeptSections({
  shown,
  setNavigationBlocker,
}: {
  shown: Section | null;
  setNavigationBlocker: (blocker: NavigationBlocker | null) => void;
}) {
  const [kept, setKept] = useState<Section[]>(() => (keptWhenLeft(shown) ? [shown] : []));
  const [lastShown, setLastShown] = useState(shown);
  const [draftGuarded, setDraftGuarded] = useState(false);
  if (shown !== lastShown) {
    setLastShown(shown);
    setKept((previous) => nextKeptSections(previous, { left: lastShown, opened: shown, letGoOfLeft: draftGuarded }));
    setDraftGuarded(false);
  }

  const guardDraft = useCallback((blocker: NavigationBlocker | null) => {
    setNavigationBlocker(blocker);
    if (blocker !== null) setDraftGuarded(true);
  }, [setNavigationBlocker]);

  return { kept, setNavigationBlocker: guardDraft };
}
