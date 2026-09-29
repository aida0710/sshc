// CA の証明書として貼り付けた PEM の形を確かめる。engine（Go の caCertificateBlocks）と
// 同じく、証明書のブロックだけが空白で区切られて並んでいることを求める。
//
// 証明書の中身（X.509 として読めるか）は engine だけが確かめる。ここでは、中身が DER の
// SEQUENCE 1つで終わっていることまでを見る。

// pemBlock は、先頭の PEM のブロックひとつである。種類は END の行と揃っていること。
const pemBlock = /^-----BEGIN ([A-Z0-9 ]+)-----\r?\n([\s\S]*?)-----END \1-----/;
const pemCertificateType = "CERTIFICATE";
const base64Body = /^[A-Za-z0-9+/]*={0,2}$/;
// derSequenceTag は、DER の SEQUENCE の先頭の1バイトである。証明書は SEQUENCE である。
const derSequenceTag = 0x30;
// derLongFormFlag は、DER の長さが後ろのバイトに続くことを表すビットである。
const derLongFormFlag = 0x80;
// maxDERLengthBytes は、読む長さのバイト数の上限である。PEM の上限（16 KiB）に足りる。
const maxDERLengthBytes = 4;

function decodeBase64(body: string): Uint8Array | null {
  const compact = body.replace(/\s+/g, "");
  if (compact === "" || compact.length % 4 !== 0 || !base64Body.test(compact)) return null;
  const binary = atob(compact);
  return Uint8Array.from(binary, (character) => character.charCodeAt(0));
}

// isSingleDERSequence は、bytes が DER の SEQUENCE 1つだけでできているかを返す。
function isSingleDERSequence(bytes: Uint8Array): boolean {
  if (bytes.length < 2 || bytes[0] !== derSequenceTag) return false;
  const first = bytes[1] ?? 0;
  if ((first & derLongFormFlag) === 0) return bytes.length === 2 + first;
  const lengthBytes = first & ~derLongFormFlag;
  if (lengthBytes === 0 || lengthBytes > maxDERLengthBytes || bytes.length < 2 + lengthBytes) return false;
  let length = 0;
  for (let index = 0; index < lengthBytes; index++) length = length * 256 + (bytes[2 + index] ?? 0);
  return bytes.length === 2 + lengthBytes + length;
}

// isPEMCertificateList は、text が1枚以上の PEM の証明書だけでできているかを返す。
export function isPEMCertificateList(text: string): boolean {
  let rest = text.trim();
  if (rest === "") return false;
  while (rest !== "") {
    const match = pemBlock.exec(rest);
    if (match === null || match[1] !== pemCertificateType) return false;
    const der = decodeBase64(match[2] ?? "");
    if (der === null || !isSingleDERSequence(der)) return false;
    rest = rest.slice(match[0].length).trim();
  }
  return true;
}
