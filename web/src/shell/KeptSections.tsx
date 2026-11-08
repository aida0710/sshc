import { Activity, Suspense, memo } from "react";
import type { Section } from "../routing/sectionRoute";
import { RouteSkeleton } from "../ui/RouteSkeleton";
import { drawnSections } from "./sectionKeeping";
import { SectionView, type SectionViewProps } from "./sections";

// A hidden section is not drawn again for what changes in the shell while it is away (the location, a handoff
// meant for the section shown): it keeps what it was last given until it is shown again.
const KeptSectionView = memo(
  function KeptSectionView({ shown: _shown, ...props }: SectionViewProps & { shown: boolean }) {
    return <SectionView {...props} />;
  },
  (_previous, next) => !next.shown,
);

// KeptSections draws the section shown, and behind it the kept sections (useKeptSections) hidden with their
// state. A hidden section's effects are stopped, so it reads and polls nothing until it is shown again.
export function KeptSections({
  kept,
  shown,
  viewProps,
}: {
  kept: readonly Section[];
  shown: Section | null;
  viewProps: Omit<SectionViewProps, "section">;
}) {
  return drawnSections(kept, shown).map((section) => (
    <Activity key={section} mode={section === shown ? "visible" : "hidden"}>
      <Suspense fallback={<RouteSkeleton />}>
        <KeptSectionView shown={section === shown} section={section} {...viewProps} />
      </Suspense>
    </Activity>
  ));
}
