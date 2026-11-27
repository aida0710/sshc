import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { fakeAndroidBridge } from "./fakeAndroidBridge";
import { parseScreenCorners, useScreenCorners } from "./screenCorners";

const roundedBelowTheBars = '{"topLeft":[43,13],"topRight":[43,13],"bottomRight":[43,25],"bottomLeft":[43,25]}';
const keyboardOpen = '{"topLeft":[43,13],"topRight":[43,13],"bottomRight":[0,0],"bottomLeft":[0,0]}';

afterEach(() => {
  delete window.sshcAndroid;
});

describe("parseScreenCorners", () => {
  it("reads how far each corner reaches into the page", () => {
    expect(parseScreenCorners(roundedBelowTheBars)).toEqual({
      topLeft: [43, 13],
      topRight: [43, 13],
      bottomRight: [43, 25],
      bottomLeft: [43, 25],
    });
  });

  it("reads nothing from a value of another shape", () => {
    expect(parseScreenCorners("")).toBeNull();
    expect(parseScreenCorners('{"topLeft":[43,13]}')).toBeNull();
    expect(parseScreenCorners('{"topLeft":[43],"topRight":[0,0],"bottomRight":[0,0],"bottomLeft":[0,0]}')).toBeNull();
    expect(parseScreenCorners('{"topLeft":[-1,0],"topRight":[0,0],"bottomRight":[0,0],"bottomLeft":[0,0]}')).toBeNull();
  });
});

describe("useScreenCorners", () => {
  it("gives nothing in a browser", () => {
    expect(renderHook(() => useScreenCorners()).result.current).toBeNull();
  });

  it("reads the corners again when the app says they changed", () => {
    const screenCorners = vi.fn(() => roundedBelowTheBars);
    window.sshcAndroid = fakeAndroidBridge({ screenCorners });
    const { result } = renderHook(() => useScreenCorners());
    expect(result.current?.bottomRight).toEqual([43, 25]);

    screenCorners.mockReturnValue(keyboardOpen);
    act(() => {
      window.dispatchEvent(new CustomEvent("sshc-android-screen-corners"));
    });
    expect(result.current?.bottomRight).toEqual([0, 0]);
    expect(result.current?.topLeft).toEqual([43, 13]);
  });

  it("gives nothing when the app's bridge fails", () => {
    window.sshcAndroid = fakeAndroidBridge({ screenCorners: vi.fn(() => { throw new Error("gone"); }) });
    expect(renderHook(() => useScreenCorners()).result.current).toBeNull();
  });
});
