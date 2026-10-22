import type { IDisposable, IParser } from "@xterm/xterm";
import { createInflatedSizeMeter } from "./inflatedSizeMeter";

export type KittyImageLimits = {
  // Most pixels one image may hold once decoded.
  pixelLimit: number;
  // Most bytes one image may be sent as, before decompression. The image
  // addon is given the same value as its kittySizeLimit.
  imageByteLimit: number;
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
  action: string;
  format: number;
  width: number;
  height: number;
  compressed: boolean;
  more: boolean;
  medium: string;
  id: number | undefined;
  imageNumber: number | undefined;
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
    action: values.get("a") ?? "t",
    format: numberAt("f") ?? rgbaFormat,
    width: numberAt("s") ?? 0,
    height: numberAt("v") ?? 0,
    compressed: values.get("o") === "z",
    more: numberAt("m") === 1,
    medium: values.get("t") ?? "d",
    id: numberAt("i"),
    imageNumber: numberAt("I"),
    deleteSelector: values.get("d") ?? "a",
  };
}

function isPngWithinPixelLimit(header: Uint8Array, pixelLimit: number): boolean {
  if (header.byteLength < pngHeaderLength) return false;
  if (pngSignature.some((byte, index) => header[index] !== byte)) return false;
  if (ihdrType.some((byte, index) => header[12 + index] !== byte)) return false;
  const view = new DataView(header.buffer, header.byteOffset, header.byteLength);
  const width = view.getUint32(16);
  const height = view.getUint32(20);
  return width > 0 && height > 0 && width * height <= pixelLimit;
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

function uncompressedPngCheck(pixelLimit: number): TransmissionCheck {
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
        if (filled === pngHeaderLength && !isPngWithinPixelLimit(header, pixelLimit)) return false;
      }
      return !last || filled === pngHeaderLength;
    },
    cancel: () => undefined,
  };
}

function compressedCheck({ inflatedLimit, pngPixelLimit }: { inflatedLimit: number; pngPixelLimit?: number }): TransmissionCheck {
  const base64 = createBase64Reader();
  const meter = createInflatedSizeMeter({
    limit: inflatedLimit,
    headLength: pngPixelLimit === undefined ? 0 : pngHeaderLength,
  });
  return {
    async take(payload, last) {
      const bytes = base64.take(payload);
      if (bytes === null || !(await meter.push(bytes))) return false;
      if (!last) return true;
      const rest = base64.end();
      if (rest === null || !(await meter.push(rest)) || !(await meter.finish())) return false;
      return pngPixelLimit === undefined || isPngWithinPixelLimit(meter.head(), pngPixelLimit);
    },
    cancel: () => meter.cancel(),
  };
}

type Transmission = {
  refused: boolean;
  // Whether the image addon already holds earlier chunks of this image.
  handedOver: boolean;
  encodedLength: number;
  check?: TransmissionCheck;
};

function startTransmission(command: KittyCommand, limits: KittyImageLimits): Transmission {
  const transmission: Transmission = { refused: false, handedOver: false, encodedLength: 0 };
  // Only transmitted images get stored and decoded. Queries and placements
  // never decode a payload, and other media are refused by the addon.
  if ((command.action !== "t" && command.action !== "T") || command.medium !== "d") return transmission;
  if (command.format === pngFormat) {
    transmission.check = command.compressed
      ? compressedCheck({ inflatedLimit: limits.imageByteLimit, pngPixelLimit: limits.pixelLimit })
      : uncompressedPngCheck(limits.pixelLimit);
    return transmission;
  }
  // Raw pixels must declare their size, and zlib must inflate to exactly
  // that many bytes, so the declaration bounds the work before any decoding.
  const pixels = command.width * command.height;
  if (!(command.width > 0 && command.height > 0 && pixels <= limits.pixelLimit)) {
    transmission.refused = true;
    return transmission;
  }
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
export function attachKittyImageGuard({ parser, images, limits }: {
  parser: IParser;
  images: { reset(): unknown };
  limits: KittyImageLimits;
}): IDisposable {
  // The addon gives up on an image whose base64 is longer than this.
  const maxEncodedLength = Math.ceil(limits.imageByteLimit * 4 / 3);
  // The image whose chunks are still arriving, under the key the addon files
  // it by: i, else 0.
  let unfinished: { key: number; transmission: Transmission } | undefined;

  const forget = () => {
    unfinished?.transmission.check?.cancel();
    unfinished = undefined;
  };

  const refuse = (transmission: Transmission) => {
    if (transmission.refused) return;
    transmission.refused = true;
    transmission.check?.cancel();
    // The addon cannot drop one half-received image, so drop all of its
    // images and unfinished data.
    if (transmission.handedOver) images.reset();
  };

  const inspect = (transmission: Transmission, payload: string, last: boolean): boolean | Promise<boolean> => {
    const decide = (safe: boolean) => {
      if (!safe) {
        refuse(transmission);
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
      const byId = (command.deleteSelector === "i" || command.deleteSelector === "I") && command.id === unfinished?.key;
      if (all || byId) forget();
      return false;
    }
    if (separator < 0) return false;

    const payload = data.slice(separator + 1);
    const key = command.id ?? unfinished?.key ?? 0;
    // The addon keeps no state for an empty chunk that promises more.
    if (command.more && payload === "") return unfinished?.transmission.refused === true;

    let transmission: Transmission;
    if (unfinished === undefined) {
      transmission = startTransmission(command, limits);
    } else {
      transmission = unfinished.transmission;
      // A chunk for another image, or one that describes an image again,
      // would be read by the addon differently from how it was checked.
      if (key !== unfinished.key || command.keys.some((name) => !continuationKeys.has(name))) {
        refuse(transmission);
        forget();
        return true;
      }
    }
    if (command.more) unfinished = { key, transmission };
    else unfinished = undefined;
    if (transmission.refused) return true;
    return inspect(transmission, payload, !command.more);
  };

  const registrations = [
    parser.registerApcHandler({ final: "G" }, handle),
    // RIS and DECSTR make the addon drop its unfinished images. Returning
    // false lets xterm and the addon handle them as before.
    parser.registerEscHandler({ final: "c" }, () => {
      forget();
      return false;
    }),
    parser.registerCsiHandler({ intermediates: "!", final: "p" }, () => {
      forget();
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
