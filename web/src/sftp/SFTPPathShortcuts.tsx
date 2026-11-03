import { useTranslate } from "../i18n/context";
import { Icon, type IconName } from "../ui/icons";
import type { SFTPBrowserModel } from "./useSFTPBrowser";

function Shortcut({ icon, label, disabled, onClick }: { icon: IconName; label: string; disabled: boolean; onClick: () => void }) {
  return (
    <button type="button" disabled={disabled} onClick={onClick}
      className="flex min-h-11 min-w-0 flex-1 items-center justify-center gap-1 rounded-md text-xs text-ink-muted active:bg-select-fill disabled:opacity-40">
      <Icon name={icon} className="size-4 shrink-0" />
      <span className="truncate">{label}</span>
    </button>
  );
}

// The other ways to move on a phone, under the path while it is being edited. A phone's toolbar has room only for the
// way up, and moving somewhere else is what opening the path is for.
export function SFTPPathShortcuts({ browser, disabled, onMove }: { browser: SFTPBrowserModel; disabled: boolean; onMove: () => void }) {
  const t = useTranslate();
  const moveBy = (move: () => Promise<unknown>) => () => {
    onMove();
    void move();
  };
  return (
    <div className="flex shrink-0 items-center gap-1 border-b border-line px-1 py-0.5">
      <Shortcut icon="arrowLeft" label={t("sftp.back")} disabled={disabled || !browser.canBack} onClick={moveBy(browser.back)} />
      <Shortcut icon="arrowRight" label={t("sftp.forward")} disabled={disabled || !browser.canForward} onClick={moveBy(browser.forward)} />
      <Shortcut icon="home" label={t("sftp.goHome")} disabled={disabled} onClick={moveBy(() => browser.load(""))} />
      <Shortcut icon="folder" label={t("sftp.goRoot")} disabled={disabled || browser.atRoot} onClick={moveBy(browser.goRoot)} />
    </div>
  );
}
