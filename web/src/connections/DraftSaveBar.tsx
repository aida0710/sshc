import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { Button } from "../ui/surface";

type DraftSaveBarProps = {
  saveLabel: string;
  saveDisabled: boolean;
  discardDisabled: boolean;
  onDiscard: () => void;
  // onSave を渡さないときは、保存ボタンが囲んでいる form を送信する。
  onSave?: () => void;
  // saving のあいだは、保存ボタンの文言を保存中の表示に変える。
  saving?: boolean;
  // note は、保存できない理由など、ボタンの前に添える補足である。
  note?: string;
};

// DraftSaveBar は、接続エディタのタブで、下書きを保存するか破棄するかを選ぶ行である。
// 下書きに変更があるときだけ置く。
export function DraftSaveBar({
  saveLabel,
  saveDisabled,
  discardDisabled,
  onDiscard,
  onSave,
  saving = false,
  note = "",
}: DraftSaveBarProps) {
  const t = useTranslate();
  return (
    <div className="flex flex-wrap items-center justify-end gap-3 border-t border-line py-3">
      {note === "" ? <span className="grow" /> : <p className={`grow ${hintText}`}>{note}</p>}
      <Button disabled={discardDisabled} onClick={onDiscard}>
        {t("conn.discardChanges")}
      </Button>
      <Button type={onSave === undefined ? "submit" : "button"} kind="primary" disabled={saveDisabled} onClick={onSave}>
        {saving ? t("conn.saving") : saveLabel}
      </Button>
    </div>
  );
}
