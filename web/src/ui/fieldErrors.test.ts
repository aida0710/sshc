import { describe, expect, it } from "vitest";
import { applicationSourceFiles } from "../testing/applicationSources";

// 入力欄の誤りは、欄の枠を赤くし（aria-invalid）、文を共通の字（fieldErrorText）で出す（2026-11-25、入力のダイアログ、
// カスタムキー、パスの欄、新しいグループの欄、マシンのポートがそれぞれ自前で書いていたのをそろえた）。Field、FieldRow、
// useFieldError を使えばこれは自ずと揃うので、この検査は aria-invalid を手で付けた欄を見る。そのファイルが文を
// FieldProblems か fieldErrorText で出していなければ報告する。外すものは、要素の前の行に field-error-exempt と理由を書く。
const exemption = "field-error-exempt";

// 受け取った aria-invalid を中の入力欄へ渡すだけの部品（PasswordInput、CodeField など）は、文を出さないので見ない。
const passedThrough = /aria-invalid=\{(invalid|description\["aria-invalid"\])\}/;
const sharedErrorText = /\b(FieldProblems|fieldErrorText)\b/;

function handSetInvalidFields(): string[] {
  return applicationSourceFiles()
    .filter((file) => file.name.endsWith(".tsx") && !sharedErrorText.test(file.text))
    .flatMap((file) => {
      const lines = file.text.split("\n");
      return lines.flatMap((line, index) =>
        /aria-invalid=\{/.test(line) && !passedThrough.test(line) && !(lines[index - 1] ?? "").includes(exemption)
          ? [`${file.name}:${index + 1}`]
          : []);
    });
}

describe("fields that say their value is wrong", () => {
  it("show what is wrong in the shared red line", () => {
    const offenders = handSetInvalidFields();
    expect(offenders, `show the problem with FieldProblems or fieldErrorText (or use Field, FieldRow, useFieldError), or say why not:\n${offenders.join("\n")}`).toEqual([]);
  });
});
