import { useRef, useState, type ReactNode } from "react";
import { useTranslate } from "../i18n/context";
import type { DescribableControlProps } from "../ui/describeControl";
import { Icon } from "../ui/icons";
import { control } from "../ui/form";
import { FieldRow } from "../ui/FieldRow";
import { Button } from "../ui/surface";

// VPN プロファイルの設定ファイル（OpenVPN の .ovpn、WireGuard の設定ファイル）の欄。
// 貼り付けるか、［ファイルを選択］で読み込む。読み取った中身の要約（サーバーなど）は
// summary として、ボタンの横に並べる。

// configRows は、設定ファイルの欄の高さ（行数）である。WireGuard の例（[Interface] と [Peer]）が
// 収まる程度にする。
const configRows = 9;

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

  // 欄と［ファイルを選択］を1つの入力として並べ、説明はその下に出す（試作の順）。
  return (
    <FieldRow label={label} hint={hint} warning={readFailed ? t("vpn.configFileReadFailed") : error} interactiveChildren>
      <ConfigFileInput
        label={label}
        value={value}
        accept={accept}
        {...(placeholder === undefined ? {} : { placeholder })}
        summary={summary}
        onEdit={edit}
        onLoad={(file) => void load(file)}
      />
    </FieldRow>
  );
}

// ConfigFileInput は、設定ファイルの欄と、ファイルを選ぶボタンと要約である。FieldRow が結び付ける
// 説明の id は、中の欄へ渡す。
function ConfigFileInput({
  label,
  value,
  accept,
  placeholder,
  summary,
  onEdit,
  onLoad,
  "aria-describedby": describedBy,
  "aria-invalid": invalid,
}: {
  label: string;
  value: string;
  accept: string;
  placeholder?: string;
  summary: ReactNode;
  onEdit: (value: string) => void;
  onLoad: (file: File) => void;
} & DescribableControlProps) {
  const t = useTranslate();
  const chooser = useRef<HTMLInputElement>(null);
  return (
    <span className="flex min-w-0 flex-col gap-2">
      {/* 文字の大きさは、control の大きさより後に効くよう sm: で付ける（狭い画面は index.css が16pxにする）。 */}
      <textarea
        aria-label={label}
        aria-describedby={describedBy}
        aria-invalid={invalid}
        className={`${control} font-mono leading-5 sm:text-[12.5px]`}
        rows={configRows}
        spellCheck={false}
        autoComplete="off"
        value={value}
        {...(placeholder === undefined ? {} : { placeholder })}
        onChange={(event) => onEdit(event.target.value)}
      />
      <span className="flex flex-wrap items-center gap-3">
        <input
          ref={chooser}
          type="file"
          className="hidden"
          accept={accept}
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file !== undefined) onLoad(file);
          }}
        />
        <Button className="inline-flex items-center gap-1.5" onClick={() => chooser.current?.click()}>
          <Icon name="openFile" />
          {t("vpn.configFileChoose")}
        </Button>
        {summary}
      </span>
    </span>
  );
}
