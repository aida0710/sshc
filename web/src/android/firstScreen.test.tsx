import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PanelState } from "../ui/PanelState";
import { fakeAndroidBridge } from "./fakeAndroidBridge";
import { useFirstScreenReport } from "./firstScreen";

function appWindow() {
  const bridge = fakeAndroidBridge();
  return { bridge, target: { sshcAndroid: bridge, document } as unknown as Window };
}

function Screen({ settled, reading, target }: { settled: boolean; reading: boolean; target: Window }) {
  useFirstScreenReport(settled, target);
  return reading ? <PanelState tone="loading" title="Reading" /> : <p>Home</p>;
}

describe("useFirstScreenReport", () => {
  it("tells the app once the settled screen shows its contents", () => {
    const { bridge, target } = appWindow();
    render(<Screen settled reading={false} target={target} />);
    expect(bridge.firstScreenShown).toHaveBeenCalledTimes(1);
  });

  it("waits while the screen still shows that it is reading", () => {
    const { bridge, target } = appWindow();
    const { rerender } = render(<Screen settled reading target={target} />);
    expect(bridge.firstScreenShown).not.toHaveBeenCalled();
    rerender(<Screen settled reading={false} target={target} />);
    return Promise.resolve().then(() => expect(bridge.firstScreenShown).toHaveBeenCalledTimes(1));
  });

  it("says nothing before the app knows what to show", () => {
    const { bridge, target } = appWindow();
    render(<Screen settled={false} reading={false} target={target} />);
    expect(bridge.firstScreenShown).not.toHaveBeenCalled();
  });

  it("says it only once, even when a later screen reads again", () => {
    const { bridge, target } = appWindow();
    const { rerender } = render(<Screen settled reading={false} target={target} />);
    rerender(<Screen settled reading target={target} />);
    rerender(<Screen settled reading={false} target={target} />);
    expect(bridge.firstScreenShown).toHaveBeenCalledTimes(1);
  });
});
