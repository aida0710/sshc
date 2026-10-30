const dayMilliseconds = 24 * 60 * 60 * 1000;
// Past this many days a day count is hard to place; months read better ("2か月前").
const daysBeforeMonths = 30;
const daysPerMonth = 30;

// relativeDay names the day of value against now in the locale's own words: 今日, 昨日, 3日前, 2か月前.
// Days are counted between calendar days where the browser is, not in 24-hour spans, so a connection at
// 23:00 is 昨日 at 01:00.
export function relativeDay(value: string, now: Date, locale?: string): string {
  const moment = new Date(value);
  if (Number.isNaN(moment.getTime())) return "";
  const startOf = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  const days = Math.round((startOf(now) - startOf(moment)) / dayMilliseconds);
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  if (days < daysBeforeMonths) return format.format(-Math.max(0, days), "day");
  return format.format(-Math.floor(days / daysPerMonth), "month");
}
