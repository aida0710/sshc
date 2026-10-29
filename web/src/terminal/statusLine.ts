// The VT320 status line: one line under the screen that a program writes to.
// DECSSDT (CSI Ps $ ~, https://vt100.net/docs/vt510-rm/DECSSDT.html) shows it
// with Ps 2, and DECSASD (CSI Ps $ }, .../DECSASD.html) sends the output that
// follows to it with Ps 1 and back to the screen with Ps 0. xterm.js knows
// neither and would print on the screen what was meant for the status line, so
// the output is split before xterm.js parses it.
//
// The indicator status line (DECSSDT 1), which the terminal itself writes, is
// not drawn: sshc shows the state of a session around the terminal instead.

type Writer = { write(text: string, callback?: () => void): void };

type StatusLineRouterOptions = {
  screen: Writer;
  // statusLine is the status line's own one-row terminal.
  statusLine(): Writer;
  // shownChanged follows DECSSDT; a status line shown anew starts empty.
  shownChanged(shown: boolean): void;
};

const ESC = "\u001b";
// CAN is what sshc writes where output was lost (output.ts): a status line
// write may have lost its end, so the output goes back to the screen.
const CAN = "\u0018";
const sequenceStarts = /[\u0018\u001b]/g;
const stringEnds = /[\u0007\u0018\u001a\u001b]/g;

const hostWritableStatusLine = 2;
const statusLineDisplay = 1;

// createStatusLineRouter hands the output to the screen or the status line. It
// takes the place of the terminal for createTerminalOutput: a write callback
// runs once the screen has parsed what came before it.
export function createStatusLineRouter(options: StatusLineRouterOptions): Writer {
  let state: "ground" | "escape" | "csi" | "string" | "stringEscape" = "ground";
  // held is a sequence cut off at the end of a write, kept until it is known
  // whether it is one of the status line's.
  let held = "";
  let shown = false;
  let toStatusLine = false;

  const show = (value: boolean) => {
    if (!value) toStatusLine = false;
    if (value === shown) return;
    shown = value;
    options.shownChanged(value);
  };

  return {
    write(chunk, callback) {
      const text = held + chunk;
      held = "";
      let sent = 0;
      let sequenceStart = 0;
      let parametersStart = 0;
      let intermediatesStart = 0;
      // send hands on the output up to end. The last part carries the callback
      // when it goes to the screen, so output with no status line in it reaches
      // the screen as the same single write as without the router.
      let callbackPending = callback !== undefined;
      const send = (end: number, last = false) => {
        if (end > sent && toStatusLine) {
          options.statusLine().write(text.slice(sent, end));
        } else if (end > sent) {
          options.screen.write(text.slice(sent, end), last ? callback : undefined);
          if (last) callbackPending = false;
        }
        sent = end;
      };
      // takeSequence leaves the sequence ending at index out of the output.
      const takeSequence = (index: number) => {
        send(sequenceStart);
        sent = index + 1;
      };
      const finishControlSequence = (index: number) => {
        const parameters = text.slice(parametersStart, intermediatesStart);
        const intermediates = text.slice(intermediatesStart, index);
        const final = text[index];
        const value = /^\d*$/.test(parameters) ? Number(parameters || "0") : null;
        if (intermediates === "$" && final === "~" && value !== null) {
          takeSequence(index);
          show(value === hostWritableStatusLine);
        } else if (intermediates === "$" && final === "}" && value !== null) {
          takeSequence(index);
          toStatusLine = shown && value === statusLineDisplay;
        } else if (intermediates === "!" && final === "p" && parameters === "") {
          // DECSTR leaves the status line; the screen takes the reset.
          send(sequenceStart);
          toStatusLine = false;
        }
      };

      let index = 0;
      while (index < text.length) {
        const character = text[index]!;
        const code = text.charCodeAt(index);
        if (character === CAN) {
          // Both parsers drop what they were in the middle of: the status line's
          // takes the CAN, and the screen's takes it again with what follows.
          if (toStatusLine) {
            send(index + 1);
            sent = index;
            toStatusLine = false;
          }
          state = "ground";
          index += 1;
          continue;
        }
        if (state === "ground") {
          sequenceStarts.lastIndex = index;
          const next = sequenceStarts.exec(text);
          if (next === null) break;
          if (next[0] === ESC) {
            sequenceStart = next.index;
            state = "escape";
            index = next.index + 1;
          } else {
            index = next.index;
          }
          continue;
        }
        if (state === "string") {
          stringEnds.lastIndex = index;
          const end = stringEnds.exec(text);
          if (end === null) break;
          state = end[0] === ESC ? "stringEscape" : "ground";
          index = end.index + 1;
          continue;
        }
        if (state === "stringEscape") {
          if (character === "\\") {
            state = "ground";
            index += 1;
          } else {
            sequenceStart = index - 1;
            state = "escape";
          }
          continue;
        }
        if (state === "escape") {
          index += 1;
          if (character === "[") {
            state = "csi";
            parametersStart = index;
            intermediatesStart = index;
          } else if ("]PX^_".includes(character)) {
            state = "string";
          } else if (character === ESC) {
            sequenceStart = index - 1;
          } else {
            state = "ground";
            if (character === "c") {
              // RIS erases the status line and leaves it; the screen takes the reset.
              send(sequenceStart);
              show(false);
            }
          }
          continue;
        }
        // A control sequence: parameters, then intermediates, then the final byte.
        if (code >= 0x30 && code <= 0x3f && intermediatesStart === index) {
          intermediatesStart = index + 1;
        } else if (code >= 0x40 && code <= 0x7e) {
          state = "ground";
          finishControlSequence(index);
        } else if (character === ESC) {
          sequenceStart = index;
          state = "escape";
        } else if (code === 0x1a || code > 0x7e) {
          state = "ground";
        }
        index += 1;
      }
      if (state === "escape" || state === "csi") {
        held = text.slice(sequenceStart);
        state = "ground";
      }
      send(text.length - held.length, true);
      if (callbackPending) options.screen.write("", callback);
    },
  };
}
