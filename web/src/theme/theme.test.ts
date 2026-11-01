import { afterEach, describe, expect, it } from "vitest";
import {
  applyTheme,
  detectTheme,
  isFirstDarkTheme,
  isTheme,
  copyTheme,
  resolveTheme,
  schemeOf,
  themeLabel,
  themes,
} from "./theme";
import { localStorageKeys } from "../ui/browserStorageKeys";

afterEach(() => {
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-scheme");
});

describe("isTheme", () => {
  it("accepts system and every theme, and nothing else", () => {
    expect(themes).toEqual(["system", "light", "white-blue", "white-purple", "dark", "dark-blue", "dark-purple"]);
    for (const theme of themes) expect(isTheme(theme)).toBe(true);
    expect(isTheme("solarized")).toBe(false);
    expect(isTheme(undefined)).toBe(false);
  });
});

describe("schemeOf", () => {
  it("tells a light theme from a dark one", () => {
    expect(schemeOf("light")).toBe("light");
    expect(schemeOf("white-blue")).toBe("light");
    expect(schemeOf("white-purple")).toBe("light");
    expect(schemeOf("dark")).toBe("dark");
    expect(schemeOf("dark-blue")).toBe("dark");
    expect(schemeOf("dark-purple")).toBe("dark");
  });
});

describe("isFirstDarkTheme", () => {
  it("marks the dark theme that comes first in the list, where the settings page starts a new row", () => {
    expect(themes.filter(isFirstDarkTheme)).toEqual(["dark"]);
  });
});

describe("themeLabel", () => {
  it("names system and each theme", () => {
    expect(themeLabel("system")).toBe("shell.themeSystem");
    expect(themeLabel("dark")).toBe("shell.themeDark");
    expect(themeLabel("dark-blue")).toBe("shell.themeDarkBlue");
    expect(themeLabel("dark-purple")).toBe("shell.themeDarkPurple");
    expect(themeLabel("white-blue")).toBe("shell.themeWhiteBlue");
    expect(themeLabel("white-purple")).toBe("shell.themeWhitePurple");
  });
});

describe("detectTheme", () => {
  it("starts in the dark product theme when nothing has been chosen", () => {
    expect(detectTheme()).toBe("dark");
  });

  it("reads the copy kept for the first paint", () => {
    copyTheme("light");
    expect(window.localStorage.getItem(localStorageKeys.theme)).toBe("light");
    expect(detectTheme()).toBe("light");
  });

  it("falls back to dark when the stored value is not a theme", () => {
    window.localStorage.setItem(localStorageKeys.theme, "solarized");
    expect(detectTheme()).toBe("dark");
  });
});

describe("resolveTheme", () => {
  it("follows the system when the choice is system", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
  });

  it("overrides the system when a theme was chosen", () => {
    expect(resolveTheme("light", true)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
  });
});

describe("applyTheme", () => {
  it("stamps the resolved theme and its scheme on the element", () => {
    const root = document.documentElement;
    applyTheme(root, "dark");
    expect([root.getAttribute("data-theme"), root.getAttribute("data-scheme")]).toEqual(["dark", "dark"]);
    applyTheme(root, "light");
    expect([root.getAttribute("data-theme"), root.getAttribute("data-scheme")]).toEqual(["light", "light"]);
  });
});
