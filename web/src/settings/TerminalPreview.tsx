import type { CSSProperties } from "react";
import { defaultTint } from "../terminal/appearance";
import { useBackgroundImage } from "../terminal/backgroundImage";
import { fontStack } from "../terminal/fonts";
import { fontSizeRange } from "../terminal/fontSize";

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
  const previewSize = Number.isFinite(chosenSize) && chosenSize >= fontSizeRange.min && chosenSize <= fontSizeRange.max ? chosenSize : 13;
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

  // One dark box as in the design: a bar with the window's three dots, then the lines. The whole box takes
  // the chosen palette and font, so the bar's faint text changes with them too.
  return (
    <div
      aria-hidden="true"
      data-terminal-preview=""
      {...(palette === "" ? {} : { "data-term-palette": palette })}
      {...(font === "" ? {} : { "data-term-font": font })}
      {...(hasBackground ? { "data-term-background": background } : {})}
      style={style}
      className="overflow-hidden rounded-[10px] border border-line bg-term-bg px-3.5 py-2.5 font-mono leading-[1.7]"
    >
      <div className="mb-1.5 flex items-center gap-1.5 text-[11px]" style={{ color: "var(--ui-term-bright-black)" }}>
        <span className="size-2 rounded-full bg-danger" />
        <span className="size-2 rounded-full bg-notice-ink" />
        <span className="size-2 rounded-full bg-live" />
        <span>sshc · preview</span>
      </div>
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
  );
}
