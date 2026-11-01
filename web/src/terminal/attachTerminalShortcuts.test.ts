import { afterEach, expect, it, vi } from "vitest";
import { defaultBindings } from "../keyconfig/bindings";
import { attachTerminalShortcuts } from "./attachTerminalShortcuts";

afterEach(() => {
  document.body.replaceChildren();
});

function press(target: HTMLElement, init: KeyboardEventInit) {
  const event = new KeyboardEvent("keydown", { bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(event);
  return event;
}

it("runs a terminal's shortcut pressed in it, and keeps the key from the program", () => {
  const container = document.createElement("div");
  const textarea = document.createElement("textarea");
  container.append(textarea);
  document.body.append(container);
  const typed = vi.fn();
  textarea.addEventListener("keydown", typed);
  const increase = vi.fn();
  const reset = vi.fn();
  const detach = attachTerminalShortcuts(container, { increaseFontSize: increase, resetFontSize: reset });

  const larger = press(textarea, { key: "=", ctrlKey: true });
  press(textarea, { key: "0", ctrlKey: true });
  press(textarea, { key: "a", ctrlKey: true });

  expect(defaultBindings.increaseFontSize).toContain("Ctrl+=");
  expect(increase).toHaveBeenCalledTimes(1);
  expect(reset).toHaveBeenCalledTimes(1);
  expect(larger.defaultPrevented).toBe(true);
  expect(typed).toHaveBeenCalledTimes(1);

  detach();
  press(textarea, { key: "=", ctrlKey: true });
  expect(increase).toHaveBeenCalledTimes(1);
});

it("steps the text size on every repeat of a held key", () => {
  const container = document.createElement("div");
  document.body.append(container);
  const increase = vi.fn();
  attachTerminalShortcuts(container, { increaseFontSize: increase });

  press(container, { key: "=", ctrlKey: true });
  press(container, { key: "=", ctrlKey: true, repeat: true });
  expect(increase).toHaveBeenCalledTimes(2);
});
