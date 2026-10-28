import { useTranslate } from "../i18n/context";
import { hintText } from "../ui/form";
import { Button } from "../ui/surface";
import type { ExecutableDirective } from "./executableDirectives";

// ExecutableDirectiveConfirmation shows the exact commands a connection would run, and asks before a check
// that would run them (unavoidableDirectives).
export function ExecutableDirectiveConfirmation({ directives, busy, onAcknowledge }: {
  directives: readonly ExecutableDirective[];
  busy: boolean;
  onAcknowledge: () => void;
}) {
  const t = useTranslate();
  return (
    <div className="rounded-[9px] bg-notice p-3 text-sm">
      <h4 className="font-medium text-notice-ink">{t("conn.checksExecutableHeading")}</h4>
      <p className={hintText}>{t("conn.checksExecutableHint")}</p>
      <ul className="mt-2 flex flex-col gap-2">
        {directives.map((directive) => (
          <li key={`${directive.path}:${directive.line}:${directive.keyword}`}>
            <p className="text-ink-muted">
              {t("conn.checksDirectiveAt", { keyword: directive.keyword, path: directive.path, line: directive.line })}
            </p>
            <pre className="whitespace-pre-wrap break-all text-xs text-ink">{directive.command}</pre>
          </li>
        ))}
      </ul>
      <Button className="mt-3" disabled={busy} busy={busy} onClick={onAcknowledge}>
        {t("conn.checksAcknowledge")}
      </Button>
    </div>
  );
}
