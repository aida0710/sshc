import { describe, expect, it } from "vitest";
import type { Translate } from "../i18n/context";
import { en, ja, type MessageKey } from "../i18n/messages";
import { describeConfigDiagnostic, describeConfigDiagnosticGroup, groupConfigDiagnostics } from "./configDiagnostics";

function translator(language: "en" | "ja"): Translate {
  const catalogue = language === "ja" ? ja : en;
  return (key: MessageKey, values: Record<string, string | number> = {}) => {
    let message: string = catalogue[key];
    for (const [name, value] of Object.entries(values)) message = message.replaceAll(`{${name}}`, String(value));
    return message;
  };
}

const noMatch = { severity: "warning", code: "include_no_match", path: "config", line: 2, detail: "~/.ssh/missing/*.conf" };

describe("config diagnostics", () => {
  it("says what the problem is about and where, instead of the engine's code", () => {
    const sentence = describeConfigDiagnostic(translator("ja"), noMatch);
    expect(sentence).toBe("Includeに当てはまるファイルがありません：~/.ssh/missing/*.conf（config:2）");
    expect(sentence).not.toContain("include_no_match");
  });

  it("leaves out a detail that only repeats the sentence", () => {
    const sentence = describeConfigDiagnostic(translator("en"), {
      severity: "info", code: "unstructured_line", path: "conf.d/old.conf", line: 7, detail: "line is preserved verbatim and can only be edited as raw text",
    });
    expect(sentence).toBe("sshc cannot read this line; it can only be edited as text (conf.d/old.conf:7)");
  });

  it("keeps the code of a kind this screen does not know, so it can be looked up", () => {
    expect(describeConfigDiagnostic(translator("en"), { severity: "warning", code: "include_new_kind", path: "config", line: 0, detail: "x" }))
      .toBe("Configuration problem (include_new_kind): x (config)");
  });

  it("puts problems of one kind together, the most serious kind first, with a heading that counts them", () => {
    const groups = groupConfigDiagnostics([
      { severity: "info", code: "include_conditional", path: "config", line: 4, detail: "Host lab" },
      noMatch,
      { ...noMatch, line: 3 },
      { severity: "error", code: "include_cycle", path: "conf.d/a.conf", line: 1, detail: "conf.d/a.conf" },
    ]);
    expect(groups.map((group) => [group.code, group.diagnostics.length])).toEqual([
      ["include_cycle", 1], ["include_no_match", 2], ["include_conditional", 1],
    ]);
    expect(describeConfigDiagnosticGroup(translator("ja"), "include_no_match", 2)).toBe("Includeに当てはまるファイルが無い行が2件あります");
  });
});
