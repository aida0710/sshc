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
 * encodeKey は、押されたものと立っている修飾から、端末へ送るバイト列を作る。
 *
 * **入口は 1 つである。** キーバーのボタンも、ソフトキーボードから打たれた
 * 一文字も、ここを通る。バーの上に英字キーは無いので、Ctrl を押した次に来る
 * のは常にシステムのキーボードからの一文字である——そこに乗らない修飾は、
 * 何も修飾しない。
 *
 * **Ctrl が効かない文字はそのまま送る。** 制御文字を持たない文字に Ctrl を
 * 乗せて何も送らないより、押した文字が出る方がよい。触れる画面では、何も
 * 起きないことと修飾が外れていないことが見分けられない。
 */
export function encodeKey(label: string, ctrl: boolean, alt: boolean): string {
  const sequence = sequences[label];
  if (sequence !== undefined) return alt ? "\x1b" + sequence : sequence;

  // 1 文字でないものは、貼り付けか、キーボードが既に組み立てた制御列である。
  // **修飾を乗せない** —— 乗せれば、貼り付けた最初の一文字だけが制御文字に化ける。
  if (label.length !== 1) return label;

  let body = label;
  if (ctrl) {
    const code = label.toLowerCase().charCodeAt(0);
    // 制御文字を持つのは a–z だけを見る。@ から _ までの記号も本来は範囲に
    // 入るが、それを打つ人は Ctrl を押していない。
    if (code >= 97 && code <= 122) body = String.fromCharCode(code - 96);
  }
  return alt ? "\x1b" + body : body;
}

// 触れる画面のソフトキーボードから遠いものだけを並べる。英数字は元から出て
// いるので、ここに要るのは制御キーと、シェルで打つ記号である。
const keys = ["Esc", "Tab", "↑", "↓", "←", "→", "|", "-", "~", "/"];

const keyShape =
  "min-h-11 min-w-11 shrink-0 rounded-md border border-control-line px-3 text-sm text-ink";

export type Modifiers = { ctrl: boolean; alt: boolean };

/**
 * KeyBar は、物理キーボードの無い端末に Esc と Ctrl を与える。
 *
 * **状態は持たない。** 修飾は打鍵の経路と同じ場所——TerminalView——に住む。
 * ここに持たせると、システムのキーボードから来た一文字にそれを乗せられない。
 *
 * **狭い画面にだけ出す。** これが無いと、触れる画面のターミナルで打てるのは
 * 制御文字を要らないコマンドだけになる——走っているものを止める手段が無い。
 */
export function KeyBar({
  modifiers,
  onToggle,
  onKey,
}: {
  modifiers: Modifiers;
  onToggle: (name: keyof Modifiers) => void;
  onKey: (label: string) => void;
}) {
  const t = useTranslate();
  return (
    <div
      aria-label={t("terminal.keyBar")}
      className="flex shrink-0 gap-1 overflow-x-auto border-t border-line bg-toolbar p-1 md:hidden"
    >
      <button
        type="button"
        aria-pressed={modifiers.ctrl}
        onClick={() => onToggle("ctrl")}
        className={`${keyShape} ${modifiers.ctrl ? "bg-select-fill" : "bg-card"}`}
      >
        Ctrl
      </button>
      <button
        type="button"
        aria-pressed={modifiers.alt}
        onClick={() => onToggle("alt")}
        className={`${keyShape} ${modifiers.alt ? "bg-select-fill" : "bg-card"}`}
      >
        Alt
      </button>
      {keys.map((label) => (
        <button
          key={label}
          type="button"
          onClick={() => onKey(label)}
          className={`${keyShape} bg-card`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}
