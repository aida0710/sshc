export function prefersNativeSelection(match: (query: string) => { matches: boolean }): boolean {
  return match("(hover: none) and (pointer: coarse)").matches;
}
