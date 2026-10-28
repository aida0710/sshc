import { Terminal } from "@xterm/xterm";
import { afterEach, describe, expect, it } from "vitest";
import { attachKittyImageGuard } from "./kittyImageGuard";

const cleanups: (() => void)[] = [];

afterEach(() => {
  for (const cleanup of cleanups.splice(0)) cleanup();
});

// The guard on its own, without the image addon, with limits small enough
// that a few bytes reach them. Only the answers to refused commands show.
function guarded({ storedOnlyByteLimit = 1000 } = {}) {
  const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
  const answers: string[] = [];
  let clock = 0;
  const guard = attachKittyImageGuard({
    parser: terminal.parser,
    images: { reset: () => undefined },
    // Four pixels at most, so the placement budget is 16 pixels a second.
    limits: { pixelLimit: 4, imageByteLimit: 1000, storedOnlyByteLimit },
    answer: (response) => answers.push(response),
    now: () => clock,
  });
  cleanups.push(() => {
    guard.dispose();
    terminal.dispose();
  });
  return {
    answers,
    advance: (milliseconds: number) => { clock += milliseconds; },
    write: (output: string) => new Promise<void>((resolve) => terminal.write(output, resolve)),
  };
}

const command = (control: string, payload?: string) => `\u001b_G${control}${payload === undefined ? "" : `;${payload}`}\u001b\\`;
// 18 bytes of raw pixels; the guard checks only the declared size of raw data.
const pixels = "A".repeat(24);

describe("placing a stored kitty image again", () => {
  it("refuses placements past the decoding budget and accepts them again as it refills", async () => {
    const subject = guarded();
    await subject.write(command("a=t,f=32,s=2,v=2,i=1", pixels));

    await subject.write(command("a=p,i=1").repeat(4));
    expect(subject.answers).toEqual([]);
    await subject.write(command("a=p,i=1,p=3"));
    expect(subject.answers).toEqual([
      "\u001b_Gi=1,p=3;EBUSY:too many placements to decode at once; place it again later\u001b\\",
    ]);

    // A quarter of a second gives back one placement of four pixels.
    subject.advance(250);
    await subject.write(command("a=p,i=1"));
    expect(subject.answers).toHaveLength(1);
    await subject.write(command("a=p,i=1,q=2"));
    expect(subject.answers).toHaveLength(1);
  });

  it("charges a placement by the pixels of its image, and an image it does not know as the largest", async () => {
    const small = guarded();
    await small.write(command("a=t,f=32,s=1,v=1,i=2", pixels));
    await small.write(command("a=p,i=2").repeat(16));
    expect(small.answers).toEqual([]);
    await small.write(command("a=p,i=2"));
    expect(small.answers).toHaveLength(1);

    const unknown = guarded();
    await unknown.write(command("a=p,i=99").repeat(4));
    expect(unknown.answers).toEqual([]);
    await unknown.write(command("a=p,i=99"));
    expect(unknown.answers).toHaveLength(1);
  });
});

describe("images sent only to be stored", () => {
  const noRoom = (id: number) => `\u001b_Gi=${id};ENOSPC:stored images exceed the terminal's limit; delete some (a=d)\u001b\\`;

  it("refuses the next stored-only image once they fill the limit, until one is deleted", async () => {
    const subject = guarded({ storedOnlyByteLimit: 30 });
    await subject.write(command("a=t,f=32,s=2,v=2,i=1", pixels));
    await subject.write(command("a=t,f=32,s=2,v=2,i=2", pixels));
    await subject.write(command("a=t,f=32,s=2,v=2,i=3", pixels));
    expect(subject.answers).toEqual([noRoom(3)]);

    // Sending an image under the same id again replaces it.
    await subject.write(command("a=t,f=32,s=2,v=2,i=2", pixels));
    expect(subject.answers).toEqual([noRoom(3)]);
    // An image that is also displayed is bounded by the addon's storage instead.
    await subject.write(command("a=T,f=32,s=2,v=2,i=4", pixels));
    expect(subject.answers).toEqual([noRoom(3)]);

    await subject.write(command("a=d,d=i,i=1"));
    await subject.write(command("a=t,f=32,s=2,v=2,i=3", pixels));
    expect(subject.answers).toEqual([noRoom(3)]);
  });

  it("does not count a query as a stored image", async () => {
    const subject = guarded({ storedOnlyByteLimit: 30 });
    await subject.write(command("a=q,f=32,s=2,v=2,i=1", pixels));
    await subject.write(command("a=q,f=32,s=2,v=2,i=2", pixels));
    await subject.write(command("a=t,f=32,s=2,v=2,i=3", pixels));

    expect(subject.answers).toEqual([]);
  });

  it("forgets the stored images when they are all deleted or the terminal is reset", async () => {
    for (const reset of [command("a=d,d=A"), "\u001bc", "\u001b[!p"]) {
      const subject = guarded({ storedOnlyByteLimit: 30 });
      await subject.write(command("a=t,f=32,s=2,v=2,i=1", pixels));
      await subject.write(command("a=t,f=32,s=2,v=2,i=2", pixels));

      await subject.write(reset);
      await subject.write(command("a=t,f=32,s=2,v=2,i=3", pixels));

      expect(subject.answers).toEqual([]);
    }
  });
});
