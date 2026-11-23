import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useRunTargetApis } from "./useRunTargetApis";

type Chosen = { alias: string | null; vaultRevision: number };

function render() {
  const bastion: Chosen = { alias: "bastion", vaultRevision: 0 };
  return renderHook(({ alias, vaultRevision }: Chosen) => useRunTargetApis(alias, vaultRevision), { initialProps: bastion });
}

describe("useRunTargetApis", () => {
  it("gives a run target the API it had when the person comes back to it, so its terminals do not start over", () => {
    const { result, rerender } = render();
    const first = result.current;
    rerender({ alias: null, vaultRevision: 0 });
    expect(result.current.sessions).not.toBe(first.sessions);
    rerender({ alias: "bastion", vaultRevision: 0 });
    expect(result.current).toBe(first);
  });

  it("builds the API again once the run target's Vault is unlocked", () => {
    const { result, rerender } = render();
    const locked = result.current;
    rerender({ alias: "bastion", vaultRevision: 1 });
    expect(result.current.sessions).not.toBe(locked.sessions);
  });

  it("ends the reads still waiting on a run target when the person leaves it, and lets it read again afterwards", () => {
    const { result, rerender } = render();
    const waiting = result.current.reads!.signal();
    rerender({ alias: null, vaultRevision: 0 });
    expect(waiting.aborted).toBe(true);
    rerender({ alias: "bastion", vaultRevision: 0 });
    expect(result.current.reads!.signal().aborted).toBe(false);
  });
});
