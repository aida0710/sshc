import {
  shortcutCommandModifiers,
  shortcutFunctionKeyPattern,
  shortcutKeyPattern,
  shortcutModifiers,
} from "./shortcuts.generated";

// isValidShortcutChord は internal/validate の ShortcutChord と同じ答えを出す。
// 画面が記録した和音を engine が断ると保存が理由なく失敗するので、名前の規則と
// 違って緩い側にもずらさない。shortcutChord.test.ts がコーパスで両方向を確かめる。
export function isValidShortcutChord(chord: string): boolean {
  const separator = chord.lastIndexOf("+");
  const key = chord.slice(separator + 1);
  if (!shortcutKeyPattern.test(key)) return false;
  const modifiers = separator < 0 ? [] : chord.slice(0, separator).split("+");
  if (!inShortcutModifierOrder(modifiers)) return false;
  return shortcutFunctionKeyPattern.test(key) || modifiers.some((modifier) => shortcutCommandModifiers.includes(modifier));
}

// 修飾が shortcutModifiers の順に、重ならずに並んでいるか。
function inShortcutModifierOrder(modifiers: readonly string[]): boolean {
  let next = 0;
  for (const modifier of modifiers) {
    while (next < shortcutModifiers.length && shortcutModifiers[next] !== modifier) next += 1;
    if (next === shortcutModifiers.length) return false;
    next += 1;
  }
  return true;
}
