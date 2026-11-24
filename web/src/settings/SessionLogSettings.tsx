import { CodeField } from "../editor/CodeField";
import { useTranslate } from "../i18n/context";
import { FieldRow } from "../ui/FieldRow";
import { CheckboxField } from "../ui/form";
import { PageSection } from "../ui/page";
import type { GlobalSettings } from "./settingsStore";
import { sessionLogRulesValid } from "./sessionLogRules";
import { useTypedSetting, type ParsedSetting } from "./useTypedSetting";

type GlobalTerminalSettings = NonNullable<GlobalSettings["terminal"]>;
type ChangeGlobal = (field: string, next: (settings: GlobalSettings) => GlobalSettings) => void;

// SessionLogSettings is whether sessions keep a log of what their terminal showed, and which connections keep none
// (internal/sessionlog). Both are global settings; where a machine keeps the logs is that machine's (Machines).
export function SessionLogSettings({ terminal, change }: { terminal: GlobalTerminalSettings; change: ChangeGlobal }) {
  const t = useTranslate();
  const enabled = terminal.sessionLog ?? false;
  return (
    <PageSection heading={t("settings.sessionLogHeading")}>
      <div>
        <FieldRow label={t("settings.sessionLogLabel")} hint={t("settings.sessionLogHint")} interactiveChildren>
          <CheckboxField label={t("settings.sessionLogKeep")} checked={enabled} className="pt-1.5"
            onChange={(checked) => change("terminal.sessionLog", (settings) => {
              const { sessionLog: _previous, ...rest } = settings.terminal ?? {};
              return { ...settings, terminal: checked ? { ...rest, sessionLog: true } : rest };
            })} />
        </FieldRow>
        {enabled ? <SessionLogRulesField rules={terminal.sessionLogRules ?? ""} change={change} /> : null}
      </div>
    </PageSection>
  );
}

// The draft takes only rules the engine takes: a half-typed pattern must not decide which sessions keep a log.
function SessionLogRulesField({ rules, change }: { rules: string; change: ChangeGlobal }) {
  const t = useTranslate();
  const typed = useTypedSetting<string>({
    text: rules,
    parse: (text): ParsedSetting<string> => sessionLogRulesValid(text) ? { value: text } : { problem: t("settings.sessionLogRulesInvalid") },
    onChange: (text) => change("terminal.sessionLogRules", (settings) => {
      const { sessionLogRules: _previous, ...rest } = settings.terminal ?? {};
      return { ...settings, terminal: text.trim() === "" ? rest : { ...rest, sessionLogRules: text } };
    }),
  });
  return (
    <FieldRow
      label={t("settings.sessionLogRulesLabel")}
      hint={t("settings.sessionLogRulesHint")}
      error={typed.problem === "" ? undefined : typed.problem}
      interactiveChildren
    >
      <CodeField
        label={t("settings.sessionLogRulesLabel")}
        value={typed.shown}
        minLines={4}
        maxLines={12}
        placeholder={"prod-*\n!prod-web\n@local"}
        onChange={typed.type}
        onBlur={typed.leave}
      />
    </FieldRow>
  );
}
