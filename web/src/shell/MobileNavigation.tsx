import type { MouseEvent } from "react";
import { useTranslate } from "../i18n/context";
import { sectionPath, type Section } from "../routing/sectionRoute";
import { Icon } from "../ui/icons";
import { sectionIcons, sectionLabels } from "./sectionPresentation";

// The sections a phone reaches in one tap from the bar at the bottom; the
// rest are under Menu.
const mobileNavigationSections = ["Home", "Connections", "Files", "Terminal", "Menu"] as const satisfies readonly Section[];

export function MobileNavigation({
  section,
  onNavigate,
}: {
  section: Section | null;
  onNavigate: (event: MouseEvent<HTMLAnchorElement>, name: Section) => void;
}) {
  const t = useTranslate();
  return (
    // A rounded bar held off the screen's edges. Only the icons show: the bar at the top already names the
    // current section, so each name is kept for screen readers and as a tooltip.
    <nav aria-label={t("shell.mobileNavigation")} className="sshc-mobile-navigation shrink-0 px-3 pt-1.5">
      <div className="flex h-14 items-center justify-between rounded-full border border-line bg-card px-2 shadow-pane">
        {mobileNavigationSections.map((name) => {
          const current = section === name;
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
