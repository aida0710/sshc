// Matching text quoted or clicked by its wording against the screen's wording catalogue, for tests that
// keep text outside the screen (pages, the browser demo's check) in step with it.

const placeholderPattern = /\{[^{}]+\}/g;
// 「（バイト）」のような末尾の補足は、本文で省いて引用してよい。
const trailingNotePattern = /（[^（）]*）$/;
// {name}の差し込みは、英数字の例（「bastionへ接続中 · 1/2」）か<名前>の形で書く。
// 日本語の文字を差し込みに含めると、「{a}の{b}」のような文言が「の」を含むどの引用とも一致してしまう。
const placeholderExample = String.raw`(?:<[^<>]*>|[\x20-\x7E]*)`;

function escapeForPattern(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function toPattern(label: string): RegExp {
  const source = label.split(placeholderPattern).map(escapeForPattern).join(placeholderExample);
  return new RegExp(`^${source}$`, "u");
}

// 差し込みを除いて文字が残らない文言（「{detail}」など）は、どんな引用とも一致してしまうので照合に使わない。
function hasWording(label: string): boolean {
  return /\p{L}/u.test(label.replace(placeholderPattern, ""));
}

// labelPatterns are the catalogue's labels as patterns a quote can match, with their {placeholders}
// standing for an example and their trailing notes optional.
export function labelPatterns(labels: string[]): RegExp[] {
  const variants = labels.flatMap((label) => [label, label.replace(trailingNotePattern, "")]);
  return [...new Set(variants)].filter(hasWording).map(toPattern);
}
