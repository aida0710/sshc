import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, vi } from "vitest";
import { resetSettingsForTests } from "./src/settings/settingsStore";

// Monaco does not run in jsdom. In the unit tests the Monaco field of CodeField (src/editor/MonacoCodeField.tsx) is a
// text area with the same name, text and events; the e2e tests drive the real editor.
vi.mock("./src/editor/MonacoCodeField", async () => {
  const { createElement, useEffect, useRef } = await import("react");
  type FieldProps = {
    label: string;
    value: string;
    onChange: (value: string) => void;
    onBlur?: () => void;
    disabled?: boolean;
    placeholder?: string;
    "aria-describedby"?: string;
    "aria-invalid"?: boolean;
    typingBeforeLoad?: { current: boolean };
    selectedLine?: { line: number; request: number } | null;
  };
  function MonacoCodeField(props: FieldProps) {
    const area = useRef<HTMLTextAreaElement>(null);
    // As the Monaco field does, it takes the focus of the text area it replaced, with the caret at the end.
    useEffect(() => {
      const field = area.current;
      if (props.typingBeforeLoad?.current !== true || field === null) return;
      field.focus();
      field.setSelectionRange(field.value.length, field.value.length);
    }, []);
    // As the Monaco field does, a line handed over is selected whole and the field takes focus.
    useEffect(() => {
      const field = area.current;
      if (props.selectedLine === null || props.selectedLine === undefined || field === null) return;
      const lines = field.value.split("\n");
      const index = Math.min(Math.max(props.selectedLine.line, 1), lines.length) - 1;
      const start = lines.slice(0, index).reduce((total, text) => total + text.length + 1, 0);
      field.focus();
      field.setSelectionRange(start, start + (lines[index]?.length ?? 0));
    }, [props.selectedLine?.request]);
    return createElement("textarea", {
      ref: area,
      "aria-label": props.label,
      "aria-describedby": props["aria-describedby"],
      "aria-invalid": props["aria-invalid"],
      value: props.value,
      disabled: props.disabled,
      placeholder: props.placeholder,
      onChange: (event: { target: { value: string } }) => props.onChange(event.target.value),
      onBlur: props.onBlur,
    });
  }
  return { MonacoCodeField };
});

// jsdom leaves canvas rendering unimplemented. xterm 6.1's DOM renderer
// measures glyph widths on a 2D canvas and will not start without one, so the
// 2D context here measures every character as one 8px cell and ignores
// drawing. WebGL stays unsupported, which is the browser fallback under test.
const testGlyphWidthPx = 8;

function measuringContext(canvas: HTMLCanvasElement): CanvasRenderingContext2D {
  const state: Record<PropertyKey, unknown> = {
    canvas,
    font: "10px monospace",
    measureText: (text: string) => ({ width: [...text].length * testGlyphWidthPx }),
  };
  return new Proxy(state, {
    get: (target, property) => (property in target ? target[property] : () => undefined),
    set: (target, property, value) => {
      target[property] = value;
      return true;
    },
  }) as unknown as CanvasRenderingContext2D;
}

Object.defineProperty(HTMLCanvasElement.prototype, "getContext", {
  configurable: true,
  value(this: HTMLCanvasElement, kind: string) {
    return kind === "2d" ? measuringContext(this) : null;
  },
});

// Unmounting runs the effects React still had pending, and one may store a choice (an update result
// shown, a folded notice). Storage is cleared after that, so no test starts with what the last one stored.
afterEach(() => {
  cleanup();
  localStorage.clear();
  sessionStorage.clear();
  resetSettingsForTests();
});
