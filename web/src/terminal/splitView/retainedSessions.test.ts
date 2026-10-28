import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { defaultRetainedTerminals, useRetainedSessions } from "./retainedSessions";

describe("the sessions kept hidden after they leave the split view", () => {
  function shownInTurn(panesInTurn: string[][], limit = defaultRetainedTerminals) {
    const existing = new Set(panesInTurn.flat());
    const { result, rerender } = renderHook(({ panes }: { panes: string[] }) => useRetainedSessions(panes, existing, limit), {
      initialProps: { panes: panesInTurn[0]! },
    });
    for (const panes of panesInTurn.slice(1)) rerender({ panes });
    return result.current;
  }

  it("keeps the sessions shown before, most recent first, in the same render they leave", () => {
    expect(shownInTurn([["a"], ["b"]])).toEqual(["a"]);
    expect(shownInTurn([["a"], ["b"], ["c"]])).toEqual(["b", "a"]);
    expect(shownInTurn([["a"], ["b"], ["a"]])).toEqual(["b"]);
    expect(shownInTurn([["a", "b"], ["c"]])).toEqual(["a", "b"]);
  });

  it.each([1, defaultRetainedTerminals, 20])("keeps as many as the setting says (%i), dropping the one shown longest ago", (limit) => {
    const sessions = Array.from({ length: limit + 2 }, (_, index) => `s${index}`);
    expect(shownInTurn(sessions.map((id) => [id]), limit)).toEqual(sessions.slice(0, -1).reverse().slice(0, limit));
  });

  it("lets go of a session that was closed", () => {
    const { result, rerender } = renderHook(({ panes, existing }: { panes: string[]; existing: Set<string> }) => useRetainedSessions(panes, existing, defaultRetainedTerminals), {
      initialProps: { panes: ["a"], existing: new Set(["a", "b"]) },
    });
    rerender({ panes: ["b"], existing: new Set(["a", "b"]) });
    expect(result.current).toEqual(["a"]);

    rerender({ panes: ["b"], existing: new Set(["b"]) });
    expect(result.current).toEqual([]);
  });
});
