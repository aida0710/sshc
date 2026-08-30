type Disposable = { dispose(): void };
type Parameters = (number | number[])[];

type KittyParser = {
  registerCsiHandler(
    identifier: { prefix: string; final: string },
    handler: (parameters: Parameters) => boolean,
  ): Disposable;
};

const KITTY_KEYBOARD_STATE_STACK_LIMIT = 64;

export type KittyKeyboard = {
  encode(event: KeyboardEvent): string | null;
  reset(): void;
  dispose(): void;
};

const functionalKeys: Readonly<Record<string, { final?: string; code?: number; prefix?: number }>> = {
  ArrowUp: { final: "A" },
  ArrowDown: { final: "B" },
  ArrowRight: { final: "C" },
  ArrowLeft: { final: "D" },
  Home: { final: "H" },
  End: { final: "F" },
  Insert: { final: "~", prefix: 2 },
  Delete: { final: "~", prefix: 3 },
  PageUp: { final: "~", prefix: 5 },
  PageDown: { final: "~", prefix: 6 },
  Enter: { code: 13 },
  Tab: { code: 9 },
  Backspace: { code: 127 },
  Escape: { code: 27 },
};

function modifierParameter(event: KeyboardEvent): number {
  return 1 + (event.shiftKey ? 1 : 0) + (event.altKey ? 2 : 0) + (event.ctrlKey ? 4 : 0);
}

export function encodeIntlYen(event: KeyboardEvent, enabled: boolean): string | null {
  if (!enabled || event.type !== "keydown" || event.isComposing || event.code !== "IntlYen") return null;
  if (event.ctrlKey || event.altKey || event.metaKey) return null;
  return event.shiftKey ? "|" : "\\";
}

function firstParameter(parameters: Parameters, fallback: number): number | null {
  const value = parameters[0] ?? fallback;
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0 ? value : null;
}

export function attachKittyKeyboardProtocol(parser: KittyParser): KittyKeyboard {
  let flags = 0;
  const stack: number[] = [];
  const registrations = [
    parser.registerCsiHandler({ prefix: ">", final: "u" }, (parameters) => {
      const next = firstParameter(parameters, 0);
      if (next === null) return true;
      if (stack.length === KITTY_KEYBOARD_STATE_STACK_LIMIT) stack.shift();
      stack.push(flags);
      flags = next;
      return true;
    }),
    parser.registerCsiHandler({ prefix: "=", final: "u" }, (parameters) => {
      const next = firstParameter(parameters, 0);
      if (next !== null) flags = next;
      return true;
    }),
    parser.registerCsiHandler({ prefix: "<", final: "u" }, (parameters) => {
      const count = Math.max(1, firstParameter(parameters, 1) ?? 1);
      const available = Math.min(count, stack.length);
      for (let index = 0; index < available; index += 1) flags = stack.pop() ?? 0;
      if (count > available) flags = 0;
      return true;
    }),
  ];

  return {
    encode(event) {
      if (flags === 0 || event.type !== "keydown" || event.isComposing) return null;
      if (event.metaKey) return null;
      const key = functionalKeys[event.key];
      if (key === undefined) {
        if (!(event.ctrlKey || event.altKey) || event.key.length !== 1) return null;
        const codePoint = event.key.codePointAt(0);
        return codePoint === undefined ? null : `\u001b[${codePoint};${modifierParameter(event)}u`;
      }
      if (!event.shiftKey && !event.altKey && !event.ctrlKey) return null;
      const modifiers = modifierParameter(event);
      if (key.code !== undefined) return `\u001b[${key.code};${modifiers}u`;
      return `\u001b[${key.prefix ?? 1};${modifiers}${key.final}`;
    },
    reset() {
      flags = 0;
      stack.length = 0;
    },
    dispose() {
      for (const registration of registrations) registration.dispose();
    },
  };
}
