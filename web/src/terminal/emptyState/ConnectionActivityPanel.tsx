import { useRef } from "react";
import type { ConnectionHour } from "../../api/terminalOverview";
import { useLanguage } from "../../i18n/context";
import { connectionActivity, type ActivityLevel } from "./connectionActivity";
import { OverviewPanel } from "./OverviewPanel";
import { useElementSize } from "./useElementSize";

// A day is a 12px square with 4px between, so a week takes 16px of width.
const weekWidth = 16;
const dayGap = 4;
// At least two months and at most a year of weeks, as many as the card is wide.
const fewestWeeks = 8;
const mostWeeks = 53;
// With a year of weeks the range reads 過去1年 rather than the same month twice.
const weeksInAYear = 52;

const levelColors: Record<ActivityLevel, string> = {
  0: "bg-[color-mix(in_srgb,var(--ui-ink)_8%,transparent)]",
  1: "bg-[color-mix(in_srgb,var(--ui-accent)_28%,transparent)]",
  2: "bg-[color-mix(in_srgb,var(--ui-accent)_48%,transparent)]",
  3: "bg-[color-mix(in_srgb,var(--ui-accent)_72%,transparent)]",
  4: "bg-accent",
};

// ConnectionActivityPanel is the chart of connections per day, a column a week, as many weeks as fit the card.
export function ConnectionActivityPanel({ hours, now = new Date() }: { hours: readonly ConnectionHour[]; now?: Date }) {
  const { t, locale } = useLanguage();
  const chart = useRef<HTMLDivElement>(null);
  const size = useElementSize(chart);
  const weeks = size === null ? fewestWeeks : Math.max(fewestWeeks, Math.min(mostWeeks, Math.floor((size.width + dayGap) / weekWidth)));
  const activity = connectionActivity(hours, now, weeks);
  const month = new Intl.DateTimeFormat(locale, { month: "short" });
  const day = new Intl.DateTimeFormat(locale, { month: "short", day: "numeric", weekday: "short" });
  const range = weeks >= weeksInAYear
    ? t("terminal.overview.activityPastYear")
    : t("terminal.overview.activityMonths", { from: month.format(activity.firstDay), to: month.format(now) });

  return (
    <OverviewPanel
      icon="activity"
      title={t("terminal.overview.activity")}
      footer={<>
        <span>{t("terminal.overview.activitySummary", { connections: activity.connections.toLocaleString(locale), hosts: activity.hosts })}</span>
        <span>{range}</span>
      </>}
    >
      <div ref={chart} className="w-full">
        <div role="img" aria-label={t("terminal.overview.activityChart")} className="grid w-max grid-rows-[repeat(7,12px)] gap-1 [grid-auto-columns:12px]">
          {activity.days.map((each) => (
            <span
              key={each.date.getTime()}
              title={t("terminal.overview.activityDay", { date: day.format(each.date), connections: each.connections })}
              className={`rounded-[3px] ${levelColors[each.level]}`}
              style={{ gridColumn: each.week + 1, gridRow: each.weekday + 1 }}
            />
          ))}
        </div>
      </div>
    </OverviewPanel>
  );
}
