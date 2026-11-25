import { describe, expect, it } from "vitest";
import { remoteSessionColorOf, remoteSessionFrameColor, withRemoteSessionColor } from "./remoteSessionColor";

describe("remoteSessionFrameColor", () => {
  it("takes the colour given to the connection before the one in the settings", () => {
    expect(remoteSessionFrameColor("#2f7fd8", "#22aa55")).toBe("#2f7fd8");
  });

  it("takes the colour in the settings for a connection with none", () => {
    expect(remoteSessionFrameColor(undefined, "#22aa55")).toBe("#22aa55");
    expect(remoteSessionFrameColor("", "#22aa55")).toBe("#22aa55");
  });

  // null は、テーマの --ui-remote（さし色）のままにすること。
  it("leaves the accent colour when neither the connection nor the settings give one", () => {
    expect(remoteSessionFrameColor(undefined, null)).toBeNull();
  });
});

describe("withRemoteSessionColor", () => {
  it("keeps a chosen colour and drops it again to go back to the accent colour", () => {
    const chosen = withRemoteSessionColor({ id: "standard", appearance: { theme: "dark" } }, "#22aa55");
    expect(remoteSessionColorOf(chosen)).toBe("#22aa55");
    const back = withRemoteSessionColor(chosen, null);
    expect(back.appearance).toEqual({ theme: "dark" });
    expect(remoteSessionColorOf(back)).toBeNull();
  });
});
