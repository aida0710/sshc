import { useTranslate } from "../i18n/context";
import { Button } from "../ui/surface";

// 端末の中身を、選べる形で出す面。
//
// **xterm の上では選べない。** 選択はマウスのためのもので、指には押して・
// 引いて・離すが無い。OS が持っている長押しからの選択も、xterm が mousedown で
// 無条件に preventDefault を呼ぶために始まらない。
//
// だからここは xterm を一切使わない。ただの pre に同じ文字を置き、選択も、
// ハンドルも、コピーの吹き出しも OS に任せる。**作り直せるものではないし、
// 作り直したものは必ず OS のものより下手である。**
export function SelectSheet({ text, onClose }: { text: string; onClose: () => void }) {
  const t = useTranslate();
  return (
    <section
      aria-label={t("terminal.selectHeading")}
      className="absolute inset-0 z-10 flex flex-col bg-canvas"
    >
      <div className="flex shrink-0 items-center justify-between gap-3 border-b border-line px-3 py-2">
        <p className="min-w-0 text-sm text-ink-muted">{t("terminal.selectHint")}</p>
        <Button className="shrink-0" onClick={onClose}>
          {t("terminal.selectClose")}
        </Button>
      </div>
      {/*
        user-select はここで明示する。xterm の CSS がその上の階層で none を
        掛けており、これはその中に描かれる。
      */}
      <pre
        className="min-h-0 flex-1 overflow-auto p-3 font-mono text-xs leading-5 text-ink"
        style={{ userSelect: "text", WebkitUserSelect: "text", whiteSpace: "pre-wrap", wordBreak: "break-word" }}
      >
        {text}
      </pre>
    </section>
  );
}
