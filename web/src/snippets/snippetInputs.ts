// withSnippetInput は、変数 name の欄を value にしたときに、エンジンへ送る次の inputs を返す。
// エンジンは inputs にキーがあれば既定値を使わない（internal/snippets/expand.go の variableValue）。
// 欄を空にした変数はキーごと消し、まだ入力していないときと同じく既定値を使わせる（欄の「既定値を使用」の表示のとおり）。
export function withSnippetInput(
  inputs: Record<string, string>,
  name: string,
  value: string,
): Record<string, string> {
  if (value !== "") return { ...inputs, [name]: value };
  const { [name]: _cleared, ...rest } = inputs;
  return rest;
}
