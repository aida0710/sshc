import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";

export function SFTPNavigationControls({ busy, canBack, canForward, canHome, canRoot, onBack, onForward, onHome, onRoot }: {
  busy: boolean;
  canBack: boolean;
  canForward: boolean;
  canHome: boolean;
  canRoot: boolean;
  onBack: () => void;
  onForward: () => void;
  onHome: () => void;
  onRoot: () => void;
}) {
  const t = useTranslate();
  return <div role="group" aria-label={t("sftp.navigation")} className="flex shrink-0 overflow-hidden rounded-md bg-toolbar/70">
    <button type="button" aria-label={t("sftp.back")} disabled={busy || !canBack} onClick={onBack} className="flex size-9 items-center justify-center text-ink-muted hover:bg-hover disabled:text-ink-faint md:size-8"><span aria-hidden="true">←</span></button>
    <button type="button" aria-label={t("sftp.forward")} disabled={busy || !canForward} onClick={onForward} className="flex size-9 items-center justify-center text-ink-muted hover:bg-hover disabled:text-ink-faint md:size-8"><span aria-hidden="true">→</span></button>
    <button type="button" aria-label={t("sftp.homeDirectory")} disabled={busy || !canHome} onClick={onHome} className="flex size-9 items-center justify-center text-ink-muted hover:bg-hover disabled:text-ink-faint md:size-8"><Icon name="home" className="size-4" /></button>
    <button type="button" aria-label={t("sftp.rootDirectory")} disabled={busy || !canRoot} onClick={onRoot} className="flex size-9 items-center justify-center font-mono text-sm text-ink-muted hover:bg-hover disabled:text-ink-faint md:size-8">/</button>
  </div>;
}
