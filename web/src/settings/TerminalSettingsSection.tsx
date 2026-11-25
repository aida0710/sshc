import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import type { LocalShellProfile, SettingsApi } from "../api/settings";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { PageSection } from "../ui/page";
import { SegmentedTabs } from "../ui/SegmentedTabs";
import { Notice } from "../ui/surface";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
import { SettingsSection } from "./SettingsSection";
import {
  TerminalConnectionFields,
  TerminalLookFields,
  TerminalStartFields,
  TerminalSwitches,
  type TerminalGroupProps,
} from "./TerminalSettingsFields";
import { TerminalPreview } from "./TerminalPreview";
import { TouchModeField } from "./TouchModeField";
import { useDescribeFailure, useDraftProblem, useSettingsDraft } from "./settingsDraftContext";
import { draftOf, settingsOf, unusableNumbers, type TerminalDraft, type TypedNumber } from "./terminalSettingsDraft";
import { PanelState } from "../ui/PanelState";

export type TerminalSettingsApi = Pick<SettingsApi, "localShellProfiles">;

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

// The font size is the only typed number outside the connection tab.
function tabOfNumber(field: TypedNumber["field"]): TerminalSettingsTab {
  return field === "fontSize" ? "look" : "connection";
}

// saveFailureOf says why the engine refused the terminal settings and which tab holds the field to fix, or null for
// a failure that is not about them.
function saveFailureOf(error: unknown): { message: MessageKey; tab: TerminalSettingsTab } | null {
  const code = failureCode(error);
  const startDirectoryMessage = startDirectoryFailures[code];
  if (startDirectoryMessage !== undefined) return { message: startDirectoryMessage, tab: "operation" };
  if (code === "terminal_limits_out_of_range") return { message: "terminal.limitsOutOfRange", tab: "connection" };
  return null;
}

// The embedded terminal: where it starts, its limits, how it looks and how it treats the clipboard and
// keyboard. A computer shows the groups as tabs so each fits on one screen; a phone shows every group as a
// card.
//
// The page edits the terminal settings of the preset chosen above the settings pages (presetId). Reconnects,
// the connection log and OSC 52 are global settings, on the General page.
export function TerminalSettingsSection({ api, presetId, showHeading }: {
  api: TerminalSettingsApi;
  presetId: string;
  showHeading: boolean;
}) {
  const t = useTranslate();
  const compact = useMediaQuery(mobileViewportQuery);
  const [profiles, setProfiles] = useState<LocalShellProfile[]>([]);
  const [tab, setTab] = useState<TerminalSettingsTab>("look");
  const { document, loadFailed, editPreset, discarded, savedCount } = useSettingsDraft();
  const preset = document?.presets.find((candidate) => candidate.id === presetId);
  // typed is what the fields hold once one was changed, with numbers as they were typed; until then they follow the
  // preset.
  const [typed, setTyped] = useState<TerminalDraft | null>(null);
  const draft = typed ?? draftOf(preset?.terminal ?? {});
  const unusable = unusableNumbers(draft);
  const problemOf = (number: TypedNumber) => t(number.outOfRange, number.range);
  // On a computer the field may be on another tab.
  const hidden = compact ? undefined : unusable.find((number) => tabOfNumber(number.field) !== tab);
  const firstProblem = hidden ?? unusable[0];
  useDraftProblem(firstProblem === undefined ? "" : problemOf(firstProblem), { outOfSight: hidden !== undefined });

  // Once discarded or saved, and for another preset, the fields follow the preset again. What was typed while a save
  // ran is in the draft already, unless it cannot be saved yet.
  const unusableNow = unusable.length > 0;
  useEffect(() => setTyped(null), [discarded, presetId]);
  useEffect(() => {
    if (!unusableNow) setTyped(null);
    // Runs when a save lands, not when a number becomes usable.
    // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [savedCount]);

  useDescribeFailure((error) => {
    const failure = saveFailureOf(error);
    if (failure === null) return null;
    setTab(failure.tab);
    return t(failure.message);
  });

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
    const next = { ...draft, ...patch };
    setTyped(next);
    if (unusableNumbers(next).length > 0) return;
    editPreset(presetId, "terminal", ({ terminal: _previous, ...current }) => {
      const terminal = settingsOf(next);
      return Object.keys(terminal).length === 0 ? current : { ...current, terminal };
    });
  }

  const ready = document !== null;
  const groupProps: TerminalGroupProps = {
    draft,
    edit,
    problems: Object.fromEntries(unusable.map((number) => [number.field, problemOf(number)])),
  };
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
      {loadFailed ? <Notice tone="danger">{t("terminal.settingsLoadFailed")}</Notice> : null}
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
      <fieldset disabled={!ready} className="min-w-0 border-0 p-0">
        {compact ? (
          <div className="flex flex-col gap-3.5">
            {/* Each card's rows sit in their own box so the first row, right under the heading, has no line above it. */}
            <PageSection heading={t("terminal.settingsGroupStart")}><div><TerminalStartFields {...groupProps} profiles={profiles} /></div></PageSection>
            <PageSection heading={t("terminal.settingsGroupConnection")}><div><TerminalConnectionFields {...groupProps} /></div></PageSection>
            <PageSection heading={t("terminal.settingsGroupLook")}>{look}</PageSection>
            <PageSection heading={t("terminal.settingsGroupOperation")}>
              <div><TerminalSwitches {...groupProps} /><div className="border-t border-line pt-1.5"><TouchModeField presetId={presetId} /></div></div>
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
                <div className="border-t border-line pt-1.5"><TouchModeField presetId={presetId} /></div>
              </>
            ) : null}
          </div>
        )}
      </fieldset>
      {!ready && !loadFailed ? <PanelState layout="line" tone="loading" title={t("terminal.settingsLoading")} /> : null}
    </SettingsSection>
  );
}
