import { describe, expect, it } from "vitest";
import { aquariumFish } from "./aquariumFish";

const recent = (alias: string) => ({ alias, hostName: `${alias}.internal`, user: "aida", port: "22", lastConnectedAt: "2026-11-08T10:00:00Z" });

describe("aquariumFish", () => {
  it("weighs each recent connection by the hours it was opened in over the last 30 days", () => {
    const fish = aquariumFish({
      recent: [recent("web-01"), recent("db"), recent("nas")],
      hours: [
        { hour: "2026-09-01T00:00:00Z", connections: 9, aliases: ["nas"] },
        { hour: "2026-11-01T00:00:00Z", connections: 3, aliases: ["web-01", "db"] },
        { hour: "2026-11-07T05:00:00Z", connections: 1, aliases: ["web-01"] },
      ],
    }, new Date("2026-11-08T12:00:00Z"));

    expect(fish).toEqual([{ label: "web-01", weight: 2 }, { label: "db", weight: 1 }, { label: "nas", weight: 0 }]);
  });

  it("has no fish before the history is read", () => {
    expect(aquariumFish(null, new Date())).toEqual([]);
  });
});
