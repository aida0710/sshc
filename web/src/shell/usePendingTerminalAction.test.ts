import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { registerTerminalActions, terminalActionNames, type TerminalActions } from "../terminal/terminalActions";
import { usePendingTerminalAction } from "./usePendingTerminalAction";

function registeredActions(sessionId: string): { actions: TerminalActions; unregister: () => void } {
  const actions = Object.fromEntries(terminalActionNames.map((name) => [name, vi.fn()])) as unknown as TerminalActions;
  return { actions, unregister: registerTerminalActions(sessionId, actions) };
}

describe("usePendingTerminalAction", () => {
  let unregister = () => {};
  afterEach(() => unregister());

  it("waits for the terminal screen before running the action on the session shown", () => {
    const registered = registeredActions("one");
    unregister = registered.unregister;
    const navigate = vi.fn(() => true);
    const { result, rerender } = renderHook((props: { terminalFace: boolean }) => usePendingTerminalAction({ ...props, activeSessionId: "one", navigate }), { initialProps: { terminalFace: false } });

    act(() => result.current("quickCommands"));
    expect(navigate).toHaveBeenCalledWith("Terminal");
    expect(registered.actions.quickCommands).not.toHaveBeenCalled();

    rerender({ terminalFace: true });
    expect(registered.actions.quickCommands).toHaveBeenCalledOnce();
    rerender({ terminalFace: true });
    expect(registered.actions.quickCommands).toHaveBeenCalledOnce();
  });

  it("drops the action when a page with unsaved changes keeps the screen", () => {
    const registered = registeredActions("one");
    unregister = registered.unregister;
    const { result, rerender } = renderHook((props: { terminalFace: boolean }) => usePendingTerminalAction({ ...props, activeSessionId: "one", navigate: () => false }), { initialProps: { terminalFace: false } });

    act(() => result.current("copy"));
    rerender({ terminalFace: true });
    expect(registered.actions.copy).not.toHaveBeenCalled();
  });
});
