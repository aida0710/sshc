type Disposable = { dispose(): void };
type Parameters = (number | number[])[];

type XTVersionParser = {
  registerCsiHandler(
    identifier: { prefix: string; final: string },
    handler: (parameters: Parameters) => boolean,
  ): Disposable;
};

// Claude Code draws pictures only in a terminal whose XTVERSION reply names
// iTerm2 3.5 or later (or kitty and Ghostty, whose placeholder images sshc
// cannot draw). sshc draws iTerm2 inline images (OSC 1337 File=), so answering
// with this name lets such programs show them here.
export const iTerm2XTVersionReply = "\x1bP>|iTerm2 3.5.0\x1b\\";

// attachITerm2Identity answers XTVERSION (CSI > 0 q) as iTerm2 while enabled
// says so; otherwise xterm.js answers with its own name. A query replayed
// from scrollback after a reconnect was answered when it first ran, so a
// second answer would only land in the shell as typed text.
export function attachITerm2Identity(options: {
  parser: XTVersionParser;
  enabled: () => boolean;
  isReplaying: () => boolean;
  reply: (text: string) => void;
}): Disposable {
  return options.parser.registerCsiHandler({ prefix: ">", final: "q" }, (parameters) => {
    // Only parameter 0 asks for the name; anything else is not XTVERSION.
    if (!options.enabled() || (parameters[0] ?? 0) !== 0) return false;
    if (!options.isReplaying()) options.reply(iTerm2XTVersionReply);
    return true;
  });
}
