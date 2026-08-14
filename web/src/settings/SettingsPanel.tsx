import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type LoginItem } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { CheckboxField, hintText, primaryAction, sectionCard, sectionHeading } from "../ui/form";
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
  const [loginItem, setLoginItem] = useState<LoginItem | null>(null);
  const [loginLoaded, setLoginLoaded] = useState(false);
  const [loginBusy, setLoginBusy] = useState(false);
  const [loginError, setLoginError] = useState("");
  const [currentMaster, setCurrentMaster] = useState("");
  const [nextMaster, setNextMaster] = useState("");
  const [confirmMaster, setConfirmMaster] = useState("");
  const [masterBusy, setMasterBusy] = useState(false);
  const [masterError, setMasterError] = useState("");
  const [changed, setChanged] = useState("");
  const [keepRunning, setKeepRunning] = useState(false);
  const [desktopBusy, setDesktopBusy] = useState(false);

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

  async function updateKeepRunning(next: boolean) {
    setDesktopBusy(true);
    try {
      await api.setDesktopSettings(next);
      setKeepRunning(next);
    } catch {
      setLoginError(t("desktop.saveFailed"));
    } finally {
      setDesktopBusy(false);
    }
  }

  useEffect(() => {
    let active = true;
    void api.loginItem()
      .then((loaded) => {
        if (!active) return;
        setLoginItem(loaded);
        setLoginLoaded(true);
      })
      .catch(() => {
        if (!active) return;
        setLoginItem(null);
        setLoginLoaded(true);
        setLoginError(t("login.loadFailed"));
      });
    return () => {
      active = false;
    };
  }, [api, t]);

  async function updateLoginItem(enabled: boolean) {
    setLoginBusy(true);
    setLoginError("");
    try {
      setLoginItem(await api.setLoginItem(enabled));
    } catch {
      setLoginError(t("login.failed"));
    } finally {
      setLoginBusy(false);
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

      {loginError === "" ? null : <Notice tone="danger">{loginError}</Notice>}
      {loginLoaded && loginItem?.supported ? (
        <section aria-label={t("login.heading")} className={sectionCard}>
          <h3 className={sectionHeading}>{t("login.heading")}</h3>
          <p className={hintText}>{t("login.note")}</p>
          <CheckboxField
            label={t("login.enable")}
            checked={loginItem.enabled}
            disabled={loginBusy}
            onChange={(next) => void updateLoginItem(next)}
          />
        </section>
      ) : null}

      <section aria-label={t("desktop.heading")} className={sectionCard}>
        <h3 className={sectionHeading}>{t("desktop.heading")}</h3>
        <p className={hintText}>{t("desktop.note")}</p>
        <CheckboxField
          label={t("desktop.keepRunning")}
          checked={keepRunning}
          disabled={desktopBusy}
          onChange={(next) => void updateKeepRunning(next)}
        />
        {/*
          ログイン時起動が有効なら、この選択は効かない。**二つの仕組みが同じ
          ものの寿命を決めていて、あちらの方が強い**——止めても launchd や
          systemd が起こし直す。黙って効かないより、効かないと言う。
        */}
        {loginItem?.enabled === true ? (
          <p className={hintText}>{t("desktop.loginItemWins")}</p>
        ) : null}
      </section>

      {/*
        開いている接続をまとめて閉じる。**エンジンは止めない**——止めると
        画面ごと落ちるうえ、ログイン項目が有効なら勝手に戻ってくる。ここが
        引き受けるのは「繋ぎっぱなしを片付けたい」という用であり、それは
        セッションを閉じれば済む。転送も agent の貸し出しも一緒に終わる。

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
