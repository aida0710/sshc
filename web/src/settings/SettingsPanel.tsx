import { useState } from "react";
import { KeyConfig } from "../keyconfig/KeyConfig";
import { settingsApi, type SettingsApi } from "../api/settings";
import { vaultApi, type PasswordVaultStatus, type VaultApi } from "../api/vault";
import { useTranslate } from "../i18n/context";
import { SettingsNavigation } from "./SettingsNavigation";
import { AppearanceSettingsSection } from "./AppearanceSettingsSection";
import { SessionSettingsSection, type SessionSettingsState } from "./SessionSettingsSection";
import { SFTPSettingsSection } from "./SFTPSettingsSection";
import { GeneralSettingsSection } from "./GeneralSettingsSection";
import { MachinesSettingsSection } from "./MachinesSettingsSection";
import { MasterPasswordSection } from "./MasterPasswordSection";
import { NotificationSettingsSection } from "./NotificationSettingsSection";
import { SettingsSection } from "./SettingsSection";
import { TerminalSettingsSection } from "./TerminalSettingsSection";
import { VaultAutoLockSection } from "./VaultAutoLockSection";
import { PresetPicker } from "./PresetPicker";
import { settingsPageMeta, type SettingsPage } from "./settingsRoute";
import { standardPresetId, useSettings } from "./settingsStore";
import { PageHeader } from "../ui/page";
import { PageBoard } from "../ui/PageBoard";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { SettingsSavedMark } from "./SettingsSavedMark";

// Settings also change the master password, which belongs to the vault.
export type SettingsPanelApi = SettingsApi & Pick<VaultApi, "passwordVault" | "changeMasterPassword">;
const settingsPanelApi: SettingsPanelApi = { ...settingsApi, ...vaultApi };

const mobileTouchTargets =
  "[&_button]:min-h-10 [&_a]:inline-flex [&_a]:min-h-10 [&_a]:items-center " +
  "md:[&_button]:min-h-0 md:[&_a]:min-h-0";

type SettingsPanelProps = {
  onVaultChanged?: ((status: PasswordVaultStatus) => void) | undefined;
  // Whether the vault has no master password; the engine never locks one.
  passwordless?: boolean;
  api?: SettingsPanelApi;
  page?: SettingsPage | "All";
  terminalSessions?: SessionSettingsState;
  // onNavigatePage moves to another settings page from the list beside the card.
  onNavigatePage?: (path: string) => void;
};

// The settings page is a list of independent sections; each owns its own
// form, load and save. This component decides which of them to show, and
// which preset the preset pages edit: the one chosen above the list of pages
// (or, on a phone, above the page), at first the one this machine uses.
export function SettingsPanel({
  api = settingsPanelApi,
  page = "All",
  terminalSessions,
  onVaultChanged,
  passwordless = false,
  onNavigatePage,
}: SettingsPanelProps) {
  const t = useTranslate();
  const compact = useMediaQuery(mobileViewportQuery);
  const settingsDocument = useSettings().document;
  // chosenPreset is the preset someone picked to edit; until then the preset pages edit the one this machine
  // uses. A preset deleted elsewhere falls back the same way.
  const [chosenPreset, setChosenPreset] = useState<string | null>(null);
  const presetInUse = settingsDocument?.presetInUse ?? standardPresetId;
  const editingPreset = chosenPreset !== null && settingsDocument?.presets.some((preset) => preset.id === chosenPreset)
    ? chosenPreset
    : presetInUse;
  const all = page === "All";
  const shows = (candidate: SettingsPage) => all || page === candidate;
  const pageTitle = all ? "settings.heading" : settingsPageMeta[page].label;
  const pageDescription = all ? undefined : settingsPageMeta[page].description;
  // On a phone the list of pages is not shown, so a preset page carries the picker itself.
  const presetPickerOnPage = (compact || onNavigatePage === undefined) && (all || settingsPageMeta[page].group === "preset");

  // The list of pages on the left and the open page as one card, as in the design.
  return (
    <PageBoard
      aside={onNavigatePage === undefined ? null : (
        <SettingsNavigation current={all ? null : page} onNavigate={onNavigatePage} editingPreset={editingPreset} onEditPreset={setChosenPreset} />
      )}
      className={mobileTouchTargets}
    >
      <PageHeader title={t(pageTitle)} meta={<SettingsSavedMark />} description={pageDescription === undefined ? undefined : t(pageDescription)} titleOnPhone />
      {presetPickerOnPage ? <PresetPicker layout="bar" editing={editingPreset} onEdit={setChosenPreset} /> : null}
      {shows("General") ? <GeneralSettingsSection showHeading={all} /> : null}
      {shows("Appearance") ? <AppearanceSettingsSection presetId={editingPreset} showHeading={all} /> : null}
      {shows("Terminal") ? <TerminalSettingsSection api={api} presetId={editingPreset} showHeading={all} /> : null}
      {shows("SFTP") ? <SFTPSettingsSection presetId={editingPreset} showHeading={all} /> : null}
      {shows("Shortcuts") ? (
        <SettingsSection id="settings-shortcuts" label={t("shortcuts.heading")} icon="settings" showHeading={all}>
          <KeyConfig presetId={editingPreset} />
        </SettingsSection>
      ) : null}
      {shows("Notifications") ? <NotificationSettingsSection presetId={editingPreset} showHeading={all} /> : null}
      {shows("Vault") ? (
        <>
          <VaultAutoLockSection presetId={editingPreset} showHeading={all} passwordless={passwordless} />
          <MasterPasswordSection api={api} showHeading onVaultChanged={onVaultChanged} />
        </>
      ) : null}
      {shows("Machines") ? <MachinesSettingsSection showHeading={all} /> : null}
      {terminalSessions !== undefined && shows("Connections") ? <SessionSettingsSection terminalSessions={terminalSessions} showHeading={all} /> : null}
    </PageBoard>
  );
}
