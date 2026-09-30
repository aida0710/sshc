import type { MouseEvent } from "react";
import { useTranslate } from "../i18n/context";
import { sectionPath, type Section } from "../routing/sectionRoute";
import { Icon } from "../ui/icons";
import { sectionIcons, sectionLabels } from "./sections";

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
    <nav
      aria-label={t("shell.mobileNavigation")}
      className="sshc-mobile-navigation grid shrink-0 grid-cols-5 border-t border-line bg-toolbar"
    >
      {mobileNavigationSections.map((name) => {
        const current = section === name;
        return (
          <a
            key={name}
            href={sectionPath(name)}
            aria-current={current ? "page" : undefined}
            onClick={(event) => onNavigate(event, name)}
            className={`relative flex min-h-13 min-w-0 flex-col items-center justify-center gap-0.5 px-1 py-1 text-[10px] transition-colors ${current ? "bg-select-fill font-semibold text-accent" : "text-ink-muted active:bg-hover"}`}
          >
            {current ? <span aria-hidden="true" className="absolute inset-x-4 top-0 h-0.5 rounded-full bg-accent" /> : null}
            <Icon name={sectionIcons[name]} className="size-5" />
            <span className="max-w-full truncate">{t(sectionLabels[name])}</span>
          </a>
        );
      })}
    </nav>
  );
}
