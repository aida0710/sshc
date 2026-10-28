import { act, render, screen } from "@testing-library/react";
import { useEffect } from "react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThemeProvider, useTheme } from "./context";
import { localStorageKeys } from "../ui/browserStorageKeys";

let prefersDark = false;
const listeners = new Set<() => void>();

beforeEach(() => {
  prefersDark = false;
  listeners.clear();
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    get matches() {
      return query.includes("dark") && prefersDark;
    },
    media: query,
    addEventListener: (_: string, handler: () => void) => listeners.add(handler),
    removeEventListener: (_: string, handler: () => void) => listeners.delete(handler),
  }));
});

afterEach(() => {
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  document.documentElement.removeAttribute("data-scheme");
});

function Probe() {
  const { theme, setTheme, resolved } = useTheme();
  return (
    <div>
      <span>{`choice ${theme} resolved ${resolved}`}</span>
      <button type="button" onClick={() => setTheme("dark")}>go dark</button>
    </div>
  );
}

describe("ThemeProvider", () => {
  it("starts in the dark product theme when nothing was chosen", () => {
    render(<ThemeProvider><Probe /></ThemeProvider>);

    expect(screen.getByText("choice dark resolved dark")).toBeInTheDocument();
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("keeps the dark default when the system says dark", () => {
    prefersDark = true;
    render(<ThemeProvider><Probe /></ThemeProvider>);

    expect(screen.getByText("choice dark resolved dark")).toBeInTheDocument();
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("remembers a chosen theme and stamps it", async () => {
    const user = userEvent.setup();
    render(<ThemeProvider><Probe /></ThemeProvider>);

    await user.click(screen.getByRole("button", { name: "go dark" }));

    expect(screen.getByText("choice dark resolved dark")).toBeInTheDocument();
    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
    expect(window.localStorage.getItem(localStorageKeys.theme)).toBe("dark");
  });

  it("ignores the system once a theme was chosen", async () => {
    const user = userEvent.setup();
    render(<ThemeProvider initial="light"><Probe /></ThemeProvider>);

    await user.click(screen.getByRole("button", { name: "go dark" }));
    prefersDark = true;
    for (const handler of listeners) handler();

    expect(document.documentElement.getAttribute("data-theme")).toBe("dark");
  });

  it("picks the system back up when the choice returns to it", async () => {
    const user = userEvent.setup();

    function Returner() {
      const { setTheme, resolved } = useTheme();
      return (
        <div>
          <span>{`resolved ${resolved}`}</span>
          <button type="button" onClick={() => setTheme("system")}>go system</button>
        </div>
      );
    }

    render(<ThemeProvider initial="light"><Returner /></ThemeProvider>);
    expect(screen.getByText("resolved light")).toBeInTheDocument();

    prefersDark = true;
    for (const handler of listeners) handler();
    await user.click(screen.getByRole("button", { name: "go system" }));

    expect(screen.getByText("resolved dark")).toBeInTheDocument();
  });

  it("puts the theme on the page before a child's effect reads the page's colours", () => {
    // The terminal and Android's system bars read the theme's colours from the stylesheet in an effect.
    const seen: string[] = [];
    let choose: (theme: "light") => void = () => undefined;
    function Reader() {
      const { resolved, setTheme } = useTheme();
      choose = setTheme;
      useEffect(() => {
        seen.push(`${resolved} on page ${document.documentElement.getAttribute("data-theme")}`);
      }, [resolved]);
      return null;
    }

    render(<ThemeProvider initial="dark"><Reader /></ThemeProvider>);
    act(() => choose("light"));

    expect(seen).toEqual(["dark on page dark", "light on page light"]);
    expect(document.documentElement.getAttribute("data-scheme")).toBe("light");
  });
});
