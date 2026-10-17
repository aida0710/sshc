import { vi } from "vitest";
import type { AndroidBridge } from "./native";

// A stand-in for the Android app's bridge in unit tests. Every method answers
// as an idle app would; a test overrides only the ones it exercises.
export function fakeAndroidBridge(overrides: Partial<AndroidBridge> = {}): AndroidBridge {
  return {
    chooseSave: vi.fn(() => ""),
    writeSaveChunk: vi.fn(() => ""),
    finishSave: vi.fn(() => ""),
    cancelSave: vi.fn(),
    notifyTransfer: vi.fn(),
    setAppearance: vi.fn(),
    notificationPermission: vi.fn(() => "granted"),
    requestNotificationPermission: vi.fn(),
    showNotification: vi.fn(() => ""),
    takeOpenedNotification: vi.fn(() => ""),
    chooseFolder: vi.fn(() => ""),
    takeChosenFolder: vi.fn(() => ""),
    releaseChosenFolder: vi.fn(),
    ...overrides,
  };
}
