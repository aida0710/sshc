import { describe, expect, it } from "vitest";
import { formatBytes, formatDateTime } from "./format";

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
