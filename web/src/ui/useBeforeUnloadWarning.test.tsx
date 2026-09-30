import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useBeforeUnloadWarning } from "./useBeforeUnloadWarning";

function closePage(): Event {
  const event = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(event);
  return event;
}

describe("useBeforeUnloadWarning", () => {
  it("asks before the page closes only while active", () => {
    const { rerender, unmount } = renderHook(({ active }) => useBeforeUnloadWarning(active), { initialProps: { active: false } });
    expect(closePage().defaultPrevented).toBe(false);
    rerender({ active: true });
    expect(closePage().defaultPrevented).toBe(true);
    rerender({ active: false });
    expect(closePage().defaultPrevented).toBe(false);
    rerender({ active: true });
    unmount();
    expect(closePage().defaultPrevented).toBe(false);
  });
});
