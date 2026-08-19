import { describe, expect, it } from "vitest";
import corpus from "./corpus.generated.json";
import { formatValues, isValidAlias, isValidGroupName, isValidHostName, parseValues } from "./rules";

// コーパスは internal/validate/cmd/rulegen が書き出す。入力の一覧と、それらに対する
// **Go の判定**を持つ。
//
// **守る契約は「画面はサーバーより厳しくしない」である。**
//
// 緩い側にずれても壊れない——サーバーが正しく断り、利用者は理由を受け取る。厳しい側
// にずれると**正しい入力が画面で止められ、利用者には直しようがない。** だからこの
// スイートは片側だけを赤にする。
//
// 一度これが無かったせいで、予約語が Go に 10・画面に 6 あった期間、`rc` という
// グループ名は画面が緑を出してサーバーが `invalid_request` だけを返していた。

function neverStricter(
  kind: string,
  cases: readonly { input: string; valid: boolean; why: string }[],
  accepts: (input: string) => boolean,
) {
  describe(`${kind}: サーバーより厳しくしない`, () => {
    for (const item of cases) {
      if (!item.valid) continue;
      it(`受け入れる: ${JSON.stringify(item.input)} — ${item.why}`, () => {
        expect(accepts(item.input)).toBe(true);
      });
    }
  });
}

neverStricter("グループ名", corpus.groupName, isValidGroupName);
neverStricter("ホスト名", corpus.hostName, isValidHostName);
neverStricter("alias", corpus.alias, isValidAlias);

// **緩すぎることも、知っておく価値はある。** 契約違反ではないので落とさないが、
// どれだけ離れているかは見えていた方がよい。ここが増えたら、画面は「どの文字が
// 間違っているか」を言えない場面をそれだけ増やしたということである。
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

// 整形には緩い厳しいの軸が無い。**同じ入力から同じ文字列が出なければ、画面が
// 見せているものと保存されるものが違う。**
describe("ssh_config の綴り", () => {
  for (const item of corpus.render) {
    if (item.refused) {
      it(`綴りが無いものを断る: ${JSON.stringify(item.input)} — ${item.why}`, () => {
        expect(formatValues(item.input)).toBeNull();
      });
      continue;
    }
    it(`Go と同じ行を書く: ${JSON.stringify(item.input)} — ${item.why}`, () => {
      expect(formatValues(item.input)).toBe(item.output);
    });
  }

  for (const item of corpus.parse) {
    it(`Go と同じ値を読む: ${JSON.stringify(item.input)} — ${item.why}`, () => {
      if (item.refused) {
        expect(() => parseValues(item.input)).toThrow();
        return;
      }
      expect(parseValues(item.input)).toEqual(item.values ?? []);
    });
  }
});
