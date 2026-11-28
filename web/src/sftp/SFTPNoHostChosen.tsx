import { useId } from "react";
import type { HostEntry } from "../api/config";
import { useTranslate } from "../i18n/context";
import type { HostLook } from "../shell/useDeclaredConfig";
import { Button } from "../ui/surface";
import { SFTPQuickHosts } from "./SFTPQuickHosts";

// SFTPNoHostChosen は、接続先を選んでいないペイン。見出しの行の右に［接続先を選択］を置き、その下に、選ばずに
// すぐ開ける接続先（SFTPQuickHosts）を Home と同じカードで並べる。見出しは、画面の見出し（h2）の下なので h3 にする。
export function SFTPNoHostChosen({ aliases, hosts, hostLook, onChooseHost, onChoose }: {
  aliases: readonly string[];
  hosts: readonly HostEntry[];
  hostLook: ReadonlyMap<string, HostLook>;
  // onChooseHost は、接続先の選択を開く。無ければ［接続先を選択］を出さない。
  onChooseHost: (() => void) | undefined;
  onChoose: (alias: string) => void;
}) {
  const t = useTranslate();
  const headingId = useId();
  return (
    <section aria-labelledby={headingId} className="m-auto flex w-[min(100%,780px)] flex-col gap-4 p-6 mobile-layout:px-0 mobile-layout:py-2">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h3 id={headingId} className="text-lg font-semibold text-ink">{t("sftp.noHostChosen")}</h3>
        {onChooseHost === undefined ? null : <Button kind="primary" onClick={onChooseHost}>{t("sftp.chooseHostAction")}</Button>}
      </div>
      <SFTPQuickHosts aliases={aliases} hosts={hosts} hostLook={hostLook} onChoose={onChoose} />
    </section>
  );
}
