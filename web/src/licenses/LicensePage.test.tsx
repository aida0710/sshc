import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { createHash } from "node:crypto";
import { resolve } from "node:path";
import { LicensePage } from "./LicensePage";
import sources from "./sources.generated.json";
import catalogue from "./catalogue.generated.json";

describe("LicensePage", () => {
  it("searches and opens bundled copyright and full license text", async () => {
    render(<LicensePage />);
    await userEvent.type(screen.getByRole("searchbox"), "Devicon");
    expect(screen.getByRole("status")).toHaveTextContent("Components: 1");
    await userEvent.click(screen.getByText("Devicon"));
    expect(screen.getByText(/Copyright \(c\) 2015 konpa/)).toBeVisible();
    expect(screen.getByText(/THE SOFTWARE IS PROVIDED/)).toBeVisible();
    await userEvent.clear(screen.getByRole("searchbox"));
    await userEvent.type(screen.getByRole("searchbox"), "not-a-real-package");
    expect(screen.getByText("No matching components.")).toBeVisible();
  });

  it("requires regeneration when dependency locks or bundled licenses change", () => {
    for (const [path, expected] of Object.entries(sources)) {
      const actual = createHash("sha256").update(readFileSync(resolve("..", path))).digest("hex");
      expect(actual, `Run scripts/licenses/generate.py after changing ${path}`).toBe(expected);
    }
  });

  it("includes font, icon, editor third-party notices and platform runtimes", () => {
    expect(catalogue.find((entry) => entry.name === "JetBrains Mono")?.license).toBe("OFL-1.1");
    expect(catalogue.find((entry) => entry.name === "monaco-editor")?.notices.some((notice) => notice.name === "ThirdPartyNotices.txt")).toBe(true);
    expect(catalogue.some((entry) => entry.name === "androidx.activity:activity")).toBe(true);
    expect(catalogue.some((entry) => entry.name === "golang.org/x/crypto")).toBe(true);
    for (const entry of catalogue) {
      expect(entry.notices.length, entry.name).toBeGreaterThan(0);
      expect(new URL(entry.url).protocol).toMatch(/^https?:$/);
    }
  });
});
