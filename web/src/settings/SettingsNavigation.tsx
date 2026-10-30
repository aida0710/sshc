import { Fragment } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Icon } from "../ui/icons";
import { opensInThisPage } from "../ui/linkClick";
import { PresetPicker } from "./PresetPicker";
import { settingsPageMeta, settingsPages, type SettingsGroup, type SettingsPage } from "./settingsRoute";

// groupLabels names the groups the list is split into. The actions on this machine's sessions come last,
// under a line rather than a name.
const groupLabels: Record<SettingsGroup, MessageKey | null> = {
  global: "settings.groupGlobal",
  preset: "settings.groupPreset",
  machines: "settings.groupMachines",
  actions: null,
};

const groupOrder: readonly SettingsGroup[] = ["global", "preset", "machines", "actions"];

// The settings pages listed beside the open one, so a page is one click from another, in the groups the
// settings are kept in: the global settings, the preset chosen above its pages, and the machines. The rows
// look like the sidebar's.
export function SettingsNavigation({
  current,
  onNavigate,
  editingPreset,
  onEditPreset,
}: {
  current: SettingsPage | null;
  onNavigate: (path: string) => void;
  editingPreset: string;
  onEditPreset: (id: string) => void;
}) {
  const t = useTranslate();
  return (
    // sticky: the list stays in view while a long settings page scrolls beside it.
    <nav aria-label={t("settings.heading")} className="sticky top-2 hidden w-[220px] shrink-0 flex-col gap-0.5 self-start px-2.5 py-4 md:flex">
      <h2 className="px-2 pb-2 text-xs font-medium text-ink-faint">{t("settings.heading")}</h2>
      {groupOrder.map((group) => {
        const label = groupLabels[group];
        return (
          <Fragment key={group}>
            {label === null
              ? <div className="mx-2 my-2 border-t border-line" />
              : <div className="px-2 pb-1 pt-3 text-[11px] font-medium text-ink-faint first-of-type:pt-0">{t(label)}</div>}
            {group === "preset" ? <PresetPicker layout="list" editing={editingPreset} onEdit={onEditPreset} /> : null}
            {settingsPages.filter((page) => settingsPageMeta[page].group === group).map((page) => {
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
          </Fragment>
        );
      })}
    </nav>
  );
}
