import { renderHook } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { LanguageProvider, useTranslate, type Translate } from "../i18n/context";
import type { Locale } from "../i18n/locale";
import { formatBytes, formatDateTime, formatDuration, formatOptionalDateTime } from "./format";

function translatorFor(locale: Locale): Translate {
  const { result } = renderHook(() => useTranslate(), {
    wrapper: ({ children }: { children: ReactNode }) => createElement(LanguageProvider, { initial: locale, children }),
  });
  return result.current;
}

describe("formatBytes", () => {
  it.each([
    [0, "0 B"],
    [512, "512 B"],
    [1024, "1.0 KiB"],
    [1536, "1.5 KiB"],
    [5 << 20, "5.0 MiB"],
    [(1 << 30) * 2.25, "2.3 GiB"],
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

  it("formats a timestamp in the requested locale", () => {
    expect(formatDateTime("2026-09-18T04:05:00Z", "en-US")).toMatch(/Sep 18, 2026/);
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
