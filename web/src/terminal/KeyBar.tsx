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
 * applyModifiers は、**打たれた文字**に、立っている修飾を乗せる。
 *
 * <p>これはキーボードから届いたバイト列の道である。ラベルの表を引かない——
 * "Esc" と打った人に ESC を送ってはならない。
 *
 * **Ctrl が効かない文字はそのまま送る。** 制御文字を持たない文字に Ctrl を
 * 乗せて何も送らないより、押した文字が出る方がよい。触れる画面では、何も
 * 起きないことと修飾が外れていないことが見分けられない。
 */
export function applyModifiers(data: string, ctrl: boolean, alt: boolean): string {
  // 1 文字でないものは、貼り付けか、キーボードが既に組み立てた制御列である。
  // **修飾を乗せない** —— 乗せれば、貼り付けた最初の一文字だけが制御文字に化ける。
  if (data.length !== 1) return data;

  let body = data;
  if (ctrl) {
    const code = data.toLowerCase().charCodeAt(0);
    // 制御文字を持つのは a–z だけを見る。@ から _ までの記号も本来は範囲に
    // 入るが、それを打つ人は Ctrl を押していない。
    if (code >= 97 && code <= 122) body = String.fromCharCode(code - 96);
  }
  return alt ? "\x1b" + body : body;
}

/**
 * encodeKey は、**バーで押されたキー**を端末へ送るバイト列にする。
 *
 * <p>こちらはラベルの表を引く道である。Esc を押した人には ESC が要る。
 *
 * <p>この 2 つを 1 つの関数で兼ねていたことが、バーのキーがラベルの文字列を
 * そのまま送っていた原因だった——`Esc` を押すと端末に "Esc" と出ていた。
 * 打たれた文字と押されたキーは別のもので、同じ入口を通せない。
 */
export function encodeKey(label: string, ctrl: boolean, alt: boolean): string {
  const sequence = sequences[label];
  if (sequence !== undefined) return alt ? "\x1b" + sequence : sequence;
  return applyModifiers(label, ctrl, alt);
}

// 触れる画面のソフトキーボードから遠いものだけを並べる。英数字は元から出て
// いるので、ここに要るのは制御キーと、シェルで打つ記号である。
const keys = ["Esc", "Tab", "↑", "↓", "←", "→", "|", "-", "~", "/"];

const keyShape =
  "min-h-11 min-w-11 shrink-0 rounded-md border border-control-line px-3 text-sm text-ink";

// **押しても焦点を奪わない。** 触れる画面では、焦点が端末の入力欄から外れた
// 瞬間にソフトキーボードが閉じる。Ctrl を押すたびにキーボードが畳まれれば、
// 次の一文字を打つ場所が無い——このバーが存在する意味そのものが消える。
//
// pointerdown を止めるのが焦点の移動を止める唯一の場所である。click では遅い。
function keepFocus(event: { preventDefault(): void }) {
  event.preventDefault();
}

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
  onSelect,
}: {
  modifiers: Modifiers;
  onToggle: (name: keyof Modifiers) => void;
  onKey: (label: string) => void;
  onSelect: () => void;
}) {
  const t = useTranslate();
  return (
    <div
      aria-label={t("terminal.keyBar")}
      className="flex shrink-0 gap-1 overflow-x-auto border-t border-line bg-toolbar p-1 md:hidden"
    >
      {/*
        **選択だけはキーではない。** 端末の上では範囲を選べないので、選べる面を
        出すための入口がここに要る。並びの先頭に置くのは、キーを打っている最中に
        誤って押す位置ではないからである。
      */}
      <button
        type="button"
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        onClick={onSelect}
        className={`${keyShape} bg-card`}
      >
        {t("terminal.selectOpen")}
      </button>
      <button
        type="button"
        aria-pressed={modifiers.ctrl}
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        onClick={() => onToggle("ctrl")}
        className={`${keyShape} ${modifiers.ctrl ? "bg-select-fill" : "bg-card"}`}
      >
        Ctrl
      </button>
      <button
        type="button"
        aria-pressed={modifiers.alt}
        onPointerDown={keepFocus}
        onMouseDown={keepFocus}
        onClick={() => onToggle("alt")}
        className={`${keyShape} ${modifiers.alt ? "bg-select-fill" : "bg-card"}`}
      >
        Alt
      </button>
      {keys.map((label) => (
        <button
          key={label}
          type="button"
          onPointerDown={keepFocus}
          onMouseDown={keepFocus}
          onClick={() => onKey(label)}
          className={`${keyShape} bg-card`}
        >
          {label}
        </button>
      ))}
    </div>
  );
}
