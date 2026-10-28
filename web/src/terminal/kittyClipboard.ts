type Disposable = { dispose(): void };
type ClipboardParser = {
  registerOscHandler(identifier: number, handler: (data: string) => boolean): Disposable;
  registerCsiHandler(id: { prefix?: string; intermediates?: string; final: string }, handler: (params: (number | number[])[]) => boolean): Disposable;
  registerEscHandler(id: { final: string }, handler: () => boolean): Disposable;
};

// The Kitty clipboard protocol (OSC 5522, https://sw.kovidgoyal.net/kitty/clipboard/)
// lets a program put more than text on the clipboard (an image, HTML), and with
// paste events (mode 5522, https://rockorager.dev/misc/bracketed-paste-mime/)
// choose what to take from a paste. sshc answers:
// - writes, when OSC 52 is allowed (the same setting: both let a program
//   change the clipboard). The browser takes text/plain, text/html and image/png.
// - reads, never on their own: a program may read only what the person pasted,
//   with the one-time password the paste event gave it.

// PastedItem is one form of what the person pasted.
export type PastedItem = { mime: string; data: string | Blob };

export type KittyClipboardOptions = {
  parser: ClipboardParser;
  // writesAllowed is whether a program may change the clipboard (OSC 52's setting).
  writesAllowed: () => boolean;
  // replaying is true while history is parsed: nothing is written or answered then.
  replaying: () => boolean;
  writeClipboard: (items: Record<string, Blob>) => Promise<void>;
  // reply sends an answer to the program, as typed input.
  reply: (sequence: string) => void;
  // copied is told once a program's write reached the clipboard.
  copied?: () => void;
  // randomBytes makes the paste event's password.
  randomBytes?: (size: number) => Uint8Array;
};

export type KittyClipboard = Disposable & {
  // takePaste hands a paste to the program when it turned paste events on, and
  // says whether it did. items is asked for only then.
  takePaste(items: () => PastedItem[]): boolean;
};

// The forms this browser can put on the clipboard.
const writableTypes = new Set(["text/plain", "text/html", "image/png"]);
// Data goes in chunks of this many bytes before base64, as the protocol says.
const chunkBytes = 4096;
// The protocol asks terminals to take at least 64 MB in one write.
const maximumWriteBytes = 64 * 1024 * 1024;
const base64 = /^[A-Za-z0-9+/]*={0,2}$/;

type Write = { id: string | undefined; parts: Map<string, string[]>; aliases: Map<string, string[]>; bytes: number; failed: boolean };
type Paste = { password: string; items: PastedItem[] };

function metadataOf(text: string): Map<string, string> {
  const entries = new Map<string, string>();
  for (const field of text.split(":")) {
    const separator = field.indexOf("=");
    if (separator > 0) entries.set(field.slice(0, separator), field.slice(separator + 1));
  }
  return entries;
}

function decodeBase64(text: string): Uint8Array<ArrayBuffer> | null {
  if (text.length % 4 !== 0 || !base64.test(text)) return null;
  try {
    return Uint8Array.from(atob(text), (character) => character.charCodeAt(0));
  } catch {
    return null;
  }
}

function decodeText(text: string): string | null {
  const bytes = decodeBase64(text);
  return bytes === null ? null : new TextDecoder().decode(bytes);
}

function encodeBytes(bytes: Uint8Array): string {
  let text = "";
  for (const value of bytes) text += String.fromCharCode(value);
  return btoa(text);
}

function encodeText(text: string): string {
  return encodeBytes(new TextEncoder().encode(text));
}

function packet(fields: string, payload?: string): string {
  return `\u001b]5522;${fields}${payload === undefined ? "" : `;${payload}`}\u001b\\`;
}

export function attachKittyClipboard(options: KittyClipboardOptions): KittyClipboard {
  const randomBytes = options.randomBytes ?? ((size) => crypto.getRandomValues(new Uint8Array(size)));
  let write: Write | null = null;
  let pasteEvents = false;
  let paste: Paste | null = null;

  const answer = (sequence: string) => {
    if (!options.replaying()) options.reply(sequence);
  };
  const withId = (fields: string, id: string | undefined) => id === undefined ? fields : `${fields}:id=${id}`;

  const finishWrite = (current: Write) => {
    const items: Record<string, Blob> = {};
    for (const [mime, parts] of current.parts) {
      const bytes = decodeBase64(parts.join(""));
      if (bytes === null) {
        answer(packet(withId("type=write:status=EINVAL", current.id)));
        return;
      }
      if (writableTypes.has(mime)) items[mime] = new Blob([bytes], { type: mime });
    }
    for (const [target, aliases] of current.aliases) {
      const data = items[target];
      for (const alias of aliases) if (data !== undefined && writableTypes.has(alias) && items[alias] === undefined) items[alias] = data;
    }
    // None of the forms is one the browser can hold.
    if (Object.keys(items).length === 0) {
      answer(packet(withId("type=write:status=EIO", current.id)));
      return;
    }
    void options.writeClipboard(items).then(
      () => {
        answer(packet(withId("type=write:status=DONE", current.id)));
        options.copied?.();
      },
      (error: unknown) => answer(packet(withId(`type=write:status=${error instanceof DOMException && error.name === "NotAllowedError" ? "EPERM" : "EIO"}`, current.id))),
    );
  };

  const acceptWrite = (metadata: Map<string, string>, payload: string) => {
    const type = metadata.get("type");
    const id = metadata.get("id");
    if (type === "write") {
      write = { id, parts: new Map(), aliases: new Map(), bytes: 0, failed: false };
      // A browser has no primary selection.
      if (metadata.get("loc") === "primary") {
        write.failed = true;
        answer(packet(withId("type=write:status=ENOSYS", id)));
      } else if (!options.writesAllowed()) {
        write.failed = true;
        answer(packet(withId("type=write:status=EPERM", id)));
      }
      return;
    }
    const current = write;
    if (current === null || current.failed) return;
    const encodedMime = metadata.get("mime");
    if (type === "wdata" && encodedMime === undefined) {
      write = null;
      finishWrite(current);
      return;
    }
    const mime = encodedMime === undefined ? null : decodeText(encodedMime);
    if (mime === null || mime === "") {
      current.failed = true;
      answer(packet(withId("type=write:status=EINVAL", current.id)));
      return;
    }
    if (type === "walias") {
      const aliases = decodeText(payload);
      if (aliases !== null) current.aliases.set(mime, aliases.split(/\s+/).filter(Boolean));
      return;
    }
    current.bytes += payload.length * 3 / 4;
    if (current.bytes > maximumWriteBytes) {
      current.failed = true;
      answer(packet(withId("type=write:status=EFBIG", current.id)));
      return;
    }
    current.parts.set(mime, [...(current.parts.get(mime) ?? []), payload]);
  };

  const sendPasted = async (items: PastedItem[], wanted: string[], id: string | undefined) => {
    answer(packet(withId("type=read:status=OK", id)));
    for (const item of items) {
      if (!wanted.includes(item.mime)) continue;
      const bytes = typeof item.data === "string" ? new TextEncoder().encode(item.data) : new Uint8Array(await item.data.arrayBuffer());
      for (let offset = 0; offset < bytes.length; offset += chunkBytes) {
        answer(packet(withId(`type=read:status=DATA:mime=${encodeText(item.mime)}`, id), encodeBytes(bytes.subarray(offset, offset + chunkBytes))));
      }
    }
    answer(packet(withId("type=read:status=DONE", id)));
  };

  const acceptRead = (metadata: Map<string, string>, payload: string) => {
    const id = metadata.get("id");
    const current = paste;
    const password = metadata.get("pw");
    // A password counts only with a name, as the protocol says; it is good for one read.
    if (current === null || password === undefined || password !== current.password || metadata.get("name") === undefined || metadata.get("loc") === "primary") {
      answer(packet(withId("type=read:status=EPERM", id)));
      return;
    }
    paste = null;
    const wanted = (decodeText(payload) ?? "").split(/\s+/).filter(Boolean);
    void sendPasted(current.items, wanted, id);
  };

  const registrations = [
    options.parser.registerOscHandler(5522, (data) => {
      const separator = data.indexOf(";");
      const metadata = metadataOf(separator < 0 ? data : data.slice(0, separator));
      const payload = separator < 0 ? "" : data.slice(separator + 1);
      const type = metadata.get("type");
      if (type === "read") acceptRead(metadata, payload);
      else if (type === "write" || type === "wdata" || type === "walias") {
        if (!options.replaying()) acceptWrite(metadata, payload);
      }
      return true;
    }),
    // Paste events follow DECSET and DECRST 5522; xterm.js keeps the other modes of the same sequence.
    options.parser.registerCsiHandler({ prefix: "?", final: "h" }, (params) => {
      if (params.includes(5522)) pasteEvents = true;
      return false;
    }),
    options.parser.registerCsiHandler({ prefix: "?", final: "l" }, (params) => {
      if (params.includes(5522)) {
        pasteEvents = false;
        paste = null;
      }
      return false;
    }),
    // xterm.js would say it does not know mode 5522.
    options.parser.registerCsiHandler({ prefix: "?", intermediates: "$", final: "p" }, (params) => {
      if (params[0] !== 5522) return false;
      answer(`\u001b[?5522;${pasteEvents ? 1 : 2}$y`);
      return true;
    }),
    options.parser.registerEscHandler({ final: "c" }, () => {
      pasteEvents = false;
      paste = null;
      return false;
    }),
  ];

  return {
    takePaste(gather) {
      if (!pasteEvents || options.replaying()) return false;
      const items = gather();
      if (items.length === 0) return false;
      const password = encodeBytes(randomBytes(18));
      paste = { password, items };
      const listed = encodeText(items.map((item) => item.mime).join(" "));
      options.reply(packet(`type=read:status=OK:pw=${password}`));
      options.reply(packet(`type=read:status=DATA:mime=${encodeText(".")}:pw=${password}`, listed));
      options.reply(packet(`type=read:status=DONE:pw=${password}`));
      return true;
    },
    dispose() {
      for (const registration of registrations) registration.dispose();
      write = null;
      paste = null;
    },
  };
}
