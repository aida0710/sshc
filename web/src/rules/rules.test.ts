import { describe, expect, it } from "vitest";
import corpus from "./corpus.generated.json";
import {
  formatDirectiveValues,
  formatValues,
  isValidAlias,
  isValidGroupName,
  isValidHostName,
  parseDirectiveValues,
  parseValues,
} from "./rules";


function neverStricter(
  kind: string,
  cases: readonly { input: string; valid: boolean; why: string }[],
  accepts: (input: string) => boolean,
) {
  describe(`${kind}: サーバーより厳しくしない`, () => {
    for (const item of cases) {
      if (!item.valid) continue;
      it(`受け入れる: ${JSON.stringify(item.input)}: ${item.why}`, () => {
        expect(accepts(item.input)).toBe(true);
      });
    }
  });
}

neverStricter("グループ名", corpus.groupName, isValidGroupName);
neverStricter("ホスト名", corpus.hostName, isValidHostName);
neverStricter("alias", corpus.alias, isValidAlias);

describe("画面がサーバーより緩いところ", () => {
  it("グループ名で見逃すものを数える", () => {
    const missed = corpus.groupName.filter((item) => !item.valid && isValidGroupName(item.input));
    expect(missed.map((item) => item.input)).toEqual([]);
  });

  it("alias で見逃すものを数える", () => {
    const missed = corpus.alias.filter((item) => !item.valid && isValidAlias(item.input));
    expect(missed.map((item) => item.input)).toEqual([]);
  });
});

describe("ssh_config の文字列検証", () => {
  for (const item of corpus.render) {
    if (item.refused) {
      it(`無効な文字列を拒否する: ${JSON.stringify(item.input)}: ${item.why}`, () => {
        expect(formatValues(item.input)).toBeNull();
      });
      continue;
    }
    it(`Go と同じ行を書く: ${JSON.stringify(item.input)}: ${item.why}`, () => {
      expect(formatValues(item.input)).toBe(item.output);
    });
  }

  for (const item of corpus.parse) {
    it(`Go と同じ値を読む: ${JSON.stringify(item.input)}: ${item.why}`, () => {
      if (item.refused) {
        expect(() => parseValues(item.input)).toThrow();
        return;
      }
      expect(parseValues(item.input)).toEqual(item.values ?? []);
    });
  }
});

describe("行の残りを値にするキーワード", () => {
  it("ProxyCommand の値は引用し直さずに書かれたとおりに見せる", () => {
    expect(formatDirectiveValues("ProxyCommand", ["sh -c 'exec nc $(cat /tmp/h) %p'"])).toBe("sh -c 'exec nc $(cat /tmp/h) %p'");
  });

  it("RemoteCommand の編集は引数に分けず、前後の空白だけを落としてひとつの値で送る", () => {
    expect(parseDirectiveValues("remotecommand", "  awk '{print $1}' /etc/passwd # it's\t")).toEqual(["awk '{print $1}' /etc/passwd # it's"]);
    expect(parseDirectiveValues("RemoteCommand", "  ")).toEqual([]);
  });

  it("閉じない引用は、行末のコメントの外にあるときだけ断る", () => {
    expect(() => parseDirectiveValues("ProxyCommand", "nc 'oops %h %p")).toThrow("unbalanced_quote");
    expect(parseDirectiveValues("ProxyCommand", "nc %h %p # it's")).toEqual(["nc %h %p # it's"]);
  });

  it("ほかのキーワードは argv_split の規則で引用する", () => {
    expect(formatDirectiveValues("SetEnv", ["GREETING=hello world"])).toBe('"GREETING=hello world"');
    expect(parseDirectiveValues("SetEnv", "'GREETING=hello world'")).toEqual(["GREETING=hello world"]);
  });
});
