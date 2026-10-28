import type { IDisposable, IParser } from "@xterm/xterm";
import { createInflatedSizeMeter } from "./inflatedSizeMeter";
import { createKittyStoredImages } from "./kittyStoredImages";

export type KittyImageLimits = {
  // Most pixels one image may hold once decoded.
  pixelLimit: number;
  // Most bytes one image may be sent as, before decompression. The image
  // addon is given the same value as its kittySizeLimit.
  imageByteLimit: number;
  // Most bytes of the images sent only to be stored (a=t) that one terminal
  // keeps. The addon bounds them by count alone.
  storedOnlyByteLimit: number;
};

// The image addon ignores a command whose control data is longer than this.
export const maxKittyControlLength = 512;
const pngFormat = 100;
const rgbaFormat = 32;
const rgbaBytesPerPixel = 4;
const rgbBytesPerPixel = 3;
// The PNG signature and the IHDR chunk's length and type take 16 bytes; the
// width and the height follow as 4 bytes each.
const pngHeaderLength = 24;
const pngSignature = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
const ihdrType = [0x49, 0x48, 0x44, 0x52];
// Keys a later chunk of a transmission may carry. Any other key describes a
// new image, which the image addon would decode on its own terms.
const continuationKeys = new Set(["m", "q", "i"]);

type KittyCommand = {
  keys: string[];
  quiet: number;
  action: string;
  format: number;
  width: number;
  height: number;
  compressed: boolean;
  more: boolean;
  medium: string;
  id: number | undefined;
  imageNumber: number | undefined;
  placement: number | undefined;
  deleteSelector: string;
};

// Values and defaults follow @xterm/addon-image's parser, so both sides read
// the same command the same way.
function parseKittyCommand(control: string): KittyCommand {
  const values = new Map<string, string>();
  for (const part of control.split(",")) {
    const separator = part.indexOf("=");
    if (separator >= 0) values.set(part.slice(0, separator), part.slice(separator + 1));
  }
  const numberAt = (key: string) => {
    const value = values.get(key);
    return value === undefined ? undefined : Number.parseInt(value, 10);
  };
  return {
    keys: [...values.keys()],
    quiet: numberAt("q") ?? 0,
    action: values.get("a") ?? "t",
    format: numberAt("f") ?? rgbaFormat,
    width: numberAt("s") ?? 0,
    height: numberAt("v") ?? 0,
    compressed: values.get("o") === "z",
    more: numberAt("m") === 1,
    medium: values.get("t") ?? "d",
    id: numberAt("i"),
    imageNumber: numberAt("I"),
    placement: numberAt("p"),
    deleteSelector: values.get("d") ?? "a",
  };
}

// pngPixels reads the pixel count from a PNG's header, or null if it is not one.
function pngPixels(header: Uint8Array): number | null {
  if (header.byteLength < pngHeaderLength) return null;
  if (pngSignature.some((byte, index) => header[index] !== byte)) return null;
  if (ihdrType.some((byte, index) => header[12 + index] !== byte)) return null;
  const view = new DataView(header.buffer, header.byteOffset, header.byteLength);
  const width = view.getUint32(16);
  const height = view.getUint32(20);
  return width > 0 && height > 0 ? width * height : null;
}

// Decodes base64 that arrives split across chunks at any character.
function createBase64Reader() {
  let carry = "";
  const decode = (text: string): Uint8Array<ArrayBuffer> | null => {
    try {
      return Uint8Array.from(atob(text), (character) => character.charCodeAt(0));
    } catch {
      return null;
    }
  };
  return {
    take(text: string): Uint8Array<ArrayBuffer> | null {
      const joined = carry + text;
      const complete = joined.length - (joined.length % 4);
      carry = joined.slice(complete);
      return decode(joined.slice(0, complete));
    },
    end(): Uint8Array<ArrayBuffer> | null {
      const rest = carry;
      carry = "";
      return decode(rest);
    },
  };
}

// A check sees each chunk's payload in order and answers whether the image
// is still safe to hand to the image addon. `last` marks the final chunk.
type TransmissionCheck = {
  take(payload: string, last: boolean): boolean | Promise<boolean>;
  cancel(): void;
};

// A PNG check passes the image's pixel count to onPixels once it has read it.
function uncompressedPngCheck(pixelLimit: number, onPixels: (pixels: number) => void): TransmissionCheck {
  const base64 = createBase64Reader();
  const header = new Uint8Array(pngHeaderLength);
  let filled = 0;
  return {
    take(payload, last) {
      if (filled < pngHeaderLength) {
        const bytes = base64.take(payload);
        if (bytes === null) return false;
        const copied = bytes.subarray(0, pngHeaderLength - filled);
        header.set(copied, filled);
        filled += copied.byteLength;
        if (filled === pngHeaderLength) {
          const pixels = pngPixels(header);
          if (pixels === null || pixels > pixelLimit) return false;
          onPixels(pixels);
        }
      }
      return !last || filled === pngHeaderLength;
    },
    cancel: () => undefined,
  };
}

function compressedCheck({ inflatedLimit, png }: {
  inflatedLimit: number;
  png?: { pixelLimit: number; onPixels: (pixels: number) => void };
}): TransmissionCheck {
  const base64 = createBase64Reader();
  const meter = createInflatedSizeMeter({
    limit: inflatedLimit,
    headLength: png === undefined ? 0 : pngHeaderLength,
  });
  return {
    async take(payload, last) {
      const bytes = base64.take(payload);
      if (bytes === null || !(await meter.push(bytes))) return false;
      if (!last) return true;
      const rest = base64.end();
      if (rest === null || !(await meter.push(rest)) || !(await meter.finish())) return false;
      if (png === undefined) return true;
      const pixels = pngPixels(meter.head());
      if (pixels === null || pixels > png.pixelLimit) return false;
      png.onPixels(pixels);
      return true;
    },
    cancel: () => meter.cancel(),
  };
}

// Kitty answers an image it cannot take with an error the program can read,
// when the program named the image (i) and did not ask for quiet (q=2).
const refusedAsMalformed = "EINVAL:width and height required for raw pixel data";
const refusedAsTooLarge = "EFBIG:image exceeds the terminal's limits";
const refusedAsInterleaved = "EINVAL:another image's chunks arrived before this image finished";
const refusedAsNoRoom = "ENOSPC:stored images exceed the terminal's limit; delete some (a=d)";
const refusedPlacement = "EBUSY:too many placements to decode at once; place it again later";

type Transmission = {
  refused: boolean;
  // Why the image was refused, sent back once its last chunk has arrived.
  refusal?: string;
  // Where the answer goes, from the command that started the image.
  answerTo?: { id: number; quiet: number };
  answered: boolean;
  // Whether the image addon already holds earlier chunks of this image.
  handedOver: boolean;
  encodedLength: number;
  check?: TransmissionCheck;
  // Whether the addon stores the image (data sent with a=t or a=T), its
  // pixels once known, and whether it is sent only to be stored (a=t).
  stores: boolean;
  pixels: number;
  storedOnly: boolean;
};

// newTransmission is the state of an image that has not been checked yet.
function newTransmission(command: KittyCommand): Transmission {
  return {
    refused: false,
    answered: false,
    handedOver: false,
    encodedLength: 0,
    stores: (command.action === "t" || command.action === "T") && command.medium === "d",
    pixels: 0,
    storedOnly: command.action === "t" && command.medium === "d",
    ...(command.id === undefined ? {} : { answerTo: { id: command.id, quiet: command.quiet } }),
  };
}

function startTransmission(command: KittyCommand, limits: KittyImageLimits): Transmission {
  const transmission = newTransmission(command);
  // Only transmitted images get stored and decoded. Queries and placements
  // never decode a payload, and other media are refused by the addon.
  if (!transmission.stores) return transmission;
  if (command.format === pngFormat) {
    const onPixels = (pixels: number) => { transmission.pixels = pixels; };
    transmission.check = command.compressed
      ? compressedCheck({ inflatedLimit: limits.imageByteLimit, png: { pixelLimit: limits.pixelLimit, onPixels } })
      : uncompressedPngCheck(limits.pixelLimit, onPixels);
    return transmission;
  }
  // Raw pixels must declare their size, and zlib must inflate to exactly
  // that many bytes, so the declaration bounds the work before any decoding.
  const pixels = command.width * command.height;
  if (!(command.width > 0 && command.height > 0)) {
    transmission.refused = true;
    transmission.refusal = refusedAsMalformed;
    return transmission;
  }
  if (pixels > limits.pixelLimit) {
    transmission.refused = true;
    transmission.refusal = refusedAsTooLarge;
    return transmission;
  }
  transmission.pixels = pixels;
  if (command.compressed) {
    const bytesPerPixel = command.format === rgbaFormat ? rgbaBytesPerPixel : rgbBytesPerPixel;
    transmission.check = compressedCheck({ inflatedLimit: pixels * bytesPerPixel });
  }
  return transmission;
}

// attachKittyImageGuard refuses kitty graphics images (APC G) that would
// decode or inflate past the limits. @xterm/addon-image only checks the pixel
// count after the browser has inflated the zlib data and decoded the PNG, so
// about a MiB of compressed data could otherwise make it allocate gigabytes.
//
// It must be registered after the addon: xterm asks the most recent handler
// first, and a handler that returns true keeps the command from the addon.
// xterm still streams every chunk through the addon's parser, which files a
// chunk without i under the last unfinished image. The guard therefore allows
// one unfinished image at a time, as the protocol asks of programs, so that a
// chunk it keeps from the addon can never land in another image.
export function attachKittyImageGuard({ parser, images, limits, answer, now = () => performance.now() }: {
  parser: IParser;
  images: { reset(): unknown };
  limits: KittyImageLimits;
  // answer sends a kitty graphics response to the program, as typed input.
  answer: (response: string) => void;
  // now is the clock, in milliseconds, that refills the placement budget.
  now?: () => number;
}): IDisposable {
  const stored = createKittyStoredImages({ pixelLimit: limits.pixelLimit, storedOnlyByteLimit: limits.storedOnlyByteLimit, now });
  // The addon gives up on an image whose base64 is longer than this.
  const maxEncodedLength = Math.ceil(limits.imageByteLimit * 4 / 3);
  // The image whose chunks are still arriving, under the key the addon files
  // it by: i, else 0.
  let unfinished: { key: number; transmission: Transmission } | undefined;

  const forget = () => {
    unfinished?.transmission.check?.cancel();
    unfinished = undefined;
  };

  // Sends the refusal once, after the image's last chunk: a program reads the
  // answer to a chunked image only when it has sent all of it.
  const answerRefusal = (transmission: Transmission) => {
    const to = transmission.answerTo;
    if (!transmission.refused || transmission.answered || to === undefined || to.quiet >= 2) return;
    transmission.answered = true;
    answer(`\u001b_Gi=${to.id};${transmission.refusal ?? refusedAsTooLarge}\u001b\\`);
  };

  const refuse = (transmission: Transmission, refusal: string) => {
    if (transmission.refused) return;
    transmission.refused = true;
    transmission.refusal = refusal;
    transmission.check?.cancel();
    // The addon cannot drop one half-received image, so drop all of its
    // images and unfinished data.
    if (transmission.handedOver) {
      images.reset();
      stored.cleared();
    }
  };

  const inspect = (transmission: Transmission, payload: string, last: boolean): boolean | Promise<boolean> => {
    const decide = (safe: boolean) => {
      if (!safe) {
        refuse(transmission, refusedAsTooLarge);
        return true;
      }
      transmission.handedOver = true;
      return false;
    };
    transmission.encodedLength += payload.length;
    if (transmission.encodedLength > maxEncodedLength) {
      // The addon drops this image by itself at the same point, so the
      // other images can stay.
      transmission.refused = true;
      transmission.refusal = refusedAsTooLarge;
      transmission.check?.cancel();
      return true;
    }
    if (transmission.check === undefined) return decide(true);
    const safe = transmission.check.take(payload, last);
    return safe instanceof Promise ? safe.then(decide) : decide(safe);
  };

  const handle = (data: string): boolean | Promise<boolean> => {
    const separator = data.indexOf(";");
    const control = separator < 0 ? data : data.slice(0, separator);
    if (control.length > maxKittyControlLength) return false;
    const command = parseKittyCommand(control);
    if (command.id !== undefined && command.imageNumber !== undefined) return false;
    if (command.action === "d") {
      const all = command.deleteSelector === "a" || command.deleteSelector === "A";
      const byId = command.deleteSelector === "i" || command.deleteSelector === "I";
      if (all) stored.cleared();
      if (byId && command.id !== undefined) stored.deleted(command.id);
      if (all || (byId && command.id === unfinished?.key)) forget();
      return false;
    }
    if (command.action === "p") {
      if (command.id === undefined || stored.mayPlace(command.id)) return false;
      if (command.quiet < 2) {
        const placement = command.placement === undefined ? "" : `,p=${command.placement}`;
        answer(`\u001b_Gi=${command.id}${placement};${refusedPlacement}\u001b\\`);
      }
      return true;
    }
    if (separator < 0) return false;

    const payload = data.slice(separator + 1);
    const key = command.id ?? unfinished?.key ?? 0;
    // The addon keeps no state for an empty chunk that promises more.
    if (command.more && payload === "") return unfinished?.transmission.refused === true;

    let transmission: Transmission;
    if (unfinished === undefined) {
      transmission = startTransmission(command, limits);
      if (transmission.storedOnly && !transmission.refused && !stored.hasRoomFor(command.id)) {
        transmission.refused = true;
        transmission.refusal = refusedAsNoRoom;
      }
    } else {
      transmission = unfinished.transmission;
      // A chunk for another image, or one that describes an image again,
      // would be read by the addon differently from how it was checked.
      if (key !== unfinished.key || command.keys.some((name) => !continuationKeys.has(name))) {
        refuse(transmission, refusedAsInterleaved);
        answerRefusal(transmission);
        forget();
        // The chunk that broke in is dropped as well; nothing of it is checked.
        if (!command.more) answerRefusal({ ...newTransmission(command), refused: true, refusal: refusedAsInterleaved });
        return true;
      }
    }
    if (command.more) unfinished = { key, transmission };
    else unfinished = undefined;
    const last = !command.more;
    const answerIfLast = (kept: boolean) => {
      if (!last) return kept;
      answerRefusal(transmission);
      if (transmission.stores && !transmission.refused) {
        // Kitty has no image 0: an image without i is numbered by the addon.
        stored.stored(key === 0 ? undefined : key, {
          pixels: transmission.pixels || limits.pixelLimit,
          storedOnlyBytes: transmission.storedOnly ? Math.floor(transmission.encodedLength * 3 / 4) : 0,
        });
      }
      return kept;
    };
    if (transmission.refused) return answerIfLast(true);
    const kept = inspect(transmission, payload, last);
    return kept instanceof Promise ? kept.then(answerIfLast) : answerIfLast(kept);
  };

  const registrations = [
    parser.registerApcHandler({ final: "G" }, handle),
    // RIS and DECSTR make the addon drop its images and unfinished ones. Returning
    // false lets xterm and the addon handle them as before.
    parser.registerEscHandler({ final: "c" }, () => {
      forget();
      stored.cleared();
      return false;
    }),
    parser.registerCsiHandler({ intermediates: "!", final: "p" }, () => {
      forget();
      stored.cleared();
      return false;
    }),
  ];
  return {
    dispose() {
      for (const registration of registrations) registration.dispose();
      forget();
    },
  };
}
