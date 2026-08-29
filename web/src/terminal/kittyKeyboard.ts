type Disposable = { dispose(): void };
type Parameters = (number | number[])[];

type KittyParser = {
  registerCsiHandler(
    identifier: { prefix: string; final: string },
    handler: (parameters: Parameters) => boolean,
  ): Disposable;
};

export type KittyKeyboard = {
  encode(event: KeyboardEvent): string | null;
  reset(): void;
  dispose(): void;
};

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
      for (let index = 0; index < count; index += 1) flags = stack.pop() ?? 0;
      return true;
    }),
  ];

  return {
    encode(event) {
      if (flags === 0 || event.type !== "keydown" || event.isComposing || event.key !== "Enter") return null;
      if (!event.shiftKey && !event.ctrlKey) return null;
      if (event.metaKey) return null;
      const modifiers = 1 + (event.shiftKey ? 1 : 0) + (event.altKey ? 2 : 0) + (event.ctrlKey ? 4 : 0);
      return `\u001b[13;${modifiers}u`;
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
