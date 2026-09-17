import { KeyConfig } from "../keyconfig/KeyConfig";
import { settingsApi, type SettingsApi, type TerminalSettings } from "../api/settings";
import { vaultApi, type PasswordVaultStatus, type VaultApi } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { PageHeader } from "../ui/page";
import { Card } from "../ui/surface";
import { ConsoleSettingsSection, type ConsoleSettingsConsoles } from "./ConsoleSettingsSection";
import { EngineSettingsSection } from "./EngineSettingsSection";
import { MasterPasswordSection } from "./MasterPasswordSection";
import { NotificationSettingsSection } from "./NotificationSettingsSection";
import { SettingsSection } from "./SettingsSection";
import { TerminalSettingsSection } from "./TerminalSettingsSection";
import { settingsPageMeta, type SettingsPage } from "./settingsRoute";

// Settings also change the master password, which belongs to the vault.
export type SettingsPanelApi = SettingsApi & Pick<VaultApi, "passwordVault" | "changeMasterPassword">;
export const settingsPanelApi: SettingsPanelApi = { ...settingsApi, ...vaultApi };

const mobileTouchTargets =
  "[&_button]:min-h-10 [&_a]:inline-flex [&_a]:min-h-10 [&_a]:items-center " +
  "md:[&_button]:min-h-0 md:[&_a]:min-h-0";

type SettingsPanelProps = {
  onVaultChanged?: ((status: PasswordVaultStatus) => void) | undefined;
  api?: SettingsPanelApi;
  page?: SettingsPage | "All";
  onTerminalSettingsChange?: (settings: TerminalSettings) => void | Promise<void>;
  consoles?: ConsoleSettingsConsoles;
};

// The settings page is a list of independent sections; each owns its own
// form, load and save. This component only decides which of them to show.
export function SettingsPanel({
  api = settingsPanelApi,
  page = "All",
  consoles,
  onVaultChanged,
  onTerminalSettingsChange,
}: SettingsPanelProps) {
  const t = useTranslate();
  const all = page === "All";
  const shows = (candidate: SettingsPage) => all || page === candidate;
  const pageTitle = all ? "settings.heading" : settingsPageMeta[page].label;
  const pageDescription = all ? "settings.pageDescription" : settingsPageMeta[page].description;

  return (
    <div className={`mx-auto flex w-full max-w-6xl flex-col gap-6 ${mobileTouchTargets}`}>
      <PageHeader title={t(pageTitle)} description={t(pageDescription)} />

      <Card radius="md">
        {shows("Shortcuts") ? (
          <SettingsSection id="settings-shortcuts" label={t("shortcuts.heading")} icon="settings" showHeading={all}>
            <KeyConfig />
          </SettingsSection>
        ) : null}
        {shows("Engine") ? <EngineSettingsSection api={api} showHeading={all} /> : null}
        {shows("Terminal") ? <TerminalSettingsSection api={api} showHeading={all} onSettingsChange={onTerminalSettingsChange} /> : null}
        {shows("Notifications") ? <NotificationSettingsSection showHeading={all} /> : null}
        {consoles !== undefined && shows("Connections") ? <ConsoleSettingsSection consoles={consoles} showHeading={all} /> : null}
        {shows("Password") ? <MasterPasswordSection api={api} showHeading={all} onVaultChanged={onVaultChanged} /> : null}
      </Card>
    </div>
  );
}
