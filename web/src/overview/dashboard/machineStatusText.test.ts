import { describe, expect, it } from "vitest";
import type { Translate } from "../../i18n/context";
import { operatingSystemName, uptimeText } from "./machineStatusText";

const t = ((key: string, values: Record<string, unknown> = {}) => `${key}:${Object.values(values).join(",")}`) as Translate;

describe("machine status text", () => {
  it("says the uptime in its largest whole unit", () => {
    expect(uptimeText(12 * 24 * 60 * 60 + 3600, t)).toBe("duration.days:12");
    expect(uptimeText(5 * 60 * 60 + 59 * 60, t)).toBe("duration.hours:5");
    expect(uptimeText(30 * 60 + 10, t)).toBe("duration.minutes:30");
    expect(uptimeText(20, t)).toBe("duration.minutes:1");
  });

  it("names the operating systems as their makers do, and keeps a name it does not know", () => {
    expect(operatingSystemName("darwin")).toBe("macOS");
    expect(operatingSystemName("openbsd")).toBe("OpenBSD");
    expect(operatingSystemName("plan9")).toBe("plan9");
  });
});
