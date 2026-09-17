import type { CSSProperties } from "react";
import { defaultTint } from "../terminal/appearance";
import { useBackgroundImage } from "../terminal/backgroundImage";
import { fontStack } from "../terminal/fonts";

// A still terminal that shows what the chosen palette, font, size and
// background will look like before they are saved.
export function TerminalPreview({
  palette,
  font,
  background,
  tint,
  fontSize,
}: {
  palette: string;
  font: string;
  background: string;
  tint: number | undefined;
  fontSize: string;
}) {
  const backgroundURL = useBackgroundImage(background);
  const chosenSize = Number(fontSize);
  const previewSize = Number.isFinite(chosenSize) && chosenSize >= 8 && chosenSize <= 32 ? chosenSize : 13;
  const hasBackground = background !== "" && backgroundURL !== "";
  const style = {
    color: "var(--ui-term-fg)",
    fontFamily: fontStack(font),
    fontSize: previewSize,
    ...(hasBackground
      ? {
          "--ui-term-image": `url("${backgroundURL}")`,
          "--ui-term-tint": String(tint ?? defaultTint),
        }
      : {}),
  } as CSSProperties;

  return (
    <div className="overflow-hidden rounded-md border border-control-line bg-term-bg shadow-sm" aria-hidden="true">
      <div className="flex items-center gap-1.5 border-b border-white/10 bg-black/20 px-3 py-2">
        <span className="h-2 w-2 rounded-full bg-danger" />
        <span className="h-2 w-2 rounded-full bg-notice-ink" />
        <span className="h-2 w-2 rounded-full bg-live" />
        <span className="ml-2 font-mono text-[10px] text-white/60">sshc · preview</span>
      </div>
      <div
        data-terminal-preview=""
        {...(palette === "" ? {} : { "data-term-palette": palette })}
        {...(font === "" ? {} : { "data-term-font": font })}
        {...(hasBackground ? { "data-term-background": background } : {})}
        style={style}
        className="min-h-40 bg-term-bg p-4 font-mono leading-relaxed"
      >
        <p>
          <span style={{ color: "var(--ui-term-green)" }}>workspace</span>{" "}
          <span style={{ color: "var(--ui-term-blue)" }}>main</span>
        </p>
        <p>
          <span style={{ color: "var(--ui-term-cyan)" }}>$</span> ssh example.com
        </p>
        <p style={{ color: "var(--ui-term-bright-black)" }}>Connected to example.com</p>
        <p>
          <span style={{ color: "var(--ui-term-cyan)" }}>$</span>{" "}
          <span className="inline-block h-[1em] w-[0.5em] translate-y-[0.12em] bg-current" />
        </p>
      </div>
    </div>
  );
}
