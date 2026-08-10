import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSectionRoute } from "./useSectionRoute";

afterEach(() => {
  window.history.replaceState(null, "", "/");
  vi.restoreAllMocks();
});

describe("useSectionRoute", () => {
  it("reads a direct deep link and pushes a new section URL", () => {
    window.history.replaceState(null, "", "/connections");
    const pushed = vi.spyOn(window.history, "pushState");
    const { result } = renderHook(() => useSectionRoute());

    expect(result.current.route).toMatchObject({ kind: "section", section: "Connections" });

    act(() => result.current.navigate("Keys"));

    expect(pushed).toHaveBeenCalledWith(null, "", "/keys");
    expect(window.location.pathname).toBe("/keys");
    expect(result.current.route).toMatchObject({ kind: "section", section: "Keys" });
  });

  it("reparses the real pathname rather than history state on popstate", () => {
    const { result } = renderHook(() => useSectionRoute());

    act(() => {
      window.history.pushState({ section: "Connections" }, "", "/history");
      window.dispatchEvent(new PopStateEvent("popstate", { state: { section: "Connections" } }));
    });

    expect(result.current.route).toMatchObject({ kind: "section", section: "History" });
  });

  it("canonicalizes one trailing slash without adding history and retains the query", () => {
    window.history.replaceState(null, "", "/connections/?source=test");
    const pushed = vi.spyOn(window.history, "pushState");
    const replaced = vi.spyOn(window.history, "replaceState");

    const { result } = renderHook(() => useSectionRoute());

    expect(window.location.pathname).toBe("/connections");
    expect(window.location.search).toBe("?source=test");
    expect(replaced).toHaveBeenCalledWith(null, "", "/connections?source=test");
    expect(pushed).not.toHaveBeenCalled();
    expect(result.current.route).toEqual({
      kind: "section",
      section: "Connections",
      canonicalPath: "/connections",
      canonical: true,
    });
  });

  it("keeps an unknown URL unchanged", () => {
    window.history.replaceState(null, "", "/missing?source=test");
    const pushed = vi.spyOn(window.history, "pushState");
    const replaced = vi.spyOn(window.history, "replaceState");

    const { result } = renderHook(() => useSectionRoute());

    expect(result.current.route).toEqual({ kind: "not-found", pathname: "/missing" });
    expect(window.location.pathname).toBe("/missing");
    expect(window.location.search).toBe("?source=test");
    expect(pushed).not.toHaveBeenCalled();
    expect(replaced).not.toHaveBeenCalled();
  });

  it("does not duplicate the current route and removes non-route URL data", () => {
    window.history.replaceState(null, "", "/keys?source=test#panel");
    const pushed = vi.spyOn(window.history, "pushState");
    const replaced = vi.spyOn(window.history, "replaceState");
    const { result } = renderHook(() => useSectionRoute());

    act(() => result.current.navigate("Keys"));

    expect(pushed).not.toHaveBeenCalled();
    expect(replaced).toHaveBeenCalledWith(null, "", "/keys");
    expect(window.location.href).toBe(`${window.location.origin}/keys`);
  });
});
