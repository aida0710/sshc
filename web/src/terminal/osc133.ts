type Disposable = { dispose(): void };
type OSCParser = { registerOscHandler(identifier: number, handler: (data: string) => boolean): Disposable };

export type OSC133Completion = {
  durationMilliseconds: number;
  exitCode: number | null;
};

export function attachOSC133Commands(
  parser: OSCParser,
  callbacks: {
    onCommandStarted?: () => void;
    onCommandCompleted?: (completion: OSC133Completion) => void;
    now?: () => number;
  },
): Disposable {
  const now = callbacks.now ?? Date.now;
  let startedAt: number | null = null;
  return parser.registerOscHandler(133, (data) => {
    const [kind, detail = ""] = data.split(";", 2);
    if (kind === "C") {
      startedAt = now();
      callbacks.onCommandStarted?.();
      return true;
    }
    if (kind === "D") {
      if (startedAt !== null) {
        const parsed = detail === "" ? Number.NaN : Number(detail);
        callbacks.onCommandCompleted?.({
          durationMilliseconds: Math.max(0, now() - startedAt),
          exitCode: Number.isInteger(parsed) ? parsed : null,
        });
      }
      startedAt = null;
      return true;
    }
    // A (prompt start) and B (prompt end) are valid shell-integration markers.
    // Consume them so they never appear as text, while C/D define the timed
    // command region used by sshc.
    return kind === "A" || kind === "B";
  });
}
