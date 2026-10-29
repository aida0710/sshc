import { useId, type MouseEvent, type ReactNode } from "react";
import { updateApi as defaultUpdateApi, type UpdateApi } from "../api/update";
import { useLanguage } from "../i18n/context";
import { localeLabelKeys, locales, type Locale } from "../i18n/locale";
import { useSignOutConfirmation } from "../session/useSignOutConfirmation";
import { ChevronSelect } from "../ui/ChevronSelect";
import { Icon, type IconName } from "../ui/icons";
import { opensInThisPage } from "../ui/linkClick";
import { PageHeader } from "../ui/page";
import { PageBoard } from "../ui/PageBoard";
import { Notice } from "../ui/surface";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import type { MenuGroup, MenuItem } from "./menuEntries";
import { availableRelease, useUpdateStatus } from "./useUpdateStatus";

// MenuPanel lists every page the sidebar does not, each with a line on what it is for. On a wide
// screen it is one card, like Settings, with the groups in columns; on a phone each group is a card.
export function MenuPanel({
  columns,
  onNavigate,
  onOpenSearch,
  api = defaultUpdateApi,
}: {
  columns: MenuGroup[][];
  onNavigate: (href: string) => void;
  // onOpenSearch opens the search. On a phone the Menu leads with it: the pages fill three screens, and the
  // search in the side navigation is behind the drawer.
  onOpenSearch?: () => void;
  api?: UpdateApi;
}) {
  const { t } = useLanguage();
  const compact = useMediaQuery(mobileViewportQuery);
  const release = availableRelease(useUpdateStatus(api).status);

  // About tells of a published version in place of its usual line, so the Menu shows it without a badge.
  function describe(item: MenuItem) {
    if (item.key === "About" && release !== undefined) return { text: t("menu.updateAvailable", { version: release }), notice: true };
    return { text: t(item.description), notice: false };
  }

  const groups = columns.map((column, index) => (
    <div key={index} className="flex min-w-0 flex-col gap-[18px]">
      {column.map((group) => (
        <MenuGroupSection key={group.label} label={t(group.label)} compact={compact}>
          {group.items.map((item) => (
            <MenuLink key={item.key} item={item} description={describe(item)} compact={compact} onNavigate={onNavigate} />
          ))}
        </MenuGroupSection>
      ))}
      {index === columns.length - 1 ? (
        <MenuGroupSection label={t("menu.displaySettings")} compact={compact}>
          <LanguageRow compact={compact} />
          <SignOutRow compact={compact} />
        </MenuGroupSection>
      ) : null}
    </div>
  ));

  return (
    <PageBoard labelledBy="menu-heading" className={compact ? "flex flex-col gap-[18px]" : ""}>
      <PageHeader title={t("section.menu")} headingId="menu-heading" className="mb-[18px]" />
      {compact && onOpenSearch !== undefined ? (
        <button
          type="button"
          onClick={onOpenSearch}
          className="flex h-11 w-full items-center gap-2 rounded-[14px] border border-line bg-sidebar px-3.5 text-left text-[15px] text-ink-faint"
        >
          <Icon name="search" className="size-4 shrink-0" />
          <span className="min-w-0 flex-1 truncate">{t("menu.search")}</span>
        </button>
      ) : null}
      {compact ? groups : <div className="grid items-start gap-x-7 gap-y-[18px] @2xl:grid-cols-2 @4xl:grid-cols-3">{groups}</div>}
    </PageBoard>
  );
}

function MenuGroupSection({ label, compact, children }: { label: string; compact: boolean; children: ReactNode }) {
  const headingId = useId();
  return (
    <section aria-labelledby={headingId}>
      <h3 id={headingId} className={`pb-1.5 text-xs leading-[17px] text-ink-faint ${compact ? "px-1.5" : "px-2.5"}`}>{label}</h3>
      <ul className={compact ? "overflow-hidden rounded-[14px] border border-line bg-sidebar [&>li+li>:first-child]:border-t [&>li+li>:first-child]:border-line" : ""}>
        {children}
      </ul>
    </section>
  );
}

// The row's look, shared by the links and the two controls under Preferences. On a phone a select is
// 44px tall (index.css), so the Language row passes tighter padding to stay 56px like the others.
function rowClassName(compact: boolean, phonePadding = "py-2") {
  return compact ? `flex min-h-14 items-center gap-3 px-3 ${phonePadding}` : "flex min-h-[46px] items-center gap-3 rounded-md px-2.5 py-1.5";
}

function RowIcon({ name, compact, tone = "plain" }: { name: IconName; compact: boolean; tone?: "plain" | "danger" }) {
  return (
    <span className={`grid shrink-0 place-items-center rounded-md ${compact ? "size-8" : "size-[30px]"} ${tone === "danger" ? "bg-danger/12 text-danger" : "bg-well text-ink-muted"}`}>
      <Icon name={name} className="size-4" />
    </span>
  );
}

function MenuLink({ item, description, compact, onNavigate }: {
  item: MenuItem;
  description: { text: string; notice: boolean };
  compact: boolean;
  onNavigate: (href: string) => void;
}) {
  const { t } = useLanguage();
  const label = t(item.label);
  const descriptionId = useId();

  function follow(event: MouseEvent<HTMLAnchorElement>) {
    if (!opensInThisPage(event)) return;
    event.preventDefault();
    onNavigate(item.href);
  }

  return (
    <li>
      <a href={item.href} aria-label={t("menu.open", { section: label })} aria-describedby={descriptionId} onClick={follow}
        className={`${rowClassName(compact)} transition-colors ${compact ? "active:bg-hover" : "hover:bg-hover"}`}>
        <RowIcon name={item.icon} compact={compact} />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13.5px] leading-[18px] text-ink">{label}</span>
          <span id={descriptionId} className={`block truncate text-xs leading-4 ${description.notice ? "text-accent" : "text-ink-faint"}`}>{description.text}</span>
        </span>
        <Icon name="chevronRight" className="size-[15px] text-ink-faint" />
      </a>
    </li>
  );
}

function LanguageRow({ compact }: { compact: boolean }) {
  const { t, locale, setLocale } = useLanguage();
  const selectId = useId();
  return (
    <li>
      <div className={rowClassName(compact, "py-1.5")}>
        <RowIcon name="language" compact={compact} />
        <label htmlFor={selectId} className="min-w-0 flex-1 text-[13.5px] text-ink">{t("menu.language")}</label>
        <ChevronSelect id={selectId} value={locale} onChange={(event) => setLocale(event.target.value as Locale)} className="bg-well">
          {locales.map((candidate) => <option key={candidate} value={candidate}>{t(localeLabelKeys[candidate])}</option>)}
        </ChevronSelect>
      </div>
    </li>
  );
}

function SignOutRow({ compact }: { compact: boolean }) {
  const { t } = useLanguage();
  const hintId = useId();
  const { requestSignOut, busy, error, confirmation } = useSignOutConfirmation();
  // Kept apart from Language on a wide screen, so the two are not pressed by mistake.
  return (
    <li className={compact ? "" : "mt-2.5"}>
      {/* The row is already a large target, so it opts out of the phone's 44px minimum (index.css). */}
      <button type="button" data-touch-compact aria-label={t("menu.signOut")} aria-describedby={hintId} disabled={busy} onClick={requestSignOut}
        className={`${rowClassName(compact)} w-full text-left transition-colors hover:bg-danger/10 disabled:opacity-60`}>
        <RowIcon name="signOut" compact={compact} tone="danger" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13.5px] leading-[18px] text-danger">{t("menu.signOut")}</span>
          <span id={hintId} className="block truncate text-xs leading-4 text-ink-faint">{t("menu.signOutRowHint")}</span>
        </span>
      </button>
      {error === "" ? null : <div className="px-2.5 pb-2"><Notice tone="danger" compact>{error}</Notice></div>}
      {confirmation}
    </li>
  );
}
