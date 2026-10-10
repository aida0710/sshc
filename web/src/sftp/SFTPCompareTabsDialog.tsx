import { useId, useRef, useState } from "react";
import { useTranslate } from "../i18n/context";
import { control, hintText } from "../ui/form";
import { ModalShell } from "../ui/ModalShell";
import { Button } from "../ui/surface";
import { localHostAlias } from "./localHost";
import type { SFTPLocation } from "./sftpLocation";

type ComparisonTab = SFTPLocation & { id: string };

export function SFTPCompareTabsDialog({ tabs, currentTabId, onCompare, onDismiss }: {
  tabs: ComparisonTab[];
  currentTabId: string;
  onCompare: (left: SFTPLocation, right: SFTPLocation) => void;
  onDismiss: () => void;
}) {
  const t = useTranslate();
  const id = useId();
  const initialFocus = useRef<HTMLSelectElement>(null);
  const availableTabs = tabs.filter((tab) => tab.alias !== "");
  const initialTabId = availableTabs.find((tab) => tab.id === currentTabId)?.id ?? availableTabs[0]?.id ?? "";
  const [firstTabId, setFirstTabId] = useState(initialTabId);
  const [secondTabId, setSecondTabId] = useState(() => availableTabs.find((tab) => tab.id !== initialTabId)?.id ?? "");
  const firstTab = availableTabs.find((tab) => tab.id === firstTabId);
  const secondTab = availableTabs.find((tab) => tab.id === secondTabId);
  const canCompare = firstTab !== undefined && secondTab !== undefined && firstTab.id !== secondTab.id;
  const tabLabel = (tab: ComparisonTab) => `${tab.alias === localHostAlias ? t("sftp.local.connection") : tab.alias}:${tab.path}`;

  function compare() {
    if (!canCompare) return;
    onCompare({ alias: firstTab.alias, path: firstTab.path }, { alias: secondTab.alias, path: secondTab.path });
  }

  return <ModalShell labelledBy={id} describedBy={`${id}-hint`} onDismiss={onDismiss} initialFocusRef={initialFocus} panelClassName="w-full max-w-md rounded-lg p-5">
    <h2 id={id} className="text-base font-semibold">{t("sftp.compare.chooseTabsHeading")}</h2>
    <p id={`${id}-hint`} className={`mt-2 ${hintText}`}>{t("sftp.compare.chooseTabsHint")}</p>
    <form className="mt-4 space-y-4" onSubmit={(event) => { event.preventDefault(); compare(); }}>
      <label className="flex flex-col gap-1 text-sm">
        {t("sftp.compare.firstTab")}
        <select ref={initialFocus} className={`${control} min-h-12 text-base`} value={firstTabId} disabled={availableTabs.length === 0} onChange={(event) => setFirstTabId(event.target.value)}>
          {availableTabs.map((tab) => <option key={tab.id} value={tab.id} disabled={tab.id === secondTabId}>{tabLabel(tab)}</option>)}
        </select>
      </label>
      <label className="flex flex-col gap-1 text-sm">
        {t("sftp.compare.secondTab")}
        <select className={`${control} min-h-12 text-base`} value={secondTabId} disabled={availableTabs.length < 2} onChange={(event) => setSecondTabId(event.target.value)}>
          {availableTabs.map((tab) => <option key={tab.id} value={tab.id} disabled={tab.id === firstTabId}>{tabLabel(tab)}</option>)}
        </select>
      </label>
      <div className="flex justify-end gap-2">
        <Button className="min-h-12 min-w-12" onClick={onDismiss}>{t("sftp.cancel")}</Button>
        <Button kind="primary" type="submit" className="min-h-12 min-w-12" disabled={!canCompare}>{t("sftp.compare.action")}</Button>
      </div>
    </form>
  </ModalShell>;
}
