const dnsOrIPv4Pattern = /^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$/;

function validIPv4(value: string): boolean {
  const parts = value.split(".");
  return parts.length === 4 && parts.every((part) =>
    /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255,
  );
}

function validIPv6(value: string): boolean {
  let expanded = value;
  if (value.includes(".")) {
    const separator = value.lastIndexOf(":");
    if (separator < 0 || !validIPv4(value.slice(separator + 1))) return false;
    expanded = `${value.slice(0, separator)}:0:0`;
  }
  const compression = expanded.indexOf("::");
  if (compression !== expanded.lastIndexOf("::")) return false;
  const compressed = compression >= 0;
  const sides = compressed ? expanded.split("::") : [expanded];
  if (sides.some((side) => side !== "" && side.split(":").some((part) => !/^[0-9A-Fa-f]{1,4}$/.test(part)))) {
    return false;
  }
  const groups = sides.reduce((total, side) => total + (side === "" ? 0 : side.split(":").length), 0);
  return compressed ? groups < 8 : groups === 8;
}

// OpenSSH accepts unbracketed IPv6 in HostName. Keep the browser check aligned
// with the Go boundary so compressed addresses beginning or ending in :: are
// not rejected before the request reaches the server.
export function validHostNameInput(value: string): boolean {
  if (value.length === 0 || value.length > 255) return false;
  return value.includes(":") ? validIPv6(value) : dnsOrIPv4Pattern.test(value);
}
