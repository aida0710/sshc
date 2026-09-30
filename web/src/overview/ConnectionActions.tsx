import { useTranslate } from "../i18n/context";
import { connectionLocation } from "../routing/connectionRoute";
import { ActionMenu } from "../ui/ActionMenu";

type ConnectionActionsProps = {
  alias: string;
  path: string;
  busy: boolean;
  opening?: boolean;
  onOpenSettings: (location: string) => void;
  onConnect: () => void;
};

export function ConnectionActions({
  alias,
  path,
  busy,
  opening = false,
  onOpenSettings,
  onConnect,
}: ConnectionActionsProps) {
  const t = useTranslate();
  const settingsLocation = connectionLocation({ path, alias, panel: "Basic", advanced: "Jump" });

  return (
    <ActionMenu
      label={t("home.connectionActions", { alias })}
      triggerClassName="pointer-events-auto flex size-8 items-center justify-center rounded-md text-ink-muted hover:bg-hover hover:text-ink"
      items={[
        { label: t("home.openConnectionSettings"), onSelect: () => onOpenSettings(settingsLocation) },
        { label: opening ? t("home.opening") : t("home.connect"), onSelect: onConnect, disabled: busy },
      ]}
    />
  );
}
