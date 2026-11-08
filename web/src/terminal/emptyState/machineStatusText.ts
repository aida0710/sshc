import type { Translate } from "../../i18n/context";

// The names the operating systems go by, for the Go names the engine gives (runtime.GOOS).
const operatingSystemNames: Record<string, string> = {
  linux: "Linux", darwin: "macOS", windows: "Windows", freebsd: "FreeBSD", openbsd: "OpenBSD", android: "Android",
};

export function operatingSystemName(goos: string): string {
  return operatingSystemNames[goos] ?? goos;
}

const secondsPerMinute = 60;
const secondsPerHour = 60 * secondsPerMinute;
const secondsPerDay = 24 * secondsPerHour;

// uptimeText is how long the machine has been up, in its largest whole unit: 12日, 5時間, 30分.
export function uptimeText(seconds: number, t: Translate): string {
  if (seconds >= secondsPerDay) return t("duration.days", { days: Math.floor(seconds / secondsPerDay) });
  if (seconds >= secondsPerHour) return t("duration.hours", { hours: Math.floor(seconds / secondsPerHour) });
  return t("duration.minutes", { minutes: Math.max(1, Math.floor(seconds / secondsPerMinute)) });
}
