import type { MouseEvent } from "react";
import { useTranslate } from "../i18n/context";
import { sectionPath, type Section } from "../routing/sectionRoute";
import { Icon } from "../ui/icons";
import { sectionIcons, sectionLabels } from "./sectionPresentation";

// The sections a phone reaches in one tap from the bar at the bottom; the
// rest are under Menu.
const mobileNavigationSections = ["Home", "Connections", "Files", "Terminal", "Menu"] as const satisfies readonly Section[];

// barSection is the tab the bar marks for a section. A page opened from Menu (Settings, SSH Keys, …) has no tab of
// its own, so the bar marks Menu: the person sees where they are and how to get back.
function barSection(section: Section | null): Section | null {
  if (section === null) return null;
  return (mobileNavigationSections as readonly Section[]).includes(section) ? section : "Menu";
}

export function MobileNavigation({
  section,
  onNavigate,
}: {
  section: Section | null;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
}) {
  const t = useTranslate();
  const marked = barSection(section);
  return (
    // A rounded bar held off the screen's edges, floating over the page rather than taking a band of its own:
    // the page shows around it, and scrolls its last line clear of it (--mobile-navigation-inset). Only the
    // icons show: the bar at the top already names the current section, so each name is kept for screen
    // readers and as a tooltip. It stays under the inspector (z-10), which covers the page on a phone.
    <nav aria-label={t("shell.mobileNavigation")} className="sshc-mobile-navigation pointer-events-none absolute inset-x-0 bottom-0 z-[5] px-3">
      <div className="pointer-events-auto flex h-14 items-center justify-between rounded-full border border-line bg-card px-2 shadow-pane">
        {mobileNavigationSections.map((name) => {
          const current = marked === name;
          const label = t(sectionLabels[name]);
          return (
            <a
              key={name}
              href={sectionPath(name)}
              title={label}
              aria-current={current ? "page" : undefined}
              onClick={(event) => onNavigate(event, name)}
              className={`flex h-11 w-14 items-center justify-center rounded-full transition-colors ${current ? "bg-accent text-accent-ink" : "text-ink-muted active:bg-hover"}`}
            >
              <Icon name={sectionIcons[name]} className="size-5" />
              <span className="sr-only">{label}</span>
            </a>
          );
        })}
      </div>
    </nav>
  );
}
