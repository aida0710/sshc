import { KeyConfig } from "../keyconfig/KeyConfig";
import { settingsApi, type SettingsApi, type TerminalSettings } from "../api/settings";
import { vaultApi, type PasswordVaultStatus, type VaultApi } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { SettingsNavigation } from "./SettingsNavigation";
import { AppearanceSettingsSection } from "./AppearanceSettingsSection";
import { SessionSettingsSection, type SessionSettingsState } from "./SessionSettingsSection";
import { SFTPSettingsSection } from "./SFTPSettingsSection";
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
  // Whether the vault has no master password; the engine never locks one.
  passwordless?: boolean;
  api?: SettingsPanelApi;
  page?: SettingsPage | "All";
  onTerminalSettingsChange?: (settings: TerminalSettings) => void | Promise<void>;
  terminalSessions?: SessionSettingsState;
  // onNavigatePage moves to another settings page from the list beside the card.
  onNavigatePage?: (path: string) => void;
};

// The settings page is a list of independent sections; each owns its own
// form, load and save. This component only decides which of them to show.
export function SettingsPanel({
  api = settingsPanelApi,
  page = "All",
  terminalSessions,
  onVaultChanged,
  passwordless = false,
  onTerminalSettingsChange,
  onNavigatePage,
}: SettingsPanelProps) {
  const t = useTranslate();
  const all = page === "All";
  const shows = (candidate: SettingsPage) => all || page === candidate;
  const pageTitle = all ? "settings.heading" : settingsPageMeta[page].label;
  const pageDescription = all ? "settings.pageDescription" : settingsPageMeta[page].description;

  // The list of pages on the left and the open page as one card, as in the design.
  return (
    <div className="mx-auto flex min-h-full w-full max-w-6xl gap-2">
      {onNavigatePage === undefined ? null : <SettingsNavigation current={all ? null : page} onNavigate={onNavigatePage} />}
      <div className={`min-w-0 flex-1 overflow-hidden rounded-xl border border-line bg-sidebar shadow-pane ${mobileTouchTargets}`}>
        <header className="px-4 pt-6 sm:px-6 lg:px-8 lg:pt-7">
          <h2 className="text-xl font-semibold tracking-tight text-ink">{t(pageTitle)}</h2>
          {pageDescription === undefined ? null : <p className="mt-1 text-xs leading-5 text-ink-faint">{t(pageDescription)}</p>}
        </header>
        {shows("Appearance") ? <AppearanceSettingsSection showHeading={all} /> : null}
        {shows("Shortcuts") ? (
          <SettingsSection id="settings-shortcuts" label={t("shortcuts.heading")} icon="settings" showHeading={all}>
            <KeyConfig />
          </SettingsSection>
        ) : null}
        {shows("Engine") ? <EngineSettingsSection api={api} showHeading={all} passwordless={passwordless} /> : null}
        {shows("Terminal") ? <TerminalSettingsSection api={api} showHeading={all} onSettingsChange={onTerminalSettingsChange} /> : null}
        {shows("SFTP") ? <SFTPSettingsSection showHeading={all} /> : null}
        {shows("Notifications") ? <NotificationSettingsSection showHeading={all} /> : null}
        {terminalSessions !== undefined && shows("Connections") ? <SessionSettingsSection terminalSessions={terminalSessions} showHeading={all} /> : null}
        {shows("Password") ? <MasterPasswordSection api={api} showHeading={all} onVaultChanged={onVaultChanged} /> : null}
      </div>
    </div>
  );
}
