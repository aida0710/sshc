import { DraftSaveBar } from "../ui/DraftSaveBar";
import { useSettingsDraft } from "./settingsDraftContext";

// SettingsSaveBar は、設定のページに変更があるあいだ、保存するか破棄するかを選ぶ帯を出す。
export function SettingsSaveBar() {
  const draft = useSettingsDraft();
  if (!draft.dirty && !draft.saving) return null;
  return (
    <DraftSaveBar
      saving={draft.saving}
      inputInvalid={draft.inputInvalid}
      note={draft.problemOutOfSight}
      failure={draft.failure}
      onSave={draft.save}
      onDiscard={draft.discard}
    />
  );
}
