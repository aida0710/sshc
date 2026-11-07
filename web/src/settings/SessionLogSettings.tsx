import { useEffect, useState } from "react";
import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField, control } from "../ui/form";
import { PageSection } from "../ui/page";
import type { GlobalSettings } from "./settingsStore";
import { sessionLogRulesValid } from "./sessionLogRules";

type GlobalTerminalSettings = NonNullable<GlobalSettings["terminal"]>;

// SessionLogSettings is whether sessions keep a log of what their terminal showed, and which connections keep none
// (internal/sessionlog). Both are global settings; where a machine keeps the logs is that machine's (Machines).
export function SessionLogSettings({ terminal, save }: {
  terminal: GlobalTerminalSettings;
  save: (change: (settings: GlobalSettings) => GlobalSettings) => void;
}) {
  const t = useTranslate();
  const enabled = terminal.sessionLog ?? false;
  return (
    <PageSection heading={t("settings.sessionLogHeading")}>
      <div>
        <FieldRow label={t("settings.sessionLogLabel")} hint={t("settings.sessionLogHint")} interactiveChildren>
          <CheckboxField label={t("settings.sessionLogKeep")} checked={enabled} className="pt-1.5"
            onChange={(checked) => save((settings) => {
              const { sessionLog: _previous, ...rest } = settings.terminal ?? {};
              return { ...settings, terminal: checked ? { ...rest, sessionLog: true } : rest };
            })} />
        </FieldRow>
        {enabled ? <SessionLogRulesField rules={terminal.sessionLogRules ?? ""} save={save} /> : null}
      </div>
    </PageSection>
  );
}

// SessionLogRulesField keeps what is typed until the box is left, as the other typed settings do, and saves only
// rules the engine takes: a half-typed pattern must not decide which sessions keep a log.
function SessionLogRulesField({ rules, save }: {
  rules: string;
  save: (change: (settings: GlobalSettings) => GlobalSettings) => void;
}) {
  const t = useTranslate();
  const [draft, setDraft] = useState<string | null>(null);
  useEffect(() => setDraft(null), [rules]);
  const text = draft ?? rules;
  const valid = sessionLogRulesValid(text);
  return (
    <FieldRow
      label={t("settings.sessionLogRulesLabel")}
      hint={t("settings.sessionLogRulesHint")}
      warning={valid ? undefined : t("settings.sessionLogRulesInvalid")}
      interactiveChildren
    >
      <textarea
        aria-label={t("settings.sessionLogRulesLabel")}
        value={text}
        rows={3}
        placeholder={"prod-*\n!prod-web\n@local"}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={() => {
          if (!valid || text === rules) return;
          save((settings) => {
            const { sessionLogRules: _previous, ...rest } = settings.terminal ?? {};
            return { ...settings, terminal: text.trim() === "" ? rest : { ...rest, sessionLogRules: text } };
          });
        }}
        className={`${control} min-h-[92px] resize-y font-mono`}
      />
    </FieldRow>
  );
}
