import { useCallback, useEffect, useRef, useState } from "react";
import { failureCode } from "../api/client";
import { integrationsApi, type IntegrationsApi, type TerminalBackground } from "../api/integrations";
import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";
import { control } from "../ui/form";
import { useBackgroundImage } from "./backgroundImage";

// 背景の画像を選び、持ち込み、捨てる。
//
// **画像の置き場はひとつである。** 全体の設定でも接続の設定でも、選ぶ相手は
// 同じ一覧である——接続ごとに別の置き場を持つと、同じ写真が何枚も溜まる。
// 違うのは「どれを選んだか」だけなので、選択だけを外から受け取る。

type BackgroundPickerProps = {
  value: string;
  onChange: (next: string) => void;
  tint: number | undefined;
  onTintChange: (next: number | undefined) => void;
  /** 何も選ばなかったときに何が起きるかを言う綴り。 */
  unchosen: string;
  api?: Pick<IntegrationsApi, "terminalBackgrounds" | "addTerminalBackground" | "deleteTerminalBackground">;
};

export function BackgroundPicker({
  value,
  onChange,
  tint,
  onTintChange,
  unchosen,
  api = integrationsApi,
}: BackgroundPickerProps) {
  const t = useTranslate();
  const [stored, setStored] = useState<TerminalBackground[]>([]);
  const [remaining, setRemaining] = useState(0);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const chooser = useRef<HTMLInputElement>(null);

  const reload = useCallback(async () => {
    const listed = await api.terminalBackgrounds();
    setStored(listed.backgrounds);
    setRemaining(listed.remainingBytes);
  }, [api]);

  useEffect(() => {
    void reload().catch(() => undefined);
  }, [reload]);

  // **断られた理由はサーバーが名指しする。** 「保存できません」で終わらせない
  // ——直すのは人であり、直すには何が悪いのかが要る。
  async function add(file: File) {
    setBusy(true);
    setProblem("");
    try {
      const added = await api.addTerminalBackground(file.name, file);
      await reload();
      onChange(added.name);
    } catch (error) {
      const code = failureCode(error);
      setProblem(
        code === "background_too_large"
          ? t("terminal.backgroundTooLarge")
          : code === "backgrounds_full"
            ? t("terminal.backgroundsFull")
            : code === "not_an_image"
              ? t("terminal.backgroundNotAnImage")
              : t("terminal.backgroundFailed"),
      );
    } finally {
      setBusy(false);
    }
  }

  async function drop(name: string) {
    setBusy(true);
    setProblem("");
    try {
      await api.deleteTerminalBackground(name);
      // **捨てた画像を選んだままにしない。** 名前だけが残ると、端末は
      // 「選ばれているのに何も出ない」状態になる。
      if (value === name) onChange("");
      await reload();
    } catch {
      setProblem(t("terminal.backgroundFailed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex flex-col gap-2">
      <select className={control} value={value} onChange={(event) => onChange(event.target.value)}>
        <option value="">{unchosen}</option>
        {stored.map((background) => (
          <option key={background.name} value={background.name}>
            {background.name}
          </option>
        ))}
      </select>

      {stored.length === 0 ? null : (
        <ul className="flex flex-wrap gap-2">
          {stored.map((background) => (
            <li key={background.name} className="flex flex-col items-start gap-1">
              <Thumbnail name={background.name} chosen={value === background.name} />
              <Button onClick={() => void drop(background.name)} disabled={busy}>
                {t("terminal.backgroundRemove", { name: background.name })}
              </Button>
            </li>
          ))}
        </ul>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <input
          ref={chooser}
          type="file"
          className="hidden"
          accept="image/png,image/jpeg,image/webp,image/gif"
          onChange={(event) => {
            const file = event.target.files?.[0];
            // **同じ写真をもう一度選べるようにする。** 値を残すと、二度目の
            // 選択で change が鳴らない。
            event.target.value = "";
            if (file !== undefined) void add(file);
          }}
        />
        <Button onClick={() => chooser.current?.click()} disabled={busy}>
          {t("terminal.backgroundAdd")}
        </Button>
        <span className="text-xs text-ink-faint">
          {t("terminal.backgroundRoom", { megabytes: String(Math.max(0, Math.round(remaining / (1 << 20)))) })}
        </span>
      </div>

      {problem === "" ? null : (
        <p role="alert" className="text-xs text-danger">
          {problem}
        </p>
      )}

      {value === "" ? null : (
        <label className="flex flex-col gap-1">
          <span className="text-xs text-ink-muted">
            {t("terminal.tintLabel")} — {tint ?? ""}
          </span>
          <input
            type="range"
            min={0}
            max={100}
            step={5}
            value={tint ?? 55}
            onChange={(event) => onTintChange(Number(event.target.value))}
          />
          <span className="text-xs text-ink-faint">{t("terminal.tintHint")}</span>
        </label>
      )}
    </div>
  );
}

// Thumbnail は 1 枚の見本である。**綴りは JS が取りに行く**ので、素の
// <img src> では出せない——読み取りにも CSRF トークンが要る。
function Thumbnail({ name, chosen }: { name: string; chosen: boolean }) {
  const url = useBackgroundImage(name);
  if (url === "") return <div className="h-16 w-24 rounded border border-control-line bg-control" />;
  return (
    <img
      src={url}
      alt={name}
      className={`h-16 w-24 rounded border object-cover ${chosen ? "border-accent" : "border-control-line"}`}
    />
  );
}
