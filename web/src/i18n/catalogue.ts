export type CatalogueDifference = {
  missing: string[];
  extra: string[];
};

export function catalogueDifference(
  master: Record<string, string>,
  candidate: Record<string, string>,
): CatalogueDifference {
  const masterKeys = new Set(Object.keys(master));
  const candidateKeys = new Set(Object.keys(candidate));
  return {
    missing: [...masterKeys].filter((key) => !candidateKeys.has(key)).sort(),
    extra: [...candidateKeys].filter((key) => !masterKeys.has(key)).sort(),
  };
}
