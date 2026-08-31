import type { MouseEvent } from "react";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { Icon, type IconName } from "../ui/icons";

export type MenuItem = {
  key: string;
  label: MessageKey;
  icon: IconName;
  href: string;
};

export type MenuGroup = {
  label: MessageKey;
  items: MenuItem[];
};

export function MenuPanel({
  groups,
  onNavigate,
}: {
  groups: MenuGroup[];
  onNavigate: (href: string) => void;
}) {
  const t = useTranslate();

  function follow(event: MouseEvent<HTMLAnchorElement>, href: string) {
    if (
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return;
    }
    event.preventDefault();
    onNavigate(href);
  }

  return (
    <section aria-labelledby="menu-heading" className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <div className="border-b border-line pb-3">
        <h2 id="menu-heading" className="text-xl font-semibold tracking-tight text-ink">
          {t("section.menu")}
        </h2>
      </div>

      <div className="grid items-start gap-x-8 gap-y-7 lg:grid-cols-2 xl:grid-cols-3">
        {groups.map((group) => (
          <section key={group.label} aria-labelledby={`menu-${group.label}`}>
            <h3
              id={`menu-${group.label}`}
              className="px-1 text-[11px] font-bold uppercase tracking-[0.14em] text-ink-faint"
            >
              {t(group.label)}
            </h3>
            <ul className="mt-2 overflow-hidden rounded border border-line bg-card">
              {group.items.map((item, index) => {
                const label = t(item.label);
                return (
                  <li key={item.key} className={index === 0 ? "" : "border-t border-line"}>
                    <a
                      href={item.href}
                      aria-label={t("menu.open", { section: label })}
                      onClick={(event) => follow(event, item.href)}
                      className="group flex min-h-14 items-center gap-3 px-3 py-2.5 transition-colors hover:bg-select-fill"
                    >
                      <span className="grid size-8 shrink-0 place-items-center rounded bg-control text-ink-muted group-hover:text-ink">
                        <Icon name={item.icon} className="size-4" />
                      </span>
                      <span className="min-w-0 flex-1 truncate text-sm font-medium text-ink">{label}</span>
                      <Icon name="chevronRight" className="size-4 shrink-0 text-ink-faint group-hover:text-ink-muted" />
                    </a>
                  </li>
                );
              })}
            </ul>
          </section>
        ))}
      </div>
    </section>
  );
}
