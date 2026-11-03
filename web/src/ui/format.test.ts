import { describe, expect, it } from "vitest";
import { translatorFor } from "../testing/translator";
import { formatBytes, formatDateTime, formatDuration, formatOptionalDateTime, formatTimeOfDay } from "./format";

describe("formatBytes", () => {
  it.each([
    [0, "0 B"],
    [512, "512 B"],
    [1024, "1.0 KiB"],
    [1536, "1.5 KiB"],
    [5 << 20, "5.0 MiB"],
    [(1 << 30) * 2.25, "2.3 GiB"],
    [2 ** 50 * 3, "3.0 PiB"],
  ])("shows %d bytes as %s in binary units", (bytes, expected) => {
    expect(formatBytes(bytes, { locale: "en" })).toBe(expected);
  });

  it.each([
    [999, "999 B"],
    [1_000, "1 kB"],
    [1_500, "1.5 kB"],
    [4_800_000, "4.8 MB"],
  ])("shows %d bytes as %s in decimal units", (bytes, expected) => {
    expect(formatBytes(bytes, { decimal: true, locale: "en" })).toBe(expected);
  });

  it("never shows a negative size", () => {
    expect(formatBytes(-40, { locale: "en" })).toBe("0 B");
  });
});

describe("formatDateTime", () => {
  it("returns a value that is not a date unchanged", () => {
    expect(formatDateTime("never")).toBe("never");
  });

  // Built from local parts, so the expected day does not depend on the time zone the test runs in.
  const now = new Date(2026, 10, 3, 18, 30);
  const at = (...parts: [number, number, number, number, number]) => new Date(...parts).toISOString();

  it("names today and yesterday by calendar day, with the time", () => {
    expect(formatDateTime(at(2026, 10, 3, 17, 49), "ja-JP", now)).toBe("今日 17:49");
    expect(formatDateTime(at(2026, 10, 3, 0, 1), "ja-JP", now)).toBe("今日 0:01");
    expect(formatDateTime(at(2026, 10, 2, 23, 59), "ja-JP", now)).toBe("昨日 23:59");
    expect(formatDateTime(at(2026, 10, 2, 9, 2), "en-US", now)).toBe("Yesterday 9:02 AM");
    expect(formatDateTime(at(2026, 10, 3, 17, 49), "en-US", now)).toBe("Today 5:49 PM");
  });

  it("writes an older day as month and day, and adds the year only when it is not this year", () => {
    expect(formatDateTime(at(2026, 10, 1, 9, 2), "ja-JP", now)).toBe("11/1 9:02");
    expect(formatDateTime(at(2025, 10, 3, 17, 49), "ja-JP", now)).toBe("2025/11/3 17:49");
    expect(formatDateTime(at(2026, 10, 1, 9, 2), "en-US", now)).toBe("11/1, 9:02 AM");
  });

  it("treats the turn of a month and a year as yesterday", () => {
    expect(formatDateTime(at(2025, 11, 31, 23, 0), "ja-JP", new Date(2026, 0, 1, 1, 0))).toBe("昨日 23:00");
  });

  it("does not call a moment in the future today", () => {
    expect(formatDateTime(at(2026, 10, 4, 9, 0), "ja-JP", now)).toBe("11/4 9:00");
  });
});

describe("formatTimeOfDay", () => {
  it("shows the seconds so that steps within one minute read apart", () => {
    const moment = new Date(2026, 9, 19, 18, 8, 12);
    expect(formatTimeOfDay(moment, "en-US")).toBe("6:08:12 PM");
    expect(formatTimeOfDay(moment, "ja-JP")).toBe("18:08:12");
  });
});

describe("formatDuration", () => {
  it.each([
    [45, "45s"],
    [90, "2m"],
    [3_660, "1h 1m"],
    [3_541, "1h 0m"],
    [3_599, "1h 0m"],
    [7_199, "2h 0m"],
  ])("shows %d seconds as %s on an English screen", (seconds, expected) => {
    expect(formatDuration(seconds, translatorFor("en"))).toBe(expected);
  });

  it.each([
    [45, "45秒"],
    [300, "5分"],
    [5_400, "1時間30分"],
  ])("shows %d seconds as %s on a Japanese screen", (seconds, expected) => {
    expect(formatDuration(seconds, translatorFor("ja"))).toBe(expected);
  });
});

describe("formatOptionalDateTime", () => {
  it("shows a dash when no moment was recorded", () => {
    expect(formatOptionalDateTime(undefined)).toBe("—");
  });

  it("formats a recorded moment like formatDateTime", () => {
    expect(formatOptionalDateTime("2026-09-18T04:05:00Z", "en-US")).toBe(formatDateTime("2026-09-18T04:05:00Z", "en-US"));
  });
});
