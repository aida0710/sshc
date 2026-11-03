import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { fakeAndroidBridge } from "../android/fakeAndroidBridge";
import type { TerminalSession } from "../api/terminalSessions";
import { localStorageKeys } from "../ui/browserStorageKeys";
import { useNotificationPermissionNotice } from "./useNotificationPermissionNotice";

type Status = Pick<TerminalSession, "programStatus">;

const programSaidSomething: Status = { programStatus: { state: "idle", version: 1 } };
const noProgram: Status = {};

function androidAnswering(permission: string) {
  window.sshcAndroid = fakeAndroidBridge({ notificationPermission: vi.fn(() => permission) });
}

describe("useNotificationPermissionNotice", () => {
  beforeEach(() => window.localStorage.clear());
  afterEach(() => {
    delete window.sshcAndroid;
    window.localStorage.clear();
  });

  it("is due once a program says what it is doing and Android has not been answered", () => {
    androidAnswering("default");
    const { result, rerender } = renderHook((status: Status) => useNotificationPermissionNotice(status), { initialProps: noProgram });
    expect(result.current.due).toBe(false);

    rerender(programSaidSomething);
    expect(result.current.due).toBe(true);
  });

  it("is never due in a browser, which asks in Settings", () => {
    const { result } = renderHook(() => useNotificationPermissionNotice(programSaidSomething));
    expect(result.current.due).toBe(false);
  });

  it.each(["granted", "denied"])("is not due when Android already answers %s", (permission) => {
    androidAnswering(permission);
    const { result } = renderHook(() => useNotificationPermissionNotice(programSaidSomething));
    expect(result.current.due).toBe(false);
  });

  it("stays finished, also for a page loaded later", () => {
    androidAnswering("default");
    const first = renderHook(() => useNotificationPermissionNotice(programSaidSomething));
    act(() => first.result.current.finish());
    expect(first.result.current.due).toBe(false);
    expect(window.localStorage.getItem(localStorageKeys.notificationPermissionNoticeFinished)).not.toBeNull();

    const later = renderHook(() => useNotificationPermissionNotice(programSaidSomething));
    expect(later.result.current.due).toBe(false);
  });
});
