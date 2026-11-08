import { describe, expect, it } from "vitest";
import { androidCatalogue } from "./androidCatalogue";
import { locales } from "./locale";
import { messages } from "./messages";

// Shown only where the Android app does not go: it asks for its own keys (terminal.androidNotifications*,
// terminal.androidNotificationSoundHint).
const browserOnlyKeys = new Set(["terminal.browserNotificationsDefault", "terminal.browserNotificationsGranted", "terminal.notificationSoundHint"]);

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

  it("calls the files beside the app those of this device, not the engine's", () => {
    expect(androidCatalogue("ja")["sftp.local.engine"]).toBe("このデバイス");
    expect(androidCatalogue("en")["sftp.local.engine"]).toBe("This device");
  });

  it("rewords what the catalogue says of this machine", () => {
    expect(androidCatalogue("ja")["settings.thisMachine"]).toBe("このデバイス");
    expect(androidCatalogue("ja")["start.openSessionNote"]).toBe("接続先かこのデバイスのシェルを選びます");
    expect(androidCatalogue("en")["settings.thisMachine"]).toBe("This device");
    expect(androidCatalogue("en")["start.openSessionNote"]).toBe("Choose a connection or a shell on this device");
  });

  it("says that Android notifies about a waiting program, where a browser's wording says the browser does", () => {
    expect(messages.ja["terminal.noticeNotificationPermissionBody"]).toContain("ブラウザの通知");
    expect(androidCatalogue("ja")["terminal.noticeNotificationPermissionBody"]).toContain("Androidの通知");
    expect(messages.en["terminal.noticeNotificationPermissionBody"]).toContain("your browser notifies");
    expect(androidCatalogue("en")["terminal.noticeNotificationPermissionBody"]).toContain("Android notifies");
  });

  it("does not send a person to a command line they do not have", () => {
    expect(androidCatalogue("ja")["shell.sessionEnded"]).not.toContain("ターミナルでsshc");
    expect(androidCatalogue("ja")["shell.bootstrapFailed"]).not.toContain("ターミナルでsshc");
    expect(androidCatalogue("en")["shell.sessionEnded"]).not.toContain("run sshc");
    expect(androidCatalogue("en")["shell.bootstrapFailed"]).not.toContain("Run sshc");
  });

  it("does not speak of an agent or an engine when closing every connection", () => {
    expect(androidCatalogue("ja")["desktop.closeAllNote"]).not.toMatch(/ssh-agent|エンジン/u);
    expect(androidCatalogue("en")["desktop.closeAllNote"]).not.toMatch(/agent|engine/iu);
  });

  it("keeps a browser's wording as it is", () => {
    expect(messages.ja["settings.languageBrowser"]).toBe("ブラウザに合わせる");
    expect(androidCatalogue("ja")["settings.languageBrowser"]).toBe("デバイスに合わせる");
  });
});
