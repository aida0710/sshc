import { useCallback, useRef, useState } from "react";

// CommandOutputPart is one read of a command's output: the text from the
// cursor that was asked for, and where the next read starts.
export type CommandOutputPart = { output: string; cursor: { next: number } };

export type CommandOutputState = {
  // output is everything read so far, keeping the newest maxLength characters.
  output: string;
  // nextCursor is where the next read starts.
  nextCursor: () => number;
  // receive appends a read and moves the cursor past it.
  receive: (part: CommandOutputPart) => void;
  // restart forgets what was read, for a command that starts over.
  restart: () => void;
};

// useCommandOutput collects the output of a command that is read in parts by
// a cursor while it runs, so a screen polling its log shows it growing.
export function useCommandOutput(maxLength: number): CommandOutputState {
  const [output, setOutput] = useState("");
  const cursor = useRef(0);

  const nextCursor = useCallback(() => cursor.current, []);
  const receive = useCallback((part: CommandOutputPart) => {
    cursor.current = part.cursor.next;
    if (part.output !== "") setOutput((shown) => keepNewest(shown + part.output, maxLength));
  }, [maxLength]);
  const restart = useCallback(() => {
    cursor.current = 0;
    setOutput("");
  }, []);

  return { output, nextCursor, receive, restart };
}

function keepNewest(output: string, maxLength: number): string {
  return output.length > maxLength ? output.slice(output.length - maxLength) : output;
}
