import { useEffect, useMemo, useRef, useState } from "react";
import { failureCode } from "../api/client";
import type { LocalShellProfile, SettingsApi } from "../api/settings";
import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { ActionArea } from "../ui/ActionArea";
import { PageSection } from "../ui/page";
import { SegmentedTabs } from "../ui/SegmentedTabs";
import { Button, Notice } from "../ui/surface";
import { useAsyncOperation } from "../ui/useAsyncOperation";
import { mobileViewportQuery, useMediaQuery } from "../ui/useMediaQuery";
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
import { updatePreset, useSettings } from "./settingsStore";

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
// The page edits the terminal settings of the preset chosen above the settings pages (presetId). Reconnects,
// the connection log and OSC 52 are global settings, on the General page.
export function TerminalSettingsSection({ api, presetId, showHeading }: {
  api: TerminalSettingsApi;
  presetId: string;
  showHeading: boolean;
}) {
  const t = useTranslate();
  const compact = useMediaQuery(mobileViewportQuery);
  const [draft, setDraft] = useState<TerminalDraft>(initialTerminalDraft);
  const [profiles, setProfiles] = useState<LocalShellProfile[]>([]);
  const [tab, setTab] = useState<TerminalSettingsTab>("look");
  const save = useAsyncOperation();
  const settings = useSettings();
  const loaded = settings.document !== null;
  // drafted is whether the draft holds the loaded settings; the fields wait for it, not just for the load.
  const [drafted, setDrafted] = useState(false);
  const stored = useMemo(() => settings.document?.presets.find((preset) => preset.id === presetId)?.terminal, [settings.document, presetId]);
  // edited is whether the draft has changes not saved yet; until it has, it follows the preset.
  const edited = useRef(false);
  // draftedFor is the preset the draft was made from.
  const draftedFor = useRef<string | null>(null);

  // The draft follows the preset as long as nothing was typed yet. Another preset opens with its own settings:
  // what was typed for the previous one is not carried over.
  useEffect(() => {
    if (settings.document === null || (draftedFor.current === presetId && edited.current)) return;
    draftedFor.current = presetId;
    edited.current = false;
    setDraft(draftOf(stored ?? {}));
    setDrafted(true);
  }, [settings.document, stored, presetId]);

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

  async function submit() {
    const next = settingsOf(draft);
    if (next === null) {
      setTab(tabOfRefusedNumber(draft));
      save.fail(t("terminal.limitsOutOfRange"));
      return;
    }
    await save.run(async () => {
      await updatePreset(presetId, ({ terminal: _previous, ...preset }) =>
        Object.keys(next).length === 0 ? preset : { ...preset, terminal: next });
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
      {settings.loadFailed ? <Notice tone="danger">{t("terminal.settingsLoadFailed")}</Notice> : null}
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
      {/* 保存のボタンは fieldset の外に置く。中に置くと、保存を待つ間に fieldset ごと薄くなり、
          回転の印が「押せない」に見える。 */}
      <ActionArea status={save.error === ""
        ? (!loaded && !settings.loadFailed
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
