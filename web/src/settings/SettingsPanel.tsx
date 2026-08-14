import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { CheckboxField, Field, control, hintText, primaryAction, sectionCard, sectionHeading } from "../ui/form";
import { PageHeader } from "../ui/page";
import { Button, Notice } from "../ui/surface";
import type { TerminalSessionsState } from "../terminal/sessions";

type SettingsPanelProps = {
  api?: IntegrationsApi;
  // 開いているセッションはシェルが持つ。ここはその数を見せ、まとめて
  // 閉じる入口を出すだけである。
  consoles?: Pick<TerminalSessionsState, "sessions" | "busy" | "closeAll">;
};

export function SettingsPanel({ api = integrationsApi, consoles }: SettingsPanelProps) {
  const t = useTranslate();
  const [desktopError, setDesktopError] = useState("");
  const [currentMaster, setCurrentMaster] = useState("");
  const [nextMaster, setNextMaster] = useState("");
  const [confirmMaster, setConfirmMaster] = useState("");
  const [masterBusy, setMasterBusy] = useState(false);
  const [masterError, setMasterError] = useState("");
  const [changed, setChanged] = useState("");
  const [keepRunning, setKeepRunning] = useState(false);
  const [desktopBusy, setDesktopBusy] = useState(false);
  // 3 つとも文字列で持つ。**空は「設定されていない」であり、0 ではない。**
  // 数として持つと、空欄と 0 を区別する場所をもう一つ作ることになる。
  const [startDirectory, setStartDirectory] = useState("");
  const [maxSessions, setMaxSessions] = useState("");
  const [scrollback, setScrollback] = useState("");
  const [terminalBusy, setTerminalBusy] = useState(false);
  const [terminalError, setTerminalError] = useState("");
  const [terminalSaved, setTerminalSaved] = useState(false);

  useEffect(() => {
    let active = true;
    void api.desktopSettings()
      .then((settings) => {
        if (active) setKeepRunning(settings.keepRunning === true);
      })
      .catch(() => {
        // 読めなければ止める側に倒す。**動かし続けるのは明示的な選択である。**
        if (active) setKeepRunning(false);
      });
    return () => {
      active = false;
    };
  }, [api]);

  useEffect(() => {
    let active = true;
    void api.terminalSettings()
      .then((settings) => {
        if (!active) return;
        setStartDirectory(settings.startDirectory ?? "");
        setMaxSessions(settings.maxSessions === undefined ? "" : String(settings.maxSessions));
        setScrollback(settings.scrollbackBytes === undefined ? "" : String(settings.scrollbackBytes));
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [api]);

  // 断られた理由はサーバーが名指しする。**「保存できません」で終わらせない**
  // ——直すのは人であり、直すには何が悪いのかが要る。
  async function saveTerminal() {
    // 数として読めないものは送らない。**空欄と「0 と書かれた」を同じにしない**
    // ——前者は既定へ戻すという意思であり、後者は範囲の外の指定である。
    const numberOr = (text: string): number | undefined => {
      const trimmed = text.trim();
      if (trimmed === "") return undefined;
      const value = Number(trimmed);
      return Number.isSafeInteger(value) ? value : Number.NaN;
    };
    const sessions = numberOr(maxSessions);
    const bytes = numberOr(scrollback);
    if (Number.isNaN(sessions) || Number.isNaN(bytes)) {
      setTerminalError(t("terminal.limitsOutOfRange"));
      setTerminalSaved(false);
      return;
    }

    setTerminalBusy(true);
    setTerminalError("");
    setTerminalSaved(false);
    try {
      const directory = startDirectory.trim();
      await api.setTerminalSettings({
        ...(directory === "" ? {} : { startDirectory: directory }),
        ...(sessions === undefined ? {} : { maxSessions: sessions }),
        ...(bytes === undefined ? {} : { scrollbackBytes: bytes }),
      });
      setTerminalSaved(true);
    } catch (error) {
      const code = failureCode(error);
      setTerminalError(t(
        code === "start_directory_missing"
          ? "terminal.startMissing"
          : code === "start_directory_not_a_directory"
            ? "terminal.startNotADirectory"
            : code === "start_directory_unusable"
              ? "terminal.startUnusable"
              : code === "terminal_limits_out_of_range" || code === "invalid_request"
                ? "terminal.limitsOutOfRange"
                : "terminal.startSaveFailed",
      ));
    } finally {
      setTerminalBusy(false);
    }
  }

  async function updateKeepRunning(next: boolean) {
    setDesktopBusy(true);
    try {
      await api.setDesktopSettings(next);
      setKeepRunning(next);
    } catch {
      setDesktopError(t("desktop.saveFailed"));
    } finally {
      setDesktopBusy(false);
    }
  }

  function clearMasterFields() {
    setCurrentMaster("");
    setNextMaster("");
    setConfirmMaster("");
  }

  async function changeMaster() {
    setMasterBusy(true);
    setMasterError("");
    setChanged("");
    try {
      const result = await api.changeMasterPassword(currentMaster, nextMaster);
      // 日付付きコピーは履歴として古いパスワードの下に残る。ここで成功と
      // 書ける対象は、ローカル一式と再 push できたライブスナップショットだけだ。
      setChanged(
        result.snapshotResealed
          ? t("secrets.changedWithSnapshot")
          : t("secrets.changedWithoutSnapshot", { reason: result.snapshotProblem ?? "" }),
      );
    } catch (caught) {
      setMasterError(
        failureCode(caught) === "wrong_passphrase" ? t("secrets.wrongCurrent") : t("secrets.changeFailed"),
      );
    } finally {
      // マスターパスワードは再試行の便宜より保持時間の短さを優先する。
      clearMasterFields();
      setMasterBusy(false);
    }
  }

  const canChangeMaster = !masterBusy && currentMaster !== "" && nextMaster.length >= 12 &&
    nextMaster === confirmMaster;

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <PageHeader title={t("settings.heading")} description={t("settings.pageDescription")} />

      {desktopError === "" ? null : <Notice tone="danger">{desktopError}</Notice>}

      <section aria-label={t("desktop.heading")} className={sectionCard}>
        <h3 className={sectionHeading}>{t("desktop.heading")}</h3>
        <p className={hintText}>{t("desktop.note")}</p>
        <CheckboxField
          label={t("desktop.keepRunning")}
          checked={keepRunning}
          disabled={desktopBusy}
          onChange={(next) => void updateKeepRunning(next)}
        />
      </section>

      {/*
        埋め込みターミナルの設定。**空欄は「設定されていない」であり、既定と
        同じ値ではない。** 既定を metadata へ書き戻すと、既定を変えた日に
        その人だけが黙って取り残される。だから空欄は空欄のまま送らない。
      */}
      <section aria-label={t("terminal.settingsHeading")} className={sectionCard}>
        <h3 className={sectionHeading}>{t("terminal.settingsHeading")}</h3>
        {terminalError === "" ? null : <Notice tone="danger">{terminalError}</Notice>}
        {!terminalSaved ? null : (
          <p role="status" className="text-sm text-live">{t("terminal.settingsSaved")}</p>
        )}
        <Field label={t("terminal.startLabel")} hint={t("terminal.startHint")}>
          <input
            type="text"
            className={control}
            value={startDirectory}
            spellCheck={false}
            placeholder="~/"
            disabled={terminalBusy}
            onChange={(event) => {
              setStartDirectory(event.target.value);
              setTerminalSaved(false);
            }}
          />
        </Field>
        <Field label={t("terminal.maxSessionsLabel")} hint={t("terminal.maxSessionsHint")}>
          <input
            type="number"
            min={1}
            max={200}
            className={control}
            value={maxSessions}
            placeholder="50"
            disabled={terminalBusy}
            onChange={(event) => {
              setMaxSessions(event.target.value);
              setTerminalSaved(false);
            }}
          />
        </Field>
        <Field label={t("terminal.scrollbackLabel")} hint={t("terminal.scrollbackHint")}>
          <input
            type="number"
            min={16384}
            max={4194304}
            className={control}
            value={scrollback}
            placeholder="262144"
            disabled={terminalBusy}
            onChange={(event) => {
              setScrollback(event.target.value);
              setTerminalSaved(false);
            }}
          />
        </Field>
        <Button
          kind="primary"
          className="self-start"
          disabled={terminalBusy}
          onClick={() => void saveTerminal()}
        >
          {t("terminal.startSave")}
        </Button>
      </section>

      {/*
        開いている接続をまとめて閉じる。**エンジンは止めない**——止めると
        画面ごと落ちる。ここが引き受けるのは「繋ぎっぱなしを片付けたい」
        という用であり、それはセッションを閉じれば済む。転送も agent の
        貸し出しも一緒に終わる。

        取り消しは開き直すことである。だから確認は挟まない——**問いを挟んで
        いいのは、押し戻せない操作だけである。**
      */}
      {consoles === undefined ? null : (
        <section aria-label={t("desktop.closeAllHeading")} className={sectionCard}>
          <h3 className={sectionHeading}>{t("desktop.closeAllHeading")}</h3>
          <p className={hintText}>{t("desktop.closeAllNote")}</p>
          <p role="status" className="text-sm text-ink-muted">
            {t("desktop.openCount", { count: consoles.sessions.length })}
          </p>
          <Button
            className="self-start"
            disabled={consoles.busy || consoles.sessions.length === 0}
            onClick={() => void consoles.closeAll()}
          >
            {t("desktop.closeAll")}
          </Button>
        </section>
      )}

      <section aria-label={t("secrets.changeHeading")} className={sectionCard}>
        <h3 className={sectionHeading}>{t("secrets.changeHeading")}</h3>
        <p className={hintText}>{t("secrets.changeNote")}</p>
        {masterError === "" ? null : <Notice tone="danger">{masterError}</Notice>}
        {changed === "" ? null : <p role="status" className="text-sm text-live">{changed}</p>}
        <PasswordField
          label={t("secrets.currentMaster")}
          value={currentMaster}
          onChange={setCurrentMaster}
          disabled={masterBusy}
        />
        <PasswordField
          label={t("secrets.newMaster")}
          value={nextMaster}
          onChange={setNextMaster}
          disabled={masterBusy}
        />
        <PasswordField
          label={t("secrets.confirmMaster")}
          value={confirmMaster}
          onChange={setConfirmMaster}
          disabled={masterBusy}
        />
        <button
          type="button"
          className={`self-start ${primaryAction}`}
          disabled={!canChangeMaster}
          onClick={() => void changeMaster()}
        >
          {t("secrets.change")}
        </button>
      </section>
    </div>
  );
}
