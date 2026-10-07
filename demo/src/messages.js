import { chooseLanguage, readSavedLanguage } from "./language.js";
import { englishMessages } from "./messages-en.js";
import { japaneseMessages } from "./messages-ja.js";

const catalogs = { ja: japaneseMessages, en: englishMessages };
export const language = chooseLanguage({ saved: readSavedLanguage(),
  browserLanguages: navigator.languages ?? [navigator.language] });
export const messages = catalogs[language];

export function applyMessages(document) {
  document.documentElement.lang = language;
  for (const element of document.querySelectorAll("[data-message]")) {
    element.textContent = messages[element.dataset.message];
  }
  document.title = messages.pageTitle;
  document.getElementById("demo-tabs").setAttribute("aria-label", messages.tabsLabel);
  document.getElementById("sshc-ui").title = messages.frameTitle;
}
