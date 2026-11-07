import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/client";
import type { OpenTerminalSessionRequest } from "../api/terminalSessions";
import { useOpenAfterRemoteUnlock } from "./useOpenAfterRemoteUnlock";

const shell: OpenTerminalSessionRequest = { kind: "shell" };
const locked = new ApiError("remote_engine_vault_locked", 423, { code: "remote_engine_vault_locked", message: "request rejected" });
const unreachable = new ApiError("remote_engine_unreachable", 502, { code: "remote_engine_unreachable", message: "request rejected" });

function render(initial: { runTarget: string | null; refusal: ApiError | null; vaultRevision: number }) {
  const open = vi.fn(async () => null);
  const hook = renderHook((props: typeof initial) => useOpenAfterRemoteUnlock({ ...props, open }), { initialProps: initial });
  return { open, ...hook };
}

describe("useOpenAfterRemoteUnlock", () => {
  it("opens the session the locked Vault refused once that Vault is unlocked", () => {
    const { open, result, rerender } = render({ runTarget: "r540", refusal: null, vaultRevision: 0 });
    act(() => result.current.noteRefused(shell));
    rerender({ runTarget: "r540", refusal: locked, vaultRevision: 0 });
    expect(open).not.toHaveBeenCalled();
    rerender({ runTarget: "r540", refusal: null, vaultRevision: 1 });
    expect(open).toHaveBeenCalledExactlyOnceWith(shell);
    rerender({ runTarget: "r540", refusal: null, vaultRevision: 2 });
    expect(open).toHaveBeenCalledOnce();
  });

  it("forgets it when the engine refused for another reason", () => {
    const { open, result, rerender } = render({ runTarget: "r540", refusal: null, vaultRevision: 0 });
    act(() => result.current.noteRefused(shell));
    rerender({ runTarget: "r540", refusal: unreachable, vaultRevision: 0 });
    rerender({ runTarget: "r540", refusal: null, vaultRevision: 1 });
    expect(open).not.toHaveBeenCalled();
  });

  it("forgets it when another engine is chosen", () => {
    const { open, result, rerender } = render({ runTarget: "r540", refusal: locked, vaultRevision: 0 });
    act(() => result.current.noteRefused(shell));
    rerender({ runTarget: "nas", refusal: locked, vaultRevision: 0 });
    rerender({ runTarget: "nas", refusal: null, vaultRevision: 1 });
    expect(open).not.toHaveBeenCalled();
  });

  it("remembers nothing for this machine, whose sessions no remote Vault holds back", () => {
    const { open, result, rerender } = render({ runTarget: null, refusal: null, vaultRevision: 0 });
    act(() => result.current.noteRefused(shell));
    rerender({ runTarget: null, refusal: null, vaultRevision: 1 });
    expect(open).not.toHaveBeenCalled();
  });
});
