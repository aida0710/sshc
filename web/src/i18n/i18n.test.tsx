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
        "diag.hostAlias",
        "explorer.fileState",
        "groups.directories",
        "host.tabRaw",
        "keys.agentHeading",
        "keys.certKeyId",
        "keys.reference",
        "keys.blockerOther",
        "keys.relocateFilePair",
        "keys.relocateReference",
        "keys.unreadableEntry",
        "kh.heading",
        "rk.alias",
        "rk.hostAlias",
        "section.knownHosts",
        "shell.languageEnglish",
        "shell.languageJapanese",
        "shell.title",
        "terminal.localhost",
        "terminal.rowDetail",
      ].sort(),
    );
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
