// GNU tar's ustar output uses 512-byte headers and pads every body to that boundary.
const tarBlockBytes = 512;
const decoder = new TextDecoder("utf-8", { fatal: true });

export function readTarFiles(archive) {
  const files = [];
  const paths = new Set();
  let offset = 0;
  while (offset + tarBlockBytes <= archive.length) {
    const header = archive.subarray(offset, offset + tarBlockBytes);
    if (header.every((byte) => byte === 0)) return files;
    const checksum = header.reduce((sum, byte, index) => sum + (index >= 148 && index < 156 ? 32 : byte), 0);
    if (readOctal(header.subarray(148, 156)) !== checksum) throw new Error("Invalid tar checksum");
    if (readText(header.subarray(257, 263)) !== "ustar") throw new Error("Expected ustar archive");
    const prefix = readText(header.subarray(345, 500));
    const name = readText(header.subarray(0, 100));
    const path = (prefix ? `${prefix}/${name}` : name).replace(/^\.\//, "");
    const size = readOctal(header.subarray(124, 136));
    const bodyStart = offset + tarBlockBytes;
    const nextOffset = bodyStart + Math.ceil(size / tarBlockBytes) * tarBlockBytes;
    if (nextOffset > archive.length) throw new Error("Truncated tar body");
    const type = header[156];
    if (type === 0 || type === 48) {
      if (!path || path.split("/").some((part) => !part || part === "." || part === "..") ||
          /[\\%?#]/.test(path) || paths.has(path)) throw new Error("Invalid tar file path");
      paths.add(path);
      files.push({ path, bytes: archive.subarray(bodyStart, bodyStart + size) });
    } else if (type !== 53) {
      throw new Error("Unsupported tar entry");
    }
    offset = nextOffset;
  }
  throw new Error("Missing tar end marker");
}

function readText(bytes) {
  const end = bytes.indexOf(0);
  return decoder.decode(end === -1 ? bytes : bytes.subarray(0, end));
}

function readOctal(bytes) {
  const text = readText(bytes).trim();
  if (!/^[0-7]+$/.test(text)) throw new Error("Invalid tar number");
  const number = Number.parseInt(text, 8);
  if (!Number.isSafeInteger(number)) throw new Error("Tar number exceeds supported range");
  return number;
}
