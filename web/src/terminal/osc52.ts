const maxClipboardBytes = 256 * 1024;
const base64Characters = /^[A-Za-z0-9+/]*={0,2}$/;

type Disposable = { dispose(): void };

type OscParser = {
  registerOscHandler(identifier: number, handler: (data: string) => boolean): Disposable;
};

type Osc52Options = {
  parser: OscParser;
  enabled: () => boolean;
  writeText: (text: string) => Promise<void>;
  copied?: () => void;
  refused?: () => void;
};

export function decodeOsc52(data: string, limit = maxClipboardBytes): string | null {
  const separator = data.indexOf(";");
  if (separator < 0) return null;
  const selection = data.slice(0, separator);
  const encoded = data.slice(separator + 1);
  if (selection !== "" && selection !== "c") return null;
  if (encoded === "?" || encoded.length > Math.ceil(limit / 3) * 4) return null;
  if (!base64Characters.test(encoded) || encoded.length % 4 === 1) return null;
  const padding = (4 - (encoded.length % 4)) % 4;
  try {
    const binary = atob(encoded + "=".repeat(padding));
    if (binary.length > limit) return null;
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
    return new TextDecoder().decode(bytes);
  } catch {
    return null;
  }
}

export function attachOsc52Clipboard({ parser, enabled, writeText, copied, refused }: Osc52Options): () => void {
  const registration = parser.registerOscHandler(52, (data) => {
    if (!enabled()) return true;
    const text = decodeOsc52(data);
    if (text === null) return true;
    void writeText(text).then(copied).catch(refused);
    return true;
  });
  return () => registration.dispose();
}
