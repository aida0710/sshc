import { describe, expect, it } from "vitest";
import { androidCatalogue } from "./androidCatalogue";
import { locales } from "./locale";
import { messages } from "./messages";

// Shown only where the Android app does not go: it asks for its own keys (terminal.androidNotifications*).
const browserOnlyKeys = new Set(["terminal.browserNotificationsDefault", "terminal.browserNotificationsGranted"]);

describe("the Android app's wording", () => {
  it("has a word for every key of the catalogue, in both languages", () => {
    for (const locale of locales) {
      expect(Object.keys(androidCatalogue(locale)).sort()).toEqual(Object.keys(messages[locale]).sort());
    }
  });

  it("calls the phone a device and never speaks of a tab or a browser of its own", () => {
    const stray = Object.entries(androidCatalogue("ja"))
      .filter(([key, text]) => !browserOnlyKeys.has(key) && /このマシン|このブラウザ|このタブ/u.test(text));
    expect(stray).toEqual([]);
    const strayEnglish = Object.entries(androidCatalogue("en"))
      .filter(([key, text]) => !browserOnlyKeys.has(key) && /\b[Tt]his (?:machine|browser)\b|this tab is in the background/u.test(text));
    expect(strayEnglish).toEqual([]);
  });

  it("does not send a person to a command line they do not have", () => {
    expect(androidCatalogue("ja")["shell.sessionEnded"]).not.toContain("ターミナルでsshc");
    expect(androidCatalogue("ja")["shell.bootstrapFailed"]).not.toContain("ターミナルでsshc");
    expect(androidCatalogue("en")["shell.sessionEnded"]).not.toContain("run sshc");
    expect(androidCatalogue("en")["shell.bootstrapFailed"]).not.toContain("Run sshc");
  });

  it("keeps a browser's wording as it is", () => {
    expect(messages.ja["settings.languageBrowser"]).toBe("ブラウザに合わせる");
    expect(androidCatalogue("ja")["settings.languageBrowser"]).toBe("デバイスに合わせる");
  });
});
