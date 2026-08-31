import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef, useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { useDismissibleLayer } from "./useDismissibleLayer";

function Layer({ name, close }: { name: string; close: () => void }) {
  const panel = useRef<HTMLDivElement>(null);
  useDismissibleLayer({ open: true, containerRefs: [panel], onDismiss: close });
  return <div ref={panel}>{name}</div>;
}

describe("useDismissibleLayer", () => {
  it("replaces the previous layer and dismisses only the topmost layer afterward", () => {
    const lower = vi.fn();
    const upper = vi.fn();
    render(<><Layer name="lower" close={lower} /><Layer name="upper" close={upper} /></>);

    expect(lower).toHaveBeenCalledOnce();
    lower.mockClear();
    fireEvent.pointerDown(document.body);

    expect(upper).toHaveBeenCalledOnce();
    expect(lower).not.toHaveBeenCalled();
  });

  it("preserves the outside click target but restores the trigger for Escape", async () => {
    function Fixture() {
      const [open, setOpen] = useState(false);
      const trigger = useRef<HTMLButtonElement>(null);
      const panel = useRef<HTMLDivElement>(null);
      useDismissibleLayer({
        open,
        containerRefs: [panel, trigger],
        onDismiss: () => setOpen(false),
        returnFocusRef: trigger,
      });
      return <><button ref={trigger} onClick={() => setOpen((value) => !value)}>Trigger</button>{open ? <div ref={panel}>Panel</div> : null}<button>Outside</button></>;
    }
    const user = userEvent.setup();
    render(<Fixture />);
    const trigger = screen.getByRole("button", { name: "Trigger" });
    const outside = screen.getByRole("button", { name: "Outside" });

    await user.click(trigger);
    await user.click(outside);
    expect(screen.queryByText("Panel")).not.toBeInTheDocument();
    expect(outside).toHaveFocus();

    await user.click(trigger);
    await user.keyboard("{Escape}");
    expect(screen.queryByText("Panel")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("consumes Android back before a lower app handler", () => {
    const close = vi.fn();
    const lower = vi.fn();
    window.addEventListener("sshc-android-back", lower);
    render(<Layer name="menu" close={close} />);
    const back = new Event("sshc-android-back", { cancelable: true });

    void act(() => window.dispatchEvent(back));

    expect(back.defaultPrevented).toBe(true);
    expect(close).toHaveBeenCalledOnce();
    expect(lower).not.toHaveBeenCalled();
    window.removeEventListener("sshc-android-back", lower);
  });
});
