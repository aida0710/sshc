import { useState, type KeyboardEvent } from "react";
import { KeyConfig } from "../keyconfig/KeyConfig";
import { KeyBarConfig } from "../keybar/KeyBarConfig";
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
import { settingsPageMeta, type SettingsGroup, type SettingsPage } from "./settingsRoute";
import { standardPresetId, useSettings } from "./settingsStore";
import { PageHeader } from "../ui/page";
import { PageBoard } from "../ui/PageBoard";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { SettingScopeTag, type SettingScope } from "./SettingScopeTag";
import { SettingsDraftProvider } from "./SettingsDraftProvider";
import { useSettingsDraft } from "./settingsDraftContext";
import { SettingsSaveBar } from "./SettingsSaveBar";
import type { NavigateLocationOptions, NavigationBlocker } from "../routing/useSectionRoute";
import { isImeComposing } from "../ui/imeComposition";

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
  // 保存していない変更があるあいだ、ほかの画面へ移る前に確かめるために使う。
  onNavigationBlockerChange?: ((blocker: NavigationBlocker | null) => void) | undefined;
  onNavigateLocation?: ((url: string, options?: NavigateLocationOptions) => void) | undefined;
};

// pageScopes is how far the settings of a page reach, by its group. A part of a page that reaches elsewhere
// (the master password on the Vault page) says so on its own heading.
const pageScopes: Record<SettingsGroup, SettingScope> = {
  global: "allMachines",
  preset: "preset",
  machines: "eachMachine",
  actions: "thisMachine",
};

// The settings page is a list of independent sections. What is changed in any of them waits in one draft
// (SettingsDraftProvider) until Save in the bar at the bottom. This component decides which sections to show, and
// which preset the preset pages edit: the one chosen above the list of pages (or, on a phone, above the page), at
// first the one this machine uses.
export function SettingsPanel({ onNavigationBlockerChange, onNavigateLocation, ...props }: SettingsPanelProps) {
  return (
    <SettingsDraftProvider onNavigationBlockerChange={onNavigationBlockerChange} onNavigateLocation={onNavigateLocation}>
      <SettingsPages {...props} />
    </SettingsDraftProvider>
  );
}

// savesOnEnter says whether Enter in this field saves the page: the one-line fields. In a box of lines Enter starts a
// line, and on a button or a list it is that control's own.
function savesOnEnter(field: EventTarget): boolean {
  return field instanceof HTMLInputElement && ["text", "number"].includes(field.type);
}

function SettingsPages({
  api = settingsPanelApi,
  page = "All",
  terminalSessions,
  onVaultChanged,
  passwordless = false,
  onNavigatePage,
}: Omit<SettingsPanelProps, "onNavigationBlockerChange" | "onNavigateLocation">) {
  const t = useTranslate();
  const draft = useSettingsDraft();
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

  // A dialog opened from the page has its own fields and Enter; its events reach here through React's tree only.
  function saveOnEnter(event: KeyboardEvent) {
    if (event.key !== "Enter" || isImeComposing(event) || !savesOnEnter(event.target)) return;
    if (!(event.target instanceof Node) || !event.currentTarget.contains(event.target)) return;
    draft.save();
  }

  // Another preset's settings are another draft.
  const editPreset = (id: string) => draft.confirmBeforeDiscarding(() => setChosenPreset(id));

  // The list of pages on the left and the open page as one card, as in the design.
  return (
    <div className="contents" onKeyDown={saveOnEnter}>
    <PageBoard
      saveBar
      aside={onNavigatePage === undefined ? null : (
        <SettingsNavigation current={all ? null : page} onNavigate={onNavigatePage} editingPreset={editingPreset} onEditPreset={editPreset} />
      )}
      className={mobileTouchTargets}
    >
      <PageHeader
        title={t(pageTitle)}
        meta={all ? undefined : (
          // A long preset name is cut short inside this frame rather than widening the page.
          <span className="inline-flex min-w-0 max-w-full items-center">
            <SettingScopeTag scope={pageScopes[settingsPageMeta[page].group]} presetId={editingPreset} />
          </span>
        )}
        description={pageDescription === undefined ? undefined : t(pageDescription)} titleOnPhone />
      {presetPickerOnPage ? <PresetPicker layout="bar" editing={editingPreset} onEdit={editPreset} /> : null}
      {shows("General") ? <GeneralSettingsSection showHeading={all} /> : null}
      {shows("Appearance") ? <AppearanceSettingsSection presetId={editingPreset} showHeading={all} /> : null}
      {shows("Terminal") ? <TerminalSettingsSection api={api} presetId={editingPreset} showHeading={all} /> : null}
      {shows("SFTP") ? <SFTPSettingsSection presetId={editingPreset} showHeading={all} /> : null}
      {shows("Shortcuts") ? (
        <SettingsSection id="settings-shortcuts" label={t("shortcuts.heading")} icon="settings" showHeading={all}>
          <KeyConfig presetId={editingPreset} />
        </SettingsSection>
      ) : null}
      {shows("KeyBar") ? (
        <SettingsSection id="settings-key-bar" label={t("keyBar.heading")} icon="settings" showHeading={all}>
          <KeyBarConfig presetId={editingPreset} />
        </SettingsSection>
      ) : null}
      {shows("Notifications") ? <NotificationSettingsSection presetId={editingPreset} showHeading={all} /> : null}
      {shows("Vault") ? (
        <SettingsSection id="settings-vault" label={t("settings.vaultHeading")} icon="secrets" showHeading={all}>
          <div className="flex flex-col gap-5">
            <VaultAutoLockSection presetId={editingPreset} passwordless={passwordless} />
            <MasterPasswordSection api={api} onVaultChanged={onVaultChanged} />
          </div>
        </SettingsSection>
      ) : null}
      {shows("Machines") ? <MachinesSettingsSection showHeading={all} /> : null}
      {terminalSessions !== undefined && shows("Connections") ? <SessionSettingsSection terminalSessions={terminalSessions} showHeading={all} /> : null}
      <SettingsSaveBar />
    </PageBoard>
    </div>
  );
}
