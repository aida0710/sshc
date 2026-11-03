// The names people know the keys by. The stored chords use the names of the browser's KeyboardEvent.
const keyNames: Record<string, string> = { ArrowUp: "↑", ArrowDown: "↓", ArrowLeft: "←", ArrowRight: "→" };

function metaKeyName(): string {
  return /Mac|iPhone|iPad/u.test(navigator.userAgent) ? "⌘" : "Win";
}

// shortcutLabel writes a stored chord (Ctrl+Shift+ArrowUp, Meta+K) as the screen shows it: Ctrl+Shift+↑, ⌘+K.
export function shortcutLabel(chord: string): string {
  return chord.split("+").map((part) => part === "Meta" ? metaKeyName() : keyNames[part] ?? part).join("+");
}
