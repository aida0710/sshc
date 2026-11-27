import { render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeAndroidBridge } from "../android/fakeAndroidBridge";
import { LanguageProvider } from "../i18n/context";
import { RemoteSessionFrame } from "./RemoteSessionFrame";

function renderFrame() {
  render(<LanguageProvider initial="en"><RemoteSessionFrame alias="r100" color={null} /></LanguageProvider>);
  return document.querySelector<HTMLElement>("[data-remote-session-frame]")!;
}

afterEach(() => {
  delete window.sshcAndroid;
});

describe("RemoteSessionFrame", () => {
  it("rounds its corners into the screen's rounded corners in the Android app", () => {
    window.sshcAndroid = fakeAndroidBridge({
      screenCorners: vi.fn(() => '{"topLeft":[43,13],"topRight":[43,13],"bottomRight":[43,25],"bottomLeft":[0,0]}'),
    });
    const frame = renderFrame();

    expect(frame.style.borderTopLeftRadius).toBe("43px 13px");
    expect(frame.style.borderTopRightRadius).toBe("43px 13px");
    expect(frame.style.borderBottomRightRadius).toBe("43px 25px");
    expect(frame.style.borderBottomLeftRadius).toBe("0px 0px");
  });

  it("keeps square corners in a browser", () => {
    const frame = renderFrame();

    expect(frame.style.borderTopLeftRadius).toBe("");
    expect(frame.style.borderBottomRightRadius).toBe("");
  });
});
