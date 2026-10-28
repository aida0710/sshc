import type { Overview } from "../api/config";
import { groupAncestry } from "../rules/groupPath";

// groupDefaultValue は、グループの接続先の既定値（groups.sshc.conf に書く設定）のうち、keyword の値を返す。
// 子のグループに無ければ親のグループの値が効くので、近い順にたどる。
export function groupDefaultValue(groups: Overview["groups"], group: string, keyword: string): string {
  const wanted = keyword.toLowerCase();
  for (const name of groupAncestry(group)) {
    const setting = groups.find((candidate) => candidate.name === name)?.settings
      ?.find((candidate) => candidate.keyword.toLowerCase() === wanted);
    if (setting !== undefined) return setting.values.join(" ");
  }
  return "";
}
