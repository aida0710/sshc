import { useTranslate } from "../i18n/context";
import type { MessageKey } from "../i18n/messages";
import { control } from "../ui/form";
import { useFieldError } from "../ui/fieldError";
import { FieldRow } from "../ui/FieldRow";
import { Notice } from "../ui/surface";
import { PageSectionFrame } from "../ui/page";
import { useSettingsDraft } from "./settingsDraftContext";
import type { SettingsPreset } from "./settingsStore";
import { useTypedSetting, type ParsedSetting } from "./useTypedSetting";

type AutoLock = NonNullable<SettingsPreset["vaultAutoLock"]>;
type IdleAutoLock = { mode: "idle"; value: number; unit: "minutes" | "hours" };

// defaultAutoLock is the engine's default: twelve hours without use (secret.IdleTimeout).
const defaultAutoLock: IdleAutoLock = { mode: "idle", value: 12, unit: "hours" };
// The limits the engine keeps to (application.MinVaultAutoLockValue, MaxVaultAutoLockValue).
const minimumValue = 1;
const maximumValue = 999;

function idleAutoLockOf(preset: SettingsPreset | undefined): IdleAutoLock {
  const stored = preset?.vaultAutoLock;
  if (stored?.mode !== "idle") return defaultAutoLock;
  return { mode: "idle", value: stored.value ?? defaultAutoLock.value, unit: stored.unit ?? defaultAutoLock.unit };
}

// withAutoLock is preset with the choice. A preset that keeps none follows the default, so going back to the default
// leaves it as it was.
function withAutoLock(preset: SettingsPreset, next: { mode: "restart" } | IdleAutoLock): SettingsPreset {
  const isDefault = next.mode === "idle" && next.value === defaultAutoLock.value && next.unit === defaultAutoLock.unit;
  if (isDefault && preset.vaultAutoLock === undefined) return preset;
  const autoLock: AutoLock = next;
  return { ...preset, vaultAutoLock: autoLock };
}

// How long the vault stays open unattended, in the preset being edited. Every machine that uses the preset
// follows it. The engine never locks a passwordless vault, so there the saved choice stays visible but cannot
// be changed until a master password is set.
export function VaultAutoLockSection({ presetId, passwordless }: {
  presetId: string;
  passwordless: boolean;
}) {
  const t = useTranslate();
  const { document, editPreset } = useSettingsDraft();
  const preset = document?.presets.find((candidate) => candidate.id === presetId);
  const mode = preset?.vaultAutoLock?.mode === "restart" ? "restart" : "idle";
  const idle = idleAutoLockOf(preset);
  const choose = (next: { mode: "restart" } | IdleAutoLock) =>
    editPreset(presetId, "vaultAutoLock", (current) => withAutoLock(current, next));

  const loaded = document !== null;
  const disabled = !loaded || passwordless;
  const notice: MessageKey | null = passwordless
    ? "settings.vaultAutoLockPasswordless"
    : mode === "restart" ? "settings.vaultAutoLockRestartWarning" : null;
  return (
    <PageSectionFrame>
      <section aria-label={t("settings.vaultAutoLockLabel")}>
        <FieldRow label={t("settings.vaultAutoLockLabel")} hint={t("settings.vaultAutoLockHint")}>
          <select className={control} value={mode} disabled={disabled}
            onChange={(event) => choose(event.target.value === "restart" ? { mode: "restart" } : idle)}>
            <option value="idle">{t("settings.vaultAutoLockIdle")}</option>
            <option value="restart">{t("settings.vaultAutoLockRestart")}</option>
          </select>
        </FieldRow>
        {mode === "idle" ? (
          <FieldRow label={t("settings.vaultAutoLockValue")} interactiveChildren>
            <div className="grid grid-cols-[minmax(0,1fr)_minmax(8rem,0.65fr)] gap-2">
              <AutoLockValueInput value={idle.value} disabled={disabled} onChange={(value) => choose({ ...idle, value })} />
              <select aria-label={t("settings.vaultAutoLockUnit")} className={control} value={idle.unit} disabled={disabled}
                onChange={(event) => choose({ ...idle, unit: event.target.value as IdleAutoLock["unit"] })}>
                <option value="minutes">{t("settings.vaultAutoLockMinutes")}</option>
                <option value="hours">{t("settings.vaultAutoLockHours")}</option>
              </select>
            </div>
          </FieldRow>
        ) : null}
        {notice === null ? null : <div className="mt-2"><Notice>{t(notice)}</Notice></div>}
        {loaded ? null : <p role="status" className="mt-2 text-sm text-ink-muted">{t("settings.vaultAutoLockLoading")}</p>}
      </section>
    </PageSectionFrame>
  );
}

function AutoLockValueInput({ value, disabled, onChange }: { value: number; disabled: boolean; onChange: (value: number) => void }) {
  const t = useTranslate();
  const typed = useTypedSetting<number>({
    text: String(value),
    parse: (text): ParsedSetting<number> => {
      const parsed = Number(text);
      return Number.isSafeInteger(parsed) && parsed >= minimumValue && parsed <= maximumValue
        ? { value: parsed }
        : { problem: t("settings.vaultAutoLockOutOfRange") };
    },
    onChange,
  });
  const described = useFieldError(typed.problem, {});
  return (
    <input type="number" inputMode="numeric" min={minimumValue} max={maximumValue} aria-label={t("settings.vaultAutoLockValue")} className={control}
      {...described} value={typed.shown} disabled={disabled} onChange={(event) => typed.type(event.target.value)} onBlur={typed.leave} />
  );
}
