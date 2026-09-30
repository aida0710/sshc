// Numbers and moments shown to people, spelled once so the same value reads
// the same on every screen.

import type { Translate } from "../i18n/context";

export type ByteUnits = {
  // Object storage bills in decimal units (kB, MB); files on disk are shown in
  // the binary units (KiB, MiB) their tools use.
  decimal?: boolean;
  // BCP 47 tag for digit grouping and the decimal separator; the browser's
  // own locale when omitted.
  locale?: string;
};

const binaryUnits = ["B", "KiB", "MiB", "GiB", "TiB"];
const decimalUnits = ["B", "kB", "MB", "GB", "TB"];

export function formatBytes(bytes: number, units: ByteUnits = {}): string {
  const names = units.decimal ? decimalUnits : binaryUnits;
  const step = units.decimal ? 1000 : 1024;
  let value = Math.max(0, bytes);
  let unit = 0;
  while (value >= step && unit < names.length - 1) {
    value /= step;
    unit++;
  }
  // File sizes sit in columns, where "2.0 KiB" above "1.5 KiB" lines up;
  // a storage total stands alone, so "1 kB" reads better than "1.0 kB".
  const fraction = unit === 0 ? 0 : 1;
  const digits = new Intl.NumberFormat(units.locale, {
    minimumFractionDigits: units.decimal ? 0 : fraction,
    maximumFractionDigits: fraction,
  }).format(value);
  return `${digits} ${names[unit]}`;
}

// A timestamp from the engine as a short local date and time. A value that
// is not a date is shown as it came, so a bad record is visible, not hidden.
export function formatDateTime(value: string, locale?: string): string {
  const moment = new Date(value);
  if (Number.isNaN(moment.getTime())) return value;
  return new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(moment);
}

const secondsPerMinute = 60;
const minutesPerHour = 60;

// A remaining time or an interval in whole seconds, in the largest units that
// keep it short. Minutes are rounded up, so a time left never reads shorter
// than it is; the rounding comes before the split into hours, so 59m 59s
// reads 1h 0m, not 60m. The unit names come from the catalogue.
export function formatDuration(seconds: number, t: Translate): string {
  if (seconds < secondsPerMinute) return t("duration.seconds", { seconds });
  const totalMinutes = Math.ceil(seconds / secondsPerMinute);
  if (totalMinutes < minutesPerHour) return t("duration.minutes", { minutes: totalMinutes });
  return t("duration.hoursMinutes", {
    hours: Math.floor(totalMinutes / minutesPerHour),
    minutes: totalMinutes % minutesPerHour,
  });
}

// A moment the engine may not have recorded yet. A dash keeps the sentence
// around it readable instead of leaving a gap where the time would go.
export function formatOptionalDateTime(value: string | undefined, locale?: string): string {
  return value === undefined ? "—" : formatDateTime(value, locale);
}
