import { control } from "../ui/form";
import { palettes } from "./palettes";

// 配色を選ぶ。**全体の設定画面と接続の画面が同じものを使う。**
//
// 2 つ書くと、配色を 1 つ足した日にどちらか片方にしか出ない——選べる場所と
// 選べない場所ができ、しかもそれは動かしてみるまで分からない。
type PalettePickerProps = {
  value: string;
  onChange: (next: string) => void;
  /**
   * 何も選ばなかったときに何が起きるかを言う綴り。
   *
   * <p>**「なし」では足りない。** 全体の画面では「テーマに従う」であり、接続の
   * 画面では「全体の設定に従う」である——同じ空欄が別のことを意味する。
   */
  unchosen: string;
  id?: string;
};

export function PalettePicker({ value, onChange, unchosen, id }: PalettePickerProps) {
  return (
    <select
      className={control}
      {...(id === undefined ? {} : { id })}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    >
      <option value="">{unchosen}</option>
      {palettes.map((palette) => (
        <option key={palette.name} value={palette.name}>
          {palette.label}
        </option>
      ))}
    </select>
  );
}
