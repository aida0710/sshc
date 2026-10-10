// Each language is named in itself so a visitor can find their own.
export const languageNames = { ja: "日本語", en: "English" };
// Like the sshc Web UI, visitors who prefer neither language read English.
const defaultLanguage = "en";
// The choice is a per-viewer convenience; without storage the browser language decides.
const languageStorageKey = "sshc-demo-language";

export function isLanguage(value) {
  return typeof value === "string" && Object.hasOwn(languageNames, value);
}

export function chooseLanguage({ saved, browserLanguages }) {
  if (isLanguage(saved)) return saved;
  for (const candidate of browserLanguages) {
    const primary = candidate.split("-")[0].toLowerCase();
    if (isLanguage(primary)) return primary;
  }
  return defaultLanguage;
}

export function readSavedLanguage() {
  try {
    return localStorage.getItem(languageStorageKey);
  } catch {
    return null;
  }
}

export function saveLanguage(language) {
  try {
    localStorage.setItem(languageStorageKey, language);
  } catch {
    // Private windows may refuse storage; the reload then keeps the browser language.
  }
}
