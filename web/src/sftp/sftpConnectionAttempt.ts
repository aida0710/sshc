import { newIdentifier } from "../ui/randomIdentifier";
import { sftpApi, type SFTPConnectionAttempt } from "./api";

// connectionAttemptPollMilliseconds is how often the stage and log are read
// again while a listing waits for its SSH connection. It matches how often the
// terminal reads a connecting session again (terminal/sessions.ts).
const connectionAttemptPollMilliseconds = 500;

export type ConnectionAttemptWatch = {
  // attempt is the name the listing request gives its connection.
  attempt: string;
  // stop ends the polling. With readFinal it reads the attempt once more, so a
  // failed connection still shows its last lines and the reason.
  stop(options: { readFinal: boolean }): Promise<void>;
};

// watchConnectionAttempt reads the connection the listing is opening until it
// is stopped, and hands every reading to onChange. A failed read is dropped:
// the listing's own answer decides what the pane shows.
export function watchConnectionAttempt(alias: string, onChange: (attempt: SFTPConnectionAttempt) => void): ConnectionAttemptWatch {
  const attempt = newIdentifier();
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  async function read() {
    try {
      return await sftpApi.connectionAttempt(alias, attempt);
    } catch {
      return null;
    }
  }
  async function poll() {
    const reading = await read();
    if (stopped) return;
    if (reading !== null) onChange(reading);
    timer = setTimeout(() => void poll(), connectionAttemptPollMilliseconds);
  }
  timer = setTimeout(() => void poll(), connectionAttemptPollMilliseconds);
  return {
    attempt,
    async stop({ readFinal }) {
      stopped = true;
      clearTimeout(timer);
      if (!readFinal) return;
      const reading = await read();
      if (reading !== null) onChange(reading);
    },
  };
}
