import { describe, expect, it } from "vitest";
import { relativeDay } from "./relativeDay";

describe("relativeDay", () => {
  const now = new Date(2026, 9, 30, 1, 0);

  it("names today, yesterday and the days before by calendar day", () => {
    expect(relativeDay(new Date(2026, 9, 30, 0, 30).toISOString(), now, "ja")).toBe("今日");
    expect(relativeDay(new Date(2026, 9, 29, 23, 0).toISOString(), now, "ja")).toBe("昨日");
    expect(relativeDay(new Date(2026, 9, 27, 12, 0).toISOString(), now, "en")).toBe("3 days ago");
  });

  it("counts months once a day count is hard to place, and nothing for a value that is not a time", () => {
    expect(relativeDay(new Date(2026, 7, 20).toISOString(), now, "ja")).toBe("2 か月前");
    expect(relativeDay("not a time", now, "ja")).toBe("");
  });
});
