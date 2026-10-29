import type { Terminal } from "@xterm/xterm";
import { setGraphemeClusters } from "./unicodeWidths";

type Disposable = { dispose(): void };

// Mode 2027 (grapheme cluster processing, https://github.com/contour-terminal/terminal-unicode-core)
// lets a program that counts a family emoji as two cells say so. Many still
// count it as wcwidth does, six cells for three people, so the terminal joins
// clusters only while a program has the mode on. A soft reset (DECSTR) and a
// full reset (RIS) turn it off, as the specification says; the engine's reset
// after a program ends (internal/terminal/leftover_modes.go) ends with DECSTR.
const graphemeClusterMode = 2027;

type GraphemeClusterModeOptions = {
  // replaying is whether the output is history played back on attach, whose
  // questions the program had answered long ago.
  replaying(): boolean;
  reply(sequence: string): void;
};

export function attachGraphemeClusterMode(view: Terminal, options: GraphemeClusterModeOptions): Disposable {
  let on = false;
  const set = (value: boolean) => {
    on = value;
    setGraphemeClusters(view, value);
  };
  // xterm.js keeps the other modes of the same sequence, so these return false.
  const registrations = [
    view.parser.registerCsiHandler({ prefix: "?", final: "h" }, (params) => {
      if (params.includes(graphemeClusterMode)) set(true);
      return false;
    }),
    view.parser.registerCsiHandler({ prefix: "?", final: "l" }, (params) => {
      if (params.includes(graphemeClusterMode)) set(false);
      return false;
    }),
    // DECRQM: xterm.js would say it does not know the mode.
    view.parser.registerCsiHandler({ prefix: "?", intermediates: "$", final: "p" }, (params) => {
      if (params[0] !== graphemeClusterMode) return false;
      if (!options.replaying()) options.reply(`\u001b[?${graphemeClusterMode};${on ? 1 : 2}$y`);
      return true;
    }),
    view.parser.registerCsiHandler({ intermediates: "!", final: "p" }, () => {
      set(false);
      return false;
    }),
    view.parser.registerEscHandler({ final: "c" }, () => {
      set(false);
      return false;
    }),
  ];
  return {
    dispose() {
      for (const registration of registrations) registration.dispose();
    },
  };
}
