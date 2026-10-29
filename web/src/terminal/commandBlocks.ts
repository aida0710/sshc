import type { IBuffer, IDisposable, IMarker, IParser } from "@xterm/xterm";

// A command block is a command the shell ran and the output it printed, as the
// shell integration marks them (OSC 133: A the prompt, B the end of the prompt,
// C the start of the output, D the end with the exit code). The engine adds the
// time it saw C and D (OSC 7742, internal/terminal/command_times.go), so the
// times are right for history replayed long after, too.

export type CommandStatus = "running" | "succeeded" | "failed" | "finished";

export type CommandBlock = {
  // start is the line of the prompt the command was typed at.
  readonly start: IMarker;
  // output is the line the output began on.
  readonly output: IMarker;
  // end is the line the next prompt began on, once the command finished.
  end: IMarker | null;
  readonly command: string;
  readonly directory: string;
  status: CommandStatus;
  exitCode: number | null;
  // startedAt and finishedAt are Unix milliseconds, once the engine has said.
  startedAt: number | null;
  finishedAt: number | null;
};

type BlockTerminal = {
  readonly parser: Pick<IParser, "registerOscHandler">;
  readonly buffer: { readonly active: IBuffer };
  registerMarker(cursorYOffset?: number): IMarker | undefined;
};

export type CommandBlocks = {
  // blocks are the commands whose prompt is still in the scrollback, oldest first.
  blocks(): readonly CommandBlock[];
  // onChange runs when a command starts or finishes, or learns its time.
  onChange(listener: () => void): IDisposable;
  dispose(): void;
};

const commandTimeOSC = 7742;

function statusOf(exitCode: number | null): CommandStatus {
  if (exitCode === null) return "finished";
  return exitCode === 0 ? "succeeded" : "failed";
}

// typedCommand is the text from where the prompt ended to the end of the line
// before the output, with wrapped lines joined.
function typedCommand(buffer: IBuffer, from: { line: number; x: number }, outputLine: number): string {
  let command = "";
  for (let line = from.line; line < outputLine; line += 1) {
    const row = buffer.getLine(line);
    if (row === undefined) break;
    if (line > from.line && !row.isWrapped) command += "\n";
    // A line the next one continues keeps its trailing spaces: they are part of the command.
    const continued = line < outputLine - 1 && buffer.getLine(line + 1)?.isWrapped === true;
    command += row.translateToString(!continued, line === from.line ? from.x : 0);
  }
  return command.trim();
}

function exitCodeOf(detail: string): number | null {
  const field = detail.split(";", 1)[0] ?? "";
  return /^-?\d+$/.test(field) ? Number(field) : null;
}

// trackCommandBlocks follows the marks in the output. It only watches: the
// sequences go on to the other handlers.
export function trackCommandBlocks(view: BlockTerminal, { directory }: { directory: () => string }): CommandBlocks {
  let blocks: CommandBlock[] = [];
  let prompt: IMarker | null = null;
  let typedFrom: { marker: IMarker; x: number } | null = null;
  const listeners = new Set<() => void>();
  const changed = () => {
    for (const listener of listeners) listener();
  };
  const live = () => {
    blocks = blocks.filter((block) => !block.start.isDisposed);
    return blocks;
  };

  const marks = view.parser.registerOscHandler(133, (data) => {
    const separator = data.indexOf(";");
    const kind = separator < 0 ? data : data.slice(0, separator);
    const detail = separator < 0 ? "" : data.slice(separator + 1);
    if (view.buffer.active.type !== "normal") return false;
    if (kind === "A") {
      const marker = view.registerMarker(0);
      if (marker === undefined) return false;
      const previous = live().at(-1);
      // The previous command ends where this prompt begins, if its D came without one.
      if (previous !== undefined && previous.end === null && previous.status !== "running") previous.end = marker;
      prompt?.dispose();
      prompt = marker;
    } else if (kind === "B") {
      const marker = view.registerMarker(0);
      typedFrom?.marker.dispose();
      typedFrom = marker === undefined ? null : { marker, x: view.buffer.active.cursorX };
    } else if (kind === "C") {
      const output = view.registerMarker(0);
      const start = prompt ?? view.registerMarker(-1);
      if (output === undefined || start === undefined) return false;
      const command = typedFrom === null ? "" : typedCommand(view.buffer.active, { line: typedFrom.marker.line, x: typedFrom.x }, output.line);
      typedFrom?.marker.dispose();
      typedFrom = null;
      prompt = null;
      blocks.push({ start, output, end: null, command, directory: directory(), status: "running", exitCode: null, startedAt: null, finishedAt: null });
      changed();
    } else if (kind === "D") {
      const running = live().at(-1);
      if (running === undefined || running.status !== "running") return false;
      running.exitCode = exitCodeOf(detail);
      running.status = statusOf(running.exitCode);
      running.end = view.registerMarker(0) ?? null;
      changed();
    }
    return false;
  });

  const times = view.parser.registerOscHandler(commandTimeOSC, (data) => {
    const [kind, time] = data.split(";");
    const at = Number(time);
    if (!Number.isSafeInteger(at)) return true;
    // Each time belongs to the latest command still waiting for one of its kind.
    const waiting = (candidate: CommandBlock) => kind === "C" ? candidate.startedAt === null : candidate.status !== "running" && candidate.finishedAt === null;
    const block = [...live()].reverse().find(waiting);
    if (block === undefined) return true;
    if (kind === "C") block.startedAt = at;
    else block.finishedAt = at;
    changed();
    return true;
  });

  return {
    blocks: live,
    onChange(listener) {
      listeners.add(listener);
      return { dispose: () => listeners.delete(listener) };
    },
    dispose() {
      marks.dispose();
      times.dispose();
      listeners.clear();
      prompt?.dispose();
      typedFrom?.marker.dispose();
      for (const block of blocks) {
        block.start.dispose();
        block.output.dispose();
        block.end?.dispose();
      }
      blocks = [];
    },
  };
}

// lastLineOf is the last line of the block: the line before the next prompt,
// or, while it runs, the line the cursor is on.
export function lastLineOf(block: CommandBlock, buffer: IBuffer): number {
  if (block.end !== null && !block.end.isDisposed) return Math.max(block.start.line, block.end.line - 1);
  if (block.status === "running") return buffer.baseY + buffer.cursorY;
  return block.output.line;
}

// blockAtTop is the command whose output fills the top of the view while its
// prompt is scrolled above it, if any.
export function blockAtTop(blocks: readonly CommandBlock[], buffer: IBuffer): CommandBlock | null {
  const top = buffer.viewportY;
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    const block = blocks[index]!;
    if (block.start.line >= top) continue;
    return lastLineOf(block, buffer) >= top ? block : null;
  }
  return null;
}

// outputText is what the command printed, as text.
export function outputText(block: CommandBlock, buffer: IBuffer): string {
  const last = lastLineOf(block, buffer);
  const lines: string[] = [];
  for (let line = block.output.line; line <= last; line += 1) {
    const row = buffer.getLine(line);
    if (row === undefined) break;
    const text = row.translateToString(true);
    if (row.isWrapped && lines.length > 0) lines[lines.length - 1] += text;
    else lines.push(text);
  }
  return lines.join("\n").trimEnd();
}
