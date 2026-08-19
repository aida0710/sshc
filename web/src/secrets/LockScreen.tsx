import { useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { PasswordField } from "../ui/PasswordField";
import { Button, Notice } from "../ui/surface";

type LockScreenProps = {
  // exists は、この画面が行う 2 つのことを区別する。両者は同じに
  // 見えて違う: 一方は復元できない vault を作り、もう一方は
  // 既にすべてを保持しているものを開く。
  exists: boolean;
  // biometric は、この端末が指紋で開けるかどうかである。**開いた瞬間に自動で
  // 尋ねる**ので、押す前に決まっていなければならない。
  biometric?: boolean;
  onOpen: () => void;
  api?: IntegrationsApi;
};

// 入り口。
//
// マスターパスワードはもはや画面ごとのアンロックではない。他の
// 何に到達するよりも前に尋ねられる。すべての世代バックアップが
// それで封印されているからだ: vault が閉じている間に起きた書き込みは、
// 平文でコピーを残すか、コピーを一切残さないかのどちらかになり、
// どちらになるかは、ファイルを編集している間誰も意識していない状態次第だった。
export function LockScreen({ exists, biometric = false, onOpen, api = integrationsApi }: LockScreenProps) {
  const t = useTranslate();
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // **一度しか自分からは尋ねない。** 断った人に、画面が繰り返し出し直すのは
  // 拒否を受け取っていないことである。二度目からは、押したときだけ出す。
  const asked = useRef(false);

  async function prove() {
    setBusy(true);
    setError("");
    try {
      await api.unlockWithBiometric();
      onOpen();
    } catch (caught) {
      const code = failureCode(caught);
      // **断られたことは失敗ではない。** パスワードの欄はそこに在るので、
      // 赤い帯を出さずに黙って戻る。
      if (code !== "biometric_refused") {
        setError(code === "biometric_not_enabled" ? t("lock.biometricGone") : t("lock.biometricFailed"));
      }
    } finally {
      setBusy(false);
    }
  }

  useEffect(() => {
    if (!exists || !biometric || asked.current) return;
    asked.current = true;
    void prove();
    // prove は毎レンダー新しくなるが、見張っているのは「預けてあるか」だけである。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [exists, biometric]);

  const minimum = 12;
  const tooShort = password.length < minimum;
  const mismatched = !exists && confirmation !== password;

  async function submit() {
    setBusy(true);
    setError("");
    try {
      if (exists) {
        await api.unlockVault(password);
      } else {
        await api.initialiseVault(password);
      }
      setPassword("");
      setConfirmation("");
      onOpen();
    } catch (caught) {
      const code = failureCode(caught);
      setError(
        code === "wrong_passphrase"
          ? t("lock.wrong")
          : code === "passphrase_too_short"
            ? t("lock.tooShort", { count: minimum })
            : t("lock.failed"),
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="mx-auto flex min-h-screen max-w-md flex-col justify-center gap-4 p-6">
      <h1 className="text-lg font-medium">{t("shell.title")}</h1>
      <p className={hintText}>{exists ? t("lock.explainOpen") : t("lock.explainNew")}</p>
      {/*
        パスワードを選ぶその場で伝え、後からは伝えない。誰も持っていない
        パスワードから導出された鍵に、復元の経路はなく、それを初めて
        入力する人こそが、それについて行動できる唯一の人物だ。
      */}
      {exists ? null : <p className="text-sm text-notice-ink">{t("lock.noRecovery")}</p>}
      {error === "" ? null : (
        <Notice tone="danger">{error}</Notice>
      )}

      <form
        className="flex flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          void submit();
        }}
      >
        <PasswordField label={t("lock.password")} value={password} onChange={setPassword} autoFocus />
        {exists ? null : (
          <PasswordField label={t("lock.confirm")} value={confirmation} onChange={setConfirmation} />
        )}
        <div className="flex flex-wrap items-center gap-2">
          <Button
            kind="primary"
            type="submit"
            disabled={busy || tooShort || mismatched}
          >
            {exists ? t("lock.open") : t("lock.create")}
          </Button>
          {/* 断られたあとでも、押せば何度でも試せる。 */}
          {exists && biometric ? (
            <button
              type="button"
              disabled={busy}
              onClick={() => void prove()}
              className="rounded border border-line px-3 py-1.5 text-sm text-ink"
            >
              {t("lock.biometric")}
            </button>
          ) : null}
        </div>
      </form>
    </main>
  );
}
