import { useState } from "react";
import { useTranslate } from "../i18n/context";

// 特殊キーが立てる制御列。ここに無いラベルは、その文字そのものを送る。
const sequences: Record<string, string> = {
  Esc: "\x1b",
  Tab: "\t",
  "↑": "\x1b[A",
  "↓": "\x1b[B",
  "→": "\x1b[C",
  "←": "\x1b[D",
};

/**
 * encodeKey は、押されたキーと修飾から、端末へ送るバイト列を組み立てる。
 *
 * **Ctrl が効かない文字はそのまま送る。** 制御文字を持たない文字に Ctrl を
 * 乗せて何も送らないより、押した文字が出る方がよい——触れる画面では、何も
 * 起きないことと修飾が外れていないことが見分けられない。
 */
export function encodeKey(label: string, ctrl: boolean, alt: boolean): string {
  const sequence = sequences[label];
  if (sequence !== undefined) return alt ? "\x1b" + sequence : sequence;

  let body = label;
  if (ctrl) {
    const code = label.toLowerCase().charCodeAt(0);
    // 制御文字を持つのは a–z だけを見る。@ から _ までの記号も本来は範囲に
    // 入るが、この一覧にそれらは無い——並べていない文字のために分岐を持たない。
    if (code >= 97 && code <= 122) body = String.fromCharCode(code - 96);
  }
  return alt ? "\x1b" + body : body;
}

// 触れる画面のソフトキーボードから遠いものだけを並べる。英数字は元から出て
// いるので、ここに要るのは制御キーと、シェルで打つ記号である。
const keys = ["Esc", "Tab", "↑", "↓", "←", "→", "|", "-", "~", "/"];

const keyShape =
  "min-h-11 min-w-11 shrink-0 rounded-md border border-control-line px-3 text-sm text-ink";

/**
 * KeyBar は、物理キーボードの無い端末に Esc と Ctrl を与える。
 *
 * **狭い画面にだけ出す。** これが無いと、触れる画面のターミナルで打てるのは
 * 制御文字を要らないコマンドだけになる——走っているものを止める手段が無い。
 */
export function KeyBar({ onSend }: { onSend: (data: string) => void }) {
  const t = useTranslate();
  const [ctrl, setCtrl] = useState(false);
  const [alt, setAlt] = useState(false);

  // **修飾は 1 打鍵で降りる。** 押しっぱなしになる修飾は、次に打った一文字が
  // 何になるか分からない端末を作る。
  function send(label: string) {
    onSend(encodeKey(label, ctrl, alt));
    setCtrl(false);
    setAlt(false);
  }

  return (
    <div
      aria-label={t("terminal.keyBar")}
      className="flex shrink-0 gap-1 overflow-x-auto border-t border-line bg-toolbar p-1 md:hidden"
    >
      <button
        type="button"
        aria-pressed={ctrl}
        onClick={() => setCtrl((on) => !on)}
        className={`${keyShape} ${ctrl ? "bg-select-fill" : "bg-card"}`}
      >
        Ctrl
      </button>
      <button
        type="button"
        aria-pressed={alt}
        onClick={() => setAlt((on) => !on)}
        className={`${keyShape} ${alt ? "bg-select-fill" : "bg-card"}`}
      >
        Alt
      </button>
      {keys.map((label) => (
        <button key={label} type="button" onClick={() => send(label)} className={`${keyShape} bg-card`}>
          {label}
        </button>
      ))}
    </div>
  );
}
