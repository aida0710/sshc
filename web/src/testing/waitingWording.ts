import { ja } from "../i18n/messages/ja";

// 待っている間を言う文言は「〜中…」「〜しています…」で終わる。「sshcエンジンを追加…」のように、押すと続きの
// 画面が開くことを言う「…」は入れない。
const waitingWording = /(中|ています)…$/;

// waitingKeys は、ソースの断片に書かれた文言のキーのうち、待っている間を言うものを返す。
export function waitingKeys(source: string): string[] {
  return [...source.matchAll(/"([A-Za-z][\w.]*)"/g)]
    .map((match) => match[1]!)
    .filter((key) => key in ja && waitingWording.test(ja[key as keyof typeof ja]));
}
