import { useEffect, useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type TerminalSettings } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { PasswordField } from "../ui/PasswordField";
import { CheckboxField, Field, control, hintText, sectionCard, sectionHeading } from "../ui/form";
import { PageHeader } from "../ui/page";
import { Button, Notice } from "../ui/surface";
import type { TerminalSessionsState } from "../terminal/sessions";
import { ConfirmDialog } from "../ui/ConfirmDialog";
import { AppearancePicker } from "../terminal/AppearancePicker";
import { BackgroundPicker } from "../terminal/BackgroundPicker";
import { appearanceOf } from "../terminal/appearance";
import { fonts } from "../terminal/fonts";
import { palettes } from "../terminal/palettes";

type SettingsPanelProps = {
  api?: IntegrationsApi;
  onTerminalSettingsChange?: (settings: TerminalSettings) => void;
  // 開いているセッションはシェルが持つ。ここはその数を見せ、まとめて
  // 閉じる入口を出すだけである。
  consoles?: Pick<TerminalSessionsState, "sessions" | "busy" | "closeAll">;
};

export function SettingsPanel({ api = integrationsApi, consoles, onTerminalSettingsChange }: SettingsPanelProps) {
  const t = useTranslate();
  // **走っている本数だけを数える。** 既に終わった行を片付ける回に問いは要らない。
  const liveConsoles = (consoles?.sessions ?? []).filter((session) => session.exited === undefined).length;
  const [confirmingCloseAll, setConfirmingCloseAll] = useState(false);
  const [currentMaster, setCurrentMaster] = useState("");
  const [nextMaster, setNextMaster] = useState("");
  const [confirmMaster, setConfirmMaster] = useState("");
  const [masterBusy, setMasterBusy] = useState(false);
  const [masterError, setMasterError] = useState("");
  const [changed, setChanged] = useState("");
  // 3 つとも文字列で持つ。**空は「設定されていない」であり、0 ではない。**
  // 数として持つと、空欄と 0 を区別する場所をもう一つ作ることになる。
  const [startDirectory, setStartDirectory] = useState("");
  const [maxSessions, setMaxSessions] = useState("");
  const [scrollback, setScrollback] = useState("");
  const [fontSize, setFontSize] = useState("");
  // 空は「選んでいない」であり、テーマに従うという意味である。
  // engine の設定。**端末のものとは別に保存される。**
  const [port, setPort] = useState("");
  const [portBusy, setPortBusy] = useState(false);
  const [portError, setPortError] = useState("");
  const [portSaved, setPortSaved] = useState(false);
  // 0 は無言。1〜3 が ssh の -v / -vv / -vvv に対応する。
  const [verbosity, setVerbosity] = useState("0");
  // **空文字は「決めていない」である。** "0" は「繋ぎ直さない」という選択なので、
  // 同じ入れ物で両方を表すには、選ばれていない状態を別の値で持つしかない。
  const [reconnect, setReconnect] = useState("");
  const [palette, setPalette] = useState("");
  const [font, setFont] = useState("");
  const [background, setBackground] = useState("");
  // undefined は「選んでいない」。0 は「かぶせない」という選択である。
  const [tint, setTint] = useState<number | undefined>(undefined);
  const [copyOnSelect, setCopyOnSelect] = useState(true);
  const [rightClickPaste, setRightClickPaste] = useState(true);
  const [terminalBusy, setTerminalBusy] = useState(false);
  const [terminalError, setTerminalError] = useState("");
  const [terminalSaved, setTerminalSaved] = useState(false);

  useEffect(() => {
    let active = true;
    void api.terminalSettings()
      .then((settings) => {
        if (!active) return;
        setStartDirectory(settings.startDirectory ?? "");
        setMaxSessions(settings.maxSessions === undefined ? "" : String(settings.maxSessions));
        setScrollback(settings.scrollbackBytes === undefined ? "" : String(settings.scrollbackBytes));
        setFontSize(settings.fontSize === undefined ? "" : String(settings.fontSize));
        setVerbosity(String(settings.verbosity ?? 0));
        setReconnect(settings.reconnect === undefined ? "" : String(settings.reconnect));
        setPalette(settings.appearance?.palette ?? "");
        setFont(settings.appearance?.font ?? "");
        setBackground(settings.appearance?.background ?? "");
        setTint(settings.appearance?.backgroundTint);
        setCopyOnSelect(settings.copyOnSelect ?? true);
        setRightClickPaste(settings.rightClickPaste ?? true);
      })
      .catch(() => undefined);
    void api
      .engineSettings()
      .then((settings) => {
        if (active) setPort(settings.port === undefined ? "" : String(settings.port));
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [api]);

  // **受け口の番号は起動時にしか読まれない。** 保存できたことと効いたことは
  // 別なので、画面はそう言う。
  async function savePort() {
    const trimmed = port.trim();
    const chosen = trimmed === "" ? undefined : Number(trimmed);
    if (chosen !== undefined && (!Number.isSafeInteger(chosen) || chosen < 1024 || chosen > 65535)) {
      setPortError(t("engine.portOutOfRange"));
      setPortSaved(false);
      return;
    }
    setPortBusy(true);
    setPortError("");
    setPortSaved(false);
    try {
      await api.setEngineSettings(chosen === undefined ? {} : { port: chosen });
      setPortSaved(true);
    } catch {
      setPortError(t("engine.saveFailed"));
    } finally {
      setPortBusy(false);
    }
  }

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
    const size = numberOr(fontSize);
    if (Number.isNaN(sessions) || Number.isNaN(bytes) || Number.isNaN(size)) {
      setTerminalError(t("terminal.limitsOutOfRange"));
      setTerminalSaved(false);
      return;
    }

    setTerminalBusy(true);
    setTerminalError("");
    setTerminalSaved(false);
    try {
      const directory = startDirectory.trim();
      const next: TerminalSettings = {
        ...(directory === "" ? {} : { startDirectory: directory }),
        ...(sessions === undefined ? {} : { maxSessions: sessions }),
        ...(bytes === undefined ? {} : { scrollbackBytes: bytes }),
        ...(size === undefined ? {} : { fontSize: size }),
        // 0 は既定なので書かない。書けば metadata に「無言を明示的に選んだ」が残る。
        ...(verbosity === "0" ? {} : { verbosity: Number(verbosity) }),
        ...(reconnect === "" ? {} : { reconnect: Number(reconnect) }),
        // on は既定なので書かない。off だけを false として明示し、再読み込み
        // しても消えないようにする。
        ...(copyOnSelect ? {} : { copyOnSelect: false }),
        ...(rightClickPaste ? {} : { rightClickPaste: false }),
        // **選んでいないなら節ごと送らない。** 空の appearance を送ると、
        // metadata に何も言っていない節が残る。
        ...(appearanceOf({ palette, font, background, tint })),
      };
      await api.setTerminalSettings(next);
      onTerminalSettingsChange?.(next);
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
                : "terminal.settingsSaveFailed",
      ));
    } finally {
      setTerminalBusy(false);
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

      {/*
        埋め込みターミナルの設定。**空欄は「設定されていない」であり、既定と
        同じ値ではない。** 既定を metadata へ書き戻すと、既定を変えた日に
        その人だけが黙って取り残される。だから空欄は空欄のまま送らない。
      */}
      {/*
        engine そのものの設定。**端末のものではない**ので節を分ける。ここを
        変えても、次に engine を起こすまで効かない。
      */}
      <section aria-label={t("engine.heading")} className={sectionCard}>
        <h3 className={sectionHeading}>{t("engine.heading")}</h3>
        {portError === "" ? null : <Notice tone="danger">{portError}</Notice>}
        {!portSaved ? null : <Notice tone="notice">{t("engine.saved")}</Notice>}
        <Field label={t("engine.portLabel")} hint={t("engine.portHint")}>
          <input
            type="number"
            min={1024}
            max={65535}
            className={control}
            value={port}
            placeholder="30000-60000"
            disabled={portBusy}
            onChange={(event) => {
              setPort(event.target.value);
              setPortSaved(false);
            }}
          />
        </Field>
        <div className="flex justify-end">
          <Button kind="primary" disabled={portBusy} onClick={() => void savePort()}>
            {t("terminal.startSave")}
          </Button>
        </div>
      </section>

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
        {/*
          既定は画面の幅で決まる。**空欄は「決めていない」であって「既定と同じ
          数字」ではない**ので、置くのは placeholder だけである。書けば、その
          日から既定を変えてもこの人だけが取り残される。
        */}
        <Field label={t("terminal.verbosityLabel")} hint={t("terminal.verbosityHint")}>
          <select
            className={control}
            value={verbosity}
            disabled={terminalBusy}
            onChange={(event) => {
              setVerbosity(event.target.value);
              setTerminalSaved(false);
            }}
          >
            <option value="0">{t("terminal.verbosityQuiet")}</option>
            <option value="1">{t("terminal.verbosityBrief")}</option>
            <option value="2">{t("terminal.verbosityDetailed")}</option>
            <option value="3">{t("terminal.verbosityFull")}</option>
          </select>
        </Field>
        {/*
          **閉じたはずのコンソールがしばらく残って見えるのは、ここが粘って
          いるあいだである。** 既定は諦めるまで数十秒あり、切りたい人には
          切る道が要る。
        */}
        <Field label={t("terminal.reconnectLabel")} hint={t("terminal.reconnectHint")}>
          <select
            className={control}
            value={reconnect}
            disabled={terminalBusy}
            onChange={(event) => {
              setReconnect(event.target.value);
              setTerminalSaved(false);
            }}
          >
            <option value="">{t("terminal.reconnectDefault")}</option>
            <option value="0">{t("terminal.reconnectNever")}</option>
            <option value="1">{t("terminal.reconnectOnce")}</option>
            <option value="2">{t("terminal.reconnectTwice")}</option>
            <option value="3">{t("terminal.reconnectThrice")}</option>
            <option value="5">{t("terminal.reconnectFive")}</option>
          </select>
        </Field>
        <Field label={t("terminal.paletteLabel")} hint={t("terminal.paletteHint")}>
          <AppearancePicker
            choices={palettes}
            value={palette}
            onChange={setPalette}
            unchosen={t("terminal.paletteFollowsTheme")}
          />
        </Field>
        <Field label={t("terminal.fontLabel")} hint={t("terminal.fontHint")}>
          <AppearancePicker
            choices={fonts}
            value={font}
            onChange={setFont}
            unchosen={t("terminal.fontFollowsSystem")}
          />
        </Field>
        <Field label={t("terminal.backgroundLabel")} hint={t("terminal.backgroundHint")}>
          <BackgroundPicker
            value={background}
            onChange={setBackground}
            tint={tint}
            onTintChange={setTint}
            unchosen={t("terminal.backgroundNone")}
          />
        </Field>
        <Field label={t("terminal.fontSizeLabel")} hint={t("terminal.fontSizeHint")}>
          <input
            type="number"
            min={8}
            max={32}
            className={control}
            value={fontSize}
            placeholder="15 / 13"
            disabled={terminalBusy}
            onChange={(event) => {
              setFontSize(event.target.value);
              setTerminalSaved(false);
            }}
          />
        </Field>
        <div className="flex flex-col gap-1">
          <CheckboxField
            label={t("terminal.copyOnSelectLabel")}
            checked={copyOnSelect}
            disabled={terminalBusy}
            onChange={(checked) => {
              setCopyOnSelect(checked);
              setTerminalSaved(false);
            }}
          />
          <p className={hintText}>{t("terminal.copyOnSelectHint")}</p>
        </div>
        <div className="flex flex-col gap-1">
          <CheckboxField
            label={t("terminal.rightClickPasteLabel")}
            checked={rightClickPaste}
            disabled={terminalBusy}
            onChange={(checked) => {
              setRightClickPaste(checked);
              setTerminalSaved(false);
            }}
          />
          <p className={hintText}>{t("terminal.rightClickPasteHint")}</p>
        </div>
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

        **規則は「問いを挟んでいいのは押し戻せない操作だけ」である。** ここは
        長らくその「押し戻せる」側に置かれていた——開き直せば済む、と。

        **開き直しは取り消しではない。** 得られるのは同じ相手への*新しい*
        セッションであり、動いていたもの——編集中のファイル、走っているビルド、
        追っているログ——は戻らない。ここが終わらせるのは、それを一度に全部で
        ある。だから訊く。

        **走っているものが 1 本も無ければ訊かない。** 終わった行を片付けるだけ
        の回に問いを出せば、次の問いも読まずに押す習慣を作る。
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
            onClick={() =>
              liveConsoles === 0 ? void consoles.closeAll() : setConfirmingCloseAll(true)
            }
          >
            {t("desktop.closeAll")}
          </Button>
          {confirmingCloseAll ? (
            <ConfirmDialog
              id="close-all-consoles-heading"
              heading={t("desktop.closeAllHeading2", { count: String(liveConsoles) })}
              body={<p className="text-sm text-ink-muted">{t("desktop.closeAllBody")}</p>}
              confirmLabel={t("desktop.closeAllConfirm")}
              cancelLabel={t("desktop.closeAllCancel")}
              onCancel={() => setConfirmingCloseAll(false)}
              onConfirm={() => {
                setConfirmingCloseAll(false);
                void consoles.closeAll();
              }}
            />
          ) : null}
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
        <Button kind="primary" className="self-start"
          disabled={!canChangeMaster}
          onClick={() => void changeMaster()}
        >
          {t("secrets.change")}
        </Button>
      </section>
    </div>
  );
}
