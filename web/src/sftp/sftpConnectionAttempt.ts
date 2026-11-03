import { newIdentifier } from "../ui/randomIdentifier";
import { sftpApi, type SFTPConnectionAttempt } from "./api";

// connectionAttemptPollMilliseconds is how often the stage and log are read
// again while a listing waits for its SSH connection. It matches how often the
// terminal reads a connecting session again (terminal/sessions.ts).
const connectionAttemptPollMilliseconds = 500;

export type ConnectionAttemptWatch = {
  // attempt is the name the listing request gives its connection.
  attempt: string;
  // answerHostKey answers the question the connection asks about an unknown
  // host key, and hands the attempt as it is after the answer to onChange.
  // False when the question had closed first (another answer, or the
  // connection went on); the next reading shows where it is.
  answerHostKey(fingerprint: string, accept: boolean): Promise<boolean>;
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
    async answerHostKey(fingerprint, accept) {
      const answered = await sftpApi.answerHostKey(alias, attempt, { fingerprint, accept });
      if (stopped) return answered !== null;
      const reading = answered ?? await read();
      if (reading !== null && !stopped) onChange(reading);
      return answered !== null;
    },
    async stop({ readFinal }) {
      stopped = true;
      clearTimeout(timer);
      if (!readFinal) return;
      const reading = await read();
      if (reading !== null) onChange(reading);
    },
  };
}
