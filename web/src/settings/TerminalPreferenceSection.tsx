import { useEffect, useMemo, useState } from "react";
import {
  integrationsApi,
  type IntegrationsApi,
  type TerminalID,
  type TerminalOptionsResponse,
  type TerminalPreferenceRequest,
} from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { control, fieldLabel, hintText, primaryAction, sectionCard, sectionHeading } from "../ui/form";
import { Notice } from "../ui/surface";

const terminalNames: Record<Exclude<TerminalID, "custom">, string> = {
  terminal: "Terminal.app",
  iterm2: "iTerm2",
  kitty: "kitty",
  ghostty: "Ghostty",
  wezterm: "WezTerm",
};

function splitArguments(value: string): string[] {
  return value.split(/\s+/).filter((argument) => argument !== "");
}

function validArguments(arguments_: string[]): boolean {
  return arguments_.length <= 8 && arguments_.every(
    (argument) => argument.length <= 64 && !/[\p{Cc}\p{Z}]/u.test(argument),
  );
}

type TerminalPreferenceSectionProps = {
  api?: Pick<IntegrationsApi, "terminalOptions" | "setTerminalPreference">;
};

export function TerminalPreferenceSection({ api = integrationsApi }: TerminalPreferenceSectionProps) {
  const t = useTranslate();
  const [saved, setSaved] = useState<TerminalOptionsResponse | null>(null);
  const [draftSelected, setDraftSelected] = useState<TerminalID | null>(null);
  const [customApplication, setCustomApplication] = useState("");
  const [customArguments, setCustomArguments] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  useEffect(() => {
    let active = true;
    void api.terminalOptions()
      .then((options) => {
        if (!active) return;
        setSaved(options);
        setCustomApplication(options.customTerminal?.application ?? "");
        setCustomArguments((options.customTerminal?.arguments ?? []).join(" "));
      })
      .catch(() => {
        if (active) setError(t("settings.terminalLoadFailed"));
      });
    return () => {
      active = false;
    };
  }, [api, t]);

  const selected = draftSelected ?? saved?.selected ?? "terminal";
  const argv = useMemo(() => splitArguments(customArguments), [customArguments]);

  async function save(request: TerminalPreferenceRequest) {
    if (saved === null || busy) return;
    setBusy(true);
    setError("");
    setSuccess("");
    try {
      const refreshed = await api.setTerminalPreference(request);
      setSaved(refreshed);
      setDraftSelected(null);
      setCustomApplication(refreshed.customTerminal?.application ?? "");
      setCustomArguments((refreshed.customTerminal?.arguments ?? []).join(" "));
      setSuccess(t("settings.terminalSaved"));
    } catch {
      setDraftSelected(null);
      setCustomApplication(saved.customTerminal?.application ?? "");
      setCustomArguments((saved.customTerminal?.arguments ?? []).join(" "));
      setError(t("settings.terminalSaveFailed"));
    } finally {
      setBusy(false);
    }
  }

  function chooseTerminal(next: TerminalID) {
    setError("");
    setSuccess("");
    if (next === "custom") {
      setDraftSelected("custom");
      return;
    }
    setDraftSelected(next);
    void save({ selected: next });
  }

  const canSaveCustom = saved !== null && !busy &&
    saved.applications.some((application) => application.path === customApplication) && validArguments(argv);

  return (
    <section aria-label={t("settings.terminalHeading")} className={sectionCard}>
      <h3 className={sectionHeading}>{t("settings.terminalHeading")}</h3>
      <p className={hintText}>{t("settings.terminalDescription")}</p>
      {error === "" ? null : <Notice tone="danger">{error}</Notice>}
      {success === "" ? null : <p role="status" className="text-sm text-live">{success}</p>}
      {saved === null ? (
        error === "" ? <p className={hintText}>{t("settings.terminalLoading")}</p> : null
      ) : saved.terminals.length === 0 ? (
        <Notice>{t("settings.terminalUnsupported")}</Notice>
      ) : (
        <>
          <label className="flex flex-col gap-1">
            <span className={fieldLabel}>{t("settings.terminalOpenWith")}</span>
            <select
              aria-label={t("settings.terminalOpenWith")}
              value={selected}
              disabled={busy}
              onChange={(event) => chooseTerminal(event.target.value as TerminalID)}
              className={control}
            >
              {saved.terminals.map((terminal) => {
                const customUnavailable = terminal.id === "custom" && saved.applications.length === 0;
                const unavailable = !terminal.installed || customUnavailable;
                const name = terminal.id === "custom" ? t("settings.terminalOtherApplication") : terminalNames[terminal.id];
                return (
                  <option key={terminal.id} value={terminal.id} disabled={unavailable}>
                    {name}{unavailable ? ` — ${t("settings.terminalNotInstalled")}` : ""}
                  </option>
                );
              })}
            </select>
          </label>
          {selected === "custom" ? (
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="flex flex-col gap-1">
                <span className={fieldLabel}>{t("settings.terminalApplication")}</span>
                <select
                  aria-label={t("settings.terminalApplication")}
                  value={customApplication}
                  disabled={busy}
                  onChange={(event) => setCustomApplication(event.target.value)}
                  className={control}
                >
                  <option value="">{t("settings.terminalChooseApplication")}</option>
                  {saved.applications.map((application) => (
                    <option key={application.path} value={application.path}>{application.name}</option>
                  ))}
                </select>
              </label>
              <label className="flex flex-col gap-1">
                <span className={fieldLabel}>{t("settings.terminalArguments")}</span>
                <input
                  aria-label={t("settings.terminalArguments")}
                  value={customArguments}
                  disabled={busy}
                  onChange={(event) => setCustomArguments(event.target.value)}
                  className={control}
                />
                <span className={hintText}>{t("settings.terminalArgumentsHint")}</span>
              </label>
              <button
                type="button"
                className={`w-fit ${primaryAction}`}
                disabled={!canSaveCustom}
                onClick={() => void save({
                  selected: "custom",
                  customTerminal: { application: customApplication, arguments: argv },
                })}
              >
                {busy ? t("settings.terminalSaving") : t("settings.terminalSaveCustom")}
              </button>
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}
