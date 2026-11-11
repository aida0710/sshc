import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const sourceRoot = join(__dirname, "..");

// 角丸の段階は index.css の @theme だけが持つ。テストは一覧を写さず、そこから読む。
function scaleNamedInTheme(): string[] {
  const css = readFileSync(join(sourceRoot, "index.css"), "utf8");
  return [...css.matchAll(/^\s*--radius-([a-z0-9]+):/gm)].map((found) => found[1] ?? "");
}

const corner = "(?:-(?:t|b|l|r|s|e|tl|tr|bl|br|ss|se|es|ee))?";
// 図の細い棒や升目に付ける 3px 以下の角丸は、部品の角丸の段階に入れない。
const hairline = String.raw`\[[123]px\]`;
const roundedClass = /(?<![\w-])rounded(?:-[\w.[\]%()-]+)?!?(?![\w-])/g;

// クラス名は文字列の中にしか現れない。コメントと、文字列を持たない行（変数名など）は見ない。
function mayHoldClassNames(line: string): boolean {
  const trimmed = line.trimStart();
  if (trimmed.startsWith("//") || trimmed.startsWith("*") || trimmed.startsWith("/*")) return false;
  return /["`]/.test(line);
}

function cornerClassesOutsideScale(line: string, scale: string[]): string[] {
  if (!mayHoldClassNames(line)) return [];
  const allowed = new RegExp(String.raw`^rounded${corner}-(?:none|full|${scale.join("|")}|${hairline})!?$`);
  return (line.match(roundedClass) ?? []).filter((found) => !allowed.test(found));
}

function sources(directory: string): string[] {
  return readdirSync(directory).flatMap((entry) => {
    const path = join(directory, entry);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.tsx?$/.test(path) && !/\.test\.tsx?$/.test(path) ? [path] : [];
  });
}

describe("the corner radius scale", () => {
  it("has the five steps and the composer's radius, each given in pixels", () => {
    const css = readFileSync(join(sourceRoot, "index.css"), "utf8");
    const steps = Object.fromEntries([...css.matchAll(/^\s*--radius-([a-z0-9]+):\s*([^;]+);/gm)].map((found) => [found[1], found[2]]));

    expect(steps).toEqual({ sm: "6px", md: "8px", inset: "10px", lg: "12px", card: "14px", composer: "22px" });
  });
});

describe("looking for a corner radius outside the scale", () => {
  const scale = ["sm", "md", "lg"];

  it("reports a bare rounded, which is Tailwind's 4px", () => {
    expect(cornerClassesOutsideScale('<div className="rounded border" />', scale)).toEqual(["rounded"]);
  });

  it("reports a radius written as an arbitrary value", () => {
    expect(cornerClassesOutsideScale('<div className="rounded-[10px] p-3" />', scale)).toEqual(["rounded-[10px]"]);
  });

  it("reports a Tailwind step that the scale does not name", () => {
    expect(cornerClassesOutsideScale('<div className="md:rounded-xl" />', scale)).toEqual(["rounded-xl"]);
  });

  it("reports a radius outside the scale on one side only", () => {
    expect(cornerClassesOutsideScale('<div className="rounded-r-[5px]" />', scale)).toEqual(["rounded-r-[5px]"]);
  });

  it("accepts the steps of the scale, on every side or one, and when marked important", () => {
    expect(cornerClassesOutsideScale('<div className="rounded-md rounded-t-lg after:rounded-sm rounded-none!" />', scale)).toEqual([]);
  });

  it("accepts a pill and a square corner", () => {
    expect(cornerClassesOutsideScale('<div className="rounded-full rounded-none" />', scale)).toEqual([]);
  });

  it("accepts three pixels or fewer, for the thin bars of a figure", () => {
    expect(cornerClassesOutsideScale('<span className="rounded-[3px] rounded-[1px]" />', scale)).toEqual([]);
    expect(cornerClassesOutsideScale('<span className="rounded-[4px]" />', scale)).toEqual(["rounded-[4px]"]);
  });

  it("ignores the word in a comment and in a variable's name", () => {
    expect(cornerClassesOutsideScale("// a card's rounded corners", scale)).toEqual([]);
    expect(cornerClassesOutsideScale("  const rounded = clampSplitRatio(ratio);", scale)).toEqual([]);
  });
});

describe("the application", () => {
  it("takes every corner radius from the scale in index.css", () => {
    const scale = scaleNamedInTheme();
    const offenders = sources(sourceRoot).flatMap((path) =>
      readFileSync(path, "utf8")
        .split("\n")
        .flatMap((line, index) => cornerClassesOutsideScale(line, scale).map((found) => `${path.slice(path.indexOf("/src/") + 1)}:${index + 1} ${found}`)),
    );

    expect(offenders, `use a radius named in index.css instead:\n${offenders.join("\n")}`).toEqual([]);
  });
});
