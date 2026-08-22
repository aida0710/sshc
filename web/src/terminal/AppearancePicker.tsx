import { control } from "../ui/form";

// 見た目をひとつ選ぶ。**全体の設定画面と接続の画面が、配色にも字体にも
// 同じものを使う。**
//
// 4 つ書くと、選べる場所と選べない場所ができる——しかもそれは、その画面を
// 開くまで分からない。
type Choice = { readonly name: string; readonly label: string };

type AppearancePickerProps = {
  choices: readonly Choice[];
  value: string;
  onChange: (next: string) => void;
  /**
   * 何も選ばなかったときに何が起きるかを言う綴り。
   *
   * <p>**「なし」では足りない。** 全体の画面では「テーマに従う」であり、接続の
   * 画面では「全体の設定に従う」である——同じ空欄が別のことを意味する。
   */
  unchosen: string;
};

export function AppearancePicker({ choices, value, onChange, unchosen }: AppearancePickerProps) {
  return (
    <select className={control} value={value} onChange={(event) => onChange(event.target.value)}>
      <option value="">{unchosen}</option>
      {choices.map((choice) => (
        <option key={choice.name} value={choice.name}>
          {choice.label}
        </option>
      ))}
    </select>
  );
}
