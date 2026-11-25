import { useTranslate } from "../i18n/context";
import { connectionLocation } from "../routing/connectionRoute";
import { ActionMenu } from "../ui/ActionMenu";

type ConnectionActionsProps = {
  alias: string;
  path: string;
  busy: boolean;
  onOpenSettings: (location: string) => void;
  onConnect: () => void;
};

export function ConnectionActions({
  alias,
  path,
  busy,
  onOpenSettings,
  onConnect,
}: ConnectionActionsProps) {
  const t = useTranslate();
  const settingsLocation = connectionLocation({ path, alias, panel: "Basic", advanced: "Settings" });

  return (
    <ActionMenu
      label={t("home.connectionActions", { alias })}
      triggerSize="panel"
      triggerClassName="pointer-events-auto"
      items={[
        { label: t("home.openConnectionSettings"), onSelect: () => onOpenSettings(settingsLocation) },
        { label: t("home.connect"), onSelect: onConnect, disabled: busy },
      ]}
    />
  );
}
