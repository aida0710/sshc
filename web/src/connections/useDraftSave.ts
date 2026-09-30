import { useState } from "react";

type SaveOutcome = {
  // draft と saved は、保存を始めたときの下書きと保存済みの値（どちらも参照）である。
  draft: unknown;
  saved: unknown;
  result: "written" | "failed";
};

// useDraftSave は、接続エディタのタブが下書きを保存するあいだと、保存した結果を持つ。
//
// 書き込みが成功した下書きは、その時点で written になり、タブは未保存の変更に数えない。
// タブが下書きと比べる保存済みの値（HostDetail）は、ページが engine から読み直すまで
// 古いままなので、比べるだけでは、読み直しが終わるまで未保存の変更が残って見え、
// そのあいだに別の接続を選ぶと破棄の確認が出てしまう。
//
// 結果は、保存を始めたときの下書きと保存済みの値が、どちらも同じ参照のあいだだけ当てはまる。
// 下書きを編集・破棄するか、保存済みの値を読み直すと、結果は外れる。読み直した値に保存した
// 変更が無ければ、下書きはふたたび未保存の変更として見える。
export function useDraftSave({ draft, saved }: { draft: unknown; saved: unknown }) {
  const [saving, setSaving] = useState(false);
  const [outcome, setOutcome] = useState<SaveOutcome | null>(null);
  const result = outcome !== null && outcome.draft === draft && outcome.saved === saved ? outcome.result : null;

  // save は、write（保存できなかったときは reject する）で下書きを書き込み、書き込めたかを返す。
  async function save(write: () => Promise<void>): Promise<boolean> {
    const started = { draft, saved };
    setSaving(true);
    setOutcome(null);
    try {
      await write();
      setOutcome({ ...started, result: "written" });
      return true;
    } catch {
      setOutcome({ ...started, result: "failed" });
      return false;
    } finally {
      setSaving(false);
    }
  }

  return { saving, written: result === "written", saveFailed: result === "failed", save };
}
