import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import type { LocalShellProfile, SettingsApi, TerminalSettings } from "../api/settings";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { ActionArea } from "../ui/ActionArea";
import { PageSection } from "../ui/page";
import { SegmentedTabs } from "../ui/SegmentedTabs";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { SettingsLoadFailure } from "./SettingsLoadFailure";
import { SettingsSection } from "./SettingsSection";
import {
  TerminalConnectionFields,
  TerminalLookFields,
  TerminalStartFields,
  TerminalSwitches,
} from "./TerminalSettingsFields";
import { TerminalPreview } from "./TerminalPreview";
import { TouchModeField } from "./TouchModeField";
import {
  draftOf,
  initialTerminalDraft,
  isFontSizeUnusable,
  settingsOf,
  type TerminalDraft,
} from "./terminalSettingsDraft";
import { useLoad } from "../ui/useLoad";
import { DiscardDraftDialog } from "../ui/DiscardDraftDialog";
import { TerminalPresetBar } from "./TerminalPresetBar";
import { refreshTerminalPresets, selectTerminalPreset, updateTerminalPresets, useTerminalPresets } from "./terminalPresetLibrary";
import { presetSettingsOf, sharedSettingsOf, standardPresetId, withPreset, type TerminalPreset } from "./terminalPresets";
import { newIdentifier } from "../ui/randomIdentifier";

export type TerminalSettingsApi = Pick<SettingsApi, "terminalSettings" | "setTerminalSettings" | "localShellProfiles">;

type TerminalSettingsTab = "look" | "connection" | "operation";

const tabLabels: Record<TerminalSettingsTab, MessageKey> = {
  look: "terminal.settingsGroupLook",
  connection: "terminal.settingsGroupConnection",
  operation: "terminal.settingsGroupOperation",
};

const tabs = Object.keys(tabLabels) as TerminalSettingsTab[];

const panelId = "terminal-settings-panel";

function tabId(tab: TerminalSettingsTab): string {
  return `terminal-settings-${tab}-tab`;
}

const startDirectoryFailures: Partial<Record<string, MessageKey>> = {
  start_directory_missing: "terminal.startMissing",
  start_directory_not_a_directory: "terminal.startNotADirectory",
  start_directory_unusable: "terminal.startUnusable",
};

// The engine refuses a number it does not accept with one code, whichever number it is; the font size is the
// only one outside the connection tab.
function tabOfRefusedNumber(draft: TerminalDraft): TerminalSettingsTab {
  return isFontSizeUnusable(draft) ? "look" : "connection";
}

// saveFailureOf says why a save was refused and which tab holds the field to fix, if any.
function saveFailureOf(error: unknown, draft: TerminalDraft): { message: MessageKey; tab: TerminalSettingsTab | null } {
  const code = failureCode(error);
  const startDirectoryMessage = startDirectoryFailures[code];
  if (startDirectoryMessage !== undefined) return { message: startDirectoryMessage, tab: "operation" };
  if (code === "terminal_limits_out_of_range" || code === "invalid_request") {
    return { message: "terminal.limitsOutOfRange", tab: tabOfRefusedNumber(draft) };
  }
  return { message: "terminal.settingsSaveFailed", tab: null };
}

// The embedded terminal: where it starts, its limits, how it looks and how it treats the clipboard and
// keyboard. A computer shows the groups as tabs so each fits on one screen; a phone shows every group as a
// card. Either way there is one draft and one save, so moving between tabs keeps what was typed.
//
// The page edits one set of settings at a time (TerminalPresetBar): the standard set, or a preset. A preset
// holds only what the browser applies; the rest of the draft is the settings every browser shares, saved with
// the standard set whichever set is open.
export function TerminalSettingsSection({ api, showHeading, onSettingsChange }: {
  api: TerminalSettingsApi;
  showHeading: boolean;
  onSettingsChange?: ((settings: TerminalSettings) => void | Promise<void>) | undefined;
}) {
  const t = useTranslate();
  const compact = useMediaQuery(mobileViewportQuery);
  const [draft, setDraft] = useState<TerminalDraft>(initialTerminalDraft);
  const [profiles, setProfiles] = useState<LocalShellProfile[]>([]);
  const [tab, setTab] = useState<TerminalSettingsTab>("look");
  const save = useAsyncOperation();
  // stored is the standard set as saved; the draft starts from it, or from it with the open preset.
  const [stored, setStored] = useState<TerminalSettings>({});
  const loadSettings = useCallback(async () => { setStored(await api.terminalSettings()); }, [api]);
  const settingsLoad = useLoad(loadSettings);
  const loaded = settingsLoad.state === "loaded";
  // drafted is whether the draft holds the loaded settings; the fields wait for it, not just for the load.
  const [drafted, setDrafted] = useState(false);
  const library = useTerminalPresets();
  // opened is the set the person chose to edit; until then the page opens the one this browser uses.
  const [opened, setOpened] = useState<string | null>(null);
  const editedPreset: TerminalPreset | undefined = library.presets.find((preset) => preset.id === (opened ?? library.selected));
  const editing = editedPreset?.id ?? standardPresetId;
  const inUse = library.presets.some((preset) => preset.id === library.selected) ? library.selected : standardPresetId;
  // edited is whether the draft has changes not saved yet; until it has, it follows the open set.
  const edited = useRef(false);
  const [discarding, setDiscarding] = useState<string | null>(null);

  // The presets come in on their own; until they do, the draft is the standard set's, and it moves to this
  // browser's preset once they arrive, if nothing was typed yet.
  useEffect(() => {
    if (!loaded || edited.current) return;
    setDraft(draftOf(withPreset(stored, editedPreset)));
    setDrafted(true);
  }, [loaded, stored, editedPreset]);

  useEffect(() => {
    let active = true;
    if (api.localShellProfiles !== undefined) {
      void api.localShellProfiles()
        .then((answer) => { if (active) setProfiles(answer.profiles); })
        .catch(() => undefined);
    }
    return () => { active = false; };
  }, [api]);

  function edit(patch: Partial<TerminalDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
    edited.current = true;
    save.touch();
  }

  function open(id: string) {
    if (edited.current) {
      setDiscarding(id);
      return;
    }
    edited.current = false;
    setOpened(id);
    save.touch();
  }

  // applied tells the rest of the app the standard set changed. The save is done by then, so a failure here
  // does not make it look failed; the session poll catches up.
  async function applied(settings: TerminalSettings) {
    setStored(settings);
    try {
      await onSettingsChange?.(settings);
    } catch {
      // The normal session poll will reconcile the view shortly.
    }
  }

  async function createPreset(name: string) {
    const settings = settingsOf(draft) ?? withPreset(stored, editedPreset);
    const preset: TerminalPreset = { id: newIdentifier(), name, settings: presetSettingsOf(settings) };
    await updateTerminalPresets([...library.presets, preset]);
    edited.current = false;
    setOpened(preset.id);
  }

  async function renamePreset(name: string) {
    await updateTerminalPresets(library.presets.map((preset) => preset.id === editing ? { ...preset, name } : preset));
  }

  async function deletePreset() {
    try {
      await updateTerminalPresets(library.presets.filter((preset) => preset.id !== editing));
      if (inUse === editing) selectTerminalPreset(standardPresetId);
      edited.current = false;
      setOpened(standardPresetId);
    } catch {
      save.fail(t("terminal.presetSaveFailed"));
    }
  }

  function useHere() {
    try {
      selectTerminalPreset(editing);
    } catch {
      save.fail(t("terminal.presetSaveFailed"));
    }
  }

  async function submit() {
    const next = settingsOf(draft);
    if (next === null) {
      setTab(tabOfRefusedNumber(draft));
      save.fail(t("terminal.limitsOutOfRange"));
      return;
    }
    await save.run(async () => {
      if (editedPreset === undefined) {
        await api.setTerminalSettings(next);
        await applied(next);
      } else {
        await updateTerminalPresets(library.presets.map((preset) =>
          preset.id === editedPreset.id ? { ...preset, settings: presetSettingsOf(next) } : preset));
        // The shared part goes to the standard set, which keeps its own part as it was.
        const shared = { ...presetSettingsOf(stored), ...sharedSettingsOf(next) };
        if (JSON.stringify(sharedSettingsOf(shared)) !== JSON.stringify(sharedSettingsOf(stored))) {
          await api.setTerminalSettings(shared);
          await applied(shared);
        }
      }
      edited.current = false;
    }, {
      describe: (error) => {
        const failure = saveFailureOf(error, draft);
        // The save button is on every tab, so open the one with the field the message is about.
        if (failure.tab !== null) setTab(failure.tab);
        return t(failure.message);
      },
    });
  }

  const busy = save.busy;
  const ready = loaded && drafted;
  const presetsUnavailable = !ready || busy || library.loading || library.busy || library.error;
  const groupProps = { draft, edit };
  // The preview takes a column of its own only where the board leaves the fields room beside it. It goes by
  // the board's width, not the screen's: on a PC the navigation and the list of settings pages take 470px.
  const look = (
    <div className="grid items-start gap-6 @2xl:grid-cols-[minmax(0,1fr)_220px]">
      <div className="min-w-0"><TerminalLookFields {...groupProps} /></div>
      <TerminalPreview
        palette={draft.palette}
        font={draft.font}
        background={draft.background}
        tint={draft.tint}
        fontSize={draft.fontSize}
      />
    </div>
  );
  return (
    <SettingsSection id="settings-terminal" label={t("terminal.settingsHeading")} icon="terminal" showHeading={showHeading}>
      <SettingsLoadFailure settingsLoad={settingsLoad} title={t("terminal.settingsLoadFailed")} />
      {library.error ? (
        <Notice tone="danger" actions={<Button small onClick={() => { void refreshTerminalPresets(); }}>{t("terminal.presetsReload")}</Button>}>
          {t("terminal.presetsReloadRequired")}
        </Notice>
      ) : null}
      <TerminalPresetBar
        presets={library.presets}
        editing={editing}
        inUse={inUse}
        disabled={presetsUnavailable}
        onOpen={open}
        onUseHere={useHere}
        onCreate={createPreset}
        onRename={renamePreset}
        onDelete={deletePreset}
      />
      {discarding === null ? null : (
        <DiscardDraftDialog
          id="terminal-preset-discard"
          onCancel={() => setDiscarding(null)}
          onConfirm={() => {
            edited.current = false;
            setOpened(discarding);
            setDiscarding(null);
            save.touch();
          }}
        />
      )}
      {compact ? null : (
        <SegmentedTabs
          label={t("terminal.settingsGroups")}
          tabs={tabs.map((value) => ({ value, label: t(tabLabels[value]) }))}
          value={tab}
          onChange={setTab}
          tabId={tabId}
          panelId={panelId}
        />
      )}
      <fieldset disabled={!ready || busy} className="min-w-0 border-0 p-0 disabled:opacity-70">
        {compact ? (
          <div className="flex flex-col gap-3.5">
            {/* Each card's rows sit in their own box so the first row, right under the heading, has no line above it. */}
            <PageSection heading={t("terminal.settingsGroupStart")}><div><TerminalStartFields {...groupProps} profiles={profiles} /></div></PageSection>
            <PageSection heading={t("terminal.settingsGroupConnection")}><div><TerminalConnectionFields {...groupProps} /></div></PageSection>
            <PageSection heading={t("terminal.settingsGroupLook")}>{look}</PageSection>
            <PageSection heading={t("terminal.settingsGroupOperation")}>
              <div><TerminalSwitches {...groupProps} /><div className="border-t border-line pt-1.5"><TouchModeField /></div></div>
            </PageSection>
          </div>
        ) : (
          <div id={panelId} role="tabpanel" aria-labelledby={tabId(tab)} className="mt-[18px] border-t border-line pt-[18px]">
            {tab === "look" ? look : null}
            {tab === "connection" ? <TerminalConnectionFields {...groupProps} /> : null}
            {tab === "operation" ? (
              <>
                <TerminalStartFields {...groupProps} profiles={profiles} />
                <div className="border-t border-line pt-1.5"><TerminalSwitches {...groupProps} /></div>
                <div className="border-t border-line pt-1.5"><TouchModeField /></div>
              </>
            ) : null}
          </div>
        )}
      </fieldset>
      {/* 保存のボタンは fieldset の外に置く。中に置くと、保存を待つ間に fieldset ごと薄くなり、
          回転の印が「押せない」に見える。 */}
      <ActionArea status={save.error === ""
        ? (settingsLoad.state === "loading"
            ? <p role="status" className="text-sm text-ink-muted">{t("terminal.settingsLoading")}</p>
            : !ready || !save.saved ? undefined : <Notice tone="success">{t("terminal.settingsSaved")}</Notice>)
        : <Notice tone="danger">{save.error}</Notice>}
      >
        <Button kind="primary" busy={busy} disabled={!ready} onClick={() => void submit()}>
          {t("terminal.startSave")}
        </Button>
      </ActionArea>
    </SettingsSection>
  );
}
