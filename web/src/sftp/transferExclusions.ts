// Match the engine's deliberately small glob language without regex from users.
export const maximumExclusionPatterns = 64;
export const maximumExclusionPatternBytes = 512;

export function validTransferExclusionPatterns(patterns: readonly string[]): boolean {
  return patterns.length <= maximumExclusionPatterns && patterns.every((pattern) => {
    if (pattern === "" || new TextEncoder().encode(pattern).length > maximumExclusionPatternBytes || pattern.trim() !== pattern ||
      /[\\\[\]!\p{Cc}]/u.test(pattern) || pattern.includes("**")) return false;
    return pattern.split("/").every((segment) => segment !== "" && segment !== "." && segment !== "..");
  });
}

// Keep matching bounded: translating repeated stars into a backtracking
// regular expression can freeze the browser on perfectly valid patterns.
function matchesExclusion(pattern: readonly string[], candidate: string): boolean {
  const characters = Array.from(candidate);
  let patternIndex = 0;
  let characterIndex = 0;
  let lastStar = -1;
  let starMatchIndex = 0;
  while (characterIndex < characters.length) {
    const character = characters[characterIndex];
    const token = pattern[patternIndex];
    if ((token !== "*" && token === character) || (token === "?" && character !== "/")) {
      patternIndex += 1;
      characterIndex += 1;
    } else if (token === "*") {
      lastStar = patternIndex;
      patternIndex += 1;
      starMatchIndex = characterIndex;
    } else if (lastStar >= 0 && characters[starMatchIndex] !== "/") {
      patternIndex = lastStar + 1;
      starMatchIndex += 1;
      characterIndex = starMatchIndex;
    } else return false;
  }
  while (pattern[patternIndex] === "*") patternIndex += 1;
  return patternIndex === pattern.length;
}

export function createTransferExclusions(patterns: readonly string[]): (relativePath: string) => boolean {
  const rules = patterns.map((pattern) => ({ pattern: Array.from(pattern), relative: pattern.includes("/") }));
  return (relativePath) => {
    const segments = relativePath.split("/");
    return segments.some((segment, index) => rules.some((rule) => matchesExclusion(rule.pattern, rule.relative ? segments.slice(0, index + 1).join("/") : segment)));
  };
}

// Directory selections include the root folder name. The root itself stays;
// rules start at its children, as they do for engine-side folder transfers.
export function browserUploadExcluded(relativePath: string, excludes: (path: string) => boolean): boolean {
  const separator = relativePath.indexOf("/");
  return separator >= 0 && excludes(relativePath.slice(separator + 1));
}
