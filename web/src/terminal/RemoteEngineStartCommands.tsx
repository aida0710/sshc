import { useTranslate } from "../i18n/context";
import { CommandLine } from "../ui/CommandLine";

// RemoteEngineStartCommands は、接続先で sshc エンジンが動いていないときに、接続先で打つコマンドを並べる。
// コマンドは CLI の sshc service install と sshc open のまま。
export function RemoteEngineStartCommands({ className = "" }: { className?: string }) {
  const t = useTranslate();
  return (
    <div className={`flex flex-col gap-1.5 ${className}`}>
      <p className="text-xs leading-5 text-ink-muted">{t("runTarget.startHint")}</p>
      <CommandLine command="sshc service install" note={t("runTarget.startAsService")} />
      <CommandLine command="sshc open" note={t("runTarget.startOnce")} />
    </div>
  );
}
