// duplicateAliasOf は、複製した接続先に付ける名前を返す。同じ名前がすでにあれば sshcエンジンが断る。
export function duplicateAliasOf(alias: string): string {
  return `${alias}-copy`;
}
