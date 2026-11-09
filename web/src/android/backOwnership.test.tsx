import { act, render } from "@testing-library/react";
import { useRef } from "react";
import { describe, expect, it } from "vitest";
import { useDismissibleLayer } from "../ui/useDismissibleLayer";
import { fakeAndroidBridge } from "./fakeAndroidBridge";
import { useBackOwnership } from "./backOwnership";

function appWindow() {
  const bridge = fakeAndroidBridge();
  return { bridge, target: { sshcAndroid: bridge } as unknown as Window };
}

function Page({ onTopOfHome, menuOpen, target }: { onTopOfHome: boolean; menuOpen: boolean; target: Window }) {
  useBackOwnership(onTopOfHome, target);
  const menu = useRef<HTMLDivElement>(null);
  useDismissibleLayer({ open: menuOpen, containerRefs: [menu], onDismiss: () => undefined });
  return <div ref={menu}>menu</div>;
}

describe("useBackOwnership", () => {
  it("leaves Back to Android on the top of Home with nothing open, and takes it back while a menu is open", () => {
    const { bridge, target } = appWindow();
    const { rerender } = render(<Page onTopOfHome menuOpen={false} target={target} />);
    expect(bridge.setBackTakenByPage).toHaveBeenLastCalledWith(false);
    act(() => rerender(<Page onTopOfHome menuOpen target={target} />));
    expect(bridge.setBackTakenByPage).toHaveBeenLastCalledWith(true);
    act(() => rerender(<Page onTopOfHome menuOpen={false} target={target} />));
    expect(bridge.setBackTakenByPage).toHaveBeenLastCalledWith(false);
  });

  it("takes Back anywhere but the top of Home", () => {
    const { bridge, target } = appWindow();
    render(<Page onTopOfHome={false} menuOpen={false} target={target} />);
    expect(bridge.setBackTakenByPage).toHaveBeenLastCalledWith(true);
  });

  it("does not repeat what it already said", () => {
    const { bridge, target } = appWindow();
    const { rerender } = render(<Page onTopOfHome={false} menuOpen={false} target={target} />);
    act(() => rerender(<Page onTopOfHome={false} menuOpen target={target} />));
    expect(bridge.setBackTakenByPage).toHaveBeenCalledTimes(1);
  });
});
