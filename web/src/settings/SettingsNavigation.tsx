import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { opensInThisPage } from "../ui/linkClick";
import { settingsPageMeta, settingsPages, type SettingsPage } from "./settingsRoute";

// The settings pages listed beside the open one, so a page is one click from
// another. The rows look like the sidebar's.
export function SettingsNavigation({
  current,
  onNavigate,
}: {
  current: SettingsPage | null;
  onNavigate: (path: string) => void;
}) {
  const t = useTranslate();
  return (
    <nav aria-label={t("settings.heading")} className="hidden w-[220px] shrink-0 flex-col gap-0.5 px-2.5 py-4 md:flex">
      <h2 className="px-2 pb-2 text-xs font-medium text-ink-faint">{t("settings.heading")}</h2>
      {settingsPages.map((page) => {
        const meta = settingsPageMeta[page];
        const active = page === current;
        return (
          <a
            key={page}
            href={meta.path}
            aria-current={active ? "page" : undefined}
            onClick={(event) => {
              if (!opensInThisPage(event)) return;
              event.preventDefault();
              onNavigate(meta.path);
            }}
            className={`flex min-h-[30px] items-center gap-2.5 rounded-[7px] px-2 text-[13px] ${
              active ? "bg-select-fill font-medium text-ink" : "text-ink-muted hover:bg-hover hover:text-ink"
            }`}
          >
            <Icon name={meta.icon} className={`size-4 shrink-0 ${active ? "text-accent" : ""}`} />
            <span className="min-w-0 truncate">{t(meta.label)}</span>
          </a>
        );
      })}
    </nav>
  );
}
