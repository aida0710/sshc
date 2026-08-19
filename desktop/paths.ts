import { join } from "node:path";

// **__dirname はもう外殻の根ではない。**
//
// TypeScript の出力は out/ に入るので、走っているのは `<root>/out/*.js` である。
// 一方 build/ の図も package.json も、その一つ上に置かれたままで、束の中でも
// 同じ相対配置で運ばれる（electron-builder は files に並べた道をそのまま写す）。
//
// ここを通さずに `join(__dirname, "build", ...)` と書くと、**開発中は動くのに
// 束にすると図だけが出ない**という壊れ方をする。図が無いことはアプリが起動
// しない理由にはならないので、誰も気付かないまま出荷されうる。だから場所を
// 知っているのはこの 1 ファイルだけにして、paths.test.ts が実在を確かめる。
export const bundleRoot = join(__dirname, "..");

/** resource は、外殻に同梱された資材の絶対パスを返す。 */
export function resource(...segments: string[]): string {
  return join(bundleRoot, ...segments);
}

/**
 * repositoryBinary は、開発中に使うリポジトリの bin/sshc を返す。
 *
 * **束の中には無い。** 束では resources の側にあるものを使う——ここが返すのは
 * checkout で `make build` した実体であり、根から二つ上に居る。
 */
export function repositoryBinary(name = "sshc"): string {
  return join(bundleRoot, "..", "bin", name);
}
