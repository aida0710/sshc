import { useRef, useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import { Icon } from "../ui/icons";
import { Field, control } from "../ui/form";
import { Button } from "../ui/surface";

// VPN プロファイルの設定ファイル（OpenVPN の .ovpn、WireGuard の設定ファイル）の欄。
// 貼り付けるか、［ファイルを選択］で読み込む。読み取った中身の要約（サーバーなど）は
// summary として、ボタンの横に並べる。

// configRows は、設定ファイルの欄の高さ（行数）である。節の始まりと主な項目が見える程度にする。
const configRows = 10;

export function ConfigFileField({
  label,
  hint,
  error,
  value,
  accept,
  placeholder,
  summary,
  onChange,
}: {
  label: string;
  hint: string;
  // error は、設定ファイルを受け取れない理由である。
  error: string | undefined;
  value: string;
  // accept は、ファイルを選ぶ画面で絞り込む拡張子である。
  accept: string;
  // placeholder は、空のときに見せる書き方の例である。
  placeholder?: string;
  summary?: ReactNode;
  onChange: (value: string) => void;
}) {
  const t = useTranslate();
  const chooser = useRef<HTMLInputElement>(null);
  const [readFailed, setReadFailed] = useState(false);

  function edit(next: string) {
    setReadFailed(false);
    onChange(next);
  }

  async function load(file: File) {
    try {
      edit(await file.text());
    } catch {
      setReadFailed(true);
    }
  }

  return (
    <div className="flex flex-col gap-2 sm:col-span-2">
      <Field label={label} hint={hint} error={readFailed ? t("vpn.configFileReadFailed") : error}>
        <textarea
          className={`${control} font-mono text-xs`}
          rows={configRows}
          spellCheck={false}
          autoComplete="off"
          value={value}
          {...(placeholder === undefined ? {} : { placeholder })}
          onChange={(event) => edit(event.target.value)}
        />
      </Field>
      <div className="flex flex-wrap items-center gap-3">
        <input
          ref={chooser}
          type="file"
          className="hidden"
          accept={accept}
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file !== undefined) void load(file);
          }}
        />
        <Button className="inline-flex items-center gap-1.5" onClick={() => chooser.current?.click()}>
          <Icon name="openFile" />
          {t("vpn.configFileChoose")}
        </Button>
        {summary}
      </div>
    </div>
  );
}
