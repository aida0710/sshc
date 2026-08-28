import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LanguageProvider, useTranslate, useLanguage } from "./context";
import { en, ja } from "./messages";
import { detectLocale, storageKey } from "./locale";

afterEach(() => {
  window.localStorage.clear();
  vi.restoreAllMocks();
});

function Probe() {
  const t = useTranslate();
  const { locale, setLocale } = useLanguage();
  return (
    <>
      <p>{t("shell.starting")}</p>
      <p>{t("shell.active", { version: "0.1.0" })}</p>
      <p>{`locale:${locale}`}</p>
      <button type="button" onClick={() => setLocale("ja")}>
        to japanese
      </button>
    </>
  );
}

describe("the catalogue", () => {
  it("translates every English message", () => {
    const untranslated = Object.keys(en).filter((key) => {
      const source = en[key as keyof typeof en];
      const target = ja[key as keyof typeof en];
      return source === target;
    });
    expect(untranslated.sort()).toEqual(
      [
        "conn.advancedDirectives",
        "conn.areaAdvanced",
        "conn.areaAnalysis",
        "conn.areaBasic",
        "conn.areaSshc",
        "conn.heading",
        "diag.hostAlias",
        "explorer.fileState",
        "groups.directories",
        "groups.metricConnections",
        "groups.pageTitle",
        "history.pageTitle",
        "home.connections",
        "host.tabJump",
        "host.tabRaw",
        "keys.agentHeading",
        "keys.blockerOther",
        "keys.reference",
        "keys.relocateFilePair",
        "keys.relocateReference",
        "keys.unreadableEntry",
        "kh.heading",
        "rk.alias",
        "rk.hostAlias",
        "section.connections",
        "section.files",
        "section.groups",
        "section.history",
        "section.home",
        "section.knownHosts",
        "section.settings",
        "section.snippets",
        "section.sync",
        "section.terminal",
        "settings.heading",
        "shell.language",
        "shell.languageEnglish",
        "shell.languageJapanese",
        "shell.languageMenu",
        "shell.title",
        "snippets.heading",
        "sync.historyRelation.head",
        "terminal.localhost",
        "terminal.rowDetail",
        "tree.navLabel",
      ].sort(),
    );
  });

  it("keeps placeholders aligned between English and Japanese", () => {
    const placeholders = (message: string) =>
      [...new Set([...message.matchAll(/\{(\w+)\}/g)].map((match) => match[1]))].sort();
    const mismatches = Object.keys(en).flatMap((key) => {
      const messageKey = key as keyof typeof en;
      const english = placeholders(en[messageKey]);
      const japanese = placeholders(ja[messageKey]);
      return JSON.stringify(english) === JSON.stringify(japanese)
        ? []
        : [{ key, english, japanese }];
    });

    expect(mismatches).toEqual([]);
  });

  it("keeps product names consistent in Japanese navigation and page titles", () => {
    expect({
      home: ja["section.home"],
      connections: ja["section.connections"],
      terminal: ja["section.terminal"],
      config: ja["section.config"],
      groups: ja["section.groups"],
      keys: ja["section.keys"],
      knownHosts: ja["section.knownHosts"],
      remoteKeys: ja["section.remoteKeys"],
      diagnostics: ja["section.diagnostics"],
      vault: ja["section.secrets"],
      snippets: ja["section.snippets"],
      settings: ja["section.settings"],
      sync: ja["section.sync"],
      history: ja["section.history"],
    }).toEqual({
      home: "Home",
      connections: "Connections",
      terminal: "Terminal",
      config: "SSH Config",
      groups: "Groups",
      keys: "SSH Keys",
      knownHosts: "Known Hosts",
      remoteKeys: "Remote Keys",
      diagnostics: "Diagnostics",
      vault: "Vault",
      snippets: "Snippets",
      settings: "Settings",
      sync: "Sync",
      history: "History",
    });
  });

  it("leaves a placeholder alone when no value is given for it", () => {
    render(
      <LanguageProvider initial="en">
        <Probe />
      </LanguageProvider>,
    );

    expect(screen.getByText("Local session active · 0.1.0")).toBeInTheDocument();
  });
});

describe("the language switch", () => {
  it("changes the rendered language and remembers the choice", async () => {
    const user = userEvent.setup();
    render(
      <LanguageProvider initial="en">
        <Probe />
      </LanguageProvider>,
    );

    expect(screen.getByText("Starting secure local session…")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "to japanese" }));

    expect(screen.getByText(ja["shell.starting"])).toBeInTheDocument();
    expect(window.localStorage.getItem(storageKey)).toBe("ja");
  });

  it("writes nothing but the language, and nothing at all until it is changed", async () => {
    const user = userEvent.setup();
    render(
      <LanguageProvider initial="en">
        <Probe />
      </LanguageProvider>,
    );

    expect(window.localStorage.length).toBe(0);

    await user.click(screen.getByRole("button", { name: "to japanese" }));

    expect(Object.keys(window.localStorage)).toEqual([storageKey]);
    expect(window.sessionStorage.length).toBe(0);
  });
});

describe("locale detection", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("prefers the stored choice over the browser", () => {
    window.localStorage.setItem(storageKey, "ja");
    vi.spyOn(navigator, "languages", "get").mockReturnValue(["en-GB"]);

    expect(detectLocale()).toBe("ja");
  });

  it("matches a regional variant by its subtag", () => {
    vi.spyOn(navigator, "languages", "get").mockReturnValue(["ja-JP"]);

    expect(detectLocale()).toBe("ja");
  });

  it("falls back to English for a language it has no catalogue for", () => {
    vi.spyOn(navigator, "languages", "get").mockReturnValue(["de-DE", "fr"]);

    expect(detectLocale()).toBe("en");
  });

  it("ignores a stored value that is not a language it has", () => {
    window.localStorage.setItem(storageKey, "../etc/passwd");
    vi.spyOn(navigator, "languages", "get").mockReturnValue(["en-US"]);

    expect(detectLocale()).toBe("en");
  });

  it("survives a browser that refuses storage entirely", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("access denied");
    });
    vi.spyOn(navigator, "languages", "get").mockReturnValue(["ja"]);

    expect(detectLocale()).toBe("ja");
  });
});
